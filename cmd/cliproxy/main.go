// Пакет main — точка входа CLIProxyNew.
//
// Здесь собирается инфраструктурный wiring: конфигурация, Postgres и SDK ядра.
// Полная интеграция Builder + Service.Run + 7 контрактов ADR-9 — фаза Ф3.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	businessaccess "github.com/ilyast91/CLIProxyNew/internal/access"
	"github.com/ilyast91/CLIProxyNew/internal/auth/identity"
	ldapidentity "github.com/ilyast91/CLIProxyNew/internal/auth/ldap"
	"github.com/ilyast91/CLIProxyNew/internal/auth/selector"
	authtesting "github.com/ilyast91/CLIProxyNew/internal/auth/testing"
	"github.com/ilyast91/CLIProxyNew/internal/config"
	"github.com/ilyast91/CLIProxyNew/internal/httpapi"
	"github.com/ilyast91/CLIProxyNew/internal/metrics"
	"github.com/ilyast91/CLIProxyNew/internal/modelregistry"
	"github.com/ilyast91/CLIProxyNew/internal/observability"
	openapidoc "github.com/ilyast91/CLIProxyNew/internal/openapi"
	"github.com/ilyast91/CLIProxyNew/internal/security"
	"github.com/ilyast91/CLIProxyNew/internal/store"
	"github.com/ilyast91/CLIProxyNew/internal/usage"
	"github.com/ilyast91/CLIProxyNew/internal/watcher"
	sdkaccess "github.com/router-for-me/CLIProxyAPI/v7/sdk/access"
	sdkapi "github.com/router-for-me/CLIProxyAPI/v7/sdk/api"
	sdkauth "github.com/router-for-me/CLIProxyAPI/v7/sdk/auth"
	cliproxy "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy"
	coreauth "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/auth"
)

const (
	appName    = "cliproxy"
	appVersion = "dev"

	shutdownTimeout = 30 * time.Second
)

type serviceShutdowner interface {
	Shutdown(context.Context) error
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "%s: %v\n", appName, err)
		os.Exit(1)
	}
}

