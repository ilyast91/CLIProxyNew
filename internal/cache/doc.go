// Package cache реализует in-process TTL-кэш (R6, ADR-8).
//
// Назначения:
//   - api_key_lookup: key_prefix → {key_hash, user_id, status}, TTL 5–15с
//   - model_overrides: полный набор (invalidation при admin-change)
//
// Кэш не хранит plaintext credentials. При необходимости Redis добавляется
// на границе репозиториев без изменения их вызывающего кода.
// Кэширование session_lookup требует общей инвалидации при блокировке
// пользователя и будет добавлено вместе с management wiring.
package cache
