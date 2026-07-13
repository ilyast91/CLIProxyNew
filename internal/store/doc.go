// Package store реализует слой репозиториев (pgx + sqlc) и контракт
// coreauth.Store (ADR-9, контракт 1).
//
// Репозитории для таблиц (см. docs/database-schema.md):
//   - users, api_keys, sessions
//   - upstream_accounts (coreauth.Store с transparent AES-шифрованием)
//   - model_overrides
//   - usage_events (партиционированная)
//   - admin_audit_log
//   - oauth_sessions
//
// Имплементация: Фаза 1 (Persistence).
package store
