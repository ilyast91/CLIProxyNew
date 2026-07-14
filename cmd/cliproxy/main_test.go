package main

import (
	"context"
	"testing"

	"github.com/ilyast91/CLIProxyNew/internal/auth/identity"
	"github.com/ilyast91/CLIProxyNew/internal/config"
)

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
