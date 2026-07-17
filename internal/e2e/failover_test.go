package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"

	businessaccess "github.com/ilyast91/CLIProxyNew/internal/access"
	"github.com/ilyast91/CLIProxyNew/internal/auth/identity"
	"github.com/ilyast91/CLIProxyNew/internal/auth/selector"
	"github.com/ilyast91/CLIProxyNew/internal/config"
	"github.com/ilyast91/CLIProxyNew/internal/httpapi"
	"github.com/ilyast91/CLIProxyNew/internal/security"
	"github.com/ilyast91/CLIProxyNew/internal/store"
	"github.com/ilyast91/CLIProxyNew/internal/usage"
	"github.com/ilyast91/CLIProxyNew/internal/watcher"
	"github.com/jackc/pgx/v5/pgxpool"
	sdkaccess "github.com/router-for-me/CLIProxyAPI/v7/sdk/access"
	sdkapi "github.com/router-for-me/CLIProxyAPI/v7/sdk/api"
	sdkauth "github.com/router-for-me/CLIProxyAPI/v7/sdk/auth"
	"github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy"
	coreauth "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/auth"
)

const (
	replicaProcessEnv        = "CLIPROXY_E2E_REPLICA_PROCESS"
	replicaDSNEnv            = "CLIPROXY_E2E_REPLICA_DSN"
	replicaAddrEnv           = "CLIPROXY_E2E_REPLICA_ADDR"
	replicaConfigPathEnv     = "CLIPROXY_E2E_REPLICA_CONFIG_PATH"
	replicaAuthDirEnv        = "CLIPROXY_E2E_REPLICA_AUTH_DIR"
	replicaReadyFileEnv      = "CLIPROXY_E2E_REPLICA_READY_FILE"
	replicaProcessTestTarget = "^TestRuntimeReplicaProcess$"
)

func TestIntegrationRuntimeReplicaFailover(t *testing.T) {
	if testing.Short() {
		t.Skip("integration replica failover requires Docker")
	}

	dsn, pool := newPostgresDatabase(t)
	seedFailoverRuntime(t, pool)
	first := startRuntimeReplicaProcess(t, "first", dsn)
	second := startRuntimeReplicaProcess(t, "second", dsn)

	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("создать failover cookie jar: %v", err)
	}
	client := &http.Client{Jar: jar, Timeout: e2eHTTPClientTimeout}

	var login struct {
		UserID int64 `json:"user_id"`
	}
	requireReplicaStatus(t, client, first.baseURL, http.MethodPost, "/api/v1/login", "", map[string]string{
		"username": "admin", "password": "admin-password",
	}, &login, http.StatusOK)
	if login.UserID <= 0 {
		t.Fatalf("failover login user_id = %d", login.UserID)
	}

	var created struct {
		Key string `json:"key"`
	}
	requireReplicaStatus(t, client, first.baseURL, http.MethodPost, "/api/v1/me/keys", "", map[string]any{
		"name": "replica-failover", "scope": map[string]any{"inference": true},
	}, &created, http.StatusCreated)
	if created.Key == "" {
		t.Fatal("failover API-key is empty")
	}

	var current struct {
		UserID int64 `json:"user_id"`
	}
	requireReplicaStatus(t, client, second.baseURL, http.MethodGet, "/api/v1/me", "", nil, &current, http.StatusOK)
	if current.UserID != login.UserID {
		t.Fatalf("second replica session user_id = %d, want %d", current.UserID, login.UserID)
	}
	requireReplicaInference(t, client, first.baseURL, created.Key)

	first.stopProcess()
	waitForReplicaUnavailable(t, first.baseURL)

	requireReplicaStatus(t, client, second.baseURL, http.MethodGet, "/healthz", "", nil, nil, http.StatusOK)
	current = struct {
		UserID int64 `json:"user_id"`
	}{}
	requireReplicaStatus(t, client, second.baseURL, http.MethodGet, "/api/v1/me", "", nil, &current, http.StatusOK)
	if current.UserID != login.UserID {
		t.Fatalf("session after failover user_id = %d, want %d", current.UserID, login.UserID)
	}
	requireReplicaInference(t, client, second.baseURL, created.Key)
}

