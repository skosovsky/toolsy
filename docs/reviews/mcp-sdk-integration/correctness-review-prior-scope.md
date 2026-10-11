# Independent correctness review

Reviewer: correctness_review. Baseline HEAD `4f7eae6cc2393da259cf30a5535bef12c93d6e2c`.
Initial reviewed snapshot `f90dbcd9c83297fafff0207c11673062182f64976e2ec01d44591abd1d9deace`.
Prior-scope verdict **APPROVE** on snapshot `27364cece6b8598b1cf6a3f5b11bbd507969fe3c8db2a3565432917e3f32bcce`. Initial verdict was REQUEST_CHANGES; all four confirmed defects are closed by independent recheck.

The reviewer independently read all four production adapter files, the updated task, SDK ownership contract, adapter and SDK-only tests, core proxy/prepared execution/view/result cloning paths. No other reviewer report was consulted. Accepted SDK limitations are distinguished from adapter defects.

## Confirmed findings

1. **P2 — closed unbound SDK sessions retained forever** (`mcp/import_hooks.go:153-158`, `:202-208` initial snapshot). observe inserts all handshake sessions into seen; cleanup was created only by Bind. A reused client connecting and closing five real in-memory SDK sessions without Bind leaves len(seen)==5. Their session/cache/connection graphs stay reachable. Fix: one connection completion observer for every observed session, including failed/unbound handshake; delete seen/mapping without closing borrowed session. Regression required.
2. **P1 — host continuation integers silently altered before SDK dispatch** (`mcp/import.go:58`, cloneSDK). WithRound accepts an sdk.ElicitResult.Content map with int64(9007199254740993), then stores float64(9007199254740992). Direct marshal of the host SDK DTO preserves the exact number; adapter ownership copying causes this extra loss before transport. Inbound SDK rounding is accepted; this detectable outbound mutation is separate. Preserve concrete host values or explicitly reject unsupported input before decoding, with numeric/nested/RawMessage/ownership regression. Per-attempt tool/resource copies must preserve the same constraint.

3. **P1 — resource tool dispatches a revoked generation after prepared execution** (`mcp/import.go`, ResourceTool handler before SDK ReadResource). The outer importTool checks before the core prepared boundary, but ResourceTool lacked the inner pre-dispatch guard used by ordinary proxy tools. An execution profile invalidating the resource generation before invoking its handler produces one actual remote read, then rejects the stale result. The expired generation must prevent that dispatch, not only delivery. Fix: check captured generation after prepared policy/profile work and immediately before resource SDK dispatch; deterministic prepared-boundary regression.

4. **P2 — canceled discovery commits a new generation** (`mcp/import.go`, Discover publication boundary). A host Classify callback cancels the discovery context and returns valid options; Discover returns success (generation1/tools1) and commits its catalog despite ctx.Err()==context.Canceled. Cancellation must abort the transaction before publication/revocation, including SDK cache paths. Fix: cancellation checks at entry and publication, regression proving the previously valid generation remains usable after canceled rediscovery.

## Independent executable checks

- Existing adapter suite independently repeated under race detector 15 times: PASS (24.011s), real SDK HTTP/in-memory/subprocess STDIO included.
- SDK-only characterization independently repeated under race detector 5 times: PASS (1.788s).
- Adversarial probes outside production at `/private/tmp/toolsy-correctness-mcp/review_probe_test.go`: all four findings reproduced under race detector before their fixes.
- Probe session cleanup result: `closed unbound sessions permanently retained: 5`.
- Probe continuation result: `host exact=9007199254740993 adapter-owned=9.007199254740992e+15 (float64)`.

## Final candidate recheck

Intended immutable snapshot independently recomputed: `27364cece6b8598b1cf6a3f5b11bbd507969fe3c8db2a3565432917e3f32bcce` (audit reports/logs excluded by snapshot.py).
All four confirmed findings are **RESOLVED**:

