# ADR-9: Контракты интеграции с SDK ядра CLIProxyAPI v7

> **Статус:** Принят (по результатам исследования SDK v7/main).
> **Дата:** 2026-07-11
> **Связанные:** ADR-1 (модель интеграции), R1–R8.

## Контекст

Ядро `github.com/router-for-me/CLIProxyAPI/v7` подключается как внешняя
Go-зависимость. Бизнес-слой `CLIProxyNew` не дублирует upstream-специфику
(refresh-протоколы, transport, стриминг, реестр моделей) — он реализует
контракты расширения ядра и делегирует остальное.

Важно: референс `CLIProxyAPIBusiness` на main зависит от **v6** и прибивает
watcher через `reflect` (v6 не экспортировал конструктор `WatcherWrapper`). В
**v7** `WatcherWrapper` имеет публичные поля-функции → reflect-хаки не нужны.
ADR фиксирует целевые контракты **v7**.

## Решение

Бизнес-слой реализует **ровно 7 контрактов** ядра. Вся остальная
ответственность (refresh, transport, стриминг, парсинг, реестр моделей как
источник истины) — в ядре.

### Точка входа — `sdk/cliproxy`

```go
service, _ := cliproxy.NewBuilder().
    WithConfig(cfg).
    WithConfigPath(configPath).
    WithCoreAuthManager(coreManager).         // Store + Selector + Hook
    WithRequestAccessManager(accessManager).  // клиентский auth
    WithWatcherFactory(dbWatcherFactory).     // poll БД вместо fs
    WithServerOptions(
        api.WithMiddleware(/* auth, logging, cors */),
        api.WithRouterConfigurator(/* admin/front роуты */),
    ).
    Build()
service.RegisterUsagePlugin(usagePlugin)     // аналитика
service.Run(ctx)
```

`Service.Run` сам: грузит auths, поднимает Gin-сервер, запускает
`coreManager.StartAutoRefresh(ctx, 15*time.Minute)` и регистрирует
model-refresh callback.

### Контракт 1 — Persists credentials: `coreauth.Store`

```go
// sdk/cliproxy/auth/store.go
type Store interface {
    List(ctx context.Context) ([]*Auth, error)
    Save(ctx context.Context, auth *Auth) (string, error)
    Delete(ctx context.Context, id string) error
}
```

Реализация: `internal/store` (pgx + sqlc). **Ядро само вызывает `Save` после
refresh/login.** Глобальная регистрация ДО Builder'а:

```go
sdkAuth.RegisterTokenStore(pgStore)  // sdk/auth/store_registry.go
```

### Контракт 2 — Выбор auth под запрос: `coreauth.Selector`

```go
// sdk/cliproxy/auth/selector.go
type Selector interface {
    Pick(ctx, provider, model string, opts Options, auths []*Auth) (*Auth, error)
}
```

Реализация: `internal/auth/selector`. Внутри — фильтр по user-группам/квотам,
model-mapping (round-robin/fill-first/stick), rate-limit проверка.

### Контракт 3 — Аналитика: `coreauth.Hook` + `usage.Plugin`

```go
// sdk/cliproxy/auth/conductor.go
type Hook interface {
    OnAuthRegistered(ctx, auth)
    OnAuthUpdated(ctx, auth)
    OnResult(ctx, Result)              // каждый выполненный запрос
}
type NoopHook struct{}                 // встраивается для частичной реализации

// sdk/cliproxy/usage/manager.go
type Plugin interface {
    HandleUsage(ctx, Record)
}
type Record struct {
    Provider, Model, Alias, AuthID, AuthType, ReasoningEffort string
    Latency, TTFT time.Duration
    Failed bool
    Failure struct{ StatusCode int; Body string }
    Detail  struct{ Input, Output, Reasoning, Cached, TotalTokens int }
}
```

Реализация: `internal/usage` — `usage.Plugin` пишет сырые события в Postgres
(R3). `coreauth.Hook.OnResult` — доп. точка для квот/алёртов.

### Контракт 4 — Клиентский auth: `access.Provider`

```go
// sdk/access/registry.go, types.go
type Provider interface {
    Identifier() string
    Authenticate(ctx, *http.Request) (*Result, *AuthError)
}
func RegisterProvider(typ string, provider Provider)
```

