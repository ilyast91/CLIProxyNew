package access

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/ilyast91/CLIProxyNew/internal/store"
	sdkaccess "github.com/router-for-me/CLIProxyAPI/v7/sdk/access"
)

const (
	// ProviderIdentifier — стабильный идентификатор DB-backed API-key provider.
	ProviderIdentifier = "db-apikey"
	metadataAPIKeyID   = "api_key_id"
)

// APIKeyRepository определяет source-aware lookup клиентского API-key.
type APIKeyRepository interface {
	AuthenticateForSource(ctx context.Context, plaintext, identitySource string) (store.APIKeyPrincipal, error)
}

// Provider реализует sdk/access.Provider для API-keys из Postgres.
type Provider struct {
	repository     APIKeyRepository
	identitySource string
}

// NewProvider создаёт provider для одного активного identity source.
func NewProvider(repository APIKeyRepository, identitySource string) *Provider {
	return &Provider{repository: repository, identitySource: identitySource}
}

// Identifier возвращает стабильный тип provider для SDK registry.
func (p *Provider) Identifier() string {
	return ProviderIdentifier
}

// Authenticate проверяет API-key из заголовков или query-параметров запроса.
func (p *Provider) Authenticate(ctx context.Context, request *http.Request) (*sdkaccess.Result, *sdkaccess.AuthError) {
	if p == nil || p.repository == nil {
		return nil, sdkaccess.NewInternalAuthError("API key authentication is unavailable", nil)
	}

	credentials, supplied := requestCredentials(request)
	if !supplied {
		return nil, sdkaccess.NewNoCredentialsError()
	}
	for _, credential := range credentials {
		if credential == "" {
			continue
		}
		principal, err := p.repository.AuthenticateForSource(ctx, credential, p.identitySource)
		if err == nil {
			return &sdkaccess.Result{
				Provider:  p.Identifier(),
				Principal: strconv.FormatInt(principal.UserID, 10),
				Metadata:  map[string]string{metadataAPIKeyID: strconv.FormatInt(principal.APIKeyID, 10)},
			}, nil
		}
		if !errors.Is(err, store.ErrInvalidCredential) {
			return nil, sdkaccess.NewInternalAuthError("API key authentication failed", err)
		}
	}

	return nil, sdkaccess.NewInvalidCredentialError()
}

func requestCredentials(request *http.Request) ([]string, bool) {
	if request == nil {
		return nil, false
	}
	authorization := request.Header.Get("Authorization")
	googleAPIKey := request.Header.Get("X-Goog-Api-Key")
	anthropicAPIKey := request.Header.Get("X-Api-Key")
	queryKey := ""
	queryAuthToken := ""
	if request.URL != nil {
		queryKey = request.URL.Query().Get("key")
		queryAuthToken = request.URL.Query().Get("auth_token")
	}

	supplied := authorization != "" || googleAPIKey != "" || anthropicAPIKey != "" || queryKey != "" || queryAuthToken != ""
	return []string{
		extractBearerToken(authorization),
		googleAPIKey,
		anthropicAPIKey,
		queryKey,
		queryAuthToken,
	}, supplied
}

func extractBearerToken(header string) string {
	parts := strings.SplitN(header, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "bearer") {
		return strings.TrimSpace(header)
	}
	return strings.TrimSpace(parts[1])
}
