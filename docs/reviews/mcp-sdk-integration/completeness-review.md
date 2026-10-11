# Independent completeness review

Reviewer: independent subagent `/root/completeness_review`; did not implement the candidate or read the correctness review.

Baseline HEAD: `4f7eae6cc2393da259cf30a5535bef12c93d6e2c`.
Reviewed initial snapshot: `f90dbcd9c83297fafff0207c11673062182f64976e2ec01d44591abd1d9deace`.

The denominator was fixed before evaluation: **40** mandatory requirements. SDK limitations accepted by the user's updated goal count as satisfied only when documented and supported by characterization or verifiable public API/source evidence. They do not waive adapter execution contracts. The denominator remains fixed for rechecks. A passed item means the listed requirement has evidence, not proof of complete absence of defects.

## Final expanded-scope verdict

**48/48 = 100%; COMPLETE** on final snapshot `b4defda488a7158b309e7c8fd4f6677aa1119af728628a18ca9a0c08648cba69`.

Administrative final recheck independently recomputed b4defd… and compared every candidate file against the verified complete-source e2e copy of a2080d…. Exactly two files differ: task status/P8/review checkbox/trailing acceptance prose, and one design sentence replacing pending acceptance with its report link. Requirements, implementation, tests, examples, dependencies and all other contract content are byte-identical. The 48-item verdict/evidence therefore remains valid on the final snapshot. No new mandatory scope appeared in this administrative update. The detailed implementation and checks below were verified on a2080d…; only the stated administrative acceptance delta forms b4defd….

The denominator remains fixed: all original 40 requirements plus PKG-01–PKG-08, including their detailed clauses. Final category coverage: architecture/spec/cleanup 8/8; client import/fidelity 10/10; lifecycle 9/9; export/execution 7/7; verification/docs/process 6/6; SDK package/auth composition 8/8.

V01 independently confirmed from complete final logs and parent exit-status evidence: race/count1 make test, zero-issue make lint, integration and complete-source isolated e2e for all 27 modules; module verification, GOWORK=off consumer, three runnable examples, dependency isolation and diff check PASS. Snapshot independently recomputed after all checks; root manifests unchanged. Reviewer independently reran final optional/SDK-only race/count3 and consumer, and ran auth example.

V06 confirmed: parent relayed the other independent reviewer's expanded-scope APPROVE for exactly a2080d… after its own hash/checks. I did not read that report or use its findings to replace my own evidence review. Both reviews now concern the full 48-item scope on one candidate. No new external/publish/neighboring-app changes or fork were introduced.

All previously reported completeness gaps are closed. Independent SDK-only repetition exposed one unsynchronized characterization fixture; public session.Wait synchronization fixed it without weakened assertions, and reviewer full race/count10 recheck passed. Necessary outbound JSON-RPC code/data now have explicit trusted SDK-public ErrorMapper projection with ownership/bounds and actual bearer HTTP evidence. Default private causes remain private.

Remaining limits are accepted SDK behavior, explicitly documented and characterized/source-supported: numeric/presence/raw/unknown-field loss, permissive DTO decoding, cache freshness, subscription failure visibility, progress completion timing, cancellable Wait absence, wrapped JSON-RPC Data loss by default, OAuth closure typed-cause collapse and child-only process supervision. These do not waive execution boundaries and are not represented as upstream fixes. No lossless migration guarantee is approved.

## Expanded-scope recheck history

Snapshot: `f949907b80ba5450c368c26e5d20e3635c7b7dfeb848e626f37a107cacc9c17d`, independently recomputed. Fixed denominator **48**, unchanged from the scope-expansion decision. Current result **46/48 = 95.83%; INCOMPLETE**, pending only final full checks V01 and independent expanded-scope correctness confirmation V06. The 38 remaining original items and all eight added requirements are confirmed. Previous verdicts below are historical scope evidence.

