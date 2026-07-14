package identity

import (
	"context"
	"errors"
	"net/http"

	"github.com/ilyast91/CLIProxyNew/internal/store"
)

const (
	// SessionCookieName — имя opaque session-cookie для management API.
	SessionCookieName = "cliproxy_session"
)

var (
	// ErrNoSession означает отсутствие session-cookie в запросе.
	ErrNoSession = errors.New("session cookie отсутствует")
	// ErrInvalidSession означает истёкшую, отозванную или неподходящую source сессию.
	ErrInvalidSession = errors.New("недействительная session cookie")
)

// SessionLookup читает active opaque session для ожидаемого identity source.
type SessionLookup interface {
	GetByTokenForSource(ctx context.Context, token, identitySource string) (store.Session, error)
}

// SessionPrincipal — identity пользователя для management middleware.
type SessionPrincipal struct {
	UserID int64
	Role   string
}

// SessionAuthenticator извлекает и проверяет management session-cookie.
type SessionAuthenticator struct {
	lookup         SessionLookup
	identitySource string
}

// NewSessionAuthenticator создаёт аутентификатор для одного активного source.
func NewSessionAuthenticator(lookup SessionLookup, identitySource string) *SessionAuthenticator {
	return &SessionAuthenticator{lookup: lookup, identitySource: identitySource}
}

// AuthenticateRequest возвращает principal только для active session текущего source.
func (a *SessionAuthenticator) AuthenticateRequest(ctx context.Context, request *http.Request) (SessionPrincipal, error) {
	if a == nil || a.lookup == nil {
		return SessionPrincipal{}, ErrInvalidSession
	}
	if request == nil {
		return SessionPrincipal{}, ErrNoSession
	}
	cookie, err := request.Cookie(SessionCookieName)
	if err != nil || cookie.Value == "" {
		return SessionPrincipal{}, ErrNoSession
	}
	session, err := a.lookup.GetByTokenForSource(ctx, cookie.Value, a.identitySource)
	if err != nil {
		return SessionPrincipal{}, ErrInvalidSession
	}
	return SessionPrincipal{UserID: session.UserID, Role: session.Role}, nil
}
