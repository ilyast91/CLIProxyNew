# OpenAPI Route Parity and Release Gates Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Полностью описать proxy HTTP surface CLIProxyAPI v7.2.80 и сделать E2E, ADR-9 contracts, integration, coverage, security и race обязательными release gates.

**Architecture:** `openapi.yaml` остаётся spec-first контрактом без upstream payload schemas; route matrix защищается тестом embedded JSON. Hardening реализуется тестовыми пакетами и небольшими shell entrypoints, а GitHub Actions запускает независимые gates до build. E2E собирает production-like graph через публичный SDK и реальный PostgreSQL testcontainer, заменяя только внешний provider executor.

**Tech Stack:** Go 1.26.5, CLIProxyAPI SDK v7.2.80, OpenAPI 3.1, ogen v1.23.0, Gin, pgx/v5, testcontainers-go, GitHub Actions, govulncheck, POSIX shell.

---

## Карта файлов

- `openapi.yaml` — полный перечень proxy URL/method без body schemas.
- `internal/openapi/document_test.go` — route/method/security/path-parameter contract.
- `internal/openapi/openapi.json` — generated embedded document.
- `internal/openapi/ogen/openapi.compat.yaml` и `internal/openapi/ogen/oas_*_gen.go` — generated ogen projection/bindings.
- `scripts/verify-adr9-contracts.sh` — видимый behavioral contract gate по семи ролям ADR-9.
- `internal/e2e/runtime_test.go` — PostgreSQL + SDK runtime E2E.
- `scripts/coverage.sh` — aggregate handwritten `internal/*` coverage gate 70.0%.
- `scripts/security-audit.sh` — repository policy scan без чтения секретов окружения.
- `.github/workflows/ci.yml` — независимые hardening jobs и build dependencies.
- `docs/implementation-phases.md` — фактический прогресс и оставшийся late scope.

### Task 1: Защитить и реализовать полный OpenAPI proxy surface

**Files:**
- Modify: `internal/openapi/document_test.go`
- Modify: `openapi.yaml`
- Regenerate: `internal/openapi/openapi.json`
- Regenerate: `internal/openapi/ogen/openapi.compat.yaml`
- Regenerate: `internal/openapi/ogen/oas_*_gen.go`

- [ ] **Step 1: Заменить сокращённый proxy test полной матрицей**

В `internal/openapi/document_test.go` определить:

```go
type proxyOperation struct {
	Method         string
	Path           string
	PathParameters []string
}

var proxyOperations = []proxyOperation{
	{Method: "get", Path: "/v1/models"},
	{Method: "post", Path: "/v1/chat/completions"},
	{Method: "post", Path: "/v1/completions"},
	{Method: "post", Path: "/v1/images/generations"},
	{Method: "post", Path: "/v1/images/edits"},
	{Method: "post", Path: "/v1/videos"},
	{Method: "post", Path: "/v1/videos/generations"},
	{Method: "post", Path: "/v1/videos/edits"},
	{Method: "post", Path: "/v1/videos/extensions"},
	{Method: "get", Path: "/v1/videos/{request_id}", PathParameters: []string{"request_id"}},
	{Method: "post", Path: "/v1/messages"},
	{Method: "post", Path: "/v1/messages/count_tokens"},
	{Method: "get", Path: "/v1/responses"},
	{Method: "post", Path: "/v1/responses"},
	{Method: "post", Path: "/v1/responses/compact"},
	{Method: "post", Path: "/v1/alpha/search"},
	{Method: "post", Path: "/openai/v1/videos"},
	{Method: "get", Path: "/openai/v1/videos/{video_id}/content", PathParameters: []string{"video_id"}},
	{Method: "get", Path: "/openai/v1/videos/{video_id}", PathParameters: []string{"video_id"}},
	{Method: "get", Path: "/backend-api/codex/responses"},
	{Method: "post", Path: "/backend-api/codex/responses"},
	{Method: "post", Path: "/backend-api/codex/responses/compact"},
	{Method: "post", Path: "/backend-api/codex/alpha/search"},
	{Method: "get", Path: "/v1beta/models"},
	{Method: "post", Path: "/v1beta/interactions"},
	{Method: "get", Path: "/v1beta/models/{model}:{action}", PathParameters: []string{"model", "action"}},
	{Method: "post", Path: "/v1beta/models/{model}:{action}", PathParameters: []string{"model", "action"}},
}
```

Operation test должен unmarshal `security` и `parameters`, проверить все 27
пар, `BearerApiKey`, обязательные path parameters и отсутствие
`/v1/models/{model}:generateContent`.

