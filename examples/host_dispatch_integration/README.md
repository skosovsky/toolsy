# Optional host integration module

Real imports of the prompt caller, continuation runtime and guard decisions are
confined to this Go module. Toolsy core has no dependency on them. The offline
provider returns real native call parts; all tool, approval, continuation and
recovery state machines are real public APIs, not imitations.

From the repository root:

```sh
make test-integration
```

Or directly from this module:

```sh
GOWORK=off go test -race -count=1 -tags=integration -run '^TestIntegration' ./...
```

The module uses pinned prompty/flowy/guardy dependencies and a local root Toolsy
replacement. Standard release preparation removes that replacement and aligns
internal requirements. Ordinary tests compile the example; the integration
profile executes its semantic fixtures. Dependency failures fail the profile.

`Prompt` serializes source schemas from the same restricted view; it does not infer
or translate tags into authorization. `Requests` copies provider identity and raw
JSON into a host-issued intent; host policy/subject/scope never comes from model
text. `ModelPart` receives only the filtered model projection. `GuardDecision`
preserves canonical fault/deny/correction without scheduling retries.

The durable activity fixture loses delivery after tool outcome persistence,
reopens the local journal, inspects it and replays through current authorized
Session. OperationID, provider CallID and activity identity remain independent.
Continuation storage is in-memory test storage; no production DB or distributed
crash guarantees are asserted. Live providers and remote authenticated approval
protocols are outside this offline fixture.
