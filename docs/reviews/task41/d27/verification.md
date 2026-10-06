# D27 verification

Baseline HEAD: fb7a2bf (D25 signed commit). D27 is a deliberate API clear break,
not an R/P1 defect. No compile-only failure is presented as a behavioral baseline.

Parent full affected E2B module `GOWORK=off GOCACHE=/tmp/toolsy-review-gocache go -C adapters/sandbox/e2b test -race -count=3 ./...`: PASS17.559s. Pinned golangci-lint2.14.0, affected module, dedicated cache, `--allow-parallel-runners`:0issues.
Retained lifecycle/capped output/timeout/cleanup tests pass after runtime fixture
migration. Args ownership test probes original slice mutation after option creation,
client mutation of actual dispatched slice, and construction reuse after changing
first sandbox config. Public ExampleNew exercises new interface, literal metacharacter
argument and canonical path using a local demo client, not a cloud/SDK implementation.

Legacy parser/tokenizer/encoder removed. CommandResult has only ExitCode. Test
transport's separate fixture bytes are written to supplied writers, not returned
through CommandResult. No uncapped output fallback introduced. Serializer belongs
to injected client only if transport needs one; adapter preserves literal argv.
No live E2B isolation/destruction/output/cancellation certification, host programs
remain trusted and may interpret their own arguments.

Independent acceptance A/B pending.

Parent final selfcheck additionally rejected raw noncanonical plus already-canonical
script references that would rewrite to two canonical args. Regression added and
latest targeted argv/example racecount3PASS1.862s/pinnedlint0. Docs agree on both
raw exact match and unique canonical script argument. Full independent acceptance
runs inspect the latest constructor change.

Final independent A/B accepted100% each, five20/20, no unresolved errors.
Latest fullmoduleracecount3PASS16.694s/16.807s and pinnedlint0; public adversarial
consumer probes PASS1.693s/4.802s. RealOSargvtransport, concurrentmutatingports,
ignoredwritererrors, exact/+1caps and lifecycle preserved. Reports/fixtures in
acceptance-a/b. Separate signed commit authorized after doubleacceptance.
