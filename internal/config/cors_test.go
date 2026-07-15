package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestValidateRejectsUnsafeCORSOrigins(t *testing.T) {
	for _, origin := range []string{"*", "https://console.example.test/path", "ftp://console.example.test"} {
		cfg := Default()
		cfg.Server.CORSAllowedOrigins = []string{origin}
		if err := cfg.Validate(); err == nil {
			t.Fatalf("Validate() accepted CORS origin %q", origin)
		}
	}
}

func TestFromEnvironmentParsesCORSOrigins(t *testing.T) {
	t.Setenv("CLIPROXY_CORS_ALLOWED_ORIGINS", "https://console.example.test, http://localhost:3000")

	cfg := FromEnvironment()
	if got, want := len(cfg.Server.CORSAllowedOrigins), 2; got != want {
		t.Fatalf("origin count=%d, want %d", got, want)
	}
	if cfg.Server.CORSAllowedOrigins[0] != "https://console.example.test" || cfg.Server.CORSAllowedOrigins[1] != "http://localhost:3000" {
		t.Fatalf("origins=%q", cfg.Server.CORSAllowedOrigins)
	}
}

func TestLoadKeepsCORSOriginsWhenEnvironmentOverrideIsUnset(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("server:\n  cors_allowed_origins:\n    - https://console.example.test\n"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got, want := len(cfg.Server.CORSAllowedOrigins), 1; got != want || cfg.Server.CORSAllowedOrigins[0] != "https://console.example.test" {
		t.Fatalf("origins=%q", cfg.Server.CORSAllowedOrigins)
	}
}
