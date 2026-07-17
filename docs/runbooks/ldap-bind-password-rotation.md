# Ротация LDAP bind-password

`LDAP_BIND_PASSWORD` принадлежит service account для LDAP search/bind и
хранится только в secret backend/Kubernetes Secret. Production использует
`auth.mode=ldap`.

## Предпочтительный вариант: второй service account

1. Создайте новый LDAP service account с теми же минимальными read/search
   правами и проверьте его вне приложения.
2. Подготовьте полный Secret с новым `LDAP_BIND_PASSWORD` и ConfigMap с новым
   `ldap.bind_dn`. Не удаляйте старый account.
3. Примените Secret/ConfigMap и выполните rolling rollout. Старые pod продолжают
   использовать старый account, новые — новый, поэтому login остаётся доступен.
4. Проверьте `/readyz`, user login и admin login на новой ревизии. Убедитесь,
   что LDAP errors не содержат credentials.
5. После полного rollout отключите старый service account.

## Fallback: смена пароля существующего account

Если каталог не позволяет держать два account, согласуйте короткое login
maintenance window:

1. Подготовьте полный защищённый env-файл для `cliproxy-secrets` с новым
   `LDAP_BIND_PASSWORD`; он должен содержать и остальные действующие secret
   keys, чтобы они не потерялись при обновлении.
2. Смените пароль service account в LDAP.
3. Немедленно обновите Kubernetes Secret и выполните rollout:

   ```sh
   kubectl -n cliproxy create secret generic cliproxy-secrets \
     --from-env-file="$SECURE_ENV_FILE" \
     --dry-run=client -o yaml | kubectl apply -f -
   kubectl -n cliproxy rollout restart deployment/cliproxy
   kubectl -n cliproxy rollout status deployment/cliproxy
   ```

4. Проверьте user/admin login. Уже выпущенные API-keys не требуют LDAP bind и
   продолжают обслуживать inference, но новые management login могут быть
   недоступны между сменой LDAP password и готовностью новых pod.

После операции безопасно удалите локальный env-файл по правилам secret
management вашей платформы.

## Rollback

Для dual-account схемы верните старые `bind_dn` и Secret и выполните rollout.
Для in-place схемы верните старый пароль в LDAP и Secret. Если старый пароль
уже нельзя восстановить, задайте новый известный пароль, синхронно обновите
Secret и повторите rollout.
