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
- ✅ **Пишем:** auth (LDAP + static source для development/test), аналитику,
  management-API, system egress proxy,
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
  auth/            — identity providers (LDAP/static), session-cookie, coreauth.Selector
                     (выбор upstream-аккаунта)
  cache/           — in-process кэш (session/API-key lookup, модели)
                     за интерфейсом, задел под Redis
  config/          — конфигурация сервиса
  httpapi/         — клиентские эндпоинты (делегируют ядру) + management-API
                     (R9: user/admin операции) + middleware (session-cookie);
                     типы/хендлеры генерируются из openapi.yaml (R11)
  modelregistry/   — ModelRegistryHook: зеркало реестра моделей в Postgres
  metrics/         — изолированный Prometheus registry: HTTP, usage, upstream, pgx pool
  observability/   — slog redaction и общие observability-компоненты
  security/        — bcrypt (API-keys) + AES-256-GCM (upstream credentials)
  store/           — репозитории (pgx + sqlc): users, api_keys, auths,
                     analytics, admin_audit_log, model_overrides
  usage/           — usage.Plugin: аналитика запросов (→ Postgres)
  watcher/         — WatcherFactory no-op + DB revision restart + advisory leader jobs
db/migrations/   — SQL-миграции (golang-migrate)
docs/            — требования (R1–R12), ADR-9/ADR-10, дизайн
```

- В `cmd/` только wiring. Логику — в `internal/`.
- Слойность: `httpapi` → сервисы → `store`. Не вызывайте HTTP-слой из store.
- Генерируемый sqlc-код не правится руками — правьте `*.sql` и `sqlc generate`.
- `internal/openapi/openapi.json`,
  `internal/openapi/ogen/openapi.compat.yaml` и `internal/openapi/ogen/oas_*_gen.go`
  генерируются из `openapi.yaml`; не правьте их вручную, используйте
  `go generate ./internal/openapi/...`.

## Технологические решения (зафиксированы, 10 ADR)

- **Доступ к БД:** `pgx/v5` + `sqlc` (без ORM) + `golang-migrate`.
- **Аналитика:** та же Postgres, партиционирование по дню + материализованные
  агрегаты; слой репозитория абстрагирован (задел под ClickHouse).
- **Auth:** LDAP в production (bind/search, live-lookup групп) или static
  identity source только в development/test + opaque session (cookie, TTL
  user=5мин/admin=10ч, фикс. без продления) + long-lived API-keys (bcrypt).
- **Шифрование at-rest (R5):** два класса —
  bcrypt (one-way, API-keys) + AES-256-GCM (two-way, upstream-credentials).
  LDAP bind-password и static credentials живут только в env/k8s Secret;
  мастер-ключ из env `CLIPROXY_ENCRYPTION_KEY`,
  key-versioning для ротации; предыдущие версии ключей — из env
  `CLIPROXY_ENCRYPTION_PREVIOUS_KEYS` (JSON-карта version → base64).
- **Multi-tenancy:** плоская (пользователи + роли user/admin).
- **Scheduler (R7):** auto-refresh делает ядро (`StartAutoRefresh`), бизнес-слой
  реализует `coreauth.Store` (ядро само зовёт Save) + leader election через
  Postgres advisory lock.
- **System proxy (R10):** все outbound HTTP-клиенты используют
  `HTTP_PROXY`/`HTTPS_PROXY`/`NO_PROXY`; не добавляйте `proxy.*`,
  `CLIPROXY_PROXY_*` или `Auth.ProxyURL` overrides.
- **Management (R9):** только REST API (UI позже). Аудит-лог admin_audit_log.
  Блокировка пользователя — обратимая (users.status active/blocked).
- **Логи:** `slog`. **Метрики:** Prometheus. **Трейсы:** OpenTelemetry.
- **Без Redis** на первой версии (ADR-8).
- **Обновление SDK (R12):** только через публичные `sdk/*` пакеты и версию в
  `go.mod`; перед merge обновления обязательны `sdk-reference.md`,
  contract/integration/race проверки. Новый major — только с ADR.

## Контракты ядра, которые мы реализуем (ADR-9)

| Роль | Контракт ядра | Где |
|------|---------------|-----|
| Persist credentials | `coreauth.Store` (List/Save/Delete) | `internal/store` |
| Выбор аккаунта | `coreauth.Selector.Pick` | `internal/auth` |
| Аналитика | `usage.Plugin.HandleUsage` + `coreauth.Hook` | `internal/usage` |
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
# OpenAPI:  go generate ./internal/openapi/...  (после правки openapi.yaml)
# миграции: migrate -path db/migrations -database "$DSN" up
```

## Gotchas

- `go build ./...` создаёт артефакт `cliproxy` в корне — он в `.gitignore`,
  но **не коммитьте бинарники**. Проверяйте `git status` перед коммитом.
- `.idea/` в `.gitignore` (GoLand/IntelliJ).
- Конфиги/секреты (`config.yaml`, `.env`, `*.pem`, `*.key`) игнорируются —
  шаблоны кладите как `*.example.*`.
- **System proxy (R10):** `Auth.ProxyURL` должен оставаться пустым при
  Load/Save; proxy policy задает окружение процесса, включая auto-refresh.
- **Principal в стриминге (R3):** `HandleUsage` вызывается асинхронно в конце
  потока — principal/user_id копируйте в Record при старте, не из context.
- **Static identity (R1.5):** разрешён только при
  `server.environment=development|test`; никогда не используйте его как LDAP
  fallback и не переключайте `auth.mode` rolling-обновлением.
- **SDK boundary (R12):** не импортируйте `internal/*` upstream SDK и не
  используйте reflect-обходы. SDK обновляется отдельным reviewable изменением;
  patch/minor после `go test -race ./internal/sdkcontract` и compatibility
  gate, major — после ADR и миграционного плана.

## Документация (читать перед sensitive правками)

- [`docs/requirements.md`](docs/requirements.md) — требования R1–R12 и ADR.
  **Читать перед:** правками go.mod/зависимостей, контрактов с SDK ядра,
  схемы БД, слойности `internal/`.
- [`docs/architecture-principles.md`](docs/architecture-principles.md) —
  требования к архитектуре: принципы, quality attributes с SLA-метриками,
  тест-пирамида (CI gate до build), ADR immutable.
  **Читать перед:** любыми архитектурными решениями и правками CI.
- [`docs/architecture.md`](docs/architecture.md) — архитектурный дизайн
  (components, потоки данных, deployment). **Читать перед:** правками
  потоков/wiring/middleware.
- [`docs/database-schema.md`](docs/database-schema.md) — схема БД (ER,
  таблицы, индексы). **Читать перед:** правками store/миграций/sqlc.
- [`docs/adr/ADR-9-sdk-contracts.md`](docs/adr/ADR-9-sdk-contracts.md) —
  контракты интеграции. **Читать перед:** реализацией Store/Selector/Hook/
  usage.Plugin/access.Provider/WatcherFactory/ModelRegistryHook.
- [`docs/sdk-reference.md`](docs/sdk-reference.md) — полный референс публичного
  API SDK ядра (типы, интерфейсы, сигнатуры). **Читать перед:** любым вызовом
  SDK ядра или реализацией контрактов ADR-9.
- [`docs/adr/ADR-10-per-call-type-proxy.md`](docs/adr/ADR-10-per-call-type-proxy.md)
  — system proxy. **Читать перед:** правками HTTP transport/credentials.
- [`docs/implementation-phases.md`](docs/implementation-phases.md) — план
  имплементации по фазам (Ф0–Ф7). **Читать перед:** началом работы над фазой.

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
- 2026-07-14 — добавлен R1.5: static identity source только для
  development/test, namespace `static:` и запрет rolling-переключения mode.
- 2026-07-14 — добавлен R12: обновляемость внешнего SDK через публичные
  контракты, compatibility gate и ADR для major-версий.
- 2026-07-15 — R10 переделан на system proxy через HTTP_PROXY/HTTPS_PROXY/
  NO_PROXY; `proxy.*` и per-account ProxyURL overrides удалены.
