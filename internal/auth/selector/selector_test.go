package selector

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/ilyast91/CLIProxyNew/internal/store"
	coreauth "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/auth"
	"github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/executor"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	oteltrace "go.opentelemetry.io/otel/trace"
)

func TestSelectorPickCreatesSafeChildSpan(t *testing.T) {
	recorder, parent := installSelectorSpanRecorder(t)
	selector := New(fakeOverrides{overrides: []store.ModelOverride{{
		Provider: "openai", ModelAlias: "business-gpt", UpstreamModel: "gpt-5", Enabled: true,
	}}})
	auths := []*coreauth.Auth{
		{ID: "anthropic", Provider: "anthropic"},
		{
			ID:         "openai-account",
			Provider:   "openai",
			Attributes: map[string]string{"api_key": "attribute-secret"},
			Metadata:   map[string]any{"access_token": "metadata-secret"},
		},
	}

	got, err := selector.Pick(oteltrace.ContextWithRemoteSpanContext(context.Background(), parent), "", "business-gpt", executor.Options{}, auths)
	if err != nil || got == nil {
		t.Fatalf("Pick()=%+v error=%v", got, err)
	}

	spans := recorder.Ended()
	if len(spans) != 1 {
		t.Fatalf("ended spans=%d, want 1", len(spans))
	}
	span := spans[0]
	if span.Name() != "selector.Pick" || span.Parent().TraceID() != parent.TraceID() || span.Parent().SpanID() != parent.SpanID() {
		t.Fatalf("span name=%q parent=%v", span.Name(), span.Parent())
	}
	attributes := selectorAttributeMap(span.Attributes())
	if attributes["selector.provider.requested"].AsString() != "" || attributes["selector.model"].AsString() != "business-gpt" || attributes["selector.candidate.count"].AsInt64() != 2 || attributes["selector.outcome"].AsString() != "success" || attributes["selector.auth.id"].AsString() != "openai-account" || attributes["selector.auth.provider"].AsString() != "openai" {
		t.Fatalf("span attributes=%v", attributes)
	}
	for key, value := range attributes {
		encoded := fmt.Sprint(value.AsInterface())
		if strings.Contains(encoded, "attribute-secret") || strings.Contains(encoded, "metadata-secret") {
			t.Fatalf("span attribute %q leaks credential metadata: %q", key, encoded)
		}
	}
}

func TestSelectorPickMarksSpanError(t *testing.T) {
	recorder, _ := installSelectorSpanRecorder(t)
	selector := New(fakeOverrides{overrides: []store.ModelOverride{{
		Provider: "openai", ModelAlias: "disabled", UpstreamModel: "gpt-5", Enabled: false,
	}}})

	if _, err := selector.Pick(context.Background(), "openai", "disabled", executor.Options{}, []*coreauth.Auth{{ID: "openai", Provider: "openai"}}); err == nil {
		t.Fatal("Pick() error = nil")
	}

	spans := recorder.Ended()
	if len(spans) != 1 {
		t.Fatalf("ended spans=%d, want 1", len(spans))
	}
	span := spans[0]
	attributes := selectorAttributeMap(span.Attributes())
	if span.Status().Code != codes.Error || attributes["selector.outcome"].AsString() != "error" {
		t.Fatalf("span status=%v attributes=%v", span.Status(), attributes)
	}
}

func TestSelectorPickDoesNotRecordOverrideErrorMessage(t *testing.T) {
	recorder, _ := installSelectorSpanRecorder(t)
	const secret = "refresh_token=selector-sentinel-secret"
	selector := New(fakeOverrides{err: errors.New(secret)})

	if _, err := selector.Pick(context.Background(), "openai", "gpt-5", executor.Options{}, []*coreauth.Auth{{ID: "openai", Provider: "openai"}}); err == nil {
		t.Fatal("Pick() error = nil")
	}

	spans := recorder.Ended()
	if len(spans) != 1 {
		t.Fatalf("ended spans=%d, want 1", len(spans))
	}
	span := spans[0]
	attributes := selectorAttributeMap(span.Attributes())
	if span.Status().Code != codes.Error || attributes["selector.outcome"].AsString() != "error" {
		t.Fatalf("span status=%v attributes=%v", span.Status(), attributes)
	}
	if selectorSpanContains(span, secret) {
		t.Fatalf("span contains override error secret %q", secret)
	}
}

func TestSelectorPickUsesEnabledOverrideProvider(t *testing.T) {
	selector := New(fakeOverrides{overrides: []store.ModelOverride{{
		Provider: "openai", ModelAlias: "business-gpt", UpstreamModel: "gpt-5", Enabled: true,
	}}})

	got, err := selector.Pick(context.Background(), "", "business-gpt", executor.Options{}, []*coreauth.Auth{
		{ID: "anthropic", Provider: "anthropic"},
		{ID: "openai", Provider: "openai"},
	})
	if err != nil {
		t.Fatalf("Pick() error = %v", err)
	}
	if got == nil || got.ID != "openai" {
		t.Fatalf("Pick() = %+v, want openai auth", got)
	}
}

