# D01 capability and migration verification

Checked contexty source HEAD8416b7b9883a09a26e3fe740d02dd8f78bff0b7f.
Targeted root rolling/drop-head/budget tests race PASS4.270s. Other packages in
that targeted run report no matching tests; no full contexty acceptance is claimed.
Host migration recipe extracted verbatim from the Markdown Go fence compiled in
an isolated module (GOWORK=off, local quoted path replacement). AAA race count3
PASS1.620s proves protected stable prefix, inclusive budget, exact recent tail,
provider-error cause propagation and absence of silent mechanical fallback.

Toolsy root full race PASS (generator52.173s), OTel full race PASS1.481s;
both pinned module lints report 0 issues. GOWORK=off root dependency listing
contains neither contexty nor deleted toolsy/history. Root full race includes transcript,
historycodec and ResultCodec suites. No core transcript/codec source changed,
no go.mod/go.sum change or contexty dependency introduced.

The removed package is generic chat policy, not tool-result persistence. Its
example and dedicated OTel helper/tests are removed with it; other tool tracing
payload/redaction tests remain. Current docs recommend host-owned composition and
explain projection, stable IDs, estimator/summarizer contracts, error and minimum
retention differences. No generic compatibility layer is claimed.

Both independent reports accept100% (five20% criteria), no unresolved detected
defects. Reviewer logs and host recipe source/tests are retained alongside reports.
Limits: checked local source revision, estimated fixture token counts, cooperative
host callbacks; no published-version recommendation or arbitrary-message lossless
projection/provider exact-token guarantee.
