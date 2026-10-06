# R07 / D18 / D19 — owned safe HTTP pools and explicit settings

Scope: row 07; no production publication or push.

## Contract and implementation

Agents create one owned pool at construction; OpenAPI/GraphQL discovery and all
operations share one pool; HTTP/web/document tool sets share one owned pool. MCP
retains one configured pool and closes owned idle connections after active posts
finish. Safe transports retain at most 32 idle connections overall, two per host,
with 90-second expiry, finite TLS handshake and total dial deadlines. These are
idle-resource bounds, not request/concurrency limits.

ClientSettings applies Timeout and a cloned TLSConfig to safe transports. Ignored
HTTPClient/Do/transport ports and MergeHTTPClient are removed; no proxy/custom
Do/custom dial port is accepted. Agent NewClient returns (*Client,error); MCP
invalid settings fail Start before dispatch. Certificate/root/callback state
referenced by TLSConfig must remain immutable. Redirect callbacks and web deny
options snapshot host slices; other D32 toolkit option work remains in row 34.

Cleanup-returning factories expose owned idle closers. Agent cleanup leaves active
calls unaffected and is not terminal Close; stop new calls at disposal. Ordinary
factories keep bounded idle expiry. One-shot web ScrapePage disposes its pool on
return. No caller HTTP client is accepted or closed.

All resolved addresses pass IP admission before any attempt. One DNS answer is
used across sequential pinned attempts, with one total deadline including lookup
and a share of remaining time per attempt. Cancellation stops further attempts.
The stream reader keeps stop-after-budget semantics: exact cap without EOF on
that Read yields a limit error on the next nonempty Read, without speculative
EOF probing. Empty reads consume nothing; cancellation takes precedence.

## Regression and checks

- [Previous-commit baseline](r07/baseline.log), with [probe source](r07/baseline-probe.go.txt):
  behavioral FAIL, 20 accepted connections for 20 Agent calls instead of one.
- Public AAA fixtures for all seven consumers prove one connection across 20
  calls and explicit owned idle closure. OpenAPI/GraphQL include discovery reuse.
- TLS roots/settings snapshot, negative timeout, mixed public/private DNS answer,
  no re-resolution, fallback/total deadline/cancellation and exact-cap/empty-read
  fixtures pass. Existing redirect/host/adversarial module regressions remain.
- Parent seven-module race and lint pass; [MCP race](r07/mcp-race.log) includes
  its full lifecycle suite. OpenAPI helper extraction was followed by a fresh
  [OpenAPI race](r07/contracts-openapi-race.log). All module logs are retained here.
- Root test helper lint and root lint pass. git diff --check passes.

## Independent final acceptance

Reviewers r07_acceptance_a and r07_acceptance_b: **100%, accepted**, five criteria
20/20 each, no unresolved detected defects. Both independently ran seven module
race/lint suites. B additionally repeats the targeted pool/settings/dial/cap tests
five times with race. Independent TLS probes require real client certificates and
custom roots, confirm no SSRF bypass without private opt-in, and verify positive
HTTP timeout/caller cancellation. A also proves cleanup during an active request
leaves that request successful and permits a later request.

A found a missing-fields Agent lint gate; B found the current root README's removed
WithHTTPClient instruction and misplaced nolint after automatic line wrapping.
All were fixed and independently rechecked. A's initial cleanup-hanging test
handler was bounded and repeated; it was a fixture issue, not a product defect.

Evidence: [A final acceptance](r07/review-a-acceptance.md),
[A final probes](r07/review-a-probe-final.log), [A probe source](r07/review-a-probe.go.txt),
[B probes](r07/review-b-probe.log), [B probe source](r07/review-b-probe.go.txt),
[A final Agent lint](r07/review-a-lint-agents-final.log),
[B final Agent lint](r07/review-b-agents-lint-final.log).

## Limits

Tests use local macOS HTTP/TLS servers. Linux live verification is not claimed.
TLS referenced state remains a host immutability contract. Cleanup releases idle
resources, not active requests or a terminal client state. Zero HTTP timeout
requires caller deadlines; bounded idle expiry does not bound response duration.
Underlying readers must unblock for cancellation. Percentages measure explicit
criteria, not universal bug freedom.
