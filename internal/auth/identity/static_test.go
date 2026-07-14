package identity

import (
	"context"
	"errors"
	"testing"
)

func TestStaticProviderAuthenticate(t *testing.T) {
	provider, err := NewStaticProvider("debug", "secret", RoleAdmin)
	if err != nil {
		t.Fatalf("NewStaticProvider() error = %v", err)
	}

	got, err := provider.Authenticate(context.Background(), "debug", "secret")
	if err != nil {
		t.Fatalf("Authenticate() error = %v", err)
	}
	if got.Username != "static:debug" || got.Source != SourceStatic || got.Role != RoleAdmin {
		t.Fatalf("Authenticate() identity = %+v", got)
	}
}

func TestStaticProviderRejectsInvalidCredentials(t *testing.T) {
	provider, err := NewStaticProvider("debug", "secret", RoleUser)
	if err != nil {
		t.Fatalf("NewStaticProvider() error = %v", err)
	}

	for _, credentials := range []struct {
		username string
		password string
	}{
		{username: "debug", password: "wrong"},
		{username: "other", password: "secret"},
	} {
		_, err := provider.Authenticate(context.Background(), credentials.username, credentials.password)
		if !errors.Is(err, ErrInvalidCredentials) {
			t.Fatalf("Authenticate(%q, %q) error = %v, want ErrInvalidCredentials", credentials.username, credentials.password, err)
		}
	}
}

func TestNewStaticProviderRejectsInvalidConfiguration(t *testing.T) {
	for _, input := range []struct {
		username string
		password string
		role     string
	}{
		{username: "", password: "secret", role: RoleUser},
		{username: "debug", password: "", role: RoleUser},
		{username: "debug", password: "secret", role: "operator"},
		{username: "static:debug", password: "secret", role: RoleUser},
	} {
		if _, err := NewStaticProvider(input.username, input.password, input.role); err == nil {
			t.Fatalf("NewStaticProvider(%q, %q, %q) accepted invalid input", input.username, input.password, input.role)
		}
	}
}
