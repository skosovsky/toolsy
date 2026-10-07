# Task36 verification

Issue: https://github.com/skosovsky/toolsy/issues/4

## Candidate and dependency identities

Implementation starts from root commit `82e51316c272de6241cae429d70a33ecbfad5220`.
The local semantic run used caller `5607832ee869f63bc3c6768a02ed60a019cd691e`,
continuation `54efe352339c44e97396a6914f974035261539dd`, and policy
`6fd6b9b2651cf41273a0c9b3f26653cc3f0a1986` checkouts. Published pins are recorded
in `examples/host_dispatch_integration/go.mod` and verified checksums in go.sum.
No credentials, live provider or production distributed store was used.

## Requirement evidence

| Requirement | Executable evidence |
|---|---|
| AC1 identity and independent intents | TestAC1IdentityAndReplay, TestAC1Admission; real native caller parts in TestRealNativeCallsSourceSchemaAndExactNumbers |
| AC2 barriers and approval | TestAC2Barriers, TestAC2InProgressStopsNextEffect, TestAC6RejectedExpiredAndConcurrentResume; real approval barrier/resume integration |
| AC3 full outcomes | TestAC3BusinessCorrectionEmptyNoop, TestAC3PartialOutcomeAndControlError, TestAC3TextMIMEProjection |
| AC4 delivery and effects | TestAC4AudienceAndProjection, TestAC4CacheReplayEffects, identity/completed replay, MIME regressions and independent adversarial probes |
| AC5 schemas and exact input | TestAC5SchemaAndCanonicalSnapshot observes binder/handler attachments and canonical default; real source-schema and int/decimal/null fixture |
| AC6 immutable approval | TestAC6BindingEditsAndCurrentAuthorization, TestAC6ToolManifestViewSchemaChanges, TestAC6RejectedExpiredAndConcurrentResume |
| AC7 uncertainty/durability | TestAC7DurableLostDeliveryReopen, TestAC7UnknownNeverBlindlyRedispatches, TestAC7PersistenceFailureAndTrustedReconciliation; real durable activity delivery-loss recovery |
| AC8 runnable independent composition | host_dispatch main/README, optional module/README, explicit CI local/published/released consumer jobs; no core consumer imports |
| AC9 gates and migration | Module-wide gates, recipe/core race, two semantic dependency modes, host-dispatch-migration.md; results below |

The implementation also rejects fresh reserved replay metadata. Exact genuine
child replay forwarding and correlation rebinding remain valid; modified replay
payload/effects/envelope/controls are rejected. Independent final correctness
report and portable probes are linked in this directory.

## Reproduction

Use a writable GOPATH/GOCACHE and GOLANGCI_LINT_CACHE if the default caches are
restricted. Those environment paths only relocate caches; they do not disable
checksum verification or change semantic assertions.

```sh
make lint
make test
go test -race -count=1 .
go test -race -count=1 -v ./examples/host_dispatch/...
go run ./examples/host_dispatch
PROMPTY_DIR=/path/to/prompty FLOWY_DIR=/path/to/flowy GUARDY_DIR=/path/to/guardy \
  scripts/task36-integration.sh local
scripts/task36-integration.sh published
make task34-preflight
```

Published mode tests candidate host recipe code as consumer code against actual
published dependencies with GOWORK=off and no replace. It is not post-release
verification of a yet unpublished candidate engine. Released mode imports the
actual published recipe and is mandatory after publication.

## Observed results

- Whole root `go test -race -count=1 .`: PASS, including nested Session replay and reserved metadata regression.
- Recipe `go test -race -count=1 -v ./examples/host_dispatch/...`: PASS.
- Provider-neutral main: PASS; approval, one fresh reducer effect, exact result and redelivery with a new CallID.
- Local integration `-race -count=1`: PASS, all four semantic tests.
- Published integration `-race -count=1`: PASS, all four semantic tests.
- Root lint after final classification changes: PASS, zero issues.
- Clear-break preflight: PASS.
- Module-wide `make lint`: PASS, zero issues across all modules.
- Module-wide `make test`: PASS, exit 0 across all modules. An earlier cold concurrent build encountered a guest workspace timeout; the final sequential run completed without failures. See make-test-summary.log.
- Independent correctness: PASS in reviewed scope; see acceptance-correctness-final.md. Independent completeness: 100%, all nine acceptance criteria; see acceptance-completeness-final.md.

## Release and closeout

Use `make release-break`: fresh producer rejection of reserved replay metadata is
a behavior break. Function signatures remain; consumers must remove manually set
provenance and use the real replay boundary. See host-dispatch-migration.md for
full caller glue changes. Publication and closeout are complete:

- `make release-break`: PASS, exit 0 after artifact verification, full candidate lint/test and clear-break preflight. Published root and optional integration tags both point to `095e2db7368021babfc6769e4319271db57b9304`; see release.log.
- Published root: `v0.19.0`, prepared from implementation commit `70371962f35132870bcc866c85c68f24e9ba3576`.
- `scripts/task36-integration.sh released v0.19.0`: PASS, all four semantic tests with `-race -count=1`, `GOWORK=off`, no replace and no copied recipe. See integration-released-race.log.
- Independent public adversarial probes against the actual published root: PASS, seven tests including nine forwarding cases, with `-race -count=1`, `GOWORK=off` and no replace. See correctness-released-race.log and correctness-final-probe_test.go.txt.
- Concrete author migration notification: https://github.com/skosovsky/toolsy/issues/4#issuecomment-6035235399 . It names the changed calls/fields, approval/recovery ownership and fresh provenance behavior.
- Issue state independently read back as CLOSED at `2026-10-07T09:38:43Z`.

SSH remote discovery did not respond and that first attempt was cancelled before publication. The successful invocation used a temporary HTTPS transport rewrite and existing `gh` credentials; source remote/config remained unchanged. No checks were skipped and checksum verification remained enabled.

The published tag preserves evidence as it existed before publication. This final verification update records the subsequent consumer checks and closeout.
