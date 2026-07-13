package config

import "testing"

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
