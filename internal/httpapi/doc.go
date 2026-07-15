// Package httpapi реализует клиентские эндпоинты (делегируют ядру) и
// management-API (R9), middleware (LDAP-cookie), системные роуты (R6).
//
// - Прокси-эндпоинты (/v1/*) — роутит ядро (Gin); бизнес-слой не пишет хендлеры.
// - Management-API (/api/v1/*) — через api.WithRouterConfigurator.
// - Системные: /healthz, /readyz, /metrics, /openapi.json (R11), /docs (опц.).
// - Типы/хендлеры генерируются из openapi.yaml (spec-first, R11).
//
// Имплементация: Фазы 4 (Management API) и 6 (system probes).
package httpapi
