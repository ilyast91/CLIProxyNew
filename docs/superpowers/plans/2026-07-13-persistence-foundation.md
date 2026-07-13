# Ф1: persistence foundation

**Цель:** получить рабочий и проверяемый доступ к `users`, `api_keys` и `sessions` через `pgx/v5` + `sqlc`, не заходя в SDK-контракты Ф2/Ф3.

## Контракт среза

- `sqlc` генерирует типобезопасный пакет из миграций и `db/queries/*.sql`.
- `internal/store` предоставляет репозитории пользователей, API-ключей и сессий.
- Полный API-ключ никогда не хранится: репозиторий сохраняет bcrypt-хэш и ищет кандидатов по 8-символьному префиксу.
- Opaque session token хранится только как SHA-256 hash.
- `cmd/cliproxy` создаёт `pgxpool`, проверяет соединение и закрывает pool при shutdown.
- Интеграционные тесты поднимают PostgreSQL через testcontainers и накатывают настоящие миграции через golang-migrate.

## TDD-порядок

1. Добавить integration tests на upsert/block пользователя, API-key lookup и session lifecycle; получить ожидаемое падение из-за отсутствующего API.
2. Добавить `sqlc.yaml` и запросы, сгенерировать пакет `internal/store/dbgen`.
3. Реализовать pool и репозитории минимально для прохождения тестов.
4. Подключить pool в `cmd/cliproxy`, сохранив бизнес-логику в `internal/store`.
5. Запустить `sqlc generate`, `go test -count=1 ./...`, `go vet ./...` и `git diff --check`.

