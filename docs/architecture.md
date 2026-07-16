# Архитектура CLIProxyNew

> **Статус:** Дизайн.
> **Связанные:** [requirements.md](requirements.md) (R1–R12), [ADR-9](adr/ADR-9-sdk-contracts.md), [ADR-10](adr/ADR-10-per-call-type-proxy.md), [database-schema.md](database-schema.md).

## 1. Обзор

`CLIProxyNew` — бизнес-обвязка над upstream relay-движком
`github.com/router-for-me/CLIProxyAPI/v7` (далее «ядро»). Бизнес-слой
реализует **7 контрактов расширения** ядра (ADR-9) и добавляет: identity auth
(LDAP в production, static в development/test), аналитику, management-API,
system egress proxy, observability, multi-replica в k8s.

```mermaid
graph TB
    subgraph Client["Клиенты"]
        U["Пользователь UI/curl"]
        APP["Приложение / SDK"]
    end

    subgraph CLIProxyNew["CLIProxyNew — бизнес-слой"]
        H["httpapi<br/>middleware + management-API"]
        A["access.Provider<br/>проверка API-keys + status"]
        IDP["auth/identity<br/>LDAP/static provider"]
        LDAP["auth/ldap<br/>LDAP provider"]
        SEL["auth/selector<br/>выбор аккаунта + R10 прокси"]
        OAUTH["auth/oauth<br/>FlowManager R9.A.1"]
        TEST["auth/testing<br/>Checker R9.A.5"]
        USG["usage.Plugin<br/>аналитика"]
        ST["store<br/>pgx+sqlc to Postgres"]
        SEC["security<br/>bcrypt + AES-GCM"]
        W["watcher<br/>poller + leader election"]
        MR["modelregistry<br/>зеркало моделей"]
    end

    subgraph Core["Ядро — CLIProxyAPI v7, внешняя go-зависимость"]
        SVC["cliproxy.Service<br/>Gin-server + StartAutoRefresh"]
        EXEC["ProviderExecutor<br/>refresh + execute + stream"]
        REG["ModelRegistry<br/>источник истины моделей"]
    end

    subgraph DS["Хранилища"]
        PG[("Postgres")]
    end

    U -->|login| H
    U -->|cookie mgmt-API| H
    APP -->|API-key /v1/* прокси| H
    H --> A
    H --> IDP
    IDP --> LDAP
    H --> OAUTH
    H --> TEST
    H --> SEL
    H --> SVC
    SVC --> EXEC
    SVC --> REG
    A --> ST
    LDAP -.->|service bind| LDAPSrv["LDAP/AD"]
    LDAP --> ST
    SEL --> ST
    OAUTH --> ST
    USG --> ST
    W --> ST
    MR --> ST
    ST <--> PG
    A -.->|шифр bcrypt| SEC
    SVC -.->|Store.Save| ST
    SVC -.->|usage.Record| USG
    REG -.->|ModelRegistryHook| MR
    SEC -.->|AES-GCM| ST
```

### Развязка ответственности

| Делает ядро | Делает бизнес-слой (этот репо) |
|-------------|-------------------------------|
| Refresh-протоколы OAuth провайдеров | `coreauth.Store` (persist в Postgres) |
| Transport / стриминг / парсинг | `coreauth.Selector` (выбор аккаунта + R10 прокси) |
| Реестр моделей (источник истины) | `usage.Plugin` (аналитика) |
| Auto-refresh (`StartAutoRefresh`) | `access.Provider` (API-key проверка) |
| Gin-сервер, роутинг `/v1/*` | `WatcherFactory` (poll БД + leader) |
| | `ModelRegistryHook` (зеркало моделей) |
| | Management-API, identity auth, observability |

### Граница обновления SDK (R12)

Ядро подключается только как версионированный модуль `CLIProxyAPI/v7`. Все
вызовы бизнес-слоя проходят через публичные `sdk/*` контракты ADR-9; код не
импортирует `internal/*` ядра и не зависит от неэкспортируемого состояния.
Обновление patch/minor проверяется сборкой, `internal/sdkcontract`, contract-
тестами и интеграциями.
Переход на новый major — отдельное архитектурное изменение с ADR и
миграционным планом.

