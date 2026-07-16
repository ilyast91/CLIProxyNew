# Phase 7 Load/SLA Gate Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Добавить verified API-key cache и обязательный E2E SLA regression gate для Ф7.

**Architecture:** Успешный bcrypt результат вместе с проверенным `users.status` кэшируется по SHA-256 отпечатку API-key на 10с. Локальная блокировка инвалидирует cache немедленно, остальные реплики сходятся в пределах TTL; non-race E2E gate измеряет p95 дочерних access/selector spans и verified-cache ratio на production-like runtime с реальным PostgreSQL и fake upstream.

**Tech Stack:** Go 1.26, pgx/v5, Gin, Prometheus, testcontainers-go, GitHub Actions.

---

### Task 1: Зафиксировать failing SLA и invalidation tests

**Files:**
- Modify: `internal/e2e/runtime_test.go`
- Create: `internal/e2e/sla_test.go`
- Modify: `internal/httpapi/admin_users_test.go`
- Modify: `internal/store/repositories_integration_test.go`

- [x] **Step 1: Добавить SLA E2E test**

Добавить build constraint `!race`, 200 запросов через 4 worker и проверки
суммарного p95 access/selector `≤5мс`, cache hit ratio `0.95` и нулевого error
count.

- [x] **Step 2: Добавить тест нескольких invalidator**

Передать в `NewAdminUserHandler` два `fakeSessionInvalidator` и потребовать, чтобы
оба получили target user ID после успешного status update.

- [x] **Step 3: Проверить RED**

Run:

```bash
go test -count=1 -run '^TestIntegrationRuntimeSLA$' ./internal/e2e -v
go test -count=1 -run '^TestAdminUserStatusInvalidatesAllCaches$' ./internal/httpapi
```

Expected: SLA fails на p95 bucket; admin invalidation test сообщает, что второй
invalidator не вызван.

### Task 2: Реализовать verified API-key cache

**Files:**
- Modify: `internal/store/api_keys.go`
- Modify: `internal/store/api_keys_cache_test.go`
- Modify: `internal/store/repositories_integration_test.go`

- [x] **Step 1: Добавить cache key/value**

Использовать `sha256.Sum256([]byte(plaintext))` вместе с identity source и
`cache.TTL[verifiedAPIKeyCacheKey, APIKeyPrincipal]` с TTL 10с.

- [x] **Step 2: Применить fast path**

На verified hit вернуть principal без bcrypt и PostgreSQL round-trip. Проверка
`users.status` выполняется при cache fill, а status mutation вызывает локальную
invalidation; межрепличная согласованность ограничена TTL согласно R2.4.

- [x] **Step 3: Заполнять только после успешной проверки**

После bcrypt compare и live user status записывать principal в verified cache.
Ошибки и неверные credentials не кэшировать.

- [x] **Step 4: Добавить invalidation**

`Revoke` очищает оба cache. `InvalidateUser(userID)` удаляет verified entries
целевого пользователя. `CacheStats()` возвращает verified-cache stats.

- [x] **Step 5: Проверить store tests**

Run:

```bash
go test -count=1 ./internal/store
```

Expected: PASS.

### Task 3: Подключить invalidation и метрики

**Files:**
- Modify: `internal/httpapi/admin_users.go`
- Modify: `internal/httpapi/admin_users_test.go`
- Modify: `internal/metrics/registry.go`
- Modify: `internal/metrics/registry_test.go`
- Modify: `cmd/cliproxy/main.go`
- Modify: `internal/e2e/runtime_test.go`

- [x] **Step 1: Поддержать несколько invalidator**

Хранить slice всех non-nil `InvalidateUser(int64)` и вызывать каждый после
успешного status update.

- [x] **Step 2: Подключить API-key invalidator**

В production wiring передать `sessionAuthenticator` и `apiKeyStore` в
`NewAdminUserHandler`.

- [x] **Step 3: Уточнить cache label**

Экспортировать verified stats с label `cache="api_key_auth"` и обновить тест.

- [x] **Step 4: Проверить GREEN**

Run:

```bash
go test -count=1 ./internal/httpapi ./internal/metrics
go test -count=1 -run '^TestIntegrationRuntimeSLA$' ./internal/e2e -v
```

Expected: PASS; SLA выводит p95 bucket ratio и cache hit ratio не ниже 0.95.

### Task 4: Добавить CI gate и актуализировать фазы

**Files:**
- Modify: `.github/workflows/ci.yml`
- Modify: `docs/architecture-principles.md`
- Modify: `docs/architecture.md`
- Modify: `docs/implementation-phases.md`
- Modify: `docs/requirements.md`

- [x] **Step 1: Добавить job**

`Load/SLA regression` запускает
`go test -count=1 -run '^TestIntegrationRuntimeSLA$' -timeout 10m ./internal/e2e -v`
и включается в `build.needs`.

- [x] **Step 2: Зафиксировать профиль нагрузки**

В architecture principles заменить ранее неопределённый load target на 4 worker и 200
steady-state запросов с fake upstream; production capacity planning остаётся
отдельной эксплуатационной задачей.

- [x] **Step 3: Закрыть пункты Ф7**

Отметить load/SLA и SLA regression CI gate выполненными; удалить их из списка
работ без внешних блокеров. Chaos и runbooks оставить открытыми.

### Task 5: Полная проверка и коммит

**Files:** все изменённые файлы текущего плана.

- [x] **Step 1: Форматирование и drift checks**

Run:

```bash
gofmt -w cmd/cliproxy/main.go internal/store internal/httpapi internal/metrics internal/e2e
git diff --check
```

Expected: exit code 0.

- [x] **Step 2: Полная проверка**

Run:

```bash
go vet ./...
go build ./...
go test -count=1 -timeout 15m ./...
go test -race -count=1 -timeout 15m ./...
```

Expected: все команды exit code 0.

- [x] **Step 3: Коммит**

```bash
git add .github/workflows/ci.yml cmd/cliproxy/main.go internal/store internal/httpapi internal/metrics internal/e2e docs/architecture-principles.md docs/architecture.md docs/implementation-phases.md docs/requirements.md docs/superpowers/specs/2026-07-16-phase7-sla-gate-design.md docs/superpowers/plans/2026-07-16-phase7-sla-gate.md
git commit -m "test: добавить SLA regression gate"
```