| ID | Expanded status | Independently inspected evidence |
|---|---|---|
| PKG-01 | PASS | docs/mcp-sdk-auth-contract.md owner matrix; design addendum maps all four packages. Production adapter still depends on public mcp only; host examples/fixtures use auth/oauthex/jsonrpc purposefully. Root dependency manifests unchanged. |
| PKG-02 | PASS | examples/auth composes stock AuthorizationCodeHandler into host-created StreamableClientTransport, HTTPClient timeout and importer hooks. Real HTTP TestSDKOAuthChallengeDiscoveryAndRefresh exercises challenge/metadata/PKCE/code exchange; counters show one business dispatch separate from auth HTTP rounds. Example independently run: n=7. |
| PKG-03 | PASS | TestSDKOAuthFailureCancellationAndSecrets covers discovery failure, consent denial, deadline cancellation and refresh failure. Success fixture checks stock refresh success; NewTokenSource is host persistence callback, authorization interaction is host callback. SDK oauth2.RetrieveError remains inspectable while BridgeError formatting excludes secrets. Stock closure cause loss separately characterized. |
| PKG-04 | PASS | Example and fixture compose RequireBearerToken, TokenVerifier and ProtectedResourceMetadataHandler with oauthex DTOs. TokenInfoFromContext is mapped to BYOT core CallContext; actual domain handler asserts verified subject/tenant. Host opaque-token map explicitly stands in for deployment issuer/audience/signature verification; SDK is not falsely credited with them. |
| PKG-05 | PASS | TestSDKBearerZeroDispatchAndIdentityRebind rejects missing/invalid/expired/insufficient tokens before protocol/business execution; closes old binding+session, creates Bob token/client partition, revokes old tool, changes digest/catalog, rejects Alice direct call and ignores spoofed metadata. ExportApprovalBindingReplayAndRevoke now runs through authenticatedSDKPair stock bearer/OAuth and verifies valid access token still needs exact-argument host approval, one effect/replay and current revoke. |
| PKG-06 | PASS | TestSDKJSONRPCErrorCodeDataCauseAndSafeFormatting sends public jsonrpc.Error through actual HTTP; errors.As retains code/data; safe verbs hide remote diagnostic secrets. External consumer also proves public SDK/error API graph. No parser/peer/transport fork added. |
| PKG-07 | PASS | Actual STDIOImportAndExport remains unchanged/passing; independent STDIO process/host ownership, no bearer/OAuth imposed; HTTP versus STDIO trust boundaries explicitly documented. |
| PKG-08 | PASS | Official pin unchanged; existing numeric/lifecycle reproductions and source gaps retained. New TestCharacterizationOAuthClosureCauseCollapse uses stock SDK public OAuth extension point and a synchronized session.Wait closure boundary. Nil/cancellation cause loss documented in module/auth contract with no invented issue and upgrade check. |

Detailed client/server clauses under PKG IDs were also checked: SDK owns authorization discovery/refresh; host owns persistence/credentials/consent/IdP/network configuration; no new adapter token store or business resend; cache authority cannot survive principal change; access scopes/consent never grant operation approval; untrusted identity metadata is inert; deployment verifier responsibilities remain explicit; errors preserve inspectable SDK causes where SDK supplies them; secret-bearing causes must be handled without logging wholesale.

Independent runs: optional module race/count3 PASS (8.415s), external consumer race/count1 PASS (1.511s), auth example n=7. Initial SDK-only race/count3 exposed unsynchronized closure characterization (three failures); reported immediately. After synchronization via public session.Wait without weakened assertion, entire SDK-only race/count10 PASS (5.274s). This reviewer evidence defect is resolved. No correctness-review report read.

### Error projection recheck

Further candidate `a2080d2574f6888e473c138cdeda1ce1bec750d49e204d9fb697bff612898064` was independently recomputed and checked. PKG-06 now additionally covers outbound necessary safe diagnostics through explicit trusted `ExportConfig.ErrorMapper`, using the SDK public jsonrpc.Error type. mapExportError owns Data bytes, bounds message+Data, rejects malformed JSON/over-budget output, preserves default safe error when mapping nil, and never initiates redispatch. TestSDKExportHostProtocolErrorProjection confirms approved code/data/message over actual bearer HTTP with one business effect and private local cause kept out of the response; ownership/limits tests cover host reuse. SDK-only WrappedJSONRPCErrorDataLoss characterizes the default upstream behavior. Module/auth docs correctly distinguish inbound inspectable causes, outbound approved projection and local causes that cannot be serialized. No protocol parser/transport workaround added; all other requirement evidence retained.