## 2. Запуск приложения (cmd/cliproxy/main.go)

Последовательность wiring'а:

```mermaid
sequenceDiagram
    participant M as main
    participant C as config
    participant DB as pgx pool
    participant S as security
    participant Store as store
    participant Reg as sdkAuth
    participant CoreMgr as coreauth.Manager
    participant B as cliproxy.Builder
    participant Svc as Service.Run

    M->>C: LoadConfig config.yaml + env
    M->>DB: pgxpool.New dsn
    M->>S: NewAES env CLIPROXY_ENCRYPTION_KEY
    M->>Store: New db aes
    M->>Reg: RegisterTokenStore store
    M->>CoreMgr: NewManager store selector hook
    M->>B: Builder WithConfig WithCoreAuthManager WithWatcherFactory WithServerOptions
    Note over B: Builder собирает Service с DI контрактов
    M->>Svc: Run ctx блокирует
```

**Ключевые шаги wiring'а** (по ADR-9):

1. **Конфиг** — `config.yaml` (ConfigMap) + env-секреты (R6).
2. **DB pool** — `pgxpool.New`.
3. **Security** — AES-ключ из env, bcrypt-cost (константа).
4. **Store** — `internal/store` реализует `coreauth.Store` с transparent-шифрованием credentials (AES-GCM). Глобальная регистрация через `sdkAuth.RegisterTokenStore` **до** Builder'а.
5. **Selector** — `internal/auth/selector`: выбор аккаунта по allow-list моделей (R10 не меняет выбранный auth).
6. **Hook** — `internal/usage`: `usage.Plugin.HandleUsage` и `coreauth.Hook`
   со счётчиками lifecycle credentials и результатов upstream-вызовов.
7. **access.Provider** — `internal/access`: проверка API-key + `users.status`.
8. **WatcherFactory** — `internal/watcher`: public no-op для file-backed SDK;
   DB revision poller делает controlled restart, advisory leader запускает cleanup sessions.
9. **ModelRegistryHook** — `internal/modelregistry`.
10. **ServerOptions** — `api.WithMiddleware` (session-cookie auth для management-API, logging, CORS), `api.WithRouterConfigurator` (management-роуты `/api/v1/*`).
11. **Builder.Build() → Service.Run(ctx)** — ядро само: грузит auths, поднимает Gin, запускает `StartAutoRefresh(15m)`, регистрирует model-refresh callback.

## 3. Поток: inference-запрос клиента

Основной use-case: клиент дёргает `/v1/chat/completions` с API-key.

```mermaid
sequenceDiagram
    participant App as Клиент
    participant Gin as Gin ядро
    participant AP as access.Provider
    participant Sel as Selector
    participant Exec as ProviderExecutor
    participant Up as Upstream
    participant USG as usage.Plugin
    participant DB as Postgres

    App->>Gin: POST /v1/chat/completions + Bearer api-key
    Gin->>AP: Authenticate req
    AP->>DB: lookup api_key by prefix cache TTL 5-15s
    AP->>AP: bcrypt verify hash vs key
    AP->>DB: check users.status = active cache
    AP-->>Gin: Result principal user_id api_key_id в ctx
    Note over Gin: Record.APIKey из versioned Principal
    Gin->>Sel: Pick provider model opts auths
    Sel->>DB: load model_overrides cache
    Sel-->>Gin: выбранный Auth
    Gin->>Exec: Execute ctx auth req opts
    Exec->>Up: HTTP через HTTP_PROXY/HTTPS_PROXY/NO_PROXY процесса
    Up-->>Exec: response или stream chunks
    Exec-->>Gin: Response StreamResult
    Gin-->>App: response stream
    Gin->>USG: HandleUsage Record principal уже внутри
    USG->>DB: INSERT usage_events async partitioned
```

