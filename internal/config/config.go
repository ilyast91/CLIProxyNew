// Package config содержит конфигурацию сервиса CLIProxyNew.
//
// Источник (R6): config.yaml (монтируется из k8s ConfigMap) + env-override
// (12-factor). Секреты — только через env (k8s Secret), никогда в config.yaml:
//   - CLIPROXY_ENCRYPTION_KEY — мастер-ключ AES-256-GCM (base64, 32 байта)
//   - CLIPROXY_ENCRYPTION_PREVIOUS_KEYS — предыдущие ключи для ротации (JSON)
//   - DB_PASSWORD — пароль Postgres
//   - LDAP_BIND_PASSWORD — пароль service-account LDAP
//   - CLIPROXY_STATIC_USER_* — credentials static identity для development/test
//
// См. docs/requirements.md R6, docs/architecture-principles.md §6.
package config

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// ErrConfigNotFound возвращается, когда файл конфигурации не существует.
var ErrConfigNotFound = errors.New("config file not found")

// Config — главный тип конфигурации сервиса (R6).
type Config struct {
	Server     ServerConfig     `yaml:"server"`
	Auth       AuthConfig       `yaml:"auth"`
	DB         DBConfig         `yaml:"db"`
	LDAP       LDAPConfig       `yaml:"ldap"`
	Proxy      ProxyConfig      `yaml:"proxy"`
	Logging    LoggingConfig    `yaml:"logging"`
	Encryption EncryptionConfig `yaml:"encryption"`
}

// ServerConfig — параметры HTTP-сервера.
type ServerConfig struct {
	Addr               string   `yaml:"addr"`
	Environment        string   `yaml:"environment"`
	CORSAllowedOrigins []string `yaml:"cors_allowed_origins"`
}

const (
	// EnvironmentDevelopment разрешает локальные development-интеграции.
	EnvironmentDevelopment = "development"
	// EnvironmentTest разрешает тестовые интеграции.
	EnvironmentTest = "test"
	// EnvironmentProduction разрешает только production-настройки.
	EnvironmentProduction = "production"

	// AuthModeLDAP выбирает LDAP identity provider.
	AuthModeLDAP = "ldap"
	// AuthModeStatic выбирает env-only static identity provider.
	AuthModeStatic = "static"

	// RoleUser даёт обычные пользовательские права.
	RoleUser = "user"
	// RoleAdmin даёт административные права.
	RoleAdmin = "admin"
)

// AuthConfig — выбор identity source. Static credentials не сериализуются в YAML.
type AuthConfig struct {
	Mode           string `yaml:"mode"`
	StaticUsername string `yaml:"-"`
	StaticPassword string `yaml:"-"`
	StaticRole     string `yaml:"-"`
}

// DBConfig — подключение к Postgres. DSN берётся из конфига,
// но пароль подставляется из env DB_PASSWORD (см. DSN()).
type DBConfig struct {
	DSN string `yaml:"dsn"`
}

// LDAPConfig — параметры LDAP-подключения (R1).
// Пароль service-account — только из env LDAP_BIND_PASSWORD.
type LDAPConfig struct {
	URL          string `yaml:"url"`
	BindDN       string `yaml:"bind_dn"`
	UserBase     string `yaml:"user_base"`
	UserFilter   string `yaml:"user_filter"`
	UserGroupDN  string `yaml:"user_group_dn"`
	AdminGroupDN string `yaml:"admin_group_dn"`
}

// ProxyConfig — per-call-type egress-прокси (R10).
// Пустая строка = direct (без прокси).
type ProxyConfig struct {
	Inference string `yaml:"inference"`
	Auth      string `yaml:"auth"`
	Quota     string `yaml:"quota"`
	Models    string `yaml:"models"`
}

// LoggingConfig — параметры логирования.
type LoggingConfig struct {
	Level  string `yaml:"level"`
	Format string `yaml:"format"`
}

// EncryptionConfig — параметры шифрования at-rest (R5).
// Активный и предыдущие мастер-ключи приходят только из env.
type EncryptionConfig struct {
	KeyVersion int `yaml:"key_version"`
}

// Default возвращает конфигурацию с безопасными значениями по умолчанию.
func Default() *Config {
	return &Config{
		Server: ServerConfig{Addr: ":8080", Environment: EnvironmentProduction},
		Auth:   AuthConfig{Mode: AuthModeLDAP},
		Logging: LoggingConfig{
			Level:  "info",
			Format: "json",
		},
		Encryption: EncryptionConfig{KeyVersion: 1},
	}
}

