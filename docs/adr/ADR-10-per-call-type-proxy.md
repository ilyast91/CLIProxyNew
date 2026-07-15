# ADR-10: System environment proxy

> **Статус:** Принят.
> **Дата:** 2026-07-15
> **Связанные:** R10, ADR-9, R12.

## Контекст

Ранее R10 задавал отдельные HTTP/SOCKS proxy URL для inference, auth, quota и
models. Для этого бизнес-слой должен был динамически менять `Auth.ProxyURL`.
Поле разделяется между запросами и имеет приоритет над transport процесса, что
создает гонки, усложняет persistence и связывает бизнес-слой с деталями SDK.

## Решение

Все исходящие HTTP-клиенты используют системный proxy процесса Go:

- `HTTP_PROXY` для HTTP;
- `HTTPS_PROXY` для HTTPS;
- `NO_PROXY` для адресов, которые должны обходить proxy.

Конфигурация приложения не содержит секции `proxy` и переменных
`CLIPROXY_PROXY_*`. Бизнес-слой всегда очищает `coreauth.Auth.ProxyURL` при
загрузке и сохранении credentials, включая legacy JSON. Поэтому не возникает
per-account override, а пустой `ProxyURL` позволяет публичному SDK оставить
transport равным `http.DefaultTransport`, использующему `http.ProxyFromEnvironment`.

Переменные передаются в контейнер/процесс через окружение deployment. Значения
могут содержать учетные данные и не должны попадать в логи, audit log или
`config.yaml`.

## Последствия

- Один proxy policy применяется одинаково к inference, OAuth refresh, quota и
  model requests; `NO_PROXY` задает исключения.
- Отсутствуют shared-state гонки и необходимость восстанавливать временный
  `ProxyURL` перед `Store.Save`.
- В v1 не поддерживается per-call-type или per-provider proxy routing.
- Поддерживаются возможности стандартного Go HTTP proxy mechanism; отдельная
  настройка SOCKS proxy в бизнес-слое не предоставляется.

## Обновляемость SDK

Решение опирается только на публичную семантику SDK: пустой `Auth.ProxyURL`
не создает explicit transport. При обновлении SDK compatibility gate проверяет
это условие вместе с contract/integration/race тестами R12.
