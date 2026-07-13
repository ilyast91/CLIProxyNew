// Package usage реализует контракты usage.Plugin и coreauth.Hook (ADR-9,
// контракт 3) — аналитика использования запросов (R3).
//
// HandleUsage: principal (user_id) из record.APIKey + api_key_id из
// record.Metadata → async bulk INSERT в usage_events (партиционированная).
//
// Важно (R3): HandleUsage может вызываться асинхронно в конце стриминга,
// когда request-context отменён → api_key_id должен быть в record.Metadata.
//
// Имплементация: Фаза 3 (Contracts ADR-9).
package usage