- [ ] **Step 2: Запустить test и подтвердить RED**

Run:

```bash
go test ./internal/openapi -run TestDocumentDescribesProxy -count=1
```

Expected: FAIL на первом отсутствующем route, например `/v1/completions`.

- [ ] **Step 3: Расширить proxy section `openapi.yaml`**

Для каждой матричной operation использовать единый шаблон без requestBody:

```yaml
  /v1/completions:
    post:
      tags: [Proxy]
      summary: OpenAI-compatible completion
      description: Тело и ответ прозрачно обрабатываются upstream SDK; бизнес-слой описывает только URL, auth и общие ошибки.
      operationId: proxyCompletions
      security:
        - BearerApiKey: []
      responses:
        "200":
          description: Upstream-compatible response
        "400":
          $ref: "#/components/responses/BadRequest"
        "401":
          $ref: "#/components/responses/Unauthorized"
        "500":
          $ref: "#/components/responses/InternalError"
```

Retrieval routes дополнить `404`; GET responses routes описать как websocket
handshake. Удалить `/v1/models/{model}:generateContent`, добавить
`/v1beta/models/{model}:{action}` с GET и POST и обоими path parameters.

- [ ] **Step 4: Перегенерировать производные файлы**

Run:

```bash
go generate ./internal/openapi/...
```

Expected: обновлены embedded JSON, compatibility YAML и ogen bindings только
через генераторы.

- [ ] **Step 5: Проверить GREEN и lint**

Run:

```bash
go test ./internal/openapi ./cmd/openapiogen
spectral lint -r .spectral.yaml openapi.yaml
git diff --check
```

Expected: PASS; Spectral сообщает 0 errors.

- [ ] **Step 6: Commit**

```bash
git add openapi.yaml internal/openapi/document_test.go internal/openapi/openapi.json internal/openapi/ogen
git commit -m "feat(openapi): описать полный proxy surface SDK"
```

### Task 2: Сделать behavioral ADR-9 gate явным

**Files:**
- Create: `scripts/verify-adr9-contracts.sh`

- [ ] **Step 1: Создать исполняемый contract entrypoint**

```sh
#!/bin/sh
set -eu

go test -race ./internal/sdkcontract
go test -race -run 'TestProviderAuthenticatesBearerTokenForActiveSource|TestProviderRejectsMissingAndInvalidCredentials' ./internal/access
go test -race -run 'TestSelectorPickUsesEnabledOverrideProvider|TestSelectorFailsClosedWhenExpiredCacheCannotBeReloaded' ./internal/auth/selector
go test -race -run 'TestHookCountsAuthLifecycleAndResults|TestPluginWritesUsageEventFromVersionedPrincipal' ./internal/usage
go test -race -run 'TestNoopFactoryReturnsUsableWatcherWrapper|TestRevisionPollerCallsShutdownAfterRevisionChange' ./internal/watcher
go test -race -run 'TestHookStoresCompleteModelSnapshot|TestHookDeletesSnapshotWhenModelsAreUnregistered' ./internal/modelregistry
go test -race -run 'TestRouterConfiguratorEnforcesManagementSessionAndRole|TestSystemRouterConfiguratorServesLivenessWithoutDatabase' ./internal/httpapi
go test -race -run '^TestIntegrationCoreAuthStoreContract$' ./internal/store
```

Этот список соответствует семи строкам ADR-9; Store command требует Docker и
поэтому job не использует `-short`.

- [ ] **Step 2: Запустить script**

Run:

```bash
chmod +x scripts/verify-adr9-contracts.sh
./scripts/verify-adr9-contracts.sh
```

Expected: все команды PASS, включая PostgreSQL contract.

- [ ] **Step 3: Commit**

```bash
git add scripts/verify-adr9-contracts.sh
git commit -m "test(sdk): добавить behavioral gate ADR-9"
```

### Task 3: Добавить настоящий SDK runtime E2E

**Files:**
- Create: `internal/e2e/runtime_test.go`

- [ ] **Step 1: Создать RED test сценария**

Определить `TestIntegrationRuntimeLoginKeyInferenceUsageAdmin`. Test harness
должен иметь следующие public-SDK fake methods:

