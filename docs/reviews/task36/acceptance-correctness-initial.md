# Task36 — независимая первоначальная приёмка корректности

Проверен текущий worktree 2026-10-07: `examples/host_dispatch/recipe`, optional adapters/semantic fixtures, `docs/host-dispatch-contract.md`, текущий CI и integration script. Реализация и CI ещё дорабатывались параллельно. Это первоначальная приёмка, не final release gate.

## Подтверждённые findings

### F1 — P1: результат handler может подделать replay provenance и потерять fresh effects

`recipe/dispatch.go:150–156` принимает metadata `toolsy.replay_source` как доказательство replay. Публичный typed handler может выставить это поле через `ToolResult.EnvelopeMetadata`. На свежем выполнении core передаёт его без запрета/очистки; handler запускается, но reducer не вызывается.

Воспроизведение: `TestFreshForgedReplayDropsEffects` создаёт настоящий `NewTypedTool`, scoped view, recipe dispatcher без cache/profile и возвращает один effect плюс `EnvelopeMetadata{toolsy.ReplaySourceMetadata: toolsy.ReplaySourceOperation}`. Наблюдение: `decision=continue`, `calls=1`, `effects=0`. AC4 и обещание trusted replay provenance нарушены. Необходим regression на fresh boundary; host должен различать происхождение metadata и handler payload.

### F2 — P2: классификация не реализует полный приоритет control + error

`recipe/dispatch.go:192–202` возвращает инфраструктурный fault до проверки фактических outcome controls, хотя таблица контракта ставит Halt/Pause/Yield выше fault. `Classify(ToolOutcome{Controls:[HaltSignal]}, errors.New("infrastructure"))` даёт `fault`, контракт требует `halt`.

Дополнительно, `recipe/dispatch.go:222–235` возвращает решение из partial controls до проверки control sentinel: `Classify(ToolOutcome{Controls:[PauseSignal]}, toolsy.ErrHalt)` даёт `pause`, хотя более приоритетный halt присутствует в отдельном канале. Оба пути сохраняют dispatch barrier, но выбирают неверный lifecycle переход для host. Regression должен объединять фактические controls из обоих каналов, сохраняя precedence uncertainty/approval.

### F3 — P2: error и empty/noop projection обходят MIME boundary

`recipe/dispatch.go:277–280` обходит MIME validation, а строка 298 копирует исходный MIME в model JSON. `Project(..., EmptyResult:true, MIME:"application/x-secret", decision:"continue")` возвращает успешную projection вместо явно оговорённого отказа unsupported MIME.

`Project(..., MIME:'text/plain; diagnostic="SECRET_DIAGNOSTIC"', decision:"business_error")` сериализует diagnostic параметр в model response, хотя value заменён safe status. Наблюдение: `{"mime":"text/plain; diagnostic=\"SECRET_DIAGNOSTIC\"", ...}`. Error projection должна иметь собственную безопасную MIME semantics; empty/noop также нуждается в явной допустимой semantics. Для обычных success путей сохранять MIME следует после определения безопасных параметров, а не передавать произвольные альтернативные diagnostic поля.

## Исполняемые доказательства

- `env GOPATH=/tmp/toolsy-task36/gopath GOCACHE=/tmp/toolsy-task36/cache go test -race -count=1 ./examples/host_dispatch/...` — PASS.
- Adversarial consumer использует опубликованный module path с local replace текущего checkout. `env GOPATH=/tmp/toolsy-task36/gopath GOCACHE=/tmp/toolsy-task36/cache GOWORK=off go test -mod=mod -race -count=1 -v .` — выявил failures F1–F3.
- Исходник probes сохранён в `docs/reviews/task36/correctness-initial-probe_test.go.txt`. Для повторения скопировать в temporary consumer module `task36probe`, добавить require Toolsy и replace на проверяемый checkout (путь с пробелами в кавычках), запустить указанную команду. Probes нарочно ожидают контрактное поведение и должны стать PASS после исправлений.

## Ограничения проверки

Просмотрены identity admission, stable operation/profile binding, scoped manifests, approval/resume fixtures, audience filtering и authorized replay. Существующие race tests покрывают повтор completed intent, barrier, deny/revoke, grant edits, concurrent resume и reopen. Подтверждённых дополнительных дефектов в этом просмотренном scope не найдено; это не утверждение отсутствия всех ошибок.

Integration local/published modes и CI дорабатывались во время проверки; их обязательный final semantic gate здесь не подтверждён. Текущий CI на момент чтения содержал общий module matrix без task36 двухрежимного job. Старые pins optional module на тот момент не содержали требуемых новых публичных APIs и recipe; parent отдельно выполняет gates. Release/consumer smoke/closeout в эту первоначальную приёмку не входили.
