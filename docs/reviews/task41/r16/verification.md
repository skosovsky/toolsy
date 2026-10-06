# R16 verification

Explicit UTF-8-only GET/POST tool bodies preserve valid bytes and reject invalid
bytes before encoding/json can substitute U+FFFD. Library byte readers unchanged.
POST response read/wire failures retain original causes in INTERNAL result phase;
request bounds remain validation. Cancellation checked before wire classification.

Both independent acceptance reports: 100% (5 x 20%), no unresolved detected errors.
Raw public baseline/current probes, final parent/reviewer race/lint logs included.
The baseline is committed e3fc1f9. Baseline failure is intentional evidence; the
initial reviewer cancellation failure is superseded by correction and final PASS.
The r16-root-lint.log file came from root module (original temporary filename
misidentified it as HTTP); parent-final-http-lint is the actual HTTP module check.
The private overlays add probe files only, never replace production code.
Local HTTP fixtures only; no live service, universal absence-of-bugs or latency
claim. UTF-8 validation adds one bounded linear scan before existing JSON encoding.
