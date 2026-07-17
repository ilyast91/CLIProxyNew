# Ротация клиентского API-key

Клиентские API-keys хранятся только как bcrypt hash и не восстанавливаются.
Ротация выполняется выпуском нового ключа и отзывом старого через management
API. Plaintext нового ключа показывается один раз.

## Процедура

1. Войдите через активный identity source и откройте `/docs` либо используйте
   management API с session cookie.
2. Выпустите replacement key через `POST /api/v1/me/keys`, сохранив требуемые
   name, expiry и scope. Запишите plaintext сразу в secret backend.
3. Обновите consumer secret/config, не удаляя старый ключ.
4. Проверьте новый ключ на `GET /v1/models` и на одном согласованном inference
   запросе:

   ```sh
   curl --fail-with-body \
     -H "Authorization: Bearer $NEW_CLIPROXY_API_KEY" \
     "$CLIPROXY_URL/v1/models"
   ```

5. Переключите всех consumers и убедитесь, что запросы со старым key prefix
   исчезли из наблюдаемой активности.
6. Отзовите старый ключ через `DELETE /api/v1/me/keys/{keyID}`.
7. Подождите не менее 10 секунд: это максимальное штатное окно verified-key
   cache на других репликах. После него старый ключ должен получать 401, а
   новый продолжать работать.

Не передавайте plaintext ключа через issue, chat, shell history или логи. Для
разных consumers выпускайте разные ключи, чтобы следующая ротация не требовала
одновременной замены у всех клиентов.

## Rollback

До revoke верните consumer на старый ключ. После revoke ключ реактивировать
нельзя: выпустите ещё один replacement key, обновите consumer и отзовите
неудачный новый ключ.
