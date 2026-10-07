# Task41 documentation checklist and executable evidence

This audit preserves all seven checklist items in the source task41 remediation.
It maps current contract/API statements to actual regression fixtures. Per-row
acceptance is evidence for its own scope; historical task28–35 scores are not
current proof. This document does not claim row40 all-module/benchmark/release or
whole-task final acceptance; those gates remain mandatory.

## 1. Public execution inputs/results/authority/failures

The [public entry-point map](public-api-contracts.md) covers core factories,
registry/view/session, profiles/reconciliation, controls/codecs, optional adapter
and toolkit execution families. Each links executable schema, authoritative
wire/typed output, audience/control/effect semantics, pre/post-handler error and
retry boundaries, cancellation and lifetime ownership. Module READMEs publish
concrete schemas/limits and borrowed ports. Current policy precedes replay; hints,
model text and post-effect failures never authenticate repair/retry authority.

Evidence: [result contract fixtures](../result_contract_test.go),
[result algebra](../result_algebra_test.go),
[post-handler regression](../post_handler_regression_test.go),
[cache delivery](../result_cache_delivery_test.go),
[approval snapshot](../approval_snapshot_contract_test.go),
[terminal operation composition](../operation_terminal_integration_test.go).
TestPostHandlerFailureNeverRepairsArguments and TestPostHandlerJournalOutcome
assert actual side effects and phase/cause; canonical/provider/terminal/operation
fixtures under `testdata/execution/` remain executable. BYOT stays independent
of host/domain libraries.

## 2. Session/checkpoint/config/codec/BYOT ownership

