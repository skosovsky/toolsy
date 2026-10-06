# Task35 — independent MCP migration inventory (before rewrites)

Interim disposition, not acceptance PASS. Baseline sources: immutable backup `/private/tmp/toolsy-mcp-pre-migration.MHKAqL`. Read all 11 requested files, 5863 lines, 198 Test-named functions: **197 acceptance tests + 1 subprocess helper**; test_support has no Test. Main may rewrite concurrently; this report anchors original assertions, not live moving files. No production/tests edited by this auditor.

Clear break is authorized. Historical Contract-First deviation is user-accepted, stays in full denominator as accepted-deviation; not retroactively proven original chronology. Full workspace make test/lint and final dual audits remain required after migration. This inventory does not revise Task35's 121-row denominator or mark delivery complete.

## Disposition rules

- `migrate`: original invariant remains required, equivalent exact coverage of the entire Test has NOT been established. Rewrite current ports/types with the original fixture/strength; partial neighbor coverage does not close it.
- `security invariant covered by exact assertion`: specific inspected tracked assertion covers semantic invariant; obsolete lifecycle details explicitly excluded, not quietly deleted.
- `obsolete forbidden behavior replaced negative assertion`: do not revive API. Replacement negative must prove rejection/no wire/no authority. Where replacement not already exact, note is an OPEN obligation, not completion.
- `retained` would require unchanged current-contract assertions; no such blanket designation used for old files.
- `missing` is an unsatisfied invariant after migration; before rewrites it is listed as open migrate/replacement, not hidden under green compilation.

Every named Test appears exactly once below. Original assertion ledger and literal table/inline-range case inventories are included so dynamic `t.Run` values cannot disappear behind a single test count. Ledger is source evidence, **not instructions to preserve forbidden expected positives**; explicit disposition wins. Setup assertions included intentionally. Non-table single scenarios remain named by original Test and source line; full source is immutable backup for exact arrangement.

## Exact inspected tracked evidence

| Current test | Exact evidence / limitation |
|---|---|
| TestConnectUsesStrictDiscoveryAndSelfDescribingMetadata | server/discover only; current protocol/capabilities/clientInfo in _meta; requests len1. |
| TestConnectRejectsOldOnlyServerWithoutFallback | nil client, typed version error, discover len1, transport closed; no fallback. |
| TestTransportBoundaryRejectsLegacyMethodsAndNotifications | Error+nil pending for initialize/initialized/roots/logging-set-level/resources-subscribe/unsubscribe/ping; initialized/roots notifications errors. Outbound boundary only, not every incoming fixture. |
| TestStreamableHTTPUsesPostOnlyRoutingAndSentinelHeaders | methods exactly POST; current protocol/name/header bindings; empty session/Last-Event-ID. Close no-DELETE not explicitly captured after Close in this test. |
| TestRequestScopedSSEFailsOnEOFAndByteLimit | EOF before terminal response, oversized line errors. Not cumulative50events nor no-retry network count. |
| TestTask34SSEAcceptsBOMLineEndingsAndFragmentedFrames | LF/CR/CRLF + BOM + fragmented read → successful correlated complete result. No resume cursor. |
| TestTask34SSERejectsTruncatedUTF8AcrossFragments | UTF8 truncated over1-byte reads→error; JSON invalid UTF8 separately migrate. |
| TestTask34StdioCrashUnblocksWaiterAndReapsProcess | typed crash+exec.ExitError, processDone, writerDone/readerDone/stderrDone closed. Inherited descriptors require distinct fixtures. |
| TestTask34StdioConcurrentCloseKillsDescendantProcessTree | 16 concurrent Close noerror; child reaped; all workers closed. |
| TestStdioBlockingWriteCancellationPreservesDeliveredFrameAndTransport | WasSent true, cancellation claim1, discover/cancel/alive frames exact; no kill/truncation. |
| TestStdioCancellationRetractsQueuedRequestWithoutNotification | WasSent false; queued discover/cancel absent; active blocker+alive retained. |
| TestAwaitCancellationNotifiesExactlyOnceAfterDelivery | canceled caller + duplicate cancellation → notification count1. Not trace/deadline/stalled DeliveryDone/terminal variants. |
| TestResourceSubscriptionAllowsOnlyExactOrPathDescendantURI | exact/child/prefix/origin/query/dot/%2e%2e boundary; legacy14-entry matrix broader. |
| TestPublicCallToolValidatesAuthoritativeOutputSchema | authoritative listed descriptor; invalid structured integer rejected in public CallTool and proxy builder. Not exact large minimum. |
| TestTask34RandomizedIntegralRPCIDsCanonicalizeWithoutStringCollision | 512 values, decimal/.0/e0 equality, quoted string distinct, .5 rejected; not all huge exponent/range tests. |
| TestTask34CancelledIDFragmentationRemainsBoundedAndFailsClosed | remove/split bound exact max ranges; no proof of all insertion/bridging/late duplicate cases. |

Current runtime inspected: HTTP Start validates endpoint/no GET; PrepareRequest checks pre-cancel+outgoing contract; Notify forbidden; buildRequest reconstructs POST and clones decorated headers, rejects target/Host/body mutation+reserved headers. Stdio Start detached request-driven lifetime. peer.dispatchEnvelope rejects server-initiated requests without any responder pool; correlated malformed responses fail pending. This is why resurrecting old server callbacks, roots, GET/resume, notification POST or kill-on-active-cancel is wrong.

## Per-file inventory

| Original file | Test functions | Checks/helper | Dispositions |
|---|---:|---|---|
| adversarial_contract_test.go | 51 | 51 checks | migrate:37; obsolete forbidden behavior replaced negative assertion:12; security invariant covered by exact assertion:2 |
| client_contract_test.go | 23 | 23 checks | obsolete forbidden behavior replaced negative assertion:12; migrate:9; security invariant covered by exact assertion:2 |
| client_features_test.go | 13 | 13 checks | migrate:13 |
| client_test.go | 25 | 25 checks | migrate:24; obsolete forbidden behavior replaced negative assertion:1 |
| peer_test.go | 15 | 15 checks | obsolete forbidden behavior replaced negative assertion:7; migrate:8 |
| protocol_contract_test.go | 20 | 20 checks | migrate:20 |
| test_support_test.go | 0 | helper only | migrate helper contract |
| transport_http_test.go | 5 | 5 checks | obsolete forbidden behavior replaced negative assertion:3; migrate:2 |
| transport_sse_test.go | 19 | 19 checks | migrate:16; obsolete forbidden behavior replaced negative assertion:3 |
| transport_stdio_contract_test.go | 16 | 15 checks + helper | migrate:14; security invariant covered by exact assertion:2 |
| transport_stdio_test.go | 11 | 11 checks | migrate:11 |

## Helper contract — test_support_test.go

Migrate fakeTransport to PrepareRequest returning PreparedRequest with explicit Deliver/Abort and current handler signatures; capture preparation separately from delivery, byte-clone request ID/params/result; preserve requestHook, captured notifications, Close counter, completion hooks, delivery barrier/cancel claim. FakePending OnComplete must run exactly once on completed race and support background Await unblocking. Replace initializeResult with completeDiscovery; remove SetProtocolVersion/OnRequest/old Call aliases. Do not silently make every fake always sent or always canceled: terminal/stall/queued tests need faithful delivery state. client_test's old captureTransport and mustInitializeResultJSON similarly replace, avoiding duplicate mustJSON/helper names. Subprocess helper modes are acceptance fixtures, not deletable duplicates.

## Test-by-test dispositions and source ledger


### TestRPCPeer_BeginRequestCloseRaceNeverLeaks

Original: `adversarial_contract_test.go:23` in immutable backup. Disposition: **migrate**.

Peer safety invariant остаётся: сохранить каждую correlation/cancellation/range/duplicate/error-code/frame assertion ниже. Internal beginRequest тест допустим; production callers use validated PreparedRequest. Перенос имён/helpers допустим, weakening/range-count reduction нет. Generated randomized ID/fragmentation tests дополняют, не заменяют весь deterministic ledger.

Original assertion obligations:

```go
require.ErrorIs(t, err, ErrTransportClosed)
```

### TestRPCPeer_IncomingRequestConcurrencyIsBounded

Original: `adversarial_contract_test.go:57` in immutable backup. Disposition: **obsolete forbidden behavior replaced negative assertion**.

Current peer.dispatchEnvelope rejects every server-initiated request with InvalidPayload; no handler pool/responder API. Preserve each malformed input and assert rejection + no outbound response/notification callback; notification same method remains independent only for allowed extension. Existing legacy boundary rejects outbound legacy methods but does NOT prove all incoming-request fixtures; required migration negatives remain open.

Original assertion obligations:

```go
require.NoError(t, json.Unmarshal(body, &response))
require.NoError(t, peer.dispatch(fmt.Appendf( nil, `{"jsonrpc":"2.0","id":%d,"method":"ping"}`, id, )))
require.Eventually(t, func() bool { return started.Load() == maxConcurrentIncoming }, time.Second, time.Millisecond)
require.NoError(t, err)
require.NotNil(t, overload.Error)
require.Equal(t, JSONRPCServerOverloaded, overload.Error.Code)
```

### TestRPCPeer_RejectsOversizedRequestIDBeforeNumericCanonicalization

Original: `adversarial_contract_test.go:95` in immutable backup. Disposition: **migrate**.

Peer safety invariant остаётся: сохранить каждую correlation/cancellation/range/duplicate/error-code/frame assertion ниже. Internal beginRequest тест допустим; production callers use validated PreparedRequest. Перенос имён/helpers допустим, weakening/range-count reduction нет. Generated randomized ID/fragmentation tests дополняют, не заменяют весь deterministic ledger.

Original assertion obligations:

```go
require.Error(t, err)
```

### TestRPCPeer_ResponseCancellationRaceConsumesOneTerminalResponse

Original: `adversarial_contract_test.go:106` in immutable backup. Disposition: **migrate**.

Peer safety invariant остаётся: сохранить каждую correlation/cancellation/range/duplicate/error-code/frame assertion ниже. Internal beginRequest тест допустим; production callers use validated PreparedRequest. Перенос имён/helpers допустим, weakening/range-count reduction нет. Generated randomized ID/fragmentation tests дополняют, не заменяют весь deterministic ledger.

Original assertion obligations:

```go
require.NoError(t, err)
require.NoError(t, dispatchErr)
require.ErrorIs(t, awaitErr, context.Canceled)
require.Error(t, duplicateErr)
```

### TestRPCPeer_ExternalCancellationUnblocksBackgroundAwait

Original: `adversarial_contract_test.go:143` in immutable backup. Disposition: **migrate**.

Peer safety invariant остаётся: сохранить каждую correlation/cancellation/range/duplicate/error-code/frame assertion ниже. Internal beginRequest тест допустим; production callers use validated PreparedRequest. Перенос имён/helpers допустим, weakening/range-count reduction нет. Generated randomized ID/fragmentation tests дополняют, не заменяют весь deterministic ledger.

Original assertion obligations:

```go
require.NoError(t, err)
require.True(t, cancelled)
require.ErrorIs(t, awaitErr, context.Canceled)
t.Fatal("external cancellation did not unblock Await")
```

### TestRPCPeer_CancelledResponseBookkeepingRemainsBounded

Original: `adversarial_contract_test.go:167` in immutable backup. Disposition: **migrate**.

Peer safety invariant остаётся: сохранить каждую correlation/cancellation/range/duplicate/error-code/frame assertion ниже. Internal beginRequest тест допустим; production callers use validated PreparedRequest. Перенос имён/helpers допустим, weakening/range-count reduction нет. Generated randomized ID/fragmentation tests дополняют, не заменяют весь deterministic ledger.

Original assertion obligations:

```go
require.NoError(t, err)
require.True(t, pending.CancelPending())
require.Equal(t, []rpcIDRange{{first: 1, last: maxCancelledResponseRanges + 16}}, peer.cancelledRanges)
require.NoError(t, peer.dispatch(retainedResponse))
require.Equal(t, []rpcIDRange{{first: 1, last: maxCancelledResponseRanges + 15}}, peer.cancelledRanges)
require.NoError(t, err)
require.True(t, pending.CancelPending())
require.False(t, peer.closed.Load())
require.NoError(t, err)
```

### TestRPCPeer_CancellationStormNeverMasksDuplicateCompletedResponse

Original: `adversarial_contract_test.go:205` in immutable backup. Disposition: **migrate**.

Peer safety invariant остаётся: сохранить каждую correlation/cancellation/range/duplicate/error-code/frame assertion ниже. Internal beginRequest тест допустим; production callers use validated PreparedRequest. Перенос имён/helpers допустим, weakening/range-count reduction нет. Generated randomized ID/fragmentation tests дополняют, не заменяют весь deterministic ledger.

Original assertion obligations:

```go
require.NoError(t, err)
require.NoError(t, peer.dispatch(completedResponse))
require.NoError(t, beginErr)
require.True(t, pending.CancelPending())
require.ErrorAs(t, err, &invalid)
```

### TestRPCPeer_OutOfOrderCancellationsRemainExactlyCorrelated

Original: `adversarial_contract_test.go:230` in immutable backup. Disposition: **migrate**.

Peer safety invariant остаётся: сохранить каждую correlation/cancellation/range/duplicate/error-code/frame assertion ниже. Internal beginRequest тест допустим; production callers use validated PreparedRequest. Перенос имён/helpers допустим, weakening/range-count reduction нет. Generated randomized ID/fragmentation tests дополняют, не заменяют весь deterministic ledger.

Original assertion obligations:

```go
require.NoError(t, err)
require.NoError(t, err)
require.True(t, second.CancelPending())
require.True(t, first.CancelPending())
require.NoError(t, firstErr)
require.NoError(t, secondErr)
require.Empty(t, peer.cancelledRanges)
```

### TestRPCPeer_CancellationRangeInsertionAndBridgingStaySorted

Original: `adversarial_contract_test.go:250` in immutable backup. Disposition: **migrate**.

Peer safety invariant остаётся: сохранить каждую correlation/cancellation/range/duplicate/error-code/frame assertion ниже. Internal beginRequest тест допустим; production callers use validated PreparedRequest. Перенос имён/helpers допустим, weakening/range-count reduction нет. Generated randomized ID/fragmentation tests дополняют, не заменяют весь deterministic ledger.

Original table/inline-range subscenario inputs (each entry requires disposition above):

```go
sequences := [][]uint64{{3, 1, 2}, {2, 3, 1}, {3, 2, 1}}
```

Original assertion obligations:

```go
require.True(t, peer.addCancelledLocked(id))
require.Equal(t, []rpcIDRange{{first: 1, last: 3}}, peer.cancelledRanges)
```

### TestRPCPeer_CancellationRangeFragmentationFailsClosedAtBound

Original: `adversarial_contract_test.go:268` in immutable backup. Disposition: **migrate**.

Peer safety invariant остаётся: сохранить каждую correlation/cancellation/range/duplicate/error-code/frame assertion ниже. Internal beginRequest тест допустим; production callers use validated PreparedRequest. Перенос имён/helpers допустим, weakening/range-count reduction нет. Generated randomized ID/fragmentation tests дополняют, не заменяют весь deterministic ledger.

Original assertion obligations:

```go
require.NoError(t, err)
require.True(t, cancelled.CancelPending())
require.NoError(t, err)
require.NoError(t, peer.dispatch(response))
require.NoError(t, err)
require.True(t, overflow.CancelPending())
require.True(t, peer.closed.Load())
require.ErrorIs(t, err, ErrTransportClosed)
```

### TestRPCPeer_LateCancelledResponseSplitsRangeAndDuplicateFailsClosed

Original: `adversarial_contract_test.go:296` in immutable backup. Disposition: **migrate**.

Peer safety invariant остаётся: сохранить каждую correlation/cancellation/range/duplicate/error-code/frame assertion ниже. Internal beginRequest тест допустим; production callers use validated PreparedRequest. Перенос имён/helpers допустим, weakening/range-count reduction нет. Generated randomized ID/fragmentation tests дополняют, не заменяют весь deterministic ledger.

Original assertion obligations:

```go
require.NoError(t, err)
require.True(t, pending.CancelPending())
require.NoError(t, firstErr)
require.ErrorAs(t, duplicateErr, &invalid)
require.Equal(t, []rpcIDRange{{first: 1, last: 1}, {first: 3, last: 3}}, peer.cancelledRanges)
```

### TestSSERetryAcceptsOnlyASCIIIntegerMilliseconds

Original: `adversarial_contract_test.go:325` in immutable backup. Disposition: **obsolete forbidden behavior replaced negative assertion**.

No resume retry controller; all 9 retry values become inert SSE fields, never delay/reconnect. Replace with POST SSE terminal/EOF fixtures proving no GET and no retry-driven lifecycle. Preserve resource boundedness; ASCII parsing helper need not survive dead controller.

Original table/inline-range subscenario inputs (each entry requires disposition above):

```go
tests := []struct {
		value string
		want  time.Duration
		ok    bool
	}{
		{value: "0", want: httpPollRetryDelay, ok: true},
		{value: "1", want: httpPollRetryDelay, ok: true},
		{value: "120000", want: 2 * time.Minute, ok: true},
		{value: "600000", want: httpMaxRetryDelay, ok: true},
		{value: "+1", ok: false},
		{value: "1.5", ok: false},
		{value: "-1", ok: false},
		{value: " 1", ok: false},
		{value: "18446744073709551615", ok: false},
	}
```

Original assertion obligations:

```go
require.Equal(t, test.ok, ok)
require.Equal(t, test.want, got)
```

### TestStreamableHTTP_CloseCompletesHeldRequestWithTransportClosed

Original: `adversarial_contract_test.go:355` in immutable backup. Disposition: **migrate**.

Перенести исходные assertions и все subscenarios без ослабления на current DTO/PreparedRequest→Deliver→Await. Полного exact tracked equivalent для этого test не установлено; это обязательство миграции, не засчитанное runtime coverage.

Original assertion obligations:

```go
require.NoError(t, transport.Start(context.Background()))
require.NoError(t, err)
t.Fatal("server did not receive held request")
require.NoError(t, transport.Close())
require.ErrorIs(t, err, ErrTransportClosed)
```

### TestStreamableHTTP_TerminalIncomingResponseErrorCannotSelfDeadlock

Original: `adversarial_contract_test.go:387` in immutable backup. Disposition: **obsolete forbidden behavior replaced negative assertion**.

Server-initiated request/response path unsupported, peer returns InvalidPayload and no server-response POST. Preserve Close nondeadlock for actual current active POST lifecycle separately; do not add OnRequest API.

Original assertion obligations:

```go
require.NoError(t, transport.Start(context.Background()))
require.NoError(t, transport.peer.dispatch( []byte(`{"jsonrpc":"2.0","id":"server-request","method":"roots/list"}`), ))
require.Eventually(t, func() bool { transport.mu.RLock() defer transport.mu.RUnlock() return transport.terminating }, time.Second, time.Millisecond)
t.Fatal("terminal response error deadlocked transport close")
```

### TestStreamableHTTP_CancellationFollowsOriginalWireRequest

Original: `adversarial_contract_test.go:428` in immutable backup. Disposition: **migrate**.

HTTP current cancellation closes originating response body; MUST NOT emit notifications/cancelled POST. Exact current TestStreamableHTTPCancellationClosesOnlyOriginatingPOST asserts requestErr canceled and methods only server/discover/tools/list; stdio ordering separately retained.

Original assertion obligations:

```go
t.Errorf("decode request: %v", err)
require.NoError(t, transport.Start(context.Background()))
require.ErrorIs(t, err, context.Canceled)
require.Eventually(t, func() bool { mu.Lock() defer mu.Unlock() return len(methods) == 2 }, time.Second, time.Millisecond)
require.Equal(t, []string{"ordered/request", MethodCancelled}, methods)
require.NoError(t, transport.Close())
```

### TestStreamableHTTP_PreCancelledRequestNeverAllocatesOrWritesID

Original: `adversarial_contract_test.go:485` in immutable backup. Disposition: **migrate**.

Перенести исходные assertions и все subscenarios без ослабления на current DTO/PreparedRequest→Deliver→Await. Полного exact tracked equivalent для этого test не установлено; это обязательство миграции, не засчитанное runtime coverage.

Original assertion obligations:

```go
require.NoError(t, transport.Start(context.Background()))
require.Nil(t, pending)
require.ErrorIs(t, err, context.Canceled)
require.Equal(t, int64(0), requests.Load())
require.NoError(t, transport.Close())
```

### TestRPCPeer_DuplicateResponseFailsClosedAndNotifyAfterCloseStops

Original: `adversarial_contract_test.go:511` in immutable backup. Disposition: **migrate**.

Peer safety invariant остаётся: сохранить каждую correlation/cancellation/range/duplicate/error-code/frame assertion ниже. Internal beginRequest тест допустим; production callers use validated PreparedRequest. Перенос имён/helpers допустим, weakening/range-count reduction нет. Generated randomized ID/fragmentation tests дополняют, не заменяют весь deterministic ledger.

Original assertion obligations:

```go
require.NoError(t, err)
require.NoError(t, peer.dispatch([]byte(response)))
require.ErrorAs(t, duplicateErr, &payloadErr)
require.ErrorIs(t, notifyErr, ErrTransportClosed)
```

### TestStreamableHTTP_InitializePOSTSSEDisconnectResumesWithPrimingID

Original: `adversarial_contract_test.go:534` in immutable backup. Disposition: **obsolete forbidden behavior replaced negative assertion**.

No GET/resume/Activate/server responses. Required replacement: request-scoped SSE EOF errors promptly, no follow-up GET/no Last-Event-ID/no initialized; server request fail closed without POST response. TestRequestScopedSSEFailsOnEOFAndByteLimit asserts EOF before terminal; TestStreamableHTTPUsesPostOnlyRoutingAndSentinelHeaders asserts single POST and empty cursor/session. Add explicit no retry after EOF; cancellation isolation remains separate.

Original assertion obligations:

```go
require.NoError(t, transport.Start(context.Background()))
require.NoError(t, err)
require.NoError(t, err)
require.NotEmpty(t, result)
require.Equal(t, "initialize-1", <-getHeader)
require.Equal(t, httpPollRetryDelay, transport.pollRetryDelay)
require.NoError(t, transport.Close())
```

### TestStreamableHTTP_SessionCannotBeReplaced

Original: `adversarial_contract_test.go:592` in immutable backup. Disposition: **obsolete forbidden behavior replaced negative assertion**.

No session establishment/replacement API: TestStreamableHTTPUsesPostOnlyRoutingAndSentinelHeaders asserts session absent. Add response header session token ignored/not forwarded and decorator reserved mcp/last-event rejection. Do not keep validateSessionID only for dead transport.

Original assertion obligations:

```go
require.NoError(t, firstErr)
require.ErrorAs(t, replacementErr, &payloadErr)
require.ErrorAs(t, lateEstablishErr, &payloadErr)
```

### TestStreamableHTTP_DecoratorCannotChangeTargetAndEmptyEventIDResetsResume

Original: `adversarial_contract_test.go:610` in immutable backup. Disposition: **migrate**.

Retain decorator cannot mutate URL/method/Host with current buildRequest(ctx,body,routing). Empty event ID/resume segment obsolete: assert it never creates GET/cursor authority. No wholesale discard of target security.

Original assertion obligations:

```go
require.ErrorAs(t, decoratorErr, &payloadErr)
require.ErrorIs(t, preservedErr, errSSEPollingBoundary)
require.Equal(t, " cursor  ", preservedID)
require.ErrorIs(t, boundaryErr, errSSEPollingBoundary)
require.Empty(t, transport.lastEventID)
```

### TestStreamableHTTP_DecoratorCannotOverrideRequestHost

Original: `adversarial_contract_test.go:637` in immutable backup. Disposition: **migrate**.

Перенести исходные assertions и все subscenarios без ослабления на current DTO/PreparedRequest→Deliver→Await. Полного exact tracked equivalent для этого test не установлено; это обязательство миграции, не засчитанное runtime coverage.

Original assertion obligations:

```go
require.Nil(t, request)
require.ErrorAs(t, err, &payloadErr)
```

### TestStreamableHTTP_DecoratorCannotSetBodyContract

Original: `adversarial_contract_test.go:656` in immutable backup. Disposition: **migrate**.

Перенести исходные assertions и все subscenarios без ослабления на current DTO/PreparedRequest→Deliver→Await. Полного exact tracked equivalent для этого test не установлено; это обязательство миграции, не засчитанное runtime coverage.

Original table/inline-range subscenario inputs (each entry requires disposition above):

```go
mutations := map[string]func(*http.Request){
		"body": func(request *http.Request) { request.Body = http.NoBody },
		"get body": func(request *http.Request) {
			request.GetBody = func() (io.ReadCloser, error) { return http.NoBody, nil }
		},
		"content length":    func(request *http.Request) { request.ContentLength = 1 },
		"transfer encoding": func(request *http.Request) { request.TransferEncoding = []string{"chunked"} },
		"trailer":           func(request *http.Request) { request.Trailer = http.Header{"X-Test": {"value"}} },
		"length header":     func(request *http.Request) { request.Header.Set("Content-Length", "1") },
	}
```

Original assertion obligations:

```go
require.Nil(t, request)
require.ErrorAs(t, err, &invalid)
```

### TestStreamableHTTP_RequestIsDetachedFromDecoratorPointer

Original: `adversarial_contract_test.go:693` in immutable backup. Disposition: **migrate**.

Перенести исходные assertions и все subscenarios без ослабления на current DTO/PreparedRequest→Deliver→Await. Полного exact tracked equivalent для этого test не установлено; это обязательство миграции, не засчитанное runtime coverage.

Original assertion obligations:

```go
require.NoError(t, err)
require.NoError(t, readErr)
require.Equal(t, "Bearer original", request.Header.Get("Authorization"))
require.Equal(t, "example.invalid", request.URL.Host)
require.JSONEq(t, `{"jsonrpc":"2.0"}`, string(body))
```

### TestStreamableHTTP_RejectsPrivateEndpointAndOversizedBody

Original: `adversarial_contract_test.go:724` in immutable backup. Disposition: **migrate**.

Перенести исходные assertions и все subscenarios без ослабления на current DTO/PreparedRequest→Deliver→Await. Полного exact tracked equivalent для этого test не установлено; это обязательство миграции, не засчитанное runtime coverage.

Original assertion obligations:

```go
require.NoError(t, bounded.Start(context.Background()))
require.NoError(t, err)
require.Error(t, privateErr)
require.Error(t, bodyErr)
require.NoError(t, bounded.Close())
```

### TestStreamableHTTP_RequestRejects202AndCloseCancelsActivePOST

Original: `adversarial_contract_test.go:751` in immutable backup. Disposition: **migrate**.

Перенести исходные assertions и все subscenarios без ослабления на current DTO/PreparedRequest→Deliver→Await. Полного exact tracked equivalent для этого test не установлено; это обязательство миграции, не засчитанное runtime coverage.

Named subscenarios: `"request rejects 202"`, `"close cancels post"`.

Original assertion obligations:

```go
require.NoError(t, transport.Start(context.Background()))
require.NoError(t, err)
require.ErrorAs(t, err, &payloadErr)
require.NoError(t, transport.Close())
require.NoError(t, transport.Start(context.Background()))
require.NoError(t, err)
require.NoError(t, transport.Close())
require.Error(t, pendingErr)
require.Zero(t, activePosts, "Close returned before the active POST stopped")
```

### TestStreamableHTTP_JSONPOSTRequiresCorrelatedTerminalResponse

Original: `adversarial_contract_test.go:804` in immutable backup. Disposition: **migrate**.

Перенести исходные assertions и все subscenarios без ослабления на current DTO/PreparedRequest→Deliver→Await. Полного exact tracked equivalent для этого test не установлено; это обязательство миграции, не засчитанное runtime coverage.

Original table/inline-range subscenario inputs (each entry requires disposition above):

```go
fixtures := []struct {
		name     string
		response string
	}{
		{
			name:     "notification",
			response: `{"jsonrpc":"2.0","method":"notifications/message","params":{}}`,
		},
		{name: "mismatched id", response: `{"jsonrpc":"2.0","id":999,"result":{}}`},
	}
```

Original assertion obligations:

```go
require.NoError(t, transport.Start(context.Background()))
require.NoError(t, err)
require.ErrorAs(t, err, &payloadErr)
require.NoError(t, transport.Close())
```

### TestStreamableHTTP_ParallelPOSTStreamsResumeIndependentlyAfterRetry

Original: `adversarial_contract_test.go:841` in immutable backup. Disposition: **obsolete forbidden behavior replaced negative assertion**.

No GET/resume/Activate/server responses. Required replacement: request-scoped SSE EOF errors promptly, no follow-up GET/no Last-Event-ID/no initialized; server request fail closed without POST response. TestRequestScopedSSEFailsOnEOFAndByteLimit asserts EOF before terminal; TestStreamableHTTPUsesPostOnlyRoutingAndSentinelHeaders asserts single POST and empty cursor/session. Add explicit no retry after EOF; cancellation isolation remains separate.

Original assertion obligations:

```go
require.NoError(t, transport.Start(context.Background()))
require.NoError(t, err)
require.NoError(t, err)
require.NoError(t, errA)
require.NoError(t, errB)
require.GreaterOrEqual(t, attempt.at.Sub(startedAt), retryDelay)
require.True(t, seen["stream-"+string(pendingA.ID())])
require.True(t, seen["stream-"+string(pendingB.ID())])
require.NoError(t, transport.Close())
```

### TestStreamableHTTP_Resume405CompletesPendingRequest

Original: `adversarial_contract_test.go:902` in immutable backup. Disposition: **obsolete forbidden behavior replaced negative assertion**.

No GET/resume/Activate/server responses. Required replacement: request-scoped SSE EOF errors promptly, no follow-up GET/no Last-Event-ID/no initialized; server request fail closed without POST response. TestRequestScopedSSEFailsOnEOFAndByteLimit asserts EOF before terminal; TestStreamableHTTPUsesPostOnlyRoutingAndSentinelHeaders asserts single POST and empty cursor/session. Add explicit no retry after EOF; cancellation isolation remains separate.

