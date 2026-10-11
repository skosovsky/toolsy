# Official MCP SDK integration: contract and coverage

Baseline HEAD: `4f7eae6cc2393da259cf30a5535bef12c93d6e2c`; the initial tree was clean.
Candidate uses official published SDK v1.8.0, origin
`3f3b699b2b67e1ed033a63d6651671dab53c2d32`, sum
`h1:KIvahhYqwtbeniWVPs3TcXEA7b8jEtwfBpOTAI+Urx4=`. No SDK fork/replace,
custom MCP codec, peer, wire DTO or transport remains. Root dependencies are unchanged.

The user updated the goal on 2026-10-11: documented, reproduced SDK limitations
are accepted; upstream fixes are not required for completion. Lossless migration
is not claimed. Adapter defects and mandatory authorization/approval/scope/replay
contracts remain blockers. Final acceptance needs independent completeness and
correctness reviews on the same snapshot; this matrix is evidence, not their verdict.

## Spec and ownership

The baseline [API/test inventory](mcp-sdk-api-inventory.md) records deleted symbols
and tests. The current [module contract](../mcp/README.md) and
[migration guide](migration-mcp-sdk.md) define the breaking API and dependency limits.
Core execution/result/control/policy contracts remain authoritative for execution.

Public import API: ComposeClient(*sdk.ClientOptions, ImportConfig), ClientHooks.Install,
ClientHooks.Bind(*sdk.ClientSession, hostIdentity), Importer.Discover/ResourceTool,
Updates/Done/Err/Close and WithRound(context.Context, Round). Export is independently
Export(*toolsy.RegistryView, ExportConfig) returning *sdk.Server.

SDK owns transports, wire types, negotiation, caches, notification dispatch, OAuth,
subscriptions and public extension methods. Host owns SDK options, connection,
authenticated identity, BYOT policy types, network budgets and process supervision.
Adapter owns SDK-visible DTO copies, transactional discovery, bounded admission,
per-call progress routing, generation revocation and core delivery projection.
Only public SDK callbacks/middleware are used; Connection is not wrapped.

ComposeClient copies options and composes callbacks before Connect. Adapter updates
precede exactly one host callback; defaults still invalidate SDK caches. Session
identity and per-call token isolate progress. Bind refuses foreign/duplicate
sessions. Close cancels adapter children, never a borrowed SDK session. SDK Wait
observer starts at handshake observation, including never-bound connections, and releases state at host session closure (TestSDKUnboundSessionCompletionReleasesHookOwnership); no cancellable SDK Wait API exists.

Imported NewProxyTool handlers validate input before dispatch and output before
successful delivery. Core prepared execution owns policy/approval/cache/journal.
Generation guards run before dispatch and replay/yield. Host classification is
explicit; remote annotations and metadata grant nothing. A result keeps owned
SDK content in TypedResult; it cannot populate internal effects/controls/provenance.
A manually continued round carries host request state/responses, never a grant.

Export uses raw Server.AddTool (not generic inferred schema) and scoped view/session
execution. Host selects one server per immutable scope; discovery and direct call
use that authority. Current authority is resolved again before dispatch. Explicit
selection is required. Core owns approval binding, operation replay and uncertainty.
Default projection is one terminal model-audience result; host controls independent
stream aggregation and approval/pause mapping. Unsupported known combinations fail
registration. Collection is bounded; post-effect delivery failure never resends.

## Capability coverage matrix

Statuses: preserved by adapter/core; delegated to SDK/host; intentionally changed
under the accepted dependency contract. Each row records baseline → current owner
and replacement executable evidence. All evidence is local actual-SDK interoperability
or characterization; no live external/provider interoperability is claimed.