**Ключевые точки:**
- **access.Provider** (R2) — единственная проверка API-key; результат в ctx.
- **Principal копируется в Record** в начале (R3), т.к. `HandleUsage` может вызваться после отмены context (стриминг).
- **System proxy** (R10) задается окружением процесса; бизнес-слой не меняет `Auth.ProxyURL`.
- **usage.Plugin** пишет асинхронно в `usage_events`.

## 4. Поток: login через identity source (R1)

В `auth.mode=ldap` используется следующий поток LDAP. В
`auth.mode=static` HTTP login вызывает static provider, который сравнивает
credentials из env, возвращает identity с role из конфигурации и internal
username `static:<username>`; LDAP-сеть при этом не используется. После
получения identity provisioning, проверка `users.status` и выпуск session
одинаковы для обоих режимов.

```mermaid
sequenceDiagram
    participant U as Пользователь
    participant H as httpapi login
    participant L as LDAP provider
    participant DB as Postgres

    U->>H: POST /api/v1/login username password
    H->>L: service bind + search username
    H->>L: user bind password аутентификация
    H->>L: search memberOf user-group admin-group
    alt не в user-group AND не в admin-group
        H-->>U: 403 forbidden
    else в admin-group или обеих
        H->>H: role = admin TTL = 10h
    else в user-group
        H->>H: role = user TTL = 5m
    end
    H->>DB: upsert users provisioning status check
    alt users.status = blocked
        H-->>U: 403 blocked
    end
    H->>DB: INSERT sessions token_hash role expires_at
    H-->>U: 200 + Set-Cookie HttpOnly Secure SameSite
```

**Решения:**
- Логика групп (R1): admin → admin; иначе user → user; иначе отказ.
- `users.status` проверяется после identity provider (R9.A.3).
- TTL фикс.: user=5мин, admin=10ч.
- Cookie: HttpOnly, SameSite=Lax; `Secure=true` в production, а в
  development/test допускается `false` для локального HTTP static identity.
  Домен и Path финализируются при deployment-дизайне.
- Static mode разрешён только в development/test и не является fallback для LDAP.
- Переключение `auth.mode` требует остановки всех dev/test реплик; mixed-mode
  rolling deployment запрещён.

## 5. Поток: auto-refresh (R7, ADR-9/ADR-10)

```mermaid
sequenceDiagram
    participant Core as coreauth.Manager
    participant Exec as ProviderExecutor
    participant Up as Upstream
    participant Store as store
    participant DB as Postgres

    Note over Core: StartAutoRefresh 15m min-heap по NextRefreshAfter до 16 воркеров
    Core->>Core: pop auth с истекшим NextRefreshAfter
    Core->>Exec: Refresh ctx auth
    Exec->>Up: OAuth refresh через system proxy процесса
    Up-->>Exec: new tokens
    Exec-->>Core: обновлённый Auth
    Core->>Store: Save auth
    Store->>Store: очистить legacy Auth.ProxyURL
    Store->>DB: UPDATE upstream_accounts credentials_enc last_refreshed_at
```

**Важно (ADR-10):** auto-refresh использует тот же system proxy процесса, что
и все остальные HTTP-вызовы; `Auth.ProxyURL` при Load/Save очищается.

## 6. Поток: management — настройка OAuth (R9.A.1)

Своя асинхронная реализация (НЕ через блокирующий `sdkAuth.Manager.Login`).
Сессии в Postgres → multi-replica. См. детальный дизайн
[docs/design/r9-oauth-and-testing.md](design/r9-oauth-and-testing.md).

```mermaid
sequenceDiagram
    participant A as Админ
    participant H as httpapi mgmt
    participant FM as auth.oauth FlowManager
    participant DB as Postgres
    participant Svc as claude.NewClaudeAuth
    participant Store as store

    A->>H: POST admin oauth provider start
    H->>FM: Start provider
    FM->>Svc: GeneratePKCE + GenerateAuthURL
    Svc-->>FM: auth_url + state + verifier
    FM->>DB: INSERT oauth_sessions state pending verifier
    FM-->>H: auth_url state
    H-->>A: 200 auth_url state
    Note over A: админ открывает auth_url в браузере вне системы
    Note over A: провайдер редиректит на localhost CallbackPort
    Note over A: админ копирует redirect_url вручную
    A->>H: POST admin oauth provider complete state redirect_url
    H->>FM: Complete provider state redirect_url
    FM->>DB: SELECT oauth_sessions WHERE state
    FM->>Svc: ExchangeCodeForTokens code verifier
    Svc-->>FM: tokens access refresh email
    FM->>Store: Save auth AES-GCM шифрование
    FM->>DB: UPDATE oauth_sessions completed auth_id
    FM->>DB: INSERT admin_audit_log action provider_oauth_setup
    FM-->>H: account_id
    H-->>A: 200 account_id status valid
```