// run — основная точка входа.
func run() error {
	// Конфигурация (R6): config.yaml (ConfigMap) + env-override (12-factor).
	configPath := os.Getenv("CLIPROXY_CONFIG")
	if configPath == "" {
		configPath = "config.yaml"
	}
	cfg, err := config.Load(configPath)
	if err != nil {
		if !errors.Is(err, config.ErrConfigNotFound) {
			return fmt.Errorf("load config: %w", err)
		}
		cfg = config.FromEnvironment()
		slog.Warn("config not found, using defaults", "path", configPath)
	}
	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("validate config: %w", err)
	}
	identityProvider, err := identityProviderFromConfig(cfg)
	if err != nil {
		return fmt.Errorf("create identity provider: %w", err)
	}

	logger := slog.New(observability.NewRedactingHandler(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})))
	slog.SetDefault(logger)

	slog.Info("starting",
		"app", appName,
		"version", appVersion,
		"server_addr", cfg.Server.Addr,
		"identity_source", cfg.Auth.Mode,
	)
	_ = identityProvider // Login HTTP wiring появится вместе с management router.

	// Graceful shutdown context.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	dbPool, err := store.OpenPool(ctx, cfg.DB.DSN)
	if err != nil {
		return fmt.Errorf("connect database: %w", err)
	}
	defer dbPool.Close()
	slog.Info("database connected")

	keyring, err := security.NewKeyringFromBase64(
		cfg.Encryption.KeyVersion,
		os.Getenv("CLIPROXY_ENCRYPTION_KEY"),
		os.Getenv("CLIPROXY_ENCRYPTION_PREVIOUS_KEYS"),
	)
	if err != nil {
		return fmt.Errorf("create encryption keyring: %w", err)
	}
	authStore := store.NewCoreAuthStore(dbPool, keyring)
	apiKeyStore := store.NewAPIKeyRepository(dbPool)
	sdkauth.RegisterTokenStore(authStore)
	cliproxy.SetGlobalModelRegistryHook(modelregistry.New(store.NewModelRegistrySnapshotRepository(dbPool)))
	apiKeyProvider := businessaccess.NewProvider(apiKeyStore, cfg.Auth.Mode)
	sdkaccess.RegisterProvider(businessaccess.ProviderIdentifier, apiKeyProvider)
	sdkaccess.SetExclusiveProvider(businessaccess.ProviderIdentifier)
	revisionPoller := watcher.NewRevisionPoller(
		store.NewRuntimeRevisionRepository(dbPool),
		store.UpstreamAccountsRevision,
		3*time.Second,
		15*time.Second,
		stop,
	)
	go revisionPoller.Run(ctx)
	users := store.NewUserRepository(dbPool)
	sessions := store.NewSessionRepository(dbPool)
	sessionCleanup := watcher.NewSessionCleanup(sessions, time.Minute)
	sessionLeader := watcher.NewLeaderRunner(
		watcher.NewPostgresAdvisoryLocker(dbPool, watcher.SessionCleanupLock),
		3*time.Second,
	)
	go sessionLeader.Run(ctx, sessionCleanup.Run)
	loginService := identity.NewLoginService(identityProvider, cfg.Auth.Mode, users, sessions)
	sessionAuthenticator := identity.NewSessionAuthenticator(sessions, cfg.Auth.Mode)
	sdkCfg, err := cfg.SDKConfig()
	if err != nil {
		return fmt.Errorf("build SDK config: %w", err)
	}
	resultHook := usage.NewHook()
	coreManager := coreauth.NewManager(authStore, selector.New(store.NewModelOverrideRepository(dbPool)), resultHook)
	usagePlugin := usage.NewBufferedPlugin(store.NewUsageEventRepository(dbPool))
	defer func() {
		flushCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := usagePlugin.Close(flushCtx); err != nil {
			slog.Error("flush usage events", "error", err)
		}
	}()
	metricsRegistry := metrics.NewRegistry(dbPool, resultHook, usagePlugin, apiKeyStore)
	service, err := cliproxy.NewBuilder().
		WithConfig(sdkCfg).
		WithConfigPath(configPath).
		WithCoreAuthManager(coreManager).
		WithWatcherFactory(watcher.NoopFactory).
		WithServerOptions(sdkapi.WithMiddleware(httpapi.RequestIDMiddleware(), httpapi.TracingMiddleware(), httpapi.RequestLogger(logger), httpapi.NewCORSMiddleware(cfg.Server.CORSAllowedOrigins), metricsRegistry.Middleware()), sdkapi.WithRouterConfigurator(httpapi.SystemRouterConfigurator(dbPool)), sdkapi.WithRouterConfigurator(httpapi.MetricsRouterConfigurator(metricsRegistry.Handler())), sdkapi.WithRouterConfigurator(httpapi.OpenAPIRouterConfigurator(openapidoc.Document())), sdkapi.WithRouterConfigurator(httpapi.RouterConfigurator(httpapi.NewLoginHandler(loginService, cfg.Server.Environment == config.EnvironmentProduction), sessionAuthenticator, httpapi.LogoutHandler(sessions, cfg.Auth.Mode), httpapi.NewAPIKeyHandler(apiKeyStore), httpapi.NewUsageHandler(store.NewUsageEventRepository(dbPool)), httpapi.NewAdminUserHandler(store.NewAdminUserRepository(dbPool)), httpapi.NewAdminAPIKeyHandler(apiKeyStore), httpapi.NewAdminOAuthSessionHandler(store.NewOAuthSessionRepository(dbPool)), httpapi.NewAdminProviderKeyHandler(coreManager), httpapi.NewAdminAccountTestHandler(authtesting.NewChecker(coreManager)), httpapi.NewAdminQuotaHandler(coreManager), httpapi.NewAdminOAuthCredentialHandler(coreManager, store.NewAdminAuditLogRepository(dbPool)), httpapi.NewAdminModelHandler(store.NewAdminModelRepository(dbPool))))).
		Build()
	if err != nil {
		return fmt.Errorf("build SDK service: %w", err)
	}
	service.RegisterUsagePlugin(usagePlugin)
	slog.Info("credential store registered", "key_version", cfg.Encryption.KeyVersion)
	slog.Info("API key provider registered", "identity_source", cfg.Auth.Mode)
	slog.Info("runtime revision poller started", "revision", store.UpstreamAccountsRevision)
	slog.Info("session cleanup leader election started", "lock_id", watcher.SessionCleanupLock)
	slog.Info("core auth result hook registered")

	slog.Info("SDK service ready")
	runErr := service.Run(ctx)
	shutdownErr := shutdownService(service)
	if runErr != nil && !errors.Is(runErr, context.Canceled) {
		return fmt.Errorf("run SDK service: %w", runErr)
	}
	if shutdownErr != nil {
		return fmt.Errorf("shutdown SDK service: %w", shutdownErr)
	}
	slog.Info("shutdown complete")
	return nil
}

// shutdownService ограничивает graceful shutdown SDK-сервиса установленным SLA.
func shutdownService(service serviceShutdowner) error {
	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	return service.Shutdown(shutdownCtx)
}

func identityProviderFromConfig(cfg *config.Config) (identity.Provider, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config is nil")
	}
	switch cfg.Auth.Mode {
	case config.AuthModeStatic:
		return identity.NewStaticProvider(cfg.Auth.StaticUsername, cfg.Auth.StaticPassword, cfg.Auth.StaticRole)
	case config.AuthModeLDAP:
		return ldapidentity.NewProvider(ldapidentity.Config{
			URL:          cfg.LDAP.URL,
			BindDN:       cfg.LDAP.BindDN,
			BindPassword: os.Getenv("LDAP_BIND_PASSWORD"),
			UserBase:     cfg.LDAP.UserBase,
			UserFilter:   cfg.LDAP.UserFilter,
			UserGroupDN:  cfg.LDAP.UserGroupDN,
			AdminGroupDN: cfg.LDAP.AdminGroupDN,
		}, nil)
	default:
		return nil, fmt.Errorf("unsupported auth.mode %q", cfg.Auth.Mode)
	}
}
