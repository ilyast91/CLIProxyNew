package access

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ilyast91/CLIProxyNew/internal/store"
	sdkaccess "github.com/router-for-me/CLIProxyAPI/v7/sdk/access"
)

func TestProviderAuthenticatesBearerTokenForActiveSource(t *testing.T) {
	repository := fakeRepository{authenticate: func(_ context.Context, key, source string) (store.APIKeyPrincipal, error) {
		if key != "cpn_live_0123456789" || source != "static" {
			t.Fatalf("AuthenticateForSource(%q, %q)", key, source)
		}
		return store.APIKeyPrincipal{UserID: 42, APIKeyID: 17}, nil
	}}
	provider := NewProvider(repository, "static")
	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	req.Header.Set("Authorization", "Bearer cpn_live_0123456789")

	result, authErr := provider.Authenticate(context.Background(), req)
	if authErr != nil {
		t.Fatalf("Authenticate() error = %v", authErr)
	}
	if result.Provider != ProviderIdentifier || result.Principal != "42" || result.Metadata["api_key_id"] != "17" {
		t.Fatalf("Authenticate() result = %+v", result)
	}
}

func TestProviderRejectsMissingAndInvalidCredentials(t *testing.T) {
	provider := NewProvider(fakeRepository{authenticate: func(context.Context, string, string) (store.APIKeyPrincipal, error) {
		return store.APIKeyPrincipal{}, store.ErrInvalidCredential
	}}, "ldap")

	_, authErr := provider.Authenticate(context.Background(), httptest.NewRequest(http.MethodGet, "/v1/models", nil))
	if !sdkaccess.IsAuthErrorCode(authErr, sdkaccess.AuthErrorCodeNoCredentials) {
		t.Fatalf("missing credentials error = %#v", authErr)
	}

	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	req.Header.Set("X-Api-Key", "cpn_live_wrong")
	_, authErr = provider.Authenticate(context.Background(), req)
	if !sdkaccess.IsAuthErrorCode(authErr, sdkaccess.AuthErrorCodeInvalidCredential) {
		t.Fatalf("invalid credentials error = %#v", authErr)
	}
}

func TestProviderHidesRepositoryFailure(t *testing.T) {
	provider := NewProvider(fakeRepository{authenticate: func(context.Context, string, string) (store.APIKeyPrincipal, error) {
		return store.APIKeyPrincipal{}, errors.New("database unavailable")
	}}, "ldap")
	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	req.Header.Set("Authorization", "Bearer cpn_live_0123456789")

	_, authErr := provider.Authenticate(context.Background(), req)
	if !sdkaccess.IsAuthErrorCode(authErr, sdkaccess.AuthErrorCodeInternal) {
		t.Fatalf("repository error = %#v", authErr)
	}
}

type fakeRepository struct {
	authenticate func(context.Context, string, string) (store.APIKeyPrincipal, error)
}

func (r fakeRepository) AuthenticateForSource(ctx context.Context, key, source string) (store.APIKeyPrincipal, error) {
	return r.authenticate(ctx, key, source)
}
