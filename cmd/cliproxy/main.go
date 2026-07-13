// Пакет main — точка входа CLIProxyNew.
//
// На фазе Ф0 (Foundation) здесь базовый wiring: загрузка конфигурации,
// проверка доступности SDK ядра CLIProxyAPI v7. Полная интеграция
// (Builder + Service.Run + 7 контрактов ADR-9) — фаза Ф3.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/ilyast91/CLIProxyNew/internal/config"
	// Импорт SDK ядра для валидации доступности на Ф0.
	// Реальное использование — в Ф3 (wiring Builder + Service.Run).
	_ "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy"
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
		// Конфиг пока опционален на Ф0 (ещё нет эндпоинтов).
		if !errors.Is(err, config.ErrConfigNotFound) {
			return fmt.Errorf("load config: %w", err)
		}
		cfg = config.Default()
		slog.Warn("config not found, using defaults", "path", configPath)
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

	// Ф0: stub. Полный wiring (Builder + Service.Run) — в Ф3.
	slog.Info("foundation phase: SDK available, wiring pending (see implementation-phases.md Ф3)")
	<-ctx.Done()
	slog.Info("shutdown signal received")
	return nil
}