Original assertion obligations:

```go
require.NoError(t, transport.Start(context.Background()))
require.NoError(t, err)
require.ErrorAs(t, err, &httpErr)
require.Equal(t, http.StatusMethodNotAllowed, httpErr.StatusCode)
require.NoError(t, transport.Close())
```

### TestStreamableHTTP_CancelledPendingStopsActiveResumeGET

Original: `adversarial_contract_test.go:930` in immutable backup. Disposition: **obsolete forbidden behavior replaced negative assertion**.

No GET/resume/Activate/server responses. Required replacement: request-scoped SSE EOF errors promptly, no follow-up GET/no Last-Event-ID/no initialized; server request fail closed without POST response. TestRequestScopedSSEFailsOnEOFAndByteLimit asserts EOF before terminal; TestStreamableHTTPUsesPostOnlyRoutingAndSentinelHeaders asserts single POST and empty cursor/session. Add explicit no retry after EOF; cancellation isolation remains separate.

Original assertion obligations:

```go
require.NoError(t, transport.Start(context.Background()))
require.NoError(t, err)
require.ErrorIs(t, err, context.Canceled)
t.Fatal("resume GET was not cancelled with its pending request")
require.NoError(t, transport.Close())
```

### TestStreamableHTTP_PostResumeCursorDoesNotLeakIntoActivate

Original: `adversarial_contract_test.go:974` in immutable backup. Disposition: **obsolete forbidden behavior replaced negative assertion**.

No GET/resume/Activate/server responses. Required replacement: request-scoped SSE EOF errors promptly, no follow-up GET/no Last-Event-ID/no initialized; server request fail closed without POST response. TestRequestScopedSSEFailsOnEOFAndByteLimit asserts EOF before terminal; TestStreamableHTTPUsesPostOnlyRoutingAndSentinelHeaders asserts single POST and empty cursor/session. Add explicit no retry after EOF; cancellation isolation remains separate.

Original assertion obligations:

```go
require.NoError(t, err)
require.Empty(t, cursor)
t.Fatal("operation-phase GET did not start")
require.NoError(t, client.Close())
```

### TestStreamableHTTP_RejectsForgedSessionIDs

Original: `adversarial_contract_test.go:1019` in immutable backup. Disposition: **obsolete forbidden behavior replaced negative assertion**.

No session establishment/replacement API: TestStreamableHTTPUsesPostOnlyRoutingAndSentinelHeaders asserts session absent. Add response header session token ignored/not forwarded and decorator reserved mcp/last-event rejection. Do not keep validateSessionID only for dead transport.

Original table/inline-range subscenario inputs (each entry requires disposition above):

```go
fixtures := []string{"contains space", "control\x1f", "delete\x7f", string([]byte{0xff})}
```

Original assertion obligations:

```go
require.ErrorAs(t, err, &payloadErr)
```

### TestStreamableHTTP_RejectsInvalidUTF8JSONAndSSE

Original: `adversarial_contract_test.go:1032` in immutable backup. Disposition: **migrate**.

Перенести исходные assertions и все subscenarios без ослабления на current DTO/PreparedRequest→Deliver→Await. Полного exact tracked equivalent для этого test не установлено; это обязательство миграции, не засчитанное runtime coverage.

Original table/inline-range subscenario inputs (each entry requires disposition above):

```go
fixtures := []struct {
		name        string
		contentType string
		body        []byte
	}{
		{
			name:        "json",
			contentType: "application/json",
			body:        []byte{'{', '"', 'j', 's', 'o', 'n', 'r', 'p', 'c', '"', ':', '"', 0xff, '"', '}'},
		},
		{name: "sse", contentType: "text/event-stream", body: []byte{'d', 'a', 't', 'a', ':', ' ', 0xff, '\n', '\n'}},
	}
```

Original assertion obligations:

```go
require.NoError(t, transport.Start(context.Background()))
require.NoError(t, err)
require.ErrorAs(t, err, &invalid)
require.NoError(t, transport.Close())
```

### TestStreamableHTTP_OversizedGETSSEFailsClosedWithoutRetry

Original: `adversarial_contract_test.go:1070` in immutable backup. Disposition: **migrate**.

Move cap+no retry to request-scoped POST SSE: limit terminates originating pending without GET/reconnect. Current TestRequestScopedSSEFailsOnEOFAndByteLimit verifies cap error but not no repeated network attempt nor shared transport unaffected.

Original assertion obligations:

```go
require.NoError(t, transport.Start(context.Background()))
require.Eventually(t, func() bool { return transport.peer.closed.Load() }, time.Second, time.Millisecond)
require.EqualValues(t, 1, gets.Load())
require.ErrorIs(t, err, ErrTransportClosed)
require.NoError(t, transport.Close())
```

### TestStreamableHTTP_EmptySSEDataPrimesResumeCursor

Original: `adversarial_contract_test.go:1102` in immutable backup. Disposition: **obsolete forbidden behavior replaced negative assertion**.

No GET/resume/Activate/server responses. Required replacement: request-scoped SSE EOF errors promptly, no follow-up GET/no Last-Event-ID/no initialized; server request fail closed without POST response. TestRequestScopedSSEFailsOnEOFAndByteLimit asserts EOF before terminal; TestStreamableHTTPUsesPostOnlyRoutingAndSentinelHeaders asserts single POST and empty cursor/session. Add explicit no retry after EOF; cancellation isolation remains separate.

Original assertion obligations:

```go
require.ErrorIs(t, err, errSSEPollingBoundary)
require.Equal(t, "primed", transport.lastEventID)
```

### TestStreamableHTTP_SSEAcceptsStandardLineEndingsAndLeadingBOM

Original: `adversarial_contract_test.go:1120` in immutable backup. Disposition: **security invariant covered by exact assertion**.

TestTask34SSEAcceptsBOMLineEndingsAndFragmentedFrames runs LF/CR/CRLF, leadingBOM fragmented 1/2/5/3/8bytes, consumeRequestSSE noerror+Await exact current complete result. Old cursor/poll-boundary assertions intentionally removed (no resume).

Original table/inline-range subscenario inputs (each entry requires disposition above):

```go
range []string{"\n", "\r\n", "\r"} {
		t.Run(fmt.Sprintf("%q", separator), func(t *testing.T) {
			// Arrange.
			peer := newRPCPeer(context.Background(), nil, func(context.Context, []byte) error { return nil })
			pending, _, err := peer.beginRequest("line-ending", nil)
			require.NoError(t, err)
			transport := &StreamableHTTPTransport{
				maxStreamBytes: httptool.DefaultMaxSSEStreamBytes,
				peer:           peer,
			}
			stream := "\ufeffid: cursor" + separator +
				fmt.Sprintf(`data: {"jsonrpc":"2.0","id":%s,"result":{}}`, pending.ID()) +
				separator + separator

			// Act.
			consumeErr := transport.consumeSSE(context.Background(), strings.NewReader(stream))
			result, awaitErr := pending.Await(context.Background())

			// Assert.
			require.ErrorIs(t, consumeErr, errSSEPollingBoundary)
			require.NoError(t, awaitErr)
			require.JSONEq(t, `{}`, string(result))
			require.Equal(t, "cursor", transport.lastEventID)
		})
	}
```

Original assertion obligations:

```go
require.NoError(t, err)
require.ErrorIs(t, consumeErr, errSSEPollingBoundary)
require.NoError(t, awaitErr)
require.JSONEq(t, `{}`, string(result))
require.Equal(t, "cursor", transport.lastEventID)
```

### TestStreamableHTTP_SSEDiscardsIncompleteEventAtEOF

Original: `adversarial_contract_test.go:1149` in immutable backup. Disposition: **migrate**.

Перенести исходные assertions и все subscenarios без ослабления на current DTO/PreparedRequest→Deliver→Await. Полного exact tracked equivalent для этого test не установлено; это обязательство миграции, не засчитанное runtime coverage.

Original assertion obligations:

```go
require.NoError(t, err)
require.ErrorIs(t, consumeErr, errSSEPollingBoundary)
require.Equal(t, "previous", state.lastEventID)
require.True(t, peer.hasPendingID(pending.ID()))
```

### TestStreamableHTTP_SSELineUsesConfiguredStreamLimit

Original: `adversarial_contract_test.go:1178` in immutable backup. Disposition: **migrate**.

Перенести исходные assertions и все subscenarios без ослабления на current DTO/PreparedRequest→Deliver→Await. Полного exact tracked equivalent для этого test не установлено; это обязательство миграции, не засчитанное runtime coverage.

Original assertion obligations:

```go
require.NoError(t, err)
require.ErrorIs(t, consumeErr, errSSEPollingBoundary)
require.NoError(t, awaitErr)
require.Len(t, raw, payloadSize+2)
```

### TestStreamableHTTP_TerminalFailureCancelsConcurrentLifecycle

Original: `adversarial_contract_test.go:1205` in immutable backup. Disposition: **migrate**.

Old HTTP request failure globally kills GET + all POST is NOT current request isolation. Replace terminal POST failure does not kill another POST, then Close cancels all and waits. TestStreamableHTTPCancellationClosesOnlyOriginatingPOST is existing isolated cancellation evidence, not equivalent ordinary HTTP failure coverage.

Original assertion obligations:

```go
t.Errorf("decode request: %v", err)
require.NoError(t, transport.Start(context.Background()))
require.NoError(t, err)
require.NoError(t, err)
require.ErrorAs(t, terminalErr, &terminalHTTPError)
require.ErrorAs(t, activeErr, &activeHTTPError)
require.Equal(t, terminalHTTPError.StatusCode, activeHTTPError.StatusCode)
t.Fatal("terminal failure did not cancel active GET")
t.Fatal("terminal failure did not cancel active POST")
require.Eventually(t, func() bool { _, requestErr := transport.Request(context.Background(), "after/terminal", struct{}{}) return errors.Is(requestErr, ErrTransportClosed) }, time.Second, time.Millisecond)
require.NoError(t, transport.Close())
```

### TestStreamableHTTP_TerminalSSEEventCancelsHeldOpenPOST

Original: `adversarial_contract_test.go:1279` in immutable backup. Disposition: **migrate**.

Перенести исходные assertions и все subscenarios без ослабления на current DTO/PreparedRequest→Deliver→Await. Полного exact tracked equivalent для этого test не установлено; это обязательство миграции, не засчитанное runtime coverage.

Original assertion obligations:

```go
t.Errorf("decode request: %v", err)
require.NoError(t, transport.Start(context.Background()))
require.NoError(t, err)
require.NoError(t, err)
require.JSONEq(t, `{}`, string(result))
t.Fatal("terminal SSE event did not cancel held-open POST")
require.Eventually(t, func() bool { transport.mu.RLock() defer transport.mu.RUnlock() return len(transport.activePosts) == 0 }, time.Second, time.Millisecond)
require.NoError(t, transport.Close())
```

### TestStreamableHTTP_RequiresExactMediaType

Original: `adversarial_contract_test.go:1331` in immutable backup. Disposition: **migrate**.

Перенести исходные assertions и все subscenarios без ослабления на current DTO/PreparedRequest→Deliver→Await. Полного exact tracked equivalent для этого test не установлено; это обязательство миграции, не засчитанное runtime coverage.

Original table/inline-range subscenario inputs (each entry requires disposition above):

```go
range []string{"application/json-evil", "text/event-stream-bogus"} {
		t.Run(contentType, func(t *testing.T) {
			// Arrange.
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				writer.Header().Set("Content-Type", contentType)
				_, _ = fmt.Fprint(writer, `{}`)
			}))
			defer server.Close()
			transport := NewStreamableHTTPTransport(server.URL, WithStreamableHTTPAllowPrivateIPs(true))
			require.NoError(t, transport.Start(context.Background()))
			pending, err := transport.Request(context.Background(), "media", struct{}{})
			require.NoError(t, err)

			// Act.
			_, err = pending.Await(context.Background())

			// Assert.
			var invalid *InvalidPayloadError
			require.ErrorAs(t, err, &invalid)
			require.NoError(t, transport.Close())
		})
	}
```

Original assertion obligations:

```go
require.NoError(t, transport.Start(context.Background()))
require.NoError(t, err)
require.ErrorAs(t, err, &invalid)
require.NoError(t, transport.Close())
```

### TestStreamableHTTP_RejectsMethodChangingRedirect

Original: `adversarial_contract_test.go:1356` in immutable backup. Disposition: **migrate**.

Перенести исходные assertions и все subscenarios без ослабления на current DTO/PreparedRequest→Deliver→Await. Полного exact tracked equivalent для этого test не установлено; это обязательство миграции, не засчитанное runtime coverage.

Original assertion obligations:

```go
require.NoError(t, transport.Start(context.Background()))
require.NoError(t, err)
require.ErrorContains(t, err, "redirect must preserve HTTP method")
require.Zero(t, targetCalls.Load())
require.NoError(t, transport.Close())
```

### TestStreamableHTTP_Enforces202ResponseShape

Original: `adversarial_contract_test.go:1382` in immutable backup. Disposition: **obsolete forbidden behavior replaced negative assertion**.

HTTP client Notify universally forbidden; old allowed202 is absent. Retain request202 rejected vs status/error coherence; assert Notify rejected before network and no POST for either old 202-with-body/200-notification fixture.

Original table/inline-range subscenario inputs (each entry requires disposition above):

```go
fixtures := []struct {
		name   string
		status int
		body   string
	}{
		{name: "202 with body", status: http.StatusAccepted, body: `{}`},
		{name: "notification with 200", status: http.StatusOK, body: ""},
	}
```

Original assertion obligations:

```go
require.NoError(t, transport.Start(context.Background()))
require.ErrorAs(t, err, &invalid)
require.NoError(t, transport.Close())
```

### TestStreamableHTTP_SafeClientRejectsRedirectToLoopback

Original: `adversarial_contract_test.go:1413` in immutable backup. Disposition: **migrate**.

Перенести исходные assertions и все subscenarios без ослабления на current DTO/PreparedRequest→Deliver→Await. Полного exact tracked equivalent для этого test не установлено; это обязательство миграции, не засчитанное runtime coverage.

Original assertion obligations:

```go
require.NoError(t, err)
require.NoError(t, err)
require.Error(t, err)
```

### TestTransport_CloseBeforeStartIsTerminal

Original: `adversarial_contract_test.go:1428` in immutable backup. Disposition: **migrate**.

Перенести исходные assertions и все subscenarios без ослабления на current DTO/PreparedRequest→Deliver→Await. Полного exact tracked equivalent для этого test не установлено; это обязательство миграции, не засчитанное runtime coverage.

Original assertion obligations:

```go
require.NoError(t, httpTransport.Close())
require.NoError(t, stdioTransport.Close())
require.ErrorIs(t, httpTransport.Start(context.Background()), ErrTransportClosed)
require.ErrorIs(t, stdioTransport.Start(context.Background()), ErrTransportClosed)
require.ErrorIs(t, httpTransport.Notify(context.Background(), "test", nil), ErrTransportClosed)
require.ErrorIs(t, stdioTransport.Notify(context.Background(), "test", nil), ErrTransportClosed)
```

### TestStdio_ProcessExitReturnsTypedCrash

Original: `adversarial_contract_test.go:1444` in immutable backup. Disposition: **security invariant covered by exact assertion**.

TestTask34StdioCrashUnblocksWaiterAndReapsProcess Await under 3s → TransportCrashError and exec.ExitError, processDone closed, Close noerror, writer/reader/stderr done. Retain independent stdout EOF/descendant-held fd cases separately.

Original assertion obligations:

```go
require.NoError(t, transport.Start(context.Background()))
require.NoError(t, err)
require.ErrorAs(t, err, &crashErr)
require.NoError(t, transport.Close())
```

### TestProtocol_StrictRequiredFieldsAndTaggedUnions

Original: `adversarial_contract_test.go:1460` in immutable backup. Disposition: **migrate**.

Current wire DTO validation/lossless invariant переносится симметрично encode/decode; добавить mandatory resultType/cache/self-describing meta. Каждая fixture ниже сохраняет свой смысл, кроме explicitly forbidden old DTO/variant → negative. Partial neighboring DTO matrices не считаются automatic complete coverage.

Original table/inline-range subscenario inputs (each entry requires disposition above):

```go
fixtures := []struct {
		name string
		raw  string
		into any
	}{
		{name: "missing tool content", raw: `{}`, into: &CallToolResult{}},
		{name: "null resource contents", raw: `{"contents":null}`, into: &ResourcesReadResult{}},
		{name: "missing text", raw: `{"content":[{"type":"text"}]}`, into: &CallToolResult{}},
		{
			name: "cross variant field",
			raw:  `{"content":[{"type":"text","text":"x","data":"bad"}]}`,
			into: &CallToolResult{},
		},
		{
			name: "invalid prompt role",
			raw:  `{"messages":[{"role":"system","content":{"type":"text","text":"x"}}]}`,
			into: &PromptsGetResult{},
		},
		{
			name: "resource without uri",
			raw:  `{"contents":[{"text":"x"}]}`,
			into: &ResourcesReadResult{},
		},
		{
			name: "invalid audience",
			raw:  `{"content":[{"type":"text","text":"x","annotations":{"audience":["system"]}}]}`,
			into: &CallToolResult{},
		},
		{name: "null text", raw: `{"content":[{"type":"text","text":null}]}`, into: &CallToolResult{}},
		{
			name: "null image data",
			raw:  `{"content":[{"type":"image","data":null,"mimeType":"image/png"}]}`,
			into: &CallToolResult{},
		},
		{
			name: "null resource text",
			raw:  `{"contents":[{"uri":"file:///x","text":null}]}`,
			into: &ResourcesReadResult{},
		},
		{name: "null progress", raw: `{"progressToken":"x","progress":null}`, into: &ProgressParams{}},
	}
```

Original assertion obligations:

```go
require.Error(t, err)
```

### TestStructuredContent_PreservesLargeInteger

Original: `adversarial_contract_test.go:1515` in immutable backup. Disposition: **migrate**.

Перенести исходные assertions и все subscenarios без ослабления на current DTO/PreparedRequest→Deliver→Await. Полного exact tracked equivalent для этого test не установлено; это обязательство миграции, не засчитанное runtime coverage.

Original assertion obligations:

```go
require.NoError(t, err)
require.JSONEq(t, string(raw), string(canonical))
require.Contains(t, string(canonical), "9007199254740993")
```

### TestClient_NonToolCancellationUsesActiveRequestID

Original: `adversarial_contract_test.go:1528` in immutable backup. Disposition: **migrate**.

Retain bounded detached cancellation context, trace preservation, no late terminal cancel, exact active request ID and exactly-once claim. Current HTTP closes origin body, stdio emits one notification after delivery; stalled DeliveryDone no indefinite wait. Current TestAwaitCancellationNotifiesExactlyOnceAfterDelivery covers count not all deadline/trace/terminal/stall cases.

Original assertion obligations:

```go
require.NoError(t, err)
require.Eventually(t, func() bool { transport.mu.Lock() defer transport.mu.Unlock() return len(transport.requests) == 2 }, time.Second, time.Millisecond)
require.ErrorIs(t, err, context.Canceled)
require.Equal(t, 1, cancellations)
```

### TestClient_ToolsInvalidationAbortsInFlightSnapshot

Original: `adversarial_contract_test.go:1570` in immutable backup. Disposition: **migrate**.

Перенести invalidation/proxy freshness на current acknowledged subscriptionId + Listen/filter contract. Не resurrect tools.listChanged capability / resources.subscribe. Retain no unauthorized generation/event, stale before RPC, snapshot abort и no authority widening.

Original assertion obligations:

```go
require.NoError(t, err)
require.ErrorAs(t, discoveryErr, &staleErr)
```

### TestClient_YieldAbortSendsCancellationExactlyOnce

Original: `adversarial_contract_test.go:1601` in immutable backup. Disposition: **migrate**.

Retain bounded detached cancellation context, trace preservation, no late terminal cancel, exact active request ID and exactly-once claim. Current HTTP closes origin body, stdio emits one notification after delivery; stalled DeliveryDone no indefinite wait. Current TestAwaitCancellationNotifiesExactlyOnceAfterDelivery covers count not all deadline/trace/terminal/stall cases.

Original assertion obligations:

```go
require.NoError(t, err)
require.NoError(t, iterErr)
require.ErrorIs(t, err, toolsy.ErrStreamAborted)
require.Equal(t, 1, cancellations)
```

### TestResourceResult_PreservesSingleContentMIMEAndBinary

Original: `adversarial_contract_test.go:1657` in immutable backup. Disposition: **migrate**.

Перенести исходные assertions и все subscenarios без ослабления на current DTO/PreparedRequest→Deliver→Await. Полного exact tracked equivalent для этого test не установлено; это обязательство миграции, не засчитанное runtime coverage.

Original assertion obligations:

```go
require.NoError(t, err)
require.NoError(t, err)
require.Equal(t, toolsy.MimeTypeJSON, jsonChunk.MimeType)
require.JSONEq(t, text, string(jsonChunk.Data))
require.Equal(t, []byte{0, 1}, binaryChunk.Data)
require.Equal(t, toolsy.DeliveryClassBinary, binaryChunk.Envelope.DeliveryClass)
```

### TestConnect_ExactVersionAndCanonicalRoots

Original: `client_contract_test.go:27` in immutable backup. Disposition: **obsolete forbidden behavior replaced negative assertion**.

Initialize/initialized, roots/list и ping больше не являются клиентским lifecycle. Exact tracked negatives: TestTransportBoundaryRejectsLegacyMethodsAndNotifications → Error + nil pending для каждого legacy method; TestConnectUsesStrictDiscoveryAndSelfDescribingMetadata → единственный server/discover + полная self-describing _meta. Не переносить положительный server callback.

Original assertion obligations:

```go
require.Equal(t, MethodInitialize, request.Method)
require.NoError(t, json.Unmarshal(request.Params, &raw))
require.NotContains(t, raw, "roots")
require.NoError(t, json.Unmarshal(request.Params, &params))
require.Equal(t, ProtocolVersion, params.ProtocolVersion)
require.NotNil(t, params.Capabilities.Roots)
require.False(t, params.Capabilities.Roots.ListChanged)
require.NoError(t, err)
require.Equal(t, ProtocolVersion, transport.version)
require.Nil(t, rpcErr)
require.NoError(t, json.Unmarshal(result, &roots))
require.Len(t, roots.Roots, 1)
require.Contains(t, roots.Roots[0].URI, "file://")
require.Contains(t, roots.Roots[0].URI, "workspace%20with%20space")
require.NoError(t, client.Close())
```

### TestConnect_PublishesOperationStateBeforeInitializedNotificationReturns

Original: `client_contract_test.go:66` in immutable backup. Disposition: **obsolete forbidden behavior replaced negative assertion**.

Initialize/initialized, roots/list и ping больше не являются клиентским lifecycle. Exact tracked negatives: TestTransportBoundaryRejectsLegacyMethodsAndNotifications → Error + nil pending для каждого legacy method; TestConnectUsesStrictDiscoveryAndSelfDescribingMetadata → единственный server/discover + полная self-describing _meta. Не переносить положительный server callback.

Original assertion obligations:

```go
require.NoError(t, err)
require.Nil(t, rootsErr)
require.NoError(t, client.Close())
```

### TestNormalizeRootsRejectsNonCanonicalTypedFileURIs

Original: `client_contract_test.go:96` in immutable backup. Disposition: **obsolete forbidden behavior replaced negative assertion**.

Roots API и нормализатор удалены, не добавлять compatibility shim. TestTransportBoundaryRejectsLegacyMethodsAndNotifications rejects roots/list + roots/list_changed. URI containment safety переносится отдельно в resource subscription boundary: исходные path/escape fixtures нельзя считать покрытыми тем же отрицательным legacy test.

Original table/inline-range subscenario inputs (each entry requires disposition above):

```go
invalid := []string{
		"file:/workspace",
		"file:///workspace?query=1",
		"file:///workspace#fragment",
	}
```

Original assertion obligations:

```go
require.Nil(t, roots)
require.Error(t, err)
```

### TestNormalizeRootsCanonicalizesTypedFilePath

Original: `client_contract_test.go:114` in immutable backup. Disposition: **obsolete forbidden behavior replaced negative assertion**.

Roots API и нормализатор удалены, не добавлять compatibility shim. TestTransportBoundaryRejectsLegacyMethodsAndNotifications rejects roots/list + roots/list_changed. URI containment safety переносится отдельно в resource subscription boundary: исходные path/escape fixtures нельзя считать покрытыми тем же отрицательным legacy test.

Original assertion obligations:

```go
require.NoError(t, err)
require.Equal(t, "file:///workspace/root", roots[0].URI)
```

### TestNormalizeRootsConvertsWindowsDrivePathPortably

Original: `client_contract_test.go:126` in immutable backup. Disposition: **obsolete forbidden behavior replaced negative assertion**.

Roots API и нормализатор удалены, не добавлять compatibility shim. TestTransportBoundaryRejectsLegacyMethodsAndNotifications rejects roots/list + roots/list_changed. URI containment safety переносится отдельно в resource subscription boundary: исходные path/escape fixtures нельзя считать покрытыми тем же отрицательным legacy test.

Original table/inline-range subscenario inputs (each entry requires disposition above):

```go
tests := []struct {
		path string
		uri  string
		name string
	}{
		{path: `C:\workspace\project`, uri: "file:///C:/workspace/project", name: "project"},
		{path: `C:\`, uri: "file:///C:/", name: "C:"},
		{path: `C:\..\..\etc`, uri: "file:///C:/etc", name: "etc"},
	}
```

Original assertion obligations:

```go
require.NoError(t, err)
require.Equal(t, test.uri, roots[0].URI)
require.Equal(t, test.name, roots[0].Name)
```

### TestNormalizeRootsRejectsUNCPath

Original: `client_contract_test.go:150` in immutable backup. Disposition: **obsolete forbidden behavior replaced negative assertion**.

Roots API и нормализатор удалены, не добавлять compatibility shim. TestTransportBoundaryRejectsLegacyMethodsAndNotifications rejects roots/list + roots/list_changed. URI containment safety переносится отдельно в resource subscription boundary: исходные path/escape fixtures нельзя считать покрытыми тем же отрицательным legacy test.

Original table/inline-range subscenario inputs (each entry requires disposition above):

```go
range []string{`\\server\share`, `//server/share`} {
		t.Run(rootPath, func(t *testing.T) {
			// Act.
			roots, err := normalizeRoots([]string{rootPath}, nil)

			// Assert.
			require.Nil(t, roots)
			require.Error(t, err)
		})
	}
```

Original assertion obligations:

```go
require.Nil(t, roots)
require.Error(t, err)
```

### TestNormalizeRootsKeepsDriveWhenCanonicalizingTypedURI

Original: `client_contract_test.go:164` in immutable backup. Disposition: **obsolete forbidden behavior replaced negative assertion**.

Roots API и нормализатор удалены, не добавлять compatibility shim. TestTransportBoundaryRejectsLegacyMethodsAndNotifications rejects roots/list + roots/list_changed. URI containment safety переносится отдельно в resource subscription boundary: исходные path/escape fixtures нельзя считать покрытыми тем же отрицательным legacy test.

Original table/inline-range subscenario inputs (each entry requires disposition above):

```go
tests := []struct {
		uri  string
		want string
	}{
		{uri: "file:///C:/../../etc", want: "file:///C:/etc"},
		{uri: "file:///c:/workspace%5C..%5Csecret", want: "file:///C:/secret"},
		{uri: "file:///c:%5Cworkspace%5C..%5Csecret", want: "file:///C:/secret"},
	}
```

Original assertion obligations:

```go
require.NoError(t, err)
require.Equal(t, test.want, roots[0].URI)
```

### TestNormalizeRootsRejectsDriveRelativeTypedURIs

Original: `client_contract_test.go:186` in immutable backup. Disposition: **obsolete forbidden behavior replaced negative assertion**.

Roots API и нормализатор удалены, не добавлять compatibility shim. TestTransportBoundaryRejectsLegacyMethodsAndNotifications rejects roots/list + roots/list_changed. URI containment safety переносится отдельно в resource subscription boundary: исходные path/escape fixtures нельзя считать покрытыми тем же отрицательным legacy test.

Original table/inline-range subscenario inputs (each entry requires disposition above):

```go
invalid := []string{"file:///C:", "file:///C:relative/path", "file:///c%3Arelative/path"}
```

Original assertion obligations:

```go
require.Nil(t, roots)
require.Error(t, err)
```

### TestNormalizeRootsRejectsTypedUNCAliases

Original: `client_contract_test.go:202` in immutable backup. Disposition: **obsolete forbidden behavior replaced negative assertion**.

Roots API и нормализатор удалены, не добавлять compatibility shim. TestTransportBoundaryRejectsLegacyMethodsAndNotifications rejects roots/list + roots/list_changed. URI containment safety переносится отдельно в resource subscription boundary: исходные path/escape fixtures нельзя считать покрытыми тем же отрицательным legacy test.

Original table/inline-range subscenario inputs (each entry requires disposition above):

```go
aliases := []string{
		"file:////server/share",
		"file:///%5C%5Cserver%5Cshare",
		"file:///%2F%2Fserver/share",
	}
