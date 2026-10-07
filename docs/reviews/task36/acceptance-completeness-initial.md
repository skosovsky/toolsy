# Task36 — первичная независимая приёмка полноты

Дата: 2026-10-07. Проверены текущие `docs/host-dispatch-contract.md`,
`examples/host_dispatch`, `examples/host_dispatch_integration`,
`scripts/task36-integration.sh`, существующий CI и доступные логи.
Это промежуточный снимок до заключительных доработок и общих gates.

Полностью выполнено **2/9 AC = 22,22%**. Частичные критерии не засчитываются.
Процент оценивает доказанную полноту, а не объём уже написанного кода.

| AC | Статус | Доказательства и недостающая часть |
|---|---|---|
| AC1 | Выполнено | `TestAC1IdentityAndReplay` проверяет независимые одинаковые payload, исходные CallID, replay с новым CallID и счётчики handler/effects. `TestAC1Admission` проверяет duplicate/missing identity и отказ до эффекта. Правила заданы контрактом. |
| AC2 | Частично | `TestAC2Barriers`, rejected/expired resume и admission покрывают основные controls, approval, неизвестный результат и concurrent batch. Нет отдельного наблюдаемого случая живого in-progress journal record с остановкой до второго handler. Успешный continue наблюдается в AC1. |
| AC3 | Частично | Business `ExecutionError` при nil infrastructure error, correction, empty/noop проверены. Нет полного host path с partial result/progress/controls одновременно с error и проверкой сохранности каждого поля; нет наблюдаемого text MIME/envelope сценария. |
| AC4 | Выполнено | Audience тесты и `Project` проверяют отсутствие секретов в сериализации на error/progress path, internal/user-only отсутствие projection. Identity/replay тест проверяет fresh effects 2 и отсутствие дополнительного reducer invocation на replay. |
| AC5 | Частично | Source schema сравнивается с реально переданным schema в integration fixture; hidden tool отклоняется до dispatch; exact int/decimal/null и canonical default проверяются. Attachments добавлены в request, но binder/handler не наблюдают их содержимое, поэтому сохранность по полному пути не доказана. |
| AC6 | Частично | Args/attachments/subject/scope/policy/dependency/activity mutations, revoke dispatch/replay, approve/resume и два concurrent resume покрыты. Нет mutations tool/manifest/view и доказательства инвалидирования прежнего grant после этих изменений. |
| AC7 | Частично | Durable reopen, Inspect completed и authorized replay с новым CallID при одном handler покрыты recipe и реальным durable activity API. Unknown после handler error блокирует повтор. Нет явного fault injection store persistence после внешнего эффекта с проверкой unknown, и нет отдельной демонстрации trusted reconciliation на этой границе. |
| AC8 | Частично | Runnable provider-neutral main, BYOT и отдельный module есть; реальные public native-call/scope APIs и durable runner/activity APIs участвуют в semantic assertions, локальные mocks заменяют только offline provider. Local-mode log успешен. Нет опубликованного режима evidence, README и ссылок из root README/execution contract; CI для двух режимов ещё не добавлен. |
| AC9 | Частично | Recipe race independently rerun успешно; local integration log успешен; root lint log содержит `0 issues`. Нет результатов `make test`, `make lint` на итоговом diff, обоих semantic modes `-race -count=1`, migration before/after и репозиторного verification index. |

## Выполненные проверки

Независимо выполнено:

```sh
GOPATH=/tmp/toolsy-task36/gopath GOCACHE=/tmp/toolsy-task36/cache \
  go test -race -count=1 -v ./examples/host_dispatch/...
```

Команда завершилась exit 0; лог: `/tmp/task36-acceptance-completeness-initial.log`.
Также прочитаны `/tmp/task36-recipe.log`,
`/tmp/task36-integration-local.log` и `/tmp/task36-root-lint.log`.
Логи второго режима и полного root test gate на момент снимка отсутствовали.

## Дополнительный CI gap

Generic matrix автоматически обнаруживает новый optional module. Его прямой
import `github.com/skosovsky/toolsy/examples/host_dispatch/recipe` отсутствует
в текущей published root dependency до выпуска recipe. Скрипт published mode
адаптирует import в temporary consumer module, но generic matrix этого не делает.
Нужны явные jobs обоих semantic modes и корректное исключение/подготовка этого
module для generic lint/test. Зависимости local mode должны предоставляться
явными checkout paths; отсутствие зависимости должно завершаться failure.

## Для окончательной приёмки

Добавить перечисленные наблюдаемые fixtures, документацию и CI; запустить
итоговые gates и оба semantic dependency modes; сохранить воспроизводимые
команды, входные revision identities и результаты. Повторно проверить AC1–AC9
на одном финальном diff. Релиз, published consumer verification, уведомление
авторов issue и закрытие issue оцениваются отдельно от процента реализации и
в этом промежуточном снимке ещё не выполнены.
