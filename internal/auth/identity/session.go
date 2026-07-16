package identity

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"time"

	"github.com/ilyast91/CLIProxyNew/internal/cache"
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
	cache          *cache.TTL[string, store.Session]
}

// NewSessionAuthenticator создаёт аутентификатор для одного активного source.
func NewSessionAuthenticator(lookup SessionLookup, identitySource string) *SessionAuthenticator {
	return &SessionAuthenticator{lookup: lookup, identitySource: identitySource}
}

// NewCachedSessionAuthenticator создаёт аутентификатор с TTL-кэшем успешных session lookup.
func NewCachedSessionAuthenticator(lookup SessionLookup, identitySource string, ttl time.Duration) *SessionAuthenticator {
	authenticator := NewSessionAuthenticator(lookup, identitySource)
	authenticator.cache = cache.NewTTL[string, store.Session](ttl, time.Now)
	return authenticator
}

// CacheStats возвращает snapshot hit/miss кэша session lookup.
func (a *SessionAuthenticator) CacheStats() cache.Stats {
	if a == nil || a.cache == nil {
		return cache.Stats{}
	}
	return a.cache.Stats()
}

// InvalidateUser удаляет из локального кэша все сессии указанного пользователя.
func (a *SessionAuthenticator) InvalidateUser(userID int64) {
	if a == nil || a.cache == nil || userID <= 0 {
		return
	}
	a.cache.DeleteWhere(func(_ string, session store.Session) bool { return session.UserID == userID })
}

// InvalidateToken удаляет из локального кэша одну session-cookie текущего source.
func (a *SessionAuthenticator) InvalidateToken(token string) {
	if a == nil || a.cache == nil || token == "" {
		return
	}
	a.cache.Delete(sessionCacheKey(a.identitySource, token))
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
	cacheKey := sessionCacheKey(a.identitySource, cookie.Value)
	if a.cache != nil {
		if session, ok := a.cache.Get(cacheKey); ok {
			return SessionPrincipal{UserID: session.UserID, Role: session.Role}, nil
		}
	}
	session, err := a.lookup.GetByTokenForSource(ctx, cookie.Value, a.identitySource)
	if err != nil {
		return SessionPrincipal{}, ErrInvalidSession
	}
	if a.cache != nil {
		a.cache.Set(cacheKey, session)
	}
	return SessionPrincipal{UserID: session.UserID, Role: session.Role}, nil
}

func sessionCacheKey(identitySource, token string) string {
	sum := sha256.Sum256([]byte(identitySource + "\x00" + token))
	return hex.EncodeToString(sum[:])
}
