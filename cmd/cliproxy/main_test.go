package main

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ilyast91/CLIProxyNew/internal/auth/identity"
	"github.com/ilyast91/CLIProxyNew/internal/config"
)

type fakeServiceShutdowner struct {
	called   bool
	deadline time.Time
	err      error
}

func (s *fakeServiceShutdowner) Shutdown(ctx context.Context) error {
	s.called = true
	s.deadline, _ = ctx.Deadline()
	return s.err
}

func TestIdentityProviderFromConfigBuildsStaticProvider(t *testing.T) {
	cfg := config.Default()
	cfg.Server.Environment = config.EnvironmentDevelopment
	cfg.Auth.Mode = config.AuthModeStatic
	cfg.Auth.StaticUsername = "debug"
	cfg.Auth.StaticPassword = "secret"
	cfg.Auth.StaticRole = config.RoleAdmin

	provider, err := identityProviderFromConfig(cfg)
	if err != nil {
		t.Fatalf("identityProviderFromConfig() error = %v", err)
	}
	got, err := provider.Authenticate(context.Background(), "debug", "secret")
	if err != nil {
		t.Fatalf("Authenticate() error = %v", err)
	}
	if got.Source != identity.SourceStatic || got.Username != "static:debug" {
		t.Fatalf("Authenticate() identity = %+v", got)
	}
}

func TestShutdownServiceUsesBoundedContext(t *testing.T) {
	service := &fakeServiceShutdowner{}
	before := time.Now()

	if err := shutdownService(service); err != nil {
		t.Fatalf("shutdownService() error = %v", err)
	}
	if !service.called {
		t.Fatal("Shutdown() was not called")
	}
	if service.deadline.Before(before.Add(shutdownTimeout-time.Second)) || service.deadline.After(before.Add(shutdownTimeout+time.Second)) {
		t.Fatalf("Shutdown() deadline = %v, want about %v", service.deadline, shutdownTimeout)
	}
}

func TestShutdownServiceReturnsSDKError(t *testing.T) {
	want := errors.New("shutdown failed")
	if err := shutdownService(&fakeServiceShutdowner{err: want}); !errors.Is(err, want) {
		t.Fatalf("shutdownService() error = %v, want %v", err, want)
	}
}
