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
