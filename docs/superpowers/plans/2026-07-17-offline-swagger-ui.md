# Offline Swagger UI Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Заменить Redoc CDN на embedded Swagger UI и запретить нецелевые внешние runtime resources.

**Architecture:** `swaggest/swgui/v5emb` обслуживает HTML и assets из Go binary по `/docs/*`, используя локальный `/openapi.json`. Security source audit блокирует CDN hosts, remote browser assets и `swguicdn` в runtime/build source.

**Tech Stack:** Go 1.26, Gin, swaggest/swgui v1.8.9, Swagger UI 5.32.8, POSIX shell.

---

### Task 1: RED HTTP contract

**Files:**
- Modify: `internal/httpapi/system_test.go`

- [x] Проверить Swagger UI HTML на `/docs` и `/docs/`.
- [x] Проверить HTML `/docs/` без remote asset tags, с local asset URLs и
  отключённым external validator.
- [x] Проверить доступность `/docs/swagger-ui-bundle.js`.
- [x] Запустить targeted test и подтвердить ожидаемый FAIL на Redoc handler.

### Task 2: Embedded Swagger UI

**Files:**
- Modify: `go.mod`
- Modify: `go.sum`
- Modify: `internal/httpapi/openapi.go`

- [x] Добавить `github.com/swaggest/swgui v1.8.9`.
- [x] Создать `v5emb.New("CLIProxyNew API", "/openapi.json", "/docs/")`.
- [x] Зарегистрировать GET `/docs` и `/docs/*path` через `gin.WrapH`.
- [x] Запустить targeted HTTP tests и подтвердить GREEN.

### Task 3: Enforcement и документация

**Files:**
- Modify: `scripts/security-audit.sh`
- Modify: `AGENTS.md`
- Modify: `README.md`
- Modify: `docs/requirements.md`
- Modify: `docs/architecture-principles.md`
- Modify: `docs/architecture.md`
- Modify: `docs/implementation-phases.md`
- Modify: `docs/superpowers/specs/2026-07-17-openapi-docs-design.md`
- Modify: `docs/superpowers/plans/2026-07-17-openapi-docs.md`

- [x] Добавить rule внешних runtime resources и CI source audit, включая
  запрет CDN-пакетов `swgui/v*cdn` и build tag `swguicdn`.
- [x] Заменить актуальные упоминания Redoc/jsDelivr на embedded Swagger UI.
- [x] Зафиксировать audit: единственный runtime CDN до изменения — Redoc;
  provider metadata URLs являются целевыми upstream resources.

### Task 4: Verification

- [x] Выполнить `gofmt`, `git diff --check`, `go mod tidy`.
- [x] Выполнить Spectral и deterministic OpenAPI generation check.
- [x] Выполнить vet, build и полный `-short -race` suite.
- [x] Выполнить full tests и full race.
- [x] Выполнить security audit.
- [x] Выполнить coverage ≥70%: aggregate coverage 74.8%.
- [x] Проверить итоговый diff и чистоту generated/module artifacts.