```go
type fakeExecutor struct{}

func (fakeExecutor) Identifier() string { return "openai" }
func (fakeExecutor) Execute(context.Context, *coreauth.Auth, executor.Request, executor.Options) (executor.Response, error) {
	return executor.Response{Payload: []byte(`{"id":"chatcmpl-e2e","object":"chat.completion","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":5,"completion_tokens":7,"total_tokens":12}}`)}, nil
}
func (fakeExecutor) ExecuteStream(context.Context, *coreauth.Auth, executor.Request, executor.Options) (*executor.StreamResult, error) {
	return nil, errors.New("stream is not used by E2E")
}
func (fakeExecutor) Refresh(_ context.Context, auth *coreauth.Auth) (*coreauth.Auth, error) { return auth, nil }
func (fakeExecutor) CountTokens(context.Context, *coreauth.Auth, executor.Request, executor.Options) (executor.Response, error) {
	return executor.Response{Payload: []byte(`{"input_tokens":5}`)}, nil
}
func (fakeExecutor) HttpRequest(context.Context, *coreauth.Auth, *http.Request) (*http.Response, error) {
	return nil, errors.New("metadata request is not used by E2E")
}
```

Harness steps:

```go
func TestIntegrationRuntimeLoginKeyInferenceUsageAdmin(t *testing.T) {
	if testing.Short() { t.Skip("integration E2E requires Docker") }
	// PostgreSQL container + migrations.
	// Static admin identity, repositories, keyring, CoreAuthStore.
	// Register db access provider exclusively and clean global registry.
	// Register one openai Auth and fakeExecutor in public coreauth.Manager.
	// Build SDK Service with management RouterConfigurator and buffered usage.
	// Start Service.Run, wait for /healthz, execute HTTP scenario, shutdown.
}
```

Assertions обязаны проверять status/body/cookie, prefix-only persistence metadata,
fake inference payload, `request_count=1`, `total_tokens=12`, model bucket и
admin visibility созданного user/key.

- [ ] **Step 2: Запустить E2E и подтвердить RED**

Run:

```bash
go test -run '^TestIntegrationRuntimeLoginKeyInferenceUsageAdmin$' ./internal/e2e -count=1 -v
```

Expected: FAIL до завершения harness или wiring.

- [ ] **Step 3: Реализовать PostgreSQL и HTTP helpers внутри `_test.go`**

Использовать pinned image:

```go
const postgresImage = "postgres@sha256:742f40ea20b9ff2ff31db5458d127452988a2164df9e17441e191f3b72252193"
```

Helpers должны:

- запускать container и применять `db/migrations` через golang-migrate;
- резервировать loopback port через `net.Listen("127.0.0.1:0")`, закрыть
  listener и передать адрес в `config.SDKConfig`;
- отправлять JSON requests через `http.Client` с cookie jar;
- ждать health endpoint с deadline 10 секунд и интервалом не более 50мс;
- закрывать usage plugin до analytics query или ждать запись bounded polling;
- отменять run context и вызывать `Service.Shutdown` с timeout.

- [ ] **Step 4: Реализовать production-like wiring**

Создать те же компоненты, что в `cmd/cliproxy/main.go`: `UserRepository`,
`SessionRepository`, cached session authenticator, `APIKeyRepository`,
`CoreAuthStore`, selector, usage hook/plugin, SDK access provider и полный
management `RouterConfigurator`. Не запускать leader jobs и model registry:
они не участвуют в проверяемом HTTP data flow.

- [ ] **Step 5: Проверить GREEN и process isolation**

Run:

```bash
go test -run '^TestIntegrationRuntimeLoginKeyInferenceUsageAdmin$' ./internal/e2e -count=1 -v
go test -short ./...
```

Expected: E2E PASS; short suite запускается отдельным test process и PASS.
Внутри `internal/e2e` остаётся ровно один runtime test, потому что публичный SDK
останавливает process-global usage manager при `Service.Shutdown` без restart API.

- [ ] **Step 6: Commit**

```bash
git add internal/e2e/runtime_test.go
git commit -m "test(e2e): проверить runtime от login до analytics"
```

### Task 4: Добавить aggregate coverage gate

**Files:**
- Create: `scripts/coverage.sh`

- [ ] **Step 1: Создать фиксированный gate 70.0%**

```sh
#!/bin/sh
set -eu

profile=${1:-coverage.out}
packages=$(go list ./internal/... | grep -Ev '/internal/(openapi/ogen|store/dbgen)$' | paste -sd, -)

go test -covermode=atomic -coverpkg="$packages" -coverprofile="$profile" ./internal/...
total=$(go tool cover -func="$profile" | awk '/^total:/ {gsub("%", "", $3); print $3}')
awk -v total="$total" 'BEGIN { if (total + 0 < 70.0) { printf "aggregate coverage %.1f%% is below 70.0%%\n", total; exit 1 } }'
printf 'aggregate coverage %s%% meets 70.0%% gate\n' "$total"
```

