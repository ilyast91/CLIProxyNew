# Ф1: PostgreSQL coreauth.Store

**Цель:** реализовать контракт SDK `coreauth.Store` поверх `upstream_accounts` без дублирования provider-specific логики.

## Контракт

- `Save` принимает ID ядра, валидирует provider/auth kind, шифрует serializable `coreauth.Auth` через активный AES-GCM key и делает upsert.
- `List` расшифровывает строки всеми доступными версиями keyring и возвращает `[]*coreauth.Auth`.
- `Delete` идемпотентно удаляет запись; FK `usage_events` сохраняет аналитику через `ON DELETE SET NULL`.
- `Auth.ProxyURL` не сохраняется: перед шифрованием он очищается, чтобы
  outbound HTTP использовал system proxy процесса.
- Открытая колонка `attributes` содержит только безопасные routing-поля; credentials остаются только в `credentials_enc`.
- `Auth.Storage` не сериализуется самим SDK, поэтому `Save` преобразует его
  credential payload в зашифрованный `Metadata`-снимок; runtime-поля не
  сохраняются.
- Встроенные storage сериализуются в памяти через JSON. Custom/plugin storage
  со скрытым состоянием обязан предоставить `RawJSON`; plaintext temp-файлы
  не используются.
- Активный key-version пишет новые строки, а предыдущие версии из
  `CLIPROXY_ENCRYPTION_PREVIOUS_KEYS` остаются доступны для чтения при ротации.

## TDD-порядок

1. Добавить compile-time assertion и integration contract test `Save → List → refresh Save → Delete`.
2. Проверить ciphertext/key version, отсутствие plaintext credentials и фильтрацию `ProxyURL`/секретных attributes.
3. Добавить sqlc-запросы и сгенерировать `dbgen`.
4. Реализовать `CoreAuthStore` с transparent AES и маппингом status/auth type/email.
5. Подключить keyring и `sdk/auth.RegisterTokenStore` в `cmd/cliproxy`.
6. Запустить generation, short/full tests, vet, build и независимый review.
