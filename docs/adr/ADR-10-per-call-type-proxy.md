# ADR-10: Per-call-type egress proxy routing

> **Статус:** Принят (после исследования контрактов ядра v7/main).
> **Дата:** 2026-07-11
> **Связанные:** R10, ADR-9 (контракты SDK), R9.A.1/A.2 (настройка провайдеров).

## Контекст

Требование R10: разные типы upstream-вызовов (inference, auth-OAuth, quota,
models) одного и того же аккаунта должны ходить через разные egress-прокси,
задаваемые администратором. Глобально per-call-type; если прокси не указан —
напрямую (direct). Отдельно: per-provider `base_url` (через
`Auth.Attributes["base_url"]`).

Изначально предполагалось проверить три подхода (C → fallback A → B).

## Исследование ядра v7 (результат)

**Per-request override прокси невозможен через публичные контракты ядра.**
Подтверждено детальным анализом:

| Точка расширения | Прокси? | Почему нет |
|------------------|---------|------------|
| `executor.Options` | ❌ | Нет поля прокси/transport; `Metadata` не интерпретируется как прокси |
| `RequestAfterAuthInterceptor` | ❌ | Меняет только headers/body |
| Plugin `RequestInterceptor`/`ResponseInterceptor` | ❌ | Меняют только headers/body; `HostHTTPClient` инжектируется ядром |
| `ProviderExecutor.HttpRequest` | ❌ | Прокси не передаётся отдельно от auth |
| `api.ServerOption`/middleware | ❌ | Inbound-only; upstream-клиентом ядро владеет само |
| `coreauth.RoundTripperProvider` | ⚠️ частично | `RoundTripperFor(auth)` — без типа вызова, отличить call-type нельзя |

**Прокси детерминированно выводится из `auth.ProxyURL` → `cfg.ProxyURL` →
контекстного RoundTripper** (`proxy_helpers.go`, приоритеты 1→2→3). Заданный
`auth.ProxyURL` жёстко побеждает, transport кэшируется per-proxy-URL.

→ **Подход C (per-request Options) отпадает.**

## Решение: Подход A — динамический ProxyURL в Selector

Прокси назначается per-call-type как **чистая функция**, и выставляется в
`auth.ProxyURL` в момент `Selector.Pick` (для inference) и в аналогичных
точках (для auth/quota/models — см. ниже).

### Принцип
```go
proxyFor(callType, provider) string  // чистая функция, не persisted
// "" → direct (без прокси), приоритет выше cfg.ProxyURL
```

### Применение по типам вызовов

| Call-type | Где задаётся прокси | Примечание |
|-----------|---------------------|------------|
| **inference** (Execute/ExecuteStream) | `Selector.Pick` выставляет `auth.ProxyURL` перед возвратом | основная точка; тип определяется по `opts.SourceFormat`/роуту |
| **auth-OAuth** (login/refresh) | точка вызова `sdkAuth.Manager.Login` / `Refresh` в business-слое | business-слой сам инициирует, может выставить ProxyURL до вызова |
| **quota** (R9.A.4) | точка запроса квоты в business-слое | аналогично |
| **models** (ListModels) | точка запроса моделей в business-слое | аналогично |

### Защита от гонок (критично)

`Auth.ProxyURL` — разделяемое поле аккаунта. Несколько параллельных call-type'ов
могут перезаписывать его. Меры:
1. **Не persist'ить** изменённый ProxyURL в `Store.Save` — ядро вызывает Save
   после refresh, бизнес-слой должен гарантировать, что ProxyURL при сохранении
   = значение аккаунта по умолчанию (не временный per-call-type).
2. **Вычислять ProxyURL синхронно в момент вызова**, не полагаться на ранее
   выставленное значение (идемпотентность).
3. При high-concurrency рассмотреть shallow-copy `*Auth` перед выставлением
   ProxyURL, если ядро допускает (проверить при имплементации).

### Проверка при имплементации
- Убедиться, что `Store.Save` от ядра (после auto-refresh) не сохраняет
  временно выставленный ProxyURL как постоянный аккаунта (фильтровать поле или
  восстанавливать default перед сохранением).
- Учесть utls-путь (Claude) — там та же логика приоритетов прокси.

## Что НЕ делаем

- Не forkаем ядро ради per-request прокси (подход C невозможен без правок ядра;
  fork противоречит ADR-1 «ядро = внешняя зависимость»).
- Не используем подход B (N регистраций аккаунта) — дублирование credentials,
  N-кратный refresh, ломает dedup R9.A.7. Оставлен как крайний fallback, если
  A упрётся в непреодолимые побочные эффекты ядра.

## Пер-провайдер base_url

Отдельное требование: per-provider `base_url` ( кастомные OpenAI-compatible
эндпоинты, напр. OpenRouter). Реализуется через `Auth.Attributes["base_url"]`
(нативное поле ядра, без обходных путей). Настраивается администратором вместе
с credentials (R9.A.2). **Не требует ADR — поддерживается ядром напрямую.**

## Открытые вопросы

- Точный механизм определения call-type в `Selector.Pick` (по
  `opts.SourceFormat`, по metadata `RequestPathMetadataKey`, или по роуту) —
  при имплементации R10.
- Гранулярность конфигурации: confirmed глобально per-call-type + direct
  fallback. Per-provider override прокси — НЕ требуется (только per-provider
  base_url). Если позже понадобится — расширить конфиг.
- Формат конфиг-секции прокси (HTTP/SOCKS URL per call-type) — в дизайне R10.

## Известное ограничение: auto-refresh не идёт через Selector

Ядро вызывает `ProviderExecutor.Refresh(ctx, auth)` напрямую по `auth.ID`
в рамках `coreManager.StartAutoRefresh` (min-heap по `NextRefreshAfter`),
**минуя `Selector.Pick`**. Поэтому динамическое выставление `auth.ProxyURL`
в Selector не сработает для auto-refresh-вызовов (call-type `auth`).

**Решение на первой версии:** default-прокси аккаунта.
`Auth.ProxyURL` по умолчанию (persisted в Store) = inference-прокси (или
direct). Auto-refresh идёт через него. Per-call-type override применяется
точно к:
- **inference** — `Selector.Pick` выставляет временный ProxyURL;
- **quota / models** — business-слой инициирует вызов (R9.A.4/A.6) и
  выставляет ProxyURL в точке вызова.

Если позже понадобится per-call-type прокси для auto-refresh — потребовалось
бы расширение ядра (противоречит ADR-1), поэтому сознательно не делаем.
