# SDK auth composition contract (PKG-01–PKG-08)

This addendum expands the reviewed migration scope from 40 to 48 requirements.
Previous adapter-code verdicts do not certify this addendum. The official release
list and Go @latest were rechecked on 2026-10-11: published v1.8.0 remains pinned.
HEAD remains 4f7eae6cc2393da259cf30a5535bef12c93d6e2c; existing core consumers are
unchanged, optional MCP examples/consumer fixtures require the new SDK API.

| Package | Owner / use | Required executable evidence |
|---|---|---|
| mcp | Adapter uses SDK client/server/session/transports and public hooks | Existing HTTP/STDIO/import/export contracts |
| jsonrpc | SDK structured protocol error/message types retained as BridgeError cause | Actual wire error code/data, safe formatting, errors.As |
| auth | Host OAuthHandler, TokenVerifier, RequireBearerToken and token-context mapping | Synthetic challenge/discovery/refresh/rejection/cancel and bearer scope tests |
| oauthex | Host SDK metadata/discovery/preregistered client configuration | ProtectedResourceMetadataHandler and AuthServerMeta fixtures, SDK discovery counters |

Host creates the transport, HTTPClient and SDK AuthorizationCodeHandler. ComposeClient
only composes client notification options and disables automatic business MRTR;
it does not replace OAuthHandler/HTTPClient. SDK owns challenge parsing, discovery,
PKCE/token exchange and refresh. Host owns credentials, persistence, consent callback,
IdP/network configuration and secure diagnostics. Examples use synthetic opaque
tokens/local servers only; they are not deployable token verification systems.

SDK bearer middleware verifies before MCP dispatch. Host verifier must enforce
issuer/audience/signature or opaque-token trust according to its IdP; middleware
alone does not implement those checks. SDK TokenInfoFromContext is mapped by a
trusted host resolver into subject/tenant scope. Per-principal SDK servers use
immutable scoped catalogs. Access scopes permit access, not confirmation of a
particular operation. Current core policy and exact-argument approval grants still
apply; OAuth consent, _meta/inputResponses and a valid token are never such grants.

A session binding is immutable to a verified principal. On credential/principal
change host closes the importer AND its SDK session and constructs a new client,
OAuth handler/host token partition and binding. Reusing an SDK cache/session with
changed credentials is unsupported: remote cacheScope does not partition authority.

Stock HTTP OAuth can retry the HTTP request after a 401/403 authorization attempt.
Do not promise zero HTTP retries. Synthetic tests count actual business handler
calls: verification before handlers means those authorization requests perform no
business effect. Adapter itself never retries business calls. An unknown/post-effect
outcome does not authorize reconnect/resend. Host must never translate a completed
effect into an authentication challenge in middleware after dispatch.

STDIO uses host/process trust with SDK Command/Stdio/IO transports, no HTTP bearer
or OAuth requirement. The same core authorization/approval boundaries still apply.

Secrets must not enter BridgeError text, MCP results/snapshots or logs. SDK/host
errors may retain sensitive causes for explicit host handling, which must not be
logged wholesale. Synthetic adversarial tests inspect safe adapter formatting and
public SDK error types separately. Any observed SDK diagnostic/cause loss is
characterized and documented rather than repaired using a parser or transport fork.

Acceptance adds PKG-01–08 to the fixed reviewer denominator. Run auth composition
AAA contracts, runnable auth example, external consumer graph and all applicable
module checks, then independent completeness and auth/identity correctness reviews
on one new snapshot. Existing numeric/raw/lifecycle limitations and upgrade steps
remain in the module README; newly observed auth gaps require the same evidence.

Pinned v1.8.0 connection closure can drop a typed OAuth cancellation cause while
retaining ErrConnectionClosed and textual detail (SDK-only
TestCharacterizationOAuthClosureCauseCollapse). Host tracks callback/context
cancellation explicitly; adapter cannot restore a cause lost before its boundary.
No separate upstream issue was confirmed; recheck on official release upgrades.


Outbound structured errors require a host-approved ExportConfig.ErrorMapper if
Data must be published. Its SDK public *jsonrpc.Error is copied/bounded and sent
top-level; nil preserves default safe wrapping. Code/outer message survive default
wrapping but SDK v1.8.0 drops wrapped Data (public SDK-only characterization).
Raw private cause Data never becomes public automatically. Import errors preserve
received code/data as inspectable causes, while local causes remain available to
the host mapping before serialization. No separate upstream issue was confirmed.
