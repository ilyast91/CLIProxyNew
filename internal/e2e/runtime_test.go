package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/golang-migrate/migrate/v4"
	migratepgx "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	businessaccess "github.com/ilyast91/CLIProxyNew/internal/access"
	"github.com/ilyast91/CLIProxyNew/internal/auth/identity"
	"github.com/ilyast91/CLIProxyNew/internal/auth/selector"
	"github.com/ilyast91/CLIProxyNew/internal/config"
	"github.com/ilyast91/CLIProxyNew/internal/httpapi"
	"github.com/ilyast91/CLIProxyNew/internal/security"
	"github.com/ilyast91/CLIProxyNew/internal/store"
	"github.com/ilyast91/CLIProxyNew/internal/usage"
	"github.com/ilyast91/CLIProxyNew/internal/watcher"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	sdkaccess "github.com/router-for-me/CLIProxyAPI/v7/sdk/access"
	sdkapi "github.com/router-for-me/CLIProxyAPI/v7/sdk/api"
	sdkauth "github.com/router-for-me/CLIProxyAPI/v7/sdk/auth"
	"github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy"
	coreauth "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/auth"
	coreexecutor "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/executor"
	sdkusage "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/usage"
	"github.com/testcontainers/testcontainers-go"
	postgrescontainer "github.com/testcontainers/testcontainers-go/modules/postgres"
)

const postgresImage = "postgres@sha256:742f40ea20b9ff2ff31db5458d127452988a2164df9e17441e191f3b72252193"
const e2eModel = "claude-sonnet-4-5-20250929"

type runtimeHarness struct {
	baseURL   string
	client    *http.Client
	pool      *pgxpool.Pool
	usageRepo *store.UsageEventRepository
}

func TestIntegrationRuntimeLoginKeyInferenceUsageAdmin(t *testing.T) {
	if testing.Short() {
		t.Skip("integration E2E requires Docker")
	}

	harness := newRuntimeHarness(t)
	harness.verifyLoginKeyInferenceUsageAdmin(t)
}

