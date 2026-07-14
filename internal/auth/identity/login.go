package identity

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"time"

	"github.com/ilyast91/CLIProxyNew/internal/store"
)

const (
	userSessionTTL  = 5 * time.Minute
	adminSessionTTL = 10 * time.Hour
)

var (
	// ErrUserBlocked означает, что identity подтверждена, но пользователь заблокирован.
	ErrUserBlocked = errors.New("пользователь заблокирован")
	// ErrSourceMismatch означает несогласованную identity от provider.
	ErrSourceMismatch = errors.New("identity source не совпадает с активным режимом")
)

// UserProvisioner сохраняет identity после успешной аутентификации.
type UserProvisioner interface {
	UpsertStatic(ctx context.Context, params store.UpsertUserParams) (store.User, error)
	UpsertFromLDAP(ctx context.Context, params store.UpsertUserParams) (store.User, error)
}

// SessionCreator сохраняет opaque сессию.
type SessionCreator interface {
	Create(ctx context.Context, params store.CreateSessionParams) (store.Session, error)
}

// LoginResult содержит данные, нужные HTTP-адаптеру для установки cookie.
type LoginResult struct {
	UserID    int64
	Role      string
	Token     string
	ExpiresAt time.Time
}

// LoginService оркестрирует identity provider, provisioning и выдачу сессии.
type LoginService struct {
	provider       Provider
	identitySource string
	users          UserProvisioner
	sessions       SessionCreator
	now            func() time.Time
}

// NewLoginService создаёт login-сервис для одного активного identity source.
func NewLoginService(provider Provider, identitySource string, users UserProvisioner, sessions SessionCreator) *LoginService {
	return &LoginService{
		provider:       provider,
		identitySource: identitySource,
		users:          users,
		sessions:       sessions,
		now:            time.Now,
	}
}

// Login аутентифицирует пользователя и создаёт новую opaque сессию без продления TTL.
func (s *LoginService) Login(ctx context.Context, username, password string) (LoginResult, error) {
	if s == nil || s.provider == nil || s.users == nil || s.sessions == nil {
		return LoginResult{}, fmt.Errorf("login service is not configured")
	}

	identity, err := s.provider.Authenticate(ctx, username, password)
	if err != nil {
		return LoginResult{}, err
	}
	if identity.Source != s.identitySource {
		return LoginResult{}, ErrSourceMismatch
	}
	if identity.Role != RoleUser && identity.Role != RoleAdmin {
		return LoginResult{}, fmt.Errorf("identity has invalid role %q", identity.Role)
	}

	params := store.UpsertUserParams{Username: identity.Username, Email: identity.Email, Role: identity.Role}
	var user store.User
	switch identity.Source {
	case SourceStatic:
		user, err = s.users.UpsertStatic(ctx, params)
	case SourceLDAP:
		user, err = s.users.UpsertFromLDAP(ctx, params)
	default:
		return LoginResult{}, ErrSourceMismatch
	}
	if err != nil {
		return LoginResult{}, fmt.Errorf("provision user: %w", err)
	}
	if user.Status != "active" {
		return LoginResult{}, ErrUserBlocked
	}

	token, err := newSessionToken()
	if err != nil {
		return LoginResult{}, fmt.Errorf("generate session token: %w", err)
	}
	expiresAt := s.now().Add(sessionTTL(identity.Role))
	if _, err := s.sessions.Create(ctx, store.CreateSessionParams{
		UserID:    user.ID,
		Token:     token,
		Role:      identity.Role,
		ExpiresAt: expiresAt,
	}); err != nil {
		return LoginResult{}, fmt.Errorf("create session: %w", err)
	}

	return LoginResult{UserID: user.ID, Role: identity.Role, Token: token, ExpiresAt: expiresAt}, nil
}

func sessionTTL(role string) time.Duration {
	if role == RoleAdmin {
		return adminSessionTTL
	}
	return userSessionTTL
}

func newSessionToken() (string, error) {
	var raw [32]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw[:]), nil
}
