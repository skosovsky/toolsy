# R13 / D20 / D21 / D22 — independent acceptance A

Final verdict: **accepted, 100%** (five explicit criteria ×20%). No unresolved detected errors. This supersedes the initial rejection after fixes and independent repeats; 100% measures the criteria, not absence of every possible defect.

Five explicit criteria, 20% each:
1. Bounded retirement and isolation — fulfilled. Default 64 active/1024 tracked/one-minute TTL, reservation before Listen delivery, no eviction, deterministic terminal retire, TTL expiry restores strict unknown-ID handling. Late canceled-A ACK/notification leaves B live and generations untouched. Malformed metadata/retired resource URI retains strict handling.
2. Transport budgets — fulfilled. 1MiB default per-frame independent from 16MiB outgoing queue and 64 in-flight requests; lifetime zero unlimited. Raw framing bytes counted before dispatch, typed frame and resource causes inspectable, terminal paths release accounting. Negative/nil options reject before dispatch.
3. Bounded transactional discovery — fulfilled. Aggregate raw bytes and raw item count checked before descriptors/schema compilation; pages/cursor limits and duplicates remain bounded. Input/output schema compilation precedes authority publication. Failed/duplicate/precommit-canceled discovery preserves previous authority; invalidation revokes authority by generation.
4. Unified typed full discovery/page asymmetry and host contract — fulfilled. ListToolsPage never authoritative; DiscoverTools publishes complete validated snapshot; Discover uses shared collector and proxy building. Migration and minimal transport/facets documented, no unrestricted authority setter. Atomic header replacement is explicitly bounded/non-reentrant and rollback-on-error host-owned; descriptor mapper can reenter page inspection. Publication has documented precommit cancellation boundary.
5. Regression/check/documentation gate — fulfilled. AAA regression source inspected; old cancellation probe fails baseline and succeeds latest. All independent probes, full MCP race and lint pass. Initial defects corrected and independently repeated. The other reviewer's verdict is deliberately not consulted; coordinator owns two-reviewer gate.

Evidence:
- verified-full-race.log: go test -race -count=1 ./mcp/... PASS19.805s.
- verified-lint.log: golangci-lint2.14.0 run --allow-serial-runners, 0 issues.
- accepted-probe.log: independent overlay race count5 PASS3.359s; public real-process CRLF and Notify regressions, expiry/malformed retirement, LF/CR/CRLF SSE inclusive raw boundaries, pending terminal accounting, duplicate snapshot retention, invalid options, page nonauthority/TTL, descriptor mutations and mapper reentry.
- final-extra-probe.log: independent precommit canceled publication and errors.Is/As queue/in-flight resource probes count5 PASS1.809s.
- probe_test.go / overlay.json: independently authored tests, outside repository. No production changes made by this reviewer.
- git diff --check: clean after final source/test edits and final README raw LF/CRLF wording synchronization.

Initial findings superseded: CRLF raw overcap frame dispatch, missing typed cause for oversized Notify, stale string assertions and lint of new test fixtures all corrected. The initial probe incorrectly expected protocol success for comment-only SSE EOF; corrected expectation requires absence of byte-limit cause at inclusive bound while allowing legitimate missing-terminal protocol failure.

Limits: macOS /bin/sh plus custom transport/overlay fixtures, no live production server, no OOM stress reproduction or universal custom transport safety. In-flight notification/cancellation can overlap; TTL expires correlation by design. User callbacks own cooperation and the replacement facet's atomicity/non-reentry. Benchmarks reviewed for allocation cost, not statistical speed claims. No publish/push.
