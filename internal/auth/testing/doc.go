// Package testing реализует health-check upstream-аккаунтов без траты
// inference-квоты (R9.A.5).
//
// Checker.Test: для OAuth → executor.Refresh (обмен refresh_token, не тратит
// квоту); для API-key → executor.HttpRequest к metadata-endpoint (GET /models).
// Не использует Execute/CountTokens (тратят квоту).
//
// Имплементация: Фаза 4 (Management API).
package testing
