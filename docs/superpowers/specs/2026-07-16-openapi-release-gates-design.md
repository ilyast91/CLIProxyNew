# Полный proxy OpenAPI и автоматизированные release gates

> **Дата:** 2026-07-16
> **Статус:** дизайн согласован; документ ожидает review перед планом реализации
> **SDK baseline:** `github.com/router-for-me/CLIProxyAPI/v7` v7.2.80

## 1. Цель

Закрыть два оставшихся автоматизируемых блока текущей итерации:

1. привести proxy-раздел `openapi.yaml` к полному HTTP surface SDK v7.2.80;
2. добавить release-hardening gates: E2E, integration CI, behavioral contracts
   ADR-9, aggregate coverage не ниже 70%, security audit и полный race suite.

После изменения документация фаз должна отражать фактическое состояние
репозитория, а не исторические незакрытые checkboxes.

## 2. Scope и ограничения

### В scope

- Все inference/proxy routes, которые SDK v7.2.80 регистрирует в группах
  `/v1`, `/openai/v1`, `/backend-api/codex` и `/v1beta`.
- Bearer API-key security, path parameters, краткие summary/description и общие
  ошибки для каждого proxy operation.
- Contract test, сравнивающий ожидаемую route/method matrix со встроенным
  OpenAPI-документом.
- Настоящий E2E через SDK runtime и PostgreSQL testcontainer.
- Отдельные CI gates для behavioral contracts, integration, E2E, coverage,
  security и полного `go test -race`.
- Актуализация `docs/implementation-phases.md` после успешной проверки.

### Не в scope

- Копирование request/response schemas upstream API: ими владеет SDK.
- Provider-specific transport, refresh, streaming или parsing в `internal/`.
- OAuth provider flows и mock OAuth provider: OAuth implementation исключён
  из текущего scope до появления подходящего публичного SDK-контракта.
- Изменение схемы, миграций или scheduling для `usage_events`. Существующая
  схема остаётся без изменений; автоматический partition scheduler больше не
  считается будущей задачей.
- Load/performance gates, chaos tests и operational runbooks. Они остаются
  отдельной поздней фазой и не блокируют этот инкремент.
- `/docs` Swagger UI / Redoc: endpoint остаётся опциональным.
- SDK-blocked refresh metrics, Execute span и runtime model rewrite.

## 3. OpenAPI route parity

### 3.1 Источник истины

Route matrix фиксируется по публично используемому runtime SDK v7.2.80 и его
регистрации маршрутов. `openapi.yaml` остаётся первичным контрактом
CLIProxyNew, но proxy payload schemas намеренно не определяются по R11: сервис
не должен создавать локальную копию upstream API-моделей.

### 3.2 Полная матрица

| Method | Path | Назначение |
|---|---|---|
| GET | `/v1/models` | OpenAI-compatible model list |
| POST | `/v1/chat/completions` | Chat Completions |
| POST | `/v1/completions` | Legacy Completions |
| POST | `/v1/images/generations` | Image generation |
| POST | `/v1/images/edits` | Image edits |
| POST | `/v1/videos` | xAI-compatible video generation alias |
| POST | `/v1/videos/generations` | xAI video generation |
| POST | `/v1/videos/edits` | xAI video edits |
| POST | `/v1/videos/extensions` | xAI video extensions |
| GET | `/v1/videos/{request_id}` | xAI video status/result |
| POST | `/v1/messages` | Anthropic Messages |
| POST | `/v1/messages/count_tokens` | Anthropic token count |
| GET | `/v1/responses` | Responses websocket handshake |
| POST | `/v1/responses` | Responses API |
| POST | `/v1/responses/compact` | Responses compaction |
| POST | `/v1/alpha/search` | Codex alpha search |
| POST | `/openai/v1/videos` | OpenAI video creation |
| GET | `/openai/v1/videos/{video_id}/content` | OpenAI video content |
| GET | `/openai/v1/videos/{video_id}` | OpenAI video status/result |
| GET | `/backend-api/codex/responses` | Direct Codex responses websocket |
| POST | `/backend-api/codex/responses` | Direct Codex Responses API |
| POST | `/backend-api/codex/responses/compact` | Direct Codex compaction |
| POST | `/backend-api/codex/alpha/search` | Direct Codex alpha search |
| GET | `/v1beta/models` | Gemini model list |
| POST | `/v1beta/interactions` | Gemini interactions |
| GET | `/v1beta/models/{model}:{action}` | Gemini model action via GET |
| POST | `/v1beta/models/{model}:{action}` | Gemini model action via POST |

