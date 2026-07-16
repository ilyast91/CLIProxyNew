# OpenAPI Docs Endpoint Design

## Цель

Закрыть Ф6 `/docs` и Ф4 OpenAPI cleanup: предоставить браузерную документацию
поверх встроенного `/openapi.json` и устранить Spectral warning для
`GET /api/v1/me`.

## Решение

`OpenAPIRouterConfigurator` регистрирует два публичных системных маршрута:

- `GET /openapi.json` — существующий встроенный OpenAPI 3.1 JSON;
- `GET /docs` — небольшой встроенный HTML-shell Redoc, который загружает
  `/openapi.json` и зафиксированный frontend bundle Redoc 2.5.0 с jsDelivr.

HTML не дублирует спецификацию и не требует новой Go-зависимости. Если CDN
недоступен, `/openapi.json` продолжает работать как основной машинный контракт,
а `/docs` отдаёт корректный HTML-shell.

## OpenAPI-контракт

В `openapi.yaml` добавляется `GET /docs` с tag `System`, `security: []` и
ответом `text/html`. Для `GET /api/v1/me` добавляется описание назначения и
границ ответа: endpoint возвращает user ID и роль management-сессии и не
раскрывает cookie/token.

После изменения выполняется `go generate ./internal/openapi/...`; generated
JSON, compatibility projection и ogen bindings коммитятся вместе с source
specification.

## Безопасность и маршрутизация

`/docs` не проходит через management session middleware и не принимает
пользовательский ввод. HTML использует фиксированный относительный URL
`/openapi.json`; API-key, cookie и credentials в страницу не внедряются.

## Проверка

- HTTP test подтверждает status 200, `text/html`, ссылку на `/openapi.json` и
  зафиксированный Redoc bundle.
- Embedded document test подтверждает `GET /docs`, media type `text/html` и
  непустой `description` у `GET /api/v1/me`.
- Spectral lint, generation drift, vet, build, full/race tests, security audit
  и coverage выполняются до коммита.

## Документация фаз

Ф6 `/docs` и Ф4 OpenAPI cleanup закрываются. Они удаляются из блока
«Осталось сделать»; архитектура, требования, README и package godoc отражают
доступность `/docs`.
