# Дизайн: static identity source для development/test

## Цель

Добавить альтернативный источник пользователей для локальной отладки и тестов
без LDAP. Production-режим сохраняет LDAP единственным источником личности.

## Решение

В конфигурации появляется выбор источника личности:

```yaml
server:
  environment: development # development | test | production
auth:
  mode: ldap # ldap | static
```

Значение по умолчанию для `server.environment` — `production`, для
`auth.mode` — `ldap`. Конфигурация с `auth.mode: static` и
`server.environment: production` невалидна: приложение завершает запуск до
открытия HTTP-порта.

Static credentials передаются только через env:

```text
CLIPROXY_STATIC_USER_USERNAME
CLIPROXY_STATIC_USER_PASSWORD
CLIPROXY_STATIC_USER_ROLE
```

Роль ограничена `user` или `admin`. Пустые username/password, неизвестный
режим или роль делают конфигурацию невалидной. Значения не попадают в YAML,
логи, метрики и трейсы.

## Компоненты и поток

`internal/auth` вводит внутренний контракт `IdentityProvider`, который
возвращает проверенную identity: username, email, role и source. Реализации:

- `ldap.Provider`: существующий LDAP bind/search/group lookup.
- `static.Provider`: точное constant-time сравнение username/password с env
  только для development/test.

HTTP login выбирает provider в wiring по `auth.mode`. После успешной identity
остальной поток единый: provisioning пользователя, проверка `users.status`,
выпуск session cookie, API-key management и аналитика.

Static mode не является fallback: ошибка LDAP в LDAP mode остаётся ошибкой
login и не переключает приложение на static user.

## Изоляция от production

Таблица `users` получает `identity_source text not null` со значениями `ldap`
или `static`. Глобальная уникальность `users.username` сохраняется: static
provider сохраняет internal username в зарезервированном формате
`static:<username>`, а LDAP provider — исходное LDAP-имя. Поэтому static user
и LDAP user с одинаковым login name получают разные `users.id`, session и
API-keys без смены существующего unique-ограничения. Префикс — технический
идентификатор и может отображаться в management API как debug identity.

Резервирование обязательно на двух уровнях: LDAP provider отклоняет имя,
начинающееся с точного префикса `static:`, а БД добавляет CHECK:

```sql
(identity_source = 'static' AND username LIKE 'static:%')
OR (identity_source = 'ldap' AND username NOT LIKE 'static:%')
```

Session middleware и `access.Provider` принимают пользователя только если его
`identity_source` соответствует активному `auth.mode`. Поэтому session и
API-key, созданные static user, не действуют в LDAP/prod режиме. При
необходимости оператор удаляет debug users/keys обычными management-операциями;
автоматического удаления при переключении режима нет.

Переключение `auth.mode` не поддерживает rolling rollout. Для development/test
deployment его выполняют только после остановки всех реплик (scale-to-zero или
recreate), затем применяют новую конфигурацию и запускают новый набор pod'ов.
Production всегда использует LDAP; static mode в production не запускается.

## Тестирование

- Unit: config validation, static provider (success/failure), запрет static в
  production.
- Unit: access/session отклоняют identity с несовпадающим source.
- Integration: static login provisions user, выпускает session/API-key;
  переключение на LDAP mode отклоняет эти credentials.
- LDAP unit-тесты используют mock transport; static mode не требует LDAP
  сервера.

## Документация и миграции

Нормативные изменения потребуются в R1, R2, R6, architecture,
architecture-principles, database-schema, implementation-phases, AGENTS,
config.example.yaml и OpenAPI-описании login.

Миграция выполняется как совместимое расширение:

1. Expand: добавить `identity_source` с `NOT NULL DEFAULT 'ldap'` и CHECK,
   сохранив существующий unique key по `username`. Старые реплики остаются
   совместимыми: их записи получают source `ldap` по default.
2. Развернуть код, который всегда читает и пишет source, но static mode ещё не
   включать.
3. Только после полной замены реплик разрешить отдельному dev/test deployment
   запуск с `auth.mode: static`.

Down-миграция разрешена только пока нет `identity_source = 'static'`. Если
static identity не использовалась, оператор может удалить её вместе с session
и API-key credentials и выполнить rollback. Если static user имеет usage или
audit history, FK не даст удалить его: rollback схемы намеренно завершается
явной ошибкой. Для такого deployment допустимы только forward-fix или
восстановление БД из backup, существовавшего до static mode. На пустой тестовой
БД миграции по-прежнему проходят `up -> down -> up`.

ADR-9, ADR-10 и sdk-reference не меняются: static identity source расположен
в бизнес-слое до SDK-контрактов и не меняет интеграцию с ядром.
