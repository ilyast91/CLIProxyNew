package identity

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"fmt"
	"strings"
)

const staticUsernamePrefix = "static:"

// StaticProvider аутентифицирует единственного development/test пользователя из env.
type StaticProvider struct {
	username       string
	usernameDigest [sha256.Size]byte
	passwordDigest [sha256.Size]byte
	role           string
}

// NewStaticProvider создаёт static provider с заранее проверенной конфигурацией.
func NewStaticProvider(username, password, role string) (*StaticProvider, error) {
	username = strings.TrimSpace(username)
	if username == "" || password == "" || strings.HasPrefix(username, staticUsernamePrefix) {
		return nil, ErrInvalidConfiguration
	}
	if role != RoleUser && role != RoleAdmin {
		return nil, fmt.Errorf("%w: unknown role %q", ErrInvalidConfiguration, role)
	}

	return &StaticProvider{
		username:       username,
		usernameDigest: sha256.Sum256([]byte(username)),
		passwordDigest: sha256.Sum256([]byte(password)),
		role:           role,
	}, nil
}

// Authenticate сопоставляет credentials с env-only static identity.
func (p *StaticProvider) Authenticate(_ context.Context, username, password string) (Identity, error) {
	if p == nil {
		return Identity{}, ErrInvalidCredentials
	}
	usernameDigest := sha256.Sum256([]byte(username))
	passwordDigest := sha256.Sum256([]byte(password))
	usernameMatch := subtle.ConstantTimeCompare(p.usernameDigest[:], usernameDigest[:])
	passwordMatch := subtle.ConstantTimeCompare(p.passwordDigest[:], passwordDigest[:])
	if usernameMatch&passwordMatch != 1 {
		return Identity{}, ErrInvalidCredentials
	}

	return Identity{
		Username: staticUsernamePrefix + p.username,
		Role:     p.role,
		Source:   SourceStatic,
	}, nil
}
