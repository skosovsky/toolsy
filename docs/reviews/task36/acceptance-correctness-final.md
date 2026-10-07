# Task36 — повторная независимая приёмка корректности

Дата: 2026-10-07. Scope: текущий host dispatch recipe, core fresh result boundary, standard policy/cache/operation composition, optional native consumer adapters и semantic fixtures. Это отчёт о корректности проверенного кода; обязательные глобальные gates, release и closeout проверяются отдельно.

## Findings initial review

| Finding | Повторная проверка | Статус |
|---|---|---|
| F1 P1: forged replay provenance подавляет fresh effects | Fresh producer отвергает reserved key с `ResultContractError.Kind=reserved_replay_metadata`; без profile host decision=fault, result/effects не доставлены. С OperationProfile первый и повторный вызовы uncertain; handler count=1 | Исправлен |
| F2 P2: partial control/error priority | Halt + infrastructure -> halt; partial Pause + ErrHalt -> halt; unknown/approval precedence остаётся выше controls | Исправлен |
| F3 P2: MIME boundary/diagnostic params | Unsupported empty MIME отклонён; business error имеет безопасный application/json без diagnostic parameters; обычный MIME canonicalized | Исправлен |

Подтверждённых незакрытых дефектов в проверенном scope не обнаружено.

## Fresh boundary и wrappers/profiles

`executePreparedResult` разделяет fresh `produce` и replay `deliver`. `prepareFreshResultChunk` проверяет наличие reserved metadata перед передачей fresh chunk в профиль; output error не может быть проигнорирован producer. Поэтому OperationProfile не сохраняет поддельный completed result, а после handler переводит claimed operation в unknown. Проверка redelivery подтверждает отсутствие повторного handler.

Replay `deliver` использует обычную contract validation и допускает library-attached provenance, поэтому cache/completed replay сохраняет корректное подавление effects. Standard policy envelope profile оборачивает handler/delivery, но не переносит собственную metadata в fresh producer; reserved policy configuration запрещена отдельным boundary. Targeted policy/cache/replay tests и recipe completed replay tests проходят. Это согласовано с documented contract для built-in producers; произвольные custom tool/profile implementations остаются trusted ports и должны соблюдать execution contract.

## Воспроизводимые проверки

1. `env GOPATH=/tmp/toolsy-task36/gopath GOCACHE=/tmp/toolsy-task36/cache go test -race -count=1 ./examples/host_dispatch/... -run .` — PASS.
2. `env GOPATH=/tmp/toolsy-task36/gopath GOCACHE=/tmp/toolsy-task36/cache go test -race -count=1 . -run 'TestTypedProducerCannotForgeReplayProvenance|TestPolicyTool|TestOperation.*Replay|Test.*ResultCache'` — PASS.
3. Temporary consumer with require Toolsy + local replace на проверенный checkout: `env GOPATH=/tmp/toolsy-task36/gopath GOCACHE=/tmp/toolsy-task36/cache GOWORK=off go test -mod=mod -race -count=1 -v .` — 6 adversarial tests PASS. Исходник сохранён в `correctness-final-probe_test.go.txt`. Проверены оба channels control priority, unsupported empty MIME, diagnostic MIME filtering, fresh spoof rejection, post-handler uncertainty и blocked redelivery.
4. `/tmp/task36-integration-published.log` просмотрен: реальные native/schema/exact-number, approval barrier/resume, guard decisions и durable activity reconciliation tests PASS с race. Published candidate mode копирует только recipe consumer code, использует `GOWORK=off` и опубликованные dependencies; это не подтверждение post-release consumer текущего Toolsy.

## Ограничения и оставшиеся внешние gates

Полный `make test`, `make lint`, local dependency mode, окончательный CI workflow, release preflight/publication, post-release `released` semantic mode и GitHub closeout не подтверждаются этим отчётом. На момент повторной проверки общий CI ещё не содержал двухрежимного task36 job; parent выполняет оставшиеся работы параллельно. Состояние этих gates нельзя выводить из отсутствия ошибок в recipe tests.

Локальный file journal reopen не доказывает distributed durability или exactly-once application внешних effects. Reducer failure и эффект до persistence остаются host recovery obligations согласно contract. Приёмка не обещает отсутствия любых ошибок вне перечисленного scope.

## Дополнительная окончательная проверка genuine replay forwarding

После общего root gate выявили и исправили отказ genuine child completed replay в nested proxy. Повторная приёмка учитывает итоговый forwarding fix, а не предыдущую безусловную проверку reserved metadata.

Private `Chunk.replayProof` содержит defensive snapshot всего chunk; mark создаётся библиотечным replay path. Сравнение исключает только `CallID`/`ToolName`, поэтому scoped correlation rebinding допустим, а wire payload, typed value, effects, controls, MIME, audience, metadata и result flags остаются связанными. Policy envelope overlay обновляет proof только когда исходный chunk имел валидный proof. JSON/history encoding не экспортирует private proof; decoded tool metadata само по себе не является доказательством.

Расширенный публичный consumer probe `TestPublicForwardedReplaySnapshot` получает настоящий child result-cache replay, затем передаёт его через `NewProxyTool`:

- exact forwarding и correlation rebinding — PASS, без отказа;
- изменённые effects, Data, TypedResult, audience, metadata, controls и empty flag — все отвергнуты `reserved_replay_metadata`, output/effects не доставлены;
- исходные manual spoof и OperationProfile uncertainty probes по-прежнему PASS.

Итого 7 adversarial tests, включая 9 forwarding subcases, прошли с `-race -count=1`. Полный root race результат `/tmp/task36-core-final.log` просмотрен: PASS. Подтверждённых незакрытых дефектов в итоговом проверенном scope не обнаружено; прежняя forwarding регрессия устранена. Глобальные make gates и post-release consumer остаются отдельными обязательными доказательствами.
