# CLIProxyNew

Бизнес-обвязка над upstream relay-движком — оборачивает CLI-агентов (Codex, Claude Code, Gemini CLI и др.) в OpenAI/Gemini/Claude/Codex/Grok-совместимый API с auth (LDAP), аналитикой использования, management-поверхностью для пользователей и администраторов, system egress proxy и observability.

## Архитектурная модель

По аналогии со связкой
[`router-for-me/CLIProxyAPI`](https://github.com/router-for-me/CLIProxyAPI)
(ядро/SDK) +
[`router-for-me/CLIProxyAPIBusiness`](https://github.com/router-for-me/CLIProxyAPIBusiness)
(бизнес-слой):

| Слой | Роль | Где живёт |
|------|------|-----------|
| **Ядро (upstream relay engine)** | Вызовы провайдеров, transport, streaming, парсинг, refresh-протоколы OAuth, реестр моделей, плагины | **Внешняя Go-зависимость** в `go.mod` (мы её не пишем) |
| **CLIProxyNew (этот репо)** | Auth (LDAP/static для development/test), аналитика, management API (user/admin), БД, system proxy, observability, k8s | Здесь |

> **Принцип:** ядро — внешняя go-зависимость. Бизнес-слой реализует **7 контрактов расширения** ядра (ADR-9): `coreauth.Store`, `coreauth.Selector`, `coreauth.Hook`, `usage.Plugin`, `access.Provider`, `WatcherFactory`, `ModelRegistryHook`. Мы не дублируем upstream-специфику (refresh-протоколы, transport) — делегируем ядру.

## Стек

- **Go 1.26.5**
- **Upstream SDK:** `github.com/router-for-me/CLIProxyAPI/v7` **v7.2.80**
- **БД:** Postgres + `pgx/v5` + `sqlc` + `golang-migrate` (без ORM)
- **Аналитика:** Postgres (партиционирование по дню + материализованные агрегаты; задел под ClickHouse)
- **Auth:** LDAP (bind/search, live-lookup групп), opaque session-токены (cookie) + long-lived API-keys (bcrypt)
- **Шифрование at-rest:** bcrypt (API-keys) + AES-256-GCM (upstream credentials); LDAP bind-password остаётся только в env/k8s Secret, key-versioning поддерживает ротацию
- **Scheduler/watcher:** ядро делает auto-refresh, бизнес-слой оркеструет (Postgres advisory lock для leader election)
- **Egress-прокси:** единая policy процесса через `HTTP_PROXY`, `HTTPS_PROXY`, `NO_PROXY` (R10)
- **Observability:** Prometheus 1.23.2, OpenTelemetry 1.44, `slog`
- **Деплой:** k8s, multi-replica, stateless (без Redis на первой версии)

Ключевые build-зависимости зафиксированы в `go.mod`: Gin 1.12.0, pgx 5.10.0,
ogen 1.23.0, testcontainers 0.43.0, swaggest/swgui 1.8.9. CI использует Go
1.26.5, GitHub Actions v7, Node.js 24 и Spectral CLI 6.16.1.

## Структура

```
cmd/cliproxy/    — точка входа (wiring: конфиг, DI, запуск Service)
internal/        — бизнес-логика
  access/          access.Provider — проверка клиентских API-keys (+ users.status)
  auth/            LDAP, session-cookie, coreauth.Selector (выбор аккаунта)
  cache/           in-process кэш (session/API-key lookup, модели)
  config/          конфигурация (LDAP, прокси, шифрование, ...)
  httpapi/         клиентские эндпоинты + management-API + middleware
  modelregistry/   ModelRegistryHook — зеркало реестра моделей в Postgres
  security/        bcrypt + AES-256-GCM (2 класса секретов)
  store/           репозитории (pgx + sqlc): users, api_keys, auths, analytics, audit
  usage/           usage.Plugin — аналитика запросов
  watcher/         WatcherFactory — poll БД + leader election
db/migrations/   — SQL-миграции golang-migrate
docs/            — требования, ADR, дизайн
  runbooks/         — production-операции и обновление SDK
```

## Документация

После запуска сервиса HTTP-контракт доступен в двух представлениях:

- `/openapi.json` — встроенный OpenAPI 3.1 JSON, сгенерированный из
  `openapi.yaml`;
- `/docs` — Swagger UI 5.32.8 поверх `/openapi.json`; HTML, JS, CSS и favicon
  встроены в Go-бинарник через `swaggest/swgui/v5emb` и не требуют CDN.

Runtime не использует неявные внешние UI/static resources. Сетевые зависимости
ограничены явно настроенной инфраструктурой и upstream/provider endpoints,
которые составляют основную функцию сервиса.

- [`docs/requirements.md`](docs/requirements.md) — требования R1–R12 (зафиксированы)
- [`docs/architecture-principles.md`](docs/architecture-principles.md) — требования к архитектуре (принципы, quality attributes, SLA, тестирование)
- [`docs/architecture.md`](docs/architecture.md) — архитектурный дизайн (components, потоки, deployment)
- [`docs/database-schema.md`](docs/database-schema.md) — схема БД (ER, таблицы, индексы, миграции)
- [`docs/sdk-reference.md`](docs/sdk-reference.md) — референс публичного API CLIProxyAPI v7.2.80 и результат compatibility-сверки
- [`docs/design/r9-oauth-and-testing.md`](docs/design/r9-oauth-and-testing.md) — дизайн OAuth login-flow и тестирования аккаунтов (R9.A.1, R9.A.5)
- [`docs/implementation-phases.md`](docs/implementation-phases.md) — план имплементации по фазам (Ф0–Ф7)
- [`docs/runbooks/sdk-upgrade.md`](docs/runbooks/sdk-upgrade.md) — обновление и rollback upstream SDK по R12
- [`docs/runbooks/postgres-restore.md`](docs/runbooks/postgres-restore.md) — проверка backup и восстановление PostgreSQL
- [`docs/runbooks/encryption-key-rotation.md`](docs/runbooks/encryption-key-rotation.md) — rolling-safe ротация AES master-key
- [`docs/runbooks/api-key-rotation.md`](docs/runbooks/api-key-rotation.md) — ротация клиентского API-key
- [`docs/runbooks/ldap-bind-password-rotation.md`](docs/runbooks/ldap-bind-password-rotation.md) — ротация LDAP bind-password
- [`docs/adr/ADR-9-sdk-contracts.md`](docs/adr/ADR-9-sdk-contracts.md) — контракты интеграции с ядром (7 интерфейсов)
- [`docs/adr/ADR-10-per-call-type-proxy.md`](docs/adr/ADR-10-per-call-type-proxy.md) — system egress proxy через окружение процесса
- [`deploy/kubernetes/README.md`](deploy/kubernetes/README.md) — production baseline, Secret, migration и rollout в Kubernetes
- [`AGENTS.md`](AGENTS.md) — инструкция для агентов

## Статус

Ф0–Ф6 закрыты в доступном SDK scope. В Ф7 закрыты автоматизированные release
gates, SDK/operations runbooks и package godoc; до production v1 остаются два
chaos/failover gate. Детальный статус:
[`docs/implementation-phases.md`](docs/implementation-phases.md).
