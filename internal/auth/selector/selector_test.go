package selector

import (
	"context"
	"testing"

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
