package config

import "testing"

func TestValidateAcceptsSupportedProxyURLs(t *testing.T) {
	cfg := Default()
	cfg.Proxy = ProxyConfig{
		Inference: "socks5://proxy.example.test:1080",
		Auth:      "https://proxy.example.test:8443",
		Quota:     "http://proxy.example.test:8080",
		Models:    "socks5h://proxy.example.test:1080",
	}

	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestValidateRejectsInvalidProxyURL(t *testing.T) {
	cfg := Default()
	cfg.Proxy.Inference = "ftp://proxy.example.test:21"

	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() accepted unsupported proxy URL")
	}
}

func TestFromEnvironmentAppliesProxyOverrides(t *testing.T) {
	t.Setenv("CLIPROXY_PROXY_INFERENCE", "socks5://inference.example.test:1080")
	t.Setenv("CLIPROXY_PROXY_AUTH", "https://auth.example.test:8443")
	t.Setenv("CLIPROXY_PROXY_QUOTA", "http://quota.example.test:8080")
	t.Setenv("CLIPROXY_PROXY_MODELS", "socks5h://models.example.test:1080")

	cfg := FromEnvironment()
	if cfg.Proxy.Inference != "socks5://inference.example.test:1080" || cfg.Proxy.Auth != "https://auth.example.test:8443" || cfg.Proxy.Quota != "http://quota.example.test:8080" || cfg.Proxy.Models != "socks5h://models.example.test:1080" {
		t.Fatalf("proxy=%+v", cfg.Proxy)
	}
}
