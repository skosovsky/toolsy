# R17 / D33 verification

MessageBody input: zero/text/plain preserves body bytes; only explicit text/html
converts to Markdown. Output JSON adds representation alongside existing metadata
and body. Unsupported declarations, invalid body UTF-8 and conversion failures
produce INTERNAL ResultContractError and no fallback success. Sender unchanged.

Both independent acceptances100% (5x20); no unresolved detected errors. Included
public old-API baseline/current probe, parent/reviewer race/lint logs and private
adversarial source. Baseline is committed64a40e5; four failing identity assertions
are intentional evidence of old behavior. Final bounds use actual Markdown escape
expansion `[x]` → `\[x]`, then JSON escaping; caps include representation bytes.

No live provider/MIME integration, universal bug-freedom or CPU-preemption claim.
Provider transport/allocation and MIME/charset decoding remain host adapter work.
Conversion failure seam permits deterministic cause/cancellation tests; ordinary
malformed HTML can be permissively parsed by the external converter.
