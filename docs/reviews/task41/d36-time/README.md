# D36 time/state evidence

Baseline signed9ac37e3. Spec and five20% criteria: ledger row36. Implementation only
by parent; both independent reviewers accepted 100%, five 20/20 each,
no unresolved detected errors. Actual timetool.go is byte-identical to
baseline (arithmetic-source-identity.log); no new arithmetic fix claimed. API option
comments/package docs/default tool description now state both tools use host zone
and calculate adds calendar days then elapsed hours. Runtime description changes;
lookup/arithmetic code does not. Current StateStore terminology already present in
active code/memory docs; add explicit distinct persistence/session-map/snapshot
contract, not another state field/implementation.

Replace three misleading/weak DST tests with exact public NY spring/fall, positive/
negative calendar/duration, days-before-hours, input-offset/fractional fidelity
fixtures. Dynamic provider tested for both tools on every invocation; provider error
has no static fallback; existing nil-provider-location fixture now uses valid full
wire args and asserts actual provider failure reason, not merely ErrValidation.

Runnable host recipe reads its explicit host.timezone key via env.StateStore, bounds
returned bytes at64 and accepts only host-selected UTC/NY. Immutable local fixture
is not durable storage. Store/callback errors and context retained; missing port,
65-byte overflow, exact64 bytes-but-unsupported value and empty zone tested. Callback
allocations/upstream reads and synchronization remain host responsibilities.

Parent corrected time full race3PASS1.764s and host1.609s, memory race3PASS2.087s;
both pinned module lint2.14.0 zero issues. Example prints noon NY after calendar day
vs13:00 after24 elapsed hours. Current public contract tests also compile/passrace3
with exact baseline production overlay; no false baseline-FAIL claim. Initial new
fixture/example omissions of required add_days/add_hours produced schema errors;
logs retained, inputs corrected. Initial example magic-number lint fixed with named
budget constant. These are parent fixture defects, not arithmetic regressions.

Embedded stdlib time/tzdata provides availability fallback for tests/example; actual
rules follow Go's host/embedded timezone data. Ambiguous/nonexistent AddDate wall
times retain Go normalization and no promised offset choice. No hard callback
preemption, automatic timezone StateStore integration or new framework introduced.


Final reports, raw checks, independent public probes and affected baseline snapshots
are retained in acceptance-a/b; caches and unrelated disposable checkouts omitted.
A full time/host race3 1.393s/1.239s, memory1.797s, pinned lint0; public probes1.666s,
baseline current public fixtures1.529s and own baseline probes1.484s pass. B full
time/host race3 1.388s/1.233s, memory1.900s, pinned lint0; public probes1.564s,
baseline probes1.505s and exact calendar/provider suite1.312s pass. Both verify
complete arithmetic identity, state terminology, explicit store integration,
provider precedence/no fallback, base instant/fractional and cancellation boundaries.
Both host examples print exact calendar12:00 vs elapsed13:00 results. Initial
independent path/checksum/sumdb-cache issues are retained and corrected using
proper quoted module replacements/checksums and writable task-local caches; they
are not product failures or false baseline regressions. Timezone-data availability,
borrowed host ports, upstream allocation and cooperative cancellation limits remain
explicit. No live/durable/distributed store proof or arithmetic fix is claimed.