func newRuntimeHarness(t *testing.T) *runtimeHarness {
	t.Helper()

	pool := newPostgresPool(t)
	keyring, err := security.NewKeyring(1, map[int][]byte{1: bytes.Repeat([]byte{0x42}, security.AES256KeySize)})
	if err != nil {
		t.Fatalf("создать E2E keyring: %v", err)
	}

	users := store.NewUserRepository(pool)
	sessions := store.NewSessionRepository(pool)
	apiKeys := store.NewAPIKeyRepository(pool)
	usageRepo := store.NewUsageEventRepository(pool)
	authStore := store.NewCoreAuthStore(pool, keyring)
	overrides := store.NewModelOverrideRepository(pool)
	staticProvider, err := identity.NewStaticProvider("admin", "admin-password", identity.RoleAdmin)
	if err != nil {
		t.Fatalf("создать static identity provider: %v", err)
	}
	loginService := identity.NewLoginService(staticProvider, identity.SourceStatic, users, sessions)
	sessionAuthenticator := identity.NewCachedSessionAuthenticator(sessions, identity.SourceStatic, time.Second)
	resultHook := usage.NewHook()

	ctx, cancelSetup := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancelSetup()
	if _, err := overrides.Upsert(ctx, store.UpsertModelOverrideParams{
		Provider: "claude", ModelAlias: e2eModel, UpstreamModel: e2eModel, Enabled: true,
	}); err != nil {
		t.Fatalf("создать E2E model override: %v", err)
	}
	coreManager := coreauth.NewManager(authStore, selector.New(overrides), resultHook)
	_, err = coreManager.Register(ctx, &coreauth.Auth{
		ID:       "e2e-claude-account",
		Provider: "claude",
		Label:    "E2E upstream",
		Status:   coreauth.StatusActive,
		Attributes: map[string]string{
			coreauth.AttributeAPIKey:   "e2e-upstream-secret",
			coreauth.AttributeAuthKind: coreauth.AuthKindAPIKey,
		},
		Metadata: map[string]any{"type": "claude"},
	})
	if err != nil {
		t.Fatalf("зарегистрировать E2E upstream auth: %v", err)
	}

	sdkaccess.ClearExclusiveProvider()
	sdkaccess.UnregisterProvider(businessaccess.ProviderIdentifier)
	principalQueue := make(chan string, 1)
	sdkaccess.RegisterProvider(businessaccess.ProviderIdentifier, &capturingAccessProvider{
		delegate:   businessaccess.NewProvider(apiKeys, identity.SourceStatic),
		principals: principalQueue,
	})
	sdkaccess.SetExclusiveProvider(businessaccess.ProviderIdentifier)
	t.Cleanup(func() {
		sdkaccess.ClearExclusiveProvider()
		sdkaccess.UnregisterProvider(businessaccess.ProviderIdentifier)
	})
	sdkauth.RegisterTokenStore(authStore)

	addr := reserveLoopbackAddress(t)
	businessConfig := config.Default()
	businessConfig.Server.Addr = addr
	businessConfig.Server.Environment = config.EnvironmentTest
	businessConfig.Auth.Mode = config.AuthModeStatic
	sdkConfig, err := businessConfig.SDKConfig()
	if err != nil {
		t.Fatalf("создать SDK config: %v", err)
	}
	sdkConfig.AuthDir = t.TempDir()

	configPath := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(configPath, []byte("# E2E config path\n"), 0o600); err != nil {
		t.Fatalf("создать временный config path: %v", err)
	}

	usagePlugin := usage.NewBufferedPlugin(usageRepo)
	service, err := cliproxy.NewBuilder().
		WithConfig(sdkConfig).
		WithConfigPath(configPath).
		WithCoreAuthManager(coreManager).
		WithWatcherFactory(watcher.NoopFactory).
		WithServerOptions(
			sdkapi.WithRouterConfigurator(httpapi.SystemRouterConfigurator(pool)),
			sdkapi.WithRouterConfigurator(httpapi.RouterConfigurator(
				httpapi.NewLoginHandler(loginService, false),
				sessionAuthenticator,
				nil,
				httpapi.NewAPIKeyHandler(apiKeys),
				httpapi.NewUsageHandler(usageRepo),
				httpapi.NewAdminUserHandler(store.NewAdminUserRepository(pool), sessionAuthenticator),
				httpapi.NewAdminAPIKeyHandler(apiKeys),
				nil, nil, nil, nil, nil, nil,
			)),
		).
		Build()
	if err != nil {
		t.Fatalf("собрать SDK service: %v", err)
	}
	service.RegisterUsagePlugin(usagePlugin)

	runCtx, cancelRun := context.WithCancel(context.Background())
	runErr := make(chan error, 1)
	go func() { runErr <- service.Run(runCtx) }()

	t.Cleanup(func() {
		cancelRun()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := service.Shutdown(shutdownCtx); err != nil {
			t.Errorf("остановить SDK service: %v", err)
		}
		select {
		case err := <-runErr:
			if err != nil && !errors.Is(err, context.Canceled) {
				t.Errorf("Service.Run завершился с ошибкой: %v", err)
			}
		case <-shutdownCtx.Done():
			t.Errorf("дождаться Service.Run: %v", shutdownCtx.Err())
		}
		if err := usagePlugin.Close(shutdownCtx); err != nil {
			t.Errorf("закрыть usage plugin: %v", err)
		}
	})

	baseURL := "http://" + addr
	waitForHealthyService(t, baseURL, runErr)
	waitForProviderRuntime(t, coreManager, "claude", e2eModel, runErr)
	coreManager.RegisterExecutor(fakeClaudeExecutor{principals: principalQueue})

	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("создать cookie jar: %v", err)
	}
	return &runtimeHarness{
		baseURL: baseURL,
		client:  &http.Client{Jar: jar, Timeout: 5 * time.Second},
		pool:    pool, usageRepo: usageRepo,
	}
}

