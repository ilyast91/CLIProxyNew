// Package usage реализует публичные контракты usage.Plugin и coreauth.Hook
// (ADR-9, контракт 3) для аналитики и наблюдения за upstream-вызовами (R3).
//
// HandleUsage: versioned principal (user_id, api_key_id) из record.APIKey →
// bounded async bulk INSERT в usage_events (партиционированная).
// После успешного batch last_used_at уникальных API-ключей обновляется не чаще
// одного раза в минуту.
//
// Hook учитывает lifecycle credentials и результаты upstream-вызовов в
// потокобезопасных счётчиках; payload и credentials в нём не сохраняются.
//
// Важно (R3): HandleUsage может вызываться асинхронно в конце стриминга,
// когда request-context отменён → principal должен быть закодирован в record.APIKey.
//
// Имплементация: Фаза 3 (Contracts ADR-9).
package usage