## 7. Компоненты — детали

### `internal/access` — access.Provider (R2)
Реализует `access.Provider`:
- `Authenticate(ctx, *http.Request) (*Result, *AuthError)`:
  1. извлечь Bearer-token из Authorization;
  2. lookup `api_keys` по `key_prefix` (in-process cache TTL 5–15с);
  3. bcrypt-verify против `key_hash`;
  4. проверить `users.status = active` (cache);
  5. проверить `users.identity_source` против активного `auth.mode`;
  6. вернуть `Result{Provider="db-apikey", Principal=<user_id>, Metadata={api_key_id, user_id, role}}`.
- Регистрируется через `access.RegisterProvider("db-apikey", provider)`, затем
  **`access.SetExclusiveProvider("db-apikey")`** — отключает встроенный
  `config-api-key` ядра (inline `cfg.APIKeys` не используются, исключает
  двойной путь auth). Manager передаётся в Builder через `WithRequestAccessManager`.
- **Прокидывание api_key_id в аналитику (R3):** access.Provider кодирует
  `user_id` и `api_key_id` в versioned principal. SDK переносит principal в
  `usage.Record.APIKey`; `usage.Plugin` декодирует его без чтения request
  context, который к концу streaming-запроса уже может быть отменён.

### `internal/auth/selector` — coreauth.Selector (ADR-9)
Реализует `coreauth.Selector`:
- `Pick(ctx, provider, model, opts, auths) (*Auth, error)`:
  1. загрузить `model_overrides` через fail-closed TTL-кэш (5с);
  2. применить allow-list и provider из enabled override;
  3. отфильтровать `auths` по provider и делегировать fill-first выбор ядру;
  4. вернуть выбранный `*Auth`.

`Selector.Pick` получает model только значением и не имеет публичного механизма
переписать downstream request. Поэтому `upstream_model` сохраняется как desired
mapping, но runtime rewrite блокирован до появления публичного SDK hook; обход
через `internal/*` запрещён R12.

### `internal/auth/identity` (R1)
Внутренний `IdentityProvider` изолирует проверку username/password от HTTP и
возвращает `{Username, Email, Role, Source}`. Wiring выбирает реализацию один
раз по `auth.mode`:
- `ldap.Provider` — production source;
- `static.Provider` — только development/test, credentials из env, username
  нормализуется в `static:<username>`.

### `internal/auth/ldap` (R1)
- `Authenticate(ctx, username, password) (Identity, error)`:
  1. service-bind + search → user DN;
  2. user-bind → аутентификация;
  3. search memberOf → проверка групп (config-defined DN);
  4. роль: admin если в admin-group, иначе user если в user-group, иначе 403;
  5. отклонить LDAP username с зарезервированным префиксом `static:`;
  6. вернуть identity с `Source=ldap`.
- Service-account пароль — **только из env** `LDAP_BIND_PASSWORD` (k8s Secret).
  Не хранится в БД → AES-шифрование не применяется (см. исправление R5).

### `internal/auth/oauth` — OAuth login-flow (R9.A.1)
Реализует асинхронный OAuth-flow (свой, не через блокирующий `sdkAuth.Manager.Login`):
- `FlowManager` — оркестратор flow; per-provider реализации `ProviderFlow`
  поверх низкоуровневых сервисов ядра (`claude.NewClaudeAuth`, `codex.NewCodexAuth`,
  `kimi.NewKimiAuth`, `xaiauth.NewXAIAuth`, `antigravity.NewAntigravityAuth`).
