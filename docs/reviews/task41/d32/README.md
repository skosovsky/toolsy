# D32 constructor configuration evidence

Baseline: signed `ad4afe2`. Parent implementation only; both independent reviewers accepted 100%,
five criteria 20/20 each, no unresolved detected errors. See ledger row 34 for the five 20% criteria and contract.

Affected modules: document, fstool, httptool, human, mail, memory, prompts, rag,
sqltool, timetool, web (including the RAG host recipe). Each full module race suite
runs three times with GOWORK=off and GOCACHE=/tmp/toolsy-review-gocache. Pinned
/opt/homebrew/bin/golangci-lint reports version 2.14.0; each module gets its own
lint cache and --allow-parallel-runners. Final logs have `-final-` in their names;
fstool's corrected final logs supersede the cognitive-complexity failure;
its latest `-before-stat.log` checks include config-before-os.Stat correction and
retained missing-root/nil/negative regression.

Baseline snapshots contain exact pre-change root production Go files as .go.txt.
A generated Go overlay replaces those production files while compiling current
D32 tests. Baseline TestD32 behavior fails on nil-option panic in nine modules,
negative-as-default handling in httptool/rag/sqltool/timetool/web, and late source
mutation of HTTP/web/SQL configuration. Human already had the intended strict
nil/negative/explicit-zero behavior and its baseline tests pass. Memory's API
changes intentionally: its baseline constructor returned no error; no current
error-returning constructor test is represented as baseline-compilable.

Initial harness failures are retained: the generated fstool test accidentally
returned a function without calling it; corrected baseline log demonstrates the
actual nil-option panic. Initial parent compile failure omitted the cleanup result
on the new httptool negative-limit path; fixed before review. Old RAG count fixture
used -1 for its default; migrated to 0. Initial lint findings were fixed without
suppressions: errors.New, direct option factories, export comments, formatting,
and private per-module configuration preparation. Baseline constructor snapshots
predate these changes; do not treat harness compile failures as behavioral proof.

Source/API/README hashes identify the acceptance candidate. Test fixtures include
nil/negative/default/order coverage for every limit, no provider/network work on
invalid web configuration, live local HTTP blocked-domain source mutation,
independent reused options, real host matching/origin normalization and SQL schema
filter semantics. ExampleNewScratchpad runs the new checked public API. No remote
provider or hard port-synchronization/deep-clone guarantee is claimed.


Final acceptance reports and independent evidence are retained in acceptance-a and
acceptance-b. A independently reconstructs nine nil panics, five negative-default
failures, three security mutation failures and the old-signature memory behavior;
B also verifies exact baseline byte identity. Both run all eleven full race suites
three times and pinned module lint with zero issues. A concurrent probes use 32
option consumers plus 1000 post-capture source mutations; B uses 128 concurrent
materializations and private config mutations, plus MinInt/-1024/-1, nil position,
zero override boundaries across all modules. Latest fstool regression/checks pass.
B's original unquoted-space-path memory baseline harness failure is retained and
excluded from behavioral proof; corrected old-signature fixture compiles and fails
on actual baseline behavior. Probe Go/manifests are archived as text so evidence
cannot accidentally participate in production module compilation.