Gin wildcard `/v1beta/models/*action` проецируется в OpenAPI как явные
`{model}` и `{action}`. Это покрывает поддерживаемые Gemini URL вида
`models/<model>:<action>` и делает оба параметра видимыми клиентам. Текущий
ошибочный `/v1/models/{model}:generateContent` удаляется.

### 3.3 Описание operations

Каждая operation получает:

- уникальный стабильный `operationId`;
- tag `Proxy`;
- `BearerApiKey` security;
- path parameters для `request_id`, `video_id`, `model`, `action`;
- успешный response без локальной payload schema;
- общие `400`, `401` и `500`, а для retrieval routes также `404`;
- явное описание websocket semantics для GET `/responses` routes.

После изменения штатно выполняется `go generate ./internal/openapi/...`.
Файлы `internal/openapi/openapi.json`, compatibility projection и ogen output
не редактируются вручную.

### 3.4 Route matrix contract test

`internal/openapi/document_test.go` получает табличный тест с полным набором
27 method/path pairs. Тест парсит встроенный JSON, нормализует paths и требует:

- отсутствие пропущенных expected operations;
- отсутствие старого `/v1/models/{model}:generateContent`;
- Bearer security на каждой proxy operation;
- наличие обязательных path parameters.

Тест намеренно не сравнивает proxy body schemas: их отсутствие является частью
архитектурного контракта.

## 4. E2E через настоящий SDK runtime

### 4.1 Размещение и изоляция

E2E размещается в отдельном test-only пакете `internal/e2e`. Он не содержит
production-кода и не попадает в coverage denominator. Тест не запускается
параллельно, потому что SDK использует process-global registries для access и
usage plugins. Cleanup обязан восстановить global access registration,
остановить Service и закрыть buffered usage plugin.

PostgreSQL поднимается через тот же pinned testcontainers image, что и store
integration tests. E2E локально применяет реальные миграции и использует
настоящие pgx repositories, AES keyring и bcrypt hashing.

### 4.2 Runtime composition

Тест собирает production-like graph без вызова `main`:

- `server.environment=development`, `auth.mode=static`;
- static identity с ролью admin;
- реальные `UserRepository`, `SessionRepository`, `APIKeyRepository`,
  `UsageEventRepository` и `CoreAuthStore`;
- реальный `access.Provider`, `Selector`, `usage.Hook` и buffered plugin;
- публичный `coreauth.Manager`;
- fake `ProviderExecutor`, зарегистрированный только через публичный SDK API;
- `cliproxy.Builder`, management configurator и `Service.Run` на loopback port;
- временный config path, потому что это обязательный Builder input.

Fake executor является только тестовой границей upstream. Он возвращает
валидный OpenAI-compatible JSON с usage tokens и не реализует бизнес-логику
провайдера. Auth для него сохраняется реальным `coreauth.Store` и загружается
менеджером SDK.

### 4.3 Проверяемый сценарий

Один последовательный сценарий доказывает сквозной data flow:

1. `POST /api/v1/login` со static admin credentials возвращает session cookie.
2. `POST /api/v1/me/keys` с cookie создаёт клиентский API-key; plaintext
   доступен только в этом ответе, в БД хранится bcrypt hash.
3. `POST /v1/chat/completions` с новым Bearer key проходит реальный
   `access.Provider`, выбирает сохранённый auth и вызывает fake executor.
4. Buffered usage plugin flush'ится; событие с `user_id`, `api_key_id`, model и
   token counters появляется в PostgreSQL.
5. `GET /api/v1/me/usage` с cookie возвращает агрегированное использование.
6. Admin endpoint (`GET /api/v1/admin/users` и/или `/admin/keys`) видит
   созданные сущности и подтверждает role guard.

Readiness ожидания ограничены deadline; тест не использует безграничные sleeps.
Shutdown выполняется через публичный `Service.Shutdown` и проверяет отсутствие
ошибки.

## 5. Behavioral contract suite ADR-9

Compile gate `internal/sdkcontract` сохраняется, но дополняется behavioral
matrix. Проверяется именно поведение реализаций бизнес-слоя:

| Контракт | Обязательное поведение |
|---|---|
| `coreauth.Store` | encrypted save/load/delete, SDK-managed ID, пустой `ProxyURL` |
| `coreauth.Selector` | allow-list/provider selection, disabled reject, fail-closed cache |
| `coreauth.Hook` + `usage.Plugin` | lifecycle/result counters и сохранение versioned principal без context dependency |
| `access.Provider` | active user/source accept; blocked, wrong source и bad key reject |
| `WatcherFactory` | usable no-op wrapper без upstream internal imports; revision change вызывает controlled restart |
| `ModelRegistryHook` | full snapshot replace и unregister delete |
| Middleware/routes | configurator регистрирует management/system routes, session и role guards применяются корректно |