| ID | Baseline capability → target owner | Contract / evidence | Disposition |
|---|---|---|---|
| M01 | Client discovery/pages/iterators → SDK ListTools + Importer.Discover | TestSDKDiscoveryRejectsCursorLoopsDuplicatesAndBudgets, TestSDKCancelledDiscoveryDoesNotPublishOrRevokeExistingGeneration; atomic complete owned catalog | preserved |
| M02 | MCPTool schemas → SDK Tool + core NewProxyTool | TestSDKImportSchemaAndOwnership, TestSDKSchemaDefaultNullZeroAndDialectParity, TestCharacterizationSchemaActualWire; explicit schema, pre-dispatch validation, SDK numeric constraints documented | preserved SDK-visible; numeric/raw limits accepted |
| M03 | proxy execution/output checks → core prepared execution | TestSDKBadOutputIsNotRetried, TestSDKImportUnknownEffectOutcomeNeverRedispatches; invalid input zero calls, post-effect uncertainty no repeat | preserved |
| M04 | ContentBlock/ResourceContents → SDK union DTOs | TestSDKContentTypesAndRemoteControlsAreData; all five blocks, order/bytes/MIME/meta and untrusted controls | preserved SDK-visible |
| M05 | raw structuredContent → SDK decoded result + owned projection | TestCharacterizationStructuredContentActualWire/DTORepresentationLoss; scalar/object/array/number/null losses explicit | intentionally changed, accepted SDK limitation |
| M06 | resources/templates/resource-as-tool → SDK methods + ResourceTool | TestSDKResourcesTemplatesPromptsAndResourceTool | preserved; SDK wire delegated |
| M07 | prompt list/get/rounds → host SDK session | TestSDKResourcesTemplatesPromptsAndResourceTool; no duplicate prompt facade | delegated |
| M08 | pre-Await progress completion → callbacks + sending middleware | TestSDKProgressCompositionIsolationAndConsumerAbort, TestSDKProgressQueueAndRetirement, TestSDKConsumerAbortAndLateProgress, TestSDKConcurrentProgressSerializedAndDrained, TestCharacterizationProgressAfterSDKTerminalBeforeMiddlewareReturns; bounded/drained serialization; strict wire-completion hook unavailable | preserved local lifecycle; SDK ordering limitation accepted |
| M09 | cancellation/transport close → child context + host SDK ownership | TestSDKCancelAndAdmissionLeaveSiblingSessionLive, TestSDKInvalidationCallbackCompositionAndDisconnect | preserved; no shared close |
| M10 | explicit MRTR → disabled SDK auto MRTR + NeedsInputError/WithRound | TestSDKMRTRManualRoundAndParamsOwnership, TestSDKHostRoundRejectsDetectableRepresentationLossBeforeDispatch, TestMRTRSingleAttemptAndManualContinuation, TestCharacterizationCallParamsMutation | preserved one call per attempt |
| M11 | Listen/ACK/filters/invalidations → SDK subscriptions + local Updates | TestSDKInvalidationCallbackCompositionAndDisconnect, TestSDKInvalidationDuringCallRevokesSuccessfulDelivery, TestSDKResourceRevokedDuringPreparationNeverDispatches, TestCharacterizationPublicResourceSubscriptionLifecycle, TestCharacterizationSubscriptionFailureIsNotReportedOrRetried, TestSDKInvalidationOverflowFailsClosedWithoutClosingBorrowedSession; no public equivalent subscription lifecycle handle; minimal gap below | intentionally changed, accepted SDK limitation |
| M12 | discovery cache vs result cache → SDK TTL + owned DTO/core cache | TestCharacterizationCatalogCacheAliasing, TestSDKImportCurrentPolicyAndReplay; no force-refresh claim, host identity/generation partition | preserved ownership; SDK freshness delegated |
| M13 | raw SnapshotDigest → versioned SDK-visible digest | TestSDKTwoSessionsOneClientIsolation, TestSDKImportCurrentPolicyAndReplay; host identity/generation, no legacy digest reuse | intentionally changed |
| M14 | extensions/_meta/unknown fields → SDK DTO + host domain codecs | TestSDKContentTypesAndRemoteControlsAreData, DTORepresentationLoss, malformed HTTP fixture; unknown inert, lost raw fields documented | SDK-visible preserved; wire limitation accepted |
| M15 | HTTP auth/safe errors → SDK OAuth/host HTTPClient + BridgeError | TestSDKExportCrossScopeDiscoveryAndDirectCall, TestSDKDiagnosticCausesStayInspectable | delegated auth; preserved safe adapter diagnostics |
| M16 | Mcp-*/x-mcp-header routing → SDK transports | TestSDKActualHTTPRoutingHeaders; Mcp-Name/Param/Version verified on actual wire, SDK owns mapping | delegated, actual-wire verified |
| M17 | transport/admission caps → public SDK caps + host client + adapter limits | TestSDKDiscoveryRejectsCursorLoopsDuplicatesAndBudgets, TestSDKCancelAndAdmissionLeaveSiblingSessionLive, queue tests; no own transport | preserved adapter bounds; transport budgets host-owned |
| M18 | STDIO/process tree → SDK Command/Stdio/IO + host supervisor | TestSDKSTDIOImportAndExport uses real subprocess and waits for child exit; descendants/stderr aggregate budgets no longer automatic | delegated, explicit guarantee change |
| M19 | exact version/no fallback → SDK negotiation/capabilities | TestNegotiationFallsBack; capability gating uses negotiated capabilities | intentionally changed |
| M20 | new server export → raw SDK Server + scoped core view/session | TestSDKExportScopedPolicyAndProjection, TestSDKExportCrossScopeDiscoveryAndDirectCall, TestSDKExportApprovalBindingReplayAndRevoke, TestSDKUnsupportedExportModesRejectedBeforeEffects, TestSDKExportOutcomeProjectionAndPostEffectFailure | implemented independently |

