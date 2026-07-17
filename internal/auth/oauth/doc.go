// Package oauth зарезервирован для отложенного интерактивного OAuth login-flow
// (R9.A.1). В production v1 provider-specific callback/device flow не
// реализуется; credentials импортируются и экспортируются через management API
// как полный JSON (R9.A.7).
//
// Будущая реализация не должна использовать upstream internal-пакеты и должна
// сохранить multi-replica семантику. См. docs/design/r9-oauth-and-testing.md.
package oauth
