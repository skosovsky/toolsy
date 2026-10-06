# D24 independent acceptance A

Decision: ACCEPTED. Total: 100%. No unresolved detected implementation defects.
Baseline: d9170c1bad1e5268b834924d2f031593f6d9b38e. Read-only repository review; no repository edits or commits. Peer acceptance report was not read.

## Criteria

1. Adapter/custom protocol naming and scope: 20/20 (100%). Package/API comments, README and migration describe the pinned Agent Protocol base-envelope projection and custom toolsy-step-stream-v1 SSE/cancel behavior. Normative POST-step lifecycle, A2A and remote scheduling/retries are explicitly excluded; no added scheduler code.
2. Optional host-only diagnostics: 20/20 (100%). Immutable client option holds an optional function reference. The single deferred post-create cleanup reports task ID, ctx.Err, context.Cause, credentials/request stage, acknowledgement and the same cleanup error value; no result/chunk/log emission or header fields. Credential failure blocks dispatch, request failure remains observable, success is only HTTP acknowledgement. Disabled observer retains prior best-effort behavior. Direct CancelTask and background creation do not call the observer by inspection.
3. Ownership/error semantics: 20/20 (100%). Only ctx.Err on the original parent triggers deferred cleanup, using WithoutCancel(parent) plus a fresh five-second deadline. StreamPolicy has its own child context; callback abort closes/stops the iterator. Active-parent stream deadline/callback error causes no remote cancel. Callback+parent interruption still preserves the callback cause. No creation retries, observer goroutine, background job or borrowed host/client ownership transfer. Cleanup credential/request/observer share one cooperative budget and retained parent values.
4. AAA tests and bounded cooperative context: 20/20 (100%). Added tests cover acknowledgement, HTTP and credentials failure, custom parent cause/deadline, absence, real AsTool stream/callback distinctions, error preservation and HTTP timeout. Independent overlay probes additionally cover 12 concurrent calls to one client, callback-stop plus parent custom cancellation, parent context values, one creation/cancel/observation each, and credential cooperation with the actual five-second deadline (observer sees expired cleanup context; exact credential cause; no HTTP request).
5. Contract agreement/verification boundaries: 20/20 (100%). README/API/migration consistently disclose synchronous cooperative callbacks, concurrent host state ownership, sensitive/untrusted causes and lack of remote-stopped proof. Independent race and pinned lint pass. This score is this reviewer's acceptance; the separate second-reviewer gate must be fulfilled by the parent before final row acceptance.

## Checks

- GOWORK=off GOCACHE=/tmp/toolsy-review-gocache go test -race -count=3 ./... in agents: PASS, 6.491s.
- GOWORK=off GOCACHE=/tmp/toolsy-review-gocache GOLANGCI_LINT_CACHE=/tmp/toolsy-task41/d24-a-lint /opt/homebrew/bin/golangci-lint run --allow-parallel-runners ./... in agents: 0 issues. Actual binary reports version 2.14.0 built with go1.27.1.
- Independent read-only Go overlay: go test -C agents -overlay=/tmp/toolsy-task41/d24-a-overlay.json -race -run '^TestAcceptanceA' -count=1 -timeout=30s: PASS, 6.983s. Probe source: /tmp/toolsy-task41/d24-a-probe_test.go.
- git diff --check: clean. HEAD matches provided baseline. All three source SHA256 entries in retained baseline-sources.json match git show of baseline.
- Reviewed all tracked changes and untracked cancellation implementation/tests plus the five D24 evidence files. Existing affected delegate, client, SSE and outcome behavior examined. Retained full/diagnostic race and lint logs agree with verification narrative.

## Findings and limits

No blocking findings. The first independent probe used error identity equality for callback errors and failed: core deliberately wraps callbacks with ErrStreamAborted (toolsy.go and errors.go). The corrected assertion tests errors.Is for both original callback error and stream-abort marker; all 12 concurrent calls passed. This was a probe assumption failure, not a D24 code defect. The final README and migration clarification was reread: they explicitly describe original callback cause preservation through the error chain and ErrStreamAborted/errors.Is/errors.As, matching the existing Tool.Execute contract. Documentation-only clarification requires no repeated runtime test; git diff --check remained clean.

The observer cannot force a noncooperative credential callback/observer to stop or recover its panic; this is accurately disclosed, not a hard execution wall-clock guarantee. Failure causes may include sensitive data from opaque host callbacks/transport errors, requiring host sanitization as documented. Local HTTP acknowledgement does not certify stopped remote effects or live protocol interoperability. No full-workspace go-doc command was run; go.work.sum was not modified. No peer result was used to reach this decision.