Реализация: `internal/access` — проверка **API-key из БД** (R2.2, bcrypt-сверка).
Внимание: LDAP-логин (R1) — это **отдельный** флоу (login endpoint → cookie),
не `access.Provider`; `access.Provider` проверяет API-key на каждый запрос к
прокси-API. Регистрируется и передаётся в Builder через
`WithRequestAccessManager`.

### Контракт 5 — Оркестрация auth-изменений: `WatcherFactory`

```go
// sdk/cliproxy/types.go
type WatcherFactory func(configPath, authDir string, reload func(*config.Config)) (*WatcherWrapper, error)
```

Реализация: `internal/watcher` — **poll БД** (не файловая система). Пушит
`coreauth.Auth`-обновления в очередь ядра через `SetAuthUpdateQueue`.
В multi-replica работает только на лидере (Postgres advisory lock, ADR-7).

### Контракт 6 — Зеркало реестра моделей: `ModelRegistryHook`

```go
// sdk/cliproxy/model_registry.go
func SetGlobalModelRegistryHook(hook ModelRegistryHook)
```

Реализация: `internal/modelregistry` — подписывается на изменения in-memory
реестра ядра, кэширует snapshot в Postgres (для UI/model-mapping). **Источник
истины — ядро**, бизнес-слой только зеркалирует. Эндпоинт `/v1/models` ядро
отдаёт само.

### Контракт 7 — HTTP middleware / свои роуты

```go
// sdk/api/options.go
api.WithMiddleware(mw ...gin.HandlerFunc)
api.WithRouterConfigurator(fn func(*gin.Engine, *BaseAPIHandler, *config.Config))
```

Используется для: auth middleware (LDAP cookie), logging, CORS, webUI,
admin/front-роутов.

## Что НЕ делает бизнес-слой (ответственность ядра)

- Refresh upstream-токенов (Codex/Claude/xAI/Antigravity/Kimi) —
  `coreManager.StartAutoRefresh` + `ProviderExecutor.Refresh`.
- Transport, стриминг, парсинг ответов провайдеров.
- Реестр моделей провайдеров (источник истины) — ядро, бизнес зеркалирует.
- Программные вызовы провайдеров (`coreauth.Manager.Execute/ExecuteStream`) —
  для клиентских HTTP-запросов ядро само роутит через свой Gin-сервер.

## Сводная таблица контрактов

| Роль | Контракт ядра | Реализация в CLIProxyNew |
|------|---------------|--------------------------|
| Создать и запустить ядро | `cliproxy.Builder` → `Service.Run` | `cmd/cliproxy/main.go` |
| Persists credentials | `coreauth.Store` | `internal/store` (pgx+sqlc) |
| Выбор auth под запрос | `coreauth.Selector` | `internal/auth/selector` |
| Аналитика запросов | `coreauth.Hook` + `usage.Plugin` | `internal/usage` |
| Клиентский auth (API-key) | `access.Provider` | `internal/access` |
| Auth-изменения (watcher) | `WatcherFactory` | `internal/watcher` |
| Зеркало моделей | `ModelRegistryHook` | `internal/modelregistry` |
| HTTP middleware / роуты | `api.With*` | `internal/httpapi` |

## Следствия для других требований

- **R7 (scheduler/watcher):** ядро само делает `StartAutoRefresh(15m)` с
  min-heap и до 16 воркеров. Бизнес-слой не пишет refresh-логику — он только
  (а) реализует `Store` (persist результата), (б) через advisory lock
  гарантирует, что watcher-poller БД работает на одной реплике. Открытые
  вопросы R7 (интервалы, retry/backoff) → уточняются у ядерных настроек
  `StartAutoRefresh` и `RefreshEvaluator`.
- **R3 (аналитика):** `usage.Plugin.HandleUsage(Record)` даёт готовую структуру
  для сырого события — Provider/Model/AuthID/токены/latency/status. Это
  снимает открытый вопрос «какие данные собирать».
- **R2 (API-key):** `access.Provider.Authenticate` — единая точка проверки на
  каждый запрос. LDAP-cookie (R1) — отдельный middleware в `WithMiddleware`.

## Открытые вопросы (после ADR-9)

- Точные настройки `StartAutoRefresh` (интервал, `RefreshEvaluator`,
  max-concurrency) — изучить при имплементации watcher'а.
- Формат persist'а `Auth` (metadata/runtime-поля) в схеме Postgres —
  при дизайне `internal/store`.
- Размер/ TTL кэша session-lookup (R6) с учётом `access.Provider` на каждый
  запрос.
