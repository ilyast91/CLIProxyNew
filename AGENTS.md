# AGENTS.md

Инструкция для агентов, работающих в этом репозитории.

## Назначение

`CLIProxyNew` — **бизнес-обвязка** над upstream relay-движком (CLI-агенты →
совместимый API). По архитектурной модели повторяет связку
[`CLIProxyAPI`](https://github.com/router-for-me/CLIProxyAPI) (ядро/SDK) +
[`CLIProxyAPIBusiness`](https://github.com/router-for-me/CLIProxyAPIBusiness)
(бизнес-слой).

- **Go module:** `github.com/ilyast91/CLIProxyNew` (go 1.26)
- **Ветка:** `main` (tracking `origin/main`).
- **Remote:** `origin` → `https://github.com/ilyast91/CLIProxyNew.git`

## Критически важно: модель интеграции

**Ядро upstream relay — внешняя Go-зависимость**
(`github.com/router-for-me/CLIProxyAPI/v7`, как `CLIProxyAPI/v6` в референсе).
В этом репозитории мы пишем ТОЛЬКО бизнес-слой.

- ❌ **Не пишем здесь:** refresh-протоколы провайдеров, transport, стриминг,
  парсинг ответов, реестр моделей как источник истины — это в ядре (SDK).
- ✅ **Пишем:** auth (LDAP), аналитику, management-API, per-call-type прокси,
  persistence (Postgres), observability, k8s-deployment.
- Бизнес-слой реализует **7 контрактов расширения** ядра (ADR-9) и делегирует
  upstream-вызовы через `Service.Run`.

→ **Не дублируйте upstream-специфику в `internal/`.** Если кажется, что нужен
  код конкретного провайдера — скорее всего, это вызов к SDK ядра.

## Структура каталогов

```
cmd/cliproxy/    — точка входа (wiring: конфиг, DI, Builder, Service.Run)
internal/        — бизнес-логика (неэкспортируемая)
  access/          — access.Provider: проверка клиентских API-keys
                     (+ проверка users.status на каждый запрос)
  auth/            — LDAP-логин, session-cookie, coreauth.Selector
                     (выбор upstream-аккаунта + per-call-type прокси R10)
  cache/           — in-process кэш (session/API-key lookup, модели)
                     за интерфейсом, задел под Redis
  config/          — конфигурация сервиса
  httpapi/         — клиентские эндпоинты (делегируют ядру) + management-API
                     (R9: user/admin операции) + middleware (LDAP-cookie)
  modelregistry/   — ModelRegistryHook: зеркало реестра моделей в Postgres
  security/        — bcrypt (API-keys) + AES-256-GCM (upstream/ldap secrets)
  store/           — репозитории (pgx + sqlc): users, api_keys, auths,
                     analytics, admin_audit_log, model_overrides
  usage/           — usage.Plugin: аналитика запросов (→ Postgres)
  watcher/         — WatcherFactory: poll БД + leader election (advisory lock)
db/migrations/   — SQL-миграции (golang-migrate)
docs/            — требования (R1–R10), ADR-9/ADR-10, дизайн
```

- В `cmd/` только wiring. Логику — в `internal/`.
- Слойность: `httpapi` → сервисы → `store`. Не вызывайте HTTP-слой из store.
- Генерируемый sqlc-код не правится руками — правьте `*.sql` и `sqlc generate`.

## Технологические решения (зафиксированы, 10 ADR)

- **Доступ к БД:** `pgx/v5` + `sqlc` (без ORM) + `golang-migrate`.
- **Аналитика:** та же Postgres, партиционирование по дню + материализованные
  агрегаты; слой репозитория абстрагирован (задел под ClickHouse).
- **Auth:** LDAP (bind/search, live-lookup групп) + opaque session (cookie,
  TTL user=5мин/admin=10ч, фикс. без продления) + long-lived API-keys (bcrypt).
- **Шифрование at-rest (R5):** два класса —
  bcrypt (one-way, API-keys) + AES-256-GCM (two-way, upstream-credentials +
  LDAP bind), мастер-ключ из env `CLIPROXY_ENCRYPTION_KEY` (k8s Secret),
  key-versioning для ротации.
- **Multi-tenancy:** плоская (пользователи + роли user/admin).
- **Scheduler (R7):** auto-refresh делает ядро (`StartAutoRefresh`), бизнес-слой
  реализует `coreauth.Store` (ядро само зовёт Save) + leader election через
  Postgres advisory lock.
- **Per-call-type прокси (R10):** подход A — динамический ProxyURL в
  `Selector.Pick` (inference/quota/models) и точках вызова. ⚠️ auto-refresh
  ядра идёт ММИМуя Selector → auth-прокси при auto-refresh = default аккаунта.
- **Management (R9):** только REST API (UI позже). Аудит-лог admin_audit_log.
  Блокировка пользователя — обратимая (users.status active/blocked).
- **Логи:** `slog`. **Метрики:** Prometheus. **Трейсы:** OpenTelemetry.
- **Без Redis** на первой версии (ADR-8).

## Контракты ядра, которые мы реализуем (ADR-9)

| Роль | Контракт ядра | Где |
|------|---------------|-----|
| Persist credentials | `coreauth.Store` (List/Save/Delete) | `internal/store` |
| Выбор аккаунта | `coreauth.Selector.Pick` | `internal/auth` |
| Аналитика | `coreauth.Hook.OnResult` + `usage.Plugin.HandleUsage` | `internal/usage` |
| Клиентский auth | `access.Provider.Authenticate` | `internal/access` |
| Auth-изменения | `cliproxy.WatcherFactory` | `internal/watcher` |
| Зеркало моделей | `cliproxy.SetGlobalModelRegistryHook` | `internal/modelregistry` |
| Middleware/роуты | `api.WithMiddleware`, `api.WithRouterConfigurator` | `internal/httpapi` |

## Команды

```bash
go build ./...          # сборка
go vet ./...            # статический анализ (всегда перед коммитом)
go run ./cmd/cliproxy   # запуск
go test ./...           # тесты
# sqlc:     sqlc generate        (после правки *.sql)
# миграции: migrate -path db/migrations -database "$DSN" up
```

## Gotchas

- `go build ./...` создаёт артефакт `cliproxy` в корне — он в `.gitignore`,
  но **не коммитьте бинарники**. Проверяйте `git status` перед коммитом.
- `.idea/` в `.gitignore` (GoLand/IntelliJ).
- Конфиги/секреты (`config.yaml`, `.env`, `*.pem`, `*.key`) игнорируются —
  шаблоны кладите как `*.example.*`.
- **Auto-refresh обходит Selector (R10):** per-call-type прокси НЕ применяется
  к авто-refresh — используется default аккаунта. См. ADR-10.
- **Principal в стриминге (R3):** `HandleUsage` вызывается асинхронно в конце
  потока — principal/user_id копируйте в Record при старте, не из context.
- **Гонки ProxyURL (R10):** `auth.ProxyURL` — разделяемое поле; при
  динамическом выставлении не persist'ите временное значение в Store.

## Документация (читать перед sensitive правками)

- [`docs/requirements.md`](docs/requirements.md) — требования R1–R10 и ADR.
  **Читать перед:** правками go.mod/зависимостей, контрактов с SDK ядра,
  схемы БД, слойности `internal/`.
- [`docs/architecture.md`](docs/architecture.md) — архитектурный дизайн
  (components, потоки данных, deployment). **Читать перед:** правками
  потоков/wiring/middleware.
- [`docs/database-schema.md`](docs/database-schema.md) — схема БД (ER,
  таблицы, индексы). **Читать перед:** правками store/миграций/sqlc.
- [`docs/adr/ADR-9-sdk-contracts.md`](docs/adr/ADR-9-sdk-contracts.md) —
  контракты интеграции. **Читать перед:** реализацией Store/Selector/Hook/
  usage.Plugin/access.Provider/WatcherFactory/ModelRegistryHook.
- [`docs/adr/ADR-10-per-call-type-proxy.md`](docs/adr/ADR-10-per-call-type-proxy.md)
  — per-call-type прокси. **Читать перед:** правками `Selector`/прокси.

## Соглашения

- **Импорты:** канонический путь `github.com/ilyast91/CLIProxyNew/...`.
- **Стиль:** табы для `.go`, пробелы для `.yaml`/`.md` (см. `.editorconfig`).
  Go-код — через `gofmt`/`goimports`.
- **Комментарии** — на русском (как в существующей документации);
  рядом с экспортируемыми идентификаторами — godoc-комментарий.
- **Без LICENSE** — пока не добавляйте лицензионные заголовки.

## История
- 2026-07-11 — создан при инициализации репозитория.
- 2026-07-11 — переписан под модель «ядро = внешняя go-зависимость».
- 2026-07-11 — актуализирован под R1–R10 и ADR-9/ADR-10 (полную структуру
  пакетов, 7 контрактов ядра, gotchas по auto-refresh/стримингу/гонкам).