- **Callback-flow** (Codex PKCE, Claude, Antigravity): `Start` → PKCE+state →
  `oauth_sessions` (Postgres) → auth_url; `Complete` → обмен code → `Store.Save`.
- **Device-flow** (Kimi, xAI, опц. Codex-device): `Start` → device_code →
  `oauth_sessions`; goroutine poll провайдера; клиент poll-ит статус.
- **Multi-replica:** сессии в Postgres → любая реплика может завершить flow.
- Хелперы из `sdk/api/management.go` (`ValidateOAuthState`, `NormalizeOAuthProvider`).
- См. детальный дизайн [docs/design/r9-oauth-and-testing.md](design/r9-oauth-and-testing.md).

### `internal/auth/testing` — тестирование валидности (R9.A.5)
Реализует health-check upstream-аккаунта **без траты inference-квоты**:
- `Checker.Test(ctx, accountID)`:
  - **OAuth** (Codex/Claude/Antigravity) → `executor.Refresh(ctx, auth)` (обмен
    refresh_token, не тратит квоту; для Antigravity бонусом обновляет
    `AntigravityCreditsHint`);
  - **API-key** → `executor.HttpRequest(ctx, auth, GET /models)` (metadata-endpoint,
    HTTP 200 = валиден).
- Не использует `Execute`/`CountTokens` (тратят квоту).
- Ответ: `{valid, method: "refresh"|"http_probe", details, quota}`.
- См. [docs/design/r9-oauth-and-testing.md](design/r9-oauth-and-testing.md).

### `internal/usage` — usage.Plugin (R3, ADR-9)
Реализует `usage.Plugin.HandleUsage(ctx, record Record)`:
- **Источник principal:** access.Provider кодирует `user_id` и `api_key_id` в
  versioned principal. SDK переносит его в `record.APIKey`, откуда
  `HandleUsage` декодирует оба идентификатора.
- ⚠️ **Стриминг (R3):** `HandleUsage` вызывается асинхронно в конце потока,
  когда request-context уже отменён. Поэтому `user_id` и `api_key_id`
  кодируются в versioned `record.APIKey`, а не читаются из context в момент
  `HandleUsage`.
- bounded async-запись в `usage_events`: очередь 1024 событий, batch до 100
  событий или 250мс, `pgx.Batch`; при остановке очередь flush'ится с timeout.
  При переполнении отбрасывается только telemetry и пишется warning;
- после успешного batch обновление уникальных `api_keys.last_used_at` не чаще
  одного раза в минуту.
- `coreauth.Hook` подключён к `coreauth.Manager` и хранит потокобезопасные
  счётчики регистраций/изменений credentials и результатов. Он не сохраняет
  payload или credentials и будет экспортирован в Prometheus на Ф6.

### `internal/store` — coreauth.Store + репозитории (R5, ADR-9)
Реализует `coreauth.Store` (List/Save/Delete) поверх `upstream_accounts`:
- **Save:** очищает legacy `auth.ProxyURL`, шифрует credentials AES-GCM; UPDATE/INSERT.
- **Load/List:** расшифровывает blob → восстанавливает `*coreauth.Auth` с пустым `ProxyURL`.
Также: репозитории для `users`, `api_keys`, `sessions`, `usage_events`
(включая `pgx.Batch`), `admin_audit_log`, `model_overrides` (sqlc-генерация).

### `internal/security` (R5)
Два класса:
- `HashPassword/CheckPassword` — bcrypt cost 12 (API-keys).
- `Encrypt/Decrypt` — AES-256-GCM с key-version prefix — **только для upstream-
  credentials в БД** (R5 исправление). LDAP bind-password живёт в env, не шифруется AES.

### `internal/watcher` — WatcherFactory (ADR-9, R7)
`NoopFactory` отключает file-backed watcher SDK, потому что credentials
хранятся в Postgres. `RevisionPoller` наблюдает `runtime_revisions` и делает
controlled restart после `Store.Save/Delete`; это загружает актуальные auth при
следующем старте. `LeaderRunner` удерживает отдельный Postgres advisory lock;
лидер запускает `SessionCleanup` раз в минуту. Публичная оболочка SDK ссылается
на `watcher.AuthUpdate` из upstream `internal/*`, поэтому прямой DB-push в
очередь ядра запрещён R12.

