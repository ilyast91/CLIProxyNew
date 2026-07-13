// Package ldap реализует LDAP-логин и выпуск session-cookie (R1).
//
// Login: service-bind + search user DN → user-bind → проверка групп
// (admin-group → admin TTL 10h; иначе user-group → user TTL 5m; иначе 403)
// → provisioning users → INSERT sessions → Set-Cookie.
//
// Service-account пароль — только из env LDAP_BIND_PASSWORD (не в БД).
//
// Имплементация: Фаза 2 (Auth).
package ldap