func TestSelectorPickRejectsDisabledOrUnknownAlias(t *testing.T) {
	selector := New(fakeOverrides{overrides: []store.ModelOverride{{
		Provider: "openai", ModelAlias: "disabled", UpstreamModel: "gpt-5", Enabled: false,
	}}})
	auths := []*coreauth.Auth{{ID: "openai", Provider: "openai"}}

	if _, err := selector.Pick(context.Background(), "openai", "disabled", executor.Options{}, auths); err == nil {
		t.Fatal("Pick(disabled) error = nil")
	}
	if _, err := selector.Pick(context.Background(), "openai", "unknown", executor.Options{}, auths); err == nil {
		t.Fatal("Pick(unknown) error = nil")
	}
}

func TestSelectorPickAllowsAllModelsWithoutOverrides(t *testing.T) {
	selector := New(fakeOverrides{})

	got, err := selector.Pick(context.Background(), "openai", "gpt-5", executor.Options{}, []*coreauth.Auth{
		{ID: "openai", Provider: "openai"},
	})
	if err != nil {
		t.Fatalf("Pick() error = %v", err)
	}
	if got == nil || got.ID != "openai" {
		t.Fatalf("Pick() = %+v", got)
	}
}

func TestSelectorCachesOverridesWithinTTL(t *testing.T) {
	overrides := &countingOverrides{overrides: []store.ModelOverride{{
		Provider: "openai", ModelAlias: "business-gpt", UpstreamModel: "gpt-5", Enabled: true,
	}}}
	selector := New(overrides)
	auths := []*coreauth.Auth{{ID: "openai", Provider: "openai"}}

	for range 2 {
		if _, err := selector.Pick(context.Background(), "", "business-gpt", executor.Options{}, auths); err != nil {
			t.Fatalf("Pick() error = %v", err)
		}
	}
	if overrides.calls != 1 {
		t.Fatalf("List() calls = %d, want 1", overrides.calls)
	}
}

func TestSelectorFailsClosedWhenExpiredCacheCannotBeReloaded(t *testing.T) {
	overrides := &countingOverrides{overrides: []store.ModelOverride{{
		Provider: "openai", ModelAlias: "business-gpt", UpstreamModel: "gpt-5", Enabled: true,
	}}}
	selector := New(overrides)
	auths := []*coreauth.Auth{{ID: "openai", Provider: "openai"}}
	if _, err := selector.Pick(context.Background(), "", "business-gpt", executor.Options{}, auths); err != nil {
		t.Fatalf("initial Pick() error = %v", err)
	}

	selector.expiresAt = time.Now().Add(-time.Second)
	overrides.err = errors.New("database unavailable")
	if _, err := selector.Pick(context.Background(), "", "business-gpt", executor.Options{}, auths); err == nil {
		t.Fatal("Pick() error = nil after expired cache reload failure")
	}
}

type fakeOverrides struct {
	overrides []store.ModelOverride
	err       error
}

func (f fakeOverrides) List(context.Context) ([]store.ModelOverride, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.overrides, nil
}

type countingOverrides struct {
	overrides []store.ModelOverride
	calls     int
	err       error
}

func (f *countingOverrides) List(context.Context) ([]store.ModelOverride, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	return f.overrides, nil
}

func installSelectorSpanRecorder(t *testing.T) (*tracetest.SpanRecorder, oteltrace.SpanContext) {
	t.Helper()
	recorder := tracetest.NewSpanRecorder()
	provider := trace.NewTracerProvider(trace.WithSpanProcessor(recorder))
	previous := otel.GetTracerProvider()
	otel.SetTracerProvider(provider)
	t.Cleanup(func() {
		otel.SetTracerProvider(previous)
		_ = provider.Shutdown(context.Background())
	})
	parent := oteltrace.NewSpanContext(oteltrace.SpanContextConfig{
		TraceID:    oteltrace.TraceID{3},
		SpanID:     oteltrace.SpanID{4},
		TraceFlags: oteltrace.FlagsSampled,
		Remote:     true,
	})
	return recorder, parent
}

func selectorAttributeMap(attributes []attribute.KeyValue) map[string]attribute.Value {
	result := make(map[string]attribute.Value, len(attributes))
	for _, attr := range attributes {
		result[string(attr.Key)] = attr.Value
	}
	return result
}

func selectorSpanContains(span trace.ReadOnlySpan, value string) bool {
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