| Finding | Fix independently inspected | Regression / independent recheck |
|---|---|---|
| F1 session retention | import_hooks.go:153 observes each connection once and installs completion cleanup before Bind | TestSDKUnboundSessionCompletionReleasesHookOwnership; old independent five-close probe now PASS |
| F2 outbound numeric mutation | import.go:68 cloneRound rejects changes in serialized host values, reused for tool/resource attempts; nil/empty continuation fields use omitempty | TestSDKHostRoundRejectsDetectableRepresentationLossBeforeDispatch; old exact-int64 probe now PASS by explicit rejection; manual MRTR PASS |
| F3 revoked resource dispatch | import.go:421 rechecks after prepared execution immediately before ReadResource; import_progress.go:147 likewise gates producer CallTool | TestSDKResourceRevokedDuringPreparationNeverDispatches; independent profile-driven revocation probe now PASS with zero dispatch |
| F4 canceled catalog commit | import.go:180 entry and locked publication cancellation checks; digest construction before generation commit | TestSDKCancelledDiscoveryDoesNotPublishOrRevokeExistingGeneration; independent cancellation-in-classifier probe now PASS |

Control callback result/error validation was also inspected; fresh outgoing InputRequests maps are recognized without relying on the SDK private resultType. Approval/manual MRTR regressions pass. Initial subscription characterization wrongly assumed propagated listener error; independent repeat10 caught that assumption. Corrected actual HTTP test deterministically documents suppressed failure and marker reset via Unsubscribe, and repeat10 passes. This is an accepted SDK limitation, not an adapter fix.

Final code rechecks: copied exact frozen adapter sources plus all four independent probes and affected approval/generation/numeric contracts under `-race -count=10`: PASS (2.577s). Actual optional module suite under `-race -count=3`: PASS (5.997s). Current complete SDK-only suite under `-race -count=10`: PASS (2.047s). Git diff --check: PASS. Final logs independently inspected after completion: make test (27 modules, race/count1), make lint, exact-candidate temporary tracked-source make test-e2e and all-module go mod verify PASS. Applicable integration logs also PASS; import/export examples return {"n":7}. Root has no SDK dependency and root go.mod/go.sum are unchanged. Final snapshot recomputed again after these checks and still matches the hash above. No unresolved confirmed defects remain.

## Scope and limits

Reviewed mandatory current policy/approval binding/scope/replay/no retry boundaries, local progress queue cancellation/retirement, owned snapshots and SDK cache handling. Trusted host Resolve/Session/Project/Classify functions remain host ports and must obey documented contracts. SDK v1.8.0 inbound numeric/null/unknown-field/strict-wire loss, missing pre-Await hook, subscription lifecycle handle, noncancellable Wait and child-only CommandTransport remain documented limitations, not claimed fixes. No live external services, OS descendant supervision or unpublished SDK commits were tested. Finding absence is not proof of complete correctness.

## Later task-snapshot recheck — expanded scope

Full task snapshot `8e4f650750817eb37e6038e6273149e0c135f8af5e27a92bd2bb9d0de2e33c49` independently recomputed. The current task includes a required SDK package/auth addendum at lines228–326, explicitly saying it expands mandatory scope, with PKG-01–08 unchecked. It requires SDK OAuthHandler client and bearer/ProtectedResourceMetadata server examples plus synthetic challenge/discovery/refresh/error/cancellation checks. Current examples are only in-memory import/export; searching adapter/examples tests finds no auth/oauthex usage (jsonrpc currently used for SDK ID characterization). These additions were not included in this review's earlier inspected contract. Therefore prior APPROVE for the narrower adapter/code review cannot serve as approval of the expanded auth/identity composition scope. Latest administrative/full-task approval is withheld pending scope reconciliation and implementation/review of these requirements. This is a scope change, not evidence that the four repaired adapter defects reappeared.

The user explicitly confirmed the goal expansion (PKG-01–08/auth composition). Administrative confirmation is stopped; expanded correctness re-review is pending a new frozen candidate and synthetic OAuth/auth evidence.
