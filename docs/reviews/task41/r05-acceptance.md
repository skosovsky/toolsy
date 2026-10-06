# R05 / D37 — isolated release checkout and exact artifacts

Scope: row 05 of task41. R06 tag scope, collision checks and atomic publication
remain a separate gate; the private clone still uses `push --tags` in this row.
No production push or publication was performed.

## Contract and implementation

The host wrapper builds committed runner code from a private Git archive. The
runner snapshots committed HEAD, rejects tracked dirt, clones without hardlinks,
and performs manifest edits, artifact verification, commit and publication in
that private checkout. Untracked and ignored source files are excluded. Source
Git reads disable optional locks and fsmonitor. Private hooks are disabled.
Only the complete tracked module inventory's go.mod/go.sum files are staged.

Go-native manifest edits align internal versions and remove development replaces.
A cached Git attribute preflight runs before private checkout and before bootstrap
archive extraction. Active transformations (including export-ignore/subst, filters,
encoding/EOL/ident attributes) fail explicitly; ordinary diff/merge/linguist metadata
and absent/unset transformations are supported. Private Git forces LF/real symlinks;
source Git preserves its EOL policy. Counted auth/identity remains available to the
native CLI while private repository/checkout/hook selectors are filtered. Bootstrap
rejects inline configuration. CRLF source and auth-preservation probes pass.
Modules are prepared in dependency order. Official x/mod ZIP creation preserves
Go packaging rules, including nested-module exclusion and inherited root LICENSE.
Each exact ZIP is downloaded from an invocation-owned module proxy and compiled
with GOWORK=off. The parity fixture compares module checksum hashes against
CreateFromVCS from the candidate commit. Go module/cache/checksum-database state
is invocation-owned; downloaded external archives are a read-only fallback.
Overlapping GONOPROXY patterns switch external fallback to cached archives/direct
VCS, preserving private names from public proxy queries. External checksum policy
is retained. Module and full-check timeouts are explicit.

The CLI runs existing lint/test and break preflight in the candidate checkout.
Checks that modify tracked candidate files block publication. Cancellation closes
the prompt input, terminates native command groups, and cleans private state.
Bootstrap build cancellation has a separate bounded process-group path. The CLI
stays in the terminal foreground group for interactive confirmation.

## Regression and checks

- [Original baseline](r05/baseline.log): the original script publishes a fake
  untracked note, removes it from the source and changes its index. This is a
  behavioral failure, rather than a build/setup failure.
- [Release race](r05/release-race.log): success, verification/publish rejection,
  tracked dirt, incomplete inventory, prepare-only, prompt cancellation, hostile
  command-group cancellation, three-module graph and committed VCS hash parity.
  Bootstrap fixture contains an invalid untracked Go file, excluded from the build.
- [Root race](r05/root-race.log): PASS.
- [Targeted lint](r05/targeted-lint.log): zero issues.
- [Reviewer A policy/process probes](r05/review-a-probes.log) and
  [hostile bootstrap cancellation](r05/review-a-wrapper.log): PASS.
- [Reviewer B probes](r05/review-b-probes.log): excluded untracked extra module,
  rejected checksum symlink without modifying its target, and blocked publication
  after full checks changed a tracked candidate file.
- [Reviewer B real PTY](r05/review-b-pty.log): confirmation succeeds in a disposable
  local bare fixture; Ctrl-C cancels the next candidate and preserves source files.

Initial acceptance found a working-tree bootstrap input gap, non-Go-compatible
GONOPROXY prefix matching and an interactive SIGTTIN regression after introducing
job groups. All were fixed and both reviewers repeated their independent checks.
Two private full-check runs failed on golangci-lint's cross-process lock while
another lint invocation was active; both 24 ZIP compilations had passed. Makefile
now waits on that lock with --allow-serial-runners. A following run failed before
publication on direct gopkg.in DNS lookup. The public gopkg.in/check.v1 archive was
then fetched through the configured proxy into a task-owned external cache, with
the hash matching the original go.sum. That read-only cache overlays existing
host archives. The final candidate rerun uses the installed pinned golangci-lint
2.14.0 through MAKEFLAGS, avoiding rebuilding the linter; rules are unchanged.
That run then found three stale post-validator assertions in document/timetool;
these tests now assert the already-established R04 INTERNAL/result_validator/cause
and no retry/input-correction contract, without changing toolkit runtime code.
The new attribute preflight also closed an independently reproduced false-positive
ZIP verification with a committed export-ignore attribute; forced private checkout
materializes files after read-tree. Its final result is tracked separately below.

## Independent final acceptance

Reviewer A and reviewer B: **100%, accepted**, each of the five criteria at
20/20. No unresolved detected defects. Both independently checked the final code
and confirmed the successful full CLI execution. Reviewer A's final release race
passed in 21.163s; review B's combined adversarial/config race passed in 15.826s.

[Final graph](r05/final-graph.log) contains 24 successful exact ZIP verifications.
[Final execution](r05/final-execution.txt) records exit 0 of the full prepare-only
CLI, including private make lint test/race, post-check cleanliness and cleanup.
The installed linter matched pinned version 2.14.0. Successful Make stdout is not
claimed as separately retained per-test logs.

## Limits

macOS was exercised. Linux shares the supported process-group implementation but
was not executed in this session. Process descendants that deliberately escape
owned groups are outside the lifecycle guarantee. Bootstrap and candidate inputs
are committed, but host tools, caches and Git signing are trusted host facilities.
Verification compiles packages/tests from ZIPs and executes the existing candidate
suite; it does not run every downloaded dependency's tests. Cyclic internal module
graphs, local external replaces and proxy-only dependencies under an overlapping
private policy fail explicitly. Migration details are in docs/migration-task41.md.
