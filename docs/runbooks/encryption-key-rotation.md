# Ротация AES master-key

Upstream credentials зашифрованы AES-256-GCM. Активная версия задаётся
`encryption.key_version`, активный ключ — `CLIPROXY_ENCRYPTION_KEY`, остальные
доступные версии — JSON-картой `CLIPROXY_ENCRYPTION_PREVIOUS_KEYS`.

## Правила безопасности

- Ключ — base64 от ровно 32 случайных байт; хранится только в secret backend.
- Не печатайте ключи в терминальный transcript, CI output или Git.
- Не меняйте `credentials_enc` и `enc_key_version` SQL-командами.
- Старый ключ удаляется только когда в БД нет строк его версии.

Перед началом сохраните rollback-копию Secret в защищённом secret backend и
проверьте распределение версий:

```sh
psql "$CLIPROXY_DB_DSN" -v ON_ERROR_STOP=1 -c \
  'SELECT enc_key_version, count(*) FROM upstream_accounts GROUP BY enc_key_version ORDER BY enc_key_version;'
```

## Фаза 1: распространить будущий ключ

Сгенерируйте новую версию `N+1`. Оставьте текущие
`encryption.key_version=N` и `CLIPROXY_ENCRYPTION_KEY=old`. Добавьте новый ключ
в `CLIPROXY_ENCRYPTION_PREVIOUS_KEYS` под версией `N+1`, сохранив все реально
используемые старые версии.

Обновите Secret и выполните rollout всех pod. После rollout каждая реплика
остаётся writer версии N, но уже умеет читать будущую версию N+1. Проверьте
`/readyz` и отсутствие `unknown key version` в логах.

## Фаза 2: переключить active version

1. Установите `encryption.key_version=N+1` в ConfigMap.
2. Установите новый ключ как `CLIPROXY_ENCRYPTION_KEY`.
3. Перенесите старый active key N в `CLIPROXY_ENCRYPTION_PREVIOUS_KEYS` и
   удалите оттуда N+1, чтобы active version не дублировалась.
4. Выполните rolling rollout и проверьте `/readyz` на каждой реплике.

Смешанный rollout безопасен: pod старой ревизии получил N+1 на фазе 1, а pod
новой ревизии получает N как previous key. Новые вызовы `coreauth.Store.Save`
записывают ciphertext уже с версией N+1.

## Завершение и удаление старого ключа

Периодически проверяйте версии:

```sh
psql "$CLIPROXY_DB_DSN" -v ON_ERROR_STOP=1 -c \
  'SELECT enc_key_version, count(*) FROM upstream_accounts GROUP BY enc_key_version ORDER BY enc_key_version;'
```

Старые записи перешифровываются только при поддерживаемом приложением
`Store.Save` (например, после refresh/update credential). Если строки версии N
остались, ключ N обязан оставаться в previous map. Отсутствие автоматического
bulk rewrite не разрешает прямое SQL-перешифрование.

Удаляйте ключ N только после нулевого количества строк N и успешной проверки
login, management credential test/quota и inference на всех репликах.

## Rollback

Верните `encryption.key_version=N` и старый active key. Новый ключ N+1 оставьте
в previous map: во время фазы 2 могли появиться строки N+1. Удалять новый ключ
можно только после того же запроса к `enc_key_version` и отсутствия строк N+1.
