// Package selector реализует контракт coreauth.Selector (ADR-9, контракт 2).
//
// Pick: apply model_overrides (R9.A.6) → filter allow-list → fill-first.
//
// Ограничение (ADR-10): auto-refresh ядра идёт минуя Selector →
// auth-прокси при auto-refresh = default аккаунта.
//
// Имплементация: Фаза 3 (Contracts ADR-9) + Фаза 5 (R10 proxy).
package selector