func (h *runtimeHarness) verifyLoginKeyInferenceUsageAdmin(t *testing.T) {
	t.Helper()

	var login struct {
		UserID int64  `json:"user_id"`
		Role   string `json:"role"`
	}
	status := h.requestJSON(t, http.MethodPost, "/api/v1/login", "", map[string]string{
		"username": "admin", "password": "admin-password",
	}, &login)
	if status != http.StatusOK || login.UserID <= 0 || login.Role != identity.RoleAdmin {
		t.Fatalf("login response: status=%d user_id=%d role=%q", status, login.UserID, login.Role)
	}

	var createdKey struct {
		APIKey struct {
			ID     int64  `json:"id"`
			Prefix string `json:"prefix"`
		} `json:"api_key"`
		Key string `json:"key"`
	}
	status = h.requestJSON(t, http.MethodPost, "/api/v1/me/keys", "", map[string]any{
		"name": "runtime-e2e", "scope": map[string]any{"inference": true},
	}, &createdKey)
	if status != http.StatusCreated || createdKey.APIKey.ID <= 0 || createdKey.Key == "" {
		t.Fatalf("create API-key response: status=%d id=%d key_empty=%t", status, createdKey.APIKey.ID, createdKey.Key == "")
	}
	if len(createdKey.Key) < store.APIKeyPrefixLength || createdKey.APIKey.Prefix != createdKey.Key[:store.APIKeyPrefixLength] {
		t.Fatalf("API-key prefix = %q, key = %q", createdKey.APIKey.Prefix, createdKey.Key)
	}
	var keyHash string
	if err := h.pool.QueryRow(context.Background(), "SELECT key_hash FROM api_keys WHERE id = $1", createdKey.APIKey.ID).Scan(&keyHash); err != nil {
		t.Fatalf("прочитать сохранённый API-key hash: %v", err)
	}
	if keyHash == createdKey.Key || !security.VerifySecret(keyHash, createdKey.Key) {
		t.Fatal("API-key должен храниться только как проверяемый bcrypt hash")
	}

	var inference map[string]any
	status = h.requestJSON(t, http.MethodPost, "/v1/chat/completions", createdKey.Key, map[string]any{
		"model":    e2eModel,
		"messages": []map[string]string{{"role": "user", "content": "ping"}},
		"stream":   false,
	}, &inference)
	if status != http.StatusOK {
		t.Fatalf("inference status = %d, response = %#v", status, inference)
	}
	choices, ok := inference["choices"].([]any)
	if !ok || len(choices) == 0 {
		t.Fatalf("inference response does not contain choices: %#v", inference)
	}

	summary := waitForUsageSummary(t, h.usageRepo, login.UserID)
	if summary.RequestCount != 1 || summary.InputTokens != 5 || summary.OutputTokens != 7 || summary.TotalTokens != 12 {
		t.Fatalf("usage summary = %+v", summary)
	}
	if len(summary.ByModel) != 1 || summary.ByModel[0].Model != e2eModel || summary.ByModel[0].TotalTokens != 12 {
		t.Fatalf("usage by model = %+v", summary.ByModel)
	}
	if len(summary.ByAPIKey) != 1 || summary.ByAPIKey[0].APIKeyID != createdKey.APIKey.ID {
		t.Fatalf("usage by API-key = %+v", summary.ByAPIKey)
	}

	var usageResponse struct {
		RequestCount int64 `json:"request_count"`
		TotalTokens  int64 `json:"total_tokens"`
	}
	status = h.requestJSON(t, http.MethodGet, "/api/v1/me/usage", "", nil, &usageResponse)
	if status != http.StatusOK || usageResponse.RequestCount != 1 || usageResponse.TotalTokens != 12 {
		t.Fatalf("management usage response: status=%d response=%+v", status, usageResponse)
	}

	var adminUsers struct {
		Data []struct {
			ID             int64  `json:"id"`
			Role           string `json:"role"`
			IdentitySource string `json:"identity_source"`
		} `json:"data"`
	}
	status = h.requestJSON(t, http.MethodGet, "/api/v1/admin/users", "", nil, &adminUsers)
	if status != http.StatusOK || len(adminUsers.Data) != 1 || adminUsers.Data[0].ID != login.UserID || adminUsers.Data[0].Role != identity.RoleAdmin || adminUsers.Data[0].IdentitySource != identity.SourceStatic {
		t.Fatalf("admin users response: status=%d data=%+v", status, adminUsers.Data)
	}

	var adminKeys struct {
		Data []struct {
			ID       int64  `json:"id"`
			UserID   int64  `json:"user_id"`
			Prefix   string `json:"prefix"`
			Username string `json:"owner_username"`
		} `json:"data"`
	}
	status = h.requestJSON(t, http.MethodGet, "/api/v1/admin/keys", "", nil, &adminKeys)
	if status != http.StatusOK || len(adminKeys.Data) != 1 || adminKeys.Data[0].ID != createdKey.APIKey.ID || adminKeys.Data[0].UserID != login.UserID || adminKeys.Data[0].Prefix != createdKey.APIKey.Prefix {
		t.Fatalf("admin keys response: status=%d data=%+v", status, adminKeys.Data)
	}

	h.verifyAdminRoleGuard(t)
}

