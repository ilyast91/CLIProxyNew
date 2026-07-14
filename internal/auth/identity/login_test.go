package identity

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ilyast91/CLIProxyNew/internal/store"
)

func TestLoginServiceCreatesStaticSessionWithRoleTTL(t *testing.T) {
	now := time.Date(2026, time.July, 14, 12, 0, 0, 0, time.UTC)
	users := fakeUserProvisioner{staticUser: store.User{
		ID:             7,
		Username:       "static:debug",
		Role:           RoleAdmin,
		Status:         "active",
		IdentitySource: SourceStatic,
	}}
	sessions := &fakeSessionCreator{}
	service := NewLoginService(fakeProvider{identity: Identity{
		Username: "static:debug",
		Role:     RoleAdmin,
		Source:   SourceStatic,
	}}, SourceStatic, users, sessions)
	service.now = func() time.Time { return now }

	result, err := service.Login(context.Background(), "debug", "secret")
	if err != nil {
		t.Fatalf("Login() error = %v", err)
	}
	if result.UserID != 7 || result.Role != RoleAdmin || result.Token == "" {
		t.Fatalf("Login() result = %+v", result)
	}
	if sessions.params.UserID != 7 || sessions.params.Role != RoleAdmin {
		t.Fatalf("Create() params = %+v", sessions.params)
	}
	if got, want := sessions.params.ExpiresAt, now.Add(adminSessionTTL); !got.Equal(want) {
		t.Fatalf("session expiry = %v, want %v", got, want)
	}
}

func TestLoginServiceRejectsBlockedUserAndSourceMismatch(t *testing.T) {
	t.Run("blocked", func(t *testing.T) {
		sessions := &fakeSessionCreator{}
		service := NewLoginService(fakeProvider{identity: Identity{
			Username: "static:debug",
			Role:     RoleUser,
			Source:   SourceStatic,
		}}, SourceStatic, fakeUserProvisioner{staticUser: store.User{Status: "blocked"}}, sessions)

		_, err := service.Login(context.Background(), "debug", "secret")
		if !errors.Is(err, ErrUserBlocked) {
			t.Fatalf("Login() error = %v, want ErrUserBlocked", err)
		}
		if sessions.called {
			t.Fatal("Login() created a session for blocked user")
		}
	})

	t.Run("source mismatch", func(t *testing.T) {
		service := NewLoginService(fakeProvider{identity: Identity{
			Username: "static:debug",
			Role:     RoleUser,
			Source:   SourceStatic,
		}}, SourceLDAP, fakeUserProvisioner{}, &fakeSessionCreator{})

		_, err := service.Login(context.Background(), "debug", "secret")
		if !errors.Is(err, ErrSourceMismatch) {
			t.Fatalf("Login() error = %v, want ErrSourceMismatch", err)
		}
	})
}

type fakeProvider struct {
	identity Identity
	err      error
}

func (p fakeProvider) Authenticate(context.Context, string, string) (Identity, error) {
	return p.identity, p.err
}

type fakeUserProvisioner struct {
	staticUser store.User
	ldapUser   store.User
}

func (p fakeUserProvisioner) UpsertStatic(context.Context, store.UpsertUserParams) (store.User, error) {
	return p.staticUser, nil
}

func (p fakeUserProvisioner) UpsertFromLDAP(context.Context, store.UpsertUserParams) (store.User, error) {
	return p.ldapUser, nil
}

type fakeSessionCreator struct {
	called bool
	params store.CreateSessionParams
}

func (c *fakeSessionCreator) Create(_ context.Context, params store.CreateSessionParams) (store.Session, error) {
	c.called = true
	c.params = params
	return store.Session{}, nil
}
