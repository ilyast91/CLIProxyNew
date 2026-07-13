// Package cache реализует in-process кэш за интерфейсом (R6, ADR-8).
//
// Назначения:
//   - session_lookup: token_hash → {user_id, role, status}, TTL 5–15с
//   - api_key_lookup: key_prefix → {key_hash, user_id, status}, TTL 5–15с
//   - model_overrides: полный набор (invalidation при admin-change)
//
// Задел под Redis (ADR-8): реализация заменяема через интерфейс.
//
// Имплементация: Фаза 2 (Auth).
package cache
