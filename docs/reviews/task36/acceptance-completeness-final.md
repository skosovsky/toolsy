# Task36 — окончательная независимая приёмка полноты

Дата: 2026-10-07. Scope: итоговый кандидат host recipe, integration module,
миграция/CI, core fresh replay provenance boundary и исполняемые доказательства.
Повторно прочитаны ТЗ и изменённые файлы, итоговые логи и verification index.

После завершения всех итоговых gates **9/9 AC = 100% полностью выполнены**.
Parent подтвердил terminal exit 0 session 13846; независимо проверен итоговый
лог полного `make test`: выполнены все 25 module, пропущенных module нет,
последний optional integration module завершился PASS, FAIL/make errors отсутствуют.
Частичные критерии не засчитывались.

| AC | Статус | Доказательство |
|---|---|---|
| AC1 | Выполнено | Independent identical intents, original provider correlation, stable operation replay и duplicate/missing admission покрыты TestAC1IdentityAndReplay/TestAC1Admission, с handler/effect counters и настоящими native call parts в optional fixture. |
| AC2 | Выполнено | TestAC2Barriers отдельно проверяет Pause/Yield/Halt, completion halt/silent_yield, pending и unknown. AC1 наблюдает continue. TestAC2InProgressStopsNextEffect создаёт live claimed journal record через persistence fault и доказывает остановку до следующего handler. Missing/rejected/expired approval и concurrent admission проверены. |
| AC3 | Выполнено | Business error с nil Err, correction/empty/noop, partial result/progress/envelope/controls одновременно с ErrHalt и остановка sequence наблюдаются в host output. Text MIME projection проверен отдельно. Классификация deny/fault/correction и их приоритеты проверены regressions/adversarial fixtures. |
| AC4 | Выполнено | Internal/user-only и diagnostic data не сериализуются в ModelResult, включая progress/error fixtures. Fresh reducer counts и completed/cache replay suppression проверены. Fresh forged reserved metadata запрещена core boundary; настоящая replay provenance допустима только с library-issued immutable proof. |
| AC5 | Выполнено | Настоящий caller получает тот же source schema из view. Hidden tool отклоняется admission. Exact integer/decimal/null сохраняются в semantic integration. Canonical default и binder/handler attachment contents/MIME наблюдаются непосредственно в recipe fixture. |
| AC6 | Выполнено | Args/attachments/subject/scope/policy/dependency/activity mutations и tool/manifest/view/schema mutations инвалидируют grant до handler. Текущая revoke policy закрывает dispatch и replay delivery. Bound description, approve/resume, rejected/expired и два host resume с максимум одним handler проверены. |
| AC7 | Выполнено | Durable file reopen и Inspect → authorized replay показывают delivery loss после completed persistence, новый CallID и один handler. Fault injection completed Finish после handler effect даёт uncertainty и blocked redelivery; trusted proof reconciliation даёт completed replay без повторного handler/reducer. Inspect используется как диагностический read, без model projection. |
| AC8 | Выполнено | Runnable main проверен, README/ownership/one-shot/unsupported limits и ссылки добавлены. Optional module содержит настоящие public caller/schema/guard/durable activity APIs; local и published semantic modes PASS. Local temporary workspace использует явные paths и module identity validation; published GOWORK=off без replace копирует только consumer recipe. CI содержит оба режима и отдельный released consumer job. Core не импортирует callers/continuation/policy dependencies, BYOT сохранён. |
| AC9 | Выполнено | Whole-module make lint и make test terminal exit 0; итоговый test log покрывает все 25 module. Root и recipe race PASS, local/published semantic `-race -count=1` PASS, migration before/after и reproduce commands есть. Непроверенная production durability не заявлена. |

## Независимые проверки и просмотренные доказательства

Самостоятельно выполнены exit 0:

```sh
GOPATH=/tmp/toolsy-task36/gopath GOCACHE=/tmp/toolsy-task36/cache \
  go test -race -count=1 -v ./examples/host_dispatch/...
GOPATH=/tmp/toolsy-task36/gopath GOCACHE=/tmp/toolsy-task36/cache \
  go run ./examples/host_dispatch
```

Recipe лог: `/tmp/task36-completeness-final-recipe.log`.
Main показал approval, ровно один fresh reducer effect и два результата с
разными provider CallID, exact integer и stable intent replay.

Просмотрены итоговые `/tmp/task36-core-final.log`,
`/tmp/task36-recipe-final.log`, `/tmp/task36-integration-local-final.log`,
`/tmp/task36-integration-published-final.log`,
`/tmp/task36-make-lint-final.log` и завершённый `/tmp/task36-make-test-final.log`.
Оба integration mode завершились PASS всех четырёх semantic tests, root race
завершился PASS, whole-module lint содержит zero issues для каждого module.
Повторная terminal проверка test log сверила module headers с каждым repository
go.mod: 25/25 без пропусков; последний module завершился semantic PASS.

## Core provenance и архитектура

Минимальная core дельта закрывает подтверждённую возможность fresh producer
выдать effects за replay. Private invocation-local replayProof не сериализуется,
связан с payload/effects/envelope/controls; исключаются лишь корреляционные
CallID/ToolName для scoped forwarding. Standard policy overlay повторно маркирует
только исходный valid proof. Genuine nested replay и adversarial modifications
проверены root regression и независимым correctness consumer probe.

Это обоснованный behavior break; migration явно требует убрать ручной
`toolsy.replay_source`. Release guidance выбирает `make release-break`.
Новый scheduler остаётся example application code; core ownership не расширен.

Прежний generic CI import gap закрыт добавлением optional module в repository
go.work: generic jobs разрешают recipe из checkout root. Published semantic mode
отдельно проверяет workspace-free dependencies. Файл docs/host-dispatch-migration.md
описывает конкретные caller glue изменения и recovery obligations.

## Отдельные обязательства после реализации

Не включены в процент AC: публикация release, released-mode semantic проверка
фактически опубликованного root, конкретное уведомление автора issue и закрытие
issue. До получения этих доказательств вся task36 не завершена. Production
distributed durability и exactly-once внешних effects не заявлены; локальные
durability fixtures проверяют только описанные границы.
