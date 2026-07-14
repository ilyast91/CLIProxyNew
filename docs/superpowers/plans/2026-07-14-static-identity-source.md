# Static Identity Source Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Добавить безопасный static identity source для development/test, не меняя LDAP как единственный production source.

**Architecture:** HTTP login использует внутренний `IdentityProvider`, выбранный конфигом. Static identity хранится в namespace `static:<username>` и маркируется `identity_source`; middleware сессий и `access.Provider` принимают её только в static mode. Миграция добавляет source совместимо с существующими LDAP-записями.

**Tech Stack:** Go 1.26, pgx/v5, sqlc, golang-migrate, bcrypt, LDAP, testcontainers PostgreSQL.

---

### Task 1: Зафиксировать документационный контракт

**Files:**
- Modify: `docs/requirements.md`
- Modify: `docs/architecture.md`
- Modify: `docs/architecture-principles.md`
- Modify: `docs/database-schema.md`
- Modify: `docs/implementation-phases.md`
- Modify: `AGENTS.md`
- Modify: `config.example.yaml`
- Modify: `openapi.yaml`

- [x] **Step 1: Описать R1/R2/R6 и конфигурационный контракт**

Зафиксировать `server.environment: development | test | production`,
`auth.mode: ldap | static`, default `production`/`ldap`, а также запрет static
в production. Описать env-переменные `CLIPROXY_STATIC_USER_USERNAME`,
`CLIPROXY_STATIC_USER_PASSWORD`, `CLIPROXY_STATIC_USER_ROLE` как dev/test
secrets.

- [x] **Step 2: Описать изоляцию identity и rollout**

В schema и architecture закрепить `identity_source`, формат static username
`static:<username>`, DB CHECK и отказ LDAP provider для этого prefix. Описать
expand-миграцию с default `ldap`, non-rolling переключение mode и guarded
down-миграцию при static history.

- [x] **Step 3: Синхронизировать OpenAPI, agent guidance и фазы**

Заменить формулировку "LDAP-логин" на "login через настроенный identity
source" там, где это описание API, не меняя путь и схему cookie. Добавить
задачи Ф2/Ф7 и исправить AGENTS: LDAP bind-password остаётся только env secret,
не AES-данные.

- [x] **Step 4: Проверить документацию**

Run: `git diff --check`

Expected: exit code 0.

### Task 2: Добавить конфигурацию и валидацию режима

**Files:**
- Modify: `internal/config/config.go`
- Modify: `internal/config/config_test.go`

- [x] **Step 1: Написать падающие тесты конфигурации**

Покрыть default `production`/`ldap`, допустимый static только в
development/test, обязательные static env credentials и роль `user|admin`.

```go
func TestValidateRejectsStaticModeInProduction(t *testing.T) {
    cfg := Default()
    cfg.Server.Environment = "production"
    cfg.Auth.Mode = "static"
    if err := cfg.Validate(); err == nil {
        t.Fatal("Validate() accepted static mode in production")
    }
}
```

- [x] **Step 2: Запустить тесты до реализации**

Run: `go test ./internal/config -run TestValidateRejectsStaticModeInProduction -count=1`

Expected: FAIL because `AuthConfig` and validation do not exist.

- [x] **Step 3: Реализовать `Server.Environment`, `AuthConfig` и validation**

Добавить YAML-поля `server.environment`, `auth.mode`; читать static credentials
только из env. `Validate` должен вернуть ошибку до запуска HTTP-сервера, если
static mode активен в production либо не задано обязательное значение.

- [x] **Step 4: Повторить тесты конфигурации**

Run: `go test ./internal/config -count=1`

Expected: PASS.

### Task 3: Ввести identity provider и persistence source

**Files:**
- Create: `internal/auth/identity/provider.go`
- Create: `internal/auth/identity/static.go`
- Create: `internal/auth/identity/static_test.go`
- Create: `internal/auth/ldap/provider.go`
- Create: `db/migrations/20260714000100_users_identity_source.up.sql`
- Create: `db/migrations/20260714000100_users_identity_source.down.sql`
- Modify: `db/queries/users.sql`
- Modify: `internal/store/dbgen/*` (только через `sqlc generate`)
- Modify: `internal/store/users.go`
- Modify: `internal/store/repositories_integration_test.go`

- [x] **Step 1: Написать тест static provider**

Проверить правильный password, отказ при неверном password, роль и internal
username `static:<username>`.

```go
func TestStaticProviderAuthenticate(t *testing.T) {
    provider := NewStaticProvider("debug", "secret", "admin")
    identity, err := provider.Authenticate(context.Background(), "debug", "secret")
    if err != nil || identity.Username != "static:debug" || identity.Source != "static" {
        t.Fatalf("identity = %#v, err = %v", identity, err)
    }
}
```

- [x] **Step 2: Запустить unit-тест до реализации**

Run: `go test ./internal/auth/identity -run TestStaticProviderAuthenticate -count=1`

Expected: FAIL because package does not exist.

- [x] **Step 3: Реализовать static provider и миграцию**

Контракт `IdentityProvider` возвращает `Username`, `Email`, `Role`, `Source`.
Миграция добавляет `identity_source text NOT NULL DEFAULT 'ldap'` с CHECK,
сохраняет unique username и добавляет CHECK namespace:

```sql
CHECK (
  (identity_source = 'static' AND username LIKE 'static:%') OR
  (identity_source = 'ldap' AND username NOT LIKE 'static:%')
)
```

Down должен отказать, если остаются static users; на пустой БД выполнить
обычный rollback.

- [x] **Step 4: Сгенерировать sqlc и выполнить тесты persistence**

Run: `go run github.com/sqlc-dev/sqlc/cmd/sqlc@v1.31.1 generate`

Expected: exit code 0.

Run: `go test -count=1 ./internal/store ./db/migrations`

Expected: PASS.

### Task 4: Подключить identity source к login, sessions и API-keys

**Files:**
- Create: `internal/httpapi/login.go`
- Create: `internal/httpapi/login_test.go`
- Create: `internal/access/provider.go`
- Create: `internal/access/provider_test.go`
- Modify: `internal/store/sessions.go`
- Modify: `internal/store/api_keys.go`

- [x] **Step 1: Написать тесты изоляции source в persistence**

Проверить, что static session/API-key отвергаются в LDAP mode, даже когда
`users.status = active`; LDAP credentials не могут быть provisioned с prefix
`static:`.

- [ ] **Step 2: Реализовать source match на каждом пути авторизации**

Login выбирает provider один раз при wiring. Session middleware и
`access.Provider` получают текущий expected source и сравнивают его с
`users.identity_source` после lookup пользователя, до выдачи principal.

- [ ] **Step 3: Запустить тесты**

Run: `go test -count=1 ./internal/access ./internal/httpapi ./internal/store`

Expected: PASS.

### Task 5: Интеграция и финальная проверка

**Files:**
- Modify: `docs/implementation-phases.md` (отметить выполненные пункты только после реализации)

- [ ] **Step 1: Добавить integration-сценарий mode isolation**

Поднять PostgreSQL, provision static user, выпустить session/API-key и
проверить, что LDAP mode отклоняет оба credential по `identity_source`.

- [ ] **Step 2: Выполнить полный набор проверок**

Run: `go test -count=1 ./...`

Expected: PASS.

Run: `go test -race -count=1 ./internal/access ./internal/auth/... ./internal/store`

Expected: PASS.

Run: `go vet ./...`

Expected: exit code 0.

Run: `go build ./...`

Expected: exit code 0.

- [ ] **Step 3: Commit**

```bash
git add docs internal db
git commit -m "feat(auth): добавить static identity source"
```
