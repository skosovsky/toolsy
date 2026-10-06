# Contract adapters

Each module translates a documented subset of an external protocol into
`[]toolsy.Tool`. Modules have separate `go.mod` files; import only the adapters
the host needs. Discovery fails before publishing tools when the selected
contract cannot be represented faithfully.

| Module | Entry point | Source | Supported contract |
| --- | --- | --- | --- |
| [openapi](openapi/README.md) | `ParseURL(ctx, specURL, opts)` | OpenAPI document | Schema dialect, parameter locations and serialization subset |
| [graphql](graphql/README.md) | `Introspect(ctx, endpoint, opts)` | GraphQL introspection | Input types and host-defined output selection |
| [grpc](grpc/README.md) | `Reflect(ctx, cc, opts)` | Protobuf descriptors from reflection | Unary RPCs and ProtoJSON mapping subset |

Read the module contract before enabling operations. An unsupported shape is a
construction error, rather than a weaker advertised schema. Schema traversal is
bounded; adapters do not resolve arbitrary network schema references.

Discovery and invocation use the supplied context. Hosts provide endpoint
configuration, credentials, service connections, authentication, authorization,
timeouts and retry policy. gRPC connection ownership stays with the caller.
HTTP adapters use the safe transport from `toolkits/httptool`; private addresses
require explicit host configuration. Credentials are resolved at invocation,
not captured from model arguments or descriptions.

Successful structured responses must remain complete valid JSON and satisfy
their advertised output contract. Response limits return errors instead of
slicing JSON. Spec/introspection reads also fail closed when their byte limit is
exceeded. See [result contracts](../docs/result-contract.md) for delivery,
persistence and validation rules.

Register discovered tools through `NewRegistryBuilder().Add(tools...)`. The
adapter does not grant business permissions or install an API gateway. The host
still controls registry views, policy and consent.

The companion [`toolsy-gen`](../docs/generator-contract.md) generates tools from
local schema manifests. Its supported schema contract and generated argument
presence rules are separate from protocol discovery.

Protocol references:

- [OpenAPI specification](https://spec.openapis.org/oas/v3.0.3.html)
- [GraphQL specification](https://spec.graphql.org/September2025/)
- [ProtoJSON mapping](https://protobuf.dev/programming-guides/json/)
