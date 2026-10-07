# Current API and integration reference

Use current source contracts below. [Task41 migration](migration-task41.md) records
breaking changes and retained boundaries; [task28–35 historical evidence](history/README.md)
has a separate index. Per-row review scores are scope-specific, not a guarantee
or the final task41 acceptance.

| Area | Current reference |
| --- | --- |
| Execution, prepared dispatch, cache, operation recovery | [Execution contract](execution-contract.md) |
| Successful output, effects, controls, delivery, failure phase | [Result contract](result-contract.md), [control contract](control-contract.md) |
| Required host policy/budget gates | [Policy gates](policy-gates.md) |
| Generator types, presence, flat subset, rollback, stream/async | [Generator contract](generator-contract.md) |
| Sandbox outcomes, cleanup and capability limits | [Result/cleanup contract](sandbox-result-contract.md), [backend matrix](../adapters/sandbox/README.md) |
| Remote bridge extension and task references | [Remote bridge contract](remote-bridge-contract.md), [agents module](../agents/README.md) |
| MCP transport, discovery and ownership | [MCP module](../mcp/README.md) |
| Schema adapters | [Contracts modules](../contracts/README.md) |
| Toolkit input/output limits and ownership | [Toolkit index](../toolkits/README.md) |
| Host recipes | [Runnable examples](../examples/README.md) |
| Current clear-break API migration | [Task41 migration](migration-task41.md) |

## Installation and module alignment

The checkout requires Go1.27.1. Core does not import optional adapters/toolkits or
harness packages. Add only modules used by the host. For a chosen published release,
replace every `vX.Y.Z` placeholder with that same verified release version:

```sh
go get github.com/skosovsky/toolsy@vX.Y.Z
go get github.com/skosovsky/toolsy/toolkits/httptool@vX.Y.Z
go install github.com/skosovsky/toolsy/cmd/toolsy-gen@vX.Y.Z
```

Do not interpret `v0.0.0` plus local replaces in source manifests as a published
installation version. Root's `go.work` connects24 checkout modules, including the
local resiliency example. Consumers should verify their module graph with
`GOWORK=off go list -m all` and tests so a workspace cannot conceal an incompatible
installed graph. Root `go test ./...` does not include nested modules; `make test`
and `make lint` visit every module.

Release tooling selects aligned module versions and verifies the rewritten
artifact/consumer graph with GOWORK=off before any explicit-ref publication. This
task verifies disposable repositories/local remotes only and publishes nothing.
The source checkout contains no assertion that the task41 APIs have been published.

## Source module map

Core and optional source modules are listed below. Choose the version-aligned
module paths in your consumer; README installation commands without a version do
not define a compatible release train. The resiliency example is a local consumer,
not an optional library module to install.

| Checkout path | Module path | Reference |
| --- | --- | --- |
| `.` | `github.com/skosovsky/toolsy` | [Reference](../README.md) |
| `./adapters/sandbox/docker` | `github.com/skosovsky/toolsy/adapters/sandbox/docker` | [Reference](../adapters/sandbox/docker/README.md) |
| `./adapters/sandbox/e2b` | `github.com/skosovsky/toolsy/adapters/sandbox/e2b` | [Reference](../adapters/sandbox/e2b/README.md) |
| `./adapters/sandbox/host` | `github.com/skosovsky/toolsy/adapters/sandbox/host` | [Reference](../adapters/sandbox/host/README.md) |
| `./adapters/sandbox/starlark` | `github.com/skosovsky/toolsy/adapters/sandbox/starlark` | [Reference](../adapters/sandbox/starlark/README.md) |
| `./adapters/sandbox/wazero` | `github.com/skosovsky/toolsy/adapters/sandbox/wazero` | [Reference](../adapters/sandbox/wazero/README.md) |
| `./agents` | `github.com/skosovsky/toolsy/agents` | [Reference](../agents/README.md) |
| `./contracts/graphql` | `github.com/skosovsky/toolsy/contracts/graphql` | [Reference](../contracts/graphql/README.md) |
| `./contracts/grpc` | `github.com/skosovsky/toolsy/contracts/grpc` | [Reference](../contracts/grpc/README.md) |
| `./contracts/openapi` | `github.com/skosovsky/toolsy/contracts/openapi` | [Reference](../contracts/openapi/README.md) |
| `./examples/resiliency` | `github.com/skosovsky/toolsy/examples/resiliency` | [Reference](../examples/resiliency/go.mod) |
| `./ext/toolsyotel` | `github.com/skosovsky/toolsy/ext/toolsyotel` | [Reference](../ext/toolsyotel/README.md) |
| `./mcp` | `github.com/skosovsky/toolsy/mcp` | [Reference](../mcp/README.md) |
| `./toolkits/document` | `github.com/skosovsky/toolsy/toolkits/document` | [Reference](../toolkits/document/README.md) |
| `./toolkits/fstool` | `github.com/skosovsky/toolsy/toolkits/fstool` | [Reference](../toolkits/fstool/README.md) |
| `./toolkits/httptool` | `github.com/skosovsky/toolsy/toolkits/httptool` | [Reference](../toolkits/httptool/README.md) |
| `./toolkits/human` | `github.com/skosovsky/toolsy/toolkits/human` | [Reference](../toolkits/human/README.md) |
| `./toolkits/mail` | `github.com/skosovsky/toolsy/toolkits/mail` | [Reference](../toolkits/mail/README.md) |
| `./toolkits/memory` | `github.com/skosovsky/toolsy/toolkits/memory` | [Reference](../toolkits/memory/README.md) |
| `./toolkits/prompts` | `github.com/skosovsky/toolsy/toolkits/prompts` | [Reference](../toolkits/prompts/README.md) |
| `./toolkits/rag` | `github.com/skosovsky/toolsy/toolkits/rag` | [Reference](../toolkits/rag/README.md) |
| `./toolkits/sqltool` | `github.com/skosovsky/toolsy/toolkits/sqltool` | [Reference](../toolkits/sqltool/README.md) |
| `./toolkits/timetool` | `github.com/skosovsky/toolsy/toolkits/timetool` | [Reference](../toolkits/timetool/README.md) |
| `./toolkits/web` | `github.com/skosovsky/toolsy/toolkits/web` | [Reference](../toolkits/web/README.md) |