## Minimal upstream gaps

G01/G02: numeric tokens across ID/result/schema/meta paths. StructuredContent fix
[#1239](https://github.com/modelcontextprotocol/go-sdk/pull/1239) merged after v1.8.0;
schema/meta [#1310](https://github.com/modelcontextprotocol/go-sdk/issues/1310) /
[#1311](https://github.com/modelcontextprotocol/go-sdk/pull/1311). Generic argument
[#1201](https://github.com/modelcontextprotocol/go-sdk/issues/1201) /
[#1302](https://github.com/modelcontextprotocol/go-sdk/pull/1302) avoided using raw AddTool.
G03: presence/raw fields and strict malformed response validation before DTO loss.
G04: public per-call cache bypass/explicit invalidate without altering host TTL.
G05: public subscription handle for ACK/effective filter/ID/completion/error/cancel.
G06: request-completion hook before Await to retire progress at the wire boundary.
G07: cancellable session completion observation. No separate upstream issues were
confirmed for G01/G03–G07; local reproductions/source inventory are used rather
than invented links. No external issue/PR was submitted.

These gaps are dependency limitations, accepted only as documented/tested behavior;
they do not permit adapter races, policy bypass, duplicate effects or secret leakage.
Upgrade procedure and concrete consequences are in the module README. SDK-only
fixture mcp/testdata/sdkcheck pins the same release without toolsy/replaces. Consumer
fixture verifies optional/core integration with GOWORK=off and explicit unpublished
source replacements, never an SDK replacement or workspace resolution.

## Verification

Run make test/lint for root and every nested module, applicable integration/e2e
checks, independent examples and GOWORK=off consumer/SDK-only fixtures. Review
logs under docs/reviews/mcp-sdk-integration. No releases, pushes or external writes.

## Expanded SDK package scope

The user expanded the goal to PKG-01–PKG-08. [Auth composition contract](mcp-sdk-auth-contract.md) specifies owners, identity/approval boundaries and acceptance. Previous reviews cover only the 40-item baseline migration; final expanded acceptance uses 48 items. The synthetic auth fixtures and runnable auth example implement this addendum. Expanded independent acceptance and the final snapshot are recorded in the [independent review](reviews/mcp-sdk-integration/completeness-review.md).


| Expanded requirement | Owner and evidence |
|---|---|
| PKG-01 | Four-package owner table in mcp-sdk-auth-contract.md; adapter remains optional; external consumer composes SDK public types |
| PKG-02 | examples/auth uses stock AuthorizationCodeHandler, host HTTPClient and importer; TestSDKOAuthChallengeDiscoveryAndRefresh counts one business dispatch |
| PKG-03 | TestSDKOAuthFailureCancellationAndSecrets; NewTokenSource host persistence callback; real SDK PKCE/exchange/refresh success/failure |
| PKG-04 | examples/auth SDK RequireBearerToken + ProtectedResourceMetadataHandler; fixture maps TokenInfo to CallContext verified subject/tenant |
| PKG-05 | TestSDKBearerZeroDispatchAndIdentityRebind; TestSDKExportApprovalBindingReplayAndRevoke now runs behind stock OAuth/bearer middleware with exact core grant/revoke |
| PKG-06 | TestSDKJSONRPCErrorCodeDataCauseAndSafeFormatting preserves inbound SDK code/data/cause; TestSDKExportHostProtocolErrorProjection sends host-approved outbound code/data over authenticated HTTP; public SDK-only wrapped Data loss characterized |
| PKG-07 | TestSDKSTDIOImportAndExport and independent import/export examples; host/process trust owns STDIO credentials and lifecycle |
| PKG-08 | Pinned release/SDK-only reproductions and limitation/upgrade tables in mcp/README.md; no fork, private hooks or replacement decoder |