```

Original assertion obligations:

```go
require.Nil(t, roots)
require.Error(t, err)
```

### TestClient_PingWorksBeforeAndAfterInitialization

Original: `client_contract_test.go:221` in immutable backup. Disposition: **obsolete forbidden behavior replaced negative assertion**.

Initialize/initialized, roots/list и ping больше не являются клиентским lifecycle. Exact tracked negatives: TestTransportBoundaryRejectsLegacyMethodsAndNotifications → Error + nil pending для каждого legacy method; TestConnectUsesStrictDiscoveryAndSelfDescribingMetadata → единственный server/discover + полная self-describing _meta. Не переносить положительный server callback.

Original assertion obligations:

```go
require.Nil(t, beforeErr)
require.Nil(t, afterErr)
require.JSONEq(t, `{}`, string(before))
require.JSONEq(t, `{}`, string(after))
```

### TestClient_PingAndRootsRejectMalformedRequestMeta

Original: `client_contract_test.go:238` in immutable backup. Disposition: **obsolete forbidden behavior replaced negative assertion**.

Initialize/initialized, roots/list и ping больше не являются клиентским lifecycle. Exact tracked negatives: TestTransportBoundaryRejectsLegacyMethodsAndNotifications → Error + nil pending для каждого legacy method; TestConnectUsesStrictDiscoveryAndSelfDescribingMetadata → единственный server/discover + полная self-describing _meta. Не переносить положительный server callback.

Original table/inline-range subscenario inputs (each entry requires disposition above):

```go
methods := []string{MethodPing, MethodRootsList}
```

Original assertion obligations:

```go
require.Nil(t, result)
require.NotNil(t, rpcErr)
require.Equal(t, JSONRPCInvalidParams, rpcErr.Code)
```

### TestClient_CancellationDeliveryWaitIsBounded

Original: `client_contract_test.go:263` in immutable backup. Disposition: **migrate**.

Retain bounded detached cancellation context, trace preservation, no late terminal cancel, exact active request ID and exactly-once claim. Current HTTP closes origin body, stdio emits one notification after delivery; stalled DeliveryDone no indefinite wait. Current TestAwaitCancellationNotifiesExactlyOnceAfterDelivery covers count not all deadline/trace/terminal/stall cases.

Original assertion obligations:

```go
require.Less(t, time.Since(started), 2*time.Second)
require.Empty(t, transport.notifications)
```

### TestClient_DoesNotCancelTerminalPendingRequest

Original: `client_contract_test.go:278` in immutable backup. Disposition: **migrate**.

Retain bounded detached cancellation context, trace preservation, no late terminal cancel, exact active request ID and exactly-once claim. Current HTTP closes origin body, stdio emits one notification after delivery; stalled DeliveryDone no indefinite wait. Current TestAwaitCancellationNotifiesExactlyOnceAfterDelivery covers count not all deadline/trace/terminal/stall cases.

Original assertion obligations:

```go
require.NoError(t, err)
require.NoError(t, peer.dispatch(response))
require.Empty(t, transport.notifications)
```

### TestValidatePrompt_AllowsHumanReadableName

Original: `client_contract_test.go:296` in immutable backup. Disposition: **migrate**.

Перенести исходные assertions и все subscenarios без ослабления на current DTO/PreparedRequest→Deliver→Await. Полного exact tracked equivalent для этого test не установлено; это обязательство миграции, не засчитанное runtime coverage.

Original assertion obligations:

```go
require.NoError(t, err)
```

### TestConnect_VersionMismatchFailsBeforeInitialized

Original: `client_contract_test.go:304` in immutable backup. Disposition: **security invariant covered by exact assertion**.

TestConnectRejectsOldOnlyServerWithoutFallback: nil client, ProtocolVersionError, ровно один server/discover, transport.closed=true. Старый initialize notify отсутствует по контракту.

Original assertion obligations:

```go
require.Nil(t, client)
require.ErrorAs(t, err, &versionErr)
require.NotEqual(t, MethodInitialized, notification.Method)
```

### TestClient_RootsSnapshotOwnsMetadata

Original: `client_contract_test.go:327` in immutable backup. Disposition: **obsolete forbidden behavior replaced negative assertion**.

Roots snapshot исключён с roots API; no roots wire authority проверяет legacy boundary. Сохранить общую ownership гарантию на актуальном ClientInfo: TestWithClientInfoDeepClonesExtensionMetadata/TestConnectResnapshotsClientInfoAfterCustomOption; не считать эти tests доказательством поддержки roots.

Original table/inline-range subscenario inputs (each entry requires disposition above):

```go
roots := []Root{{
		URI:  "file:///workspace",
		Meta: Meta{"origin": metaRaw},
	}}
```

Original assertion obligations:

```go
require.NoError(t, err)
require.Nil(t, rpcErr)
require.NoError(t, decodeErr)
require.Len(t, result.Roots, 1)
require.JSONEq(t, `{"owner":"caller"}`, string(result.Roots[0].Meta["origin"]))
require.NotContains(t, result.Roots[0].Meta, "attacker")
require.NoError(t, client.Close())
```

### TestClient_MissingCapabilityRejectedWithoutRPC

Original: `client_contract_test.go:363` in immutable backup. Disposition: **migrate**.

Retain capability failure before outbound RPC; use current typed capabilities + self-describing discover. No implicit automatic capability authority or fallback.

Original assertion obligations:

```go
require.NoError(t, err)
require.ErrorAs(t, gotErr, &capabilityErr)
require.Len(t, transport.requests, requestCount)
```

### TestClient_ToolStructuredResultMapsOutputSchemaAndTypedEnvelope

Original: `client_contract_test.go:385` in immutable backup. Disposition: **migrate**.

Перенести исходные assertions и все subscenarios без ослабления на current DTO/PreparedRequest→Deliver→Await. Полного exact tracked equivalent для этого test не установлено; это обязательство миграции, не засчитанное runtime coverage.

Original assertion obligations:

```go
require.NoError(t, err)
require.NoError(t, iterErr)
require.NotNil(t, proxy)
require.NotEmpty(t, proxy.Manifest().OutputSchema)
require.NoError(t, err)
require.Equal(t, toolsy.MimeTypeJSON, chunk.MimeType)
require.JSONEq(t, `{"ok":true}`, string(chunk.Data))
require.IsType(t, CallToolResult{}, chunk.TypedResult)
require.NotNil(t, chunk.Envelope)
require.Equal(t, toolsy.ToolEnvelopeKindResult, chunk.Envelope.Kind)
```

### TestClient_ToolResultViolatingOutputSchemaFailsClosed

Original: `client_contract_test.go:442` in immutable backup. Disposition: **security invariant covered by exact assertion**.

TestPublicCallToolValidatesAuthoritativeOutputSchema: real ListTools descriptor authority, invalid structured value → InvalidPayload subject structuredContent for public CallTool AND buildToolResultChunk proxy. Preserve exact large numeric constraints separately.

Original assertion obligations:

```go
require.NoError(t, err)
require.ErrorAs(t, err, &payloadErr)
```

### TestClient_OutputSchemaPreservesExactNumericConstraints

Original: `client_contract_test.go:463` in immutable backup. Disposition: **migrate**.

Перенести исходные assertions и все subscenarios без ослабления на current DTO/PreparedRequest→Deliver→Await. Полного exact tracked equivalent для этого test не установлено; это обязательство миграции, не засчитанное runtime coverage.

Original assertion obligations:

```go
require.NoError(t, err)
require.ErrorAs(t, err, &payloadErr)
```

### TestClient_RemoteToolErrorHasDistinctEnvelope

Original: `client_contract_test.go:482` in immutable backup. Disposition: **migrate**.

Retain typed remote error and ToolResult envelope. Old isError mapped ValidationFailed conflict: current RemoteExecution/RemoteToolError separate from local validation; migrate expected code explicitly, preserve structured message/context.

Original assertion obligations:

```go
require.NoError(t, err)
require.True(t, chunk.IsError)
require.Equal(t, toolsy.CodeRemoteExecution, chunk.Envelope.Error.Code)
require.ErrorAs(t, chunk.Envelope.Error, &remoteErr)
```

### TestClient_CancellationUsesActiveRequestIDExactlyOnce

Original: `client_contract_test.go:500` in immutable backup. Disposition: **migrate**.

Retain bounded detached cancellation context, trace preservation, no late terminal cancel, exact active request ID and exactly-once claim. Current HTTP closes origin body, stdio emits one notification after delivery; stalled DeliveryDone no indefinite wait. Current TestAwaitCancellationNotifiesExactlyOnceAfterDelivery covers count not all deadline/trace/terminal/stall cases.

Original assertion obligations:

```go
require.NoError(t, err)
require.NoError(t, iterErr)
require.Eventually(t, func() bool { transport.mu.Lock() defer transport.mu.Unlock() return len(transport.requests) >= 3 }, time.Second, time.Millisecond)
require.ErrorIs(t, <-result, context.Canceled)
require.NoError(t, json.Unmarshal(notification.Params, &params))
require.Len(t, cancellations, 1)
require.JSONEq(t, `3`, string(cancellations[0].RequestID))
```

### TestClient_ProgressIsFractionalAndMonotonic

Original: `client_contract_test.go:557` in immutable backup. Disposition: **migrate**.

Retain fractional progress, monotonic no regression, no after-terminal emission; bind current token+request provenance. Do not replace with token lexeme-only test.

Original assertion obligations:

```go
require.NoError(t, json.Unmarshal(request.Params, &params))
require.NoError(t, err)
require.NoError(t, iterErr)
require.NoError(t, err)
require.Len(t, progress, 1)
require.InDelta(t, 1.5, *progress[0].Current, 0.0001)
require.InDelta(t, 3.5, *progress[0].Total, 0.0001)
require.Equal(t, "first", progress[0].Message)
```

### TestClient_InvalidListChangedMetaDoesNotAdvanceGeneration

Original: `client_features_test.go:17` in immutable backup. Disposition: **migrate**.

Перенести invalidation/proxy freshness на current acknowledged subscriptionId + Listen/filter contract. Не resurrect tools.listChanged capability / resources.subscribe. Retain no unauthorized generation/event, stale before RPC, snapshot abort и no authority widening.

Original assertion obligations:

```go
require.Zero(t, client.toolGeneration.Load())
require.Empty(t, client.invalidations)
```

### TestClient_SubscribeResourceRequiresObjectResultBeforeMutation

Original: `client_features_test.go:36` in immutable backup. Disposition: **migrate**.

resources/subscribe запрещён. Перенести все malformed result fixtures и extension success в subscriptions/listen tagged ACK: ACK validated before registration/publication, subscriptionId required, correct extended CompleteResult allowed. TestListenRegistersStateBeforeRequestDelivery и immediate ACK test частично покрывают порядок, не malformed ACK matrix.

Original table/inline-range subscenario inputs (each entry requires disposition above):

```go
range []string{"null", `[]`, `42`, `"scalar"`} {
		t.Run(fixture, func(t *testing.T) {
			// Arrange.
			transport := newFakeTransport()
			transport.requestHook = func(request capturedRequest) (json.RawMessage, error, bool) {
				if request.Method == MethodInitialize {
					return initializeResult(ServerCapabilities{
						Resources: &ResourcesCapability{Subscribe: true},
					}), nil, true
				}
				return json.RawMessage(fixture), nil, true
			}
			client, err := Connect(context.Background(), transport)
			require.NoError(t, err)

			// Act.
			err = client.SubscribeResource(context.Background(), "file:///resource")

			// Assert.
			var invalid *InvalidPayloadError
			require.ErrorAs(t, err, &invalid)
			client.mu.RLock()
			_, subscribed := client.subscribed["file:///resource"]
			client.mu.RUnlock()
			require.False(t, subscribed)
			require.NoError(t, client.Close())
		})
	}
```

Original assertion obligations:

```go
require.NoError(t, err)
require.ErrorAs(t, err, &invalid)
require.False(t, subscribed)
require.NoError(t, client.Close())
```

### TestClient_SubscribeResourceAcceptsExtendedEmptyResult

Original: `client_features_test.go:67` in immutable backup. Disposition: **migrate**.

resources/subscribe запрещён. Перенести все malformed result fixtures и extension success в subscriptions/listen tagged ACK: ACK validated before registration/publication, subscriptionId required, correct extended CompleteResult allowed. TestListenRegistersStateBeforeRequestDelivery и immediate ACK test частично покрывают порядок, не malformed ACK matrix.

Original assertion obligations:

```go
require.NoError(t, err)
require.NoError(t, err)
require.True(t, subscribed)
require.NoError(t, client.Close())
```

### TestClient_CloseIsConcurrentIdempotentAndClosesEventChannels

Original: `client_features_test.go:93` in immutable backup. Disposition: **migrate**.

Retain 32 callers, underlying transport.Close exactly1, event-channel closure and no panic/data race on current Client/subscription channels.

Original assertion obligations:

```go
require.NoError(t, err)
require.NoError(t, closeErr)
require.EqualValues(t, 1, transport.closeCalls.Load())
require.False(t, invalidationsOpen)
require.False(t, logMessagesOpen)
```

### TestClient_ToolDescriptorFailuresAreFailClosed

Original: `client_features_test.go:127` in immutable backup. Disposition: **migrate**.

Retain each missing/null/invalid schema/name case, including the original invalid MCP proxy tool name `has space`. Current `mcpToolNamePattern` is `^[A-Za-z0-9_.-]{1,128}$`; both discovery authority publication and `toolToProxyAtGeneration` reject spaces. Human-readable names belong to prompts (`validatePrompt`), not MCP proxy tool identifiers. The previous rationale incorrectly conflated these surfaces; the original fixture/assertion ledger remains unchanged. Removed taskSupport surface does not create executable extension authority; explicit unsupported-current requirement fails closed.

Original table/inline-range subscenario inputs (each entry requires disposition above):

```go
cases := []struct {
		name string
		tool MCPTool
		as   any
	}{
		{name: "missing input schema", tool: MCPTool{Name: "valid"}, as: new(*InvalidPayloadError)},
		{
			name: "null input schema",
			tool: MCPTool{Name: "valid", InputSchema: json.RawMessage(`null`)},
			as:   new(*InvalidPayloadError),
		},
		{
			name: "invalid name",
			tool: MCPTool{Name: "has space", InputSchema: json.RawMessage(`{"type":"object"}`)},
			as:   new(*InvalidPayloadError),
		},
		{
			name: "required tasks",
			tool: MCPTool{
				Name:        "valid",
				InputSchema: json.RawMessage(`{"type":"object"}`),
				Execution:   &ToolExecution{TaskSupport: "required"},
			},
			as: new(*UnsupportedFeatureError),
		},
	}
```

Original assertion obligations:

```go
require.Error(t, err)
require.ErrorAs(t, err, &target)
require.ErrorAs(t, err, &target)
```

### TestClient_ToolsListChangedMakesExistingProxyStale

Original: `client_features_test.go:176` in immutable backup. Disposition: **migrate**.

Перенести invalidation/proxy freshness на current acknowledged subscriptionId + Listen/filter contract. Не resurrect tools.listChanged capability / resources.subscribe. Retain no unauthorized generation/event, stale before RPC, snapshot abort и no authority widening.

Original assertion obligations:

```go
require.NoError(t, err)
require.NoError(t, iterErr)
require.ErrorAs(t, err, &staleErr)
require.Equal(t, uint64(1), client.DiscoveryGeneration(InvalidationTools))
require.Equal(t, InvalidationTools, event.Kind)
t.Fatal("missing invalidation event")
```

### TestClient_ContradictingInvalidationIsIgnored

Original: `client_features_test.go:227` in immutable backup. Disposition: **migrate**.

Перенести invalidation/proxy freshness на current acknowledged subscriptionId + Listen/filter contract. Не resurrect tools.listChanged capability / resources.subscribe. Retain no unauthorized generation/event, stale before RPC, snapshot abort и no authority widening.

Original assertion obligations:

```go
require.NoError(t, err)
require.Zero(t, client.DiscoveryGeneration(InvalidationTools))
```

### TestClient_ResourceUpdateRequiresNegotiatedSubscription

Original: `client_features_test.go:245` in immutable backup. Disposition: **migrate**.

Перенести invalidation/proxy freshness на current acknowledged subscriptionId + Listen/filter contract. Не resurrect tools.listChanged capability / resources.subscribe. Retain no unauthorized generation/event, stale before RPC, snapshot abort и no authority widening.

Original assertion obligations:

```go
require.NoError(t, err)
require.NoError(t, client.SubscribeResource(context.Background(), "file:///watched"))
require.Equal(t, InvalidationResource, event.Kind)
require.Equal(t, "file:///watched", event.URI)
t.Fatal("missing resource update")
```

### TestClient_ResourceSubscriptionAcceptsSubResourceUpdate

Original: `client_features_test.go:277` in immutable backup. Disposition: **migrate**.

Перенести invalidation/proxy freshness на current acknowledged subscriptionId + Listen/filter contract. Не resurrect tools.listChanged capability / resources.subscribe. Retain no unauthorized generation/event, stale before RPC, snapshot abort и no authority widening.

Original assertion obligations:

```go
require.NoError(t, err)
require.NoError(t, client.SubscribeResource(context.Background(), "file:///watched"))
require.Equal(t, InvalidationResource, event.Kind)
require.Equal(t, "file:///watched/child", event.URI)
t.Fatal("missing sub-resource update")
```

### TestResourceUpdateMatchesSubscriptionURIBoundaries

Original: `client_features_test.go:309` in immutable backup. Disposition: **migrate**.

Current resourceSubscriptionAllows: TestResourceSubscriptionAllowsOnlyExactOrPathDescendantURI проверяет exact/descendant/prefix-collision/origin/query/dot/encoded-dot. Не покрыты всей этой matrix: hostname case, escaped unreserved character, encoded slash, default HTTPS port, repeated slash, fragment escape equivalence, query exactness. Перенести КАЖДЫЙ исходный URI pair, сохранив актуальный expected boundary.

Original table/inline-range subscenario inputs (each entry requires disposition above):

```go
fixtures := []struct {
		name         string
		subscription string
		updated      string
		want         bool
	}{
		{name: "exact", subscription: "file:///watched", updated: "file:///watched", want: true},
		{name: "child", subscription: "file:///watched", updated: "file:///watched/child", want: true},
		{name: "sibling", subscription: "file:///watched", updated: "file:///unrelated"},
		{name: "prefix collision", subscription: "file:///watched", updated: "file:///watchedness"},
		{
			name:         "host case normalized",
			subscription: "https://EXAMPLE.com/root?v=1",
			updated:      "https://example.com/root?v=1",
			want:         true,
		},
		{
			name:         "dot segment escapes subtree",
			subscription: "file:///watched",
			updated:      "file:///watched/../other",
		},
		{
			name:         "encoded dot segments escape subtree",
			subscription: "file:///watched",
			updated:      "file:///watched/%2e%2E/other",
		},
		{
			name:         "encoded unreserved path",
			subscription: "file:///watched",
			updated:      "file:///%77atched/child",
			want:         true,
		},
		{
			name:         "encoded slash remains segment data",
			subscription: "file:///watched",
			updated:      "file:///watched%2fchild",
		},
		{
			name:         "default HTTPS port",
			subscription: "https://example.com:443/root",
			updated:      "https://example.com/root/child",
			want:         true,
		},
		{
			name:         "repeated slash remains distinct",
			subscription: "https://example.com/root/private",
			updated:      "https://example.com/root//private",
		},
		{
			name:         "reserved fragment escape remains distinct",
			subscription: "https://example.com/root#%2F",
			updated:      "https://example.com/root#/",
		},
		{name: "different origin", subscription: "https://a.example/root", updated: "https://b.example/root/child"},
		{
			name:         "query identity is exact only",
			subscription: "https://a.example/root?v=1",
			updated:      "https://a.example/root/child?v=1",
		},
	}
