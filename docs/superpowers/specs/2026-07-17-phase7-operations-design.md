# Phase 7 Operations Documentation Design

## Цель

Закрыть документальную часть Ф7: сделать обновление SDK и обязательные
операционные процедуры воспроизводимыми, завершить package godoc и защитить его
CI-проверкой.

## Состав

В `docs/runbooks/` добавляются пять независимых процедур:

- `sdk-upgrade.md` — patch/minor upgrade CLIProxyAPI v7, compatibility gates и
  rollback; новый major остаётся ADR-only;
- `postgres-restore.md` — восстановление единственного state store в отдельную
  БД, проверка и контролируемый cutover;
- `encryption-key-rotation.md` — двухфазная rolling-safe ротация AES keyring;
- `api-key-rotation.md` — выпуск replacement key, проверка, revoke и учёт
  10-секундного multi-replica cache window;
- `ldap-bind-password-rotation.md` — предпочтительная ротация через второй
  service account и fallback с коротким login maintenance window.

README и Kubernetes deployment guide получают ссылки на эти runbooks.

## Безопасность операций

Runbooks не содержат реальные секреты и не предлагают записывать их в Git или
печатать в логи. Backup restore выполняется сначала в отдельной БД. AES key
rotation не меняет ciphertext SQL-командами: приложение продолжает читать
старые версии через `CLIPROXY_ENCRYPTION_PREVIOUS_KEYS`, а старый ключ можно
удалить только после того, как запрос к `upstream_accounts.enc_key_version`
покажет отсутствие ссылок на него.

Rolling-safe AES rotation состоит из двух rollout:

1. Все pod получают будущий ключ в previous-key map, сохраняя старую active
   version.
2. Active version переключается на новый ключ, а старый переносится в
   previous-key map.

Поэтому pod старой и новой ревизии умеют расшифровать записи друг друга.

## Package godoc gate

`scripts/check-package-docs.sh` использует `go list` и завершает CI ошибкой,
если у пакета нет package comment. Недостающие комментарии добавляются только
в handwritten `doc.go`; generated sqlc/ogen файлы не редактируются.

## Проверка

- RED/GREEN для package-doc script;
- `gofmt -l .`, `go vet ./...`, `go test -short ./...`;
- shell syntax и package-doc gate;
- markdown link audit, security audit и `git diff --check`.

Поведенческий runtime-код и схема БД не меняются, поэтому integration/race
проверяются пропорционально риску существующим short suite и CI gates.