func (h *runtimeHarness) verifyAdminRoleGuard(t *testing.T) {
	t.Helper()

	withoutSession := *h
	withoutSession.client = &http.Client{Timeout: 5 * time.Second}
	if status := withoutSession.requestJSON(t, http.MethodGet, "/api/v1/admin/users", "", nil, nil); status != http.StatusUnauthorized {
		t.Fatalf("anonymous admin status = %d, want %d", status, http.StatusUnauthorized)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	user, err := store.NewUserRepository(h.pool).UpsertStatic(ctx, store.UpsertUserParams{
		Username: "static:runtime-user", Email: "runtime-user@example.test", Role: identity.RoleUser,
	})
	if err != nil {
		t.Fatalf("создать E2E user для role guard: %v", err)
	}
	const userToken = "runtime-user-session"
	if _, err := store.NewSessionRepository(h.pool).Create(ctx, store.CreateSessionParams{
		UserID: user.ID, Token: userToken, Role: identity.RoleUser, ExpiresAt: time.Now().Add(time.Hour),
	}); err != nil {
		t.Fatalf("создать E2E user session: %v", err)
	}

	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("создать user cookie jar: %v", err)
	}
	baseURL, err := url.Parse(h.baseURL)
	if err != nil {
		t.Fatalf("разобрать E2E base URL: %v", err)
	}
	jar.SetCookies(baseURL, []*http.Cookie{{Name: identity.SessionCookieName, Value: userToken, Path: "/"}})
	userSession := *h
	userSession.client = &http.Client{Jar: jar, Timeout: 5 * time.Second}
	if status := userSession.requestJSON(t, http.MethodGet, "/api/v1/admin/users", "", nil, nil); status != http.StatusForbidden {
		t.Fatalf("user admin status = %d, want %d", status, http.StatusForbidden)
	}
}

func (h *runtimeHarness) requestJSON(t *testing.T, method, path, bearer string, requestBody, responseBody any) int {
	t.Helper()

	var body io.Reader
	if requestBody != nil {
		encoded, err := json.Marshal(requestBody)
		if err != nil {
			t.Fatalf("marshal %s %s request: %v", method, path, err)
		}
		body = bytes.NewReader(encoded)
	}
	request, err := http.NewRequestWithContext(context.Background(), method, h.baseURL+path, body)
	if err != nil {
		t.Fatalf("создать %s %s request: %v", method, path, err)
	}
	if requestBody != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	if bearer != "" {
		request.Header.Set("Authorization", "Bearer "+bearer)
	}
	response, err := h.client.Do(request)
	if err != nil {
		t.Fatalf("выполнить %s %s request: %v", method, path, err)
	}
	defer response.Body.Close()
	payload, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("прочитать %s %s response: %v", method, path, err)
	}
	if responseBody != nil && len(payload) > 0 {
		if err := json.Unmarshal(payload, responseBody); err != nil {
			t.Fatalf("decode %s %s response %q: %v", method, path, payload, err)
		}
	}
	return response.StatusCode
}

func waitForUsageSummary(t *testing.T, repository *store.UsageEventRepository, userID int64) store.UsageSummary {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		now := time.Now().UTC()
		summary, err := repository.GetSummaryByUser(context.Background(), userID, now.Add(-time.Hour), now.Add(time.Hour))
		if err != nil {
			t.Fatalf("прочитать usage summary: %v", err)
		}
		if summary.RequestCount > 0 {
			return summary
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatal("usage event не появился до deadline")
	return store.UsageSummary{}
}

func waitForHealthyService(t *testing.T, baseURL string, runErr <-chan error) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	client := &http.Client{Timeout: 500 * time.Millisecond}
	for time.Now().Before(deadline) {
		select {
		case err := <-runErr:
			t.Fatalf("Service.Run завершился до readiness: %v", err)
		default:
		}
		response, err := client.Get(baseURL + "/healthz")
		if err == nil {
			response.Body.Close()
			if response.StatusCode == http.StatusOK {
				return
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("SDK service не стал healthy до deadline")
}

func waitForProviderRuntime(t *testing.T, manager *coreauth.Manager, provider, model string, runErr <-chan error) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		select {
		case err := <-runErr:
			t.Fatalf("Service.Run завершился до provider runtime readiness: %v", err)
		default:
		}
		_, hasExecutor := manager.Executor(provider)
		models := cliproxy.GlobalModelRegistry().GetAvailableModelsByProvider(provider)
		for _, available := range models {
			if hasExecutor && available != nil && available.ID == model {
				return
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("provider runtime %q model %q не стал ready до deadline", provider, model)
}

func reserveLoopbackAddress(t *testing.T) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("зарезервировать loopback port: %v", err)
	}
	address := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatalf("освободить loopback port: %v", err)
	}
	return address
}

func newPostgresPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	container, err := postgrescontainer.Run(ctx, postgresImage,
		postgrescontainer.WithDatabase("cliproxy_e2e"),
		postgrescontainer.WithUsername("cliproxy"),
		postgrescontainer.WithPassword("cliproxy"),
		postgrescontainer.BasicWaitStrategies(),
	)
	testcontainers.CleanupContainer(t, container)
	if err != nil {
		t.Fatalf("запустить PostgreSQL testcontainer: %v", err)
	}
	dsn, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("получить PostgreSQL DSN: %v", err)
	}

	applyMigrations(t, dsn)
	pool, err := store.OpenPool(ctx, dsn)
	if err != nil {
		t.Fatalf("открыть E2E pool: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func applyMigrations(t *testing.T, dsn string) {
	t.Helper()
	config, err := pgx.ParseConfig(dsn)
	if err != nil {
		t.Fatalf("разобрать E2E DSN: %v", err)
	}
	database := stdlib.OpenDB(*config)
	driver, err := migratepgx.WithInstance(database, &migratepgx.Config{DatabaseName: config.Database})
	if err != nil {
		database.Close()
		t.Fatalf("создать E2E migrate driver: %v", err)
	}
	migrationsPath, err := filepath.Abs(filepath.Join("..", "..", "db", "migrations"))
	if err != nil {
		database.Close()
		t.Fatalf("получить путь E2E миграций: %v", err)
	}
	sourceURL := (&url.URL{Scheme: "file", Path: migrationsPath}).String()
	migrator, err := migrate.NewWithDatabaseInstance(sourceURL, "pgx5", driver)
	if err != nil {
		database.Close()
		t.Fatalf("создать E2E migrator: %v", err)
	}
	if err := migrator.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		t.Fatalf("применить E2E миграции: %v", err)
	}
	if sourceErr, databaseErr := migrator.Close(); sourceErr != nil || databaseErr != nil {
		t.Fatalf("закрыть E2E migrator: source=%v database=%v", sourceErr, databaseErr)
	}
}

