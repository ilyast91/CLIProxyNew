package access

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ilyast91/CLIProxyNew/internal/store"
	sdkaccess "github.com/router-for-me/CLIProxyAPI/v7/sdk/access"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	oteltrace "go.opentelemetry.io/otel/trace"
)

func TestProviderAuthenticateCreatesSafeChildSpan(t *testing.T) {
	recorder := installAccessSpanRecorder(t)

	provider := NewProvider(fakeRepository{authenticate: func(_ context.Context, key, source string) (store.APIKeyPrincipal, error) {
		return store.APIKeyPrincipal{UserID: 42, APIKeyID: 17}, nil
	}}, "static")
	request := httptest.NewRequest(http.MethodGet, "/v1/models?key=cpn_live_query_secret", nil)
	request.Header.Set("Authorization", "Bearer cpn_live_header_secret")
	parent := oteltrace.NewSpanContext(oteltrace.SpanContextConfig{
		TraceID:    oteltrace.TraceID{1},
		SpanID:     oteltrace.SpanID{2},
		TraceFlags: oteltrace.FlagsSampled,
		Remote:     true,
	})

	result, authErr := provider.Authenticate(oteltrace.ContextWithRemoteSpanContext(context.Background(), parent), request)
	if authErr != nil || result == nil {
		t.Fatalf("Authenticate() result=%+v error=%v", result, authErr)
	}

	spans := recorder.Ended()
	if len(spans) != 1 {
		t.Fatalf("ended spans=%d, want 1", len(spans))
	}
	span := spans[0]
	if span.Name() != "access.Provider.Authenticate" || span.Parent().TraceID() != parent.TraceID() || span.Parent().SpanID() != parent.SpanID() {
		t.Fatalf("span name=%q parent=%v", span.Name(), span.Parent())
	}
	attributes := accessAttributeMap(span.Attributes())
	if attributes["auth.provider"].AsString() != ProviderIdentifier || attributes["auth.identity_source"].AsString() != "static" || attributes["auth.outcome"].AsString() != "success" || attributes["user.id"].AsInt64() != 42 || attributes["api_key.id"].AsInt64() != 17 {
		t.Fatalf("span attributes=%v", attributes)
	}
	for key, value := range attributes {
		encoded := fmt.Sprint(value.AsInterface())
		if strings.Contains(encoded, "cpn_live_header_secret") || strings.Contains(encoded, "cpn_live_query_secret") {
			t.Fatalf("span attribute %q leaks credential: %q", key, encoded)
		}
	}
}

func TestProviderAuthenticateDoesNotRecordRepositoryErrorMessage(t *testing.T) {
	recorder := installAccessSpanRecorder(t)
	const secret = "credential=repository-sentinel-secret"
	provider := NewProvider(fakeRepository{authenticate: func(context.Context, string, string) (store.APIKeyPrincipal, error) {
		return store.APIKeyPrincipal{}, errors.New(secret)
	}}, "ldap")
	request := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	request.Header.Set("Authorization", "Bearer cpn_live_header_secret")

	_, authErr := provider.Authenticate(context.Background(), request)
	if !sdkaccess.IsAuthErrorCode(authErr, sdkaccess.AuthErrorCodeInternal) {
		t.Fatalf("Authenticate() error=%#v", authErr)
	}

	spans := recorder.Ended()
	if len(spans) != 1 {
		t.Fatalf("ended spans=%d, want 1", len(spans))
	}
	span := spans[0]
	attributes := accessAttributeMap(span.Attributes())
	if span.Status().Code != codes.Error || attributes["auth.outcome"].AsString() != "internal_error" {
		t.Fatalf("span status=%v attributes=%v", span.Status(), attributes)
	}
	if accessSpanContains(span, secret) {
		t.Fatalf("span contains repository error secret %q", secret)
	}
}

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
	if result.Provider != ProviderIdentifier || result.Principal != "cliproxy:v1:42:17" || result.Metadata["api_key_id"] != "17" {
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

func accessAttributeMap(attributes []attribute.KeyValue) map[string]attribute.Value {
	result := make(map[string]attribute.Value, len(attributes))
	for _, attr := range attributes {
		result[string(attr.Key)] = attr.Value
	}
	return result
}

func installAccessSpanRecorder(t *testing.T) *tracetest.SpanRecorder {
	t.Helper()
	recorder := tracetest.NewSpanRecorder()
	provider := trace.NewTracerProvider(trace.WithSpanProcessor(recorder))
	previous := otel.GetTracerProvider()
	otel.SetTracerProvider(provider)
	t.Cleanup(func() {
		otel.SetTracerProvider(previous)
		_ = provider.Shutdown(context.Background())
	})
	return recorder
}

func accessSpanContains(span trace.ReadOnlySpan, value string) bool {
	if strings.Contains(span.Status().Description, value) {
		return true
	}
	for _, attr := range span.Attributes() {
		if strings.Contains(fmt.Sprint(attr.Value.AsInterface()), value) {
			return true
		}
	}
	for _, event := range span.Events() {
		for _, attr := range event.Attributes {
			if strings.Contains(fmt.Sprint(attr.Value.AsInterface()), value) {
				return true
			}
		}
	}
	return false
}