### `internal/modelregistry` — ModelRegistryHook (ADR-9)
Реализует `cliproxy.ModelRegistryHook`:
- подписка на изменения in-memory реестра ядра → атомарный JSON snapshot в
  `model_registry_snapshots` по `(provider, client_id)` для UI/model-mapping.
  Поля модели не дублируются в локальной схеме, чтобы обновление публичного SDK
  не требовало миграции бизнес-слоя; источник истины остаётся в ядре.

### `internal/httpapi` (R8, R9, R11)
- **Прокси-эндпоинты** (`/v1/*`) — роутит ядро (Gin); бизнес-слой не пишет хендлеры.
- **Management-API** (`/api/v1/*`) — через `api.WithRouterConfigurator`:
  - `/api/v1/login`, `/api/v1/logout` (R1);
  - `/api/v1/me/keys` (CRUD user API-keys, R9.U.2);
  - `/api/v1/me/usage` (личная статистика, R9.U.3);
  - `/api/v1/admin/users`, `/api/v1/admin/keys`, `/api/v1/admin/oauth/*`, `/api/v1/admin/providers/keys`, `/api/v1/admin/quota/*`, `/api/v1/admin/accounts/test`, `/api/v1/admin/models`, `/api/v1/admin/oauth/export|import` (R9.A).
- **Системные роуты:** отдельный SDK configurator регистрирует `/healthz`
  (liveness без БД) и `/readyz` (readiness = PostgreSQL `Ping` с timeout,
  без текста ошибки). `/openapi.json` отдаёт JSON, встроенный в бинарник и
  сгенерированный из `openapi.yaml`; CI проверяет отсутствие drift. `/metrics`
  (Prometheus) и `/docs` (Swagger UI / Redoc, R11) остаются задачами Ф6.
- **OpenAPI (R11):** spec-first — `openapi.yaml` первичен; Go-типы и хендлеры
  генерируются из спецификации. Покрытие: все роуты (management с полными
  схемами; прокси `/v1/*` без body-схем, только auth + общие errors).
- **Метрики (R6):** `internal/metrics` держит private Prometheus registry;
  `/metrics` отдаёт HTTP count/latency с labels method/route/status, snapshot
  upstream-hook, usage queue depth, candidate cache API-keys (`hit|miss`) и pgx
  pool stats. Refresh-метрики подключаются только в соответствующих business hooks.
- **Логи (R6):** глобальный JSON `slog` handler из `internal/observability`
  рекурсивно заменяет значения attrs с password/secret/token/credential/
  authorization/API-key на `[REDACTED]`, включая `WithAttrs` и slog groups.
  HTTP middleware пишет access log только с method, route template, status,
  duration, request ID и user ID; headers, query и body не читаются.
- **Middleware:** session-cookie auth для management; OpenTelemetry server span
  с propagation context в SDK; безопасный access logging; CORS; role-guard
  (user/admin).

### `internal/cache` (R6)
In-process кэш за интерфейсом:
- `session_lookup` (TTL 10с): token_hash → user_id, role; admin block и logout
  удаляют local entries, остальные реплики сходятся не позднее TTL;
- `api_key_lookup`: key_prefix → key_hash, user_id, status;
- `model_overrides`: полный набор (invalidation при admin-change).
Задел под Redis (ADR-8).

### `internal/config` (R6)
Парсинг `config.yaml` (структурированный: ldap, server, db, logging) + env-override (12-factor). System proxy задается `HTTP_PROXY`, `HTTPS_PROXY`, `NO_PROXY`; секреты — только env.

## 8. Deployment (k8s)

`HTTP_PROXY`, `HTTPS_PROXY` и `NO_PROXY` передаются как env контейнера; они
не входят в `config.yaml` и не логируются приложением.

