# Independent adversarial acceptance B — row 37 / D40

Baseline: `6907b80`. Scope: D40 only. Reviewed specification row D40, row37 five criteria, tracked diff from baseline, untracked public fixtures and D40 evidence README, fstool and all five sandbox adapters. Did not read another reviewer's report/verdict. No repository edits, staging or commits. Independent results below are primary evidence; parent results were not substituted.

## Assessment

1. **20/20 — Actual filesystem boundary.** Package API docs and README accurately state root-relative pathname resolution rather than storage isolation. `relativePath` rejects absolute/NUL/parent-segment paths and operations use `os.OpenRoot`. Hardlinks are regular files and retain outside aliases; mounts are not rejected. Root replacement/directory movement policy belongs to the host. Unix opens use O_NONBLOCK and then regular-file checks; non-Unix opening of device paths remains a host responsibility. No mount or all-platform certification is claimed.
2. **20/20 — Consistency, atomicity and durability.** `doWriteFile` opens without truncation, checks regular-file status, then calls Truncate(0) and io.WriteString on the existing descriptor. No rename, sync, rollback or version test; close errors are discarded. README correctly describes empty/partial observations, side effects on failure and delivery-error non-rollback. Read stat/seek/read and reopened directory enumeration do not pin versions. Stable sequence needs host-owned snapshot/immutable tree; replacement requires separate authorized same-filesystem temporary write/checked-close/rename and filesystem-specific sync protocol. No new lifecycle implementation.
3. **20/20 — Starlark and host limits.** Starlark exposes request env/files, steps and output limits; ExecFileOptions parsing and running Go builtins are not preempted by thread cancellation and share process memory. Own bounded public probe allocates a 4MiB string within 50 interpreted steps, confirming steps are not allocation quota. Host uses host executable and workspace as cwd, no filesystem/network/resource confinement; own shell probe reads outside workspace. Unix anchor/group ownership logic provides cancellation/cleanup of the owned group while unreaped, not escaped groups or malicious guest isolation. Non-Unix is best effort; cleanup syscall deadlines remain cooperative. Documentation matches.
4. **20/20 — Evidence matrix.** Docker daemon capability validation and container policy request mandatory local Linux/cgroups, read-only filesystems, non-root user, disabled network, cap drop and no-new-privileges; unit clients are mocks. Optional live fixture gates on TOOLSY_DOCKER_LIVE=1; independent run explicitly SKIP. No live isolation acceptance inferred. Wazero real WASI tests verify path policy, linear memory cap and cancellation, not host process memory. E2B supplies BYO-client contract with cooperative remote contexts and cleanup success meaning client success; no live service/SDK behavior verified. Matrix and detailed READMEs preserve these distinctions.
5. **20/20 — Independent evidence and synchronized migration.** All six module race/count3 and pinned lint checks pass independently. Public hardlink/inode/range fixtures and separate public probes pass; implementation identity is retained. Migration, API/README and evidence statements agree. No unresolved detected errors. Acceptance here is reviewer B's independent verdict; agreement by a separate reviewer remains the parent's responsibility.

## Execution record

All module checks use GOWORK=off and GOCACHE=/tmp/toolsy-review-gocache. Lint uses `/opt/homebrew/bin/golangci-lint` version2.14.0, `run --allow-parallel-runners ./...`, unique cache `/tmp/toolsy-task41/d40-acceptance-b/lint/<module>`.

| Module | go test -race -count=3 ./... | Pinned lint |
| --- | --- | --- |
| fstool | PASS 2.248s | PASS exit0 |
| starlark | PASS 8.645s | PASS exit0 |
| host | PASS 32.855s | PASS exit0 |
| docker | PASS 2.396s | PASS exit0 |
| e2b | PASS 16.813s | PASS exit0 |
| wazero | PASS 233.835s | PASS exit0 |

Additional independent checks:
- New public hardlink/inode and range fixtures run explicitly with race/count3: PASS1.296s, six visible test executions (`fstool-public-fixtures.log`).
- External standalone public probe, `go run -race .`: PASS hardlink outside read, preserved inode/outside alias mutation, mixed-version ranges, retained write after delivery failure, parent traversal rejection, bounded Starlark allocation demonstration, host outside-workspace read (`probe/main.go`, `public-probe.log`). All files disposable; no escaped processes or mount administration used.
- Byte identity of16 production implementation/options .go files across six modules against6907b80: PASS (`identity.py`, `production-identity.log`). Documentation and public tests intentionally change. Limitations are original behavior, not a baseline regression. Public limitation assertions are expected to pass against unchanged baseline algorithms.
- Docker optional live test: explicit SKIP (`docker-live-skip.log`). E2B no live call. No Docker daemon/platform/image certification.
- `git diff --check`: PASS (`diff-check.log`). Migration D40, fstool API/README, sandbox matrix and evidence README agree on retained implementation and host responsibilities.

Detected product/documentation defects: **none**. Harness setup issues (initial sumdb cache permission, corrected read paths, unavailable optional ps inspection) are preserved in `harness-notes.log`; no product failure inferred. Public probe go.sum was seeded from reviewed local module sums and tidy/run used GOPROXY=off.

Limits: no actual mount test, intentional disk failure, partial-write concurrency timing, power-loss/durability test, OS matrix, hard memory exhaustion or malicious process escape test performed. These are explicitly outside the claimed capabilities and were assessed from implementation. Verified hardlink/range/delivery-failure examples demonstrate limits rather than security certification.

## Final verdict

**Accepted: 100/100 (100%).** Five criteria each20/20. No unresolved detected errors. Retained hardlink/mount aliasing, non-atomic/non-durable writes, non-snapshot ranges and in-process/host resource limitations are documented design decisions, not regression failures. This verdict does not certify live Docker, E2B service behavior, mount isolation or crash recovery.
