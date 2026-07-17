# Обновление upstream SDK

Runbook реализует R12 для зависимости
`github.com/router-for-me/CLIProxyAPI/v7`. Текущая проверенная версия указана в
`go.mod`, `README.md` и `docs/sdk-reference.md`.

## Границы

- Patch/minor внутри v7 выполняется отдельным reviewable commit/PR.
- Новый major требует отдельного ADR, migration plan и явного решения о
  breaking changes до изменения `go.mod`.
- Разрешены только публичные пакеты `sdk/*`. Импорт upstream `internal/*`, fork,
  patch, replace и reflection-обходы запрещены.
- Обновление SDK не совмещается с несвязанными изменениями бизнес-логики.

## Подготовка

1. Зафиксируйте текущую версию:

   ```sh
   go list -m github.com/router-for-me/CLIProxyAPI/v7
   git status --short
   ```

2. Создайте отдельную upgrade-ветку от зелёной `main`.
3. Изучите upstream release notes и public API diff между текущей и целевой
   версиями. Отдельно проверьте семь контрактов ADR-9, lifecycle `Service`,
   server options и используемые config-типы.
4. Зафиксируйте в описании изменения:
   - целевую версию и диапазон release notes;
   - breaking/behavioral changes;
   - новые или удалённые public extension points;
   - влияние на известные SDK-blocked пункты из `implementation-phases.md`.

## Обновление

```sh
go get github.com/router-for-me/CLIProxyAPI/v7@v7.x.y
go mod tidy
```

Проверьте границу SDK — команда должна завершиться без совпадений:

```sh
rg 'github.com/router-for-me/CLIProxyAPI/v[0-9]+/internal/' --glob '*.go' .
```

Актуализируйте `docs/sdk-reference.md`:

- версию и дату сверки;
- диапазон public API diff;
- изменённые сигнатуры и поведение;
- наличие или отсутствие extension points для отложенных требований.

Если изменились маршруты или management DTO, правьте source `openapi.yaml` и
перегенерируйте артефакты, не редактируя generated-файлы вручную.

## Обязательные gates

```sh
go mod tidy
git diff --check -- go.mod go.sum
go vet ./...
go build ./...
go test -short -race -count=1 -timeout 10m ./...
go test -race -count=1 ./internal/sdkcontract
./scripts/verify-adr9-contracts.sh
go test -count=1 -timeout 15m ./...
go test -race -count=1 -timeout 15m ./...
go test -count=1 -run '^TestIntegrationRuntimeSLA$' -timeout 10m ./internal/e2e -v
./scripts/coverage.sh /tmp/cliproxy-sdk-upgrade-coverage.out
./scripts/security-audit.sh
go generate ./internal/openapi/...
git diff --exit-code -- internal/openapi/openapi.json internal/openapi/ogen
```

После gates проверьте, что `go.mod`, `go.sum`, `sdk-reference.md` и при
необходимости boundary-адаптация находятся в одном reviewable изменении.

## Rollback

При несовместимости верните последнюю проверенную версию обычным revert либо
новым commit:

```sh
go get github.com/router-for-me/CLIProxyAPI/v7@v7.2.80
go mod tidy
go test -short -race -count=1 ./...
```

Не используйте `git reset --hard` в общей рабочей копии. Если upgrade уже
развёрнут, откатите image на предыдущий digest и убедитесь, что `/readyz`,
login, API-key inference и persistence upstream credentials работают.
