package selector

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ilyast91/CLIProxyNew/internal/store"
	coreauth "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/auth"
	"github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/executor"
)

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
