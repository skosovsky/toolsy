# Migration guide: task30 (Fail-Closed read operations)

Task30 extends task29 architectural goals (Engine/IoC — already delivered in task29) with a **breaking read I/O contract**: transport reads fail closed; explicit opt-in truncation is a separate API.

## Architectural DoD (verify-only)

Task30 §1–4 (Dual Mode, IoC formatters, RAG router, SSRF dedup, middleware `ToolInput.ArgsJSON`) is an extension of task29 — no duplicate implementation required. See [migration-task29.md](migration-task29.md).

**IoC verify-only (task29):** Dual Mode, IoC formatters on five toolkits (`timetool`, `web`, `rag`, `sqltool`, `document` — see [migration-task29.md](migration-task29.md#ioc-formatters)), RAG router, middleware `ArgsJSON`; task30 adds read I/O breaking changes only.

**New breaking surface in task30:** read primitives only (`textprocessor`, `httptool.ReadBodyLimited`, toolkit call sites).

## Breaking changes

### `textprocessor.ErrReadLimitExceeded`

```go
var ErrReadLimitExceeded = errors.New("read operation exceeded configured byte limit")

func IsReadLimitExceeded(err error) bool
```

- **`ReadLimitedBytes`** returns **`nil, ErrReadLimitExceeded`** when input exceeds `maxBytes` (never partial data + error).
- On any limit error, data is **`nil`** — hosts must not parse truncated payloads.

### Removed: `textprocessor.ReadLimited`

Replaced by:

| API                                         | Use case                                           |
| ------------------------------------------- | -------------------------------------------------- |
| `ReadLimitedBytes(ctx, r, maxBytes)`        | Transport / security-sensitive reads (fail-closed) |
| `ReadAndTruncate(ctx, r, maxBytes, suffix)` | LLM/display tier (explicit opt-in truncation)      |

### `httptool.ReadBodyLimited` — returns `[]byte`, fail-closed

**Before (task29):** `(string, error)` with UTF-8 truncation and `\n[Truncated]` suffix.

**After (task30):**

```go
data, err := httptool.ReadBodyLimited(ctx, resp.Body, maxBytes)
if errors.Is(err, textprocessor.ErrReadLimitExceeded) {
    // handle limit — library mode
}
```

- Returns **`[]byte`**, not `string`.
- On exceed: **`nil, ErrReadLimitExceeded`** (no silent truncate).

### Tool mode: `ToolError` mapping

LLM-facing tools map limit errors to validation errors so the model can adjust inputs. Use toolkit golden-order helpers (see §170):

```go
// After ReadLimitedBytes / ReadBodyLimited:
if mapped := toolsy.MapToolkitReadError(ctx, err, "toolkit/example: read", maxBytes, "body", ""); mapped != nil {
    return result{}, mapped
}

// After stat size / semantic cap detection:
if mapped := toolsy.MapToolkitCapError(ctx, "toolkit/example: stat size", maxBytes, "file", ""); mapped != nil {
    return result{}, mapped
}
```

Applied in `httptool` probe GET/POST, `fstool` read_file, `web` scrape, `document` parsers, `rag` wire budget.

**Sentinel chain:** `toolsy.NewValidationError` stores only `ErrValidation` in the error chain — tool mode does **not** preserve `ErrReadLimitExceeded` for `errors.Is`. Hosts should use `toolsy.AsToolError` + `CodeValidationFailed` and parse the limit from `Reason` in tool mode; use `errors.Is(err, textprocessor.ErrReadLimitExceeded)` only on library primitives (`ReadBodyLimited`, `ReadLimitedBytes`).

Library mode primitives return bare `ErrReadLimitExceeded`; the host decides wrapping.

## Read tier matrix (after task30)

| Path                                    | Primitive                                                                                          | On exceed                                                                                                                                                                                                                                                                                                                                                                                                |
| --------------------------------------- | -------------------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `httptool` probe (tool)                 | `ReadBodyLimited` → bytes                                                                          | `CodeValidationFailed` + limit in message                                                                                                                                                                                                                                                                                                                                                                |
| `httptool` probe (library)              | `ReadBodyLimited`                                                                                  | `ErrReadLimitExceeded`                                                                                                                                                                                                                                                                                                                                                                                   |
| `web` scrape                            | `ReadLimitedBytes` + markdown cap                                                                  | `CodeValidationFailed` + limit in message (HTML or markdown)                                                                                                                                                                                                                                                                                                                                             |
| `document` local/remote/CSV/PDF/DOCX    | `ReadLimitedBytes` (CSV/DOCX/remote); PDF: `os.Stat` + per-page `GetPlainText` with running budget | `CodeValidationFailed` in tool path (e.g. `file exceeds N byte limit`, `pdf file exceeds N byte limit`, `pdf text exceeds N byte limit`, `csv exceeds N byte limit`)                                                                                                                                                                                                                                     |
| `exectool` sandbox stdout/stderr        | `sandboxfs.CappedBuffer` → `toolsy.MapSandboxReadLimitError` via `mapExecError`                    | `CodeValidationFailed` with subject (`stdout`, `stderr`, `container stdout`, …)                                                                                                                                                                                                                                                                                                                          |
| `agents` REST                           | `ReadLimitedBytes`                                                                                 | validation error                                                                                                                                                                                                                                                                                                                                                                                         |
| contracts spec/introspection            | `ReadLimitedBytes`                                                                                 | error                                                                                                                                                                                                                                                                                                                                                                                                    |
| contracts execute                       | `ReadAndTruncate` + `ContractsTruncationSuffix`                                                    | truncated body (display tier)                                                                                                                                                                                                                                                                                                                                                                            |
| `fstool` read_file                      | `ReadLimitedBytes` (content cap)                                                                   | `CodeValidationFailed` + content cap in message                                                                                                                                                                                                                                                                                                                                                          |
| SSE/MCP streams                         | `httptool.LimitStreamReaderWithContext` / `DrainResponseBody`                                      | Library mode: `ErrReadLimitExceeded`. MCP client maps stream cap to `CodeValidationFailed` via `toolsy.MapReadLimitError` before proxy wrap; bare sentinel in proxy handlers is also remapped by `wrapHandlerError`. Tool mode + `WithErrorFormatter`: `toolErrorFromExecutionErr` / `NewErrorChunkFromErr` use the same golden order as `wrapHandlerError` (cancel/deadline/timeout before read-limit). |
| `rag` JSON shape (`ShapeDocumentsJSON`) | wire budget pre-check in `capDocumentsForWire`                                                     | `CodeValidationFailed` when results cannot fit wire budget                                                                                                                                                                                                                                                                                                                                               |

Wire truncation (`format.CapWireJSON`, semantic suffixes for sqltool/web search) is unchanged — post-read cap, not read primitive.

Post-read semantic caps fail-closed when expansion exceeds byte budget: `document` CSV markdown table overhead, DOCX XML text extraction, `web` `HTMLToMarkdown` (markdown semantic cap via `scrapeContentByteCap`). `contracts/grpc` post-marshal truncate remains display/wire tier.

**Display / wire tier (explicit truncate after fetch):** `mail` body (`defaultMaxBodyBytes`, `TruncateStringUTF8` after IMAP fetch), `prompts` template output (`TruncateStringUTF8` after render), `agents/delegate` step output (`formatStepOutput`, 256 KiB), `contracts/grpc` dynamic execute (`TruncateBytesToValidUTF8String` after protojson), `sqltool` per-cell markdown truncate, and `format.CapWireJSON` post-marshal wire cap across document/fstool/web/sqltool — not fail-closed transport reads. `rag` `capDocumentsForWire` drops documents then fails with `CodeValidationFailed` when a single result cannot fit the wire budget. See toolkit READMEs.

**Stream tier (partial read + error):** `httptool.LimitStreamReaderWithContext` (`limitedStreamReader`) may return partial bytes together with `ErrReadLimitExceeded` during SSE/stdio JSON-RPC collection — unlike `ReadLimitedBytes` nil-on-error. Used by `agents/sse`, `mcp/transport_sse`, `mcp/transport_stdio`.

**Structural / count caps (not byte-read tier):** `document` DOCX zip entry count (`maxZipEntries`), `memory` item count limit — return `CodeValidationFailed` without `ErrReadLimitExceeded` in the tool chain.

**Sparse file stat guard:** `fstool`, `document` local paths, `toolsygen` generator reads, and PDF `os.Stat` use `stat.Size > cap` as a fail-closed pre-check. Sparse files whose logical size exceeds the cap are rejected before read even when actual content is smaller — by design.

**Observability display tier:** `ext/toolsyotel` `truncatePayload` appends `... [truncated]` to trace/log payloads — not a transport read primitive.

**Sandbox docker archive:** workspace files are read with bounded `ReadLimitedBytes` per file (`defaultMaxArchiveFileBytes`, 64 MiB) and total tar budget (`defaultMaxArchiveTotalBytes`, 256 MiB) — see `adapters/sandbox/docker`. Container log collection uses fail-closed `sandboxfs.CappedBuffer` during demux (256 KiB per stream).

**Sandbox host/wazero/starlark/e2b/docker:** process stdout/stderr use `sandboxfs.CappedBuffer` during collection (`DefaultMaxSandboxOutputBytes`, 256 KiB). Terminal paths call `sandboxfs.FinalizeOrInterrupt` (ctx interrupt before overflow/`FinishRun`; golden order: deadline/cancel/`ErrTimeout` wins over read-limit in composite). Docker container logs demux into `CappedBuffer` and finalize via the same helper. Starlark eval stderr is capped incrementally via `CappedBuffer` during `io.WriteString`. Guest script failures that surface read-limit in stderr must pass **nil** `runErr` with `exitOK=false` to `FinishRun`/`FinalizeOrInterrupt` (see `run_result.go` godoc). Starlark `fs.read` rejects files above `DefaultMaxSandboxFileReadBytes` (64 MiB).

Transport reads now fail-closed for **`document` CSV** (`parser_csv.go` via `ReadLimitedBytes`) in addition to remote/PDF/DOCX paths listed above.

## Web scrape: `WithMaxPageBytes`

`ScrapePage` is fail-closed by default. Pages larger than the derived HTML cap return an error, not truncated HTML. Markdown output after HTML→Markdown conversion is also capped (`scrapeContentByteCap`); expansion beyond the cap returns `CodeValidationFailed` with `markdown exceeds N byte limit` in `Reason` — not bare `ErrReadLimitExceeded` on the tool error chain.

Raise the budget explicitly:

```go
md, err := web.ScrapePage(ctx, url, web.WithMaxPageBytes(5*1024*1024))
```

Default wire budget is 2MB (see `toolkits/web/options.go`). For opt-in truncate of HTML without fail, wrap with `ReadAndTruncate` in host code — not via `ScrapePage` transport path.

## Middleware / raw JSON (unchanged)

`Middleware` wraps `Tool.Execute` and receives `ToolInput{ArgsJSON}` before typed parsing. Hosts can inspect raw args for PII/injection without toolkit changes — see task29 wrapper pattern in [toolkits/README.md](../toolkits/README.md).

## Context cancel / deadline (core routing)

Toolkit handlers should wrap I/O failures idiomatically — no bare `return ctx.Err()` and no inline `errors.Is(..., context.Canceled|DeadlineExceeded)` guards in tool handlers:

```go
return result{}, toolsy.NewInternalError(fmt.Errorf("toolkit/prompts: get failed: %w", err))
```

Cooperative `ctx.Err()` checks inside long loops (document parsers, directory walks) **should** use `NewInternalError(fmt.Errorf("...: %w", ctx.Err()))` — this is expected, not legacy `errwrap`.

The core recognizes `context.Canceled` and `context.DeadlineExceeded` via `errors.Is` on the `%w` chain **before** treating the error as `CodeInternal`:

- **`wrapHandlerError`**, **`toolErrorFromExecutionErr`**, **`toolErrorFromExistingToolError`**, **`formatExecutionError`**, and **`RunCall`** classify interrupts as infra (agent must stop), not business Error-as-Value for LLM self-correction. Golden order (all classifiers): `context.Canceled` → `context.DeadlineExceeded` / `ErrTimeout` → `ErrReadLimitExceeded` → existing `ToolError`. `isContextInterrupt` includes `ErrTimeout` for batch/iter/outcome symmetry.
- **`NewErrorChunkFromErr(bare context.Canceled)`** returns an INTERNAL error chunk (not validation); `toolErrorFromExecutionErr` returns `nil` for bare cancel so the fallback switch builds the chunk.
- **`NewTimeoutErrorFrom`** maps deadline exceeded to `CodeTimeout` while preserving `context.DeadlineExceeded` in the unwrap chain.
- **`handleBatchToolError`**, **`Registry.ExecuteIter`**, and **`Session.ExecuteIter`** treat cancel and deadline symmetrically: no soft error chunk / no terminal `(Chunk{}, err)` yield for `isContextInterrupt` errors.

**Host / agent checklist:**

1. On tool failure, check `errors.Is(err, context.Canceled)` and `errors.Is(err, context.DeadlineExceeded)` — do not rely on `ExecutionError.Code` alone for interrupts.
2. After deadline mapping, `errors.Is(err, context.DeadlineExceeded)` remains valid when the error passed through `NewTimeoutErrorFrom`.
3. Do not add `PassThroughCtx` or inline `client.Do` cancel branches in toolkits — the core handles `%w` chains.

**Documented exceptions (do not wrap as `NewInternalError`):**

| Tier                  | Location                                                                  | Rationale                                                                                                                                                                                                                                         |
| --------------------- | ------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Library I/O primitive | `textprocessor/truncate.go` (`ReadLimitedBytes`)                          | Returns bare `ctx.Err()` / sentinel at primitive boundary                                                                                                                                                                                         |
| HTTP drain helper     | `toolkits/httptool/body.go` (`DrainResponseBody`)                         | Best-effort connection reuse; drain errors ignored                                                                                                                                                                                                |
| Transport parser      | `agents/sse.go` (`parseSSESteps`, `StreamSteps`)                          | Library SSE tier; golden order: `ctx.Err()` → `IsContextInterrupt(err)` → read-limit; `%w` propagate to proxy handlers                                                                                                                            |
| Sandbox adapter       | `adapters/sandbox/*` (host, docker, wazero, e2b, starlark)                | Adapter boundary; caps wrap `ErrSandboxFailure` + `ErrReadLimitExceeded`; `classify*Error` / `mapExecError` use `errors.Is` for ctx + `MapSandboxReadLimitError` for read-limit                                                                   |
| Exectool sandbox map  | `exectool/tool.go` `mapExecError`                                         | `errors.Is` at sandbox→tool boundary; deadline→`ErrTimeout`, cancel pass-through, read-limit→`CodeValidationFailed` with stream subject                                                                                                           |
| Registry batch remap  | `registry.go` `executeOneCall`                                            | `normalizeExecutionInterrupt`: bare cancel pass-through; deadline/`ErrTimeout` → `NewTimeoutErrorFrom`                                                                                                                                            |
| MCP transport         | `mcp/transport_sse.go`, `mcp/transport_stdio.go`                          | JSON-RPC stream boundary; `finish*CallResponse`, `streamLimitErr()`, and `call()` select: `ctx.Err()` → `IsContextInterrupt(err)` → read-limit                                                                                                    |
| MCP discovery         | `mcp/client.go` Initialize / ToolsList / PromptsList                      | `mapCallReadLimit(ctx, err)` — ctx/interrupt before `MapReadLimitError`                                                                                                                                                                           |
| Core registry/async   | `registry.go`, `async.go`                                                 | Yield/meta paths return bare `ctx.Err()` at orchestration boundary                                                                                                                                                                                |
| Codegen walk          | `internal/toolsygen/generator.go` (`checkContext`, `finishGeneratorRead`) | Generator internal cooperative cancel; post-read uses ctx-first + `IsContextInterrupt` like library tier                                                                                                                                          |
| E2B output cap        | `adapters/sandbox/e2b/e2b.go`                                             | `StartAndWait` streams into `CappedBuffer` writers; `FinishRun` checks overflow on success and failure paths                                                                                                                                      |
| Web semantic cap      | `toolkits/web/scraper.go` (`ErrMarkdownExceedsLimit`)                     | Markdown conversion tier after HTML read; tool mode maps via `MapToolkitCapError` (ctx-first)                                                                                                                                                     |
| RAG wire budget       | `toolkits/rag/cap.go`                                                     | Semantic wire cap after marshal; drops docs then `MapToolkitCapError` (ctx-first)                                                                                                                                                                 |
| Delegate step display | `agents/delegate.go` `formatStepOutput`                                   | `TruncateStringUTF8` after SSE step aggregation (256 KiB)                                                                                                                                                                                         |
| Memory count cap      | `toolkits/memory/memory.go`                                               | Item count limit, not I/O read tier                                                                                                                                                                                                               |
| DOCX zip entry cap    | `toolkits/document/parser_docx.go`                                        | Structural bomb guard (`maxZipEntries`), not byte-read                                                                                                                                                                                            |
| Stream byte budget    | `toolkits/httptool/body.go` `limitedStreamReader`                         | Partial bytes + `ErrReadLimitExceeded` on long-lived streams (SSE/stdio). Callers **must** re-check `ctx.Err()` after limit/`scanner.Err()` — MCP SSE/stdio readLoop, `agents/sse` `parseSSESteps`, MCP `call`/`getPostURL`/`waitSSECallResponse` |

| Starlark eval stderr  | `adapters/sandbox/starlark` adapter boundary                 | Eval stderr written via `CappedBuffer`; oversized eval errors fail closed at adapter boundary |

**E2B streaming cap:** `Session.StartAndWait` accepts `stdout`/`stderr` `io.Writer` parameters; the adapter passes `sandboxfs.NewCappedBuffer` and finalizes with `sandboxfs.FinishRun` on every exit code. Contract tests: `TestRunRejectsOversizedRemoteOutput`, `TestRunRejectsOversizedRemoteOutputStreaming`.

**Read-limit subject/maxBytes parsing:** `textprocessor.ReadLimitError` typed errors (preferred) with regex fallback in `textprocessor.ReadLimitSubject` / `ReadLimitMaxBytes` (used by `toolsy.MapReadLimitError`, `MapSandboxReadLimitError`, and `sandboxfs` helpers). `sandboxfs.CappedBuffer` emits typed `ReadLimitError` on overflow.

**MapSandboxReadLimitError tier:** maps only sandbox-formatted chains (parseable subject or maxBytes in error). Bare `ErrReadLimitExceeded` returns `nil` — callers use `MapReadLimitError(err, 0)` for generic/proxy/MCP paths.

**Toolkit read golden order:** `toolsy.ToolkitContextError`, `toolsy.MapToolkitReadError`, and `toolsy.MapToolkitCapError` enforce ctx interrupt before read-limit mapping on toolkit post-read, stat pre-check, and semantic/wire cap paths. `MapToolkitReadError` checks `ctx.Err()` via `ToolkitContextError`, then `toolsy.IsContextInterrupt(err)` on the read error chain, then read-limit validation.

**Docker setup classifier:** only the docker adapter exposes `classifySetupError` for archive/read-limit setup paths (materialize, wait, log collection). Host/wazero/starlark/e2b return raw `ErrSandboxFailure` on workspace setup — intentional adapter-specific contract; docker alone performs tar/archive bounded reads during setup.

**Starlark guest `fs.read` limit:** guest script read-limit surfaces as `exit_code:1` + stderr text from eval — not promoted to `CodeValidationFailed` at the exectool boundary (intentional guest-exit contract).

**RAG wire budget:** `toolkits/rag/cap.go` applies semantic JSON wire trim synchronously (no I/O); cancel asymmetry does not apply.

**MCP cancel-over-limit:** when a composite error carries both `ctx.Err()` and `ErrReadLimitExceeded`, MCP transport `finish*CallResponse`, `streamLimitErr()`, `call()` select branches, `mapCallReadLimit`, and `handleToolCallResult` return the interrupt — not `CodeValidationFailed`. Same rule for `agents/sse` `parseSSESteps` when `scanner.Err()` chains an interrupt before read-limit.

**Web/mail HTML convert:** `toolkits/web/scraper.go` and `toolkits/mail/mail.go` run bounded `htmltomarkdown.ConvertString` inline (no goroutine leak on ctx cancel); input is already capped upstream. **Intentional display-tier asymmetry:** on ctx cancel during HTML→markdown, **web** returns `NewInternalError(ctx.Err())` (fail-closed tool mode); **mail** returns the raw HTML body (fail-open display). Align only if product requires consistent tool-mode behavior.

**Library tier ctx-first:** `agents/client.CreateTask`, `contracts/openapi` spec fetch, and `contracts/graphql` introspection check `ctx.Err()` then `toolsy.IsContextInterrupt(err)` before mapping `ErrReadLimitExceeded`. Use `toolsy.IsContextInterrupt` in proxy/delegate/classify paths for consistent golden order (`ErrTimeout` included).

**Stat pre-check vs read expansion:** Local file tools (`document`, `fstool`) and PDF use `os.Stat` size guards before read; size exceed maps via `MapToolkitCapError` (ctx-first). Remaining edge: when `Stat` succeeds but `ctx` is canceled before `Open`/`ReadLimitedBytes`, cancel may still lose to a subsequent validation if work continues — narrow race between Stat and I/O. DOCX mitigates zip bombs via `UncompressedSize64` + `ReadLimitedBytes`. PDF uses synchronous `pdf.Open`, per-page extraction with a running byte budget, and early `remaining <= 0` guard before each page. A single page whose extracted text exceeds the remaining budget still allocates that page in memory (library limitation) — rejected fail-closed before returning partial document text. `internal/toolsygen` propagates generator `ctx` into `ReadLimitedBytes` for mid-read cancel and re-checks `ctx` before stat size validation.

Proxy handlers (`agents/delegate`, `mcp/client` tool **Execute**) must call `MapReadLimitError` with an explicit byte cap (`Client.maxStreamBytes()` / `maxSSEStreamBytes()`). MCP **list pagination** (`GetTools`, `GetPrompts`), **Initialize**, **GetResourceTool**, and **GetPrompt** map read-limit via `mapCallReadLimit(ctx, err)` when the transport returns `ErrReadLimitExceeded`.

## Migration checklist

1. Replace `textprocessor.ReadLimited` → `ReadLimitedBytes` (transport) or `ReadAndTruncate` (display).
2. Update `httptool.ReadBodyLimited` callers: expect `[]byte`; handle `ErrReadLimitExceeded`.
3. Use `errors.Is(err, textprocessor.ErrReadLimitExceeded)` instead of string matching on `"read exceeds"`.
4. For large web pages, pass `web.WithMaxPageBytes` or handle validation errors in the agent loop.
5. IoC / Engine items (Dual Mode, IoC formatters, RAG router, middleware `ArgsJSON`): verify against [migration-task29.md](migration-task29.md) — not reimplemented in task30.
6. Run tests: `go test -race ./textprocessor/... ./toolkits/... ./contracts/... ./agents/...`
