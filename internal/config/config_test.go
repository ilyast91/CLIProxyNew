package config

import "testing"

func TestDefaultUsesProductionLDAP(t *testing.T) {
	cfg := Default()

	if cfg.Server.Environment != EnvironmentProduction {
		t.Fatalf("Server.Environment = %q, want %q", cfg.Server.Environment, EnvironmentProduction)
	}
	if cfg.Auth.Mode != AuthModeLDAP {
		t.Fatalf("Auth.Mode = %q, want %q", cfg.Auth.Mode, AuthModeLDAP)
	}
}

func TestValidateRejectsStaticModeInProduction(t *testing.T) {
	cfg := Default()
	cfg.Auth.Mode = AuthModeStatic
	cfg.Auth.StaticUsername = "debug"
	cfg.Auth.StaticPassword = "secret"
	cfg.Auth.StaticRole = RoleAdmin

	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() accepted static mode in production")
	}
}

func TestValidateAcceptsStaticModeInDevelopment(t *testing.T) {
	cfg := Default()
	cfg.Server.Environment = EnvironmentDevelopment
	cfg.Auth.Mode = AuthModeStatic
	cfg.Auth.StaticUsername = "debug"
	cfg.Auth.StaticPassword = "secret"
	cfg.Auth.StaticRole = RoleAdmin

	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestValidateRequiresStaticCredentialsAndKnownRole(t *testing.T) {
	cfg := Default()
	cfg.Server.Environment = EnvironmentTest
	cfg.Auth.Mode = AuthModeStatic

	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() accepted missing static credentials")
	}

	cfg.Auth.StaticUsername = "debug"
	cfg.Auth.StaticPassword = "secret"
	cfg.Auth.StaticRole = "operator"
	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() accepted unknown static role")
	}
}

func TestFromEnvironmentAppliesOverridesToDefaults(t *testing.T) {
	t.Setenv("CLIPROXY_SERVER_ADDR", ":9090")
	t.Setenv("CLIPROXY_DB_DSN", "postgres://cliproxy@example/cliproxy")
	t.Setenv("CLIPROXY_LOG_LEVEL", "debug")

	cfg := FromEnvironment()

	if cfg.Server.Addr != ":9090" {
		t.Fatalf("Server.Addr = %q, want :9090", cfg.Server.Addr)
	}
	if cfg.DB.DSN != "postgres://cliproxy@example/cliproxy" {
		t.Fatalf("DB.DSN = %q", cfg.DB.DSN)
	}
	if cfg.Logging.Level != "debug" {
		t.Fatalf("Logging.Level = %q, want debug", cfg.Logging.Level)
	}
}
