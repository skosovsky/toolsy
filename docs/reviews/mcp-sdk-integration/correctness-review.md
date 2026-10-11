# Independent correctness review — expanded SDK scope

Reviewer: correctness_review. Baseline HEAD `4f7eae6cc2393da259cf30a5535bef12c93d6e2c`.
Reviewed expanded candidate: `a2080d2574f6888e473c138cdeda1ce1bec750d49e204d9fb697bff612898064`, independently recomputed with snapshot.py. Audit outputs are excluded.

Final expanded verdict **APPROVE** on full snapshot `b4defda488a7158b309e7c8fd4f6677aa1119af728628a18ca9a0c08648cba69` (code/check candidate `a2080d2574f6888e473c138cdeda1ce1bec750d49e204d9fb697bff612898064`); no unresolved confirmed code defects remain after independent recheck. Previous APPROVE covered only the prior scope and is not substituted for this expanded review.

## Independent scope

Read the updated task including PKG-01–08, auth contract, all production adapter files, new synthetic OAuth/bearer fixtures, authenticated approval/replay fixture, public consumer graph, runnable auth example, SDK-only auth/wrapped-error characterization and relevant pinned SDK source. No completeness reviewer report was consulted. No implementation work was delegated to or performed by this reviewer; probes live outside production.

The expanded review checked stock AuthorizationCodeHandler challenge/discovery/PKCE/exchange/refresh, failures and cancellation; SDK RequireBearerToken/ProtectedResourceMetadataHandler, verified TokenInfo mapping, principal disposal/rebinding, scope isolation, current core policy/approval/replay, error code/data/cause and safe formatting. Host HTTPClient/OAuthHandler/token-source ownership remains unchanged by adapter composition. Public SDK auth/oauthex/jsonrpc types are used in host fixtures/examples; root remains independent.

## Findings and disposition

The four prior confirmed adapter defects remain **RESOLVED**; their full reproductions/fix evidence are in [prior-scope history](correctness-review-prior-scope.md): unbound session retention, outbound host-round numeric mutation, revoked resource dispatch after preparation, canceled discovery publication. All old independent probes pass against the expanded production candidate.

**Expanded observation E1 — outbound wrapped JSON-RPC Data loss.** Independently exporting a local public jsonrpc.Error preserves code and safe wrapper message but returns null/absent Data. Pinned SDK internal/jsonrpc2/messages.go toWireError keeps Data only for a top-level public error, not an errors.As-wrapped error. This is reproduced SDK-only through public EncodeMessage/DecodeMessage. It was not accepted as silent adapter loss: the implementation now offers explicit trusted ExportConfig.ErrorMapper to return a fresh public SDK error containing approved safe message/code/data. Receiving middleware covers tools/list and tools/call, including resolver errors. Raw private causes/data stay host-local by default. Mapper output copies Data, rejects malformed JSON and bounds combined message/data bytes. Nil mapping preserves safe default behavior; it cannot return successful execution or authorize redispatch. Actual OAuth/bearer HTTP tests and independent projection/ownership/budget probes pass. Observation **CLOSED**, dependency limitation documented rather than falsely claimed fixed.

**Expanded observation E2 — typed OAuth cause collapse.** SDK connection closure can retain ErrConnectionClosed while dropping typed callback cancellation. SDK-only characterization now waits for public session completion before examining the subsequent call, avoiding a teardown race. Context/callback cancellation remains independently observable by the host. This is documented accepted upstream behavior, not an adapter cause loss or hidden parser workaround. Observation **CLOSED**.

## Independent adversarial checks

- New auth/error/approval suite independently repeated `-race -count=15`: PASS (9.897s).
- Valid actual tools/call JSON with forged subject/scope/approval plus missing/invalid/expired/insufficient bearer: HTTP401/403 and zero business effects. Independent probe repeat10 PASS.
- Stock OAuth callback mismatched state: rejected before token exchange and business dispatch. Independent probe repeat10 PASS (combined bearer/state run2.327s).
- TokenSource failure after successful connection/catalog discovery: imported execution fails safely, zero effects, secret-bearing token error absent from formatted adapter diagnostics. Independent probe repeat10 PASS (2.092s).
- Explicit public error projection over authenticated HTTP preserves approved code/data, hides private message; alias/malformed/oversize error data fail safely. Independent probes plus durable regression repeat10 PASS (4.508s).
- Complete exact expanded adapter source copy plus all old/new independent probes and current tests `-race -count=3`: PASS (11.328s).
- Current SDK-only auth closure and wrapped-error data characterization `-race -count=20`: PASS (1.857s).
- Runnable auth example independently prints `{"n":7}`. Git diff --check PASS. Candidate hash recomputed after these checks and remains a2080d…8064.

## Remaining limitations

Published SDK v1.8.0 remains pinned without fork/private hooks/parser/transport workaround. Its numeric/null/raw/unknown-field/cache/subscription/pre-Await/Wait/process-tree limitations remain documented. Wrapped-error Data and typed OAuth cancellation loss are now explicitly characterized; host-approved error projection preserves necessary outbound fields without publishing raw causes. Stock OAuth may retry HTTP after401/403; bearer verification must precede effects, and host must not emit authorization challenges after effects. Adapter adds no business retry. OAuth access scopes/consent do not replace exact-operation grants.

Trusted Resolve/Session/Classify/Project/ErrorMapper/verifier/token-source ports must obey host contracts. Tests use local synthetic tokens/servers, not production JWT verification, real IdP/accounts, external effects or OS descendant supervision. Absence of findings is not proof of universal correctness. Final evidence inspected after completion: make test27 modules, fresh make lint27 (0 issues), integration27, complete-source temporary tracked-candidate make test-e2e27, go mod verify27, GOWORK=off consumer graph, root dependency isolation, all three examples and diff check PASS. Independent hash recompute and own per-file manifest comparison show no candidate changes after review. Task completion marking, if subsequently applied, requires a separate administrative hash recheck.

## Final administrative snapshot confirmation

Final full snapshot independently recomputed: `b4defda488a7158b309e7c8fd4f6677aa1119af728628a18ca9a0c08648cba69`. Compared every candidate file against the reviewer's own captured a208 manifest: only `.cursor/tasks/mcp-official-sdk-integration.md` and `docs/mcp-sdk-integration-design.md` changed. Compared the task against the reviewer's saved pre-administration content: changes are COMPLETE status, P8/final-review checkboxes and the expanded acceptance summary; no requirements, goal, limits or contracts changed. Compared design against the temporary frozen candidate copy, whose hash matched the own approved manifest: exactly one pending-acceptance sentence became an independent-review link. No completeness report was opened. Implementation/tests/examples/dependencies and detailed requirements are byte-identical to the independently reviewed/tested expanded code candidate. Git diff --check PASS. Final expanded correctness **APPROVE** is confirmed for b4defda…ba69; no substantive approval was inferred from a status edit.