## Prior-scope final result

**Final-snapshot confirmation withheld:** administrative recheck of snapshot
`8e4f650750817eb37e6038e6273149e0c135f8af5e27a92bd2bb9d0de2e33c49`
found additional mandatory PKG-01–PKG-08 and a new goal requiring explicit SDK
auth/oauthex composition and OAuth/bearer interoperability. Those requirements were
not in the scope used to fix the original 40-item denominator and have not been
evaluated or implemented by this review. The COMPLETE verdict below applies only
to the previously reviewed scope; it must not be presented as completion of the
new full task. Parent notified immediately. A scope-change review must add the
new mandatory requirements without reducing the existing denominator and verify
their evidence before final completion.

### Expanded scope denominator

The user explicitly expanded the active goal with PKG-01–PKG-08. Before evaluating
its implementation, the next review denominator is fixed at **48**: the original
40 items remain unchanged and eight package-composition requirements are added.
Each PKG item includes all its detailed client/server clauses in the task (network
ownership, token persistence, secret handling, refresh/error/cancellation,
principal replacement, identity/scope/approval distinctions); these clauses are
not discarded by grouping them under eight IDs.

Current expanded-scope verdict: **41/48 = 85.42%; INCOMPLETE**. Previously completed
V06 becomes unverified for the expanded scope; PKG-07 and PKG-08 are confirmed by
existing evidence. No denominator reduction or accepted exception is applied.

| New ID | Status | Evidence / required work |
|---|---|---|
| PKG-01 | PARTIAL | Existing mcp/jsonrpc ownership and independent core confirmed; design lacks auth/oauthex composition and owner matrix. |
| PKG-02 | ABSENT | No client OAuthHandler example or actual SDK synthetic OAuth integration/business-dispatch-count fixture. |
| PKG-03 | ABSENT | Challenge/discovery, refresh success/failure, authorization refusal/cancellation and secret-safe token persistence evidence not present. |
| PKG-04 | ABSENT | No SDK bearer middleware/protected-resource-metadata example or verified TokenInfoFromContext mapping to scoped executor. |
| PKG-05 | PARTIAL | Prior scope/approval tests exist; required negative bearer-token/scopes/principal composition and principal replacement not exercised. |
| PKG-06 | PARTIAL | SDK JSON-RPC ID characterization and error causes exist; public error code/data/cause roundtrip on adapter boundary lacks explicit preservation fixture. |
| PKG-07 | PASS | Actual STDIOImportAndExport, process reaping and documented host connection ownership; no HTTP auth imposed. |
| PKG-08 | PASS | Official v1.8.0 pin, SDK-only reproductions, verified issue links and explicit upgrade procedure. |

Next independent review must evaluate these eight requirements, rerun affected
checks and reconfirm all original 40 items plus correctness on the new snapshot.

**40/40 = 100%; COMPLETE** on candidate snapshot
`27364cece6b8598b1cf6a3f5b11bbd507969fe3c8db2a3565432917e3f32bcce`.
I independently recomputed this digest after the recheck. The fixed 40-item denominator is unchanged.

Final confirmed category coverage: architecture/spec/cleanup **8/8**; client import/fidelity **10/10**; lifecycle **9/9**; export/execution **7/7**; verification/docs/process **6/6**.

The initial table below records original evidence and gaps; this final recheck supersedes its eight non-PASS statuses. All other 32 items remain confirmed after independent review of the final candidate changes.