Порог не читается из environment. Baseline до изменений измерен как 73.7% с
integration tests и исключёнными generated packages.

- [ ] **Step 2: Проверить gate и profile**

Run:

```bash
chmod +x scripts/coverage.sh
./scripts/coverage.sh /tmp/cliproxy-coverage.out
go tool cover -func=/tmp/cliproxy-coverage.out | tail -1
```

Expected: PASS и total не ниже 70.0%; generated paths отсутствуют в profile.

- [ ] **Step 3: Commit**

```bash
git add scripts/coverage.sh
git commit -m "ci: добавить aggregate coverage gate"
```

### Task 5: Добавить security audit

**Files:**
- Create: `scripts/security-audit.sh`

- [ ] **Step 1: Создать source policy scan**

```sh
#!/bin/sh
set -eu

fail() {
  printf '%s\n' "$1" >&2
  exit 1
}

tracked=$(git ls-files)

printf '%s\n' "$tracked" | grep -E '(^|/)(\.env|[^/]+\.(pem|key|p12|pfx))$' && fail 'tracked secret/key artifact detected'
printf '%s\n' "$tracked" | grep -E '(^|/)cliproxy$' && fail 'tracked binary artifact detected'

if git grep -nE '-----BEGIN (RSA |EC |OPENSSH )?PRIVATE KEY-----' -- . ':!scripts/security-audit.sh'; then
  fail 'private key material detected'
fi

if git grep -nE '\b(fmt|log)\.Print(f|ln)?\(' -- ':(glob)cmd/**/*.go' ':(glob)internal/**/*.go' ':(exclude,glob)**/*_test.go'; then
  fail 'unstructured runtime printing detected'
fi

if git grep -nE 'slog\.(Debug|Info|Warn|Error)\([^\n]*(os\.Getenv|StaticPassword|BindPassword|Credentials|RefreshToken|AccessToken|APIKey)' -- ':(glob)cmd/**/*.go' ':(glob)internal/**/*.go' ':(exclude,glob)**/*_test.go'; then
  fail 'potential sensitive slog payload detected'
fi

printf 'security source audit passed\n'
```

Тестовые redaction fixtures остаются разрешены, потому что scan логирования
ограничен runtime calls и не запрещает безопасные ключи attributes, которые
глобальный handler редактирует.

- [ ] **Step 2: Запустить audit и известные security tests**

Run:

```bash
chmod +x scripts/security-audit.sh
./scripts/security-audit.sh
go test ./internal/observability ./internal/security ./internal/access
go run golang.org/x/vuln/cmd/govulncheck@v1.1.4 ./...
```

Expected: policy scan и tests PASS; govulncheck не находит достижимых
уязвимостей.

- [ ] **Step 3: Commit**

```bash
git add scripts/security-audit.sh
git commit -m "ci: добавить security audit gate"
```

### Task 6: Перестроить CI до build

**Files:**
- Modify: `.github/workflows/ci.yml`

- [ ] **Step 1: Добавить независимые jobs**

После `static-checks` определить jobs:

```yaml
  unit-contract:
    needs: static-checks
    steps:
      - uses: actions/checkout@v7
      - uses: actions/setup-go@v7
        with:
          go-version: ${{ env.GO_VERSION }}
          cache: true
      - run: go test -short -race -timeout 5m ./...
      - run: ./scripts/verify-adr9-contracts.sh
      - run: |
          go mod tidy
          git diff --exit-code go.mod go.sum

  integration:
    needs: static-checks
    steps:
      - uses: actions/checkout@v7
      - uses: actions/setup-go@v7
        with:
          go-version: ${{ env.GO_VERSION }}
          cache: true
      - run: go test -run Integration -timeout 10m ./internal/store ./internal/watcher

  e2e:
    needs: static-checks
    steps:
      - uses: actions/checkout@v7
      - uses: actions/setup-go@v7
        with:
          go-version: ${{ env.GO_VERSION }}
          cache: true
      - run: go test -run '^TestIntegrationRuntime' -timeout 10m ./internal/e2e -v

  coverage:
    needs: static-checks
    steps:
      - uses: actions/checkout@v7
      - uses: actions/setup-go@v7
        with:
          go-version: ${{ env.GO_VERSION }}
          cache: true
      - run: ./scripts/coverage.sh coverage.out

  security:
    needs: static-checks
    steps:
      - uses: actions/checkout@v7
      - uses: actions/setup-go@v7
        with:
          go-version: ${{ env.GO_VERSION }}
          cache: true
      - run: ./scripts/security-audit.sh
      - run: go run golang.org/x/vuln/cmd/govulncheck@v1.1.4 ./...

  full-race:
    needs: static-checks
    steps:
      - uses: actions/checkout@v7
      - uses: actions/setup-go@v7
        with:
          go-version: ${{ env.GO_VERSION }}
          cache: true
      - run: go test -race -timeout 15m ./...
```

