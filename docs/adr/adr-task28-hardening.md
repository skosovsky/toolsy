# ADR: Task28 engine hardening

## Context

Task27 introduced typed tools, `RunCall`, `ManifestSet`, opaque snapshots, and structured error wire. Task28 closes remaining leaks: generic `map[string]any` on snapshot import, legacy text error chunks, and examples that encourage manual chunk parsing.

## Decisions

### 1. Opt-in strict state codecs

`WithStrictStateCodecs(true)` requires a registered `StateCodec` for every non-nil state key on export/import. Clear operations (`{}`, JSON `null`) skip codec lookup. Default remains permissive for backward compatibility within v1.x.

Missing codec → `CodeStateCodecMissing`, `Retryable: false`.

### 2. Normalize-before-validate error chunks

`prepareChunk` calls `normalizeErrorChunk` before `validateErrorChunk`. Legacy text error chunks become `MimeTypeToolErrorJSON` with `CodeInternal`. `validateErrorChunk` accepts only structured wire after normalization.

Rationale: custom tools violating the contract must not break the yield pipeline; `RunCall` still receives routable `ToolError`.

### 3. Snapshot hydration as infrastructure errors

Version mismatch and corrupt payloads → `NewSnapshotHydrationError` → `CodeInternal`, `Retryable: false`. Not `CodeValidationFailed` (LLM cannot fix DB snapshots).

### 4. Examples as canonical host API

`examples/resiliency` uses `RunCall` + `DecodeOutcomeAs` + `NewTypedTool` wrapper. Routery adapter may use low-level `Execute` internally.

## Consequences

- Hosts enabling strict codecs must register all state keys they persist.
- Emitters of plain-text error chunks should migrate to `NewErrorChunkFromErr`; normalization is a safety net, not the preferred API.
- Snapshot import failures route as infra errors in orchestrators.
