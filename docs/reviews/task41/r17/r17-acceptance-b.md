# R17 / D33 independent acceptance B

Base: 64a40e5. Reviewed current tracked diff and untracked representation.go / representation_test.go; latest doc.go host terminology and actual `[x]` expansion fixture included. Read original R17/D33 and ledger row 17. Read-only review; no repository edits, commits, delegation or other reviewer verdicts.

| Criterion | Score | Evidence |
|---|---:|---|
| 1. Explicit representation, plaintext losslessness, only declared HTML conversion | 20/20 | Zero and text/plain pass body unchanged; no TrimSpace/sniff path. Addresses, placeholders, XML, whitespace and empty fixtures. Only text/html invokes converter. Finite unsupported declarations fail. |
| 2. Truthful output and inspectable errors, no repair, interrupts | 20/20 | Read result representation is text/plain or text/markdown. Unsupported declaration typed cause, UTF-8 sentinel, conversion original cause preserved through INTERNAL ResultContractError. WithErrorFormatter public tests emit no chunks for representation/encoding failures. Shared core ResultContractError precedence prevents repair/retry. Cancellation guards before/after synchronous converter return interrupt without fallback. |
| 3. Raw/final inclusive bounds and expansion | 20/20 | Body, aggregate source and item fields (including representation) checked before conversion. Final json.Marshal budget includes metadata, annotation and escaping; exact/one-over fixtures pass. Independent Markdown-escape expansion probe also rejects final wire overflow; pre-canceled converter is not called. |
| 4. API/docs, host parsing ownership, send unchanged | 20/20 | README, migration, MessageBody and package comments assign MIME/charset parsing to host adapter, show BodyHTML migration and synchronous cancellation limitation. OutgoingMessage/sendArgs/doSend and dangerous/confirmation annotations unchanged. |
| 5. AAA proof and independent verification | 20/20 | Public old-API baseline rejects regression in all four plaintext fixtures (baseline changed bytes, current passes). Independent final mail race count=2 passes, targeted adversarial overlay count=3 passes, pinned lint zero. Permanent tests use AAA behavior assertions. |

Total: **100%**. Verdict: **ACCEPT**. Unresolved detected implementation errors: **none**.

Independent verification:
- /tmp/toolsy-task41/r17-b-race-final.log: `go -C toolkits/mail test -race -count=2 ./...`, PASS 1.617s.
- /tmp/toolsy-task41/r17-b-lint.log: golangci-lint 2.14.0, private cache, --allow-parallel-runners, 0 issues.
- /tmp/toolsy-task41/r17-b-probes-final.log: overlay adversarial converter pre-cancel and actual Markdown expansion/final bound plus representation/conversion tests, race count=3, PASS 2.046s.
- Reviewed supplied r17-baseline-public.log (four byte-loss failures) and r17-current-public.log (PASS 2.335s), same public old API probe/overlay.
- git diff --check clean.

Limits: No live provider SDK/MIME integration claimed. Provider allocation/transport caps remain host responsibility. HTML converter synchronous, cancellation checkpoints do not promise CPU preemption. Final JSON limit checked after conversion/encoding; no preallocation hard bound claimed. Supplemental initial blockquote fixture did not actually expand (213 output vs 236 input), so its invalid expansion assumption was replaced with verified Markdown-escape expansion; no product failure arose from that fixture. Existing metadata prefix intentionally retained; plaintext identity applies provider body suffix.
