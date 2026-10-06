// Package historycodec encodes a narrow transcript representation of tool calls
// and delivered results. It is not an operation store or an execution replay
// codec. Version 2 preserves raw payloads, empty/noop flags and explicit delivery
// audience/classification/JSON metadata, including replay provenance.
//
// Runtime context, attachments, typed results, effects, controls, progress and
// wrapped Go errors are unsupported and fail explicitly. Use toolsy.ResultCodec
// for complete host-typed execution outcomes. Decoding rejects older versions,
// missing delivery bindings, unknown or duplicate fields, and trailing data.
package historycodec
