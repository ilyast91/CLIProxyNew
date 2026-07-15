# Требования к CLIProxyNew

> **Статус:** Требования зафиксированы (10 ADR закрыты). Открытые пункты ❓ —
> имплементационные детали, разрешаемые в дизайне компонентов.
> **Дата начала:** 2026-07-11
> Пункты, помеченные ❓ **Открыто:** — требуют решения.

## Контекст

`CLIProxyNew` — бизнес-обвязка над upstream relay-движком. По архитектурной
модели повторяет связку
[`router-for-me/CLIProxyAPI`](https://github.com/router-for-me/CLIProxyAPI)
(ядро/SDK) +
[`router-for-me/CLIProxyAPIBusiness`](https://github.com/router-for-me/CLIProxyAPIBusiness)
(бизнес-слой):
- **Ядро (upstream relay engine)** подключается как **обычная Go-зависимость**
  в `go.mod` (как `github.com/router-for-me/CLIProxyAPI/v6` в референсе).
  Ядро отвечает за: вызовы провайдеров (Codex, Claude Code, Gemini CLI и др.),
  transport, streaming, парсинг ответов, плагины, обновление upstream-токенов и
  реестр моделей. **Мы его не форкаем и не пишем** — это внешняя зависимость.
- **`CLIProxyNew` (этот репозиторий)** — бизнес-слой: auth, аналитика, БД,
  API-слой для клиентов, watcher-оркестрация, observability,
  multi-replica в k8s.

> ⚠️ **Принципиальная правка от 2026-07-11:** ядро/upstream-SDK —
> **внешняя go-зависимость**, а не компонент, который мы разрабатываем в этом
> репозитории. Вся ответственность CLIProxyNew — бизнес-обвязка поверх него.

### Глоссарий (разграничение терминов)

В документе встречаются два разных смысла слова **«квота»** — не путать:
- **Upstream-квота подписки** (R9.A.4) — лимиты
  upstream-аккаунта провайдера (Codex/Claude/...: остаток запросов/токенов,
  срок подписки). **Просматривается** администратором, не ограничивает
  пользователей.
- **Пользовательская квота/rate-limit** (роль репо, отложено) — лимиты на
  пользователя/модель/период внутри нашего сервиса. **На первой версии не
  реализуется.**

---

## Роль репозитория

Что мы пишем (бизнес-слой):
- HTTP API для клиентов (OpenAI/Gemini/Claude/Codex/Grok-совместимые
  эндпоинты — R8), который авторизует запрос, считает аналитику, нормализует
  модель и делегирует вызов ядру (SDK).
- Auth: LDAP-логин в production или static identity в development/test, выпуск
  session-токенов и API-keys, хранение в Postgres.
- Management-поверхность (R9): пользовательские операции (свои API-keys,
  личная статистика) и администраторские (настройка провайдеров OAuth/API-key,
  управление пользователями, квоты подписок, тестирование аккаунтов, список
  моделей, экспорт/импорт OAuth JSON).
- System egress proxy (R10): `HTTP_PROXY`, `HTTPS_PROXY`, `NO_PROXY`.
- Аналитика использования (события → Postgres → агрегаты).
- Scheduler/watcher-оркестрация: координация refresh-джоб (leader election),
  persistence кэша токенов/моделей от ядра.
- Observability, k8s-deployment.

Чего мы НЕ пишем:
- Протоколы refresh токенов конкретных провайдеров — это в ядре.
- Transport/стриминг/парсинг ответов провайдеров — это в ядре.
- Реестр моделей провайдеров — это в ядре (мы храним кэш/override).
- **Пользовательские квоты и rate-limit** — на первой версии не реализуются
  (отложено). См. глоссарий: upstream-квота подписки (R9.A.4) — другое.

---

## Функциональные требования

### R1. Аутентификация и выпуск токенов
- R1.1 В production пользователь аутентифицируется по LDAP (bind/search).
- R1.2 После успешного входа сервис выпускает **opaque session-токен**
  (DB-stored, immediate revocation).
- R1.3 Сессия имеет TTL, **без продления** (см. решение ниже).
- R1.4 **Provisioning пользователя в БД при первом логине.** Без проверки
  LDAP-группы (R1.1 + группы) войти нельзя — соответственно, строка в таблице
  `users` создаётся **только после успешной авторизации** (проход по группам).
  LDAP дальше проверяет пароль и членство в группах (live-lookup), профиль
  пользователя берётся из БД. API-keys (R2) и аналитика (R3) привязаны к
  `users.id` через FK. Без LDAP-группы пользователь не может быть создан.
  Запись `users` имеет поле `status` (`active`/`blocked`) — логин
  блокированного пользователя отклоняется (см. R9.A.3).
- R1.5 **Static identity source для development/test.** Конфигурация
  `auth.mode` принимает `ldap` (default) или `static`; static разрешён только
  при `server.environment=development|test`. `auth.mode=static` в production
  делает конфигурацию невалидной и приложение не запускается. Static username,
  password и role (`user`/`admin`) приходят только из env; static не является
  fallback при ошибке LDAP. Для изоляции static user сохраняется в БД как
  `username=static:<username>` с `identity_source=static`; LDAP provider
  отклоняет LDAP-имя с префиксом `static:`. Session middleware и
  `access.Provider` принимают credential только при совпадении
  `users.identity_source` с активным `auth.mode`. Переключение mode выполняется
  только после остановки всех dev/test реплик, rolling switch запрещён.
- ✅ **Решено:** формат — opaque session-токен (не JWT), хранится в БД →
  мгновенный revocation, но каждый запрос требует lookup (кэшировать).
- ✅ **Решено:** сессия передаётся в **cookie** (браузерный SSO-flow). Отдельный
  login-эндпоинт устанавливает cookie; дальнейшие запросы авторизуются по ней.
- ✅ **Решено:** TTL сессии зависит от роли:
  - обычный пользователь — **5 минут**;
  - администратор — **10 часов**.
- ✅ **Решено:** роли/членство в LDAP-группах проверяются **live-lookup** при
  каждом логине (без фоновой синхронизации в БД на данном этапе).
- ✅ **Решено:** **две отдельные LDAP-группы, обе задаются в конфиге сервиса:**
  - **группа пользователей** — членство в ней даёт право на вход в сервис
    (авторизация доступа). Пользователь не в этой группе — доступ запрещён,
    даже если LDAP-bind успешен;
  - **группа администраторов** — отдельная проверка членства, определяет роль
    admin (→ TTL сессии 10 часов).
  Обе группы — независимые настройки в `config.yaml` (DN группы), чтобы
  покрыть разные схемы каталога без правок кода.
- ✅ **Решено (логика групп):** роль определяется так:
  - если в группе администраторов → роль **admin** (admin подразумевает право
    входа, отдельной проверки user-группы не требуется);
  - иначе если в группе пользователей → роль **user**;
  - иначе → **отказ** в логине.
  Пользователь может состоять одновременно в обеих группах — это не ошибка,
  роль будет admin (старшая). Группа пользователей проверяется только если
  пользователь не админ.
- ✅ **Решено:** политика refresh — **фиксированный TTL, без продления**.
  Сессия строго истекает через 5 минут (user) / 10 часов (admin) после логина,
  sliding window и refresh-токен не предусмотрены.
  ⚠️ Следствие: для браузерного/admin-доступа требуется перелогин по
  истечении TTL. Непрерывный программный доступ покрывается **long-lived
  API-keys** (см. R2), а не сессией.
- ✅ **Решено:** обе группы (пользователей и администраторов) задаются через
  **конфиг** сервиса, что даёт гибкость под конкретную схему каталога без
  правок кода.
- ❓ **Открыто:** точный формат конфиг-секции LDAP (DN группы пользователей,
  DN группы администраторов, base DN, bind DN, filter-шаблон) — зафиксировать
  в R1-дизайне.
- ❓ **Открыто:** атрибуты cookie (HttpOnly, Secure, SameSite, имя cookie,
  путь/домен) — зафиксировать в R1-дизайне.

### R2. Авторизация запросов к прокси-API
- R2.1 Каждый запрос к прокси-API авторизуется **long-lived API-key** (как у OpenAI).
- R2.2 API-key привязан к пользователю, **хэшируется** в БД (только hash),
  пользователь видит открытый ключ один раз при создании.
  Алгоритм хэширования — bcrypt (cost 12), см. R5 (класс «односторонние хэши»).
- R2.3 API-key можно отзывать, опциональный expiry и scope.
- R2.4 **Revocation в multi-replica:** отзыв истечения/expiry приводит к
  удалению/обновлению строки в Postgres. In-process кэш (R6) обеспечивает
  eventual consistency — отозванный ключ может работать до TTL кэша (5–15с).
  Это сознательное окно риска; при необходимости мгновенного revocation —
  уменьшить TTL кэша или ввести Redis (ADR-8).
- ✅ **Решено:** два механизма — opaque session (люди/UI) + long-lived API-keys
  (программный доступ).
- ✅ **Решено (ADR-9):** проверка API-key на каждый запрос — через реализацию
  `access.Provider.Authenticate` (контракт ядра). Session-cookie после login
  через identity source (R1) — отдельный middleware, не access.Provider.
- ✅ **Решено:** `access.Provider` дополнительно проверяет `users.status` —
  API-key заблокированного пользователя (`status=blocked`) отклоняется
  (с учётом eventual consistency R2.4).
- ✅ **Решено:** `access.Provider` и session middleware дополнительно сверяют
  `users.identity_source` с текущим `auth.mode`; static API-key/cookie не
  действуют в LDAP/prod режиме.
- ✅ **Решено:** бизнес-слой **полностью заменяет** встроенный `config-api-key`
  ядром inline-провайдер через `access.SetExclusiveProvider("db-apikey")` —
  inline `cfg.APIKeys` ядра **не используются** (все клиентские API-keys живут
  только в БД). Это исключает двойной путь auth и случайно оставленные
  inline-ключи в конфиге.

### R3. Аналитика использования
- R3.1 Сбор событий на каждый запрос: пользователь, токен/API-key, провайдер,
  модель, prompt/completion токены, latency, статус, ошибка.
- R3.2 Агрегации по: пользователям, моделям, провайдерам, токенам, времени.
- ✅ **Решено:** хранилище — та же Postgres. Сырые события в партиционированной
  (по дню) таблице + материализованные агрегаты.
- ✅ **Решено (намерение):** слой репозитория абстрагировать так, чтобы при
  росте объёмов заменить реализацию на ClickHouse без переписывания потребителей.
- ✅ **Решено (ADR-9):** источник данных — `usage.Plugin.HandleUsage(Record)`
  контракта ядра. `Record` содержит готовую структуру: Provider, Model, Alias,
  AuthID, AuthType, ReasoningEffort, Latency, TTFT, Failed, Failure{StatusCode,
  Body}, Detail{Input/Output/Reasoning/Cached/TotalTokens}. Привязка к
  пользователю/API-key — по principal из `access.Provider.Result` в context.
  ⚠️ **Важно для стриминга:** `HandleUsage` может вызываться асинхронно в конце
  потока, когда request-context уже отменён. Поэтому principal/user_id должен
  **копироваться в Record** в момент старта запроса, а не читаться из context
  в момент `HandleUsage`.
- ❓ **Открыто:** глубина ретенции сырых событий, TTL-джобы.
- ❓ **Открыто:** встроенные дашборды в сервисе или только API + Grafana.

### R4. Контроль промптов и анонимизация (на будущее)
- R4.1 Анонимизация использования по ключевым словам.
- 🕒 Выносится на отдельную итерацию (TBD).

### R5. Хранение данных (Postgres)
- R5.1 Postgres хранит: auth-токены, API-keys, пользователей, аналитику,
  конфигурацию/override моделей, кэш моделей из ядра, учётные данные upstream.
- R5.2 **Стек доступа:** `pgx` (v5, пул) + `sqlc` (type-safe генерация из SQL).
- R5.3 **Миграции:** `golang-migrate` (SQL-файлы в `db/migrations/`).
- ✅ **Решено:** pgx + sqlc + golang-migrate.
- ✅ **Решено:** шифрование секретов at-rest — **два класса по криптосвойствам:**
  - **Односторонние хэши** (one-way, восстановить нельзя, только сверить):
    API-keys, пароли (если храним). Алгоритм — **bcrypt** (cost 12) или
    argon2id. Открытое значение API-key показывается пользователю один раз при
    создании; в БД — только hash.
    Эталонный интерфейс пакета `internal/security`:
    ```go
    package security

    import "golang.org/x/crypto/bcrypt"

    const bcryptCost = 12

    // HashPassword — односторонний хэш (для API-key/пароля).
    func HashPassword(password string) (string, error) { ... }

    // CheckPassword — сверка с хэшем.
    func CheckPassword(hash, password string) bool { ... }
    ```
  - **Обратимое шифрование** (two-way, нужно расшифровать для использования):
    upstream-credentials (OAuth/refresh-токены провайдеров) — секреты, которые
    сервис хранит **в БД** и расшифровывает для передачи ядру.
    Алгоритм — **AES-256-GCM** (симметричный, аутентифицированный), ключ — из
    конфига/KMS.
    ⚠️ bcrypt сюда НЕ подходит (однонаправленный, токен не восстановить).
  - **Секреты вне БД (только env/k8s Secret):** LDAP bind-password service-
    account, мастер-ключ AES, DB password. Эти секреты **не в БД** → AES-шифрование
    к ним не применяется (k8s Secret + at-rest шифрование etcd — достаточная
    защита). Ранее LDAP bind ошибочно упоминался в классе AES-шифруемых —
    исправлено: он только в env.
- ✅ **Решено:** источник мастер-ключа для AES-256-GCM — **env-переменная**
  (base64, 32 байта). В k8s кладётся в Secret и монтируется как env
  (напр. `CLIPROXY_ENCRYPTION_KEY`). Без внешних зависимостей (KMS/Vault)
  на данном этапе. Слой шифрования держим за интерфейсом, чтобы позже
  подключить KMS без правок потребителей.
  ⚠️ **Ротация ключа:** формат шифртекста должен включать **key-version**
  (префикс/тег), чтобы при ротации мастер-ключа старые шифртексты
  расшифровывались предыдущим ключом. По умолчанию используется один
  key-version=1; схема хранения — набор key-version → ключ, активный
  key-version в конфиге. Активный ключ передаётся через
  `CLIPROXY_ENCRYPTION_KEY`, предыдущие — через опциональную JSON-карту
  `CLIPROXY_ENCRYPTION_PREVIOUS_KEYS` (`{"1":"<base64>"}`).
  Без этого ротация невозможна (старые секреты станут нечитаемы).

### R6. Надёжность, масштабирование, observability, k8s
- R6.1 Приложение **stateless**, всё состояние — в Postgres.
- ✅ **Решено:** `Config.Routing.SessionAffinity` ядра — **отключён**
  (`SessionAffinity: false`). `SessionCache` ядра — in-memory, в multi-replica
  аффинность нарушается между репликами (stateless R6.1 конфликтует с
  in-memory affinity). Используется только round-robin/fill-first роутинг.
- R6.2 Запуск в нескольких репликах в k8s; горизонтальное масштабирование.
- R6.3 Liveness/readiness probes, graceful shutdown.
- R6.4 **Бэкапы Postgres — вне ответственности репозитория** (задача
  эксплуатации: CloudNativePostgres / Velero / внешний DBA). Внимание: БД —
  единственное состояние сервиса (сессии, API-keys, аналитика, upstream-
  credentials), потеря = полная катастрофа. Рекомендация эксплуатации:
  регулярные logical/physical-бэкапы + WAL-archiving (PITR-готовность) +
  задокументированная restore-процедура.
- R6.5 Observability: метрики (Prometheus), трейсинг (OpenTelemetry),
  структурные логи (`slog`).
- ✅ **Решено:** **без Redis** на данном этапе (ADR-8).
  ⚠️ Следствия для multi-replica (важно для дизайна):
  - **Session/token lookup** — каждый запрос читает сессию/API-key из Postgres.
    Чтобы не бить БД на каждый запрос, нужен **in-process кэш с коротким TTL**
    (напр. 5–15с). Допустима рассинхронизация между репликами в пределах TTL
    (revocation вступает в силу с задержкой до TTL). Приемлемо, т.к. session TTL
    пользователя и так 5 минут.
  - **Кэш моделей/override** — in-process с invalidation по TTL; источник
    истины — Postgres.
  - **Coordination (leader election)** — Postgres advisory lock (ADR-7),
    Redis не требуется.
  Слой кэша держим за интерфейсом (`internal/cache`), чтобы при росте
  подключить Redis без правок потребителей.
- ✅ **Решено:** конфигурация — **config.yaml (k8s ConfigMap) + env для
  секретов (k8s Secret)**. Секции: `server` (включая environment), `auth`,
  `ldap`, `encryption`, `db`, `logging`. System proxy задается переменными
  `HTTP_PROXY`, `HTTPS_PROXY`, `NO_PROXY`. Поддержка
  env-override поверх файла (12-factor). Секреты
  (`CLIPROXY_ENCRYPTION_KEY`, DB password, LDAP bind password, static user
  credentials для development/test) — только через env, не в config.yaml.

### R7. Scheduler / watcher (оркестрация refresh токенов и моделей)
- R7.1 Координация периодического обновления upstream-токенов и списка моделей.
  Сами refresh-протоколы выполняет ядро (SDK); бизнес-слой оркестрирует
  запуск (scheduler), persistence результата и обработку сбоев.
- R7.2 В multi-replica — **leader election**, чтобы планировщик работал в одной
  реплике одновременно.
- R7.3 Observability джоб: метрики успешности/latency, алёрты на сбои refresh.
- ✅ **Решено:** leader election — **Postgres advisory lock** (не добавляет
  зависимостей, Postgres уже есть).
- ✅ **Решено (ADR-9):** архитектура refresh определена контрактами ядра:
  - ядро само вызывает `coreManager.StartAutoRefresh(ctx, 15*time.Minute)` с
    min-heap по `NextRefreshAfter`, до 16 воркеров, вызывает
    `ProviderExecutor.Refresh(ctx, auth)` (refresh-протоколы Codex/Claude/xAI
    — в ядре);
  - бизнес-слой НЕ пишет refresh-логику — он реализует `coreauth.Store`, и ядро
    само зовёт `Store.Save` после refresh;
  - `WatcherFactory` отключает file-backed source: изменения credentials
    синхронизируются через Postgres revision и controlled restart. Прямой
    DB-push в SDK-очередь ждёт публичный тип AuthUpdate, поскольку текущий
    находится в upstream `internal/*` (R12).
- ❓ **Открыто:** точные настройки `StartAutoRefresh` (интервал по умолчанию 15
  мин, max-concurrency=16, `RefreshEvaluator`) — зафиксировать при
  имплементации watcher'а; retry/backoff — на стороне ядра.

### R8. Клиентские API-форматы
- R8.1 Поддерживаются **все форматы, которые предоставляет ядро (SDK):**
  - **OpenAI** — Chat Completions (`/v1/chat/completions`) и Responses API;
  - **Anthropic/Claude** — Messages API (`/v1/messages`);
  - **Gemini** — `generateContent` (`/v1beta/models/...:generateContent`);
  - **Codex** (GPT-модели через OAuth);
  - **Grok**.
- R8.2 Бизнес-слой не реализует парсинг/трансляцию форматов — это делает ядро.
  Бизнес-слой: авторизует запрос (R2), считает аналитику (R3), применяет
  allow-list и provider selection модели, делегирует вызов ядру. Runtime
  model-rewrite ждёт отдельный публичный SDK hook (R12).
- ✅ **Решено:** набор форматов = все из ядра (ADR-2).

### R9. Пользовательский и административный функционал (management)

Два уровня доступа: **user** и **admin** (роли из R1). Управление доступно
через отдельную management-поверхность API (и, опционально, UI).

### R9.U — Функционал пользователя
- **R9.U.1** Авторизация (login через выбранный identity source →
  cookie-сессия, см. R1).
- **R9.U.2** Выпуск собственных **API-keys** (CRUD: создать / список /
  отозвать). API-key привязан к пользователю (R2.2), хэшируется в БД.
  Открытое значение показывается один раз при создании.
- **R9.U.3** **Личная статистика** по собственным API-keys и использованию
  (отдельно от общей аналитики R3 — пользователь видит только свои данные:
  свои ключи, свои запросы, свои токены/модели). Источник — та же аналитика
  R3, отфильтрованная по `user_id`.

### R9.A — Функционал администратора
- **R9.A.1** Настройка **OAuth для провайдеров с подписками** (Codex/Claude/
  Gemini/Grok и т.д.). Запуск authorization-flow, хранение полученных
  upstream-credentials в БД (зашифрованные, R5).
  ✅ **Решено: собственная асинхронная реализация поверх `sdk/api/management.go`
  хелперов, сессии в Postgres** (а НЕ через блокирующий `sdkAuth.Manager.Login`,
  который не отдаёт URL наружу и не годится для API).
  - `sdkAuth.Manager.Login()` — **блокирующий, синхронный**: URL печатается в
    stdout (не возвращается), callback ловится локальным сервером на
    `CallbackPort`, `LoginOptions.Prompt` срабатывает через 15с как CLI-fallback.
    Непригоден для management-API.
  - **Ядро уже содержит асинхронный flow** в `internal/api/handlers/management/`:
    `GET /*-auth-url → {url, state}`, `POST /oauth-callback → файловый poller`,
    `GET /get-auth-status`. Но OAuth-сессии там **in-memory в одной реплике** →
    в multi-replica (R6.2) replica A стартует flow, replica B не может завершить.
  - **Решение:** бизнес-слой переносит асинхронную логику (PKCE + state +
    обмен) поверх `claude.NewClaudeAuth`/`codex.NewCodexAuth`/... (низкоуровневые
    сервисы, без блокирующего `Login`), но **сессии хранит в Postgres**
    (`oauth_sessions`) — любая реплика может завершить flow.
  ✅ **API-flow (callback-провайдеры Codex/Claude/Antigravity):**
  - `POST /api/v1/admin/oauth/{provider}/start` → создаёт PKCE+state, persist
    в `oauth_sessions`, возвращает `{auth_url, state}`.
  - админ открывает `auth_url` в браузере; провайдер редиректит на
    `localhost:<CallbackPort>` (redirect_uri вшит у провайдера).
  - админ копирует callback URL → `POST /api/v1/admin/oauth/{provider}/complete`
    `{state, redirect_url}` → бизнес-слой обменивает code на токены, сохраняет
    через `Store.Save` (AES-GCM, R5), пишет `admin_audit_log`.
  ✅ **API-flow (device-провайдеры Kimi/xAI, и опц. Codex-device):**
  - `POST /api/v1/admin/oauth/{provider}/start` → `StartDeviceFlow`, persist
    session, возвращает `{auth_url, user_code, expires_in, flow: "device"}`.
  - админ вводит `user_code` на странице провайдера; бизнес-слой poll-ит
    завершение в goroutine.
  - клиент poll-ит статус: `GET /api/v1/admin/oauth/sessions/{state}`.
  ✅ **Статус и отмена:**
  - `GET /api/v1/admin/oauth/sessions/{state}` → `{status: pending|completed|error, ...}`.
  - `DELETE /api/v1/admin/oauth/sessions/{state}` → отмена flow.
  ❓ **Открыто:** callback-port — нужен ли exposed-port в k8s для
  callback-forwarder (`is_webui` режим), или всегда ручной copy-paste
  callback-URL админом. На первой версии — ручной copy-paste (проще, не требует
  exposed-port в k8s). См. дизайн [docs/design/r9-oauth-and-testing.md](design/r9-oauth-and-testing.md).
- **R9.A.2** Настройка **API-keys для провайдеров** (batch, по API).
  Регистрация набора статических upstream API-keys (напр. OpenRouter,
  кастомные OpenAI-compatible эндпоинты) — сохраняются как `coreauth.Auth`
  с `AuthType=api-key`, шифруются (R5). Поддержка множества провайдеров за
  один вызов (batch endpoint).
- **R9.A.3** Управление **API-keys пользователей**: список всех ключей в
  системе, блокировка пользователя. Блокировка обратимая — через поле
  `status` на `users` (`active`/`blocked`): перевод в `blocked` блокирует
  новые логины и все его API-keys (перестают авторизоваться через
  `access.Provider`, с учётом eventual consistency R2.4), но не удаляет их;
  разблокировка возвращает всё в строй. Сессии блокированного пользователя
  инвалидируются (revocation в БД).
- **R9.A.4** Просмотр **квоты/лимитов текущей подписки** провайдера (сколько
  доступно по upstream-аккаунту: остаток запросов/токенов, срок действия
  подписки). Источник — metadata `coreauth.Auth` (поля `expired`,
  `expires_at`, quota-поля) + опрос провайдера через ядро при необходимости.
- **R9.A.5** **Тестирование настроек авторизации**: проверка валидности
  конкретного upstream-аккаунта (OAuth-токена или API-key) **без траты
  inference-квоты подписки.**
  ✅ **Решено: собственная обёртка с разделением по типу креды:**
  - **OAuth-аккаунты (Codex/Claude/Antigravity):** вызов
    `executor.Refresh(ctx, auth)` — обмен `refresh_token → access_token`.
    Успех = `refresh_token` жив, получен свежий access_token. **Не тратит
    inference-квоту** (только token-endpoint провайдера). Для Antigravity
    Refresh заодно обновляет `AntigravityCreditsHint` (актуальный баланс —
    бонус для R9.A.4).
  - **API-key аккаунты (Gemini-key/Claude-key/OpenAI-compat):** `Refresh`
    бесполезен (no-op). Вместо него — лёгкий
    `executor.HttpRequest(ctx, auth, req)` к metadata-endpoint провайдера
    (`GET /models` или эквивалент) без inference. Валиден если HTTP 200.
  - **Не используются:** `Execute`/`ExecuteCount`/`CountTokens` — это
    реальные inference-запросы, тратят квоту. `Auth.Status`/`LastError`/
    `Quota` — stale-кэш (неактуальная гарантия), только как доп. контекст.
  ✅ **Ответ API:** `{valid: bool, method: "refresh"|"http_probe", details:
  {status_code, expires_at, last_refreshed_at}, quota: {exceeded, reason}}`.
  ❓ **Открыто:** per-provider список metadata-endpoint'ов для health-check
  (Claude `GET /v1/models`, Codex `GET /backend-api/me`, Gemini `GET /models`,
  OpenAI-compat `GET {base_url}/models`) — зафиксировать в дизайне.
- **R9.A.6** Настройка **списка доступных моделей**: override/фильтр
  глобального реестра моделей (R8, ModelRegistryHook). Администратор задаёт,
  какие модели из реестра ядра разрешены клиентам (allow-list), а также
  desired model-mapping (алиасы, переадресация). Хранится в Postgres;
  `Selector` применяет allow-list и provider selection. Runtime rewrite
  `upstream_model` заблокирован до отдельного публичного SDK hook, поскольку
  `Selector.Pick` возвращает только выбранный auth (R7/ADR-9, R12).
- **R9.A.7** **Экспорт/импорт OAuth-credentials в JSON.** Администратор может
  скачать JSON с upstream OAuth-authorization конкретного аккаунта и загрузить
  его обратно (backup/restore, миграция между инсталляциями, импорт
  pre-authenticated аккаунтов).
  - **Экспорт:** сериализация `coreauth.Auth` в JSON (нативный формат ядра,
    `Storage`/`Metadata` поля). Отдаётся как файл через management-API.
  - **Импорт:** парсинг JSON → валидация → сохранение через `coreauth.Store.Save`
    (с шифрованием at-rest, R5) → регистрация в ядре (через `WatcherFactory`
    или напрямую).
  ⚠️ **Безопасность:** JSON содержит чувствительные токены. См. вопросы ниже.
  - ✅ **Решено:** **содержимое JSON — полные credentials** (access_token +
    refresh_token + metadata: provider, email, expiry, атрибуты). Полноценный
    экспорт, позволяющий мигрировать аккаунт между инсталляциями и сразу
    пользоваться. Файл = полный доступ к upstream-аккаунту → обращаться как с
    секретом (транспорт только по HTTPS, не логировать, короткий TTL доступа).
  - ✅ **Решено:** **dedup при импорте** — по уникальному ключу
    `provider + email` (из metadata). Если аккаунт уже есть в системе, импорт
    отклоняет или (опционально, по флагу запроса) делает upsert (обновляет
    credentials существующей записи, сохраняя её ID/привязки). Защищает от
    случайных дублей.
  - ✅ **Решено:** экспорт и импорт — **mutating/чувствительные действия**,
    пишутся в `admin_audit_log` (R9.G): кто, когда, какой аккаунт (provider +
    email), направление (export/import).

### R9.G — Общее для management-поверхности
- **R9.G.1** Все management-эндпоинты требуют аутентифицированной сессии
  (cookie, R1) и проверки роли. User-эндпоинты — доступ только к своим
  данным; admin-эндпоинты — только роль admin.
- **R9.G.2** Management API не пересекается с прокси-API (R8) — отдельные
  роуты (напр. `/api/v1/...` для management, `/v1/...` для прокси-форматов).
  Регистрируются через `api.WithRouterConfigurator` (ADR-9).
- ✅ **Решено:** на первой версии — **только REST API** (без встроенного UI).
  UI откладывается на отдельную итерацию; контракты API проектируем так,
  чтобы UI можно было добавить позже без переделки.
- ✅ **Решено:** **аудит-лог действий администратора** — отдельная таблица
  `admin_audit_log` (кто/когда/что изменило: настройки провайдеров, OAuth,
  блокировка пользователя, список моделей). Пишется на каждое mutating-
  действие в R9.A. Источник для compliance и разбора инцидентов.
- ❓ **Открыто:** форматы management-API (REST/JSON-схемы эндпоинтов) —
  зафиксировать в R9-дизайне.

### R10. System egress proxy
- **R10.1** Все исходящие HTTP-запросы к upstream используют системный proxy
  процесса: `HTTP_PROXY`, `HTTPS_PROXY`, `NO_PROXY` (стандартный
  `http.ProxyFromEnvironment`). Единая policy применяется к inference, OAuth,
  refresh, quota и model requests.
- **R10.2** В `config.yaml` и в namespace `CLIPROXY_*` нет proxy URL. Значения
  proxy передаются окружением deployment; они не логируются и не попадают в
  audit log. `NO_PROXY` задает адреса direct-подключения.
- **R10.3** **Per-provider `base_url`** (отдельное требование): каждый
  провайдер может иметь свой базовый URL upstream-эндпоинта (для кастомных
  OpenAI-compatible эндпоинтов, напр. OpenRouter). Реализуется через
  `coreauth.Auth.Attributes["base_url"]` (нативное поле ядра).
- ✅ **Решено (ADR-10):** бизнес-слой очищает `Auth.ProxyURL`, не создает
  per-call-type override и делегирует выбор proxy стандартному HTTP transport
  процесса. См. [ADR-10](adr/ADR-10-per-call-type-proxy.md).

### R11. OpenAPI/Swagger-спецификация
- **R11.1** Все HTTP-роуты сервиса описываются в **OpenAPI 3.1** спецификации.
- **R11.2** **Spec-first подход:** спецификация (`openapi.yaml`) — первичный
  контракт. Go-код хендлеров и типы генерируются из спецификации
  (генератор: `ogen` или `oapi-codegen` — зафиксировать при настройке CI).
  Ручные правки сгенерированного кода не допускаются.
- **R11.3** **Покрытие — все роуты:**
  - **Management-API** (`/api/v1/*`): login/logout, user (me/keys, me/usage),
    admin (users, oauth/*, accounts/test, providers/keys, models, oauth/export|import),
    oauth/sessions — **с полными схемами request/response**.
  - **Прокси-роуты** (`/v1/*`: chat/completions, messages, generateContent,
    responses, models, и т.д.): **перечисление URL без детальных body-схем**
    (тела проксируются ядром, мы не владеем их схемой); описывается только
    auth (Bearer API-key) и общие error-ответы.
  - **Пользовательские роуты выпуска токенов** (если отличны от management):
    полные схемы.
  - **Системные роуты:** `/healthz`, `/readyz`, `/metrics` — описание без схем.
- **R11.4** **Доступ к спецификации:** `/openapi.json` (или `/swagger`) эндпоинт
  отдаёт спецификацию; `/docs` (опционально) — Swagger UI / Redoc для
  интерактивной документации.
- **R11.5** **CI-валидация:** спецификация валидируется на:
  - синтаксис OpenAPI 3.1 (lint: `spectral` или `vacuum`);
  - согласованность с Go-кодом (сгенерированные типы соответствуют спеке);
  - все management-эндпоинты имеют response-схемы (no `Any` для управляемых
    роутов).
- ✅ **Решено:** spec-first, OpenAPI 3.1, все роуты (прокси без body-схем).
- ❓ **Открыто:** генератор (`ogen` vs `oapi-codegen`) — зафиксировать при
  настройке CI/проекта.
- ❓ **Открыто:** версия URL (`/api/v1` confirmed; нужен ли `/api/v2`
  механизм версионирования с самого начала).

### R12. Обновляемость SDK ядра
- R12.1 `github.com/router-for-me/CLIProxyAPI/v7` остаётся внешней
  версионированной зависимостью. Бизнес-слой использует только публичные
  пакеты `sdk/*`; импорт `internal/*` SDK, fork, patch и reflect-обходы
  запрещены.
- R12.2 Версия SDK фиксируется в `go.mod`. Обновления выполняются отдельным
  reviewable изменением с release notes и перечнем breaking changes upstream;
  автоматическое обновление на новый major запрещено.
- R12.3 Перед merge обновления SDK обязательны `go mod tidy`, `go vet`, build,
  unit/race/integration тесты и `go test -race ./internal/sdkcontract` для
  публичных extension points ADR-9.
  [`sdk-reference.md`](sdk-reference.md) сверяется с публичным API и
  актуализируется в том же изменении.
- R12.4 Несовместимость SDK адаптируется в boundary-пакете `internal/*`, без
  копирования upstream-логики и без изменения бизнес-контракта без ADR.
  Откат — возврат `go.mod`/`go.sum` к последней проверенной версии.
- ✅ **Решено:** обновления patch/minor внутри v7 допускаются только после
  compatibility gate; переход на новый major требует отдельного ADR и
  миграционного плана.

---

## Открытые архитектурные решения

| ID | Решение | Статус |
|----|---------|--------|
| ADR-1 | Модель интеграции: ядро = внешняя go-зависимость, CLIProxyNew = бизнес-слой (как CLIProxyAPI + CLIProxyAPIBusiness) | ✅ Решено |
| ADR-2 | Клиентские API-форматы: ✅ все поддерживаемые ядром — OpenAI (Chat Completions + Responses), Anthropic/Claude (Messages), Gemini (generateContent), Codex, Grok | ✅ Решено |
| ADR-3 | LDAP-интеграция: ✅ прямой bind/search + live-lookup групп (без фоновой синхронизации) | ✅ Решено |
| ADR-4 | Модель multi-tenancy: ✅ плоская (пользователи + роли user/admin) | ✅ Решено |
| ADR-5 | Хранилище аналитики: ✅ Postgres (с заделом под замену на ClickHouse) | ✅ Решено |
| ADR-6 | Доступ к БД: ✅ pgx + sqlc + golang-migrate | ✅ Решено |
| ADR-7 | Leader election для scheduler: ✅ Postgres advisory lock | ✅ Решено |
| ADR-8 | Redis: ✅ пока без Redis. Кэш/сессии — in-process; coordination (leader election) — Postgres advisory lock | ✅ Решено |
| ADR-9 | Контракты интеграции с SDK ядра: ✅ бизнес-слой реализует 7 контрактов (coreauth.Store, Selector, Hook; usage.Plugin; access.Provider; WatcherFactory; ModelRegistryHook). См. [docs/adr/ADR-9-sdk-contracts.md](adr/ADR-9-sdk-contracts.md) | ✅ Решено |
| ADR-10 | System egress proxy: ✅ `HTTP_PROXY`/`HTTPS_PROXY`/`NO_PROXY`, без `Auth.ProxyURL` и `proxy.*`. Per-provider base_url через Auth.Attributes. См. [docs/adr/ADR-10-per-call-type-proxy.md](adr/ADR-10-per-call-type-proxy.md) | ✅ Решено |

## История изменений
- 2026-07-11 — черновик по первому набору требований (6 пунктов).
- 2026-07-11 — **правка модели интеграции:** ядро upstream relay — внешняя
  go-зависимость (по аналогии с CLIProxyAPI + CLIProxyAPIBusiness), а не
  компонент этого репозитория. Обновлены R5, R7, ADR.
- 2026-07-11 — зафиксированы ответы по R1: сессия в cookie; TTL user=5мин /
  admin=10ч; LDAP-группы через live-lookup (ADR-3 закрыт).
- 2026-07-11 — R1: политика refresh = фиксированный TTL без продления;
  роль админа определяется по LDAP через конфиг. Непрерывный доступ —
  через long-lived API-keys (R2).
- 2026-07-11 — R1: добавлена отдельная LDAP-группа пользователей (право на
  вход) помимо группы администраторов. Обе группы — в конфиге.
- 2026-07-11 — R5: шифрование секретов at-rest. Два класса: односторонние
  хэши (bcrypt, для API-keys/паролей) и обратимое шифрование (AES-256-GCM,
  для upstream-credentials). Мастер-ключ — env-переменная (k8s Secret).
- 2026-07-11 — ADR-2: поддерживаются все форматы ядра (OpenAI, Anthropic,
  Gemini, Codex, Grok) → новая секция R8. ADR-8: без Redis → последствия
  для multi-replica (in-process кэш, Postgres-счётчики) зафиксированы в R6.
- 2026-07-11 — ADR-9 закрыт: исследованы контракты SDK v7, бизнес-слой
  реализует 7 контрактов (Store, Selector, Hook, usage.Plugin, access.Provider,
  WatcherFactory, ModelRegistryHook). См. docs/adr/ADR-9-sdk-contracts.md.
  Уточнены R3 (usage.Record) и R7 (StartAutoRefresh ядра).
- 2026-07-11 — **ревью требований:** исправлены противоречия и закрыты пробелы:
  - R1.3: убрано «есть refresh» (противоречило фикс. TTL); добавлена R1.4
    (provisioning пользователя в БД при первом логине, после проверки LDAP-групп);
  - логика групп: admin подразумевает user (но может состоять в обеих);
  - R2.4: revocation API-key в multi-replica (eventual consistency до TTL кэша);
  - R3: principal копируется в Record при стриминге (context может быть отменён);
  - R5: формат шифртекста с key-version для ротации мастер-ключа AES;
  - R6.1: убран «опц. Redis» (противоречило ADR-8); убран rate-limit из
    следствий (квоты/лимиты отложены);
  - R7/R8: порядок секций исправлен;
  - роль репо: квоты/rate-limit помечены как отложенные.
- 2026-07-11 — добавлен блок R9 (management-поверхность): пользовательские
  операции (R9.U — авторизация, свои API-keys, личная статистика) и
  администраторские (R9.A — настройка OAuth/API-key провайдеров, управление
  пользователями, квоты подписок, тестирование аккаунтов, список моделей).
- 2026-07-11 — R9 закрыты: только REST API (UI позже), аудит-лог
  admin_audit_log, блокировка пользователя через users.status (обратимая,
  проверяется в access.Provider и при логине). R1.4 и R2 дополнены.
- 2026-07-11 — R9.A.7: экспорт/импорт OAuth-credentials в JSON. Полные
  credentials, dedup по provider+email, аудит в admin_audit_log.
- 2026-07-11 — R10 per-call-type proxy + ADR-10 (историческое решение,
  заменено system proxy policy 2026-07-15).
- 2026-07-11 — **второе ревью требований:** исправлены противоречия и пробелы:
  - добавлен глоссарий (два смысла «квоты»: upstream-подписка vs
    пользовательский rate-limit);
  - R5: два класса секретов (bcrypt для API-keys, AES-256-GCM для
    upstream-credentials); ⚠️ позже LDAP bind убран из AES-класса (см. 2026-07-12);
  - R6: убрано упоминание квот/лимитов из роли репо; добавлен R6.4 (бэкапы —
    вне репо, задача эксплуатации); нумерация R6 исправлена;
  - R9.A.1: OAuth-flow зафиксирован как API-driven (GET → ссылка, POST →
    callback, redirect_uri=localhost, сервис не слушает inbound redirect);
  - R10/ADR-10: историческое ограничение auto-refresh для per-call-type proxy
    заменено единым system proxy процесса;
  - статус документа переведён из DRAFT в «зафиксированы».
- ❓ **Открыто (после второго ревью):** R9.A.5 — тестирование через `Execute`
  тратит upstream-квоту (ограничить минимальным запросом или принять стоимость);
  R9.A.7 — propagation импортированных аккаунтов в multi-replica (через
  WatcherFactory только на лидере). Оба — в дизайне R9.
- 2026-07-11 — **актуализация всех документов:** README и AGENTS переписаны
  под R1–R10 + ADR-9/ADR-10 (полная структура пакетов, стек, gotchas).
  Исправлены китайские артефакты в тексте (生效 → «вступает в силу»,
  短期 → «короткий»), роль репо дополнена (Codex/Grok, management, прокси).
  ADR-9 дополнен cross-reference к ADR-10 про auto-refresh.
- 2026-07-11 — **R6 закрыт:** конфиг = config.yaml (ConfigMap) + env-секреты
  (k8s Secret). Секреты только через env. Поддержка env-override (12-factor).
- 2026-07-11 — **архитектурный дизайн:** созданы
  docs/architecture.md (components, data flows, deployment) и
  docs/database-schema.md (ER, таблицы, индексы, миграции).
- 2026-07-12 — **третье ревью (сверка с SDK-референсом):** исправлены
  критические неточности, выявленные после анализа реальных контрактов ядра:
  - DB: `upstream_accounts.id` = text PK (= Auth.ID string), FK в
    usage_events/admin_audit_log приведены к text; политика удаления аккаунта
    (ON DELETE SET NULL для аналитики);
  - R9.A.1: OAuth-flow переписан — PKCE-провайдеры (Codex/Claude) сами
    открывают callback-сервер на CallbackPort; device-flow (Codex/Kimi/xAI)
    не требует; вопрос exposed-port в k8s открыт;
  - R10: per-call-type proxy заменен system proxy процесса;
  - R2: SetExclusiveProvider("db-apikey") — inline cfg.APIKeys не нужны;
  - R6.1: SessionAffinity отключён (конфликтует с stateless multi-replica);
  - R9.A.5: отмечено как открытое (Execute тратит квоту);
  - ADR-9: добавлены поля watcher.AuthUpdate, опциональный CooldownStateStore;
  - architecture.md: исправлен вызов RegisterTokenStore в wiring-диаграмме.
- 2026-07-12 — **детализация R9.A.1 и R9.A.5** (после анализа реальной механики
  `sdkAuth.Manager.Login` и `ProviderExecutor`):
  - R9.A.1: `Manager.Login` блокирующий, не подходит для API. Своя асинхронная
    реализация поверх низкоуровневых auth-сервисов ядра, сессии в Postgres
    (`oauth_sessions`) для multi-replica. Callback-провайдеры (Codex/Claude/
    Antigravity) — PKCE + ручной copy-paste redirect_url; device-провайдеры
    (Kimi/xAI) — device-code + polling.
  - R9.A.5: своя обёртка Refresh (OAuth, без траты квоты) + HttpRequest к
    metadata-endpoints (API-key). Execute/CountTokens не используются.
  - Новая таблица `oauth_sessions` в БД; дизайн в docs/design/.
  - См. [docs/design/r9-oauth-and-testing.md](design/r9-oauth-and-testing.md).
- 2026-07-12 — **сравнение R9.A.1 с CLIProxyAPIBusiness** подтвердило решения:
  референс переиспользует готовые handlers ядра, но OAuth-сессии in-memory +
  файл callback → multi-replica не работает (сознательное ограничение).
  Наш подход (своя реализация + Postgres-сессии) обоснован требованием R6.2.
  R5 (AES-GCM) остаётся строже референса (plaintext) — подтверждено. Сравнение
  зафиксировано в docs/design/r9-oauth-and-testing.md.
- 2026-07-12 — **верификация дизайна** (сверка architecture/design/schema с
  SDK-референсом). Исправлены критические неточности:
  - **LDAP bind-password:** убран из класса AES-шифруемых секретов R5 — он
    только в env (k8s Secret), не в БД. AES-GCM применяется только к upstream-
    credentials в БД. architecture.md синхронизирован.
  - **`watcher.AuthUpdate`:** исправлен тип в architecture.md (был ошибочный
    `coreauth.AuthUpdate`).
  - **R3 principal механизм:** уточнён — `Record.APIKey` ядро заполняет из
    `access.Result.Principal`; `api_key_id` прокидывается через
    `executor.Options.Metadata`. Заявление «principal копируется в Record»
    заменено на корректное описание.
  - **`SetExclusiveProvider("db-apikey")`:** добавлен в architecture.md
    (раньше был только в requirements.md).
  - **Новые компоненты** в architecture.md: `internal/auth/oauth` (R9.A.1),
    `internal/auth/testing` (R9.A.5).
  - R9.A.5: уточнено про quota для свежих аккаунтов (`unknown` vs `exceeded=false`).
- 2026-07-12 — **R11: OpenAPI/Swagger-спецификация.** Spec-first (OpenAPI 3.1,
  код генерируется из спецификации). Покрытие — все роуты (management с полными
  схемами; прокси `/v1/*` без body-схем; системные). Доступ: `/openapi.json` +
  опц. `/docs`. CI lint + drift-check.
- 2026-07-14 — **R1.5:** добавлен static identity source для development/test:
  явный `auth.mode`, secrets только из env, namespace `static:`, source-gating
  session/API-key и запрет rolling-переключения с LDAP.
- 2026-07-14 — **R12:** добавлен compatibility gate для обновления внешнего
  SDK: публичные `sdk/*` контракты, фиксированная версия, contract/integration
  проверки и отдельный ADR для нового major.
