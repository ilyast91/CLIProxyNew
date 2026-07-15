package usage

import (
	"context"
	"sync/atomic"

	coreauth "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/auth"
)

// HookSnapshot — текущие значения счётчиков lifecycle и результатов upstream-вызовов.
type HookSnapshot struct {
	AuthRegistered uint64
	AuthUpdated    uint64
	Results        uint64
	Succeeded      uint64
	Failed         uint64
}

// Hook реализует публичный coreauth.Hook для наблюдения за lifecycle credentials и результатами вызовов.
type Hook struct {
	authRegistered atomic.Uint64
	authUpdated    atomic.Uint64
	results        atomic.Uint64
	succeeded      atomic.Uint64
	failed         atomic.Uint64
}

var _ coreauth.Hook = (*Hook)(nil)

// NewHook создаёт потокобезопасный hook наблюдения.
func NewHook() *Hook {
	return &Hook{}
}

// OnAuthRegistered учитывает регистрацию upstream credential.
func (h *Hook) OnAuthRegistered(context.Context, *coreauth.Auth) {
	if h != nil {
		h.authRegistered.Add(1)
	}
}

// OnAuthUpdated учитывает изменение состояния upstream credential.
func (h *Hook) OnAuthUpdated(context.Context, *coreauth.Auth) {
	if h != nil {
		h.authUpdated.Add(1)
	}
}

// OnResult учитывает результат upstream-вызова без сохранения request payload или credentials.
func (h *Hook) OnResult(_ context.Context, result coreauth.Result) {
	if h == nil {
		return
	}
	h.results.Add(1)
	if result.Success {
		h.succeeded.Add(1)
		return
	}
	h.failed.Add(1)
}

// Snapshot возвращает текущие значения накопленных счётчиков.
func (h *Hook) Snapshot() HookSnapshot {
	if h == nil {
		return HookSnapshot{}
	}
	return HookSnapshot{
		AuthRegistered: h.authRegistered.Load(),
		AuthUpdated:    h.authUpdated.Load(),
		Results:        h.results.Load(),
		Succeeded:      h.succeeded.Load(),
		Failed:         h.failed.Load(),
	}
}
