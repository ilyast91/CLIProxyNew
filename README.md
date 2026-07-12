# CLIProxyNew

Бизнес-обвязка над upstream relay-движком — оборачивает CLI-агентов (Codex, Claude Code, Gemini CLI и др.) в OpenAI/Gemini/Claude/Codex/Grok-совместимый API с auth (LDAP), аналитикой использования, management-поверхностью для пользователей и администраторов, per-call-type egress-прокси и observability.

## Архитектурная модель

По аналогии со связкой
[`router-for-me/CLIProxyAPI`](https://github.com/router-for-me/CLIProxyAPI)
(ядро/SDK) +
[`router-for-me/CLIProxyAPIBusiness`](https://github.com/router-for-me/CLIProxyAPIBusiness)
(бизнес-слой):

| Слой | Роль | Где живёт |
|------|------|-----------|
| **Ядро (upstream relay engine)** | Вызовы провайдеров, transport, streaming, парсинг, refresh-протоколы OAuth, реестр моделей, плагины | **Внешняя Go-зависимость** в `go.mod` (мы её не пишем) |
| **CLIProxyNew (этот репо)** | Auth (LDAP), аналитика, management-API (user/admin), per-call-type прокси, БД, observability, k8s | Здесь |

> **Принцип:** ядро — внешняя go-зависимость. Бизнес-слой реализует **7 контрактов расширения** ядра (ADR-9): `coreauth.Store`, `coreauth.Selector`, `coreauth.Hook`, `usage.Plugin`, `access.Provider`, `WatcherFactory`, `ModelRegistryHook`. Мы не дублируем upstream-специфику (refresh-протоколы, transport) — делегируем ядру.

## Стек

- **Go 1.26**
- **БД:** Postgres + `pgx/v5` + `sqlc` + `golang-migrate` (без ORM)
- **Аналитика:** Postgres (партиционирование по дню + материализованные агрегаты; задел под ClickHouse)
- **Auth:** LDAP (bind/search, live-lookup групп), opaque session-токены (cookie) + long-lived API-keys (bcrypt)
- **Шифрование at-rest:** bcrypt (API-keys) + AES-256-GCM (upstream-credentials, LDAP bind), мастер-ключ из env (k8s Secret), key-versioning для ротации
- **Scheduler/watcher:** ядро делает auto-refresh, бизнес-слой оркеструет (Postgres advisory lock для leader election)
- **Egress-прокси:** per-call-type (inference/auth/quota/models), direct по умолчанию (R10)
- **Observability:** Prometheus, OpenTelemetry, `slog`
- **Деплой:** k8s, multi-replica, stateless (без Redis на первой версии)

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
```

## Документация

- [`docs/requirements.md`](docs/requirements.md) — требования R1–R10 (зафиксированы)
- [`docs/architecture.md`](docs/architecture.md) — архитектурный дизайн (components, потоки, deployment)
- [`docs/database-schema.md`](docs/database-schema.md) — схема БД (ER, таблицы, индексы, миграции)
- [`docs/sdk-reference.md`](docs/sdk-reference.md) — референс публичного API SDK ядра (типы, интерфейсы, сигнатуры)
- [`docs/design/r9-oauth-and-testing.md`](docs/design/r9-oauth-and-testing.md) — дизайн OAuth login-flow и тестирования аккаунтов (R9.A.1, R9.A.5)
- [`docs/adr/ADR-9-sdk-contracts.md`](docs/adr/ADR-9-sdk-contracts.md) — контракты интеграции с ядром (7 интерфейсов)
- [`docs/adr/ADR-10-per-call-type-proxy.md`](docs/adr/ADR-10-per-call-type-proxy.md) — per-call-type egress proxy routing
- [`AGENTS.md`](AGENTS.md) — инструкция для агентов

## Статус

📋 Требования зафиксированы (10 ADR закрыты). Архитектурный дизайн и схема БД описаны. Следующий этап — имплементация компонентов.
