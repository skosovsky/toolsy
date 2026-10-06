# R16 independent acceptance A

Verdict: ACCEPTED, 100%. Reviewed final uncommitted R16 implementation against HEAD e3fc1f9, original TLS-R16 and row16 contract; tracked changes plus untracked response.go/response_encoding_test.go. No other reviewer verdict consulted. No repository changes made by reviewer.

## Five criteria

1. UTF-8-only lossless representation: 20/20. Actual bytes checked with utf8.Valid before string conversion/encoding. Valid Unicode, U+FFFD, empty octetstream, NUL/CRLF and mismatched charset headers preserve decoded bytes. Invalid byte, Latin1, truncated UTF8 and UTF16 reject. No MIME/charset guessing; library byte readers unchanged.
2. Result-phase errors/effects: 20/20. Encoding failure exposes ErrInvalidUTF8Response, ResponseEncodingError method/status and ResultContractError kind. Both GET/POST produce nonretryable CodeInternal and no result or self-correction chunk. POST read/wire failures preserve original limit causes within noncorrectable result phase. Endpoint effect counters remain exactly one, no rollback claims.
3. Bounds/context: 20/20. Inclusive source and final escaped status/body JSON budgets remain independent; exact encoded wire length and one-byte smaller boundary covered. Found initial responseFailure cancellation gap: canceled POST wire validation returned client-correctable validation. Deterministic overlay FAILED with `cancellation lost: VALIDATION_FAILED`. Parent fixed by checking ctx.Err before nil/method handling and added permanent regression. Re-read final code and reran overlay: PASS, no unresolved issue.
4. Contracts/docs: 20/20. README/migration accurately explain UTF8-only, binary library alternative, preserved status/body shape, byte bounds and outer-envelope exclusion, explicit original causes, reconciliation/host-owned idempotency and cancellation without rollback. ClientSettings.Timeout documentation corrected.
5. Evidence/verification: 20/20. AAA permanent tests and public baseline/current proof reviewed. Independent initial whole HTTP race PASS 2.129s, lint zero. Final full HTTP race with cancellation overlay repeated three times PASS 4.825s; pinned golangci-lint 2.14.0 zero; git diff --check clean. This independent acceptance supplies one required review; orchestration owns collecting the second before committing.

## Raw evidence

- /tmp/toolsy-task41/r16-a-race.log: initial affected race PASS.
- /tmp/toolsy-task41/r16-a-lint.log: initial pinned lint zero.
- /tmp/toolsy-task41/r16-a-probe_test.go and r16-a-probe-overlay.json: deterministic cancellation regression, read-only overlay.
- /tmp/toolsy-task41/r16-a-cancel-before-fix.log: initial defect FAIL.
- /tmp/toolsy-task41/r16-a-final-race.log: corrected whole-module race plus overlay count3 PASS 4.825s.
- /tmp/toolsy-task41/r16-a-final-lint.log: final pinned lint zero (private cache, --allow-parallel-runners).
- /tmp/toolsy-task41/r16-baseline-encoding.log: original public GET/POST accepted invalid UTF8 as success, FAIL as intended.
- /tmp/toolsy-task41/r16-current-encoding.log: current public/new probes PASS 2.144s, count5 race.

Limits: local httptest fixtures, no live external API. No claim of universal bug freedom, manual host retry prohibition, remote effect rollback or hard CPU cancellation. UTF8 validation adds one bounded linear scan; no latency guarantee claimed. General transport/request timeout semantics are existing broader contracts, not newly remapped by this encoding remediation.