// Validate проверяет конфигурацию до создания внешних подключений.
func (c *Config) Validate() error {
	if c == nil {
		return fmt.Errorf("config is nil")
	}

	c.Server.Environment = strings.TrimSpace(c.Server.Environment)
	c.Auth.Mode = strings.TrimSpace(c.Auth.Mode)
	switch c.Server.Environment {
	case EnvironmentDevelopment, EnvironmentTest, EnvironmentProduction:
	default:
		return fmt.Errorf("unknown server.environment %q", c.Server.Environment)
	}
	if err := validateCORSOrigins(c.Server.CORSAllowedOrigins); err != nil {
		return err
	}
	if err := validateProxyConfig(&c.Proxy); err != nil {
		return err
	}

	switch c.Auth.Mode {
	case AuthModeLDAP:
		return nil
	case AuthModeStatic:
		if c.Server.Environment == EnvironmentProduction {
			return errors.New("auth.mode=static is forbidden in production")
		}
		if strings.TrimSpace(c.Auth.StaticUsername) == "" || c.Auth.StaticPassword == "" {
			return errors.New("static identity requires username and password from environment")
		}
		switch strings.TrimSpace(c.Auth.StaticRole) {
		case RoleUser, RoleAdmin:
			return nil
		default:
			return fmt.Errorf("unknown static user role %q", c.Auth.StaticRole)
		}
	default:
		return fmt.Errorf("unknown auth.mode %q", c.Auth.Mode)
	}
}

// FromEnvironment возвращает defaults с применёнными env-override.
// Используется для env-only запуска без config.yaml.
func FromEnvironment() *Config {
	cfg := Default()
	cfg.applyEnvOverrides()
	return cfg
}

// Load читает config.yaml и применяет env-override.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("%w: %s", ErrConfigNotFound, path)
		}
		return nil, fmt.Errorf("read config %s: %w", path, err)
	}

	cfg := Default()
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parse config %s: %w", path, err)
	}

	cfg.applyEnvOverrides()
	return cfg, nil
}

// applyEnvOverrides применяет env-переменные поверх значений из yaml (12-factor).
// Ф0: пока только базовые override'ы; полный набор — по мере имплементации фаз.
func (c *Config) applyEnvOverrides() {
	if v := os.Getenv("CLIPROXY_SERVER_ADDR"); v != "" {
		c.Server.Addr = v
	}
	if v := os.Getenv("CLIPROXY_DB_DSN"); v != "" {
		c.DB.DSN = v
	}
	if v := strings.TrimSpace(os.Getenv("CLIPROXY_LOG_LEVEL")); v != "" {
		c.Logging.Level = v
	}
	if v := strings.TrimSpace(os.Getenv("CLIPROXY_SERVER_ENVIRONMENT")); v != "" {
		c.Server.Environment = v
	}
	if v := strings.TrimSpace(os.Getenv("CLIPROXY_AUTH_MODE")); v != "" {
		c.Auth.Mode = v
	}
	if value, ok := os.LookupEnv("CLIPROXY_CORS_ALLOWED_ORIGINS"); ok {
		c.Server.CORSAllowedOrigins = splitCSV(value)
	}
	overrideEnv(&c.Proxy.Inference, "CLIPROXY_PROXY_INFERENCE")
	overrideEnv(&c.Proxy.Auth, "CLIPROXY_PROXY_AUTH")
	overrideEnv(&c.Proxy.Quota, "CLIPROXY_PROXY_QUOTA")
	overrideEnv(&c.Proxy.Models, "CLIPROXY_PROXY_MODELS")
	c.Auth.StaticUsername = os.Getenv("CLIPROXY_STATIC_USER_USERNAME")
	c.Auth.StaticPassword = os.Getenv("CLIPROXY_STATIC_USER_PASSWORD")
	c.Auth.StaticRole = os.Getenv("CLIPROXY_STATIC_USER_ROLE")
}

func validateCORSOrigins(origins []string) error {
	for _, origin := range origins {
		parsed, err := url.ParseRequestURI(origin)
		if err != nil || parsed.Scheme == "" || parsed.Host == "" || parsed.User != nil || parsed.Path != "" || parsed.RawQuery != "" || parsed.Fragment != "" {
			return fmt.Errorf("invalid server.cors_allowed_origins entry %q", origin)
		}
		if parsed.Scheme != "http" && parsed.Scheme != "https" {
			return fmt.Errorf("invalid server.cors_allowed_origins scheme %q", parsed.Scheme)
		}
	}
	return nil
}

func splitCSV(value string) []string {
	parts := strings.Split(value, ",")
	origins := make([]string, 0, len(parts))
	for _, part := range parts {
		if origin := strings.TrimSpace(part); origin != "" {
			origins = append(origins, origin)
		}
	}
	return origins
}

func validateProxyConfig(proxy *ProxyConfig) error {
	if proxy == nil {
		return nil
	}
	for _, value := range []*string{&proxy.Inference, &proxy.Auth, &proxy.Quota, &proxy.Models} {
		*value = strings.TrimSpace(*value)
		if *value == "" {
			continue
		}
		parsed, err := url.ParseRequestURI(*value)
		if err != nil || parsed.Host == "" || parsed.Path != "" || parsed.RawQuery != "" || parsed.Fragment != "" {
			return fmt.Errorf("invalid proxy URL %q", *value)
		}
		switch parsed.Scheme {
		case "http", "https", "socks5", "socks5h":
		default:
			return fmt.Errorf("unsupported proxy URL scheme %q", parsed.Scheme)
		}
	}
	return nil
}

func overrideEnv(target *string, name string) {
	if value, ok := os.LookupEnv(name); ok {
		*target = value
	}
}
