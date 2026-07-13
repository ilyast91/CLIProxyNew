// Package config содержит конфигурацию сервиса CLIProxyNew.
//
// Источник (R6): config.yaml (монтируется из k8s ConfigMap) + env-override
// (12-factor). Секреты — только через env (k8s Secret), никогда в config.yaml:
//   - CLIPROXY_ENCRYPTION_KEY — мастер-ключ AES-256-GCM (base64, 32 байта)
//   - DB_PASSWORD — пароль Postgres
//   - LDAP_BIND_PASSWORD — пароль service-account LDAP
//
// См. docs/requirements.md R6, docs/architecture-principles.md §6.
package config

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// ErrConfigNotFound возвращается, когда файл конфигурации не существует.
var ErrConfigNotFound = errors.New("config file not found")

// Config — главный тип конфигурации сервиса (R6).
type Config struct {
	Server     ServerConfig     `yaml:"server"`
	DB         DBConfig         `yaml:"db"`
	LDAP       LDAPConfig       `yaml:"ldap"`
	Proxy      ProxyConfig      `yaml:"proxy"`
	Logging    LoggingConfig    `yaml:"logging"`
	Encryption EncryptionConfig `yaml:"encryption"`
}

// ServerConfig — параметры HTTP-сервера.
type ServerConfig struct {
	Addr string `yaml:"addr"`
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
// Сам мастер-ключ — из env CLIPROXY_ENCRYPTION_KEY.
type EncryptionConfig struct {
	KeyVersion int `yaml:"key_version"`
}

// Default возвращает конфигурацию с безопасными значениями по умолчанию.
func Default() *Config {
	return &Config{
		Server: ServerConfig{Addr: ":8080"},
		Logging: LoggingConfig{
			Level:  "info",
			Format: "json",
		},
		Encryption: EncryptionConfig{KeyVersion: 1},
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
}