// TestRuntimeReplicaProcess запускается только как subprocess chaos-теста.
func TestRuntimeReplicaProcess(t *testing.T) {
	if os.Getenv(replicaProcessEnv) != "1" {
		return
	}

	dsn := os.Getenv(replicaDSNEnv)
	addr := os.Getenv(replicaAddrEnv)
	configPath := os.Getenv(replicaConfigPathEnv)
	authDir := os.Getenv(replicaAuthDirEnv)
	readyFile := os.Getenv(replicaReadyFileEnv)
	if dsn == "" || addr == "" || configPath == "" || authDir == "" || readyFile == "" {
		t.Fatal("replica helper environment is incomplete")
	}

	ctx, cancelSetup := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancelSetup()
	pool, err := store.OpenPool(ctx, dsn)
	if err != nil {
		t.Fatalf("открыть replica pool: %v", err)
	}
	defer pool.Close()
	keyring := newFailoverKeyring(t)
	users := store.NewUserRepository(pool)
	sessions := store.NewSessionRepository(pool)
	apiKeys := store.NewAPIKeyRepository(pool)
	authStore := store.NewCoreAuthStore(pool, keyring)
	overrides := store.NewModelOverrideRepository(pool)
	staticProvider, err := identity.NewStaticProvider("admin", "admin-password", identity.RoleAdmin)
	if err != nil {
		t.Fatalf("создать replica identity provider: %v", err)
	}
	loginService := identity.NewLoginService(staticProvider, identity.SourceStatic, users, sessions)
	sessionAuthenticator := identity.NewCachedSessionAuthenticator(sessions, identity.SourceStatic, time.Second)
	resultHook := usage.NewHook()
	coreManager := coreauth.NewManager(authStore, selector.New(overrides), resultHook)

	sdkaccess.ClearExclusiveProvider()
	sdkaccess.UnregisterProvider(businessaccess.ProviderIdentifier)
	principalQueue := make(chan string, 16)
	sdkaccess.RegisterProvider(businessaccess.ProviderIdentifier, &capturingAccessProvider{
		delegate: businessaccess.NewProvider(apiKeys, identity.SourceStatic), principals: principalQueue,
	})
	sdkaccess.SetExclusiveProvider(businessaccess.ProviderIdentifier)
	sdkauth.RegisterTokenStore(authStore)

	businessConfig := config.Default()
	businessConfig.Server.Addr = addr
	businessConfig.Server.Environment = config.EnvironmentTest
	businessConfig.Auth.Mode = config.AuthModeStatic
	sdkConfig, err := businessConfig.SDKConfig()
	if err != nil {
		t.Fatalf("создать replica SDK config: %v", err)
	}
	sdkConfig.AuthDir = authDir

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
				httpapi.NewUsageHandler(store.NewUsageEventRepository(pool)),
				httpapi.NewAdminUserHandler(store.NewAdminUserRepository(pool), sessionAuthenticator, apiKeys),
				httpapi.NewAdminAPIKeyHandler(apiKeys),
				nil, nil, nil, nil, nil, nil,
			)),
		).
		Build()
	if err != nil {
		t.Fatalf("собрать replica SDK service: %v", err)
	}

	runCtx, cancelRun := context.WithCancel(context.Background())
	defer cancelRun()
	runErr := make(chan error, 1)
	go func() { runErr <- service.Run(runCtx) }()
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = service.Shutdown(shutdownCtx)
	}()

	baseURL := "http://" + addr
	waitForHealthyService(t, baseURL, runErr)
	waitForProviderRuntime(t, coreManager, "claude", e2eModel, runErr)
	coreManager.RegisterExecutor(fakeClaudeExecutor{principals: principalQueue})
	if err := os.WriteFile(readyFile, []byte("ready\n"), 0o600); err != nil {
		t.Fatalf("записать replica readiness marker: %v", err)
	}

	if err := <-runErr; err != nil && !errors.Is(err, context.Canceled) {
		t.Fatalf("replica Service.Run завершился: %v", err)
	}
}

func seedFailoverRuntime(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := store.NewModelOverrideRepository(pool).Upsert(ctx, store.UpsertModelOverrideParams{
		Provider: "claude", ModelAlias: e2eModel, UpstreamModel: e2eModel, Enabled: true,
	}); err != nil {
		t.Fatalf("создать failover model override: %v", err)
	}
	if _, err := store.NewCoreAuthStore(pool, newFailoverKeyring(t)).Save(ctx, &coreauth.Auth{
		ID:       "failover-claude-account",
		Provider: "claude",
		Label:    "Failover upstream",
		Status:   coreauth.StatusActive,
		Attributes: map[string]string{
			coreauth.AttributeAPIKey:   "failover-upstream-secret",
			coreauth.AttributeAuthKind: coreauth.AuthKindAPIKey,
		},
		Metadata: map[string]any{"type": "claude"},
	}); err != nil {
		t.Fatalf("сохранить failover upstream auth: %v", err)
	}
}

func newFailoverKeyring(t *testing.T) *security.Keyring {
	t.Helper()
	keyring, err := security.NewKeyring(1, map[int][]byte{1: bytes.Repeat([]byte{0x42}, security.AES256KeySize)})
	if err != nil {
		t.Fatalf("создать failover keyring: %v", err)
	}
	return keyring
}

type runtimeReplicaProcess struct {
	baseURL string
	command *exec.Cmd
	output  bytes.Buffer
	done    chan struct{}
	mu      sync.Mutex
	waitErr error
	stop    sync.Once
}

