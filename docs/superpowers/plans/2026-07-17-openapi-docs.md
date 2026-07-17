# OpenAPI Docs Endpoint Implementation Plan

> **Superseded 2026-07-17:** Redoc implementation заменена планом
> `2026-07-17-offline-swagger-ui.md`.

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Добавить публичный Redoc endpoint `/docs`, описать его в OpenAPI и устранить warning отсутствующего description у `GET /api/v1/me`.

**Architecture:** `OpenAPIRouterConfigurator` отдаёт встроенный HTML-shell, который читает существующий `/openapi.json`; OpenAPI остаётся единственным источником истины. Новая Go-зависимость не добавляется, generated artifacts обновляются штатной командой.

**Tech Stack:** Go 1.26, Gin, OpenAPI 3.1, ogen 1.23.0, Redoc 2.5.0.

---

### Task 1: Зафиксировать HTTP и OpenAPI поведение

**Files:**
- Modify: `internal/httpapi/system_test.go`
- Modify: `internal/openapi/document_test.go`

- [x] **Step 1: Добавить HTTP test `/docs`**

Проверить status 200, `text/html; charset=utf-8`, `<redoc
spec-url="/openapi.json">` и pinned URL Redoc 2.5.0.

- [x] **Step 2: Добавить embedded contract test**

Проверить наличие `GET /docs`, response media `text/html` и непустой
`description` у `GET /api/v1/me`.

- [x] **Step 3: Проверить RED**

Run:

```bash
go test -count=1 -run 'TestOpenAPI(RouterConfiguratorServesDocumentationUI|DocumentDescribesDocsAndCurrentUser)$' ./internal/httpapi ./internal/openapi
```

Expected: `/docs` возвращает 404, а embedded document не содержит новый
контракт/description.

### Task 2: Реализовать endpoint и source specification

**Files:**
- Modify: `internal/httpapi/openapi.go`
- Modify: `openapi.yaml`
- Regenerate: `internal/openapi/openapi.json`
- Regenerate: `internal/openapi/ogen/openapi.compat.yaml`
- Regenerate: `internal/openapi/ogen/oas_*_gen.go`

- [x] **Step 1: Добавить встроенный Redoc HTML**

Зарегистрировать `GET /docs`, вернуть `text/html; charset=utf-8`; HTML должен
использовать `/openapi.json` и
`https://cdn.jsdelivr.net/npm/redoc@2.5.0/bundles/redoc.standalone.js`.

- [x] **Step 2: Обновить OpenAPI source**

Добавить `/docs` как публичный System endpoint и `description` для
`GET /api/v1/me`.

- [x] **Step 3: Перегенерировать artifacts**

Run:

```bash
go generate ./internal/openapi/...
```

Expected: embedded JSON, compatibility YAML и ogen Go files соответствуют
обновлённому `openapi.yaml`.

- [x] **Step 4: Проверить GREEN**

Run:

```bash
go test -count=1 ./internal/httpapi ./internal/openapi
```

Expected: PASS.

### Task 3: Актуализировать документацию и фазы

**Files:**
- Modify: `README.md`
- Modify: `docs/requirements.md`
- Modify: `docs/architecture.md`
- Modify: `docs/implementation-phases.md`
- Modify: `internal/httpapi/doc.go`

- [x] **Step 1: Описать доступ к документации**

Указать `/docs` как Redoc UI поверх `/openapi.json`, сохранив JSON источником
истины и отметив внешний pinned frontend bundle.

- [x] **Step 2: Закрыть пункты фаз**

Отметить Ф6 `/docs` выполненным, удалить Ф6 OpenAPI и Ф4 cleanup из блока
«Осталось сделать», обновить сводку и историю.

### Task 4: Проверка, коммит и push

**Files:** все файлы плана и ранее актуализированный блок фаз.

- [x] **Step 1: Выполнить проверки**

Run:

```bash
gofmt -w internal/httpapi/openapi.go internal/httpapi/system_test.go internal/openapi/document_test.go
git diff --check
go vet ./...
go build ./...
go test -count=1 -timeout 15m ./...
go test -race -count=1 -timeout 15m ./...
./scripts/security-audit.sh
./scripts/coverage.sh /tmp/cliproxy-openapi-docs-coverage.out
```

- [x] **Step 2: Проверить generated drift**

Повторно выполнить `go generate ./internal/openapi/...` и проверить, что
generated paths больше не изменились.

- [x] **Step 3: Создать коммит**

```bash
git add README.md openapi.yaml internal/httpapi internal/openapi docs
git commit -m "feat(openapi): добавить Redoc документацию"
```

- [x] **Step 4: Отправить main в origin**

```bash
git push origin main
```