type capturingAccessProvider struct {
	delegate   sdkaccess.Provider
	principals chan<- string
}

func (p *capturingAccessProvider) Identifier() string { return p.delegate.Identifier() }

func (p *capturingAccessProvider) Authenticate(ctx context.Context, request *http.Request) (*sdkaccess.Result, *sdkaccess.AuthError) {
	result, authError := p.delegate.Authenticate(ctx, request)
	if authError == nil && result != nil {
		select {
		case p.principals <- result.Principal:
		default:
		}
	}
	return result, authError
}

type fakeClaudeExecutor struct {
	principals <-chan string
}

func (fakeClaudeExecutor) Identifier() string { return "claude" }

func (e fakeClaudeExecutor) Execute(ctx context.Context, auth *coreauth.Auth, request coreexecutor.Request, _ coreexecutor.Options) (coreexecutor.Response, error) {
	var principal string
	select {
	case principal = <-e.principals:
	case <-ctx.Done():
		return coreexecutor.Response{}, ctx.Err()
	}
	if principal == "" {
		return coreexecutor.Response{}, errors.New("access provider returned an empty principal")
	}
	model := request.Model
	alias := sdkusage.RequestedModelAliasFromContext(ctx)
	sdkusage.PublishRecord(ctx, sdkusage.Record{
		Provider: "claude", Model: model, Alias: alias, APIKey: principal, AuthID: auth.ID,
		Detail: sdkusage.Detail{InputTokens: 5, OutputTokens: 7, TotalTokens: 12},
	})
	return coreexecutor.Response{
		Payload: []byte(`{"id":"chatcmpl-e2e","object":"chat.completion","model":"claude-sonnet-4-5-20250929","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":5,"completion_tokens":7,"total_tokens":12}}`),
		Headers: http.Header{"Content-Type": []string{"application/json"}},
	}, nil
}

func (fakeClaudeExecutor) ExecuteStream(context.Context, *coreauth.Auth, coreexecutor.Request, coreexecutor.Options) (*coreexecutor.StreamResult, error) {
	return nil, fmt.Errorf("stream execution is not used by E2E")
}

func (fakeClaudeExecutor) Refresh(_ context.Context, auth *coreauth.Auth) (*coreauth.Auth, error) {
	return auth, nil
}

func (fakeClaudeExecutor) CountTokens(context.Context, *coreauth.Auth, coreexecutor.Request, coreexecutor.Options) (coreexecutor.Response, error) {
	return coreexecutor.Response{Payload: []byte(`{"input_tokens":5}`)}, nil
}

func (fakeClaudeExecutor) HttpRequest(context.Context, *coreauth.Auth, *http.Request) (*http.Response, error) {
	return nil, fmt.Errorf("metadata request is not used by E2E")
}
