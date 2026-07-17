# Восстановление PostgreSQL

PostgreSQL — единственное состояние CLIProxyNew: в нём находятся пользователи,
сессии, API-keys, upstream credentials, аналитика и audit log. Backup/PITR
создаёт оператор БД (CloudNativePostgres, Velero или DBA), но восстановление
должно проходить по этой процедуре.

## Предварительные условия

- выбран backup/PITR target и зафиксированы ожидаемые RPO/RTO;
- доступны image приложения и миграции, соответствующие backup;
- доступны AES active/previous keys, действовавшие на момент backup;
- подготовлена отдельная пустая restore-БД, не production DSN;
- назначены ответственные за БД, Kubernetes и функциональную проверку.

Никогда не проверяйте backup прямым восстановлением поверх production-БД.

## Восстановление в изолированную БД

Для logical backup:

```sh
pg_restore \
  --exit-on-error \
  --no-owner \
  --no-privileges \
  --dbname="$RESTORE_DSN" \
  "$BACKUP_FILE"
```

Для physical backup/PITR используйте процедуру PostgreSQL-оператора и создайте
отдельный cluster/instance. Не запускайте две writable копии с одним service
endpoint.

## Проверка

1. Проверьте migration state и основные таблицы:

   ```sh
   psql "$RESTORE_DSN" -v ON_ERROR_STOP=1 -c 'TABLE schema_migrations;'
   psql "$RESTORE_DSN" -v ON_ERROR_STOP=1 -c \
     'SELECT count(*) FROM users; SELECT count(*) FROM upstream_accounts; SELECT count(*) FROM admin_audit_log;'
   psql "$RESTORE_DSN" -v ON_ERROR_STOP=1 -c \
     'SELECT enc_key_version, count(*) FROM upstream_accounts GROUP BY enc_key_version ORDER BY enc_key_version;'
   ```

2. Убедитесь, что keyring содержит каждую показанную `enc_key_version`.
3. Запустите одну canary-реплику в изолированном namespace с restore DSN и без
   production ingress. Проверьте `/healthz`, `/readyz`, login, чтение списка
   моделей и management read operations.
4. Не запускайте inference к реальному upstream без согласованной проверки:
   восстановленные credentials могут быть активны и расходовать квоту.

## Cutover

1. Остановите запись в старую БД: scale deployment до нуля либо включите
   согласованное maintenance window.
2. Если нужен минимальный RPO, примените финальный WAL/PITR step по процедуре
   оператора и повторите проверки целостности.
3. Обновите `CLIPROXY_DB_DSN` в Secret на восстановленный cluster endpoint.
4. Разверните одну реплику, дождитесь `/readyz`, затем верните штатное число
   реплик и проверьте rollout, HPA/PDB и advisory leader.
5. Проверьте login, выпуск/отзыв API-key, inference, usage и audit log.

## Rollback

Пока новая БД не подтверждена, не удаляйте старую. При ошибке снова остановите
deployment, верните предыдущий DSN Secret и image digest, затем выполните
rollout. После успешного cutover оставьте старую БД read-only на срок политики
возврата и только затем удаляйте её по change-management процедуре.
