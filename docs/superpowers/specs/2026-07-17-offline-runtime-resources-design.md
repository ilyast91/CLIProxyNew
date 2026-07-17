# Offline Runtime Resources Design

## Цель

Убрать нецелевую runtime-зависимость `/docs` от jsDelivr и закрепить правило:
приложение не загружает внешние UI/static resources, telemetry SaaS или иные
неявные сервисы для собственной работы.

## Swagger UI

Используется `github.com/swaggest/swgui v1.8.9`, package `v5emb`. Он содержит
Swagger UI 5.32.8 и статические assets через Go embed.

- `GET /docs` и `GET /docs/` отдают Swagger UI HTML.
- `GET /docs/*` отдаёт встроенные JS/CSS/favicon assets.
- Swagger UI читает существующий `/openapi.json` с того же origin.
- Build tag `swguicdn` запрещён, так как переводит `v5emb` обратно на CDN.

## Правило внешних ресурсов

Runtime может обращаться только к ресурсам, которые являются частью функции
сервиса или явно заданы оператором: PostgreSQL, LDAP, upstream/provider APIs,
system egress proxy и настроенные telemetry exporters. UI assets, шрифты,
analytics scripts, схемы и документация должны поставляться внутри бинарника
или deployment artifact.

Новое нецелевое внешнее runtime-обращение требует явного требования,
конфигурации endpoint, описанного failure mode и архитектурного review.

## Enforcement

`scripts/security-audit.sh` запрещает в `cmd/**`, `internal/**`, Dockerfile и CI:

- известные CDN hosts (`cdn.jsdelivr.net`, `cdnjs.cloudflare.com`,
  `unpkg.com`, `fonts.googleapis.com`, `fonts.gstatic.com`);
- remote `<script src>` и `<link href>`;
- произвольные remote JS/CSS/font URLs и CSS `url(https://...)`;
- прямые импорты CDN-вариантов `swaggest/swgui/v*cdn`;
- build tag/flag `swguicdn`.

Provider endpoints в `internal/auth/testing` не попадают под запрет: они нужны
для явной проверки настроенного upstream account и относятся к функции сервиса.

## Проверка

HTTP tests подтверждают локальные asset URLs, отсутствие remote asset tags в
Swagger UI HTML и доступность embedded JS. Source audit, vet,
build, full/race tests и coverage выполняются после изменения зависимости.
