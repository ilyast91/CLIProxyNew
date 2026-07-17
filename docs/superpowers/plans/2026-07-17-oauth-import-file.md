# OAuth Credential File Import Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Поддержать загрузку OAuth credential JSON как multipart-файла, сохранив обычный POST JSON и отложив interactive OAuth login.

**Architecture:** Handler выбирает reader по media type, ограничивает body/file размером 1 MiB и передаёт оба формата в единую decode/validate/register pipeline. OpenAPI описывает JSON и multipart, а проектная документация переводит provider login из SDK blocker в deferred post-v1 scope.

**Tech Stack:** Go 1.26.5, Gin, MIME multipart, OpenAPI 3.1, ogen.

---

### Task 1: RED multipart contract

**Files:**
- Modify: `internal/httpapi/admin_oauth_credentials_test.go`

- [x] Добавить successful multipart file upload test.
- [x] Добавить missing file, oversized file и unsupported media type tests.
- [x] Запустить targeted tests и подтвердить FAIL текущего JSON-only handler.

### Task 2: Shared import decoder

**Files:**
- Modify: `internal/httpapi/admin_oauth_credentials.go`

- [x] Выбрать JSON/multipart reader через parsed media type.
- [x] Ограничить JSON/file 1 MiB и multipart envelope.
- [x] Использовать существующую duplicate/register/audit pipeline без
  расхождения поведения между форматами.
- [x] Запустить targeted tests и подтвердить GREEN.

### Task 3: OpenAPI

**Files:**
- Modify: `openapi.yaml`
- Regenerate: `internal/openapi/openapi.json`
- Regenerate: `internal/openapi/ogen/*`

- [x] Добавить multipart binary `file`, responses 413/415 и описание лимита.
- [x] Выполнить generation, Spectral и deterministic drift check.

### Task 4: Scope/status documentation

**Files:**
- Modify: `README.md`
- Modify: `internal/auth/oauth/doc.go`
- Modify: `docs/requirements.md`
- Modify: `docs/architecture.md`
- Modify: `docs/design/r9-oauth-and-testing.md`
- Modify: `docs/sdk-reference.md`
- Modify: `docs/implementation-phases.md`

- [x] Зафиксировать interactive OAuth login как deferred post-v1.
- [x] Описать JSON body и multipart file import.
- [x] Удалить OAuth login из списка текущих SDK blockers; оставить четыре
  extension-point blockers.

### Task 5: Verification

- [x] Выполнить targeted/full tests, race, vet и build.
- [x] Выполнить OpenAPI, security, godoc и coverage gates.
- [x] Проверить итоговый status/diff.