func startRuntimeReplicaProcess(t *testing.T, name, dsn string) *runtimeReplicaProcess {
	t.Helper()
	addr := reserveLoopbackAddress(t)
	directory := t.TempDir()
	configPath := filepath.Join(directory, "config.yaml")
	if err := os.WriteFile(configPath, []byte("# replica failover config\n"), 0o600); err != nil {
		t.Fatalf("создать replica config path: %v", err)
	}
	readyFile := filepath.Join(directory, "ready")
	command := exec.Command(os.Args[0], "-test.run="+replicaProcessTestTarget, "-test.timeout=5m")
	command.Env = append(os.Environ(),
		replicaProcessEnv+"=1",
		replicaDSNEnv+"="+dsn,
		replicaAddrEnv+"="+addr,
		replicaConfigPathEnv+"="+configPath,
		replicaAuthDirEnv+"="+directory,
		replicaReadyFileEnv+"="+readyFile,
	)
	replica := &runtimeReplicaProcess{
		baseURL: "http://" + addr,
		command: command,
		done:    make(chan struct{}),
	}
	command.Stdout = &replica.output
	command.Stderr = &replica.output
	if err := command.Start(); err != nil {
		t.Fatalf("запустить replica process %s: %v", name, err)
	}
	go func() {
		err := command.Wait()
		replica.mu.Lock()
		replica.waitErr = err
		replica.mu.Unlock()
		close(replica.done)
	}()
	t.Cleanup(replica.stopProcess)
	waitForReplicaProcessReady(t, name, replica, readyFile)
	return replica
}

func (p *runtimeReplicaProcess) stopProcess() {
	p.stop.Do(func() {
		select {
		case <-p.done:
			return
		default:
		}
		_ = p.command.Process.Kill()
		<-p.done
	})
}

func waitForReplicaProcessReady(t *testing.T, name string, replica *runtimeReplicaProcess, readyFile string) {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(readyFile); err == nil {
			return
		}
		select {
		case <-replica.done:
			replica.mu.Lock()
			waitErr := replica.waitErr
			replica.mu.Unlock()
			t.Fatalf("replica process %s exited before readiness: %v\n%s", name, waitErr, replica.output.String())
		default:
		}
		time.Sleep(50 * time.Millisecond)
	}
	replica.stopProcess()
	t.Fatalf("replica process %s did not become ready\n%s", name, replica.output.String())
}

func requireReplicaStatus(t *testing.T, client *http.Client, baseURL, method, path, bearer string, requestBody, responseBody any, want int) {
	t.Helper()
	status, payload, err := replicaRequestJSON(client, baseURL, method, path, bearer, requestBody, responseBody)
	if err != nil {
		t.Fatalf("%s %s request failed: %v", method, path, err)
	}
	if status != want {
		t.Fatalf("%s %s status=%d want=%d body=%s", method, path, status, want, payload)
	}
}

func replicaRequestJSON(client *http.Client, baseURL, method, path, bearer string, requestBody, responseBody any) (int, []byte, error) {
	var body io.Reader
	if requestBody != nil {
		encoded, err := json.Marshal(requestBody)
		if err != nil {
			return 0, nil, fmt.Errorf("marshal request: %w", err)
		}
		body = bytes.NewReader(encoded)
	}
	request, err := http.NewRequest(method, baseURL+path, body)
	if err != nil {
		return 0, nil, fmt.Errorf("create request: %w", err)
	}
	if requestBody != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	if bearer != "" {
		request.Header.Set("Authorization", "Bearer "+bearer)
	}
	response, err := client.Do(request)
	if err != nil {
		return 0, nil, err
	}
	defer response.Body.Close()
	payload, err := io.ReadAll(response.Body)
	if err != nil {
		return 0, nil, fmt.Errorf("read response: %w", err)
	}
	if responseBody != nil && len(payload) > 0 {
		if err := json.Unmarshal(payload, responseBody); err != nil {
			return response.StatusCode, payload, fmt.Errorf("decode response: %w", err)
		}
	}
	return response.StatusCode, payload, nil
}

func requireReplicaInference(t *testing.T, client *http.Client, baseURL, apiKey string) {
	t.Helper()
	var response map[string]any
	requireReplicaStatus(t, client, baseURL, http.MethodPost, "/v1/chat/completions", apiKey, map[string]any{
		"model": e2eModel, "messages": []map[string]string{{"role": "user", "content": "ping"}}, "stream": false,
	}, &response, http.StatusOK)
	choices, ok := response["choices"].([]any)
	if !ok || len(choices) == 0 {
		t.Fatalf("replica inference response has no choices: %#v", response)
	}
}

func waitForReplicaUnavailable(t *testing.T, baseURL string) {
	t.Helper()
	client := &http.Client{Timeout: 250 * time.Millisecond}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		response, err := client.Get(baseURL + "/healthz")
		if err != nil {
			return
		}
		response.Body.Close()
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("stopped replica %s is still reachable", baseURL)
}
