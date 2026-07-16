# Phase 7 Load/SLA Gate Design

## Цель

Закрыть load/SLA и SLA regression gate Ф7 без ослабления bcrypt cost 12 и без
ложного измерения latency под race detector.

## Исходная проблема

`APIKeyRepository` кэширует только список bcrypt-кандидатов по префиксу. Даже
при cache hit каждый inference-запрос повторяет bcrypt compare. Локальный тест
одного hash и двух compare занимает около 0.9с, поэтому цели
`Authenticate cache hit ≤2мс` и общий business overhead `≤5мс p95` недостижимы
при текущем кэше.

## Решение

### Verified API-key cache

- Ключ кэша: identity source + SHA-256 отпечаток полного API-key. Plaintext в
  памяти кэша не хранится.
- Значение: `APIKeyPrincipal` после успешного bcrypt compare.
- TTL: 10 секунд, как у текущего candidate cache.
- Cache hit пропускает candidate lookup, bcrypt и PostgreSQL round-trip;
  `users.status` уже проверен при заполнении cache.
- Revoke очищает verified и candidate caches локальной реплики.
- Блокировка/разблокировка пользователя инвалидирует verified entries этого
  пользователя вместе с session cache.
- Локальная admin-блокировка немедленно инвалидирует cache. Межрепличная
  блокировка/revocation остаётся bounded TTL, что соответствует R2.4 и
  принятому решению без Redis.

Prometheus `cliproxy_cache_lookups_total` отражает verified authentication cache,
а не candidate cache: только такой hit действительно устраняет дорогой bcrypt.

### SLA harness

Отдельный non-race E2E-тест поднимает существующий production-like SDK runtime,
PostgreSQL testcontainer, business middleware и fake upstream executor.

Профиль нагрузки:

- один предварительный запрос для прогрева auth/selector caches;
- 200 inference-запросов;
- 4 параллельных worker;
- fake upstream без искусственной задержки;
- 0 HTTP-ошибок.

Gate читает дочерние OpenTelemetry spans и `/metrics`, затем требует:

- сумма p95 `access.Provider.Authenticate` и `selector.Pick` ≤5мс;
- verified API-key cache hit ratio не менее 95%.

HTTP-запросы остаются способом подачи нагрузки, но SDK routing, JSON и logging
не включаются в business overhead: репозиторий отвечает только за auth,
selector и неблокирующую передачу usage. Отдельный Execute span остаётся
SDK-blocked пунктом Ф6.

Тест находится в файле с build constraint `!race`: абсолютная latency под race
instrumentation не является валидной performance-метрикой. Корректность кэша и
гонки по-прежнему проверяются unit/integration/full-race jobs.

### CI

Новый независимый job `Load/SLA regression` запускает только SLA E2E-тест после
static checks. `build` зависит от него так же, как от остальных release gates.

## Безопасность и ошибки

- SHA-256 используется только как безопасный lookup key для случайного
  высокоэнтропийного API-key; источником истины остаётся bcrypt hash в Postgres.
- Неуспешный bcrypt, blocked user и repository error не попадают в verified
  cache.
- Метрики, сообщения тестов и traces не содержат API-key или его отпечаток.
- При ошибке любого load request gate завершается до оценки SLA.

## Проверка

- Red: новый SLA test не проходит на текущей реализации из-за повторного bcrypt.
- Green: store/httpapi unit и integration tests, SLA gate, full race, vet и
  build проходят.
- `docs/implementation-phases.md` закрывает load/SLA и CI regression пункты,
  оставляя chaos и operational runbooks открытыми.
