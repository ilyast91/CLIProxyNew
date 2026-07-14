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

	businessaccess "github.com/ilyast91/CLIProxyNew/internal/access"
	"github.com/ilyast91/CLIProxyNew/internal/config"
	"github.com/ilyast91/CLIProxyNew/internal/security"
	"github.com/ilyast91/CLIProxyNew/internal/store"
	sdkaccess "github.com/router-for-me/CLIProxyAPI/v7/sdk/access"
	sdkauth "github.com/router-for-me/CLIProxyAPI/v7/sdk/auth"
)

const (
	appName    = "cliproxy"
	appVersion = "dev"
)

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

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	slog.Info("starting",
		"app", appName,
		"version", appVersion,
		"server_addr", cfg.Server.Addr,
	)

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
	authStore := store.NewCoreAuthStore(dbPool, keyring, cfg.Proxy.Inference)
	sdkauth.RegisterTokenStore(authStore)
	apiKeyProvider := businessaccess.NewProvider(store.NewAPIKeyRepository(dbPool), cfg.Auth.Mode)
	sdkaccess.RegisterProvider(businessaccess.ProviderIdentifier, apiKeyProvider)
	sdkaccess.SetExclusiveProvider(businessaccess.ProviderIdentifier)
	slog.Info("credential store registered", "key_version", cfg.Encryption.KeyVersion)
	slog.Info("API key provider registered", "identity_source", cfg.Auth.Mode)

	// Полный wiring SDK (Builder + Service.Run) — в Ф3.
	slog.Info("persistence ready, SDK wiring pending (see implementation-phases.md Ф3)")
	<-ctx.Done()
	slog.Info("shutdown signal received")
	return nil
}
