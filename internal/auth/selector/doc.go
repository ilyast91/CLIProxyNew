// Package selector реализует контракт coreauth.Selector (ADR-9, контракт 2).
//
// Pick: TTL-кэш model_overrides (R9.A.6) → allow-list/provider filter → fill-first.
// Публичный Selector SDK не меняет downstream model, поэтому upstream_model
// сохраняется как desired mapping до появления публичного rewrite-hook.
//
// Ограничение (ADR-10): auto-refresh ядра идёт минуя Selector →
// auth-прокси при auto-refresh = default аккаунта.
//
// Имплементация: Фаза 3 (Contracts ADR-9) + Фаза 5 (R10 proxy).
package selector
