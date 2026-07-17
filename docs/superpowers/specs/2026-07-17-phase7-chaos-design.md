# Phase 7 Chaos Gates Design

## Цель

Автоматизировать два последних release gate Ф7: failover PostgreSQL advisory
leader и доступность runtime после остановки одной из двух реплик.

## Advisory leader failover

Integration test запускает два `LeaderRunner` с отдельными pgx pools и одним
session-level advisory lock в реальном PostgreSQL testcontainer. Первая
реплика получает lock и запускает `SessionCleanup`, вторая остаётся standby.
После отмены контекста первой реплики lock освобождается, cleanup второй
реплики запускается в пределах retry deadline.

Recorder проверяет:

- standby не выполняет cleanup, пока leader жив;
- после остановки leader cleanup продолжается на второй реплике;
- число одновременно активных leader jobs никогда не превышает один.

## Runtime replica failover

Две реплики запускаются отдельными subprocess текущего Go test binary. Это
обязательно: upstream SDK использует process-global registries, тогда как в
Kubernetes каждый pod имеет отдельный process. In-process запуск двух
`Service` создавал бы несуществующее production-взаимодействие.

Оба subprocess:

- используют общий PostgreSQL testcontainer;
- загружают один upstream credential и model override;
- регистрируют DB-backed access provider и fake SDK executor;
- поднимают полный SDK HTTP runtime на разных loopback ports.

Parent test логинится и выпускает API-key на первой реплике, проверяет session
на второй, затем принудительно завершает process первой реплики. На второй
повторно проверяются `/healthz`, `/api/v1/me` и inference с ранее выпущенным
API-key. Так проверяются stateless session/API-key persistence и продолжение
основной функции сервиса.

## CI gate

`scripts/verify-chaos-gates.sh` сначала проверяет наличие обоих точных test
names, затем запускает их. Отдельный GitHub Actions job `Chaos/failover`
включается в обязательные dependencies build.

## Границы

- Production-код и SDK boundary не меняются.
- Тесты не импортируют upstream `internal/*`.
- Failover PostgreSQL самого DB cluster не входит в scope: R6.4 относит backup
  и HA PostgreSQL к оператору БД.
- Тесты не запускаются при `testing.Short()`.

## Проверка

- RED: chaos verification script завершается ошибкой до добавления test names;
- targeted leader и runtime failover tests;
- full non-race и full race suites;
- coverage, vet, build, security audit и package godoc gate.
