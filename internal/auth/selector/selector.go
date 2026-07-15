package selector

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/ilyast91/CLIProxyNew/internal/store"
	coreauth "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/auth"
	"github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/executor"
)

const overrideCacheTTL = 5 * time.Second

// OverrideLister возвращает актуальный allow-list model overrides.
type OverrideLister interface {
	List(ctx context.Context) ([]store.ModelOverride, error)
}

// Selector реализует allow-list моделей поверх стандартного выбора ядра.
type Selector struct {
	overrides OverrideLister
	fallback  coreauth.FillFirstSelector
	mu        sync.Mutex
	cache     []store.ModelOverride
	expiresAt time.Time
}

// New создаёт selector с источником model overrides.
func New(overrides OverrideLister) *Selector {
	return &Selector{overrides: overrides}
}

// Pick применяет allow-list, ограничивает кандидатов provider и делегирует выбор ядру.
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

	overrides, err := s.listOverrides(ctx)
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

func (s *Selector) listOverrides(ctx context.Context) ([]store.ModelOverride, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.expiresAt.IsZero() && time.Now().Before(s.expiresAt) {
		return s.cache, nil
	}
	overrides, err := s.overrides.List(ctx)
	if err != nil {
		return nil, err
	}
	s.cache = append(s.cache[:0], overrides...)
	s.expiresAt = time.Now().Add(overrideCacheTTL)
	return s.cache, nil
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
