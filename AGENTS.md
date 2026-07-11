# AGENTS.md

Инструкция для агентов, работающих в этом репозитории.

## Назначение

`CLIProxyNew` — **бизнес-обвязка** над upstream relay-движком (CLI-агенты →
совместимый API). По архитектурной модели повторяет связку
[`CLIProxyAPI`](https://github.com/router-for-me/CLIProxyAPI) (ядро/SDK) +
[`CLIProxyAPIBusiness`](https://github.com/router-for-me/CLIProxyAPIBusiness)
(бизнес-слой).

- **Go module:** `github.com/ilyast91/CLIProxyNew` (go 1.26)
- **Ветка:** `main`, git remote пока не настроен.

## Критически важно: модель интеграции

**Ядро upstream relay — внешняя Go-зависимость** (как `router-for-me/CLIProxyAPI/v6`
в референсе). В этом репозитории мы пишем ТОЛЬКО бизнес-слой.

- ❌ **Не пишем** здесь: refresh-протоколы провайдеров, transport, стриминг,
  парсинг ответов, реестр моделей провайдеров — это в ядре (SDK).
- ✅ **Пишем:** auth (LDAP), аналитику, API для клиентов, квоты/лимиты,
  watcher-оркестрацию (scheduler), persistence (Postgres), observability,
  k8s-deployment.
- Бизнес-слой вызывает ядро через контракты его SDK (напр. `sdk/cliproxy`),
  а сам хранит кэш моделей/credentials в Postgres и оркестрирует запуск refresh-джоб.

→ **Не дублируйте upstream-специфику в `internal/`.** Если кажется, что нужен код
  конкретного провайдера — скорее всего, это должен делать вызов к SDK ядра.

## Структура каталогов (конвенция)

```
cmd/            — точки входа (бинарники), только wiring (флаги, конфиг, запуск)
internal/       — бизнес-логика (неэкспортируемая)
  auth/           — LDAP, session-токены, API-keys
  usage/          — аналитика (события, агрегаты)
  watcher/        — оркестрация refresh токенов/моделей (leader election)
  store/          — репозитории (pgx + sqlc-генерация)
  httpapi/        — клиентские эндпоинты (OpenAI/Gemini/Claude-совместимые)
db/migrations/   — SQL-миграции (golang-migrate)
docs/            — требования, ADR, спецификации
```

- В `cmd/` только wiring. Логику — в `internal/`.
- Слойность: handlers → services → repositories. Не вызывайте HTTP-слой из store.
- Генерируемый sqlc-код кладётся рядом с пакетами (не правьте его руками).

## Технологические решения (зафиксированы)

- **Доступ к БД:** `pgx/v5` + `sqlc` (без ORM).
- **Миграции:** `golang-migrate` (SQL-файлы в `db/migrations/`).
- **Аналитика:** та же Postgres, партиционирование по дню + материализованные агрегаты.
- **Auth:** LDAP + opaque session-токены (DB-stored) + long-lived API-keys (hashed).
- **Scheduler leader election:** Postgres advisory lock.
- **Логи:** stdlib `slog`. **Метрики:** Prometheus. **Трейсы:** OpenTelemetry.
- **Multi-tenancy:** плоская (пользователи + роли user/admin).

## Команды

```bash
go build ./...          # сборка
go vet ./...            # статический анализ (всегда перед коммитом)
go run ./cmd/cliproxy   # запуск
go test ./...           # тесты
# sqlc:     sqlc generate        (после правки *.sql запросов)
# миграции: migrate -path db/migrations -database "$DSN" up
```

## Соглашения

- **Импорты:** канонический путь `github.com/ilyast91/CLIProxyNew/...`.
- **Стиль:** табы для `.go`, пробелы для `.yaml`/`.md` (см. `.editorconfig`).
  Go-код — через `gofmt`/`goimports`.
- **Комментарии** — на русском (как в существующем коде и документации);
  рядом с экспортируемыми идентификаторами — godoc-комментарий.
- **Без LICENSE** — пока не добавляйте лицензионные заголовки.

## Gotchas

- `go build ./...` кладёт артефакт `cliproxy` в корень — он в `.gitignore`, но
  бинарники коммитить нельзя.
- `.idea/` в `.gitignore` (GoLand/IntelliJ).
- Конфиги/секреты (`config.yaml`, `.env`, `*.pem`, `*.key`) игнорируются —
  шаблоны кладите как `*.example.*`.

## Документация (читать перед sensitive правками)

- [`docs/requirements.md`](docs/requirements.md) — требования и зафиксированные ADR.
  **Читать перед:** правками `go.mod`/зависимостей, контрактов с SDK ядра,
  схемы БД, слойности `internal/`.

## История
- 2026-07-11 — создан при инициализации репозитория.
- 2026-07-11 — переписан под модель «ядро = внешняя go-зависимость».