| ID | Final status | Additional evidence closing initial gap |
|---|---|---|
| I02 | PASS | TestSDKSchemaDefaultNullZeroAndDialectParity: actual HTTP, draft-07 and 2020-12, unchanged schema/manifest, n=0, nullable=null, absent default not materialized, required field rejected with zero dispatch. Existing output-schema tests retain validation-before-success coverage. |
| L03 | PASS | TestSDKConcurrentProgressSerializedAndDrained: four distinct call payloads; unique tokens; per-call yield concurrency guard; progress before each terminal; accepted queue drained. Existing queue-overflow, retirement and consumer-abort tests retained. |
| L06 | PASS | TestSDKInvalidationDuringCallRevokesSuccessfulDelivery: actual SDK list-change while dispatched call blocked; zero successful deliveries; stale reuse rejected; one effect. ResourceRevokedDuringPreparationNeverDispatches and canceled-discovery regression add predispatch/publication checks; replay tests retained. |
| L07 | PASS | SDK-only TestCharacterizationPublicResourceSubscriptionLifecycle: actual HTTP ACK narrowing, supported updates, absent capability, Unsubscribe releasing listener and live shared session. SubscriptionFailureIsNotReportedOrRetried reproduces nil Subscribe/sticky listener marker after upstream failure and explicit reset. Missing public handle remains documented accepted gap, not falsely claimed adapter parity. |
| E04 | PASS | TestSDKExportOutcomeProjectionAndPostEffectFailure: empty/noop/soft/fault/limit/projector failure over SDK HTTP; secret-safe diagnostics; each case exactly one dispatch/effect and no implicit resend. Default one-terminal source checks and scoped JSON tests retained. |
| V01 | PASS | Parent confirmed exit 0 for final make test (27 modules, race/count1), make lint, make test-e2e and module verification; complete logs independently inspected through final module. Integration log PASS. E2E ran identical candidate files in isolated staged local tree because baseline release fixture exports git-tracked files; original untracked-only failure retained transparently. No original index or external system changed. |
| V05 | PASS | Baseline test inventory uses plain historical references, no links to deleted tests. Module/migration/design docs include final replacement evidence and actual subscription failure consequences. Breaking changes, ownership and SDK loss limits remain explicit. |
| V06 | PASS | Parent relayed independent correctness review FINAL APPROVE on the identical 27364c… snapshot. I did not read that report or its findings; reviewers remained independent. Source scope remains toolsy; no publication, SDK fork or neighboring-app changes. |

Independent final reruns: optional module `go test -race -count=3 -timeout=90s ./...` PASS (8.309s); SDK-only fixture same command PASS (2.990s). Root core graph excludes SDK/optional module; root manifests unchanged; git diff --check PASS. Details in completeness-review-checks.log.

Accepted remaining limitations are upstream dependency behavior: numeric precision/presence/unknown fields, permissive SDK DTOs, discovery cache freshness, subscription failure visibility and handle lifecycle, post-wire-terminal progress scheduling, cancellable Wait absence, and child-only process supervision. They are documented and supported by characterization or public API evidence; they do not constitute open adapter defects. No lossless migration claim is approved. Completion percentage measures mandatory evidence coverage, not probability of perfect correctness.

## Initial result (retained audit history)

**32/40 = 80%; INCOMPLETE.** Eight items need additional evidence or completion. All other items were checked through independent source/test reading, module graph inspection and logs. I independently ran the optional module with `GOWORK=off go test -race -count=1 -timeout=90s ./...` (PASS). The SDK-only fixture was also independently rerun.

Status `PASS` means fully confirmed; `PARTIAL` and `UNVERIFIED` do not enter the numerator.