[Session/DI reference](../README.md#session-state-and-runenv-di),
[prepared snapshots](execution-contract.md) and task41 migration distinguish
state+binding snapshots from authority/budgets/workflow continuation. Rebind
publishes validated immutable configuration atomically; in-flight invocation
retains its captured config. State map is protected, borrowed BYOT/host ports
remain host synchronized; no universal serializer/deep-copy claim. Codec builders
freeze on successful session construction; callbacks execute outside state lock.
External StateStore is distinct from session map and snapshot contents.

Evidence: [session concurrency](../session_concurrency_test.go),
[run-policy snapshot](../runpolicy_snapshot_test.go),
[state codec tests](../session_state_test.go),
[prepared BYOT snapshot](../prepared_snapshot_test.go).
TestConcurrentSessionRebindExecutionAndCheckpoint,
TestSnapshotCallbacksReenterWithoutLocks,
TestIncompatibleRebindPreservesConfigurationAndState and
TestCatalogRequirementsValidateBeforeCodecFreeze exercise these exact boundaries.

## 3. HTTP origin/DNS/host/encoding/client/lifetime

[HTTP contract](../toolkits/httptool/README.md) and task41 migration specify
exact/suffix host syntax, deny precedence before DNS, checked-address dial without
re-resolve, credential scheme/host/port binding, safe transport/TLS customization,
UTF-8 conversion/rejection and owned idle-pool disposal. Mandatory SSRF policy is
retained; remote effectful redirects do not introduce hidden dispatch replay.

Evidence: [redirect origin/method](../toolkits/httptool/client_redirect_test.go),
[host syntax](../toolkits/httptool/hosts_test.go),
[pinned dial](../toolkits/httptool/pinned_dial_test.go),
[TLS/ownership](../toolkits/httptool/settings_test.go),
[response encoding](../toolkits/httptool/response_encoding_test.go),
[pool disposal](../toolkits/httptool/pool_test.go), plus public agents/OpenAPI/
GraphQL redirect/pool fixtures in their modules. Exact-cap streaming versus
inclusive buffered readers is documented separately, not mislabeled EOF proof.

## 4. MCP cancellation/discovery/aggregate limits

[MCP module](../mcp/README.md) documents bounded per-frame/queue/optional-lifetime,
aggregate discovery, typed full-snapshot authority publication and minimal custom
transport facets. Manual pages do not publish authority; full discovery validates
before publication. Cancellation retires invocation routes and closes owned IO
without falsely closing a shared transport or releasing another call's permit.

Evidence: [resource limits](../mcp/transport_resource_limits_test.go),
[discovery limits](../mcp/tool_discovery_limits_test.go),
[subscription retirement](../mcp/subscription_retirement_test.go),
[custom pending abort](../mcp/custom_pending_abort_migration_test.go),
[cancellation races](../mcp/transport_http_revision_test.go).
TestFullToolDiscoveryBudgetsPreserveExistingAuthority,
TestCustomPendingConsumerAbortCleansInvocationWithoutClosingTransport and
TestStdioWriteQueueByteBudgetReleasesAcrossSixtyFourCancellationRaces
exercise real library lifecycle with fixtures; they are not universal live remote
protocol conformance or proof that hostile custom ports honor cancellation.

## 5. Sandbox outcomes/exact bytes/descendants/live scope

[Result/cleanup contract](sandbox-result-contract.md),
[deadline policy](sandbox-deadlines.md) and [backend matrix](../adapters/sandbox/README.md)
distinguish guest exit, collection/transport, output cap, parent interruption and
secondary cleanup. Completed bytes are exact; retained result with cleanup failure
is not success/retry permission. Owned Unix group cleanup cannot contain escaped
malicious groups or override OS uninterruptible states. Starlark fs.read guest
failure remains distinct from missing complete output; memory is not isolated.

Evidence: [exec cleanup](../exectool/cleanup_test.go),
[effect/journal collection failure](../exectool/journal_integration_test.go),
[Docker policy/owned body](../adapters/sandbox/docker/policy_test.go),
[host group lifecycle](../adapters/sandbox/host/host_unix_test.go),
[host output collection](../adapters/sandbox/host/collection_unix_test.go),
[Starlark exact outputs](../adapters/sandbox/starlark/starlark_test.go),
[Wazero actual WASI](../adapters/sandbox/wazero/policy_test.go),
[E2B literal argv](../adapters/sandbox/e2b/e2b_test.go).
Docker live is opt-in and reported as SKIP when not enabled; E2B client mocks do
not certify cloud isolation, SDK argv or actual remote destruction. No live claim
is derived from green unit tests.

## 6. Generator rollback/DTO/stream/async/runnable host

The [normative generator contract](generator-contract.md) owns required/optional/
nullable/root/item types, exact numbers, bounded flat subset, rollback and stream
lifecycle. [Presence recipe](../examples/generated_presence/README.md) shows complete
manifest/CLI/handler setup; [nested typed recipe](../examples/nested_contract/main.go)
keeps executable nested schema; [stream recipe](../examples/generated_stream/README.md)
explicitly configures host async callback/timeout/cap/shutdown. Accepted is not
terminal completion. Task-history prose is separate from this current reference.

Evidence: [public presence tests](../examples/generated_presence/main_test.go),
[documentation regeneration](../internal/toolsygen/documentation_contract_test.go),
[source-schema parity](../internal/toolsygen/contract_test.go),
[generated consumer lifecycle](../internal/toolsygen/generator_test.go),
[commit recovery](../internal/toolsygen/commit_recovery_test.go).
Consumer modules compile and execute actual emitted factories; failed install vs
completed install/post-commit cleanup remain distinct. Filesystem crashes or
concurrent external writers are not a crash-atomic multi-file transaction guarantee.

## 7. Release artifacts and checkout

The [release implementation](../internal/release/release.go) and
[committed-code bootstrap](../scripts/release.sh) prepare an isolated tracked-input
checkout, stage expected manifests, verify rewritten graph/artifacts with GOWORK=off,
and publish only explicit refs with atomic/collision checks. Original branch/index/
untracked bytes survive success/failure/cancel. A green workspace graph cannot
prove the rewritten release graph; final artifact verification is required.

Evidence: [artifact parity](../internal/release/artifact_parity_test.go),
[exact artifact graph](../internal/release/lifecycle_test.go),
[untracked preservation](../internal/release/release_fixture_test.go),
[explicit publication scope](../internal/release/publication_test.go),
[command cancellation](../internal/release/command_lifecycle_test.go).
TestReleaseExactArtifactGraph, TestReleasePreservesUntrackedSource and
TestPublicationScope use disposable repositories/local bare remotes.
TestReleaseBootstrapDrainsArchivePadding exercises a committed-code bootstrap
with valid trailing tar padding: the complete archive is written to a temporary
file before extraction, so early tar completion cannot SIGPIPE its producer.
Prepare-only preserves the source and publishes no new tag. Production
publish/push is outside this task. Row40 must rerun final candidate artifacts and
all24modules/race/lint, targeted adversarial regressions, necessary benchmarks and
two fresh whole-scope acceptance reviews before completion.
