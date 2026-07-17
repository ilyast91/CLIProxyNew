# Kubernetes deployment

`kustomization.yaml` описывает production baseline для stateless CLIProxyNew:
две реплики, rolling update без недоступных pod, HPA, PDB, probes и graceful
termination. Перед применением замените LDAP значения в `configmap.yaml`, image
в `deployment.yaml` и host/ingress class в `ingress.yaml`.

## Предварительные условия

- В кластере работают metrics-server и указанный Ingress controller.
- Миграции Postgres применены отдельным release шагом до rollout приложения.
- В namespace создан `Secret` `cliproxy-secrets`. Он содержит как минимум:
  - `CLIPROXY_DB_DSN` — полный DSN Postgres, включая пароль;
  - `CLIPROXY_ENCRYPTION_KEY` — base64-ключ ровно из 32 байт;
  - `LDAP_BIND_PASSWORD` — пароль LDAP service account.
- При ротации добавляйте `CLIPROXY_ENCRYPTION_PREVIOUS_KEYS` до переключения
  `encryption.key_version` в ConfigMap.

System proxy задаётся обычными ключами того же Secret: `HTTP_PROXY`,
`HTTPS_PROXY`, `NO_PROXY`. Не добавляйте proxy URL в `config.yaml` и не
сохраняйте его в upstream credentials.

Пример создания Secret без записи его значений в Git:

```sh
kubectl -n cliproxy create secret generic cliproxy-secrets \
  --from-literal=CLIPROXY_DB_DSN='postgres://...' \
  --from-literal=CLIPROXY_ENCRYPTION_KEY='...' \
  --from-literal=LDAP_BIND_PASSWORD='...'
```

## Rollout

```sh
kubectl -n cliproxy apply -k deploy/kubernetes
kubectl -n cliproxy rollout status deployment/cliproxy
kubectl -n cliproxy get pods -l app.kubernetes.io/name=cliproxy
```

`terminationGracePeriodSeconds` равен 35 секундам: сервис получает `SIGTERM`,
останавливает приём новых запросов и вызывает публичный SDK `Service.Shutdown`
с лимитом 30 секунд. Liveness использует `/healthz`, readiness — `/readyz`.

Static identity source не предназначен для этого deployment: production
ConfigMap фиксирует `auth.mode=ldap`. Изменение identity mode требует
scale-to-zero или recreate, а не rolling update.

## Смена identity mode в development/test

Эта процедура допустима только вне production и нужна, чтобы session/API-key
данные разных identity sources не пересекались в работающих pod:

```sh
kubectl -n cliproxy scale deployment/cliproxy --replicas=0
# Обновите ConfigMap: server.environment=development|test, auth.mode=static.
# Добавьте CLIPROXY_STATIC_USER_* в Secret.
kubectl -n cliproxy apply -k deploy/kubernetes
kubectl -n cliproxy scale deployment/cliproxy --replicas=2
kubectl -n cliproxy rollout status deployment/cliproxy
```

Для возврата к LDAP повторите процедуру в обратном порядке. Production всегда
использует `auth.mode=ldap`; запуск с `static` дополнительно отклоняется самим
приложением.

## Операционные процедуры

- [Восстановление PostgreSQL](../../docs/runbooks/postgres-restore.md)
- [Ротация AES master-key](../../docs/runbooks/encryption-key-rotation.md)
- [Ротация клиентского API-key](../../docs/runbooks/api-key-rotation.md)
- [Ротация LDAP bind-password](../../docs/runbooks/ldap-bind-password-rotation.md)
- [Обновление upstream SDK](../../docs/runbooks/sdk-upgrade.md)

Runbooks требуют проверки в изолированной среде до production change. Secret
values не записываются в Git, логи rollout или terminal transcript.
