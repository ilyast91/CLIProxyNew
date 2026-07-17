# Phase 7 Operations Documentation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Закрыть R12/Operations documentation gates Ф7 и закрепить package godoc в CI.

**Architecture:** Пять узких runbook-файлов описывают безопасные процедуры и rollback без изменения runtime-кода. Package comments проверяются отдельным POSIX shell gate через `go list`, а README, deployment guide и phase status связывают документы в единый production checklist.

**Tech Stack:** Go 1.26, POSIX shell, PostgreSQL, Kubernetes, GitHub Actions, Markdown.

---

### Task 1: Package godoc gate

**Files:**
- Create: `scripts/check-package-docs.sh`
- Create: `db/migrations/doc.go`
- Create: `internal/e2e/doc.go`
- Create: `internal/sdkcontract/doc.go`
- Create: `internal/store/dbgen/doc.go`
- Modify: `.github/workflows/ci.yml`

- [x] Добавить shell gate, который печатает import paths без `.Doc` и
  завершает работу с exit code 1.
- [x] Запустить gate до `doc.go` и подтвердить FAIL для четырёх пакетов.
- [x] Добавить package comments, не меняя generated sqlc-код.
- [x] Добавить gate в `static-checks` CI и подтвердить PASS локально.

### Task 2: SDK upgrade runbook

**Files:**
- Create: `docs/runbooks/sdk-upgrade.md`

- [x] Описать release-notes/public API review, patch/minor boundary и ADR для
  major upgrade.
- [x] Зафиксировать обновление `go.mod`, `go.sum`, `sdk-reference.md` и
  обязательные contract/integration/race/SLA/security gates.
- [x] Добавить rollback до последней проверенной SDK-версии.

### Task 3: Operational runbooks

**Files:**
- Create: `docs/runbooks/postgres-restore.md`
- Create: `docs/runbooks/encryption-key-rotation.md`
- Create: `docs/runbooks/api-key-rotation.md`
- Create: `docs/runbooks/ldap-bind-password-rotation.md`

- [x] Описать isolated restore, validation, cutover и rollback PostgreSQL.
- [x] Описать двухфазную rolling-safe AES rotation и условие удаления previous
  key через `enc_key_version`.
- [x] Описать create/verify/revoke API-key rotation и cache convergence.
- [x] Описать dual-account LDAP rotation и maintenance fallback.

### Task 4: Navigation и phase status

**Files:**
- Modify: `README.md`
- Modify: `deploy/kubernetes/README.md`
- Modify: `AGENTS.md`
- Modify: `docs/implementation-phases.md`

- [x] Добавить ссылки на runbooks и package-doc command.
- [x] Закрыть Ф7/R12 и Ф7/Operations в чеклистах и блоке «Осталось сделать».
- [x] Оставить в безблокерном scope только два chaos release gate.

### Task 5: Verification и commit

- [x] Выполнить `sh -n scripts/check-package-docs.sh` и сам package-doc gate.
- [x] Выполнить `gofmt -l .`, `go vet ./...`, `go test -short ./...`.
- [x] Выполнить `./scripts/security-audit.sh` и `git diff --check`.
- [x] Проверить ссылки на новые runbooks и итоговый status diff.
- [x] Создать один commit инкремента Ф7.
