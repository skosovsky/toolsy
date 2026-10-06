# Transcript codec

`MarshalToolCall` and `MarshalToolResult` produce version 2 JSON transcript records.
They preserve tool/call IDs and raw bytes (base64), including binary payloads.
Results preserve empty/noop flags and explicit envelope kind, audience, delivery
class, raw bytes, MIME, JSON metadata and soft error classification. A missing
source envelope is resolved with `Chunk.ToolEnvelope()` during encoding; decoding
always returns the explicit recorded envelope and never chooses an audience.

Only `EventResult` chunks without typed values, effects, controls or progress are
supported. Envelope result values and wrapped `ToolError.Err` are rejected. The
supported error representation contains code, retryable, reason, fixable args and
safe message; it does not reconstruct Go error identity. Call attachments, RunEnv
and any nonzero CallContext are rejected rather than silently discarded. Hosts
must deliberately project execution records to transcripts before encoding them.

Metadata accepts JSON values: null, bool, string, finite float64, `json.Number`,
`[]any` and `map[string]any`. Numbers decode as `json.Number`, preserving their JSON
lexeme; arbitrary Go values inside metadata are rejected rather than losing their
type implicitly. Cyclic metadata fails encoding. No credentials or authenticated
context are serialized.

Decoding requires all declared fields, explicit valid delivery bindings, a matching
record kind and exactly one document. Unknown and duplicate fields, including in
nested metadata, fail. Version 1 is unsupported; there is no fallback or migration.

These bytes are transcript data, not authority to re-execute an action or apply an
effect. For complete typed persistence/replay use `toolsy.ResultCodec` (for example
`JSONResultCodec[YourResult, YourEffect]`) through the existing cache/journal ports.
