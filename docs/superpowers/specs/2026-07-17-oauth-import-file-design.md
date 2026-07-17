# OAuth Credential File Import Design

## Решение по scope

Интерактивный provider OAuth login (`start/complete/device poll`) откладывается
до отдельного решения после появления подходящего публичного SDK API. Текущий
production v1 сохраняет перенос credentials через export/import JSON.

`POST /api/v1/admin/oauth/import` принимает два эквивалентных формата:

- `application/json` — полный `coreauth.Auth` в request body;
- `multipart/form-data` — JSON-файл в обязательном поле `file`.

Оба пути используют одну декодировку, проверку OAuth auth kind,
provider/email deduplication, SDK-managed ID, encrypted `Store.Save` и
`admin_audit_log`.

## Ограничения и ошибки

- Максимальный JSON credential — 1 MiB.
- Multipart request дополнительно ограничен небольшим запасом на headers и
  boundary; файл отдельно проверяется по лимиту 1 MiB.
- Отсутствующий/повреждённый файл и несколько JSON values дают `400`.
- Превышение лимита даёт `413`.
- Неподдерживаемый `Content-Type` даёт `415`.
- Токены не попадают в audit, error response или логи.

## OpenAPI

Один operation `importOAuthCredential` описывает `application/json` и
`multipart/form-data` request bodies. Multipart schema содержит binary field
`file`; новые маршруты интерактивного login не добавляются.

## Документация

Requirements, architecture, package godoc и implementation phases должны явно
различать реализованный JSON transfer и отложенный interactive login. Пункт
OAuth удаляется из списка SDK-блокеров текущего scope и остаётся сознательно
отложенной возможностью вне v1.