```

Original assertion obligations:

```go
require.Equal(t, fixture.want, matched)
```

### TestClient_LoggingNotificationAllowsNullData

Original: `client_features_test.go:383` in immutable backup. Disposition: **migrate**.

Retain null log data if current logging schema permits; current request-scoped opt-in and provenance required. Do not restore capability-derived global log acceptance.

Original assertion obligations:

```go
require.NoError(t, err)
require.JSONEq(t, `null`, string(message.Data))
t.Fatal("missing logging notification with null data")
```

### TestClient_LoggingNotificationRequiresCapabilityAndPreservesData

Original: `client_features_test.go:404` in immutable backup. Disposition: **migrate**.

Logging capability теперь inert, не authorization. Перенести preservation data и rejection unsolicited log в request-scoped opt-in/provenance. TestContextualHTTPLoggingDoesNotRequireProgressToken counts one log, no progressToken; TestSSEProvenanceRejectsCrossRequestAndPreAcknowledgementNotifications checks source binding. Old capability gating заменить отрицательным no capability-derived permission.

Original assertion obligations:

```go
require.NoError(t, err)
require.Equal(t, "info", message.Level)
require.JSONEq(t, `{"ok":true}`, string(message.Data))
t.Fatal("missing log message")
```

### TestClient_PromptTaggedContentAndMetadata

Original: `client_features_test.go:429` in immutable backup. Disposition: **migrate**.

Перенести исходные assertions и все subscenarios без ослабления на current DTO/PreparedRequest→Deliver→Await. Полного exact tracked equivalent для этого test не установлено; это обязательство миграции, не засчитанное runtime coverage.

Original assertion obligations:

```go
require.NoError(t, err)
require.NoError(t, err)
require.Equal(t, "resource", result.Messages[0].Content.Type)
require.JSONEq(t, `1`, string(result.Messages[0].Content.Resource.Meta["x"]))
```

### TestNotifyCancelledRequestUsesBoundedContext

Original: `client_test.go:126` in immutable backup. Disposition: **migrate**.

Retain bounded detached cancellation context, trace preservation, no late terminal cancel, exact active request ID and exactly-once claim. Current HTTP closes origin body, stdio emits one notification after delivery; stalled DeliveryDone no indefinite wait. Current TestAwaitCancellationNotifiesExactlyOnceAfterDelivery covers count not all deadline/trace/terminal/stall cases.

Original assertion obligations:

```go
require.NotNil(t, transport.notifyCtx)
require.Equal(t, MethodCancelled, transport.method)
require.Equal(t, "trace-123", transport.notifyCtx.Value(traceKey{}))
require.NoError(t, transport.notifyErr, "cancel notify context must be detached from parent cancellation")
require.True(t, ok, "expected bounded cancel notification context")
require.True(t, deadline.After(before))
require.True(t, deadline.Before(after.Add(6*time.Second)))
```

### TestConnect_EagerHandshakeSuccess

Original: `client_test.go:148` in immutable backup. Disposition: **migrate**.

Connect обязан делать current server/discover, не initialize; retain Start count, metadata/capability/progress registration assertions с new DTO. TestConnectUsesStrictDiscoveryAndSelfDescribingMetadata covers exact discover metadata/count, но не весь legacy harness.

Original assertion obligations:

```go
require.NoError(t, err)
require.NotNil(t, client)
require.Equal(t, 1, base.startCalls)
require.Equal(t, MethodProgress, base.onNotificationMethod)
require.Equal(t, MethodInitialize, base.calledMethod)
require.Equal(t, MethodInitialized, base.notifiedMethod)
require.NotNil(t, client.serverCaps)
require.NoError(t, err)
require.NoError(t, json.Unmarshal(paramsBytes, &params))
require.Equal(t, "2024-11-05", params["protocolVersion"])
require.Equal(t, []any{"/workspace"}, params["roots"])
require.NoError(t, client.Close())
require.Equal(t, 1, base.closeCalls)
```

### TestConnect_StartFailureClosesTransport

Original: `client_test.go:178` in immutable backup. Disposition: **migrate**.

Перенести исходные assertions и все subscenarios без ослабления на current DTO/PreparedRequest→Deliver→Await. Полного exact tracked equivalent для этого test не установлено; это обязательство миграции, не засчитанное runtime coverage.

Original assertion obligations:

```go
require.Error(t, err)
require.Nil(t, client)
require.Equal(t, 1, base.closeCalls)
```

### TestConnect_InitializeCallFailureClosesTransport

Original: `client_test.go:190` in immutable backup. Disposition: **migrate**.

Те же independent failure cases в server/discover: RPC failure / malformed current DiscoverResult → error + transport Close exactly once. Parser диагностирует discover, не initialize.

Original assertion obligations:

```go
require.Error(t, err)
require.Nil(t, client)
require.Equal(t, MethodInitialize, base.calledMethod)
require.Equal(t, 1, base.closeCalls)
```

### TestConnect_InitializeParseFailureClosesTransport

Original: `client_test.go:203` in immutable backup. Disposition: **migrate**.

Те же independent failure cases в server/discover: RPC failure / malformed current DiscoverResult → error + transport Close exactly once. Parser диагностирует discover, не initialize.

Original assertion obligations:

```go
require.Error(t, err)
require.Nil(t, client)
require.Contains(t, err.Error(), "parse initialize result")
require.Equal(t, 1, base.closeCalls)
```

### TestConnect_InitializedNotifyFailureReturnsErrorAndClosesTransport

Original: `client_test.go:216` in immutable backup. Disposition: **obsolete forbidden behavior replaced negative assertion**.

Initialized notification отсутствует. Заменить assertion: Connect emits no notifications и ничего не делает после единственного discover. Не сохранять фиктивную ошибку nonexistent notify phase.

Original assertion obligations:

```go
require.Error(t, err)
require.Nil(t, client)
require.Contains(t, err.Error(), "notifications/initialized")
require.Equal(t, MethodInitialized, base.notifiedMethod)
require.Equal(t, 1, base.closeCalls)
```

### TestHandleToolCallResult_ErrorChunkUsesStructuredWire

Original: `client_test.go:231` in immutable backup. Disposition: **migrate**.

Retain typed remote error and ToolResult envelope. Old isError mapped ValidationFailed conflict: current RemoteExecution/RemoteToolError separate from local validation; migrate expected code explicitly, preserve structured message/context.

Original assertion obligations:

```go
require.NoError(t, err)
require.NoError(t, err)
require.True(t, got.IsError)
require.Equal(t, toolsy.MimeTypeToolErrorJSON, got.MimeType)
require.NoError(t, json.Unmarshal(got.Data, &wire))
require.Equal(t, string(toolsy.CodeValidationFailed), wire.Code)
```

### TestHandleToolCallResult_ReadLimitExceeded_MapsValidation

Original: `client_test.go:259` in immutable backup. Disposition: **migrate**.

Retain exact default/custom cap diagnostic, code ValidationFailed where current mapping requires, caller context cancellation/deadline/timeout precedence and errors.Join cases. Connect uses discover label; tool/prompt/resource/list contexts retained individually. Нужен current transport cap port, не old impl interface.

Original assertion obligations:

```go
require.Error(t, err)
require.True(t, ok)
require.Equal(t, toolsy.CodeValidationFailed, te.Code)
require.Contains(t, te.Reason, fmt.Sprintf("%d byte limit", httptool.DefaultMaxSSEStreamBytes))
```

### TestGetResourceTool_ReadLimitExceeded

Original: `client_test.go:273` in immutable backup. Disposition: **migrate**.

Retain exact default/custom cap diagnostic, code ValidationFailed where current mapping requires, caller context cancellation/deadline/timeout precedence and errors.Join cases. Connect uses discover label; tool/prompt/resource/list contexts retained individually. Нужен current transport cap port, не old impl interface.

Original assertion obligations:

```go
require.NoError(t, err)
require.Error(t, err)
require.True(t, ok)
require.Equal(t, toolsy.CodeValidationFailed, te.Code)
require.Contains(t, te.Reason, fmt.Sprintf("%d byte limit", httptool.DefaultMaxSSEStreamBytes))
```

### TestGetPrompt_ReadLimitExceeded

Original: `client_test.go:291` in immutable backup. Disposition: **migrate**.

Retain exact default/custom cap diagnostic, code ValidationFailed where current mapping requires, caller context cancellation/deadline/timeout precedence and errors.Join cases. Connect uses discover label; tool/prompt/resource/list contexts retained individually. Нужен current transport cap port, не old impl interface.

Original assertion obligations:

```go
require.Error(t, err)
require.True(t, ok)
require.Equal(t, toolsy.CodeValidationFailed, te.Code)
require.Contains(t, te.Reason, fmt.Sprintf("%d byte limit", httptool.DefaultMaxSSEStreamBytes))
```

### TestGetResourceTool_ReadLimit_CustomStreamCap

Original: `client_test.go:301` in immutable backup. Disposition: **migrate**.

Retain exact default/custom cap diagnostic, code ValidationFailed where current mapping requires, caller context cancellation/deadline/timeout precedence and errors.Join cases. Connect uses discover label; tool/prompt/resource/list contexts retained individually. Нужен current transport cap port, не old impl interface.

Original assertion obligations:

```go
require.NoError(t, err)
require.Error(t, err)
require.True(t, ok)
require.Equal(t, toolsy.CodeValidationFailed, te.Code)
require.Contains(t, te.Reason, fmt.Sprintf("%d byte limit", customCap))
require.NotContains(t, te.Reason, fmt.Sprintf("%d byte limit", httptool.DefaultMaxSSEStreamBytes))
```

### TestHandleToolCallResult_ReadLimit_CustomStreamCap

Original: `client_test.go:324` in immutable backup. Disposition: **migrate**.

Retain exact default/custom cap diagnostic, code ValidationFailed where current mapping requires, caller context cancellation/deadline/timeout precedence and errors.Join cases. Connect uses discover label; tool/prompt/resource/list contexts retained individually. Нужен current transport cap port, не old impl interface.

Original assertion obligations:

```go
require.Error(t, err)
require.True(t, ok)
require.Equal(t, toolsy.CodeValidationFailed, te.Code)
require.Contains(t, te.Reason, fmt.Sprintf("%d byte limit", customCap))
```

### TestGetTools_MapsAnnotationsToManifest

Original: `client_test.go:342` in immutable backup. Disposition: **migrate**.

Retain remote readOnly/destructive/idempotent annotation preservation in manifest; hints не локальные ACL/approval authority. Existing policy_test covers separate mapping function, not discovered end-to-end proxy fixture.

Original assertion obligations:

```go
require.NoError(t, err)
require.NoError(t, iterErr)
require.Len(t, tools, 2)
require.NotNil(t, readTool)
require.True(t, readTool.Manifest().ReadOnly)
require.False(t, readTool.Manifest().Dangerous)
require.NotNil(t, deleteTool)
require.True(t, deleteTool.Manifest().Dangerous)
require.True(t, deleteTool.Manifest().Idempotent)
require.False(t, deleteTool.Manifest().ReadOnly)
```

### TestConnect_Initialize_ReadLimitMapsValidation

Original: `client_test.go:392` in immutable backup. Disposition: **migrate**.

Retain exact default/custom cap diagnostic, code ValidationFailed where current mapping requires, caller context cancellation/deadline/timeout precedence and errors.Join cases. Connect uses discover label; tool/prompt/resource/list contexts retained individually. Нужен current transport cap port, не old impl interface.

Original assertion obligations:

```go
require.Error(t, err)
require.True(t, ok)
require.Equal(t, toolsy.CodeValidationFailed, te.Code)
require.Contains(t, te.Reason, "byte limit")
require.Contains(t, te.Reason, "MCP initialize response")
require.Equal(t, MethodInitialize, base.calledMethod)
```

### TestGetTools_ReadLimitExceeded

Original: `client_test.go:405` in immutable backup. Disposition: **migrate**.

Retain exact default/custom cap diagnostic, code ValidationFailed where current mapping requires, caller context cancellation/deadline/timeout precedence and errors.Join cases. Connect uses discover label; tool/prompt/resource/list contexts retained individually. Нужен current transport cap port, не old impl interface.

Original assertion obligations:

```go
require.Error(t, iterErr)
require.True(t, ok)
require.Equal(t, toolsy.CodeValidationFailed, te.Code)
require.Contains(t, te.Reason, "byte limit")
require.Contains(t, te.Reason, "MCP tools list response")
```

### TestGetPrompts_ReadLimitExceeded

Original: `client_test.go:423` in immutable backup. Disposition: **migrate**.

Retain exact default/custom cap diagnostic, code ValidationFailed where current mapping requires, caller context cancellation/deadline/timeout precedence and errors.Join cases. Connect uses discover label; tool/prompt/resource/list contexts retained individually. Нужен current transport cap port, не old impl interface.

Original assertion obligations:

```go
require.Error(t, iterErr)
require.True(t, ok)
require.Equal(t, toolsy.CodeValidationFailed, te.Code)
require.Contains(t, te.Reason, "byte limit")
```

### TestConnect_Initialize_ReadLimit_CustomStreamCap

Original: `client_test.go:440` in immutable backup. Disposition: **migrate**.

Retain exact default/custom cap diagnostic, code ValidationFailed where current mapping requires, caller context cancellation/deadline/timeout precedence and errors.Join cases. Connect uses discover label; tool/prompt/resource/list contexts retained individually. Нужен current transport cap port, не old impl interface.

Original assertion obligations:

```go
require.Error(t, err)
require.True(t, ok)
require.Equal(t, toolsy.CodeValidationFailed, te.Code)
require.Contains(t, te.Reason, fmt.Sprintf("%d byte limit", customCap))
```

### TestGetTools_ReadLimit_CustomStreamCap

Original: `client_test.go:455` in immutable backup. Disposition: **migrate**.

Retain exact default/custom cap diagnostic, code ValidationFailed where current mapping requires, caller context cancellation/deadline/timeout precedence and errors.Join cases. Connect uses discover label; tool/prompt/resource/list contexts retained individually. Нужен current transport cap port, не old impl interface.

Original assertion obligations:

```go
require.Error(t, iterErr)
require.True(t, ok)
require.Equal(t, toolsy.CodeValidationFailed, te.Code)
require.Contains(t, te.Reason, fmt.Sprintf("%d byte limit", customCap))
```

### TestGetPrompts_ReadLimit_CustomStreamCap

Original: `client_test.go:476` in immutable backup. Disposition: **migrate**.

Retain exact default/custom cap diagnostic, code ValidationFailed where current mapping requires, caller context cancellation/deadline/timeout precedence and errors.Join cases. Connect uses discover label; tool/prompt/resource/list contexts retained individually. Нужен current transport cap port, не old impl interface.

Original assertion obligations:

```go
require.Error(t, iterErr)
require.True(t, ok)
require.Equal(t, toolsy.CodeValidationFailed, te.Code)
require.Contains(t, te.Reason, fmt.Sprintf("%d byte limit", customCap))
```

### TestMapCallReadLimit_CancelOverLimit

Original: `client_test.go:497` in immutable backup. Disposition: **migrate**.

Retain exact default/custom cap diagnostic, code ValidationFailed where current mapping requires, caller context cancellation/deadline/timeout precedence and errors.Join cases. Connect uses discover label; tool/prompt/resource/list contexts retained individually. Нужен current transport cap port, не old impl interface.

Original assertion obligations:

```go
require.ErrorIs(t, err, context.Canceled)
```

### TestMapCallReadLimit_DeadlineOverLimit

Original: `client_test.go:506` in immutable backup. Disposition: **migrate**.

Retain exact default/custom cap diagnostic, code ValidationFailed where current mapping requires, caller context cancellation/deadline/timeout precedence and errors.Join cases. Connect uses discover label; tool/prompt/resource/list contexts retained individually. Нужен current transport cap port, не old impl interface.

Original assertion obligations:

```go
require.ErrorIs(t, err, context.DeadlineExceeded)
```

### TestMapCallReadLimit_TimeoutOverLimit

Original: `client_test.go:515` in immutable backup. Disposition: **migrate**.

Retain exact default/custom cap diagnostic, code ValidationFailed where current mapping requires, caller context cancellation/deadline/timeout precedence and errors.Join cases. Connect uses discover label; tool/prompt/resource/list contexts retained individually. Нужен current transport cap port, не old impl interface.

Original assertion obligations:

```go
require.ErrorIs(t, err, toolsy.ErrTimeout)
```

### TestHandleToolCallResult_CancelOverReadLimit

Original: `client_test.go:526` in immutable backup. Disposition: **migrate**.

Retain exact default/custom cap diagnostic, code ValidationFailed where current mapping requires, caller context cancellation/deadline/timeout precedence and errors.Join cases. Connect uses discover label; tool/prompt/resource/list contexts retained individually. Нужен current transport cap port, не old impl interface.

Original assertion obligations:

```go
require.ErrorIs(t, err, context.Canceled)
```

### TestMapCallReadLimitFor_InterruptInChainOverReadLimit

Original: `client_test.go:539` in immutable backup. Disposition: **migrate**.

Retain exact default/custom cap diagnostic, code ValidationFailed where current mapping requires, caller context cancellation/deadline/timeout precedence and errors.Join cases. Connect uses discover label; tool/prompt/resource/list contexts retained individually. Нужен current transport cap port, не old impl interface.

Original assertion obligations:

```go
require.ErrorIs(t, err, context.Canceled)
```

### TestHandleToolCallResult_InterruptInChainOverReadLimit

Original: `client_test.go:550` in immutable backup. Disposition: **migrate**.

Retain exact default/custom cap diagnostic, code ValidationFailed where current mapping requires, caller context cancellation/deadline/timeout precedence and errors.Join cases. Connect uses discover label; tool/prompt/resource/list contexts retained individually. Нужен current transport cap port, не old impl interface.

Original assertion obligations:

```go
require.ErrorIs(t, err, context.Canceled)
```

### TestRPCPeer_RequestAndNotificationWithSameMethodAreDistinct

Original: `peer_test.go:15` in immutable backup. Disposition: **obsolete forbidden behavior replaced negative assertion**.

Current peer.dispatchEnvelope rejects every server-initiated request with InvalidPayload; no handler pool/responder API. Preserve each malformed input and assert rejection + no outbound response/notification callback; notification same method remains independent only for allowed extension. Existing legacy boundary rejects outbound legacy methods but does NOT prove all incoming-request fixtures; required migration negatives remain open.

Original assertion obligations:

```go
require.NoError(t, peer.dispatch([]byte(`{"jsonrpc":"2.0","method":"same","params":{}}`)))
require.NoError( t, peer.dispatch([]byte(`{"jsonrpc":"2.0","id":"x","method":"same","params":{}}`)), )
require.Eventually(t, func() bool { mu.Lock() defer mu.Unlock() return len(sent) == 1 }, time.Second, time.Millisecond)
require.Equal(t, 1, notifications)
require.Equal(t, 1, requests)
require.NoError(t, json.Unmarshal(sent[0], &response))
require.JSONEq(t, `"x"`, string(response.ID))
require.JSONEq(t, `{"ok":true}`, string(response.Result))
```

### TestRPCPeer_UnknownIncomingMethodReturnsMethodNotFound

Original: `peer_test.go:56` in immutable backup. Disposition: **obsolete forbidden behavior replaced negative assertion**.

Current peer.dispatchEnvelope rejects every server-initiated request with InvalidPayload; no handler pool/responder API. Preserve each malformed input and assert rejection + no outbound response/notification callback; notification same method remains independent only for allowed extension. Existing legacy boundary rejects outbound legacy methods but does NOT prove all incoming-request fixtures; required migration negatives remain open.

Original assertion obligations:

```go
require.NoError(t, err)
require.JSONEq(t, `7`, string(response.ID))
require.Equal(t, JSONRPCMethodNotFound, response.Error.Code)
```

### TestRPCPeer_StringAndNumericIDsDoNotCollide

Original: `peer_test.go:78` in immutable backup. Disposition: **migrate**.

Peer safety invariant остаётся: сохранить каждую correlation/cancellation/range/duplicate/error-code/frame assertion ниже. Internal beginRequest тест допустим; production callers use validated PreparedRequest. Перенос имён/helpers допустим, weakening/range-count reduction нет. Generated randomized ID/fragmentation tests дополняют, не заменяют весь deterministic ledger.

Original assertion obligations:

```go
require.NotEqual(t, mustIDKey(t, `1`), mustIDKey(t, `"1"`))
require.Error(t, fractionalErr)
require.Error(t, nullErr)
require.Equal(t, "n:900719925474099312345", mustIDKey(t, `900719925474099312345`))
require.Equal(t, mustIDKey(t, `1`), mustIDKey(t, `1.0`))
require.Equal(t, mustIDKey(t, `1`), mustIDKey(t, `1e0`))
require.Equal(t, mustIDKey(t, `1000`), mustIDKey(t, `1e3`))
require.Equal(t, "n:1e1000000", mustIDKey(t, `1e1000000`))
```

### TestRPCPeer_EmptyMethodUsesNormalUnknownMethodSemantics

Original: `peer_test.go:96` in immutable backup. Disposition: **obsolete forbidden behavior replaced negative assertion**.

Current peer.dispatchEnvelope rejects every server-initiated request with InvalidPayload; no handler pool/responder API. Preserve each malformed input and assert rejection + no outbound response/notification callback; notification same method remains independent only for allowed extension. Existing legacy boundary rejects outbound legacy methods but does NOT prove all incoming-request fixtures; required migration negatives remain open.

Original assertion obligations:

```go
require.NoError(t, json.Unmarshal(body, &response))
require.NoError(t, requestErr)
require.NoError(t, notificationErr)
require.Equal(t, JSONRPCMethodNotFound, response.Error.Code)
t.Fatalf("unknown notification produced response: %+v", extra)
```

### TestRPCPeer_HybridRequestResponseEnvelopeIsRejected

Original: `peer_test.go:122` in immutable backup. Disposition: **migrate**.

Retain each malformed frame→InvalidPayload + no callback; old outbound JSONRPC error response code/id null obsolete because client has no server responder. Correlated malformed response still completes active pending, no hangs.

Original assertion obligations:

```go
require.ErrorAs(t, err, &payloadErr)
require.Equal(t, JSONRPCInvalidRequest, response.Error.Code)
require.JSONEq(t, `null`, string(response.ID))
```

### TestRPCPeer_FractionalIncomingRequestIDIsRejectedWithoutDispatch

Original: `peer_test.go:145` in immutable backup. Disposition: **obsolete forbidden behavior replaced negative assertion**.

Current peer.dispatchEnvelope rejects every server-initiated request with InvalidPayload; no handler pool/responder API. Preserve each malformed input and assert rejection + no outbound response/notification callback; notification same method remains independent only for allowed extension. Existing legacy boundary rejects outbound legacy methods but does NOT prove all incoming-request fixtures; required migration negatives remain open.

Original assertion obligations:

```go
require.NoError(t, json.Unmarshal(body, &response))
require.ErrorAs(t, err, &invalid)
require.Equal(t, JSONRPCInvalidRequest, response.Error.Code)
require.JSONEq(t, `null`, string(response.ID))
require.False(t, handled)
```

### TestRPCPeer_MalformedIncomingRequestsReceiveInvalidRequest

Original: `peer_test.go:172` in immutable backup. Disposition: **obsolete forbidden behavior replaced negative assertion**.

Current peer.dispatchEnvelope rejects every server-initiated request with InvalidPayload; no handler pool/responder API. Preserve each malformed input and assert rejection + no outbound response/notification callback; notification same method remains independent only for allowed extension. Existing legacy boundary rejects outbound legacy methods but does NOT prove all incoming-request fixtures; required migration negatives remain open.

Original table/inline-range subscenario inputs (each entry requires disposition above):

```go
fixtures := []string{
		`{"jsonrpc":"1.0","id":7,"method":"roots/list"}`,
		`{"id":"missing-version","method":"roots/list"}`,
		`{"jsonrpc":"2.0","id":9,"method":42}`,
		`{"jsonrpc":"2.0","id":10,"method":null}`,
	}
```

Original assertion obligations:

```go
require.NoError(t, json.Unmarshal(body, &response))
require.ErrorAs(t, err, &invalid)
require.Equal(t, JSONRPCInvalidRequest, response.Error.Code)
```

### TestRPCPeer_JSONRPCErrorCodePreservesExactIntegers

Original: `peer_test.go:202` in immutable backup. Disposition: **migrate**.

Peer safety invariant остаётся: сохранить каждую correlation/cancellation/range/duplicate/error-code/frame assertion ниже. Internal beginRequest тест допустим; production callers use validated PreparedRequest. Перенос имён/helpers допустим, weakening/range-count reduction нет. Generated randomized ID/fragmentation tests дополняют, не заменяют весь deterministic ledger.

Original table/inline-range subscenario inputs (each entry requires disposition above):

```go
range []string{"1.0", "1e0", "9007199254740993"} {
		t.Run(code, func(t *testing.T) {
			// Arrange.
			peer := newRPCPeer(context.Background(), nil, func(context.Context, []byte) error { return nil })
			pending, _, err := peer.beginRequest("error", nil)
			require.NoError(t, err)
			response := fmt.Appendf(
				nil,
				`{"jsonrpc":"2.0","id":%s,"error":{"code":%s,"message":"failure"}}`,
				pending.ID(), code,
			)

			// Act.
			dispatchErr := peer.dispatch(response)
			_, awaitErr := pending.Await(context.Background())

			// Assert.
			require.NoError(t, dispatchErr)
			var rpcErr *RPCError
			require.ErrorAs(t, awaitErr, &rpcErr)
			require.Equal(t, JSONNumber(code), rpcErr.Code)
		})
	}
```

Original assertion obligations:

```go
require.NoError(t, err)
require.NoError(t, dispatchErr)
require.ErrorAs(t, awaitErr, &rpcErr)
require.Equal(t, JSONNumber(code), rpcErr.Code)
```

### TestRPCPeer_JSONRPCErrorCodeRejectsFraction

Original: `peer_test.go:228` in immutable backup. Disposition: **migrate**.

Peer safety invariant остаётся: сохранить каждую correlation/cancellation/range/duplicate/error-code/frame assertion ниже. Internal beginRequest тест допустим; production callers use validated PreparedRequest. Перенос имён/helpers допустим, weakening/range-count reduction нет. Generated randomized ID/fragmentation tests дополняют, не заменяют весь deterministic ledger.

Original assertion obligations:

```go
require.NoError(t, err)
require.ErrorAs(t, dispatchErr, &invalid)
require.ErrorAs(t, awaitErr, &invalid)
```

### TestRPCPeer_NonObjectAndInvalidJSONReceiveProtocolErrors

Original: `peer_test.go:249` in immutable backup. Disposition: **migrate**.

Retain each malformed frame→InvalidPayload + no callback; old outbound JSONRPC error response code/id null obsolete because client has no server responder. Correlated malformed response still completes active pending, no hangs.

Original table/inline-range subscenario inputs (each entry requires disposition above):

```go
fixtures := []struct {
		name string
		raw  string
		code JSONNumber
	}{
		{name: "array", raw: `[]`, code: JSONRPCInvalidRequest},
		{name: "null", raw: `null`, code: JSONRPCInvalidRequest},
		{name: "syntax", raw: `{`, code: JSONRPCParseError},
	}
```

Original assertion obligations:

```go
require.NoError(t, json.Unmarshal(body, &response))
require.ErrorAs(t, err, &invalid)
require.Equal(t, fixture.code, response.Error.Code)
require.JSONEq(t, `null`, string(response.ID))
```

### TestRPCPeer_MalformedNotificationShapedMessagesNeverReceiveResponse

Original: `peer_test.go:283` in immutable backup. Disposition: **migrate**.

Peer safety invariant остаётся: сохранить каждую correlation/cancellation/range/duplicate/error-code/frame assertion ниже. Internal beginRequest тест допустим; production callers use validated PreparedRequest. Перенос имён/helpers допустим, weakening/range-count reduction нет. Generated randomized ID/fragmentation tests дополняют, не заменяют весь deterministic ledger.

Original table/inline-range subscenario inputs (each entry requires disposition above):

```go
fixtures := []string{
		`{"jsonrpc":"2.0","method":"notifications/progress","params":[]}`,
		`{"jsonrpc":"1.0","method":"notifications/progress","params":{}}`,
		`{"jsonrpc":"2.0","method":42,"params":{}}`,
		`{"jsonrpc":"2.0","method":"notifications/progress","result":{}}`,
	}
```

Original assertion obligations:

```go
require.ErrorAs(t, err, &invalid)
t.Fatalf("malformed notification produced response: %s", response)
```

### TestRPCPeer_CloseCancelsAndWaitsForIncomingRequestHandlers

Original: `peer_test.go:314` in immutable backup. Disposition: **obsolete forbidden behavior replaced negative assertion**.

Current peer.dispatchEnvelope rejects every server-initiated request with InvalidPayload; no handler pool/responder API. Preserve each malformed input and assert rejection + no outbound response/notification callback; notification same method remains independent only for allowed extension. Existing legacy boundary rejects outbound legacy methods but does NOT prove all incoming-request fixtures; required migration negatives remain open.

Original assertion obligations:

```go
require.NoError(t, peer.dispatch([]byte(`{"jsonrpc":"2.0","id":1,"method":"roots/list"}`)))
t.Fatal("peer close returned before incoming handler cleanup")
t.Fatal("peer close did not return after incoming handler completed")
```

### TestRPCPeer_InvalidErrorObjectCompletesCorrelatedRequest

Original: `peer_test.go:352` in immutable backup. Disposition: **migrate**.

Peer safety invariant остаётся: сохранить каждую correlation/cancellation/range/duplicate/error-code/frame assertion ниже. Internal beginRequest тест допустим; production callers use validated PreparedRequest. Перенос имён/helpers допустим, weakening/range-count reduction нет. Generated randomized ID/fragmentation tests дополняют, не заменяют весь deterministic ledger.

Original table/inline-range subscenario inputs (each entry requires disposition above):

```go
fixtures := []string{`{}`, `{"code":null,"message":"x"}`, `{"code":1,"message":null}`}
```

Original assertion obligations:

```go
require.NoError(t, err)
require.ErrorAs(t, dispatchErr, &invalid)
require.ErrorAs(t, awaitErr, &invalid)
```

### TestRPCPeer_RejectsNonObjectParams

Original: `peer_test.go:374` in immutable backup. Disposition: **obsolete forbidden behavior replaced negative assertion**.

Current peer.dispatchEnvelope rejects every server-initiated request with InvalidPayload; no handler pool/responder API. Preserve each malformed input and assert rejection + no outbound response/notification callback; notification same method remains independent only for allowed extension. Existing legacy boundary rejects outbound legacy methods but does NOT prove all incoming-request fixtures; required migration negatives remain open.

Original table/inline-range subscenario inputs (each entry requires disposition above):

```go
fixtures := []string{"null", `[]`, `"scalar"`, `1`}
```

Original assertion obligations:

```go
require.ErrorAs(t, err, &invalid)
require.Contains(t, string(response), `"code":-32600`)
t.Fatal("invalid request response was not sent")
```

### TestRPCPeer_CloseUnblocksEveryPendingRequest

Original: `peer_test.go:404` in immutable backup. Disposition: **migrate**.

Peer safety invariant остаётся: сохранить каждую correlation/cancellation/range/duplicate/error-code/frame assertion ниже. Internal beginRequest тест допустим; production callers use validated PreparedRequest. Перенос имён/helpers допустим, weakening/range-count reduction нет. Generated randomized ID/fragmentation tests дополняют, не заменяют весь deterministic ledger.

Original assertion obligations:

```go
require.NoError(t, err)
require.ErrorIs(t, err, ErrTransportClosed)
require.ErrorIs(t, err, ErrTransportClosed)
```

### TestResourceContents_HasNoNonStandardAnnotationsSurface

Original: `protocol_contract_test.go:19` in immutable backup. Disposition: **migrate**.

Current wire DTO validation/lossless invariant переносится симметрично encode/decode; добавить mandatory resultType/cache/self-describing meta. Каждая fixture ниже сохраняет свой смысл, кроме explicitly forbidden old DTO/variant → negative. Partial neighboring DTO matrices не считаются automatic complete coverage.

Original assertion obligations:

```go
require.False(t, exists)
```

### TestResourceContents_RejectsUnknownAndLegacyAnnotationFields

Original: `protocol_contract_test.go:30` in immutable backup. Disposition: **migrate**.

Current wire DTO validation/lossless invariant переносится симметрично encode/decode; добавить mandatory resultType/cache/self-describing meta. Каждая fixture ниже сохраняет свой смысл, кроме explicitly forbidden old DTO/variant → negative. Partial neighboring DTO matrices не считаются automatic complete coverage.

Original table/inline-range subscenario inputs (each entry requires disposition above):

```go
fixtures := []string{
		`{"uri":"file:///a","text":"x","annotations":{}}`,
		`{"uri":"file:///a","text":"x","future":true}`,
	}
```

Original assertion obligations:

```go
require.Error(t, err)
```

### TestProgressToken_StringAndNumberWireContract

Original: `protocol_contract_test.go:49` in immutable backup. Disposition: **migrate**.

Current wire DTO validation/lossless invariant переносится симметрично encode/decode; добавить mandatory resultType/cache/self-describing meta. Каждая fixture ниже сохраняет свой смысл, кроме explicitly forbidden old DTO/variant → negative. Partial neighboring DTO matrices не считаются automatic complete coverage.

Original table/inline-range subscenario inputs (each entry requires disposition above):

```go
cases := []struct {
		name string
		raw  string
		want string
	}{
		{name: "string", raw: `"token"`, want: "token"},
		{name: "integer", raw: `42`, want: "42"},
		{name: "fractional", raw: `1.5`, want: "1.5"},
		{name: "integral exponent", raw: `1e0`, want: "1e0"},
	}
```

Original assertion obligations:

```go
require.NoError(t, err)
require.NoError(t, marshalErr)
require.Equal(t, tc.want, token.String())
require.JSONEq(t, tc.raw, string(encoded))
```

### TestEmptyResult_RequiresObjectAndPreservesExtensions

Original: `protocol_contract_test.go:79` in immutable backup. Disposition: **migrate**.

Current wire DTO validation/lossless invariant переносится симметрично encode/decode; добавить mandatory resultType/cache/self-describing meta. Каждая fixture ниже сохраняет свой смысл, кроме explicitly forbidden old DTO/variant → negative. Partial neighboring DTO matrices не считаются automatic complete coverage.

Original table/inline-range subscenario inputs (each entry requires disposition above):

```go
range []string{"null", `[]`, `42`, `"scalar"`} {
		var result EmptyResult
		require.Error(t, json.Unmarshal([]byte(invalid), &result))
	}
```

Original assertion obligations:

```go
require.Error(t, json.Unmarshal([]byte(invalid), &result))
require.NoError(t, err)
require.NoError(t, marshalErr)
require.JSONEq(t, fixture, string(roundTrip))
```

### TestMeta_KeyFormatIsValidatedOnBothDirections

Original: `protocol_contract_test.go:99` in immutable backup. Disposition: **migrate**.

Все исходные valid/invalid identifiers проверить marshal/unmarshal; old accepted empty string/prefix-only key now reject. Exact tracked TestMetaIdentifiersRejectEmptyAndPrefixOnlyKeys обеспечивает новые negatives, не всю прежнюю key-format matrix.

Original table/inline-range subscenario inputs (each entry requires disposition above):

```go
valid := []string{"", "trace", "trace.id", "com.example/trace-id", "com.example/"}

invalid := []string{"bad prefix/value", "vendor?/trace", ".vendor/trace", "vendor./trace", "-trace"}
```

Original assertion obligations:

```go
require.NoError(t, json.Unmarshal([]byte(raw), &meta), key)
require.NoError(t, err, key)
require.Error(t, json.Unmarshal([]byte(raw), &meta), key)
require.Error(t, err, key)
```

### TestRequestMeta_ExtensionKeyFormatIsValidated

Original: `protocol_contract_test.go:118` in immutable backup. Disposition: **migrate**.

Current wire DTO validation/lossless invariant переносится симметрично encode/decode; добавить mandatory resultType/cache/self-describing meta. Каждая fixture ниже сохраняет свой смысл, кроме explicitly forbidden old DTO/variant → negative. Partial neighboring DTO matrices не считаются automatic complete coverage.

Original assertion obligations:

```go
require.Error(t, unmarshalErr)
require.Error(t, marshalErr)
```

### TestProgressToken_RejectsNullAndCompositeValues

Original: `protocol_contract_test.go:133` in immutable backup. Disposition: **migrate**.

Current wire DTO validation/lossless invariant переносится симметрично encode/decode; добавить mandatory resultType/cache/self-describing meta. Каждая fixture ниже сохраняет свой смысл, кроме explicitly forbidden old DTO/variant → negative. Partial neighboring DTO matrices не считаются automatic complete coverage.

Original table/inline-range subscenario inputs (each entry requires disposition above):

```go
invalid := []string{`null`, `{}`, `[]`, `true`}
```

Original assertion obligations:

```go
require.Error(t, err, raw)
```

### TestCallToolResult_AllContentVariantsRemainLossless

Original: `protocol_contract_test.go:147` in immutable backup. Disposition: **migrate**.

Current wire DTO validation/lossless invariant переносится симметрично encode/decode; добавить mandatory resultType/cache/self-describing meta. Каждая fixture ниже сохраняет свой смысл, кроме explicitly forbidden old DTO/variant → negative. Partial neighboring DTO matrices не считаются automatic complete coverage.

Original assertion obligations:

```go
require.NoError(t, err)
require.NoError(t, marshalErr)
require.Len(t, result.Content, 5)
require.Equal(t, "aW1n", result.Content[1].Data)
require.Equal(t, "image/png", result.Content[1].MIMEType)
require.Nil(t, result.Content[1].Meta)
require.NotNil(t, result.Content[4].Resource)
require.JSONEq(t, fixture, string(roundTrip))
require.NotContains(t, string(roundTrip), `"base64"`)
require.NotContains(t, string(roundTrip), `"mediaType"`)
```

### TestContentBlock_EmptyRequiredPayloadRoundTrips

Original: `protocol_contract_test.go:179` in immutable backup. Disposition: **migrate**.

Current wire DTO validation/lossless invariant переносится симметрично encode/decode; добавить mandatory resultType/cache/self-describing meta. Каждая fixture ниже сохраняет свой смысл, кроме explicitly forbidden old DTO/variant → negative. Partial neighboring DTO matrices не считаются automatic complete coverage.

Original assertion obligations:

```go
require.NoError(t, err)
require.NoError(t, marshalErr)
require.JSONEq(t, fixture, string(roundTrip))
```

### TestResourceLinkSize_PreservesArbitraryJSONNumbers

Original: `protocol_contract_test.go:194` in immutable backup. Disposition: **migrate**.

Huge integral size lossless retained; fractional 1.5 положительный case запрещён current schema. TestResourceSizesRequireLosslessJSONIntegersSymmetrically/TestTask34PinnedSchemaRequiresIntegerResourceSizes покрывают current integer requirement; сохранить old exponent integral spelling case отдельно, fraction now negative.

Original table/inline-range subscenario inputs (each entry requires disposition above):

```go
fixtures := []string{"9007199254740993", "1.5", "1e3"}
```

Original assertion obligations:

```go
require.NoError(t, err)
require.NoError(t, marshalErr)
require.Equal(t, size, result.Content[0].Size.String())
require.JSONEq(t, fixture, string(roundTrip))
```

### TestStrictDTOsRejectPresentNullAndFractionalInteger

Original: `protocol_contract_test.go:218` in immutable backup. Disposition: **migrate**.

Current wire DTO validation/lossless invariant переносится симметрично encode/decode; добавить mandatory resultType/cache/self-describing meta. Каждая fixture ниже сохраняет свой смысл, кроме explicitly forbidden old DTO/variant → negative. Partial neighboring DTO matrices не считаются automatic complete coverage.

Original table/inline-range subscenario inputs (each entry requires disposition above):

```go
fixtures := []struct {
		name string
		raw  string
		into any
	}{
		{name: "isError null", raw: `{"content":[],"isError":null}`, into: &CallToolResult{}},
		{
			name: "resource size null",
			raw:  `{"content":[{"type":"resource_link","uri":"file:///x","name":"x","size":null}]}`,
			into: &CallToolResult{},
		},
		{
			name: "resource size non-number",
			raw:  `{"content":[{"type":"resource_link","uri":"file:///x","name":"x","size":"1"}]}`,
			into: &CallToolResult{},
		},
		{
			name: "resource link invalid icon theme",
			raw:  `{"content":[{"type":"resource_link","uri":"file:///x","name":"x","icons":[{"src":"https://example/icon.png","theme":"auto"}]}]}`,
			into: &CallToolResult{},
		},
		{
			name: "mime type null",
			raw:  `{"contents":[{"uri":"file:///x","mimeType":null,"text":"x"}]}`,
			into: &ResourcesReadResult{},
		},
		{
			name: "prompt required null",
			raw:  `{"prompts":[{"name":"p","arguments":[{"name":"a","required":null}]}]}`,
			into: &PromptsListResult{},
		},
		{
			name: "capability flag null",
			raw:  `{"protocolVersion":"2025-11-25","capabilities":{"tools":{"listChanged":null}},"serverInfo":{"name":"s","version":"1"}}`,
			into: &InitializeResult{},
		},
		{
			name: "logging logger null",
			raw:  `{"level":"info","logger":null,"data":{}}`,
			into: &LogMessageParams{},
		},
		{
			name: "logging meta null",
			raw:  `{"level":"info","data":{},"_meta":null}`,
			into: &LogMessageParams{},
		},
		{
			name: "resource update meta null",
			raw:  `{"uri":"file:///x","_meta":null}`,
			into: &ResourceUpdatedParams{},
		},
		{name: "base request meta null", raw: `{"_meta":null}`, into: &RequestParams{}},
		{
			name: "initialize request meta null",
			raw:  `{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"c","version":"1"},"_meta":null}`,
			into: &InitializeParams{},
		},
		{name: "cursor request meta null", raw: `{"_meta":null}`, into: &CursorParams{}},
		{name: "tool call meta null", raw: `{"name":"x","_meta":null}`, into: &ToolsCallParams{}},
		{name: "resource read meta null", raw: `{"uri":"file:///x","_meta":null}`, into: &ResourcesReadParams{}},
		{name: "prompt get meta null", raw: `{"name":"x","_meta":null}`, into: &PromptsGetParams{}},
	}