`openapi-lint` сохраняет Spectral/generation drift и дополнительно запускает
`go test ./internal/openapi`.

- [ ] **Step 2: Перенести build после всех gates**

```yaml
  build:
    needs:
      - unit-contract
      - integration
      - e2e
      - coverage
      - security
      - full-race
      - openapi-lint
```

Удалить старый `test` job и зависимость test от build.

- [ ] **Step 3: Проверить YAML и локальные entrypoints**

Run:

```bash
git diff --check
./scripts/verify-adr9-contracts.sh
./scripts/coverage.sh /tmp/cliproxy-coverage.out
./scripts/security-audit.sh
```

Expected: scripts PASS; workflow содержит build dependency на каждый gate.

- [ ] **Step 4: Commit**

```bash
git add .github/workflows/ci.yml
git commit -m "ci: включить release hardening gates"
```

### Task 7: Актуализировать implementation phases

**Files:**
- Modify: `docs/implementation-phases.md`

- [ ] **Step 1: Исправить stale phase state**

Внести конкретные изменения:

- Ф1: отметить sqlc для всех таблиц и repositories admin audit/OAuth sessions
  выполненными; заменить обещание cron partition wiring на решение не добавлять
  scheduler и не менять существующую схему этим инкрементом.
- Ф2: отметить LDAP mock, access cache/blocked и session TTL tests выполненными.
- Ф4: отметить полный SDK v7.2.80 proxy surface и route matrix test; перенести
  provider OAuth flows в out-of-current-scope SDK-blocked section.
- Ф7: отметить E2E, behavioral contracts, PostgreSQL integration CI, static
  regression, coverage, security и full race только после их зелёных команд.
- Оставить открытыми load/SLA, chaos, operational runbooks, optional `/docs` и
  SDK-blocked observability/model rewrite.
- Разделить acceptance automated hardening и поздние release-operations gates.

- [ ] **Step 2: Проверить документацию на противоречия**

Run:

```bash
rg -n 'partition.*cron|OAuth flow|E2E|Coverage|Race detection|Chaos|runbook' docs/implementation-phases.md
git diff --check
```

Expected: scheduler не указан как будущая задача; OAuth не обозначен текущим
release blocker; automated gates отмечены фактическим результатом.

- [ ] **Step 3: Commit**

```bash
git add docs/implementation-phases.md
git commit -m "docs: актуализировать фазы hardening"
```

### Task 8: Полная verification и итоговый commit при необходимости

**Files:**
- Verify: all changed files

- [ ] **Step 1: Проверить generated drift и formatting**

```bash
go generate ./internal/openapi/...
git diff --exit-code -- internal/openapi/openapi.json internal/openapi/ogen
test -z "$(gofmt -l .)"
git diff --check
```

- [ ] **Step 2: Запустить статические проверки и build**

```bash
go vet ./...
go build ./...
```

- [ ] **Step 3: Запустить все test gates**

```bash
go test -short -race -timeout 5m ./...
./scripts/verify-adr9-contracts.sh
go test -run Integration -timeout 10m ./internal/store ./internal/watcher
go test -run '^TestIntegrationRuntime' -timeout 10m ./internal/e2e -v
./scripts/coverage.sh /tmp/cliproxy-coverage.out
./scripts/security-audit.sh
go run golang.org/x/vuln/cmd/govulncheck@v1.1.4 ./...
go test -race -timeout 15m ./...
```

Expected: все команды PASS; coverage ≥70.0%; no reachable vulnerabilities.

- [ ] **Step 4: Проверить scope и историю**

```bash
git status --short
git diff 93359ef..HEAD -- db/migrations internal/store/dbgen go.mod go.sum
git log --oneline 93359ef..HEAD
```

Expected: нет изменений DB schema/generated sqlc/dependencies; история состоит
из reviewable commits по OpenAPI, contracts, E2E, coverage, security, CI и docs.

- [ ] **Step 5: Зафиксировать verification-only правки, если форматирование изменило файлы**

```bash
git add -u
git commit -m "chore: завершить release hardening verification"
```

Этот commit создаётся только при наличии реальных tracked изменений после
verification; пустой commit не создаётся.
