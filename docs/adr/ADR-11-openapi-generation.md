# ADR-11: Генерация OpenAPI bindings

- **Статус:** Принято
- **Дата:** 2026-07-15

## Контекст

`openapi.yaml` — единственный источник истины для внешнего HTTP-контракта.
Спецификация использует OpenAPI 3.1 и JSON Schema union-типы с `null`.
Нужны воспроизводимые typed Go bindings и CI-проверка того, что сгенерированные
файлы не отстали от контракта.

## Решение

Используем `ogen` v1.20.3. Перед генерацией команда `cmd/openapiogen` создаёт
`internal/openapi/ogen/openapi.compat.yaml`: узкую compatibility projection для
двух используемых union-вариантов OAS 3.1:

- `[string, null]` преобразуется в `type: string` и `nullable: true`;
- `[object, array, null]` преобразуется в `oneOf` object/array и `nullable: true`.

Исходный `openapi.yaml` не изменяется и остаётся источником истины. Генерация
запускается командой `go generate ./internal/openapi/...`; генератор закреплён
точной версией в `go:generate`, а runtime-библиотека сгенерированного кода
закреплена на той же версии в `go.mod`. CI пересоздаёт JSON-документ, projection
и bindings, затем проверяет git diff.

## Последствия

Generated package компилируется вместе с приложением и готов служить основой
для постепенного перевода HTTP-adapter'ов на typed server interfaces. Переход
существующих handlers не входит в это решение и выполняется отдельным шагом.

При обновлении `ogen` необходимо выполнить generation и полный compatibility
gate из R12; изменения generated-кода и runtime-зависимостей должны быть
reviewable в отдельном коммите.
