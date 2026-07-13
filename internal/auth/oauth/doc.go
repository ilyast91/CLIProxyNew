// Package oauth реализует асинхронный OAuth login-flow (R9.A.1) —
// собственная реализация поверх низкоуровневых auth-сервисов ядра
// (claude.NewClaudeAuth, codex.NewCodexAuth, ...).
//
// НЕ использует блокирующий sdkAuth.Manager.Login (см. ADR-9 и
// docs/design/r9-oauth-and-testing.md). Сессии в Postgres (oauth_sessions)
// → multi-replica.
//
// Два режима: callback-flow (Codex/Claude/Antigravity) и device-flow
// (Kimi/xAI).
//
// Имплементация: Фаза 4 (Management API).
package oauth
