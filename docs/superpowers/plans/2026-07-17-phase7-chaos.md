# Phase 7 Chaos Gates Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Автоматизировать advisory leader и runtime replica failover и закрыть Ф7 production v1 scope.

**Architecture:** Advisory test использует два `LeaderRunner` и один реальный PostgreSQL lock. Runtime test запускает две изолированные SDK-реплики как subprocess тестового бинарника с общей БД, затем убивает первую и проверяет persisted session/API-key и inference на второй; отдельный CI job делает оба сценария обязательными до build.

**Tech Stack:** Go 1.26.5, pgx/v5, testcontainers PostgreSQL, CLIProxyAPI v7 public SDK, GitHub Actions, POSIX shell.

---

### Task 1: RED chaos gate

**Files:**
- Create: `scripts/verify-chaos-gates.sh`

- [x] Добавить проверку наличия точных test names
  `TestIntegrationAdvisoryLeaderFailover` и
  `TestIntegrationRuntimeReplicaFailover`.
- [x] Запустить script до добавления тестов и подтвердить ожидаемый FAIL.

### Task 2: Advisory leader failover

**Files:**
- Modify: `internal/watcher/leader_integration_test.go`

- [x] Запустить два runner с разными pools и общим advisory lock.
- [x] Подтвердить, что standby cleanup не стартует до остановки leader.
- [x] Остановить leader, дождаться второго cleanup и проверить max active=1.
- [x] Запустить targeted test и подтвердить GREEN под race detector.

### Task 3: Runtime replica failover

**Files:**
- Modify: `internal/e2e/runtime_test.go`
- Create: `internal/e2e/failover_test.go`

- [x] Выделить helper, возвращающий DSN и pool одного PostgreSQL container.
- [x] Добавить helper-process SDK replica с отдельным process-global state.
- [x] В parent test создать session/API-key на первой реплике и проверить
  management session на второй.
- [x] Убить первую реплику и проверить health, management и inference на
  второй с теми же persisted credentials.
- [x] Запустить targeted test и подтвердить GREEN под race detector.

### Task 4: CI и status

**Files:**
- Modify: `.github/workflows/ci.yml`
- Modify: `README.md`
- Modify: `docs/architecture-principles.md`
- Modify: `docs/implementation-phases.md`

- [x] Добавить independent `Chaos/failover` job и dependency build.
- [x] Закрыть оба chaos checkbox, Ф7 и текущий production v1 scope.
- [x] Оставить SDK-blocked расширения отдельным неблокирующим списком.

### Task 5: Verification и commit

- [x] Выполнить targeted chaos gate и full race suite.
- [x] Выполнить coverage ≥70%, vet, build, security и godoc gates.
- [x] Проверить status/diff.
- [x] Создать один commit.
