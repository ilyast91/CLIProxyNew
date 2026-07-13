// Package security реализует шифрование секретов at-rest (R5).
//
// Два класса:
//   - Односторонние хэши (one-way): bcrypt cost 12 — для API-keys/паролей
//     (HashPassword/CheckPassword).
//   - Обратимое шифрование (two-way): AES-256-GCM с key-version prefix —
//     для upstream-credentials в БД (Encrypt/Decrypt).
//
// Секреты вне БД (LDAP bind, мастер-ключ, DB password) — только env,
// не шифруются AES (см. R5 исправление в requirements.md).
//
// Имплементация: Фаза 1 (Persistence).
package security
