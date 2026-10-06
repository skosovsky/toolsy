# R13 independent acceptance A — initial review (superseded only after fixes)

Verdict: not accepted. Completeness: 40% (criteria 1 and 3 fulfilled; criteria 2, 4 and 5 blocked by detected defects and documentation gaps).

Detected errors:
- CRLF stdio raw frame exceeds MaxFrameBytes by one but dispatches because ScanLines strips CR. Public process probe fails (probe-repeat.log).
- Oversized stdio Notify returns a plain fmt error rather than the typed read-limit cause used by requests/incoming frame rejection. Public Notify probe fails (outgoing-limit.log).
- README scanner/stream-budget text describes removed lifetime-as-frame semantics; ToolAnnotations comment references removed ListTools. Exact supported non-reentrant authority facet contract is being clarified.

Independent checks: full MCP race PASS45.132s; lint 0 issues; retirement expiry/malformed IDs, pending accounting terminal paths, SSE raw exact cap across LF/CR/CRLF, discovery duplicate transactional preservation, invalid options, retired resource malformed URI, page nonauthority/cache hints, descriptor mutation and mapper reentry inspected/probed. Probes outside repository; no production edits.

Limits: macOS real /bin/sh stdio plus package overlay/custom transport fixtures. No live remote server, production traffic, universal transport callback safety, real-memory OOM experiment or performance guarantee. Expected errors in first SSE fixture were corrected to require absence of the read-limit sentinel (EOF without terminal response legitimately fails protocol); the corrected boundary fixture passes.
