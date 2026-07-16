# Access and Selector Tracing Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Добавить безопасные дочерние OpenTelemetry spans для клиентской аутентификации и выбора upstream-аккаунта.

**Architecture:** Существующие реализации `access.Provider` и `selector.Selector` начинают internal span непосредственно на границе публичного SDK-контракта. Span наследует входной context, записывает только безопасные атрибуты результата и завершает статусом error при отказе; HTTP headers, query, API-key и содержимое credentials никогда не добавляются.

**Tech Stack:** Go 1.26, OpenTelemetry Go 1.44, стандартный `testing`, `tracetest`.

---

### Task 1: Trace `access.Provider.Authenticate`

**Files:**
- Modify: `internal/access/provider_test.go`
- Modify: `internal/access/provider.go`

- [x] **Step 1: Write the failing success-span test**

Добавить тест с `tracetest.SpanRecorder`, parent span context и реальным вызовом `Provider.Authenticate`. Проверить имя `access.Provider.Authenticate`, parent span, атрибуты `auth.provider`, `auth.identity_source`, `auth.outcome`, `user.id`, `api_key.id` и отсутствие значений API-key среди атрибутов.

- [x] **Step 2: Run the test and verify RED**

Run: `go test ./internal/access -run TestProviderAuthenticateCreatesSafeChildSpan -count=1`

Expected: FAIL, потому что `Provider.Authenticate` ещё не создаёт span.

- [x] **Step 3: Implement minimal access tracing**

В `Authenticate` запустить internal span через package-local tracer. На success записать provider/source/outcome и числовые IDs; для no credentials, invalid credentials и repository failure записать outcome и стабильный `SetStatus(codes.Error, ...)`. Не вызывать `RecordError` для boundary-errors: исходный текст ошибки может содержать credential payload.

- [x] **Step 4: Run access tests**

Run: `go test ./internal/access -count=1`

Expected: PASS.

### Task 2: Trace `selector.Selector.Pick`

**Files:**
- Modify: `internal/auth/selector/selector_test.go`
- Modify: `internal/auth/selector/selector.go`

- [x] **Step 1: Write the failing selector-span tests**

Добавить success test с parent span и error test для disabled model. Проверить имя `selector.Pick`, parent context, safe attributes `provider`, `model`, `candidate.count`, `auth.id`, `outcome` и error status. Не добавлять attributes из `Auth.Storage`, `Auth.Metadata` или credential payload.

- [x] **Step 2: Run the tests and verify RED**

Run: `go test ./internal/auth/selector -run 'TestSelectorPickCreatesSafeChildSpan|TestSelectorPickMarksSpanError' -count=1`

Expected: FAIL, потому что `Selector.Pick` ещё не создаёт span.

- [x] **Step 3: Implement minimal selector tracing**

Запустить internal span в `Pick`, записать requested provider/model и число входных кандидатов. При успешном выборе добавить selected auth ID/provider и outcome `success`; при ошибках allow-list, reload или fallback записать стабильный error status и outcome `error`, не экспортируя исходный текст ошибки через span events.

- [x] **Step 4: Run selector tests**

Run: `go test ./internal/auth/selector -count=1`

Expected: PASS.

### Task 3: Update project status and verify

**Files:**
- Modify: `docs/implementation-phases.md`

- [x] **Step 1: Update Phase 6 status**

Отметить spans для `access.Provider` и `Selector` выполненными, сохранив SDK Execute как отдельный незакрытый пункт из-за SDK boundary.

- [x] **Step 2: Format and run focused verification**

Run: `gofmt -w internal/access/provider.go internal/access/provider_test.go internal/auth/selector/selector.go internal/auth/selector/selector_test.go`

Run: `go test ./internal/access ./internal/auth/selector -count=1`

Expected: PASS.

- [x] **Step 3: Run repository verification**

Run: `go test -short ./...`

Run: `go vet ./...`

Run: `go build ./...`

Expected: все команды завершаются с exit code 0. Если sandbox блокирует глобальный Go cache, повторить важную команду с разрешением пользователя.

- [x] **Step 4: Inspect and commit**

Run: `git diff --check`

Run: `git status --short`

Commit message: `feat(observability): трассировать access и selector`
