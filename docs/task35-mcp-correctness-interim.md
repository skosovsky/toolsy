# Task 35 — MCP migration correctness, interim review

Дата: 2026-10-04. Независимый adversarial review; основной final correctness report пока не изменён. Production и tests не изменялись этим аудитором. Это промежуточный snapshot во время переноса legacy tests, не final acceptance и не утверждение об отсутствии ошибок.

## Подтверждённая находка

### MCP-COR-001 — P2: неполный SSE frame на EOF принимается как успешный terminal response

`mcp/transport_http.go:996–998` вызывает `dispatch()` после завершения scanner, даже если последняя пустая строка, завершающая event, не была получена. Тем самым правильный JSON внутри оборванного SSE event становится полноценным ответом. Аналогично возможен dispatch незавершённого ACK/notification.

Нормативный [WHATWG SSE parsing contract](https://html.spec.whatwg.org/multipage/server-sent-events.html#event-stream-interpretation) требует отбросить pending data на EOF до финальной пустой строки. Это framing invariant, не legacy session/GET/resume semantics. Старый `TestStreamableHTTP_SSEDiscardsIncompleteEventAtEOF` проверял этот существенный invariant, хотя его resume-specific assertions больше не нужны.

Воспроизведение: `/private/tmp/toolsy-mcp-sse-eof-proof.go`, выполнен `go run` с exit 0. Public `NewStreamableHTTPTransport` → `Start` → `PrepareRequest(server/discover)` с корректными современными metadata → `Deliver` → `Await`; httptest сервер возвращает `text/event-stream` с `data: {"jsonrpc":"2.0","id":<correlated>,"result":{"resultType":"complete"}}` без завершающей пустой строки. Вывод:

```text
unterminated SSE result={"resultType":"complete"} error=<nil>
```

Ожидание: event не dispatch-ится; pending request завершается typed framing/EOF-before-terminal error, не success. Не возобновлять stream и не вводить GET fallback.

Regression checks: no-final-newline и one-final-newline (LF/CR/CRLF), fragmented reader; terminal response не завершает pending успешно, truncated request-scoped progress/log/ACK не вызывает notification callbacks; правильно завершённый event с пустой строкой по-прежнему работает. Завершённый terminal event не требует дополнительного EOF для завершения вызова.

## Проверенный scope

- PreparedRequest registration-before-wire, exactly-once Deliver/Abort, cancellation/OnComplete lifecycle.
- Peer pending map vs close, strict JSON/UTF-8/envelope validation, numeric ID exact canonicalization, cancellation ranges and bounded fragmentation, malformed correlated responses.
- Stdio queue cancellation vs active writes, onSent verdict ordering, byte budgets, detached process lifetime, stdout limits, bounded stderr, process-tree close/reaping.
- Streamable HTTP POST-only paths, current routing descriptor snapshot, safe client composition, decorator request reconstruction, exact Content-Type/status/correlation gates, per-request SSE provenance, ACK ordering, scoped notifications, concurrent POST termination/Close ordering.
- Relevant independent invariants in old untracked adversarial/peer/stdio/HTTP suites and existing modern task34 suites. Obsolete positive initialize/server-request/GET/session/resume behavior is not a target for preservation.

## Checks to retain when porting old suites

These are required regression candidates, not additional confirmed production bugs:

1. Peer beginRequest/Close race leaves no pending waiter; completed duplicate responses must not be mistaken for a once-allowed late cancelled response; out-of-order cancellation range insertion/bridging/splitting stays sorted and bounded. Keep numeric-vs-string ID separation and exact large-integer code handling. Do not restore server-initiated request replies/handlers.
2. Process exit unblocks waiters with typed crash even when descendants keep stdout/stderr open; live process that closes stdout is terminated/reaped; high-volume/long stderr never causes transport crash. Existing two `stdio_process_task34_test.go` process tests cover crash and concurrent tree close, but do not independently establish every one of those races.
3. Queued cancelled write is retracted without cancelling an unrelated active write; write loop rechecks context after activation; a successful active write is marked finished before delivery callback. Frame and aggregate queued-byte limits release budgets under races. Active write cancellation is not equivalent to retracting an already delivered request.
4. Decorator cannot mutate URL/Host/method/body or reserved routing headers, and retaining/mutating its request pointer cannot change the detached wire request. SSRF checks survive custom-client injection and redirects; method-changing redirect remains forbidden. Exact media types, body bounds, UTF-8, strict correlated JSON/non-2xx responses and Close cancelling all held POSTs remain independent checks.
5. POST SSE obeys event framing and fragmented BOM/CR/LF/CRLF handling; request provenance rejects cross-request progress/notifications, subscription updates before ACK and wrong terminal subscription ID. No resume/id/session behavior should return with these tests.
6. Client callback cancellation emits at most one stdio cancellation for the actual delivered request; invalidation aborts in-flight schema snapshots; dynamic numeric outputs remain exact; progress preserves fractional values and rejects regressions. Current API resultType/cache/meta contracts replace obsolete fixtures rather than weakening assertions.

## Limitations and acceptance

Main implementation is still changing its legacy tests. No all-module final gate result is asserted here; no production fixes, release, issue closure, or main final report update was performed. Once candidate is frozen: rerun proof and scoped race regressions, review resulting migrations, then independent final correctness and completeness checks plus mandatory all-module gates.

Potential credential forwarding under HTTP redirects was inspected but is not filed as a confirmed migration regression: current redirect policy intentionally validates SSRF/method without endpoint credential isolation, while remote authorization profile is separately deferred. This review does not claim authorization/token-audience conformance.

## Targeted recheck — 2026-10-05

Это проверка локальных исправлений во время незавершённой миграции. Production/tests не изменялись аудитором; основной final report не обновлялся.

### MCP-COR-001: исправление подтверждено

На текущем snapshot после scanner EOF больше нет dispatch накопленного pending event. Повторный public `/private/tmp/toolsy-mcp-sse-eof-proof.go` возвращает `stream ended before terminal response`, не успешный result. `transport_sse_eof_migration_test.go` проверяет terminal без окончания/с одним LF/CRLF и unfinished ACK/progress; positive framed control отдельно подтверждает ровно один handler invocation и ACK state только после настоящего frame boundary. Предложенное дополнительное разнообразие CR-only/fragmented framing остаётся проверкой final candidate, не подтверждённым остаточным дефектом.

### Empty method notification: локальное исправление подтверждено

До изменения `peer.dispatchEnvelope` разрешал пустую строку через `requireStringField(fields, "method", true)` и возвращал nil для notification без registered handler. При этом `Notification.UnmarshalJSON` уже отклоняет `v.Method == ""` (`mcp/protocol_validate.go:2358`): существовал dispatch/DTO validation drift. Новый `allowEmpty=false` устраняет конкретный bypass до notification lookup/callback; request и notification не получают автоматических reply и возвращают InvalidPayloadError. `TestRPCPeer_EmptyMethodFailsClosed` подтвердил оба варианта и zero sends. Проблема относится к strict validation (P2), не к возвращению server-initiated request support.

### Stdio cumulative reader error precedence: локальное исправление подтверждено

Старый readLoop dispatch-ил каждый Scanner token до проверки Scanner.Err. Scanner может выдать финальный обрезанный token вместе с non-EOF reader error: framing JSON validation маскировала authoritative aggregate read limit как unexpected EOF. Внешний независимый `/private/tmp/toolsy-mcp-reader-limit-proof.go` использует тот же exported LimitStreamReaderWithContext и Scanner: 10 padding frames, 300 chars, cap2048. Вывод `old dispatch jsonErr=unexpected end of JSON input scannerReadLimit=true tokenLen=223`; новый guard отвергает token до JSON (`new guard rejectsBeforeJSON=true`). Это P2 потеря typed resource-boundary error; произвольный partial token не должен dispatch-иться.

Новый `scanner.Err()` guard до dispatch single-reader local и не меняет callback/queue locks. Ordinary EOF по-прежнему даёт Scanner.Err=nil; successfully scanned normal tokens не отклоняются из-за EOF. Cancellation/close may race, но peer.complete/close остаются exactly-once и guard не вводит независимого второго terminal completion. `TestStdioTransport_StdoutExceedsMaxStreamBytes` теперь подтверждает errors.Is(ErrReadLimitExceeded), не только наличие любого error. Подтверждённых новых bypasses/races от этого guard не найдено.

### Выполненная диагностика

Из `mcp`: `go test -race $(go list -f '{{join .GoFiles " "}}' .) peer_test.go transport_stdio_test.go transport_http_migration_test.go transport_sse_eof_migration_test.go -count=3` — terminal PASS (`command-line-arguments`, 1.552s). Это production current-platform GoFiles и выбранные переносимые suites, **не package/full-module/all-module acceptance**: другие старые fixtures ещё мигрируются. После freeze нужен полный независимый final audit и обязательные workspace gates.

## Targeted progress/empty-result checkpoint — 2026-10-05

Независимо просмотрены изменения `runMCPToolCall`, `handleProgress`, `prepareParams`, peer completion ordering, typed result/resource chunk builders и fakeTransport lifecycle. Подтверждённых новых production defects в этих изменениях не найдено; это не broad final acceptance.

Progress route установлен до подготовки; Completion hook зарегистрирован до Deliver. Retirement и enqueue сериализованы одним state.mu; enqueue использует nonblocking select, поэтому holding state.mu не блокирует wire completion при заполненном буфере. Hook удаляет именно текущий state через CompareAndDelete, а stale handler, уже получивший pointer, после lock увидит retired. Built-in pending callbacks исполняются до release Await; после получения finalCh producer retired, поэтому draining buffered pre-terminal progress не теряет уже принятые события и не принимает поздние. Yield выполняется без state.mu, consumer abort не вызывает lock recursion.

Отсутствие CompletionPendingRequest приводит к Abort до Deliver, а не к молчаливой смене wire authority или выполнению call без progress lifecycle. Новый текущий prepareParams сохраняет transport-owned identity/capabilities и typed ProgressToken без второго JSON rewrite. Удаление unused requestWithProgress не открывает reserved metadata injection.

Empty tool payload теперь имеет пустой MIME вместе с EmptyResult и сохранённым typed MCP result/envelope. Empty single text/blob resource также имеет пустой chunk/envelope MIME, а исходный typed resource сохраняет wire MIME и contents; delivery class остаётся семантикой typed value. Consumer abort возвращает одновременно ErrStreamAborted и исходную cause через wrapping, не маскирует её.

Независимая диагностика из `mcp`: `go test -race $(go list -f '{{join .GoFiles " "}}' .) task34_contract_test.go test_support_test.go test_support_migration_test.go client_contract_test.go progress_terminal_migration_test.go transport_http_migration_test.go -count=5` — terminal PASS (`command-line-arguments`, 6.992s). В том числе held-Await late progress suppression, accepted fractional monotonic event draining, no-completion abort before delivery, progress/result consumer cause, три empty-resource core-contract cases. **Это выбранные suites, не full package или mandatory all-module gate.**

Final candidate review должен отдельно проверить контракты cancellation/cleanup custom pending без optional CancellablePendingRequest: наличие Completion само по себе не является local-cancellation guarantee. Здесь подтверждённого нового regression по этому потенциальному расширению scope не заявлено. Подсчёт миграционных counterparts и immutable original inventory этим review не изменялись.

## Subscription URI targeted review — 2026-10-05

### MCP-COR-004 — P2: пустой fragment расширяет exact-only scope до subtree

`mcp/client.go:1971–1973` запрещает descendant matching только при nonempty parsed Fragment. Go url.Parse не сохраняет отдельный ForceFragment marker: URI, оканчивающийся `#`, имеет пустой Fragment как URI без fragment. Новый documented contract требует query/fragment-bearing subscriptions matching exact only. Поэтому delimiter-only fragment нельзя молча терять при subtree routing.

Подтверждено новым независимым `mcp/subscription_uri_adversarial_migration_test.go` (добавлен по явному поручению parent; runtime не изменялся):

- `https://e.test/root#` разрешает update `https://e.test/root/child` — true вместо false.
- `https://e.test/root` разрешает update `https://e.test/root/child#` — true вместо false.

Причина: проверка parse.Fragment не различает absent и explicitly empty fragment. Следует сохранять raw fragment-presence для обоих URI до normalization и запрещать descendant route с таким delimiter, сохраняя opaque exact match и точные эквивалентные URI по выбранному контракту. Это notification routing overscope, не доказанный authentication/ACL bypass: библиотека прямо не объявляет эти rules authorization.

Диагностика: production current-platform GoFiles + independent test file `go test -race ... -count=1` — RED ровно указанные два subtest; остальные 20 fixtures PASS. Они покрывают encoded slash as data (including hex normalization), literal-vs-encoded separator, encoded dot traversal and encoded slash traversal, userinfo/password/origin/scheme/nondefault-port differences, default HTTPS port equivalence, nonempty/empty query, nonempty fragment, exact empty fragment, repeated slash distinction, invalid percent escape/malformed relative, opaque exact and prefix. Full package/all-module acceptance не заявляется; original inventory/denominator не менялись.

### MCP-COR-004 fix recheck

Текущее исправление сохраняет raw fragment delimiter-presence отдельно для subscription и candidate. Exact normalized matching требует одинаковое наличие delimiter, descendant matching запрещает delimiter в любом URI. Независимый тест расширен до 28 fixtures: empty-vs-absent в обоих направлениях, normalized identity с empty fragment, encoded `%23` как path data (не ложный fragment), одинаковый и различающийся nonempty fragment. Production не изменялась аудитором. Scoped production GoFiles + adversarial URI test `-race -count=5` — PASS. **MCP-COR-004 закрыта в проверенном routing scope; общего final acceptance нет.**

## Connect Start failure / stdio ownership — 2026-10-05

### MCP-COR-005 — P2: Connect не освобождает ресурсы при ошибке Transport.Start

`mcp/client.go:187–188` возвращает Start error без Transport.Close, хотя discovery failure и последующий Client.Close очищают этот же transport. Public Transport contract не требует atomic Start или обязательного self-rollback до возвращения error. Поэтому custom transport, частично открывший соединение/reader/process перед ошибкой, остаётся без cleanup из Connect, а Client возвращён nil и не может быть закрыт вызывающим кодом.

Независимый public repro `/private/tmp/toolsy-mcp-connect-start-proof.go`: Transport.Start открывает ephemeral local TCP listener и возвращает конкретную start error; PrepareRequest panic гарантирует отсутствие discovery. После Connect listener остаётся доступен, Close не вызывался: `clientNil=true connectErr=start failed after acquiring listener closeCalls=0 acquiredListenerStillOpen=true`. Proof самостоятельно закрывает listener через defer после измерения; никакие ресурсы не оставлены.

Original `TestConnect_StartFailureClosesTransport` остаётся необходимым lifecycle invariant, а не obsolete initialize fixture. Требуется сохранить once-attempted-Start cleanup и оригинальную error cause; не переносить ownership на transport при configuration validation error до Start без явного изменения контракта. Регрессия должна использовать partial-resource fake/custom Transport и проверять Close once, nil Client, original Start cause и отсутствие discovery/delivery.

Stdio active-write cancellation не является rollback доставленных байтов и не разрешает глобальный shutdown unrelated requests. Новый migrated contract сохраняет prompt Await(ctx) cancellation, delivery verdict after actual write, queued request retraction и explicit Close, который закрывает pipes/process tree и ждёт writer/reader/stderr/process completion. Проверки process/writer завершения за 5 секунд и unaffected concurrent request нельзя удалять. Это изменение не следует считать waiver bounded Close cleanup; bounded user cancellation не тождественно forced transport destruction.

Независимо выполнены scoped current-platform production GoFiles + task34/test_support/HTTP migration helpers + `transport_stdio_contract_test.go` + `process_exists_unix_test.go`, six selected lifecycle tests (blocked active write, queued-vs-active cancellation, closed stdout, descendant-held stdout/stderr, process-tree close), `-race -count=3`: PASS 17.822s. Первая diagnostic invocation без platform process helper не собиралась; добавлен правильный helper без production/test изменений. Это не общий package/all-module gate.

### MCP-COR-005 fix recheck

После contract-first ownership documentation и `_ = transport.Close()` в Start failure branch повторный public proof вернул `closeCalls=1 acquiredListenerStillOpen=false`, исходная Start error сохранена. **MCP-COR-005 закрыта**; partial acquired resource освобождён до возврата nil Client. Runtime аудитором не изменялась.

## MCP-COR-006 — P2: три list API обходят typed read-limit mapping

`ListResources`, `ListResourceTemplates`, `ListPrompts` возвращают requestAndAwait error напрямую (`mcp/client.go:1488,1510,1558` на reviewed snapshot), в отличие от `listToolsPage` и GetPrompt. Это теряет публичную typed validation boundary (domain subject/cap), хотя исходный raw read-limit sentinel остаётся. Original GetPrompts read-limit ожидания нельзя ослаблять: issue не относится к удалённым initialize/session API.

Независимый public repro `/private/tmp/toolsy-mcp-list-limit-proof.go`: custom Transport корректно возвращает current discovery, declares resources/prompts и MaxStreamBytes2048; subsequent prepared requests Await возвращают ErrReadLimitExceeded. Все три public list API печатают `typed=false limit=true interrupt=false`. Второй набор с errors.Join(context.Canceled,limit) остаётся original interrupt chain — это правильный приоритет, который исправление обязано сохранить.

Targeted remedy: существующий `mapCallReadLimitFor(ctx, err, preciseSubject)` на этих request-error branches. Helper уже проверяет caller ctx и IsContextInterrupt до read-limit conversion. Regression: все три API имеют validation ToolError, exact domain subject/cap и исходный sentinel chain; joined/wrapped interrupts не переименовываются validation errors и не теряют исходную cause. Общего acceptance не заявлено; production/tests аудитором не изменялись.

### MCP-COR-006 fix recheck

Текущие три list branches применяют общий mapper с разными precise subjects. Независимый public proof теперь выводит validation ToolError с 2048 cap для всех трёх API, errors.Is(read-limit)=true; joined cancellation+limit остаётся unmapped interrupt. Дополнительная original cause с marker `DO-NOT-LEAK-secret` сохраняет diagnostic chain, но marker не появляется в Error()/Reason — они используют bounded validation reason.

Mapper копирует наружный freshly mapped ToolError и joins original error только в Err. Original/shared instance не изменяется; ToolError.Error при nonempty Reason не печатает Err, AsToolError возвращает ближайший наружный validation ToolError прежде вложенных causes. Interrupt priority выполнен до mapping и join. Сохранение cause в Unwrap является намеренной host diagnostics boundary, не обещанием сокрыть cause от явно обходящего chain host.

Независимые current-platform production GoFiles + task34/test_support/HTTP migration/client_contract helpers + client_test selected list/interrupt/start-failure regressions `-race -count=5` — PASS. Первая diagnostic invocation без mustJSON helper не собиралась; исправлен выбор helper, не assertions/runtime. **MCP-COR-006 закрыта локально; final package/workspace gates и broad audit всё ещё обязательны.**

## MCP-COR-007 — P2: POST SSE scanner max маскирует byte-budget boundary

`consumeRequestSSE` задаёт Scanner max равным maxStreamBytes. При одной oversized line Scanner достигает max до следующего обращения к authoritative limited reader и возвращает bufio.ErrTooLong; error chain не содержит ErrReadLimitExceeded. Client mapper поэтому возвращает InvalidPayload вместо typed validation subject/cap.

Независимый public transport proof `/private/tmp/toolsy-mcp-http-line-limit-proof.go`: httptest POST SSE `data: x256`, streamcap32, valid modern discovery request; вывод `limitCause=false error=mcp: invalid Streamable HTTP SSE payload: bufio.Scanner: token too long`. `Client.discover` уже вызывает mapper, но raw chain не распознаётся как limit.

Предпочтительное local remedy: Scanner имеет небольшое bounded probe headroom относительно maxStreamBytes, а настоящий limited reader остаётся authoritative. Не классифицировать произвольные scanner/reader errors как read limit; caller context interrupt priority сохранить первым. One-byte headroom достаточно дать capped reader выполнить следующий Read, но current limitedStreamReader возвращает ErrReadLimitExceeded при n>=max без проверки actual EOF. Поэтому exact-cap unfinished line тоже fail-closed budget exhaustion, а below-cap unfinished line — EOF-before-terminal. Нельзя утверждать exact-cap EOF discrimination без изменения shared reader contract и отдельного аудита. Подтверждённое finding пока открыто до проверки выбранного fix и граничных regression cases.

### MCP-COR-007 fix recheck

Текущий scannerLimit имеет one-byte headroom с math.MaxInt overflow guard, reader budget не увеличен. Budget exhaustion классифицируется reader, а не blanket conversion произвольного scanner error. Ни extra readable byte, ни resume/GET path не добавлены. Exact-cap fail-closed exhaustion явно документирован; below-cap unfinished event остаётся EOF-before-terminal.

Независимые external public proofs: transport oversized line32/256 теперь `limitCause=true`; Connect на том же actual HTTP server возвращает validation ToolError с точным `MCP server discovery response exceeds 32 byte limit`, исходный sentinel сохранён; no-final-empty-line SSE proof по-прежнему возвращает EOF-before-terminal вместо success.

Actual MCP package выбранные scanner/framing tests `go test -race ./... -run 'Test(StreamableHTTP_SSELineBudgetOwnsScannerFailure|StreamableHTTP_OversizedPOSTSSEFailsClosedWithoutRetry|MigratedSSEDiscardsUnfinishedTerminalAtEOF|MigratedSSEUnfinishedNotificationsDoNotDispatch|Task34SSEAcceptsBOMLineEndingsAndFragmentedFrames|Task34SSERejectsTruncatedUTF8AcrossFragments)' -count=5` — PASS 1.383s, examples builds included. Проверены below/exact/above cap, pre-cancel priority, no retry, fragmented line endings/UTF-8, unfinished ACK/progress not dispatch and positive controls. **MCP-COR-007 закрыта targeted; это actual package compile плюс выбранные tests, не full-suite/all-module acceptance.** Runtime/tests аудитором не изменялись.
