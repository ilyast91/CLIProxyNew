// Package access реализует контракт access.Provider (ADR-9, контракт 4) —
// проверку клиентских API-keys (R2).
//
// Authenticate: lookup api_keys по prefix → bcrypt verify → check users.status
// → Result{Principal=user_id, Metadata={api_key_id}}.
// Регистрируется через access.RegisterProvider("db-apikey", ...) +
// access.SetExclusiveProvider("db-apikey") (отключает inline cfg.APIKeys ядра).
//
// Имплементация: Фаза 2 (Auth).
package access