| ID | Mandatory requirement | Initial status | Independent evidence / gap |
|---|---|---|---|
| A01 | Check current HEAD and preserve initial user delta | PASS | Design/task record exact baseline and clean tree; current git HEAD matches baseline. |
| A02 | Spec/Contract-First design and complete capability inventory | PASS | Updated task contract, design M01–M20 and baseline exported declarations/test inventory; independent comparison against deleted families. |
| A03 | Official published pinned SDK, no fork or SDK replacement | PASS | mcp/go.mod and sdkcheck/go.mod pin v1.8.0, matching checksum/origin; no SDK replace. |
| A04 | Single protocol engine; delete custom peer/DTO/codec/transports | PASS | git deletions of peer.go, protocol*.go, transport*.go; remaining production files use public SDK; no hidden Connection wrapper. |
| A05 | Core independence and optional-module boundary | PASS | root go.mod/go.sum unchanged; root dependency graph inspected; SDK imports restricted to optional module. |
| A06 | Independent import and export, runnable examples | PASS | examples/import and examples/export independently construct peers without opposite adapter; both example logs show n=7. |
| A07 | BYOT, SDK types/public extension points; no broad settings facade | PASS | ExportConfig.Resolve returns core CallContext; public SDK Tool/Result/ClientOptions; callbacks/middleware only; no reflection/unsafe/private accesses. |
| A08 | Remove legacy dependencies, fixtures/examples/current docs | PASS | legacy task34 fixture tree and old examples/docs removed; old task35/task34 migration guides explicitly historical; httptool adapter dependency removed. |
| I01 | Transactional bounded tool discovery/pagination/duplicates | PASS | import.go fetchCatalog/Discover; TestSDKDiscoveryRejectsCursorLoopsDuplicatesAndBudgets. |
| I02 | Input/output schema parity: required/default/null/zero/dialect | PARTIAL | TestSDKImportSchemaAndOwnership verifies integer schema/ownership but lacks default/null/zero/dialect executable cases. |
| I03 | Prepared execution, input zero dispatch, output before success | PASS | proxy NewProxyTool/importTool SupportsPreparedExecution; schema ownership and bad-output tests. |
| I04 | All content types preserve order/MIME/bytes/annotations | PASS | TestSDKContentTypesAndRemoteControlsAreData checks all five SDK content types and exact marshalled ordered block equality. |
| I05 | Structured content shapes/numbers/presence accepted limits | PASS | SDK-only actual wire object/array/scalar/exponent and DTO null/missing tests; README explicitly rejects lossless claim. |
| I06 | Resources/templates/prompts and resource-as-tool | PASS | TestSDKResourcesTemplatesPromptsAndResourceTool over actual SDK HTTP; SDK methods remain directly available. |
| I07 | Errors distinguish soft fault/schema/cancel/unsupported; safe causes | PASS | TestSDKSoftErrorAndFaultAreDistinct, DiagnosticCausesStayInspectable, BadOutputIsNotRetried, admission/cancellation tests; BridgeError unwrap. |
| I08 | Extensions/meta preserve SDK-visible data, inert authority | PASS | content test checks forged effects/replay/control metadata do not become core authority; unknown fields/duplicates actual HTTP characterization. |
| I09 | Explicit host classification; remote hints confer no permissions | PASS | Classify required by proxy; owned descriptor; host options; README explicitly classifies this trusted port. |
| I10 | Versioned digest binds host authority/generation, no legacy reuse | PASS | digest domain source; TestSDKTwoSessionsOneClientIsolation and current-policy/replay; migration mandates discard legacy digests. |
| L01 | Pre-Connect composition, exactly one preserved host callback | PASS | ComposeClient composes handlers; Install observes handshake; Bind rejects foreign sessions; host progress/list-change assertions. |
| L02 | Session/call isolation with unique tokens and bounded admission | PASS | ClientHooks sessions map, per-importer calls; two-session test, concurrent calls, admission no dispatch. |
| L03 | Serialized progress, accepted drain-before-terminal, no cross-call data | PARTIAL | Existing concurrent test only counts host callbacks; lacks adapter payload/token cross-talk and buffered ordering assertions. Queue/retirement/consumer-abort cases exist. |
| L04 | Consumer abort/cancel/disconnect cleanup without shared close | PASS | CancelAndAdmissionLeaveSiblingSessionLive, ConsumerAbortAndLateProgress, Disconnect; Close revocation; actual SDK child cancellation. |
| L05 | Manual single-attempt MRTR, owned params and host continuation | PASS | MRTRManualRoundAndParamsOwnership, SDK SingleAttemptAndManualContinuation, CallParamsMutation; automatic setting rejected. |
| L06 | Invalidation before/during calls/replay; atomic replacement/revocation | PARTIAL | Idle invalidation and rediscovery replay tested; no in-flight invalidation prevents success delivery fixture. |
| L07 | Subscription capability/error/listener/ACK lifecycle disposition | PARTIAL | Missing public handles documented/source-supported, local overflow tested; direct SDK subscribe/unsubscribe and absent capability/error behavior not exercised. |
| L08 | SDK discovery cache ownership/freshness separate from core replay | PASS | SDK CatalogCacheAliasing actual HTTP plus adapter clone test; current-policy replay test; Discover explicitly not force refresh. |
| L09 | Host transports/OAuth/headers/STDIO budgets/negotiation intact | PASS | no adapter transport; HTTPRoutingHeaders actual wire; STDIOImportAndExport child reaped; negotiation fallback; process-tree guarantee change clearly delegated to host. |
| E01 | Explicit scoped export selection/schema/raw AddTool | PASS | Export selection rejects duplicates/missing; validates schemas; raw sdk.Server.AddTool preserving raw args. |
| E02 | Trusted transport auth, same discovery/dispatch scope, no spoofing | PASS | actual HTTP CrossScopeDiscoveryAndDirectCall, ScopedPolicyAndProjection; unknown direct names denied. |
| E03 | Scoped core executor and reauthorization before dispatch/replay | PASS | exportCall uses view.Execute or trusted same-view Session port; current Resolve and core policy; revoke/replay test. |
| E04 | Terminal/empty/soft/fault projection and post-effect delivery bounds | PARTIAL | source implements paths but replacement tests lack export empty/noop/soft cases, projector/byte-limit failure and no-resend after effect. |
| E05 | Approval/pause bound exact args/identity, no model grants | PASS | ExportApprovalBindingReplayAndRevoke: spoofed state/meta no effects; changed arguments fail; host-issued grant succeeds once; replay/revoke checked. |
| E06 | Independent streams/attachments/unsupported combinations explicit | PASS | registration rejects independent/no Project and confirmation/no Control; default rejects unmapped controls/audiences/binary; explicit Project contract. |
| E07 | Effects/control uncertainty and no automatic redispatch | PASS | ImportUnknownEffectOutcomeNeverRedispatches claimed operation transitions unknown; export/core operation replay one dispatch. |
| V01 | Root/all nested race/lint and applicable integration/e2e/build | UNVERIFIED | make-test/make-lint/integration logs pass; final candidate e2e still pending (tracked-only attempt excludes untracked candidate files). |
| V02 | AAA actual SDK HTTP/STDIO/in-memory interoperability tests | PASS | source AAA structure and real transports; independent optional race run passed; no account/provider effects. |
| V03 | GOWORK=off SDK-only and external consumer module graph | PASS | no SDK fixture replace; consumer replaces only unpublished toolsy/core+adapter; make-test includes nested fixtures; consumer API compilation external package. |
| V04 | Minimal upstream gaps, issue comments, characterization/upgrade | PASS | README and design G01–G07, exact upstream links without invented issues; sdkcheck; raw AddTool avoids known generic precision path. |
| V05 | Accurate breaking migration/docs/evidence without false guarantees | PARTIAL | current guides sound; inventory test list has broken links to deleted files despite header promising historical references. Fix links; gaps above require matrix evidence update. |
| V06 | Two independent final-snapshot reviews; no prohibited writes | UNVERIFIED | this review is independent; second verdict/final same-snapshot confirmation still pending; diff stays within toolsy, no SDK fork/publish changes. |

## Initial required recheck (all closed above)

Close I02, L03, L06, L07 and E04 with focused executable evidence; fix V05 historical references; attach final applicable checks for V01 and final independent verdict/same snapshot evidence for V06. Rerun affected suites and confirm final digest. No accepted numeric/raw representation SDK limitation is being treated as a migration blocker.
