# contracts/

Contract-based translators that turn external API specs into [toolsy](..) tools. Each submodule is **isolated** (own `go.mod`); add only the one you need.

| Module       | Entry point              | Input                    | Output              |
|-------------|---------------------------|--------------------------|---------------------|
| **openapi/**  | `ParseURL(ctx, specURL, opts)` | OpenAPI 3.x spec URL     | `[]toolsy.Tool` (one per operation) |
| **graphql/**  | `Introspect(ctx, endpoint, opts)` | GraphQL endpoint URL     | `[]toolsy.Tool` (one per Query/Mutation field) |
| **grpc/**     | `ConnectAndReflect(ctx, target, opts)` | gRPC server address      | `[]toolsy.Tool` (one per RPC method) |

Common behavior:

- **Tool names** are sanitized to `^[a-zA-Z0-9_-]{1,64}$`; collisions get a numeric suffix (`_2`, `_3`, …).
- **Response truncation**: if the response body exceeds `MaxResponseBytes` (default 512 KiB), it is truncated and a short message is appended.
- **Context**: all network calls use the given `context`; cancellation aborts the request.

Register each tool with your registry: `for _, t := range tools { reg.Register(t) }`.

## openapi/

- **Requires:** `github.com/getkin/kin-openapi`
- **Options:** `HTTPClient`, `BaseURL`, `AuthHeader`, `AllowedTags`, `AllowedMethods`, `MaxResponseBytes`
- **BaseURL:** If empty, the first server from the spec (`doc.Servers[0].URL`) is used. URL placeholders `{variable}` are replaced with `Server.Variables[variable].Default` when defined in the spec. If the spec has no servers and `BaseURL` is empty, **tool execution** returns an error: `openapi: base URL required (set Options.BaseURL or add servers to the OpenAPI spec)`.
- **Naming:** Prefers `operationId` (sanitized); fallback is method + path (e.g. `get_users_id`).
- **Path / query / body:** At execution, path parameters go only into the URL path, query parameters only into the query string, and only keys from the operation’s `requestBody` schema are sent in the request body for POST/PUT/PATCH. This avoids 400 from strict APIs (e.g. Spring, ASP.NET) that reject extra body fields.
- **Spec loading:** The spec is loaded from the fetched bytes; external `$ref` (e.g. to other files) may not resolve. In-document refs (`#/components/...`) are resolved by kin-openapi. Body schema keys for the above split use the resolved `Schema.Value`.

## graphql/

- **Requires:** only stdlib (`net/http`, `encoding/json`)
- **Options:** `HTTPClient`, `AuthHeader`, `Operations` (e.g. `["query"]` or `["query","mutation"]`), `MaxResponseBytes`
- **Safety:** Query text is generated once at introspect time with variables; at runtime only `variables` is filled from LLM args (no GraphQL injection).
- **Introspection errors:** If the introspection response contains GraphQL `errors` or the HTTP status is not 2xx, `Introspect` returns an explicit error (e.g. `graphql: introspection errors: <message>` or `graphql: introspection HTTP 401: ...`), so you get a clear cause instead of an empty tool list.
- **Selection set:** Each generated query uses a minimal selection set `{ __typename }`; the response is minimal. Building a deeper field graph for the response is a separate feature request.
- **Type depth:** Argument types use a recursive `graphQLTypeRef` (name, kind, ofType); the introspection query requests full type depth via the `TypeRef` fragment, so nested wrappers like `[String!]` are resolved correctly.

## grpc/

- **Requires:** `google.golang.org/grpc`, `google.golang.org/protobuf` (reflection: `protoreflect`, `protodesc`, `protoregistry`; dynamic messages: `dynamicpb`; JSON: `protojson`). No third-party reflection libraries.
- **Options:** `DialOptions`, `Services` (allowlist of full service names; empty = all), `MaxResponseBytes`
- **Reflection:** Uses gRPC Server Reflection (v1alpha) over the official stream API: `ListServices` then `FileContainingSymbol` per service; file descriptors are merged and resolved via `protodesc.NewFiles`. No `.proto` files needed at runtime.
- **Invocation:** RPC calls use `grpc.ClientConn.Invoke` with `dynamicpb.NewMessage` and `protojson` (request: `UnmarshalOptions{DiscardUnknown: true}` for LLM output; response: `Marshal`).
- **Connection lifecycle:** The connection is closed on reflection error or when no tools are returned (`len(tools) == 0`). Otherwise it is kept open for the lifetime of the returned tools and released when they are no longer used.
