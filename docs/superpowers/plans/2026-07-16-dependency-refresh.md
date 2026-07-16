# Dependency Refresh Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Обновить runtime, build и CI-зависимости проекта до актуальных совместимых версий и синхронизировать документацию публичного SDK API.

**Architecture:** Go modules обновляются в пределах текущих major; CLIProxyAPI остаётся на major v7 согласно R12. После обновления сравнивается публичная поверхность `sdk/*`, обновляется `sdk-reference.md`, перегенерируются OpenAPI bindings и выполняются contract/integration/race gates до коммита.

**Tech Stack:** Go 1.26.5, CLIProxyAPI v7.2.80, ogen v1.23.0, pgx/v5, GitHub Actions v7, Spectral CLI 6.16.1.

---

### Task 1: Refresh Go modules

**Files:**
- Modify: `go.mod`
- Modify: `go.sum`

- [x] Обновить прямые модули до последних совместимых версий: Gin 1.12.0, LDAP 3.4.14, pgx 5.10.0, ogen 1.23.0, CLIProxyAPI 7.2.80, testcontainers 0.43.0 и OTel SDK 1.44.0.
- [x] Выполнить `go get -u ./...` для обновления разрешённых transitive-зависимостей, реально используемых пакетами проекта.
- [x] Выполнить `go mod tidy` и убедиться, что `go list -m -u all` больше не показывает обновлений прямых модулей в текущих major.

### Task 2: Refresh build and CI pins

**Files:**
- Modify: `.github/workflows/ci.yml`
- Modify: `internal/openapi/ogen/generate.go`
- Verify: `Dockerfile`

- [x] Обновить CI Go patch с 1.26.0 до официального stable 1.26.5; Dockerfile уже использует 1.26.5.
- [x] Обновить `actions/checkout`, `actions/setup-go` и `actions/setup-node` до major v7.
- [x] Обновить Node.js для lint job до 24 и pin Spectral CLI на 6.16.1.
- [x] Обновить go:generate pin ogen до v1.23.0.

### Task 3: Reconcile CLIProxyAPI public SDK reference

**Files:**
- Modify: `docs/sdk-reference.md`
- Modify as required: `docs/requirements.md`
- Modify as required: `docs/architecture.md`
- Modify as required: `docs/architecture-principles.md`
- Modify as required: `docs/adr/ADR-9-sdk-contracts.md`
- Modify: `README.md`
- Modify: `AGENTS.md`
- Modify: `docs/implementation-phases.md`

- [x] Сравнить экспортируемые сущности `sdk/*` между v7.2.71 и v7.2.80 и записать версию/дату сверки в `sdk-reference.md`.
- [x] Описать добавленные, изменённые и удалённые публичные контракты; проверить существующие blockers OAuth, model rewrite, watcher AuthUpdate и Execute tracing.
- [x] Зафиксировать release note: patch-upgrade внутри v7, перечень breaking changes либо явное отсутствие breaking changes для используемых контрактов.
- [x] Синхронизировать описание зависимостей и verified version во всех актуальных обзорных документах, не переписывая исторические планы как будто они создавались на новой версии.

### Task 4: Regenerate and verify

**Files:**
- Regenerate: `internal/openapi/openapi.json`
- Regenerate: `internal/openapi/ogen/openapi.compat.yaml`
- Regenerate: `internal/openapi/ogen/oas_*_gen.go`

- [x] Выполнить `go generate ./internal/openapi/...` с ogen v1.23.0 и проверить generated drift.
- [x] Выполнить `go test -race ./internal/sdkcontract`.
- [x] Выполнить `go test -short -race -timeout 5m ./...`.
- [x] Выполнить полный `go test ./...` с PostgreSQL testcontainers.
- [x] Выполнить `go vet ./...`, `go build ./...`, `git diff --check`.
- [x] Провести code review обновления и исправить Critical/Important замечания.
- [x] Создать один reviewable commit `chore(deps): актуализировать зависимости`.
