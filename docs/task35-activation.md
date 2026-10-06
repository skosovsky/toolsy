# Task35 conditional activation evidence

Source: https://github.com/skosovsky/toolsy/issues/3. The current issue describes a conditional backlog, explicitly not one indivisible release gate. It specifies scenarios, but no confirmed external consumer, selected server transport, deployed catalog size or required remote profile. The issue has no comments supplying that evidence. Decisions below preserve all requirements in the matrix; deferred work is not counted as implemented.

## TLS-003 — deferred pending a programmatic consumer

`exectool/sandbox.go` exposes one-shot `Run(language/code/env/files)`. `adapters/sandbox/starlark/starlark.go` exports only `env` and read-only `fs`; its tests execute computation/file reads, not nested tools. No checked-in consumer uses a script to combine scoped registry calls. An illustrative loop in the issue is a proposal, not an activated integration.

Remaining scope: scoped Starlark tool namespace, bounded JSON converters, executor/Session budget preservation, nested operation identity/trace, sticky pending and explicit restart recipe, complete outcome metadata and conformance cases. Activation: a host provides its script/tool composition workflow, scoped executor and limits. No bridge capability is advertised meanwhile.

## TLS-004 — deferred pending a persistent workspace workflow

Existing sandbox adapters implement one `Run`. The host/docker adapter creates a temporary workspace per invocation; the remote adapter creates a session and schedules cleanup. Checked-in examples/tests do not attach another worker to a workspace after review/pause. File toolkits accessing host-selected files do not by themselves establish a persistent sandbox lifecycle requirement.

Remaining scope: Create/Attach/Execute/ListArtifacts/Close for one selected existing backend; owner/scope/generation/lease enforcement; truthful isolation requirements; portable artifact refs; explicit incomplete cleanup; capability-gated Resume/Snapshot and conformance coverage. Activation requires a confirmed multi-step consumer plus backend/lifetime/isolation/cleanup requirements. Existing one-shot execution is not re-labelled as persistence.

## TLS-005 — deferred pending a named external server consumer

The MCP examples are clients connecting to a host-supplied endpoint. No checked-in consumer publishes a registry or selects a server transport. The issue does not select one. Client support and the separate existing agent bridge are not evidence for an MCP server requirement.

Remaining scope: one consumer-selected transport, fixed wire fixtures, authenticated scoped executor, bounded/redacted discovery/call, approval/operation mapping, current-policy checks and worker-leak/replay conformance. Activation requires the external consumer, transport and supported protocol scope. No server capability is advertised and the client is not rewritten for this deferred card.

## TLS-006 — deferred pending a measured large deployed catalog

`examples/full_agent/catalog_test.go` measures the actual checked-in two-tool consumer: cardinality, full manifest payload, schema payload and defensive schema-load/encoding cost. Exact measured output is recorded in the requirements verification log. This is a local example, not a claim about a deployed host. Synthetic benchmark registries are stress fixtures, not proof that a consumer needs catalog search. The issue provides no deployed catalog/payload measurement or activation threshold.

Remaining scope: snapshot-bound keyword/filter index, bounded scoped refs/cursors, lazy resolve, existing digest reuse, forbidden/stale errors, policy rechecks and large-catalog conformance. Activation requires real host catalog size/payload/loading cost and a consumer's budget. No provider DTO or speculative second identity system is added.

## TLS-007 — deferred independently for authorization, elicitation and remote tasks

The HTTP client example uses a host-provided bearer token through the existing decorator. No endpoint/issuer requiring discovery/PKCE/refresh is identified. Generic round/extension APIs and codec registration do not implement or justify advertising remote capabilities. No checked-in consumer needs elicitation resume or a remote task handle after reconnect; local async tools are not remote tasks.

Remaining authorization scope: one selected profile with pinned normative fixtures, host credential/consent ports, issuer/resource/audience/state checks, redirect/SSRF-safe token handling and no refresh replay of an unknown effect. Elicitation and task adapters remain independently pending with their own scoped round/handle/persistence/TTL contracts and conformance. Activation requires a concrete server and the capability it requires. This decision does not reject those features or claim their implementation.
