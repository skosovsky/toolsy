# D24 optional interrupted-task cancellation diagnostics

Baseline d9170c1, source SHA256 manifest retained. Existing parent-only best-effort
cancel semantics retained; optional synchronous host observer adds visibility without
changing the primary tool outcome. Stage/parent interrupt/custom cause/request cause
and acknowledgement are inspectable fields; no tool chunks/logging/credentials fields.
Observer absence/nil intentionally disables diagnostics. Direct CancelTask returns
errors directly and does not invoke observer; background acknowledgement takes no
observation/cancellation ownership.

AAA tests cover cancel acknowledgement, HTTP and credential failures, parent
cancel/deadline/custom cause, observer absence, real AsTool parent interruption versus
stream-only timeout/consumer callback stop, original errors and no implicit cleanup
for active parents. A blocking local cancel endpoint with positive HTTP timeout
preserves its deadline cause separately from the parent cancellation diagnostic.

Parent full affected agents module race count3 PASS5.638s, pinned lint0. New HTTP
fixture initially stalled in httptest.Server.Close because POST body was not consumed;
given stack evidence, only the identified test binary was stopped for diagnosis. An
unchanged subsequent fixture run ended via60s test timeout. The actual corrected
fixture drains the small request body and has explicit cleanup release; final run
passes. Those are fixture failures, not evidence of adapter cancellation failure.

Contracts name the pinned base envelopes plus custom SSE/cancel profile explicitly.
Credentials and observer are host callbacks requiring cooperative deadline handling,
thread safety and no panic; no forced preemption or remote scheduler. HTTP ack is
not remote stopped proof, and diagnostics are not replay permission. Local fixtures
alone do not certify live remote interoperability. Both independent reviewers accepted all five criteria20/20 (100%), no unresolved detected defects. See acceptance-a.md/acceptance-b.md and retained overlay probe fixtures. A tested12 concurrent calls and full cooperative credential budget; B independently checked combined parent/callback interruption, preserved values and exact expired cleanup context. Both full agents race count3 and pinnedlint0 passed. Documentation clarification preserves callback causes through errors.Is/errors.As with the existing core ErrStreamAborted wrapper, not top-level error identity.
