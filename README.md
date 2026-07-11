# CLIProxyNew

Бизнес-обвязка над upstream relay-движком — оборачивает CLI-агентов (Codex, Claude Code, Gemini CLI и др.) в OpenAI/Gemini/Claude-совместимый API с auth, аналитикой, квотами и observability.

## Архитектурная модель

По аналогии со связкой
[`router-for-me/CLIProxyAPI`](https://github.com/router-for-me/CLIProxyAPI)
(ядро/SDK) +
[`router-for-me/CLIProxyAPIBusiness`](https://github.com/router-for-me/CLIProxyAPIBusiness)
(бизнес-слой):

| Слой | Роль | Где живёт |
|------|------|-----------|
| **Ядро (upstream relay engine)** | Вызовы провайдеров, transport, streaming, парсинг, refresh токенов, реестр моделей, плагины | **Внешняя Go-зависимость** в `go.mod` (мы её не пишем) |
| **CLIProxyNew (этот репо)** | Auth (LDAP), аналитика, API для клиентов, квоты, watcher-оркестрация, БД, observability, k8s | Здесь |

> Принцип: ядро — внешняя go-зависимость. Вся ответственность этого репозитория — бизнес-обвязка поверх него. Мы не дублируем upstream-специфику (refresh-протоколы провайдеров, transport) — делегируем ядру через его SDK.

## Стек

- **Go 1.26**
- **БД:** Postgres + `pgx` + `sqlc` + `golang-migrate`
- **Аналитика:** Postgres (партиционированные таблицы + материализованные агрегаты)
- **Auth:** LDAP, opaque session-токены + long-lived API-keys
- **Scheduler:** Postgres advisory lock (leader election)
- **Observability:** Prometheus, OpenTelemetry, `slog`
- **Деплой:** k8s, multi-replica, stateless

## Структура

```
cmd/         — точки входа (бинарники)
internal/    — внутренняя бизнес-логика
  auth/        LDAP, session/API-key
  usage/       аналитика
  watcher/     оркестрация refresh
  store/       репозитории (pgx + sqlc)
db/migrations/ — SQL-миграции golang-migrate
docs/        — документация и требования
```

## Документация

- [`docs/requirements.md`](docs/requirements.md) — требования (в активном обсуждении)
- [`AGENTS.md`](AGENTS.md) — инструкция для агентов

## Статус

🚧 Ранний этап: сбор требований и проектирование архитектуры.
