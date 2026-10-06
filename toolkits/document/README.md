# Toolsy document toolkit

Extract CSV, DOCX and explicitly enabled PDF text. A local input requires a host-selected source: `WithLocalRoot(root)` opens model `file_path` relative to a host-owned `*os.Root`; `WithLocalSource(open)` delegates authorization to a host port. Without either option local reads fail. The toolkit never interprets an arbitrary model path as permission. The host keeps the root alive until executions finish. Provider readers are closed by the toolkit and bounded before parsing into a private temporary snapshot. Root sources must be regular files; Unix opens reject FIFOs without waiting for a peer. On other platforms the host must exclude device paths. Blocking provider I/O and non-Unix filesystem I/O require host cancellation/isolation where hard deadlines are needed.

`ExtractWireResult` contains `text` and `source` (the input reference, including URL query). Custom formatters receive both; preserving provenance in their custom DTO is the host's responsibility. Extracted text is untrusted data, never privileged instructions.

Limits are independent: `WithMaxBytes` bounds final JSON bytes after escaping (default 2 MiB); `WithLimits(Limits{...})` bounds source bytes (2 MiB), expanded DOCX XML/text (2 MiB), CSV rows and columns / DOCX ZIP entries / PDF pages (1024 each), and individual CSV cells / XML text nodes / PDF page text (64 KiB). Zero selects the finite default; negative values fail construction. Oversize returns a validation error with no partial output, truncation marker or invented continuation token. These extractors do not support pagination. ZIP central-directory allocations are bounded by the source snapshot size; actual directory records are counted before metadata allocation (forged counts rejected; ZIP64 unsupported); only document.xml is decompressed, with an expanded byte cap. CSV formatting stays within the parsed text budget.

PDF is disabled by default. `WithInProcessPDF(true)` opts into the third-party parser: source, page count and returned text are checked, but page decoding, compressed streams, object graphs and parser CPU/allocations cannot be bounded by this parser API. Context is checked between pages, not during a page decode. Do not use this opt-in for hostile PDFs when hard memory/time isolation is required; parse in host-owned isolated workers instead. This toolkit does not implement an execution sandbox. Rejected PDFs never enter the parser.

Remote reads require `WithAllowRemote(true)` and retain DNS pinning/private-IP validation across redirects through httptool. There is no document-specific host blacklist. `WithHTTPClient` merges only Timeout onto the safe transport; custom transports are not trusted. Private-IP override is host-controlled and intended for tests.

Remote GET redirects must remain within the original scheme/hostname/effective-port origin, including when private IPs are explicitly allowed. Configure the final document URL explicitly. Redirect refusal exposes `*httptool.RedirectError` with outer nonretryable `CodeRemoteExecution` and does not authorize argument correction or blind retry. Initial URL validation remains an input-validation error.

```go
root, err := os.OpenRoot("/srv/documents")
if err != nil { panic(err) }
defer root.Close() // after all tool executions
extract, err := document.AsTool(document.WithLocalRoot(root))
// Input: {"file_path":"reports/month.csv"}
```

Clear break: default unrestricted local access is removed; result adds source; PDF parsing requires explicit opt-in; parser budgets no longer derive from wire-envelope estimates. Result formatter/validator ports remain domain-independent. Dependencies: core toolsy, httptool, ledongthuc/pdf. No artifact store or backend domain model is introduced.