```mermaid
graph LR
    subgraph Ingress["Ingress"]
        ALB["Ingress / ALB"]
    end
    subgraph Deploy["Deployment: N реплик"]
        P1["Pod cliproxy"]
        P2["Pod cliproxy"]
        P3["Pod cliproxy"]
    end
    subgraph Config["Config"]
        CM["ConfigMap<br/>config.yaml"]
        SEC["k8s Secret<br/>ENCRYPTION_KEY<br/>DB_PASSWORD<br/>LDAP_BIND_PASSWORD"]
    end
    subgraph Data["Data"]
        PG[("Postgres<br/>StatefulSet / managed")]
    end

    ALB --> P1
    ALB --> P2
    ALB --> P3
    CM --> P1
    CM --> P2
    CM --> P3
    SEC --> P1
    SEC --> P2
    SEC --> P3
    P1 --> PG
    P2 --> PG
    P3 --> PG
```

**Параметры deployment:**
- **Replicas:** ≥ 2 (HPA по CPU/RPS).
- **Stateless** — любой pod взаимозаменяем.
- **Config:** ConfigMap (config.yaml) + Secret (env).
- **DB:** Postgres (StatefulSet или managed: CloudNativePostgres / RDS / AlloyDB). Backup/PITR — вне репо (R6.4).
- **Probes:**
  - liveness: `GET /healthz` (процесс жив);
  - readiness: `GET /readyz` (DB ping успешен);
- **Graceful shutdown:** `Service.Shutdown(ctx)` + drain in-flight requests.

### Leader election в deployment
- **Advisory lock** в Postgres — ровно один pod держит lock → становится лидером для watcher/audit-rollup.
- При падении лидера → lock освобождается → следующий pod забирает (нет split-brain, т.к. PG-транзакция).

### Observability
- **Metrics** `/metrics` (Prometheus): request_count, request_latency, upstream_refresh_success/failure, cache_hit/miss, db_pool stats.
- **Tracing** (OpenTelemetry): span на inference-запрос, access.Provider, Selector, Execute.
- **Logs** (`slog` JSON): structured, с request_id, user_id, provider, model.

## 9. Cross-cutting — конфигурация (R6)

`config.yaml` (пример, финализируется в дизайне):
```yaml
server:
  addr: ":8080"
  environment: "production" # development | test | production
auth:
  mode: "ldap"              # ldap | static; static только development/test
ldap:
  url: "ldaps://ldap.corp.example"
  bind_dn: "CN=svc-cliproxy,OU=Service,DC=corp,DC=example"
  user_base: "OU=Users,DC=corp,DC=example"
  user_filter: "(sAMAccountName={username})"
  user_group_dn: "CN=cliproxy-users,OU=Groups,..."
  admin_group_dn: "CN=cliproxy-admins,OU=Groups,..."
db:
  dsn: "postgres://cliproxy@pg:5432/cliproxy"  # password из env DB_PASSWORD
logging:
  level: "info"
  format: "json"
encryption:
  key_version: 1  # активный, сам ключ — env CLIPROXY_ENCRYPTION_KEY
```

**Env-секреты** (никогда в config.yaml):
- `CLIPROXY_ENCRYPTION_KEY` (base64 32 байта)
- `CLIPROXY_ENCRYPTION_PREVIOUS_KEYS` (опциональная JSON-карта
  `key-version → base64`, для ротации)
- `DB_PASSWORD`
- `LDAP_BIND_PASSWORD`
- `CLIPROXY_STATIC_USER_USERNAME` (только `auth.mode=static`)
- `CLIPROXY_STATIC_USER_PASSWORD` (только `auth.mode=static`)
- `CLIPROXY_STATIC_USER_ROLE` (только `auth.mode=static`, `user` или `admin`)

## 10. Открытые вопросы (для след. итерации дизайна)

- Формат `api_keys.scope` (jsonb) — в R9 API-дизайне.
- TTL ретенции `usage_events` (R3) — в дизайне retention-job.
- Cookie атрибуты (SameSite по домену) — финализировать по deployment-домену.
- Партиционирование `admin_audit_log` — при росте.
