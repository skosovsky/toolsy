# gRPC contract adapter

The source contract is protobuf descriptors and [ProtoJSON](https://protobuf.dev/programming-guides/json/).
Discovery supports unary RPCs with proto3 ordinary messages. Input and output use canonical JSON
field names, closed objects, optional fields, repeated fields, string-keyed maps, booleans,
strings, canonical padded base64 bytes, bounded 32-bit integers, decimal-string 64-bit integers,
and named/numeric enum values. Non-finite double values use ProtoJSON's strings.
Input is deliberately a canonical subset of ProtoJSON: aliases, null-as-omission, quoted 32-bit
numbers, and exponent/number forms of 64-bit integers are not advertised. Unknown fields are errors.
Default-valued output fields may be absent. A compiled output schema validates each successful result.

Construction fails with `UnsupportedError` for recursive message graphs, depth above 64,
more than 2048 total field and expanded enum-entry visits, proto2/editions, real oneofs, groups/extensions, non-string map keys,
float32 (its shortest decimal encoding needs a separate rounding contract), and well-known types (including nested WKT); none are projected as ordinary objects.
Shared nonrecursive messages are legal. Streaming methods fail discovery explicitly; use a service
allowlist to select a unary-only service. No partial discovery success conceals rejected methods.
Names and schema output are deterministic. Response byte limits return errors without slicing JSON.
Connections, deadlines, authentication and authorization remain the host's responsibility.

## Bounded reflection discovery

`Options.Services` selects service full names before `FileContainingSymbol` requests.
Excluded services (including reflection services) are never fetched. Duplicate names
fetch once. Descriptors returned for selected services retain their dependencies;
allowlisting service names does not discard dependency files or bypass schema rules.
The existing supported unary/schema subset remains unchanged.

Discovery uses finite inclusive aggregate budgets, configured in `Options`:

| Field | Zero default | Counted work |
|---|---:|---|
| `MaxDiscoveryServices` | 256 | All entries in the service-list response, including excluded, empty and duplicate entries |
| `MaxDescriptorFiles` | 512 | All received descriptor blobs across responses, including dependencies and repeated files |
| `MaxDiscoveryBytes` | 8MiB | Sum of `proto.Size` for all received reflection responses, including service-list/envelope/unknown fields |

Positive limits are inclusive; negative discovery or execution-response limits fail
`Reflect` before opening the reflection RPC. Zero `MaxResponseBytes` keeps the
existing 512KiB execution-result default. Adapter aggregate quota refusals wrap
`ErrDiscoveryLimit`; no partial tools are returned. Repeated files consume budget
before deduplication; structurally identical files merge, conflicting descriptors
with the same filename reject the complete discovery. Selected dependencies must
be supplied by the reflection server; missing imports fail rather than compiling
an incomplete registry. No additional remote dependency scheduler is provided.

A gRPC per-message receive cap equal to `MaxDiscoveryBytes` also bounds each decoded
response. Oversized messages can fail earlier with a gRPC resource-exhausted error.
Aggregate `proto.Size` counts canonical protobuf encoded size, not transport headers,
compression size or exact incoming wire spelling. Received/decoded objects, schema
projection and tool construction need additional memory; this is not an overall
process heap quota. A response is already decoded by gRPC before aggregate checks,
but checks precede descriptor decoding/append/registry compilation by this adapter.

The allowlist slice is captured on entry. Hosts must not concurrently mutate input
options during capture. The connection remains borrowed; discovery cancels its own
stream on return and never closes the connection. Host deadlines, authentication,
server trust and authorization remain required. Local fixtures prove these supported
paths, not arbitrary remote-server conformance or live distributed behavior.
