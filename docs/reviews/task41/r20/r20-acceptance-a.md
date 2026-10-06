# R20 / D35 DOCX — independent acceptance A

Verdict: ACCEPT. Total completion: 100%. No unresolved detected defects.
Reviewed working tree relative to HEAD f63179a, including both untracked Go test files, original R20/D35 requirements, row 20 contract, migration and toolkit README. No implementation edits or delegation.

| Criterion | Score | Evidence |
| --- | --- | --- |
| Public whitespace and styled runs | 20/20 | Public AsToolWithCleanup fixture covers transitional, strict and legacy empty namespaces with arbitrary prefixes; TAB/LF, default/page/column breaks, paragraph LF and styled-run concatenation are exact. Independent overlay additionally covers explicit textWrapping and default strict namespace. |
| Bounds and cancellation | 20/20 | Separator helper checks context and inclusive byte cap before WriteByte; text fragments retain combined ItemBytes/ParsedBytes guards. Permanent tests cover multibyte text + exact/over separator boundaries, leading separators and paragraph separators. Independent entity/combined-separator fixture passes exact five-byte boundary, rejects four without partial text and preserves context.Canceled. Full suite covers source cap, forged directory count, compressed expanded XML cap, combined text-node items, public wire overflow and cancellation. |
| Exact namespace support | 20/20 | Equality matcher accepts only transitional, strict or empty URI. No substring path remains. Public spoof URI fixture ignores foreign text/separators; independent near-match trailing-slash URI returns empty output. The contract is node namespace recognition for a text-only subset, not full schema or ancestry validation. |
| API and documentation | 20/20 | README and migration describe TAB/LF, layout flattening, retained paragraph/styled-run behavior, exact namespace set, finite bounds and cooperative synchronous parsing without hard decoder allocation/CPU claims. No duplicate separator implementation introduced. Public result shape unchanged. |
| Regression and independent verification | 20/20 | Baseline public log shows all three whitespace namespaces lose separators and foreign substring text is accepted; current public tests pass. Independent complete module race and pinned lint pass; extra overlay adversarial tests pass five iterations with race. AAA structure reviewed. |

Independent execution:
- /tmp/toolsy-task41/r20-a-race.log: go -C toolkits/document test -race -count=1 ./... — PASS, 2.620s.
- /tmp/toolsy-task41/r20-a-lint.log: golangci-lint 2.14.0 run --allow-parallel-runners, private cache — 0 issues.
- /tmp/toolsy-task41/r20-a-adversarial.log: overlay TestR20AAdversarialExactNamespaceAndEntities, race, count=5 — PASS, 1.523s.
- /tmp/toolsy-task41/r20-baseline-public.log: inspected historical baseline failures for three namespaces plus foreign spoof case.
- git diff --check: clean.

Limits: text-only DOCX extraction intentionally flattens page/column layout and does not enforce the complete OOXML schema. XML decoder work is synchronous and cooperatively canceled, with finite source/expanded/text/wire limits; hard CPU or decoder intermediate-allocation isolation is not proved or claimed. Review and tests establish the scoped contract, not mathematical absence of all possible bugs.

Final fixture refresh: read the permanent public test after parent added explicit textWrapping and its expected wrapped line; production code unchanged. Independently reran public DOCX tests with race/count=3: /tmp/toolsy-task41/r20-a-final-public.log PASS (1.272s). Acceptance remains 100%.