```

Original assertion obligations:

```go
require.Error(t, err)
```

### TestRequestDTOsRoundTripTypedProgressMetaAndExtra

Original: `protocol_contract_test.go:294` in immutable backup. Disposition: **migrate**.

RequestMeta current self-describing обязательные поля не заменяются пустым {}. Retain null fail without silent mutation, reuse reset for optional progress/extra при valid required metadata. Initialize DTO replaced Discover; progress numeric/string preserved.

Original table/inline-range subscenario inputs (each entry requires disposition above):

```go
fixtures := []any{
		InitializeParams{Meta: meta},
		CursorParams{Meta: meta},
		ToolsCallParams{Name: "tool", Meta: meta},
		ResourcesReadParams{URI: "file:///x", Meta: meta},
		PromptsGetParams{Name: "prompt", Meta: meta},
	}
```

Original assertion obligations:

```go
require.NoError(t, err)
require.JSONEq(t, `{"progressToken":42,"trace":"abc"}`, string(mustObjectField(t, encoded, metaField)))
```

### TestRequestMetaRejectsNullAndResetsOnReuse

Original: `protocol_contract_test.go:319` in immutable backup. Disposition: **migrate**.

RequestMeta current self-describing обязательные поля не заменяются пустым {}. Retain null fail without silent mutation, reuse reset for optional progress/extra при valid required metadata. Initialize DTO replaced Discover; progress numeric/string preserved.

Original assertion obligations:

```go
require.Error(t, nullErr)
require.Equal(t, "old", retainedToken)
require.JSONEq(t, `true`, string(retainedExtra))
require.NoError(t, emptyErr)
require.True(t, meta.ProgressToken.IsZero())
require.Nil(t, meta.Extra)
```

### TestRequestMetaRejectsReservedExtraCollision

Original: `protocol_contract_test.go:341` in immutable backup. Disposition: **migrate**.

Current wire DTO validation/lossless invariant переносится симметрично encode/decode; добавить mandatory resultType/cache/self-describing meta. Каждая fixture ниже сохраняет свой смысл, кроме explicitly forbidden old DTO/variant → negative. Partial neighboring DTO matrices не считаются automatic complete coverage.

Original table/inline-range subscenario inputs (each entry requires disposition above):

```go
fixtures := []RequestMeta{
		{Extra: Meta{"progressToken": json.RawMessage(`"forged"`)}},
		{
			ProgressToken: NewStringProgressToken("typed"),
			Extra:         Meta{"progressToken": json.RawMessage(`null`)},
		},
	}
```

Original assertion obligations:

```go
require.Error(t, err)
```

### TestResourceContents_TextAndBlobAreDistinct

Original: `protocol_contract_test.go:360` in immutable backup. Disposition: **migrate**.

Current wire DTO validation/lossless invariant переносится симметрично encode/decode; добавить mandatory resultType/cache/self-describing meta. Каждая fixture ниже сохраняет свой смысл, кроме explicitly forbidden old DTO/variant → negative. Partial neighboring DTO matrices не считаются automatic complete coverage.

Original assertion obligations:

```go
require.NoError(t, err)
require.NotNil(t, result.Contents[0].Text)
require.Nil(t, result.Contents[0].Blob)
require.Nil(t, result.Contents[1].Text)
require.NotNil(t, result.Contents[1].Blob)
```

### TestServerCapabilities_PreservesUnknownAdditiveCapabilities

Original: `protocol_contract_test.go:376` in immutable backup. Disposition: **migrate**.

Current wire DTO validation/lossless invariant переносится симметрично encode/decode; добавить mandatory resultType/cache/self-describing meta. Каждая fixture ниже сохраняет свой смысл, кроме explicitly forbidden old DTO/variant → negative. Partial neighboring DTO matrices не считаются automatic complete coverage.

Original assertion obligations:

```go
require.NoError(t, err)
require.NoError(t, marshalErr)
require.JSONEq(t, `{"enabled":true}`, string(capabilities.Extra["future"]))
require.JSONEq(t, fixture, string(roundTrip))
```

### TestResultDTOsPreserveUnknownTopLevelExtensions

Original: `protocol_contract_test.go:392` in immutable backup. Disposition: **migrate**.

Каждая existing current DTO family retains Extra losslessly. InitializeResult → DiscoverResult; RootsResult отсутствует → no legacy roots claim; add current resultType/ttl/cache as mandatory. TestStrictDTOFamiliesRejectNullAndPreserveAdditiveFields covers several families, не автоматом все table entries.

Original table/inline-range subscenario inputs (each entry requires disposition above):

```go
fixtures := []struct {
		name   string
		raw    string
		target any
	}{
		{
			name: "initialize",
			raw: `{"protocolVersion":"2025-11-25","capabilities":{},` +
				`"serverInfo":{"name":"server","version":"1"},"vendor":{"trace":1}}`,
			target: new(InitializeResult),
		},
		{name: "tools list", raw: `{"tools":[],"vendor":{"trace":1}}`, target: new(ToolsListResult)},
		{name: "tool call", raw: `{"content":[],"vendor":{"trace":1}}`, target: new(CallToolResult)},
		{name: "resource read", raw: `{"contents":[],"vendor":{"trace":1}}`, target: new(ResourcesReadResult)},
		{name: "prompts list", raw: `{"prompts":[],"vendor":{"trace":1}}`, target: new(PromptsListResult)},
		{name: "prompt get", raw: `{"messages":[],"vendor":{"trace":1}}`, target: new(PromptsGetResult)},
		{name: "roots list", raw: `{"roots":[],"vendor":{"trace":1}}`, target: new(RootsListResult)},
	}
```

Original assertion obligations:

```go
require.NoError(t, err)
require.NoError(t, marshalErr)
require.NoError(t, json.Unmarshal(roundTrip, &fields))
require.JSONEq(t, `{"trace":1}`, string(fields["vendor"]))
```

### TestResultDTORejectsExtensionCollisionWithReservedField

Original: `protocol_contract_test.go:427` in immutable backup. Disposition: **migrate**.

Current wire DTO validation/lossless invariant переносится симметрично encode/decode; добавить mandatory resultType/cache/self-describing meta. Каждая fixture ниже сохраняет свой смысл, кроме explicitly forbidden old DTO/variant → negative. Partial neighboring DTO matrices не считаются automatic complete coverage.

Original assertion obligations:

```go
require.Error(t, err)
```

### TestServerCapabilities_KnownCapabilitiesMustBeObjects

Original: `protocol_contract_test.go:441` in immutable backup. Disposition: **migrate**.

Current wire DTO validation/lossless invariant переносится симметрично encode/decode; добавить mandatory resultType/cache/self-describing meta. Каждая fixture ниже сохраняет свой смысл, кроме explicitly forbidden old DTO/variant → negative. Partial neighboring DTO matrices не считаются automatic complete coverage.

Original table/inline-range subscenario inputs (each entry requires disposition above):

```go
invalid := []string{`{"tools":null}`, `{"logging":true}`, `{"resources":[]}`}
```

Original assertion obligations:

```go
require.Error(t, err)
```

### TestPrimitiveMetadata_RoundTripsOnCurrentWireDTOs

Original: `protocol_contract_test.go:455` in immutable backup. Disposition: **migrate**.

Root DTO отсутствует, resource legacy updated replaces subscribed resourceUpdated. Retain Icon/theme, fractional progress, request cancellation ID raw discrimination, resource metadata; attach current mandatory metadata/resultType.

Original assertion obligations:

```go
require.NoError(t, err)
require.NoError(t, marshalErr)
require.Equal(t, "dark", decoded.Icon.Theme)
require.JSONEq(t, fixture, string(roundTrip))
```

### TestStreamableHTTP_JSONSessionVersionAndDeleteContract

Original: `transport_http_test.go:18` in immutable backup. Disposition: **obsolete forbidden behavior replaced negative assertion**.

TestStreamableHTTPUsesPostOnlyRoutingAndSentinelHeaders asserts only POST + current version/name/param headers, empty session/Last-Event-ID. Дополнить Close assertion no DELETE и response session header не создаёт authority. Полезные version/Accept assertions перенести current POST.

Original assertion obligations:

```go
require.NoError(t, transport.Start(context.Background()))
require.NoError(t, err)
require.NoError(t, err)
require.NoError(t, err)
require.NoError(t, closeErr)
require.Equal(t, ProtocolVersion, initializedHeaders.Get(mcpProtocolVersionHeader))
require.Equal(t, "session-1", initializedHeaders.Get(mcpSessionIDHeader))
require.Contains(t, initializedHeaders.Get("Accept"), "application/json")
require.Contains(t, initializedHeaders.Get("Accept"), "text/event-stream")
require.True(t, deleteCalled)
```

### TestStreamableHTTP_POSTAcceptsSSEResponse

Original: `transport_http_test.go:80` in immutable backup. Disposition: **migrate**.

Перенести исходные assertions и все subscenarios без ослабления на current DTO/PreparedRequest→Deliver→Await. Полного exact tracked equivalent для этого test не установлено; это обязательство миграции, не засчитанное runtime coverage.

Original assertion obligations:

```go
require.NoError(t, transport.Start(context.Background()))
require.NoError(t, err)
require.NoError(t, err)
require.JSONEq(t, `{"ok":true}`, string(result))
require.NoError(t, transport.Close())
```

### TestStreamableHTTP_GETResumesAndHandlesServerRequest

Original: `transport_http_test.go:107` in immutable backup. Disposition: **obsolete forbidden behavior replaced negative assertion**.

No GET/resume/Activate/server responses. Required replacement: request-scoped SSE EOF errors promptly, no follow-up GET/no Last-Event-ID/no initialized; server request fail closed without POST response. TestRequestScopedSSEFailsOnEOFAndByteLimit asserts EOF before terminal; TestStreamableHTTPUsesPostOnlyRoutingAndSentinelHeaders asserts single POST and empty cursor/session. Add explicit no retry after EOF; cancellation isolation remains separate.

Original assertion obligations:

```go
require.NoError(t, transport.Start(context.Background()))
require.Equal(t, MethodRootsList, request.Method)
require.NoError(t, err)
require.NoError(t, err)
require.JSONEq(t, `"server-1"`, string(response.ID))
require.JSONEq(t, `{"roots":[]}`, string(response.Result))
t.Fatal("server request response was not posted")
require.Equal(t, "event-a", header)
t.Fatal("GET stream was not resumed")
require.NoError(t, transport.Close())
```

### TestStreamableHTTP_MalformedIncomingRequestReceivesInvalidRequest

Original: `transport_http_test.go:180` in immutable backup. Disposition: **obsolete forbidden behavior replaced negative assertion**.

No GET/resume/Activate/server responses. Required replacement: request-scoped SSE EOF errors promptly, no follow-up GET/no Last-Event-ID/no initialized; server request fail closed without POST response. TestRequestScopedSSEFailsOnEOFAndByteLimit asserts EOF before terminal; TestStreamableHTTPUsesPostOnlyRoutingAndSentinelHeaders asserts single POST and empty cursor/session. Add explicit no retry after EOF; cancellation isolation remains separate.

Original assertion obligations:

```go
require.NoError(t, transport.Start(context.Background()))
require.NoError(t, err)
require.NoError(t, err)
require.Equal(t, JSONRPCInvalidRequest, response.Error.Code)
require.JSONEq(t, `"server-invalid"`, string(response.ID))
t.Fatal("invalid request response was not posted")
require.NoError(t, transport.Close())
```

### TestStreamableHTTP_SessionExpiryAndLegacyEndpointFailClosed

Original: `transport_http_test.go:226` in immutable backup. Disposition: **migrate**.

Перенести исходные assertions и все subscenarios без ослабления на current DTO/PreparedRequest→Deliver→Await. Полного exact tracked equivalent для этого test не установлено; это обязательство миграции, не засчитанное runtime coverage.

Named subscenarios: `"session expiry"`, `"legacy endpoint"`.

Original assertion obligations:

```go
require.NoError(t, transport.Start(context.Background()))
require.NoError(t, err)
require.NoError(t, err)
require.NoError(t, err)
require.ErrorIs(t, err, ErrSessionExpired)
require.NoError(t, transport.Close())
require.NoError(t, transport.Start(context.Background()))
require.NoError(t, err)
require.ErrorAs(t, err, &payloadErr)
require.NoError(t, transport.Close())
```

### TestSSETransport_resolvePostURL_ValidateRemoteURL_BlocksPrivateIP

Original: `transport_sse_test.go:19` in immutable backup. Disposition: **migrate**.

SSRF/private-IP safety перенести HTTP POST endpoint + safe redirect/dial validation. Endpoint discovery GET запрещён; explicit allowPrivateIPs positive сохранить, default deny сохранить. Exact equivalent старого URL/allow pair не засчитан.

Original assertion obligations:

```go
require.NoError(t, err)
require.Error(t, err)
```

### TestSSETransport_resolvePostURL_AllowPrivateIPs

Original: `transport_sse_test.go:31` in immutable backup. Disposition: **migrate**.

SSRF/private-IP safety перенести HTTP POST endpoint + safe redirect/dial validation. Endpoint discovery GET запрещён; explicit allowPrivateIPs positive сохранить, default deny сохранить. Exact equivalent старого URL/allow pair не засчитан.

Original assertion obligations:

```go
require.NoError(t, err)
require.NoError(t, err)
```

### TestSSETransport_ValidateInitialURL_BlocksPrivateIP

Original: `transport_sse_test.go:43` in immutable backup. Disposition: **migrate**.

SSRF/private-IP safety перенести HTTP POST endpoint + safe redirect/dial validation. Endpoint discovery GET запрещён; explicit allowPrivateIPs positive сохранить, default deny сохранить. Exact equivalent старого URL/allow pair не засчитан.

Original assertion obligations:

```go
require.Error(t, err)
```

### TestSSETransport_notify_Non2xxStatus

Original: `transport_sse_test.go:49` in immutable backup. Disposition: **migrate**.

Сохранить typed HTTP status error на current request POST. HTTP Notify запрещён до wire; negative Notify/no network вместо old notification POST. Status/body/correlated RPC error current matrix не отменяет эту отдельную fixture.

Original assertion obligations:

```go
require.Error(t, err)
require.Contains(t, err.Error(), "500")
```

### TestSSETransport_call_Non2xxPOSTStatus

Original: `transport_sse_test.go:71` in immutable backup. Disposition: **migrate**.

Сохранить typed HTTP status error на current request POST. HTTP Notify запрещён до wire; negative Notify/no network вместо old notification POST. Status/body/correlated RPC error current matrix не отменяет эту отдельную fixture.

Original assertion obligations:

```go
require.Error(t, err)
require.Contains(t, err.Error(), "400")
```

### TestSSETransport_Start_Non2xxGET

Original: `transport_sse_test.go:97` in immutable backup. Disposition: **obsolete forbidden behavior replaced negative assertion**.

No legacy GET/endpoint handshake. Replace Start→no HTTP call, all operational requests POST only; pre-canceled Start rejects; event:endpoint rejected/no downgrade. Status204GET never activates lifecycle.

Original assertion obligations:

```go
require.Error(t, err)
require.Contains(t, err.Error(), "500")
```

### TestSSETransport_Start_Accepts204GET

Original: `transport_sse_test.go:110` in immutable backup. Disposition: **obsolete forbidden behavior replaced negative assertion**.

No legacy GET/endpoint handshake. Replace Start→no HTTP call, all operational requests POST only; pre-canceled Start rejects; event:endpoint rejected/no downgrade. Status204GET never activates lifecycle.

Original assertion obligations:

```go
require.NotContains(t, err.Error(), "status 204")
```

### TestSSETransport_Start_CancelBeforeEndpoint

Original: `transport_sse_test.go:127` in immutable backup. Disposition: **obsolete forbidden behavior replaced negative assertion**.

No legacy GET/endpoint handshake. Replace Start→no HTTP call, all operational requests POST only; pre-canceled Start rejects; event:endpoint rejected/no downgrade. Status204GET never activates lifecycle.

Original assertion obligations:

```go
require.Error(t, err)
require.ErrorIs(t, err, context.Canceled)
require.NoError(t, tr.Close())
```

### TestSSETransport_Call_CancelUnblocksPending

Original: `transport_sse_test.go:151` in immutable backup. Disposition: **migrate**.

Current HTTP POST pending cancel unblocks and cancels response body. TestStreamableHTTPCancellationClosesOnlyOriginatingPOST gives current model; no MethodCancelled POST, no old endpoint callback.

Original assertion obligations:

```go
require.NoError(t, tr.Start(context.Background()))
require.Error(t, err)
require.ErrorIs(t, err, context.Canceled)
```

### TestSSETransport_ExceedsMaxStreamBytes

Original: `transport_sse_test.go:201` in immutable backup. Disposition: **migrate**.

Cumulative POST SSE byte limit сохранён: маленькие legal events суммарно превышают cap, не только одна oversized line. TestRequestScopedSSEFailsOnEOFAndByteLimit has oversized line but not cumulative original50events; migrate separately.

Original assertion obligations:

```go
require.NoError(t, tr.Start(context.Background()))
require.Error(t, err)
require.ErrorIs(t, err, textprocessor.ErrReadLimitExceeded)
```

### TestSSETransport_finishSSECallResponse_CancelOverReadLimit

Original: `transport_sse_test.go:233` in immutable backup. Disposition: **migrate**.

Readlimit/stale/canceled/deadline/error-chain precedence перенести actual current pending + client normalization boundary (не revive impl()/finishSSECallResponse/getPostURL). Все исходные context and errors.Join cases ниже retain; запрещённые endpoint behaviors заменить no-wire/canceled negative. Exact corresponding precedence matrix ещё не подтверждена.

Original assertion obligations:

```go
require.ErrorIs(t, err, context.Canceled)
```

### TestSSETransport_finishSSECallResponse_CancelOverStaleStream

Original: `transport_sse_test.go:242` in immutable backup. Disposition: **migrate**.

Readlimit/stale/canceled/deadline/error-chain precedence перенести actual current pending + client normalization boundary (не revive impl()/finishSSECallResponse/getPostURL). Все исходные context and errors.Join cases ниже retain; запрещённые endpoint behaviors заменить no-wire/canceled negative. Exact corresponding precedence matrix ещё не подтверждена.

Original assertion obligations:

```go
require.ErrorIs(t, err, context.Canceled)
```

### TestSSETransport_finishSSECallResponse_LimitWithoutCancel

Original: `transport_sse_test.go:251` in immutable backup. Disposition: **migrate**.

Readlimit/stale/canceled/deadline/error-chain precedence перенести actual current pending + client normalization boundary (не revive impl()/finishSSECallResponse/getPostURL). Все исходные context and errors.Join cases ниже retain; запрещённые endpoint behaviors заменить no-wire/canceled negative. Exact corresponding precedence matrix ещё не подтверждена.

Original assertion obligations:

```go
require.ErrorIs(t, err, textprocessor.ErrReadLimitExceeded)
```

### TestSSETransport_finishSSECallResponse_InterruptInChainOverReadLimit

Original: `transport_sse_test.go:258` in immutable backup. Disposition: **migrate**.

Readlimit/stale/canceled/deadline/error-chain precedence перенести actual current pending + client normalization boundary (не revive impl()/finishSSECallResponse/getPostURL). Все исходные context and errors.Join cases ниже retain; запрещённые endpoint behaviors заменить no-wire/canceled negative. Exact corresponding precedence matrix ещё не подтверждена.

Original assertion obligations:

```go
require.ErrorIs(t, err, context.Canceled)
```

### TestSSETransport_streamLimitErr_InterruptOverReadLimit

Original: `transport_sse_test.go:269` in immutable backup. Disposition: **migrate**.

Readlimit/stale/canceled/deadline/error-chain precedence перенести actual current pending + client normalization boundary (не revive impl()/finishSSECallResponse/getPostURL). Все исходные context and errors.Join cases ниже retain; запрещённые endpoint behaviors заменить no-wire/canceled negative. Exact corresponding precedence matrix ещё не подтверждена.

Original assertion obligations:

```go
require.ErrorIs(t, err, context.Canceled)
```

### TestSSETransport_getPostURL_StreamLimitBlocksEndpoint

Original: `transport_sse_test.go:280` in immutable backup. Disposition: **migrate**.

Readlimit/stale/canceled/deadline/error-chain precedence перенести actual current pending + client normalization boundary (не revive impl()/finishSSECallResponse/getPostURL). Все исходные context and errors.Join cases ниже retain; запрещённые endpoint behaviors заменить no-wire/canceled negative. Exact corresponding precedence matrix ещё не подтверждена.

Original assertion obligations:

```go
require.ErrorIs(t, err, textprocessor.ErrReadLimitExceeded)
```

### TestSSETransport_getPostURL_CancelOverStreamLimit

Original: `transport_sse_test.go:290` in immutable backup. Disposition: **migrate**.

Readlimit/stale/canceled/deadline/error-chain precedence перенести actual current pending + client normalization boundary (не revive impl()/finishSSECallResponse/getPostURL). Все исходные context and errors.Join cases ниже retain; запрещённые endpoint behaviors заменить no-wire/canceled negative. Exact corresponding precedence matrix ещё не подтверждена.

Original assertion obligations:

```go
require.ErrorIs(t, err, context.Canceled)
```

### TestSSETransport_call_CancelOverStreamLimit

Original: `transport_sse_test.go:302` in immutable backup. Disposition: **migrate**.

Readlimit/stale/canceled/deadline/error-chain precedence перенести actual current pending + client normalization boundary (не revive impl()/finishSSECallResponse/getPostURL). Все исходные context and errors.Join cases ниже retain; запрещённые endpoint behaviors заменить no-wire/canceled negative. Exact corresponding precedence matrix ещё не подтверждена.

Original assertion obligations:

```go
require.ErrorIs(t, err, context.Canceled)
```

### TestSSETransport_waitSSECallResponse_DeadlineOverReadLimit

Original: `transport_sse_test.go:314` in immutable backup. Disposition: **migrate**.

Readlimit/stale/canceled/deadline/error-chain precedence перенести actual current pending + client normalization boundary (не revive impl()/finishSSECallResponse/getPostURL). Все исходные context and errors.Join cases ниже retain; запрещённые endpoint behaviors заменить no-wire/canceled negative. Exact corresponding precedence matrix ещё не подтверждена.

Original assertion obligations:

```go
require.ErrorIs(t, err, context.DeadlineExceeded)
require.NotErrorIs(t, err, textprocessor.ErrReadLimitExceeded)
```

### TestStdio_StderrVolumeAndLongLinesNeverCrashTransport

Original: `transport_stdio_contract_test.go:26` in immutable backup. Disposition: **migrate**.

Сохранить actual-process/worker assertion из ledger и helper mode. Replace Request with PreparedRequest/Deliver, Initialize with server/discover mandatory metadata/current resultType. Не подменять inherited-descriptor, EOF vs exit, stderr longline, four malformed response, delivery callback order cases только generic crash test.

Original assertion obligations:

```go
require.False(t, closed)
require.NoError(t, terminalErr)
```

### TestStdio_RequestDrivenServerReceivesInitializeBeforeWriting

Original: `transport_stdio_contract_test.go:44` in immutable backup. Disposition: **migrate**.

Self-exec fixture waits stdin before stdout; replace Initialize with valid server/discover. Start returns without firstline and process remains usable after Start context canceled; response + stderrDone cleanup retained.

Original assertion obligations:

```go
require.NoError(t, transport.Start(startCtx))
require.NoError(t, err)
require.NoError(t, err)
require.NoError(t, json.Unmarshal(result, &initialized))
require.Equal(t, ProtocolVersion, initialized.ProtocolVersion)
require.NoError(t, transport.Close())
t.Fatal("stderr forwarder and parent pipe were not closed")
```

### TestStdio_ProcessExitUnblocksPendingRequest

Original: `transport_stdio_contract_test.go:78` in immutable backup. Disposition: **migrate**.

Сохранить actual-process/worker assertion из ledger и helper mode. Replace Request with PreparedRequest/Deliver, Initialize with server/discover mandatory metadata/current resultType. Не подменять inherited-descriptor, EOF vs exit, stderr longline, four malformed response, delivery callback order cases только generic crash test.

Original assertion obligations:

```go
require.NoError(t, transport.Start(context.Background()))
require.NoError(t, err)
require.Error(t, err)
t.Fatal("exited child was not reaped without external Close")
require.NoError(t, transport.Close())
```

### TestStdio_NonzeroProcessExitWinsStdoutEOFRace

Original: `transport_stdio_contract_test.go:103` in immutable backup. Disposition: **security invariant covered by exact assertion**.

TestTask34StdioCrashUnblocksWaiterAndReapsProcess Await under 3s → TransportCrashError and exec.ExitError, processDone closed, Close noerror, writer/reader/stderr done. Retain independent stdout EOF/descendant-held fd cases separately.

Original assertion obligations:

```go
require.NoError(t, transport.Start(context.Background()))
require.NoError(t, err)
require.ErrorAs(t, err, &crash)
require.ErrorAs(t, err, &exitErr)
require.NoError(t, transport.Close())
```

### TestStdio_ClosedStdoutTerminatesAndReapsLiveProcess

Original: `transport_stdio_contract_test.go:126` in immutable backup. Disposition: **migrate**.

Сохранить actual-process/worker assertion из ledger и helper mode. Replace Request with PreparedRequest/Deliver, Initialize with server/discover mandatory metadata/current resultType. Не подменять inherited-descriptor, EOF vs exit, stderr longline, four malformed response, delivery callback order cases только generic crash test.

Original assertion obligations:

```go
require.NoError(t, transport.Start(context.Background()))
require.NoError(t, err)
require.Error(t, err)
t.Fatal("stdout closure did not terminate and reap the live child")
require.NoError(t, transport.Close())
```

### TestStdio_BlockedWriteHonorsContextCancellation

Original: `transport_stdio_contract_test.go:151` in immutable backup. Disposition: **migrate**.

Request/Await cancellation must unblock caller. Delivery already inside Write must finish frame, not kill transport; prepare valid current metadata, cap payload under current outgoing limit. Preserve eventual Close process/writer reaping, no kill-on-cancel guarantee.

Original table/inline-range subscenario inputs (each entry requires disposition above):

```go
params := map[string]string{"payload": strings.Repeat("x", 4<<20)}
```

Original assertion obligations:

```go
require.NoError(t, transport.Start(context.Background()))
require.NoError(t, err)
require.Eventually(t, func() bool { return transport.activeWrites.Load() > 0 }, 2*time.Second, time.Millisecond)
require.ErrorIs(t, err, context.Canceled)
require.NotEmpty(t, pending.ID())
t.Fatal("blocked pipe cancellation did not terminate and reap the session")
t.Fatal("stdio writer leaked after blocked write cancellation")
require.NoError(t, transport.Close())
```

### TestStdio_CancellingQueuedWriteDoesNotAbortActiveWrite

Original: `transport_stdio_contract_test.go:186` in immutable backup. Disposition: **migrate**.

Mixed: queued cancellation leaves active operation/process alive retained; old active cancel kills process/healthy operation replaced negative no truncation/no transport kill. Exact TestStdioCancellationRetractsQueuedRequestWithoutNotification → queued request+cancel absent; TestStdioBlockingWriteCancellationPreservesDeliveredFrameAndTransport → WasSent, claim1, follow-up alive frame. Do not transplant old healthyErr TransportCrash assertion.

Original assertion obligations:

```go
require.NoError(t, transport.Start(context.Background()))
require.NoError(t, err)
require.Eventually(t, func() bool { return transport.activeWrites.Load() == 1 }, 2*time.Second, time.Millisecond)
require.NoError(t, err)
require.Eventually(t, func() bool { return len(transport.writeQueue) == 1 }, time.Second, time.Millisecond)
require.ErrorIs(t, queuedErr, context.Canceled)
t.Fatal("queued cancellation terminated another request's active write")
require.NoError(t, err)
require.ErrorIs(t, activeErr, context.Canceled)
require.ErrorAs(t, healthyErr, &crash)
require.ErrorIs(t, healthyErr, context.Canceled)
t.Fatal("active write cancellation did not terminate the blocked process")
require.NoError(t, transport.Close())
```

### TestStdio_WriteLoopRechecksCancellationAfterActivation

Original: `transport_stdio_contract_test.go:240` in immutable backup. Disposition: **migrate**.

Сохранить actual-process/worker assertion из ledger и helper mode. Replace Request with PreparedRequest/Deliver, Initialize with server/discover mandatory metadata/current resultType. Не подменять inherited-descriptor, EOF vs exit, stderr longline, four malformed response, delivery callback order cases только generic crash test.

Original assertion obligations:

```go
require.ErrorIs(t, err, context.Canceled)
require.Zero(t, writer.calls)
require.Equal(t, stdioWriteCancelled, write.state.Load())
```

### TestStdio_SuccessfulWriteIsFinishedBeforeDeliveryCallback

Original: `transport_stdio_contract_test.go:269` in immutable backup. Disposition: **migrate**.

Сохранить actual-process/worker assertion из ledger и helper mode. Replace Request with PreparedRequest/Deliver, Initialize with server/discover mandatory metadata/current resultType. Не подменять inherited-descriptor, EOF vs exit, stderr longline, four malformed response, delivery callback order cases только generic crash test.

Original assertion obligations:

```go
require.Equal(t, stdioWriteFinished, write.state.Load())
require.Never(t, func() bool { transport.mu.Lock() defer transport.mu.Unlock() return transport.closed }, 100*time.Millisecond, time.Millisecond)
require.NoError(t, <-write.result)
```

### TestStdio_InvalidUTF8DoesNotCorruptCorrelation

Original: `transport_stdio_contract_test.go:307` in immutable backup. Disposition: **migrate**.

Old helper waits client protocol error reply (unsupported server requests). New helper must not require response; malformed frame/invalid UTF8 handled fail-closed, later valid correlated response still unblocks pending. Assert no dispatch/no response and unchanged ID, not client InvalidRequest wire emission.

Original assertion obligations:

```go
require.NoError(t, transport.Start(context.Background()))
require.NoError(t, err)
require.NoError(t, err)
require.NotEmpty(t, result)
require.NoError(t, transport.Close())
```

### TestStdio_MalformedCorrelatedResponseUnblocksPendingRequest

Original: `transport_stdio_contract_test.go:330` in immutable backup. Disposition: **migrate**.

Сохранить actual-process/worker assertion из ledger и helper mode. Replace Request with PreparedRequest/Deliver, Initialize with server/discover mandatory metadata/current resultType. Не подменять inherited-descriptor, EOF vs exit, stderr longline, four malformed response, delivery callback order cases только generic crash test.

Original table/inline-range subscenario inputs (each entry requires disposition above):

```go
range []string{
		"malformed-response-then-sleep",
		"malformed-typed-response-then-sleep",
		"wrong-version-response-then-sleep",
		"missing-version-response-then-sleep",
	} {
		t.Run(mode, func(t *testing.T) {
			// Arrange.
			transport := NewStdioTransport(
				os.Args[0],
				[]string{"-test.run=TestStdioHelperProcess", "--", mode},
			)
			require.NoError(t, transport.Start(context.Background()))
			pending, err := transport.Request(context.Background(), "test/malformed", struct{}{})
			require.NoError(t, err)
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()

			// Act.
			_, err = pending.Await(ctx)

			// Assert.
			var invalid *InvalidPayloadError
			require.ErrorAs(t, err, &invalid)
			require.NoError(t, transport.Close())
		})
	}
