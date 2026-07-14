package selector

import (
	"context"
	"fmt"

	"github.com/ilyast91/CLIProxyNew/internal/store"
	coreauth "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/auth"
	"github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/executor"
)

// OverrideLister возвращает актуальный allow-list model overrides.
type OverrideLister interface {
	List(ctx context.Context) ([]store.ModelOverride, error)
}

// Selector реализует allow-list моделей поверх стандартного выбора ядра.
type Selector struct {
	overrides OverrideLister
	fallback  coreauth.FillFirstSelector
}

// New создаёт selector с источником model overrides.
func New(overrides OverrideLister) *Selector {
	return &Selector{overrides: overrides}
}

// Pick ограничивает кандидатов provider из override и делегирует выбор ядру.
func (s *Selector) Pick(
	ctx context.Context,
	provider, model string,
	opts executor.Options,
	auths []*coreauth.Auth,
) (*coreauth.Auth, error) {
	targetProvider, err := s.providerForModel(ctx, provider, model)
	if err != nil {
		return nil, err
	}

	candidates := filterByProvider(auths, targetProvider)
	if len(candidates) == 0 {
		return nil, fmt.Errorf("pick auth for provider %q: no candidates", targetProvider)
	}
	return s.fallback.Pick(ctx, targetProvider, model, opts, candidates)
}

func (s *Selector) providerForModel(ctx context.Context, provider, model string) (string, error) {
	if s == nil || s.overrides == nil {
		return provider, nil
	}

	overrides, err := s.overrides.List(ctx)
	if err != nil {
		return "", fmt.Errorf("list model overrides: %w", err)
	}
	if len(overrides) == 0 {
		return provider, nil
	}
	for _, override := range overrides {
		if override.ModelAlias != model {
			continue
		}
		if !override.Enabled {
			return "", fmt.Errorf("model %q is disabled", model)
		}
		return override.Provider, nil
	}
	return "", fmt.Errorf("model %q is not allowed", model)
}

func filterByProvider(auths []*coreauth.Auth, provider string) []*coreauth.Auth {
	if provider == "" {
		return append([]*coreauth.Auth(nil), auths...)
	}

	filtered := make([]*coreauth.Auth, 0, len(auths))
	for _, auth := range auths {
		if auth != nil && auth.Provider == provider {
			filtered = append(filtered, auth)
		}
	}
	return filtered
}