Существующие focused tests переиспользуются. Недостающие cases добавляются в
соответствующие packages, а CI выполняет отдельный behavioral-contract command,
чтобы contract gate был видимым и не растворялся в общем unit suite. Store и
revision behavior, требующие PostgreSQL, дополнительно подтверждаются
integration job.

## 6. Coverage gate

Coverage считается агрегированно по handwritten Go packages внутри
`internal/*`. Порог фиксирован: **70.0%**, без возможности понизить его через
environment variable или CI input.

Из denominator исключаются только generated packages:

- `internal/openapi/ogen`;
- `internal/store/dbgen`.

Также не добавляется test-only `internal/e2e`, в котором нет production
statements. Остальные packages, включая `store` и `watcher`, остаются в
denominator. Coverage job запускает integration tests с Docker, создаёт
`coverage.out`, вычисляет total через `go tool cover -func` и падает при
результате ниже 70.0%.

Если baseline ниже порога, добавляются targeted tests к наименее покрытым
handwritten packages. Generated code не маскируется ручными правками, а порог
не снижается.

## 7. Security gate

Security job состоит из двух независимых проверок:

1. `govulncheck ./...` с зафиксированной версией инструмента проверяет известные
   уязвимости достижимого Go dependency graph.
2. Репозиторный audit script проверяет tracked source/config/test fixtures на:
   private-key и credential markers; запрещённые `fmt.Print*`/`log.Print*` в
   business runtime; очевидное логирование password/secret/token/API-key или
   credential payload; случайно добавленные `.env`, key/pem и binary artifacts.

Существующие runtime tests redaction, bcrypt-only API-key storage и AES-GCM
credential storage остаются частью unit/integration suites. Audit patterns и
разрешённые test fixtures документируются рядом со скриптом; исключения должны
быть точечными, чтобы gate нельзя было обойти глобальным ignore.

## 8. CI topology

После `static-checks` независимо запускаются:

- `unit-contract`: short unit suite, SDK compile gate и behavioral ADR-9 gate;
- `openapi-lint`: Spectral, generation drift и route matrix contract;
- `integration`: все PostgreSQL integration tests без `-short` фильтрации;
- `e2e`: отдельный SDK runtime scenario;
- `coverage`: aggregate handwritten internal coverage ≥70%;
- `security`: `govulncheck` и repository audit;
- `full-race`: `go test -race -timeout 15m ./...`, включая testcontainers.

`build` зависит от всех перечисленных gates. Таким образом build/package не
может быть зелёным при пропущенных integration, security, coverage или race
проверках. Jobs используют Go 1.26.5 и текущие pinned tool/action versions.

Full-race и coverage сознательно повторяют часть integration workload: это
отдельные release guarantees, а не оптимизация времени CI.

## 9. Актуализация фаз

После зелёной реализации `docs/implementation-phases.md` обновляется так:

- Ф1: sqlc и repositories `admin_audit_log`/`oauth_sessions` отмечаются как
  выполненные; partition scheduler удаляется из будущих задач без изменения
  текущей схемы БД.
- Ф2: существующие LDAP/access/session unit tests отмечаются выполненными.
- Ф4: полный proxy surface и route parity test отмечаются выполненными;
  OAuth provider flows явно переносятся за пределы текущего scope, а не
  остаются ложным release blocker.
- Ф7: E2E, behavioral contracts, PostgreSQL integration CI, static source
  regression, coverage, security и full race отмечаются только после
  соответствующей проверки.
- Load/SLA, chaos, operational runbooks и optional docs остаются открытыми в
  поздней фазе.
- Acceptance Ф7 разделяется на «automated hardening complete» и оставшиеся
  performance/operations criteria, чтобы документ не объявлял v1 release-ready
  до load/chaos/runbook работ.

## 10. Порядок реализации и критерии готовности

Работа выполняется test-first:

1. падающий OpenAPI route matrix test;
2. route additions и штатная генерация;
3. behavioral contract gaps;
4. E2E test и минимальная test harness;
5. coverage/security scripts и targeted tests до 70%;
6. CI topology;
7. полная локальная verification;
8. только затем обновление phase checkboxes.

Инкремент готов, когда одновременно выполнены:

- все 27 proxy operations присутствуют в embedded OpenAPI и проходят lint;
- E2E доказывает login → key → inference → analytics → admin;
- behavioral matrix покрывает все семь ролей ADR-9;
- aggregate handwritten `internal/*` coverage не ниже 70.0%;
- security audit и `govulncheck` зелёные;
- полный race suite, integration tests, `go vet`, `go build` и generation drift
  checks зелёные;
- `usage_events` schema/scheduler и OAuth provider implementation не изменены;
- фазы отражают только подтверждённый результат.
