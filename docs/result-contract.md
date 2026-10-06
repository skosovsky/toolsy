# Executable result contract

Built-in typed, generic, dynamic, proxy and streaming builders compile the declared
output schema at construction. An invalid schema prevents registration/dispatch.
The compiler uses the existing bounded local JSON Schema machinery; external
references never cause network access.

## Representations

| Output | Schema instance |
|---|---|
| Successful JSON result | Exactly one decoded wire JSON value from `Chunk.Data` |
| Typed result with JSON raw override | Raw JSON bytes, not the retained Go value |
| Formatter or `WireJSONResult` | Final encoded value, not the wrapper Go struct |
| Text or binary result | No JSON schema validation; declared MIME remains authoritative |
| Progress/control | Not a business result; JSON progress must still be valid JSON |
| Business error | Valid structured error contract, not the success schema |
| Explicit empty/noop | No required business value; success schema is not applied |
| Terminal stream | Existing exactly-one/schema-valid JSON candidate and producer-completion contract |
| Independent stream | Each successful JSON result is validated; no cardinality/aggregation promise |

An explicit envelope must agree with the chunk raw bytes, MIME and result/error
classification; it cannot provide a second unchecked wire representation.

`TypedResult` remains the host's original Go value. Validation does not JSON-roundtrip
it or reconstruct interface values. Schema evaluation preserves JSON numeric tokens.
An absent output schema permits any JSON shape; it does not permit malformed JSON,
duplicate keys or trailing documents. MIME parameters and `+json` media types are
recognized as JSON. Text/NDJSON is a separate representation, not a complete JSON value.

The generic typed builder infers a schema from its return type except for
`WireJSONResult` and `json.Marshaler`, whose Go storage types cannot reliably describe their wire shape. Hosts using raw
formatters supply `WithOutputSchema` when they want shape enforcement. Custom tools
are responsible for compiling their own output contract before advertising it;
the core prepared profile port does not invent their construction lifecycle.

## Execution and persistence

Validation runs inside the handler continuation before cache/journal capture and
again before delivering profile replay. Producer rejection is sticky even when a
raw/stream handler ignores the yield error. `ResultContractError` distinguishes
invalid JSON, missing value and schema mismatch; it is an internal non-retryable
failure, not correctable model arguments. Terminal streams retain their distinct
`StreamContractError` classifications.

Output rejection does not undo a prior side effect. The operation profile records
unknown outcome when it cannot prove completion, and repeated delivery never
blindly dispatches the action again. Existing current authorization, audience
intersection, fencing and replay-effect suppression remain in force.

## Wire budgets

`internal/format.ValidateWireJSON` returns unchanged valid bytes or an error. A
positive budget measures final bytes after JSON escaping; oversized values return
a typed `WireLimitError` wrapped in a non-retryable tool validation error. A zero
budget means no formatter-local byte cap. The error travels out-of-band; it is not
an attempted JSON response squeezed into a tiny budget.

Shape-aware truncation belongs to an explicit toolkit DTO with truncation or
continuation semantics before serialization. Arbitrary JSON is never sliced or
given a text suffix. Standalone UTF-8 text truncation remains available.

## Clear break

`CapWireJSON` and invalid pre-marshaled JSON passthrough are removed. Update callers
to handle limit errors or use an explicit bounded representation. Mutating a
returned manifest cannot change the compiled output contract.

`historycodec` version 2 is a strict raw transcript representation, preserving
explicit delivery/audience and replay metadata. Unsupported runtime fields fail
encoding, and version 1 fails decoding. See its [supported contract](../historycodec/README.md).
Complete typed persistence uses the existing `ResultCodec` cache/journal port.

Tracing capture defaults to metadata. Error and panic text use the same opt-in,
host redaction and finite byte bounds as other content; span status descriptions
are fixed metadata. See [tracing configuration](../ext/toolsyotel/README.md).