```

Original assertion obligations:

```go
require.NoError(t, transport.Start(context.Background()))
require.NoError(t, err)
require.ErrorAs(t, err, &invalid)
require.NoError(t, transport.Close())
```

### TestStdio_ProcessExitDetectedWhileDescendantHoldsStdout

Original: `transport_stdio_contract_test.go:360` in immutable backup. Disposition: **migrate**.

Сохранить actual-process/worker assertion из ledger и helper mode. Replace Request with PreparedRequest/Deliver, Initialize with server/discover mandatory metadata/current resultType. Не подменять inherited-descriptor, EOF vs exit, stderr longline, four malformed response, delivery callback order cases только generic crash test.

Original assertion obligations:

```go
require.NoError(t, transport.Start(context.Background()))
require.NoError(t, err)
require.ErrorAs(t, err, &crash)
t.Fatal("exited parent was not independently reaped")
require.NoError(t, transport.Close())
```

### TestStdio_ProcessExitDetectedWhileDescendantHoldsStderr

Original: `transport_stdio_contract_test.go:389` in immutable backup. Disposition: **migrate**.

Сохранить actual-process/worker assertion из ledger и helper mode. Replace Request with PreparedRequest/Deliver, Initialize with server/discover mandatory metadata/current resultType. Не подменять inherited-descriptor, EOF vs exit, stderr longline, four malformed response, delivery callback order cases только generic crash test.

Original assertion obligations:

```go
require.NoError(t, transport.Start(context.Background()))
require.NoError(t, err)
require.ErrorAs(t, err, &crash)
t.Fatal("exited parent was not independently reaped")
require.NoError(t, transport.Close())
```

### TestStdio_CloseKillsDescendantProcessTree

Original: `transport_stdio_contract_test.go:418` in immutable backup. Disposition: **security invariant covered by exact assertion**.

TestTask34StdioConcurrentCloseKillsDescendantProcessTree asserts childPID positive+exists, 16 concurrent Close noerror, Eventually !processExists, processDone/writerDone/readerDone/stderrDone closed. This strengthens single-close old process tree case.

Original assertion obligations:

```go
require.NoError(t, transport.Start(context.Background()))
require.NoError(t, err)
require.NoError(t, err)
require.NoError(t, json.Unmarshal(result, &child))
require.True(t, processExists(child.PID))
require.NoError(t, closeErr)
require.Eventually(t, func() bool { return !processExists(child.PID) }, 2*time.Second, 10*time.Millisecond)
```

### TestStdio_MalformedIncomingRequestReceivesInvalidRequest

Original: `transport_stdio_contract_test.go:445` in immutable backup. Disposition: **migrate**.

Old helper waits client protocol error reply (unsupported server requests). New helper must not require response; malformed frame/invalid UTF8 handled fail-closed, later valid correlated response still unblocks pending. Assert no dispatch/no response and unchanged ID, not client InvalidRequest wire emission.

Original assertion obligations:

```go
require.NoError(t, transport.Start(context.Background()))
require.NoError(t, err)
require.NoError(t, err)
require.NotEmpty(t, result)
require.NoError(t, transport.Close())
```

### TestStdioHelperProcess

Original: `transport_stdio_contract_test.go:468` in immutable backup. Disposition: **migrate**.

Helper (не acceptance test): replace InitializeParams/initializeResult with current Discover; update four malformed correlated response modes, request-driven/exit/nonzero/closed-stdout/descendant-stdio/descendant-tree/never-read modes; malformed incoming/UTF8 no protocol response wait. Retain child-reaping fixtures.

### TestStdioTransport_Start_CancelContext

Original: `transport_stdio_test.go:22` in immutable backup. Disposition: **migrate**.

Old waits first stdout after Start and cancels during wait; new Start is request-driven and immediate. Preserve explicit pre-canceled Start rejected/no process started, plus cancellation of subsequent Await separately.

Original assertion obligations:

```go
require.Error(t, err)
require.ErrorIs(t, err, context.Canceled)
require.NoError(t, transport.Close())
```

### TestStdioTransport_StderrLongLineDoesNotBreakStart

Original: `transport_stdio_test.go:36` in immutable backup. Disposition: **migrate**.

Long stderr line должна переслаться/не crash; replace unsolicited first stdout response with current request-driven fixture; Start immediate, follow-up valid request succeeds.

Original assertion obligations:

```go
require.NoError(t, err)
require.NoError(t, transport.Close())
```

### TestStdioTransport_StdoutExceedsMaxStreamBytes

Original: `transport_stdio_test.go:51` in immutable backup. Disposition: **migrate**.

Stdio current PrepareRequest→Deliver→Await; retain caller cancel/deadline/joined/readlimit precedence or stdout cumulative cap from ledger. Remove impl()/finishStdioCallResponse private legacy helpers, not their safety semantics. Test-specific exact migration required.

Original assertion obligations:

```go
require.NoError(t, transport.Start(ctx))
require.Error(t, err)
require.ErrorIs(t, err, textprocessor.ErrReadLimitExceeded)
```

### TestStdioTransport_Call_CancelUnblocksPending

Original: `transport_stdio_test.go:75` in immutable backup. Disposition: **migrate**.

Stdio current PrepareRequest→Deliver→Await; retain caller cancel/deadline/joined/readlimit precedence or stdout cumulative cap from ledger. Remove impl()/finishStdioCallResponse private legacy helpers, not their safety semantics. Test-specific exact migration required.

Original assertion obligations:

```go
require.NoError(t, transport.Start(context.Background()))
require.Error(t, err)
require.ErrorIs(t, err, context.Canceled)
```

### TestStdioTransport_CallAfterStartContextCanceled

Original: `transport_stdio_test.go:92` in immutable backup. Disposition: **migrate**.

Start ctx canceled must not terminate detached process lifetime; canceled caller PrepareRequest/Await rejects. Add successful valid request after Start cancellation; old pre-canceled Call alone insufficient.

Original assertion obligations:

```go
require.NoError(t, transport.Start(ctx))
require.Error(t, err)
require.ErrorIs(t, err, context.Canceled)
```

### TestStdioTransport_finishStdioCallResponse_CancelOverReadLimit

Original: `transport_stdio_test.go:109` in immutable backup. Disposition: **migrate**.

Stdio current PrepareRequest→Deliver→Await; retain caller cancel/deadline/joined/readlimit precedence or stdout cumulative cap from ledger. Remove impl()/finishStdioCallResponse private legacy helpers, not their safety semantics. Test-specific exact migration required.

Original assertion obligations:

```go
require.ErrorIs(t, err, context.Canceled)
```

### TestStdioTransport_call_CancelOverStreamLimit

Original: `transport_stdio_test.go:118` in immutable backup. Disposition: **migrate**.

Stdio current PrepareRequest→Deliver→Await; retain caller cancel/deadline/joined/readlimit precedence or stdout cumulative cap from ledger. Remove impl()/finishStdioCallResponse private legacy helpers, not their safety semantics. Test-specific exact migration required.

Original assertion obligations:

```go
require.NoError(t, transport.Start(context.Background()))
require.Error(t, err)
require.ErrorIs(t, err, context.Canceled)
```

### TestStdioTransport_finishStdioCallResponse_CancelOverStaleStream

Original: `transport_stdio_test.go:136` in immutable backup. Disposition: **migrate**.

Stdio current PrepareRequest→Deliver→Await; retain caller cancel/deadline/joined/readlimit precedence or stdout cumulative cap from ledger. Remove impl()/finishStdioCallResponse private legacy helpers, not their safety semantics. Test-specific exact migration required.

Original assertion obligations:

```go
require.ErrorIs(t, err, context.Canceled)
```

### TestStdioTransport_finishStdioCallResponse_LimitWithoutCancel

Original: `transport_stdio_test.go:145` in immutable backup. Disposition: **migrate**.

Stdio current PrepareRequest→Deliver→Await; retain caller cancel/deadline/joined/readlimit precedence or stdout cumulative cap from ledger. Remove impl()/finishStdioCallResponse private legacy helpers, not their safety semantics. Test-specific exact migration required.

Original assertion obligations:

```go
require.ErrorIs(t, err, textprocessor.ErrReadLimitExceeded)
```

### TestStdioTransport_finishStdioCallResponse_InterruptInChainOverReadLimit

Original: `transport_stdio_test.go:155` in immutable backup. Disposition: **migrate**.

Stdio current PrepareRequest→Deliver→Await; retain caller cancel/deadline/joined/readlimit precedence or stdout cumulative cap from ledger. Remove impl()/finishStdioCallResponse private legacy helpers, not their safety semantics. Test-specific exact migration required.

Original assertion obligations:

```go
require.ErrorIs(t, err, context.Canceled)
```

### TestStdioTransport_streamLimitErr_InterruptOverReadLimit

Original: `transport_stdio_test.go:166` in immutable backup. Disposition: **migrate**.

Stdio current PrepareRequest→Deliver→Await; retain caller cancel/deadline/joined/readlimit precedence or stdout cumulative cap from ledger. Remove impl()/finishStdioCallResponse private legacy helpers, not their safety semantics. Test-specific exact migration required.

Original assertion obligations:

```go
require.ErrorIs(t, err, context.Canceled)
```

## Open migration gates (no blanket deletion approval)

1. Rewrite exact cases above with current ports. Migrated file/test manifest must name each original test and its replacements; compilation PASS alone cannot close the ledger.
2. Preserve cumulative SSE/stdout cap, default/custom diagnostics and canceled/deadline/timeout/joined precedence. Generic one-long-line test is insufficient.
3. Preserve decorator target/Host/body fields (six old mutations), retained pointer detachment, redirect method/SSRF, exact media types, correlated terminal-response/error matrices; HTTP Notify is forbidden, not tested as accepted202.
4. Preserve malformed current ACK before state publication, subscription authority, full URI matrix, stale in-flight discovery and existing proxy generation, request-scoped logging/null data and fractional monotonic progress.
5. Preserve request-driven Start, long stderr/volume, independent process reap when descendants hold stdout/stderr, closed stdout killing live process, four malformed correlated response cases, completion-before-delivery callback, cancel queued vs active writer isolation. No accidental restoration of kill-on-cancel.
6. Add negatives proving no GET/DELETE/initialize/initialized/session/resume/server response, no silent downgrade, no forwarding response session/cursor and no caller/decorator reserved-header injection. Existing tests provide partial exact evidence only.
7. After migration rerun full make test and make lint without excluding MCP, then independent completeness + correctness audits on the same final implementation snapshot. This report is inventory, not final green gate.

## Independent scoped checkpoint — 2026-10-05 (49 originals + helper)

Read current peer_test.go, transport_stdio_test.go, client_contract_test.go, test_support_test.go and five helper-proof tests completely; compared every original test/table/assertion with immutable backup and root mapping in task35-mcp-migration.md. Also inspected current tool-name proxy regex and extra held-Await/missing-Completion progress proofs. Original inventory above remains intact except corrected tool-name rationale; historical generic `migrate` entries are BEFORE-rewrite obligations, not latest implementation statuses.

**Initial checkpoint: 47/49 counterparts preserve relevant assertions/inputs or explicitly replace forbidden behavior; 2/49 have missing inbound negative scenarios.** No removed-root/responder/firstline feature is counted as required runtime support. This is only checkpoint scope, not a 121-row goal percentage or whole-package acceptance. Seven other original files remain unreviewed after migration, and full-package compilation remains reported failing; no overall green assertion.

### Per-original comparison


#### peer_test.go

| Original suffix / name | Current counterpart | Verdict | Exact source observation |
|---|---|---|---|
| RequestAndNotificationWithSameMethodAreDistinct | RequestAndNotificationWithSameMethodAreDistinct | preserved | same method notif dispatch1; incoming id x rejected; send0 replaces old handler+reply |
| UnknownIncomingMethodReturnsMethodNotFound | UnknownIncomingMethodFailsClosed | preserved | id7 unknown preserved; InvalidPayload/send0 replaces MethodNotFound responder |
| StringAndNumericIDsDoNotCollide | StringAndNumericIDsDoNotCollide | preserved | 1 vs quoted1; null/1.5 errors; huge900719925474099312345;1.0/1e0/1e3; huge exponent1e1000000 all exact |
| EmptyMethodUsesNormalUnknownMethodSemantics | EmptyMethodFailsClosed | preserved | requestid7+notification empty name rejected,send0; no obsolete empty method acceptance |
| HybridRequestResponseEnvelopeIsRejected | HybridRequestResponseEnvelopeIsRejected | preserved | method roots/list+result+fractional id1.5 original frame preserved; InvalidPayload/send0 |
| FractionalIncomingRequestIDIsRejectedWithoutDispatch | FractionalIncomingRequestIDIsRejectedWithoutDispatch | preserved | original id1.5 roots/list; callback0+send0 |
| MalformedIncomingRequestsReceiveInvalidRequest | MalformedIncomingRequestsFailClosed | preserved | all4 original frames wrong/missing version, method42/null; InvalidPayload/send0 each |
| JSONRPCErrorCodePreservesExactIntegers | JSONRPCErrorCodePreservesExactIntegers | preserved | code1.0/1e0/9007199254740993 → exact JSONNumber in Await RPCError |
| JSONRPCErrorCodeRejectsFraction | JSONRPCErrorCodeRejectsFraction | preserved | 1.5 rejected in dispatch and correlated Await |
| NonObjectAndInvalidJSONReceiveProtocolErrors | NonObjectAndInvalidJSONFailClosed | preserved | []/null/{ retained; InvalidPayload/send0 instead of unsupported client responder |
| MalformedNotificationShapedMessagesNeverReceiveResponse | MalformedNotificationShapedMessagesNeverReceiveResponse | preserved | all4 original shapes retained; InvalidPayload/send0 |
| CloseCancelsAndWaitsForIncomingRequestHandlers | CloseAfterForbiddenIncomingRequest | preserved | no incoming worker lifecycle exists; roots id1 rejection/send0 then closed peer denies beginRequest; removed handler wait obsolete, not weakened active client cleanup |
| InvalidErrorObjectCompletesCorrelatedRequest | InvalidErrorObjectCompletesCorrelatedRequest | preserved | all3 invaliderror objects retained; both dispatch/Await typed InvalidPayload |
| RejectsNonObjectParams | RejectsNonObjectParams | preserved | null/[]/scalar/1 all retained; extra notification nohandler/no send strengthened |
| CloseUnblocksEveryPendingRequest | CloseUnblocksEveryPendingRequest | preserved | 10 actual pending Await ErrTransportClosed; post-close begin rejected |

#### transport_stdio_test.go

| Original suffix / name | Current counterpart | Verdict | Exact source observation |
|---|---|---|---|
| Start_CancelContext | Start_CancelContext | preserved | pre-canceled Start nilcmd/no process replaces obsolete firstline wait, Close noerror |
| StderrLongLineDoesNotBreakStart | StderrLongLineDoesNotBreakStart | preserved | same70000byte stderr; waits request before reply; successful correlated{} after stderr stronger than Start alone |
| StdoutExceedsMaxStreamBytes | StdoutExceedsMaxStreamBytes | preserved | same cap2048/10×300padding legal notifications; Await raw ErrReadLimitExceeded, not unexpectedEOF |
| Call_CancelUnblocksPending | PreparedCancellationUnblocksPending | preserved | actual request deliverysettled WasSenttrue; canceled Await context.Canceled |
| CallAfterStartContextCanceled | PrepareAfterStartContextCanceled | preserved | detached Start lifetime proven by successful reply after cancelStart; subsequent pre-cancel Prepare nil+Canceled |
| finishStdioCallResponse_CancelOverReadLimit | CallerCancellationOverReadLimit | preserved | canceled caller context outranks raw readlimit at current client boundary |
| call_CancelOverStreamLimit | PreCancelledPrepareOverTerminalReadLimit | preserved | actual prior terminalreadlimit closed transport then pre-cancel Prepare nil+Canceled |
| finishStdioCallResponse_CancelOverStaleStream | CallerCancellationOverStaleStream | preserved | canceled caller outranks same stale stream error at client normalization |
| finishStdioCallResponse_LimitWithoutCancel | ReadLimitWithoutCancellation | preserved | current client emits typed CodeValidationFailed exact default cap diagnostic; transport raw sentinel independently asserted in cumulative test, not erased |
| finishStdioCallResponse_InterruptInChainOverReadLimit | InterruptInChainOverReadLimit | preserved | same fmt-wrapped errors.Join(Canceled,readlimit); returned samecomposite +Canceled |
| streamLimitErr_InterruptOverReadLimit | TerminalInterruptOverReadLimit | preserved | closeWithCause originalcomposite unblocks prepared Await with BOTH Canceled andreadlimit causes |

#### client_contract_test.go

| Original suffix / name | Current counterpart | Verdict | Exact source observation |
|---|---|---|---|
| TestConnect_ExactVersionAndCanonicalRoots | TestConnect_ExactVersionWithoutRootsAuthority | preserved | discover exact current meta version; Roots capnil/no workspace,request1/no notifications; original relativeworkspace path outbound+inbound roots rejected,no writes |
| TestConnect_PublishesOperationStateBeforeInitializedNotificationReturns | TestConnect_DiscoveryPublishesReadyWithoutLegacyNotification | preserved | roots incoming during discovery rejected; readytrue,no initialized notify/peer writes; obsolete callback operation state absent |
| TestNormalizeRootsRejectsNonCanonicalTypedFileURIs | TestLegacyRootsRejectNonCanonicalTypedFileURIs | preserved | all3 file:/query/fragment inputs retained as both-directions no-authority negatives |
| TestNormalizeRootsCanonicalizesTypedFilePath | TestLegacyRootsDoNotCanonicalizeTypedFilePath | preserved | same dir/../root path; rejects rather than exposing removed canonicalizer |
| TestNormalizeRootsConvertsWindowsDrivePathPortably | TestLegacyRootsDoNotConvertWindowsDrivePaths | preserved | all3 Cdrive/project/root/upward inputs retained; expected canonicalURI/name outputs intentionally forbidden |
| TestNormalizeRootsRejectsUNCPath | TestLegacyRootsRejectUNCPaths | preserved | both UNC spellings retained |
| TestNormalizeRootsKeepsDriveWhenCanonicalizingTypedURI | TestLegacyRootsDoNotCanonicalizeTypedDriveURIs | preserved | all3 drive/encodedbackslash inputs retained |
| TestNormalizeRootsRejectsDriveRelativeTypedURIs | TestLegacyRootsRejectDriveRelativeTypedURIs | preserved | all3 drive-relative URI inputs retained |
| TestNormalizeRootsRejectsTypedUNCAliases | TestLegacyRootsRejectTypedUNCAliases | preserved | all3 UNC alias URIs retained |
| TestClient_PingWorksBeforeAndAfterInitialization | TestClient_LegacyPingRejectedBeforeAndAfterDiscovery | preserved after CP-MCP-001 fix | inbound id1 ping params{} dispatched before and after Connect; both typed InvalidPayload, callback0/peerWrites0; outbound negatives retained; discovery only wire request |
| TestClient_PingAndRootsRejectMalformedRequestMeta | TestClient_LegacyPingAndRootsRejectMalformedRequestMeta | preserved after CP-MCP-002 fix | both methods retain inbound id1 params _meta:null plus outbound Prepare; typed InvalidPayload each, callback0/peerWrites0/nil pending/empty preparations+requests |
| TestClient_CancellationDeliveryWaitIsBounded | TestClient_CancellationDeliveryWaitIsBounded | preserved | ID99 stalled DeliveryDone<2seconds/no notifications same |
| TestClient_DoesNotCancelTerminalPendingRequest | TestClient_DoesNotCancelTerminalPendingRequest | preserved | sent/completed pending,no late cancellation same; nilparams changed object{} pervalidRPC |
| TestValidatePrompt_AllowsHumanReadableName | TestValidatePrompt_AllowsHumanReadableName | preserved | daily standup remains valid prompt name; not MCP proxytool name |
| TestConnect_VersionMismatchFailsBeforeInitialized | TestConnect_VersionMismatchFailsWithoutFallback | preserved | same incompatibleversion represented supportedVersions; ProtocolVersionError,nilclient,no notification,request1/closed strengthened |
| TestClient_RootsSnapshotOwnsMetadata | TestClient_InputRootsSnapshotOwnsMetadataWithoutRootsService | preserved | same raw nested origin owner caller detached under caller rawmutation; no attacker insertion leaks; host-owned inputResponses replaces rootsservice. Removed map/slice service mutations not applicable to raw currentport |
| TestClient_MissingCapabilityRejectedWithoutRPC | TestClient_MissingCapabilityRejectedWithoutRPC | preserved | typed CapabilityError+unchanged requestcount |
| TestClient_ToolStructuredResultMapsOutputSchemaAndTypedEnvelope | TestClient_ToolStructuredResultMapsOutputSchemaAndTypedEnvelope | preserved | same schemas/oktrue,proxyExecute,MIMEJSON/JSONdata,typed CallToolResult,envelopekind; required resultType/cache explicit |
| TestClient_ToolResultViolatingOutputSchemaFailsClosed | TestClient_ToolResultViolatingOutputSchemaFailsClosed | preserved | same okstring wrong →typedInvalidPayload |
| TestClient_OutputSchemaPreservesExactNumericConstraints | TestClient_OutputSchemaPreservesExactNumericConstraints | preserved | same exact minimum9007199254740993/value9007199254740992 typedfailure |
| TestClient_RemoteToolErrorHasDistinctEnvelope | TestClient_RemoteToolErrorHasDistinctEnvelope | preserved | same textbadinput/isError,CodeRemoteExecution/RemoteToolError |
| TestClient_CancellationUsesActiveRequestIDExactlyOnce | TestClient_CancellationUsesActiveRequestIDExactlyOnce | preserved | same activeID3/canceled caller/notificationcount1; bounded wait+locked snapshot removes race, does not relaxcount |
| TestClient_ProgressIsFractionalAndMonotonic | TestClient_ProgressIsFractionalAndMonotonic | preserved | same1.5/3.5 accepted;1regression+2afterterminal filtered,count1/messagefirst; real Completion registered before Deliver |

### Helper observations

Prepared/delivered captures are genuinely separate; current helper uses real preparedRequest/pendingRequest completion/cancel/claim lifecycle, not always-sent/always-canceled shims. Start checks ctx/closed; params and IDs cloned at capture+hook; result cloned before completion; peerWrites byte-cloned and independently recorded. Five tests assert inert preparation and private mutations (params.ClientInfo/ID/hook params/result), abort exactlyonce before traffic and late hook, single cancellation notification ownership, 64 racing completion workers hookcount1, and Close unblocking BOTH prepared and delivered pending. OnRequest/protocol setter/Initialize helper absent; no aliases. No weakened helper ownership/completion invariant found.

Extra held-Await progress proof shows terminal retirement before Await returns, accepted progress before empty terminal result, no callback leak; missingCompletion proof aborts prepared tools/call before delivery, requests2/preparations3/no notifications. These supplement, not replace, original fractional+monotonic source fixtures. Empty MCP result emits typed/enveloped empty content with empty MIME; new tests route three emptyresource cases through actual core proxy. None of these add denominator credit to erase missing inbound negatives.

### Independent diagnostic

Ran scoped `go test -race` against platform-selected production files and currently compilable test files, with explicit diagnostic omission of seven still-obsolete files; selected peer/stdio/client-contract/helper families count3: **PASS, command-line-arguments 4.883s**. This is not package/all-module verification, and not a claim excluded files are optional. Full make test/lint and both same-candidate final audits remain gates.

### Initial checkpoint gaps — closed on source recheck

- CP-MCP-001 — **closed**: `TestClient_LegacyPingRejectedBeforeAndAfterDiscovery` now calls peer.dispatch with id1, ping, params{} both before and after Connect; asserts both incoming errors as InvalidPayload, zero registered notification callbacks, empty peerWrites, and exactly one actual request (discovery). Outbound Prepare negatives remain. No handler API restored.
- CP-MCP-002 — **closed**: `TestClient_LegacyPingAndRootsRejectMalformedRequestMeta` now dispatches inbound id1 frames for each of ping and roots/list with params _meta:null; asserts incoming/outbound InvalidPayload, zero registered callbacks, empty peerWrites/preparations/requests and nil pending.

No runtime defect is claimed from these omissions: current peer rejects all server requests. They are missing adversarial assertions, so counterpart names alone do not certify all original scenarios.

Follow-up source review: **49/49 scoped originals reconciled**, with no remaining missing/weakened assertion in these three migrated suites and helper. Initial 47/49 result above is preserved as checkpoint history; this closure adds no credit for the other seven suites or the whole Task35 goal. A fresh targeted race diagnostic is recorded separately after completion.

Follow-up execution: exact two changed LegacyPing test names, scoped platform-selected production/current-compilable tests, `-race -count=5` → **PASS, command-line-arguments 1.522s**. First attempt could not read the system Go build cache under sandbox and failed setup; retry used `GOCACHE=/private/tmp/toolsy-completeness-go-cache` and completed. No production/test files edited. This remains diagnostic only, not a full-package gate.

## Independent protocol checkpoint — 2026-10-05 (20 originals)

Read all current protocol_contract_test.go, full original backup and complete diff, plus current RequestMeta/ResourceContents codec and forbidden-method validation order. No runtime/test files edited. All20 original Test functions mapped; added mandatory current resultType/cache/meta are explicit source, not automatic fixture repair. **19/20 reconciled at checkpoint; one partial forbidden-roots proof CP-MCP-004 remains below.** This is protocol checkpoint only, not overall Task35 percentage/acceptance. Original197checks+helper inventory and full121requirements denominator unchanged.

| Original suffix | Current suffix | Verdict | Exact observation |
|---|---|---|---|
| ResourceContents_HasNoNonStandardAnnotationsSurface | ResourceContents_HasNoNonStandardAnnotationsSurface | preserved | reflection still proves no typed Annotations field |
| ResourceContents_RejectsUnknownAndLegacyAnnotationFields | ResourceContents_UnknownAnnotationFieldsRemainInert | preserved current-contract replacement | both exact annotations{} and futuretrue inputs retained; typed field absent, data preserved Extra and JSON roundtrip. Current ResourceContents decoder captures inert additive fields; no domain annotation authority installed |
| ProgressToken_StringAndNumberWireContract | ProgressToken_StringAndNumberWireContract | preserved | string token/integer42/fraction1.5/exponent1e0 all string/JSON lexemes retained |
| EmptyResult_RequiresObjectAndPreservesExtensions | EmptyResult_RequiresObjectAndPreservesExtensions | preserved | null/array42/scalar fail; extended meta/extension roundtrip now explicit resultTypecomplete |
| Meta_KeyFormatIsValidatedOnBothDirections | Meta_KeyFormatIsValidatedOnBothDirections | preserved current-contract replacement | all ten original key inputs retained across encode/decode; empty and prefix-only moved from accepted to rejected under current identifiers |
| RequestMeta_ExtensionKeyFormatIsValidated | RequestMeta_ExtensionKeyFormatIsValidated | preserved | bad prefix/value tested encode/decode with VALID required self-describing metadata; no missing-meta masking |
| ProgressToken_RejectsNullAndCompositeValues | ProgressToken_RejectsNullAndCompositeValues | preserved | null/object/array/true unchanged negatives |
| CallToolResult_AllContentVariantsRemainLossless | CallToolResult_AllContentVariantsRemainLossless | preserved | all5 exact text/image/audio/resource_link/resource variants,annotations/icons/nestedmeta/structuredmeta retained; tagged complete added; no legacy base64/mediaType fields |
| ContentBlock_EmptyRequiredPayloadRoundTrips | ContentBlock_EmptyRequiredPayloadRoundTrips | preserved | emptytext and emptyimagebytes stay valid required payload; complete tag explicit |
| ResourceLinkSize_PreservesArbitraryJSONNumbers | ResourceLinkSize_PreservesIntegersAndRejectsFraction | preserved current-contract replacement | all3 original number lexemes retained; hugeinteger/exponent remain lossless; fraction1.5 negative decode+encode on otherwise valid complete/content/uri/name |
| StrictDTOsRejectPresentNullAndFractionalInteger | StrictDTOsRejectPresentNullAndFractionalInteger | preserved | all16 original named negative inputs retained+new discover_meta null (17total); tagged result/cache/currentdiscover fields explicit. Existing null _meta cases intentionally test metadata invalidity; initialize shape is retained opaqueextra onDiscover not an API alias |
| RequestDTOsRoundTripTypedProgressMetaAndExtra | RequestDTOsRoundTripTypedProgressMetaAndExtra | preserved | all5current method DTO counterparts preserve progress42/traceabc + required version/caps/clientInfo; Initialize replaced Discover |
| RequestMetaRejectsNullAndResetsOnReuse | RequestMetaRejectsNullAndResetsOnReuse | preserved | null fail preserves captured oldtoken/extra; {} now correctlyfails; validrequiredmeta resets optionalprogress/extra |
| RequestMetaRejectsReservedExtraCollision | RequestMetaRejectsReservedExtraCollision | preserved after CP-MCP-003 | both original progressToken forged/null Extra collisions retained; fixedvalidbase marshalpositive+requiredversion/clientInfo eachfixture+ErrorContains collisioncause. Earlier incompleteMeta guard test could pass for wrong reason |
| ResourceContents_TextAndBlobAreDistinct | ResourceContents_TextAndBlobAreDistinct | preserved | same texthello vs blobAAE pointerunion; explicit complete/cache added |
| ServerCapabilities_PreservesUnknownAdditiveCapabilities | ServerCapabilities_PreservesUnknownAdditiveCapabilities | preserved | same toolscap + futureenabledtrue roundtrip/Extra |
| ResultDTOsPreserveUnknownTopLevelExtensions | ResultDTOsPreserveUnknownTopLevelExtensions | preserved after CP-MCP-004 fix | all7originalfamilies/trace1 preserved: Initialize replaced currentDiscover;6currentfamilyvendor roundtrip; forbiddenroots original rawfixture retained with typed InvalidPayload and explicit legacy-method cause, nil pending/no preparation/no request |
| ResultDTORejectsExtensionCollisionWithReservedField | ResultDTORejectsExtensionCollisionWithReservedField | preserved | original Extra.content shadow rejected; validcontent[]/complete fixture prevents tagmask |
| ServerCapabilities_KnownCapabilitiesMustBeObjects | ServerCapabilities_KnownCapabilitiesMustBeObjects | preserved | toolsnull/loggingtrue/resourcesarray all original inputs retained |
| PrimitiveMetadata_RoundTripsOnCurrentWireDTOs | PrimitiveMetadata_RoundTripsOnCurrentWireDTOs | preserved current-contract replacement | Icon dark/numericprogress7+0.5/cancelidx/resourceupdatedtrace unchanged; raw root payload bytepreserved as host-owned opaque field, no removed Root type/service reinstated |

### Wrong-reason negative audit

CP-MCP-003 — **closed on recheck**. Initial RequestMeta collision fixtures lacked required version/clientInfo; current production catches Extra collision first, but require.Error alone could still pass after removing collision guards because invalid metadata would fail later. Current fixtures now have validrequired metadata and validbase positive, each failure contains `collides with reserved key`. Both forged-string and typed-token+null Extra source inputs retained. Independent fixed-test scoped race/count5 PASS **1.485s**.

CP-MCP-004 — **closed on source recheck**. Roots-list counterpart in top-level extension matrix sends original raw roots/vendor result as forbidden request params; raw intentionally lacks current request metadata. Runtime rejects `roots/list` before parsing params. Initial generic InvalidPayload/nil pending/no preparations could remain green if the forbidden-method guard regressed, failing later on missing metadata. The current branch now requires `legacy method` error cause, so missing-metadata failure cannot satisfy it. Original roots/vendor raw input and no-traffic assertions remain. No runtimebug asserted.

Other negative fixtures reviewed: CallToolResult and ResourceRead/PromptList nullfield cases have mandatory tag/cache; Discover capabilityflag-null has versions/cache/capability/validserver metadata; fractionalresource_link otherwisevalid. RequestParams/Discover/Cursor/Tool/Resource/Prompt null _meta are intentionally about invalid/nullmetadata, so not hidden field tests. Invalidrequest extension key has validmandatory metadata bothdirections. Reservedcontent Extra collision has valid complete resultbase.

### Independent diagnostic

Scoped race/count3 using production GoFiles + protocol_contract_test.go/task34_contract_test.go/test_support_test.go/client_contract_test.go/transport_http_migration_test.go PASS **4.649s**. That run overlapped CP-MCP-003 sourcefix; therefore it is not used to certify fixedcollision snapshot. Exact post-fix collision test run above provides that evidence. Full package/allmodule gates and final independent fullcandidate audits remain pending; the other six original suites are not exempted by this diagnostic.

Follow-up source verdict: **20/20 protocol originals reconciled**, CP-MCP-003/004 closed, no missing input fixture or weakened negative remains in this set. Initial19/20 is checkpoint history, not the current scoped verdict. This does not advance whole-goal acceptance or claim the remaining original suites are complete.

Fresh independent scoped execution after reading both fixes: same listed production + five support/test files, `go test -race -count=3` → **PASS, command-line-arguments 5.567s**. Only this repeat is the full protocol-checkpoint runtime evidence for both fixed negative assertions; still not full-package/all-module verification.

## Independent client-features checkpoint — 2026-10-05 (13 originals)

Read complete current client_features_test.go and immutable original diff, compared all original assertions/case values and inspected current Listen/awaitSubscription/invalidation/logger routing. **Initial checkpoint: 11/13 reconciled, 2 partial assertion proofs CP-MCP-005/006 below.** No runtime defect is asserted; current source executes appropriate validation. This is only migrated feature-suite completeness, not a whole-goal verdict.

| Original/current TestClient suffix (URI test has no Client prefix) | Verdict | Evidence |
|---|---|---|
| InvalidListChangedMetaDoesNotAdvanceGeneration | preserved after CP-MCP-006 fix | Original _meta:null/gen0/emptyevent retained; ACKed source ID2/effectiveToolsfilter and positive same-ID generation1/event control now exercised; invalidcase requires missingmetadata stateerror/canceled state/no sourceevents |
| SubscribeResourceRequiresObjectResultBeforeMutation | preserved after CP-MCP-005 fix | All4 original null/[]/42/scalar terminals retained; Done then zeroactive registry; original typed InvalidPayload ErrorAs restored |
| SubscribeResourceAcceptsExtendedEmptyResult | preserved current-contract replacement | same trace x/extension oktrue input preserved with explicit complete/subscriptionId and matching Resource ACK; successful terminal+Done replaces removed persistent SubscribeResource bit; current terminal cleanup is correct, not lost subscription support |
| CloseIsConcurrentIdempotentAndClosesEventChannels | preserved | same32concurrentcalls/Closecount1/noerrors/Invalidationsclosed; added active current subscription Done/Eventsclosed. Removed global LogMessages replaced host logger, not retained channel API |
| ToolDescriptorFailuresAreFailClosed | preserved current-contract replacement | missing schema/nullschema/has space still typed InvalidPayload; original requiredtaskSupport now preserved opaque Extra.execution and roundtripped, valid proxy created; no task API/capability installed under current additive extension contract |
| ToolsListChangedMakesExistingProxyStale | preserved | same descriptor/stale Execute typedStale/generation1/invalidationkind; valid ACK/filter/matching subscriptionID required. Default hook failure+typedStale ensures staleproxy cannot reach RPC |
| ContradictingInvalidationIsIgnored | preserved | original tools.ListChangedfalse retained; unsupportedListen typedCapability/nilbeforewire, requestcount1; unauthorizedID2event cannotadvancegen/event |
| ResourceUpdateRequiresNegotiatedSubscription | preserved | original filewatched valid subscribed event retained via current matchingACK+activeid; same kindResource/URIwatched |
| ResourceSubscriptionAcceptsSubResourceUpdate | preserved | original watched/child event retained via current matchingACK+activeid; same kindResource/URIchild |
| ResourceUpdateMatchesSubscriptionURIBoundaries | preserved | All14 original names/subscription/updated/want and assertion byte-identical except replacement function invocation. No expected outcome relaxed |
| LoggingNotificationAllowsNullData | preserved current-contract replacement | same info/datanull input now host JSONlogger only while requestID2 active; data literalnull preserved, no removed globalchannel/cap authority |
| LoggingNotificationRequiresCapabilityAndPreservesData | preserved current-contract replacement | same info/logger server/dataoktrue input; host INFO/logger/data preserved; before activecorrelation and afterretirement nooutput added. Loggingcap no longer authorizes unsolicited global logs |
| PromptTaggedContentAndMetadata | preserved | same user/resource content/prompttext/fileURI/nestedMeta x1 retained; explicit completeresult added; GetPrompt typedcontent/resource metadata assertions unchanged |

### Exact URI inventory

The full original table and assertion block compare byte-equal after substituting only `resourceUpdateMatchesSubscription(fixture.subscription, fixture.updated)` → `resourceSubscriptionAllows([]string{fixture.subscription}, fixture.updated)`. All14 names retained: exact; child; sibling; prefix collision; host case normalized; dot segment escapes subtree; encoded dot segments escape subtree; encoded unreserved path; encoded slash remains segment data; default HTTPS port; repeated slash remains distinct; reserved fragment escape remains distinct; different origin; query identity exact only. Both positive and false expectations remain original. This does NOT certify any later expanded adversarial URI tests or fix under separate correctness review.

### Assertion gaps, not API-restoration requests

CP-MCP-005 — **closed on source recheck**: malformed subscription terminals all use original raw inputs and settle/noactive checks. Initial generic Error weakened original ErrorAs(*InvalidPayloadError); current ErrorAs is restored for each of four fixtures. Timeout/cancel/missing ACK cannot satisfy typed payload validation. Missing mandatory resultType is intentional for these original nonobject inputs, not automatic valid fixture repair.

CP-MCP-006 — **closed on source recheck**: current fixture adds ACKed subscription state ID2/requested+effectiveToolsfilter. Same valid-ID notification on positive control yields generation1/global invalidation1/sourceevent1, which is drained before invalidcase. Original _meta:null now asserts clientgen0/no global event/no sourceevent, state.err containing `missing metadata`, and canceled statecontext. Thus missing-source/no-active-subscription no-op cannot satisfy this test. No legacy handler API restored.

### Independent diagnostic

Platform production GoFiles + current features/protocol/task34/helper/client_contract/http_migration support suites, selected13 feature names, race/count3 → **PASS, command-line-arguments 1.679s**. This run proves executions only; it does not close the two incomplete assertion proofs, nor excuse the other five original files or full package/allmodule gates. Original197checks+helper inventory and Task35 denominator121 remain intact.

Follow-up source verdict: **13/13 feature originals reconciled**, CP-MCP-005/006 closed; all original14 URI expectations unchanged. Initial11/13 is historical checkpoint. No whole-goal acceptance or expanded URI correctness claim is made.

Fresh independent post-fix diagnostic: exact13 feature test names with the listed current production/support files, `-race -count=3` → **PASS, command-line-arguments 1.713s**. No remaining weakened assertion/fixture found in this suite. Five other original files and full-package/all-module gates remain pending; this run deliberately does not compile those files and is not their disposition.

## Independent HTTP checkpoint — 2026-10-05 (5 originals / 6 scenarios)

Read complete current transport_http_test.go, immutable original and full diff; inspected actual request-scoped SSE provenance/parser ordering. **Initial checkpoint: 2/5 fully reconciled, 3 partial cause-specific negatives (CP-MCP-007).** All five originals and both fifth-test subscenarios retained; no old lifecycle restored. This is a scoped assertion review, not fullgoal acceptance.

| Original suffix | Current counterpart | Verdict / exact evidence |
|---|---|---|
| JSONSessionVersionAndDeleteContract | JSONVersionIgnoresSessionAndNeverDeletes | preserved current-contract replacement: original session-1 advertisement inert; two explicit correlated JSON POST successes; current version header on FIRST and second POST; JSON/SSE Accept preserved; empty session/cursor; after Close methods exactly POST,POST proves no DELETE/GET/initialized |
| POSTAcceptsSSEResponse | same | preserved: event-1 and oktrue original payload retained; now actual request ID copied, framed POST SSE success/Await and Close noerror |
| GETResumesAndHandlesServerRequest | ServerRequestFailsWithoutGETResumeOrReply | partial CP-MCP-007: exact event-a/server-1/roots-list input retained over supported POST; typed InvalidPayload, Last-Event-ID empty, calls1 after Close proves no resume/reply/retry; currently generic failure could be EOF rather than frame rejection |
| MalformedIncomingRequestReceivesInvalidRequest | MalformedIncomingRequestFailsWithoutReply | partial CP-MCP-007: original version1.0/server-invalid/roots-list frame unchanged over POST; typed InvalidPayload/no extra call replaces unsupported responder; error must prove frame rejection rather than ignored frame then EOF |
| SessionExpiryAndLegacyEndpointFailClosed | HTTP404AndLegacyEndpointFailClosed | partial CP-MCP-007 only legacy endpoint branch; session expiry keeps original session advertisement+404, asserts HTTPError status404/operationPOST/calls2/no forwarded session/no retry. endpoint/message exact bytes retained, typed InvalidPayload/calls1; add parse failure cause instead of generic EOF |

decodeRPCEnvelope and writeResponse retain their original JSON decode/Response exact correlation helper behavior. httpMigrationRequest uses a valid current DiscoverParams with mandatory metadata, explicit Prepare/Deliver/Await and bounded5second context; no wrong-reason missingmeta failure at preparation. Header/method/call-count assertions are checked after transport.Close, so asynchronous lifecycle cleanup cannot conceal already initiated GET/DELETE/reply traffic.

### CP-MCP-007 — initial wrong-reason negative gap

Both server-request framed payloads currently fail at validateSSEProvenance with `server-initiated request is forbidden` before the peer version check. This is correct clear-break behavior: malformed-version server request cannot revive a client responder or require an outgoing InvalidRequest RPC. The tests need that exact forbidden-request failure cause; generic InvalidPayload alone also accepts a regression which drops the frame and later fails EOF-before-terminal.

The old endpoint data `/message` is not valid JSON: current parser wraps JSON decode failure in InvalidPayload. Preserve this concrete cause, not merely EOF failure. Event endpoint discovery stays absent; no compatibility alias needed. No runtime defect claimed.

Independent scoped production+HTTP5/http_migration/task34 race/count3 PASS **1.851s** before cause-strengthening. This proves execution only, not CP-MCP-007 closure. Four remaining original files and actual full-package/allmodule gates remain pending.

### HTTP follow-up closure

CP-MCP-007 **closed** on independent source recheck: both incoming server-request branches now require `server-initiated request is forbidden`; the legacy endpoint branch requires `ErrorAs(*json.SyntaxError)` in addition to InvalidPayload. Ignored-frame/EOF cannot satisfy these assertions. First POST also explicitly asserts MethodServerDiscover. All original fixtures and post-Close request counts remain unchanged.

Current scoped verdict: **5/5 HTTP originals reconciled**, including both fifth-test scenarios. The 2/5 table above records the initial checkpoint, not the current verdict. Fresh independent production+HTTP5/http_migration/task34 `-race -count=3` **PASS, command-line-arguments 1.599s**. No whole-goal acceptance; remaining files and full gates are not covered by this run.

## Independent stdio-contract checkpoint — 2026-10-05 (15 originals + subprocess helper)

Read current suite, immutable backup, full diff and actual readLoop/write/process lifetime paths. All15 original Test scenarios and helper modes remain accounted for. **Initial current verdict: 13/15 reconciled, 2 partial assertion replacements (CP-MCP-008)**. Process ownership changes below are current-contract replacements, not an excuse to drop cancellation/process-tree invariants.

| Original TestStdio suffix | Current counterpart / disposition | Exact evidence |
|---|---|---|
| StderrVolumeAndLongLinesNeverCrashTransport | same / preserved | same stderr 2×scanner-limit line plus next line; forwardStderr leaves closedfalse/terminalErrnil |
| RequestDrivenServerReceivesInitializeBeforeWriting | RequestDrivenServerReceivesDiscoveryBeforeWriting / preserved replacement | Start context canceled before explicit Prepare+Deliver; helper waits for first request, asserts server/discover/version, supportedVersions contains current version; result succeeds, Close/stderrDone retained |
| ProcessExitUnblocksPendingRequest | same / preserved | exit-without-response mode; bounded3sec Await error; processDone1sec before external Close |
| NonzeroProcessExitWinsStdoutEOFRace | same / preserved | exact exit7 mode; typed TransportCrash AND exec.ExitError instead of generic EOF |
| ClosedStdoutTerminatesAndReapsLiveProcess | same / preserved | stdout closed then10sec sleep; Await error and processDone1sec prove live process reaped |
| BlockedWriteHonorsContextCancellation | same / preserved replacement | original4MiB payload/never-read helper/activeWrites>0/request ID/CallerCanceled preserved; added required_meta prevents preparation masking. Active cancel leaves transport open; explicit Close now owns processDone5sec/writerDone/repeated Close |
| CancellingQueuedWriteDoesNotAbortActiveWrite | same / preserved replacement | original4MiB activewrite/count1/queue length1, queuedCanceled and process alive100ms retained; healthy Prepare+Deliver succeeds; activeCanceled now explicitly leaves transport open. Explicit Close settles healthy ErrTransportClosed and reaps process, replacing forbidden cancellation-triggered globalCrash |
| WriteLoopRechecksCancellationAfterActivation | same / preserved | pre-canceled write selected from queue: contextCanceled/zero recordingWriter calls/stateCancelled/writerDone |
| SuccessfulWriteIsFinishedBeforeDeliveryCallback | same / preserved | held onSent callback, abortWrite leaves stateFinished; Never(closed)100ms; release then writeNoError/writerDone |
| InvalidUTF8DoesNotCorruptCorrelation | InvalidUTF8FailsClosedWithoutReply / partial CP-MCP-008 | exact0xff newline retained; bounded3sec Await typedInvalidPayload AND exact UTF8 cause/emptyresult/Close. Current readLoop closes on dispatch error, so old success-after-protocol-reply forbidden. No-reply assertion is not parent-observable yet |
| MalformedCorrelatedResponseUnblocksPendingRequest | same / preserved | all4 exact original helper modes/raw response frames retained: result+error, stringerror, version1.0, missingversion; each bounded1sec Await typedInvalidPayload/Close. Explicit valid request metadata prevents preparation masking |
| ProcessExitDetectedWhileDescendantHoldsStdout | same / preserved | distinct inherited stdout child sleep3 fixture/POSIXskip; bounded2500ms typedCrash/processDone1sec beforeClose |
| ProcessExitDetectedWhileDescendantHoldsStderr | same / preserved | distinct inherited stderr child sleep3 fixture/POSIXskip; bounded2500ms typedCrash/processDone1sec beforeClose |
| CloseKillsDescendantProcessTree | same / preserved | original child PID correlated result/processExists positive control; Close then Eventually !processExists2sec |
| MalformedIncomingRequestReceivesInvalidRequest | MalformedIncomingRequestFailsClosedWithoutReply / partial CP-MCP-008 | exactversion1.0/server-invalid/roots-list frame retained; Await typedInvalidPayload AND exact invalid-version cause/emptyresult/Close replaces removed responder-success. No-reply assertion is not parent-observable yet |

### Subprocess helper and CP-MCP-008

Helper is support, not a sixteenth acceptance test. All original modes and distinguishing process conditions retained. Explicit os.Exit(0) after discovery and pre/post-read terminal modes avoids Go test PASS output contaminating protocol stdout. Four malformed correlated frames are unchanged. The removed protocol reply validation is correctly replaced by failure behavior, not by a compatibility responder.

However, exerciseStdioIncomingProtocolError calls os.Exit(9) if it reads any reply, and neither parent test checks that child verdict. Parent's original InvalidPayload remains settled even if an erroneous reply was sent; Close returns nil independently of that child exit. Thus this helper branch is not evidence of no reply. Existing TestRPCPeer_MalformedIncomingRequestsFailClosed has zero-sent assertions for version1.0 roots/list (different ID), but no exact UTF8 zero-sent assertion found in the selected peer/task34/stdio suites. CP-MCP-008 requests parent-observable zero outbound writes for BOTH exact replacement fixtures (current peer send-spy proof or actual-process capture), not restoration of legacy replies. No production defect asserted.

### Independent execution

Current platform production GoFiles + transport_stdio_contract_test.go, transport_http_migration_test.go, task34_contract_test.go, process_exists_unix_test.go; selected ^TestStdio_, `-race -count=3` → **PASS, command-line-arguments 20.799s**. This diagnostic includes all15 current counterparts/helper process execution but does not close CP-MCP-008. Three remaining original files and full package/allmodule gates remain pending. Original197checks+helper ledger and Task35 denominator121 unchanged; no overall percentage or acceptance inferred from this scoped run.

### Stdio-contract follow-up closure

CP-MCP-008 **closed**: BOTH process counterparts now invoke assertStdioInputProducesNoReply with the exact corresponding scanner payload (0xff; version1.0/server-invalid/roots-list). This uses a real newRPCPeer with parent-visible captured send frames, requires typedInvalidPayload/exactcause and Empty(sent). Existing actual-process typedcause/emptyresult/Close checks remain. The peer spy is the explicit no-reply proof; unobserved helper exit9 is not counted as evidence. Current scoped verdict **15/15 originals reconciled**, plus helper support retained. Independent exacttwo post-fix race/count3 **PASS, command-line-arguments 1.910s**. No overall acceptance.

## Independent client checkpoint — 2026-10-05 (25 originals)

Read complete immutable/current client_test.go and diff, current fakeTransport preparation/delivery/finish and production mapping/result paths. **25/25 original tests reconciled**; new public-list table is additional coverage, not an original-counterpart count. No weakened assertion or missing original scenario found in this scoped migration. The preserved original ledger is unchanged.

| Original suffix (full prefixes retained below) | Current disposition / assertion evidence |
|---|---|
| TestNotifyCancelledRequestUsesBoundedContext | preserved via notifyCancelledPending/current raw request ID req-1; same canceled parent/trace-123, detached un-canceled notify context, MethodCancelled and deadline afterbefore/beforeafter+6sec |
| TestConnect_EagerHandshakeSuccess | current discovery replacement: Startcount1/progress handler/current discover/ready/serverinfo, successful result and Closecount1 retained; original initialize/version+roots/initialized authority replaced explicit NO roots/top-level protocolVersion/notifications and mandatory metadata version+emptycapabilities |
| TestConnect_StartFailureClosesTransport | preserved original startfailed input, error/nilclient/Closecount1 |
| TestConnect_InitializeCallFailureClosesTransport | DiscoveryCallFailureClosesTransport; original callfailed input, error/nilclient/Closecount1/current method |
| TestConnect_InitializeParseFailureClosesTransport | DiscoveryParseFailureClosesTransport; exact invalid_json input, error with current result-subject/nilclient/Closecount1 |
| TestConnect_InitializedNotifyFailureReturnsErrorAndClosesTransport | LegacyInitializedNotificationIsAbsent; same notifyfailed injection+valid discovery; success/nonnilclient/notifiedMethod empty/notifiedParamsnil/Closecount1 prove removed notification cannot be attempted or fail discovery |
| TestHandleToolCallResult_ErrorChunkUsesStructuredWire | ToolResult_ErrorChunkPreservesTypedRemoteEnvelope; same permissiondenied text+IsError preserved, explicit completeresult; current chunk IsError/text MIME+exactbytes, typedResult JSON equality, RemoteExecution envelope/RemoteToolError typedoriginalcontent. Old flatten-to-ValidationFailed/toolerrorJSON intentionally forbidden by current typed remote result contract |
| TestHandleToolCallResult_ReadLimitExceeded_MapsValidation | retained name; removed handler replaced current mapCallReadLimitFor; same ErrReadLimitExceeded/CodeValidationFailed/default limit reason |
| TestGetResourceTool_ReadLimitExceeded | preserved publictool Execute with exact file:///x input and stream callback; same typedValidation/default bytecap |
| TestGetPrompt_ReadLimitExceeded | preserved public GetPrompt greeting/nilargs; same typedValidation/default bytecap |
| TestGetResourceTool_ReadLimit_CustomStreamCap | preserved publictool Execute/file:///x/cap2048; typedValidation/reason2048/NOT defaultcap |
| TestHandleToolCallResult_ReadLimit_CustomStreamCap | retained name; current mapping helper/cap2048/same readlimit typedValidation/exactcap reason |
| TestGetTools_MapsAnnotationsToManifest | preserved read_tool/read only/objectschema/readOnlytrue and delete_tool/destructive/objectschema/destructivetrue/idempotenttrue; same2 tools/byName/ReadOnly/Dangerous/Idempotent assertions, complete/cachefields merely make valid current list result |
| TestConnect_Initialize_ReadLimitMapsValidation | Discovery_ReadLimitMapsValidation; same readlimit input/typedValidation/byte limit/current discovery subject+method |
| TestGetTools_ReadLimitExceeded | preserved public iterator same error/typedValidation/byte limit/toolslist subject |
| TestGetPrompts_ReadLimitExceeded | preserved public iterator same error/typedValidation/byte limit |
| TestConnect_Initialize_ReadLimit_CustomStreamCap | Discovery_ReadLimit_CustomStreamCap; same cap2048/readlimit/typedValidation/exactcap reason |
| TestGetTools_ReadLimit_CustomStreamCap | preserved public iterator same cap2048/readlimit/typedValidation/exactcap reason |
| TestGetPrompts_ReadLimit_CustomStreamCap | preserved public iterator same cap2048/readlimit/typedValidation/exactcap reason |
| TestMapCallReadLimit_CancelOverLimit | preserved canceledcontext plusreadlimit/ErrorIsCanceled |
| TestMapCallReadLimit_DeadlineOverLimit | preserved deadline now-1sec plusreadlimit/ErrorIsDeadlineExceeded |
| TestMapCallReadLimit_TimeoutOverLimit | preserved wrapped slow ErrTimeout/ErrorIsTimeout |
| TestHandleToolCallResult_CancelOverReadLimit | removed private handler→current mapping helper, same canceledcontext/readlimit/ErrorIsCanceled; production toolterminal path invokes same mapper |
| TestMapCallReadLimitFor_InterruptInChainOverReadLimit | preserved exact stream wrapper+Join(Canceled,ReadLimitExceeded)/ErrorIsCanceled |
| TestHandleToolCallResult_InterruptInChainOverReadLimit | removed private handler→current mapper, same stream wrapper+Join/ErrorIsCanceled; production toolterminal path invokes same mapper |

### Fixture validity and replacement boundaries

connectCaptureTransport now delegates to current fakeTransport: Prepare validates required outgoing metadata and started state, Deliver records actual wire request and completes real pending state. Errors/results are injected ONLY after delivery, not before mandatory metadata checks; explicit newReadyCaptureClient starts transport and supplies ready/capabilities/schema maps/logger/subscriptions, so disconnected/unsupported/preparation errors cannot satisfy limit assertions accidentally. Discovery success fixtures include complete/supportedVersions/cache/serverinfo; the original test-server/version identity is retained. Malformed discovery exact bytes unchanged. Removed requestID-only private handler wrappers do not lose an asserted correlation invariant (those old mapper tests never asserted their req-limit IDs).

Legacy roots option and initial notification are not reinstated: the former positive /workspace handshake input is obsolete library-owned roots behavior, replaced no-roots wire assertion; other migrated contract tests independently exercise prohibited roots metadata/callback paths. Result helper replacement is grounded in the production proxy terminal buildToolResultChunk call and mapper, not an unused test-only alternate. It does not by itself claim new public proxy integration coverage.

TestClient_ListBoundariesMapReadLimitsAndPreserveInterrupts adds three public list APIs × default/2048 =6 limit cases plus3 joined-interrupt cases, with exactsubject/code/bytecap/underlyingcause and no erroneous validationmapping of interrupts. These are additional current regressions and do not inflate original25 count. Two remaining original files and authoritative full gates are not accepted by this checkpoint; Task35 denominator121 unchanged.

Independent diagnostic: production GoFiles + client_test.go/client_contract_test.go/test_support_test.go/task34_contract_test.go/transport_http_migration_test.go, selected current25 names plus new list-boundary table, `-race -count=3` → **PASS, command-line-arguments 1.625s**. Initial diagnostic omitted client_contract_test.go (owner of mustJSON helper), so failed compile with undefined mustJSON; corrected explicit support selection above passed. No whole-package/allmodule claim follows from the scoped run.

## Independent legacy-SSE migration checkpoint — 2026-10-05 (19 originals)

Read entire immutable/current transport_sse_test.go, all changed bodies and current result mapper/preparation/terminal ownership contracts. **19/19 original scenarios reconciled** as retained security invariants or explicit forbidden-legacy replacements. Current suite has20 test functions: the added exact-frame three-case test is extra evidence, not a twentieth original. No missing/weakening found in this scoped mapping; original inventory stays unchanged.

| Original TestSSETransport suffix | Current TestSSEMigration counterpart | Disposition / evidence |
|---|---|---|
| resolvePostURL_ValidateRemoteURL_BlocksPrivateIP | ConfiguredPostURL_BlocksPrivateIP | caller-owned URL resolution of exact example.com/sse→127.0.0.1:8080/rpc; original ValidateRemoteURL(false) rejects plus actual configured transport Start rejects |
| resolvePostURL_AllowPrivateIPs | ConfiguredPostURL_AllowPrivateIPs | same originalbase/target resolution/true override; validator and actual configured Start succeed; Close succeeds. Removed server-owned endpoint discovery not revived |
| ValidateInitialURL_BlocksPrivateIP | ValidateInitialURL_BlocksPrivateIP | exact127.0.0.1/sse validator negative retained, current Start negative added |
| notify_Non2xxStatus | HTTPNotificationsForbiddenWith500Fixture | original500/messages/notifications-test retained; explicit POST typedHTTP500 provesserverfixture; current validobject notification typedInvalidPayload with exact undefined-notifications cause/calls1 proves no notification traffic instead of removed notification POST |
| call_Non2xxPOSTStatus | Call_Non2xxPOSTStatus | exact messages path/HTTP400 fixture and tools/list request retained; typedHTTPError status400 strengthens old error-string assertion |
| Start_Non2xxGET | StartDoesNotGET500Endpoint | original500 fixture retained; Start success/zerotraffic proves GETremoved; subsequent realPOST typedHTTP500/calls1 |
| Start_Accepts204GET | StartDoesNotGET204Endpoint | original204 fixture retained; Start success/zerotraffic; realPOST with same200ms bound typedInvalidPayload containing Content-Type/calls1, no false RPC success or unsupported GET |
| Start_CancelBeforeEndpoint | Start_PreCancelledWithoutEndpointWait | obsolete100ms waitforGET removed; pre-cancelledStart ErrorIsCanceled/zerotraffic/Close preserves caller interruption and forbids endpointwait. Original keepalive/noendpoint server fixture retained; active request cancellation tested next |
| Call_CancelUnblocksPending | Call_CancelUnblocksPending | same tools/list cancellation afterserveracceptance, now real request-scopedPOST keepalive; bounded3sec, ErrorIsCanceled AND serverfinished1sec/calls1. Removed GETendpoint+202 lifecycle is not allowed |
| ExceedsMaxStreamBytes | ExceedsMaxStreamBytes | exact50×200 payloadvolume/cap2048/currentPOST/tools-list/ErrorIsReadLimit retained; comments replace syntactically invalid RPC data only in cumulativebyte fixture. Originaldata/control exactbytes separatelynegativetested below |
| finishSSECallResponse_CancelOverReadLimit | ResultBoundary_CancelOverReadLimit | same canceledcontext+ReadLimit/ErrorIsCanceled via production currentmapper; removed private legacy responsehelper not retained |
| finishSSECallResponse_CancelOverStaleStream | ResultBoundary_CancelOverStaleStream | same canceledcontext+exact sse-stream-closed error/ErrorIsCanceled via production currentmapper |
| finishSSECallResponse_LimitWithoutCancel | ResultBoundary_LimitWithoutCancel | same noncancelledReadLimit/ErrorIsReadLimit plus typedValidation/defaultcap reason |
| finishSSECallResponse_InterruptInChainOverReadLimit | ResultBoundary_InterruptInChainOverReadLimit | same stream wrapper+Join(Canceled,ReadLimit)/ErrorIsCanceled; Same(composite) strengthens intact cause |
| streamLimitErr_InterruptOverReadLimit | TerminalInterruptOverReadLimit | same joinedcause now terminates actual transport with prepared waiter; Await contains BOTH Canceled andReadLimit, no legacy global stream helper |
| getPostURL_StreamLimitBlocksEndpoint | StreamLimitPrecedesForbiddenEndpoint | original absolute example.com/messages endpoint retained AFTER50×200 comments withcap2048; actualPOST ErrorIsReadLimit/calls1 proves exhausted bytes prohibit processing/following endpoint. No endpoint URLservice |
| getPostURL_CancelOverStreamLimit | PreCancelledPrepareOverTerminalLimit | actual current transport terminatedReadLimit + canceledctx; Prepare returnsCanceled/nilpending/requestIDcounter0 |
| call_CancelOverStreamLimit | PreCancelledCallOverTerminalLimit | original terminalReadLimit + canceledctx/currentmapper failureCause preservesCanceled/NOTReadLimit; paired currentPrepare counterpart proves no request preparation. This is a private result-boundary check, not an additional public Call integration claim |
| waitSSECallResponse_DeadlineOverReadLimit | ResultBoundary_DeadlineOverReadLimit | exact expireddeadline now-1ms/readlimit/ErrorIsDeadlineExceeded/NOTReadLimit preserved via production currentmapper. Removed global waiter/timeout helper not retained |

### Exact old frame fixtures and masked-negative checks

Additional OriginalEndpointAndMalformedDataFramesFailClosed table explicitly retains `event: endpoint\ndata: /messages\n\n`, absolute example.com/messages endpoint and `data: ` +200x bytes. Every input requires InvalidPayload AND json.SyntaxError plus singlePOST; ignoredframe/EOF cannot satisfy syntax cause. The cumulative50×200 fixture is independently valid comment framing, so malformedJSON no longer preempts the desired ReadLimit assertion. This is a truthful separation of incompatible legacy control frames from retained bytebudget behavior, not weakened security.

Every HTTP request uses valid current metadata, explicit Prepare/Deliver/Await with3second upperbound. Notification payload is a valid object, so its exact forbidden-client-notification cause cannot be masked by malformed params. HTTP500/400 typedstatus and204ContentType assertions cannot pass missingmeta/timeout. Configured private-address validators are still independently checked, not merely transport constructors. New prepared-waiter terminal test injects cause beforedelivery, deliberately avoiding networkaccess; new pre-cancel preparation proves requestID0. No current endpoint resolution API aliases or GET/reconnect runtime retained.

One original file (adversarial51) and authoritative full-package/allmodule gates remain pending. Original197 acceptance checks+helper inventory and Task35 denominator121 unchanged; no wholegoal percentage/acceptance derived here.

Fresh independent diagnostic: production GoFiles + transport_sse_test.go/transport_http_migration_test.go/task34_contract_test.go, selected ^TestSSEMigration_, `-race -count=3` → **PASS, command-line-arguments 1.444s**. This executes all19 counterparts plus additional exact-frame table; it does not certify remaining adversarial51 or authoritative gates.

## Independent adversarial peer checkpoint — 2026-10-05 (12 of51 originals)

Read all current adversarial_peer_migration_test.go and original corresponding12 bodies (including original later duplicate-response test). Initial complete-function comparison found11 bodies byte-identical, but execution exposed invalid original nil params and a mismatched negative error literal. Thus that initial source-only preservation did NOT establish current acceptance. After mandatory fixture repair, independent complete-function comparison finds **11 bodies identical apart from nil→struct{}{} request params** (two need no repair). Twelfth is explicitly obsolete server-handler permission replaced by concurrent fail-closed client behavior. Remaining39 adversarial originals pending, not silently counted. In the table below, byte-identical refers to original assertions/scenario structure with this mandatory validobject params substitution.

| Original TestRPCPeer suffix | Disposition / evidence |
|---|---|
| BeginRequestCloseRaceNeverLeaks | byte-identical512 begin/Await/Close race, everyerror ErrTransportClosed |
| IncomingRequestConcurrencyIsBounded | replaced IncomingRequestsAreForbiddenUnderConcurrentLoad:513 concurrent exact ping/version2.0/numericID requests; each typedInvalidPayload/exact server-initiated-forbidden cause; atomic repliescount0. Removed server-handler admission/overload-response contract is forbidden now, not retained positivebehavior |
| RejectsOversizedRequestIDBeforeNumericCanonicalization | byte-identical oversized exponent 1e + maxRPCIDBytes nine digits rejected |
| ResponseCancellationRaceConsumesOneTerminalResponse | byte-identical256 response/cancel races; first dispatchNoError; AwaitsuccessORCanceled; duplicate rejected |
| ExternalCancellationUnblocksBackgroundAwait | byte-identical CancelPendingtrue/backgroundAwaitCanceled within1sec |
| CancelledResponseBookkeepingRemainsBounded | byte-identical maxRanges+16 contiguous cancellations, exactrange shrink upon retainedlastresponse; usable long-livedpeer/nextrequest afterstorm |
| CancellationStormNeverMasksDuplicateCompletedResponse | byte-identical completedresponse beforestorm; maxRanges+16 cancellations; completedduplicate typedInvalidPayload |
| OutOfOrderCancellationsRemainExactlyCorrelated | byte-identical secondthenfirst cancel, bothlate responsesaccepted, cancelledRangesempty |
| CancellationRangeInsertionAndBridgingStaySorted | byte-identical3 permutations [3,1,2]/[2,3,1]/[3,2,1], exactsingle range1..3 |
| CancellationRangeFragmentationFailsClosedAtBound | byte-identical alternating canceled/completed IDs untilbound; overflow cancellation closespeer and after-overflow rejects ErrTransportClosed |
| LateCancelledResponseSplitsRangeAndDuplicateFailsClosed | byte-identical3 canceledIDs/middlelatefirstaccepted/duplicatetypedInvalidPayload/exacttwo remaining singletonranges |
| DuplicateResponseFailsClosedAndNotifyAfterCloseStops | byte-identical firstsuccess/duplicatetypedInvalidPayload, close thenNotify ErrTransportClosed |

Concurrency negative now checks actual `server-initiated requests are not supported` cause, so absent mandatory params/metadata cannot mask a wrong failure after accidental server-handler resurrection. Current dispatch path rejects synchronously before any request-handler authority; all513 goroutines finish before replycount assertion. This review does not reauthorize legacy handler installation or assert obsolete overload codes.

Independent initial scoped race/count3 **FAIL**: lifecycle fixtures with nilparams failed required wire Request.params validation before exercising their intended races; concurrency assertion expected a different cause string. Root repaired only validobject input and exact existing cause; production unchanged. Fresh execution is required before accepting all12. This is a test-migration inventory review, NOT Task35 completeness or full-workspace gate acceptance. Task35 denominator121 unchanged.

Fresh independent production GoFiles + adversarial_peer_migration_test.go, selected ^TestRPCPeer_, `-race -count=3` → **PASS, command-line-arguments 3.290s** after those source-rechecked fixture repairs. Current scoped verdict **12/12 reconciled**. Aggregate migration inventory **158/197 originals reconciled,39 pending**, plus subprocess helper/support separately retained. All originalbounds512/256/maxranges and exactcancellation sequences/assertions preserved; no wholegoal/fullgate acceptance claimed.

## Independent adversarial lifecycle checkpoint — 2026-10-05 (next17 originals)

Read complete adversarial_http_lifecycle_migration_test.go and original17 corresponding bodies with independent complete-function comparison. **14/17 reconciled;3 partial cause-specific negatives CP-MCP-009**. Assertions/fixture payloads are preserved, but these three original negative families require specific failure causes to prove the invariant rather than a different rejection. No runtime defect claimed.

| Original name (all current names unchanged) | Disposition / evidence |
|---|---|
| TestStreamableHTTP_CloseCompletesHeldRequestWithTransportClosed | preserved held SSE/received≤1sec; Close settles Await ErrTransportClosed |
| TestStreamableHTTP_PreCancelledRequestNeverAllocatesOrWritesID | preserved canceledcontext/nilprepared/Cancelled/zerotraffic; valid currentdiscover fixture. No new requestID assertion claimed (original also only checks traffic) |
| TestStreamableHTTP_DecoratorCannotOverrideRequestHost | preserved internal-vhost override/current buildRequest/nullrequest/typedInvalidPayload; updated body/routingheaders API, no legacy GETbuilder |
| TestStreamableHTTP_DecoratorCannotSetBodyContract | all6 exact body/GetBody/ContentLength/TransferEncoding/Trailer/Content-Length mutationfixtures and nullrequest/typedInvalidPayload retained; exact minimalJSONbytes preserved |
| TestStreamableHTTP_RequestIsDetachedFromDecoratorPointer | preserved positive decoratorcontrol then mutate Authorization/URL/Body; originalauth/host/JSONbody unchanged |
| TestStreamableHTTP_RejectsPrivateEndpointAndOversizedBody | exactprivate127.0.0.1:1/padding response/cap32 retained; privateStart error, strengthened body ErrorIsReadLimit proves intended cause |
| TestStreamableHTTP_RequestRejects202AndCloseCancelsActivePOST | partial CP-MCP-009 only202branch; exact202 retained but typedInvalidPayload alone could be missingContentType if statuscheck removed. Closebranch preserved serverstarted/Close then strengthened ErrTransportClosed/activePosts0 |
| TestStreamableHTTP_JSONPOSTRequiresCorrelatedTerminalResponse | originalnotification paramsobject and mismatchedID999 fixtures unchanged; typedInvalidPayload/bounded1sec/Close retained, current outgoingmetadata valid |
| TestStreamableHTTP_RejectsInvalidUTF8JSONAndSSE | partial CP-MCP-009: exactboth0xff bytefixtures/contenttypes unchanged, typedInvalidPayload/Close retained; missing UTF8guard can still fail invalidjsonrpc/invalidJSON, need exactUTF8cause |
| TestStreamableHTTP_TerminalSSEEventCancelsHeldOpenPOST | exactcorrelatedrawresult{} SSE/success/JSONEq, servercancel≤1sec/EventuallyactivePosts0/Close retained; raw transport doesn't require discovery-domain tags |
| TestStreamableHTTP_RequiresExactMediaType | partial CP-MCP-009: both original application/json-evil and text/event-stream-bogus/body{} retained; genericInvalidPayload can arise from invalidenvelope after incorrectprefixaccept; need exact unsupportedcontenttypecause |
| TestStreamableHTTP_RejectsMethodChangingRedirect | original302 targetfixture, exactredirectmethodcause/targetcalls0/Close preserved |
| TestStreamableHTTP_SafeClientRejectsRedirectToLoopback | byte-identical https-example→127/private CheckRedirect negative |
| TestTransport_CloseBeforeStartIsTerminal | preserved bothclosedbeforeStart/ErrClosed; HTTP now explicit validPrepare nil/ErrClosed plus validobjectnotification forbiddencause (no HTTPnotificationAPI), stdio validobjectnotification stillErrClosed; no masking from legacy invalidmethod/nullparams |
| TestStdio_ProcessExitReturnsTypedCrash | exact /bin/sh read-line exit7 fixture retained; current prepared-delivered discovery substitutes arbitrary legacycrashmethod/nullparams; typedCrash/Close unchanged |
| TestStructuredContent_PreservesLargeInteger | byte-identical raw9007199254740993 canonicalJSON/JSONEq/exactintegertext |
| TestResourceResult_PreservesSingleContentMIMEAndBinary | byte-identical file:///a JSONtext andfile:///b AAEblob; JSONMIME/JSONEq/binary[0,1]/DeliveryClassBinary |

### CP-MCP-009 — cause assertions needed

Keep ALL original negative payloads. Add exact `202 Accepted is invalid for a JSON-RPC request` in202branch. Add concrete UTF8cause for both JSON andSSE branches; JSON fixture otherwise violates jsonrpc/missingID even after lossydecode, SSEdata is otherwiseinvalidJSON. Add unsupportedcontenttypecause for both near-prefixmedia inputs; body{} is not a valid RPC and would fail later even if media were incorrectlyaccepted. Generic InvalidPayload does not establish these three intended guards.

Other fixture migrations use test-only adversarialDeliveredDiscovery with mandatorymetadata, explicitPrepare/Deliver and no rewritten legacywiremethods/runtimealias. This ensures response/lifecycle checks actually reach their intended boundary. Decorator positivecontrol proves direct buildRequest permits the unchanged minimalJSONbody; body/routingheaders signature adapts API, not weakening assertions.

Independent production GoFiles + adversarial_http_lifecycle_migration_test.go/task34_contract_test.go/transport_http_migration_test.go, selected17 names, `-race -count=3` **PASS, command-line-arguments 1.784s**. Passing execution does NOT close CP-MCP-009. Inventory158previous+14accepted=**172/197 reconciled**,3partial+22pending. No Task35/fullgate acceptance; denominator unchanged.

### CP-MCP-009 follow-up closure

**Closed** after source recheck:202 branch now requires actual202-invalid cause; both near-prefixmedia branches require exact unsupportedcontenttype+fixturevalue. Both original UTF8rawfixtures remain unchanged. Original malformedJSON fixture lacksID and is correctly rejected BEFORE payload dispatch; it now explicitly asserts response-id subject rather than falsely claiming UTF8guard ordering. Added otherwisevalid correlated result differs only x→0xff, with matching ASCII positivecontrol accepted; invalid counterpart requires typedInvalidPayload/exact not-valid-UTF8 cause. OriginalSSE fixture now also requires exactUTF8cause. This preserves old inputs while proving the intended UTF8guard with a nonmasked valid-envelope case. Current lifecycle scope **17/17 reconciled**; initial14/17 above historical.

## Independent adversarial SSE-framing checkpoint — 2026-10-05 (next4 originals)

Read complete current adversarial_sse_framing_migration_test.go and all4 immutable original bodies. **4/4 reconciled**, including explicit removed-resume dispositions. Direct parser tests use actual current provenance and valid pending request; no hasPendingID compatibility shim.

| Original TestStreamableHTTP suffix | Current counterpart / evidence |
|---|---|
| EmptySSEDataPrimesResumeCursor | EmptySSEDataDoesNotCompleteRequest: exact id:primed/dataempty/eventboundary retained; current EOF typedInvalidPayload/exact ended-before-terminal cause, peerpending length1 underlock. Removed resume cursor sideeffect intentionally not retained |
| SSEAcceptsStandardLineEndingsAndLeadingBOM | same: all3 LF/CRLF/CR separators, leadingBOM/idcursor and correlatedresult{} exactoriginalbytes retained; NoError consume/Await+JSONEq{}. Old polling-boundary/cursorstate replaced terminalrequestsuccess; no GET/replay mechanism |
| SSEDiscardsIncompleteEventAtEOF | same: exact id:next/result{} singlefinalnewline without eventboundary; typedInvalidPayload/exactEOFbeforeterminal cause; pendinglength1 underlock. Old previous-resumecursor/pollingboundary intentionally removed, not mistaken for responsecompletion |
| SSELineUsesConfiguredStreamLimit | same: original payloadSize=scannerMax+1024, streamcap2×payload, x-string rawresult; parser/Awaitsuccess and exactrawlengthpayload+2 retained; no stdio scanner-limit erroneously imposed on HTTP SSE |

Independent production GoFiles + lifecycle/framing/task34/http_migration support, `-race -count=3` **PASS, command-line-arguments 5.635s**, includes post-CP-MCP-009 lifecycle cases and all4 framing counterparts (additional support tests not counted asoriginals). Aggregate migration inventory **179/197 reconciled**, **18 pending**, helper separate. No whole-goal/fullgate acceptance; Task35 denominator121 unchanged.

## Independent adversarial client checkpoint — 2026-10-05 (next3 originals)

Read complete adversarial_client_migration_test.go, corresponding3 immutable originals, real pending completion/cancel, current toolterminal delivery and fakeTransport finish paths. **3/3 reconciled**. No lost fixture/assertion or maskednegative found; final yield cancellation is a justified current-contract break, not expectation weakened to green.

| Original TestClient suffix | Current counterpart / evidence |
|---|---|
| NonToolCancellationUsesActiveRequestID | same: original pending GetTools after2wire requests, contextcancel/ErrorIsCanceled/exact1notification preserved; boundedEventually notificationcount1 prevents raced sampling, typedCancelledParams now matches exactactive listrequestID, cleanup added |
| ToolsInvalidationAbortsInFlightSnapshot | same: original stalename/objectschema/ToolsListChangedtrue and invalidationDURING tools/list retained; actual Listen→ACK/effectiveToolsfilter/currentsubscriptionID authorizes event before generation change; result complete/cachefields valid; typedStaleDiscovery preserved |
| YieldAbortSendsCancellationExactlyOnce | TerminalYieldAbortDoesNotSendObsoleteCancellation: same abortdescriptor/objectschema/Args{} and finaltextdone result; callback stop error now preserved alongside ErrStreamAborted. Original cancellation1 afterterminal intentionally replaced cancellation0 because requestalreadycompleted, not an active progressabort |

### Terminal boundary justification

fakeTransport.finish removes pending from peer registry BEFORE pending.complete delivers terminal rawresult. Actual current proxy final result is then validated/built and yielded; cancellation helper claims once but CancelPending cannot cancel removed terminalpending, so no notification is authorized. This agrees with current completion/wire-terminal retirement contract; does not assert rollback or make consumer failure successful. Original stop error remains streamaborted and is additionally ErrorIs(stop). No runtime legacycompatibility restored. Active/preterminal progress cancellation cases in other suites remain separate: this terminalcase does not replace or certify them by inference.

Current self-describing discovery/list/toolresult mandatoryfields make fixtures valid before cancellation/invalidation assertions. Invalidation has actualACKed source, not unsolicited legacy callback; generic stale assertion cannot be achieved through missingmetadata because the event uses matching subscription identity and list result is valid.

Fresh independent production GoFiles + adversarial_client_migration_test.go/client_contract_test.go/test_support_test.go/task34_contract_test.go/http_migration support, selected exact3 names, `-race -count=3` **PASS, command-line-arguments 1.551s**. Migration aggregate **182/197 reconciled,15 pending**, helper separate; no Task35/full-workspace gate acceptance, denominator121 unchanged.

## Independent adversarial DTO checkpoint — 2026-10-05 (next1 original /11 subcases)

Read entire current adversarial_dto_migration_test.go and immutable original TestProtocol_StrictRequiredFieldsAndTaggedUnions. **1/1 reconciled, all11 original malformed semantics preserved**: missing toolcontent; null resourcecontents; missingtext; crossvariant data ontext; invalid promptrole system; resourcewithoutURI; audience system; textnull; image datanull withimagePNG; resourcetextnull/file:///x; progressnull/tokenx.

Only unrelated mandatory current resultType/cachefields added explicitly (ResourcesRead complete/ttl0/private), no automatic fixture normalization or relaxed malformed fields. Every negative is paired with fresh DTO positivecontrol from reflect.New(sametype), changing the targeted field only; positive decoding succeeds. ErrorContains(content/contents/text/data/role/uri/audience/progress respectively) proves the target field, not absent mandatoryenvelope fields. All11 original names and DTO types retained; controlfields stay literal. No production change or broader runtime acceptance inferred.

Migration inventory now **183/197 reconciled,14 pending**, helper separate. Root reports actual full MCP go test ./... still RED on remaininglegacy references; this checkpoint neither excludes that gate nor claims final Task35 completeness.

Independent production GoFiles + adversarial_dto_migration_test.go, exactoriginalname, `-race -count=3` **PASS, command-line-arguments 1.262s** (all11 negative/positive pairs).

## Independent final legacy adversarial migration checkpoint — 2026-10-05 (last14 originals)

Read current entire adversarial_contract_test.go and all corresponding immutable14 original bodies. **Initial11/14 reconciled,3 partial CP-MCP-010 execution blockers**. All14 are accounted for; this is not197/197 acceptance yet. Removed resume/session/server-responder behaviors require explicit current forbidden/inert dispositions, not aliases.

| Original name | Current counterpart / disposition and evidence |
|---|---|
| TestSSERetryAcceptsOnlyASCIIIntegerMilliseconds | SSERetryValuesCannotEnableReplay: all9 exact original strings0/1/120000/600000/+1/1.5/-1/space1/maxuint64 preserved as inert SSE fields; correlated{} success/Close/POST1/nonPOST0. Old parseduration/okflags obsolete without retry scheduler |
| TerminalIncomingResponseErrorCannotSelfDeadlock | ForbiddenIncomingRequestCannotSelfDeadlock: original server-request/roots frame retained; current typedforbidden/no errorreply/requestcount1 and boundedClose2sec replace removed OnRequest→HTTP400reply. Partial: expected peererror cause differs from actual HTTPprovenance cause |
| CancellationFollowsOriginalWireRequest | CancellationClosesOriginalWireRequestWithoutNotification: exact ordered/request retained; originstarted BEFORE cancel, ErrorIsCanceled/originstopped, postClose methods exactly[ordered/request]. Original cancellationPOST is now forbidden; no extratraffic |
| InitializePOSTSSEDisconnectResumesWithPrimingID | DiscoveryDisconnectCannotResumeOrInitialize: original resume-session/initialize-1/emptydata/retry25 priming retained; explicit initializePrepare denied beforetraffic; current request EOFbeforeterminal/emptyresult/Close/POST1/GET0. Old initialize-2 GETrecovery/polldelay obsolete forbidden behavior, not silently positive |
| SessionCannotBeReplaced | SessionAdvertisementsAreInert: original session-a/session-b first/replacement/fresh-late variants remain; three requests require no session/cursor header, statelesscorrelated{}success. Partial: response helper omitted application/json, so successfixture currently fails text/plain |
| DecoratorCannotChangeTargetAndEmptyEventIDResetsResume | DecoratorTargetGuardAndInertEventIDs: original attacker.invalid/whitespacecursor/emptyID exactframes retained; exactdecoratorguard/nullrequest; both streamEOFbeforeterminal/pending1. Removed old/previous cursorstate has no client-owned representation |
| ParallelPOSTStreamsResumeIndependentlyAfterRetry | ParallelDisconnectedPOSTsNeverReplay: two originrequests bothstarted before release, distinct pendingIDs and originalstream-ID/retry75 frames; both boundedAwait typedEOFbeforeterminal/POST2/GET0. Old resume-time/doneGET fixtures superseded by explicit no replay |
| Resume405CompletesPendingRequest | DisconnectFailsBeforeResume405: exact stranded/retry1/405 fallback preserved; typedEOFbeforeterminal/POST1/GET0 replaces unsupported resume405 outcome |
| CancelledPendingStopsActiveResumeGET | CancelledPendingStopsOriginPOSTWithoutResumeGET: original cancellable/retry1 frame retained on current originPOST; Await-only cancel afterstart returnsCanceled/originstopped≤1sec/activePosts empty/GET0. Removed resumeWG not aliased |
| PostResumeCursorDoesNotLeakIntoActivate | ConnectNeverActivatesResumeCursor: original initialize-priming/emptydata/retry1 retained; boundedConnect nilclient/exactEOFbeforeterminal/POST1/GET0. Removed initialize-terminal/recovery/initialized/activation phases explicitly do not exist |
| RejectsForgedSessionIDs | ForgedSessionHeadersCannotBeDecorated: all4 exact space/control0x1f/delete0x7f/invalid0xff values retained; actualrequest decorator rejects reserved sessionheader with exactcause/nullrequest beforeI/O. No obsolete session validation/ownership installed |
| OversizedGETSSEFailsClosedWithoutRetry | OversizedPOSTSSEFailsClosedWithoutRetry: originalx256/cap32 retained on supportedoriginPOST; peerclosed/afterrequestErrClosed/POST1/GET0 assertions retained. Partial: new ErrorIsReadLimit currently observes scanner-token-too-long wrapped InvalidPayload instead; cannot claim testgreen or change originalbytes to bypass |
| TerminalFailureCancelsConcurrentLifecycle | TerminalFailureCancelsConcurrentPOSTLifecycle: original active/request+terminal/request/fatal400 retained, removed activeGET replaced second activePOST; both started/bothstopped, exactHTTP400 propagated to bothpending, afterPrepare ErrClosed/GET0; no global unsolicitedGET phase |
| Enforces202ResponseShape | NotificationsForbiddenRegardlessOfResponseShape: original202body{} and200empty exactfixtures retained; validobject notifications/test forbiddenexactcause/traffic0; actualcurrent request thenproves202invalid orContentType rejection/Close/traffic1. Old response-shaped notificationPOST removed |

### CP-MCP-010 — current evidence

Independent actual package `go test -race -count=3 ./...` with exact14-name selection **FAIL** (package compiles; examples have no tests). Three failures repeated: serverrequestcause mismatch (actual `server-initiated request is forbidden`); sessionadvertisement successresponse Content-Type text/plain instead of application/json; x256/cap32 scanner `token too long` instead of asserted textprocessor.ErrReadLimitExceeded. All originalframes retained; firsttwo are exact fixture/assertion repairs. Last needs correct current bounded-scanner/read-budget disposition, without dropping originalbound/payload or reducing test to generic error. No implementation fix performed by auditor.

Current migration aggregate **194/197 reconciled,3 partial**, helper separate. Initial last14 candidate is NOT final197/197 and NOT Task35 acceptance. Authoritative wholepackage/allmodule race/lint plus both same-candidate finalaudits still needed; scope filter is diagnostic only, not an excludedgate.

### CP-MCP-010 follow-up closure / complete migration inventory

**Closed** on independent current source and fresh execution: incoming HTTP frame asserts actual singular `server-initiated request is forbidden`; all sessionadvertisement responses explicitly set application/json. Original oversized x256/cap32 fixture and strengthened ErrorIsReadLimit unchanged. Current SSE scanner allows one byte of tokenheadroom over unchanged boundedreader budget (MaxInt overflow guard), so readerbudget exhaustion owns typedcause rather than scanner token ceiling. The audit did not modify runtime or accept a weakened fixture.

New additional SSELineBudgetOwnsScannerFailure checks validcomments of31/32/256bytes under32cap:31 gives EOFbeforeterminal NOTReadLimit; exactcap32 and256 give typedReadLimit; pre-cancel wins as Canceled. These new cases support the repair, not original denominatorcount. Headroom does not authorize additional readablebytes.

Independent actualpackage last14 selection `-race -count=3 ./...` **PASS, github.com/skosovsky/toolsy/mcp 1.556s**, examplescompile/no tests. Current last14 scope **14/14 reconciled**. Migration inventory **197/197 original acceptance checks reconciled (100%)**, plus subprocess helper/support retained separately. Original1..197 ledger preserved; obsolete legacy behavior explicitly replaced negative/inert current-contract cases. Historical scoped RED states above remain visible, not replaced by green-only narrative.

This closes ONLY migration-inventory preservation. It is not121-row Task35 completeness, not a full-package/allmodule runtime/lint gate, and not both same-candidate finalaudits. Fullpackage compiles in this diagnostic, but nonselected tests do not execute; acceptance remains pending final checks.

Independent additional budget-regression selection `-race -count=3 ./...` **PASS, mcp 1.264s**; additionalcase execution does not inflate197 denominator.
