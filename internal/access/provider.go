package access

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/ilyast91/CLIProxyNew/internal/store"
	sdkaccess "github.com/router-for-me/CLIProxyAPI/v7/sdk/access"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
)

const (
	// ProviderIdentifier — стабильный идентификатор DB-backed API-key provider.
	ProviderIdentifier = "db-apikey"
	metadataAPIKeyID   = "api_key_id"
	tracingName        = "github.com/ilyast91/CLIProxyNew/internal/access"
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
	ctx, span := otel.Tracer(tracingName).Start(ctx, "access.Provider.Authenticate")
	defer span.End()
	span.SetAttributes(attribute.String("auth.provider", ProviderIdentifier))
	if p != nil {
		span.SetAttributes(attribute.String("auth.identity_source", p.identitySource))
	}

	if p == nil || p.repository == nil {
		authErr := sdkaccess.NewInternalAuthError("API key authentication is unavailable", nil)
		span.SetAttributes(attribute.String("auth.outcome", "unavailable"))
		span.SetStatus(codes.Error, "authentication unavailable")
		return nil, authErr
	}

	credentials, supplied := requestCredentials(request)
	if !supplied {
		authErr := sdkaccess.NewNoCredentialsError()
		span.SetAttributes(attribute.String("auth.outcome", "no_credentials"))
		span.SetStatus(codes.Error, "credentials not supplied")
		return nil, authErr
	}
	for _, credential := range credentials {
		if credential == "" {
			continue
		}
		principal, err := p.repository.AuthenticateForSource(ctx, credential, p.identitySource)
		if err == nil {
			span.SetAttributes(
				attribute.String("auth.outcome", "success"),
				attribute.Int64("user.id", principal.UserID),
				attribute.Int64("api_key.id", principal.APIKeyID),
			)
			return &sdkaccess.Result{
				Provider:  p.Identifier(),
				Principal: EncodePrincipal(principal.UserID, principal.APIKeyID),
				Metadata:  map[string]string{metadataAPIKeyID: strconv.FormatInt(principal.APIKeyID, 10)},
			}, nil
		}
		if !errors.Is(err, store.ErrInvalidCredential) {
			authErr := sdkaccess.NewInternalAuthError("API key authentication failed", err)
			span.SetAttributes(attribute.String("auth.outcome", "internal_error"))
			span.SetStatus(codes.Error, "authentication failed")
			return nil, authErr
		}
	}

	authErr := sdkaccess.NewInvalidCredentialError()
	span.SetAttributes(attribute.String("auth.outcome", "invalid_credentials"))
	span.SetStatus(codes.Error, "invalid credentials")
	return nil, authErr
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
