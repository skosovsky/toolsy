# D40 — filesystem and sandbox boundaries

Baseline: signed `6907b80 docs: time semantics`. Five acceptance criteria are
recorded in the progress ledger, each 20%. No execution algorithm is changed.
Production identity hashes cover all six relevant module implementations.

Decision: retain filesystem pathname policy, in-place writes, non-snapshot ranges,
Starlark instruction/output bounds and trusted host execution. The host owns
hardlink/mount policy, root placement, snapshot consistency and atomic/durable
replacement. No storage manager, scheduler or universal isolation mechanism added.

Public fixtures use disposable local files, explicitly demonstrate hardlink read
and outside alias mutation with inode identity retained, and changed content
between read ranges. They prove documented limitations, not security isolation,
mount containment, crash recovery or all-platform behavior. Existing path/symlink,
FIFO, output/cancellation/lifecycle tests remain intact.

Parent raw checks: `/tmp/toolsy-task41/d40`. All six relevant module race count3 and pinned lint2.14.0 pass. Wazero
race3 took241.989s; its final lint reports0 issues. Raw final logs copied. Docker live fixture is
explicitly SKIP without opt-in; it is not live proof. E2B uses unit client mocks;
no real cloud service or SDK verification. Parent initial godoclint finding was
fixed with a standard-library type link and retained as a failed initial log.

Independent A and B accepted100%, five20/20 criteria each, no unresolved detected defects.
Reports, probes and logs archived acceptance-a/b. Both ran all six module race3
and pinned lint0; Wazero238.707s/233.835s respectively. They independently prove
write effects survive delivery errors, Starlark builtin allocations are not
process memory bounded, and trusted host processes can read outside workspace.
No runtime caches or full checkout archives retained.


Parent current and exact6907b80 public limitation fixtures visibly execute three
times and pass. This confirms retained behavior, rather than asserting a baseline
regression. Baseline production snapshots are text evidence, not compilable Go
packages. Initial godoclint finding retained; fstool-lint-final is the corrected
nested module check, module-0-lint-corrected is supplemental root lint.
