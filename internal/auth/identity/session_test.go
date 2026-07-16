package identity

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ilyast91/CLIProxyNew/internal/store"
)

func TestSessionAuthenticatorUsesCookieAndActiveSource(t *testing.T) {
	lookup := fakeSessionLookup{session: store.Session{ID: 5, UserID: 42, Role: RoleAdmin}}
	authenticator := NewSessionAuthenticator(&lookup, SourceStatic)
	request := httptest.NewRequest(http.MethodGet, "/api/v1/me/keys", nil)
	request.AddCookie(&http.Cookie{Name: SessionCookieName, Value: "opaque-token"})

	principal, err := authenticator.AuthenticateRequest(context.Background(), request)
	if err != nil {
		t.Fatalf("AuthenticateRequest() error = %v", err)
	}
	if principal.UserID != 42 || principal.Role != RoleAdmin {
		t.Fatalf("AuthenticateRequest() principal = %+v", principal)
	}
	if lookup.token != "opaque-token" || lookup.source != SourceStatic {
		t.Fatalf("GetByTokenForSource(%q, %q)", lookup.token, lookup.source)
	}
}

func TestSessionAuthenticatorRejectsMissingAndInvalidCookie(t *testing.T) {
	authenticator := NewSessionAuthenticator(&fakeSessionLookup{err: store.ErrInvalidCredential}, SourceLDAP)
	request := httptest.NewRequest(http.MethodGet, "/api/v1/me/keys", nil)
	if _, err := authenticator.AuthenticateRequest(context.Background(), request); !errors.Is(err, ErrNoSession) {
		t.Fatalf("missing cookie error = %v, want ErrNoSession", err)
	}

	request.AddCookie(&http.Cookie{Name: SessionCookieName, Value: "expired"})
	if _, err := authenticator.AuthenticateRequest(context.Background(), request); !errors.Is(err, ErrInvalidSession) {
		t.Fatalf("invalid cookie error = %v, want ErrInvalidSession", err)
	}
}

func TestCachedSessionAuthenticatorCachesAndInvalidatesUserSessions(t *testing.T) {
	lookup := &fakeSessionLookup{session: store.Session{ID: 5, UserID: 42, Role: RoleAdmin}}
	authenticator := NewCachedSessionAuthenticator(lookup, SourceLDAP, time.Minute)
	request := httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
	request.AddCookie(&http.Cookie{Name: SessionCookieName, Value: "opaque-token"})

	for range 2 {
		principal, err := authenticator.AuthenticateRequest(context.Background(), request)
		if err != nil || principal.UserID != 42 {
			t.Fatalf("AuthenticateRequest() principal=%+v error=%v", principal, err)
		}
	}
	if lookup.calls != 1 {
		t.Fatalf("lookup calls = %d, want 1 after cache hit", lookup.calls)
	}
	stats := authenticator.CacheStats()
	if stats.Hits != 1 || stats.Misses != 1 {
		t.Fatalf("CacheStats() = %+v, want hits=1 misses=1", stats)
	}

	authenticator.InvalidateUser(42)
	if _, err := authenticator.AuthenticateRequest(context.Background(), request); err != nil {
		t.Fatalf("AuthenticateRequest() after invalidation error = %v", err)
	}
	if lookup.calls != 2 {
		t.Fatalf("lookup calls = %d, want 2 after invalidation", lookup.calls)
	}
}

type fakeSessionLookup struct {
	session store.Session
	err     error
	token   string
	source  string
	calls   int
}

func (l *fakeSessionLookup) GetByTokenForSource(_ context.Context, token, source string) (store.Session, error) {
	l.calls++
	l.token = token
	l.source = source
	return l.session, l.err
}
