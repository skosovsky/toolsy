# Optional host integration module

Real imports of the prompt caller, continuation runtime and guard decisions are
confined to this Go module. Toolsy core has no dependency on them. The offline
provider returns real native call parts; all tool, approval, continuation and
recovery state machines are real public APIs, not imitations.

From the repository root, with explicit sibling checkout paths:

```sh
PROMPTY_DIR=/path/to/prompty FLOWY_DIR=/path/to/flowy GUARDY_DIR=/path/to/guardy \
  scripts/task36-integration.sh local
scripts/task36-integration.sh published
scripts/task36-integration.sh released ROOT_TAG
```

Local mode creates a temporary workspace with exactly the passed checkouts, the
candidate root and this consumer. Published mode runs with GOWORK=off and no
replace: before the recipe is published, it copies only candidate host application
code into the temporary consumer and imports published engine/consumer modules.
It never copies engine code or patches external libraries. Released mode imports
the actual published recipe from the supplied root tag and runs the same semantic
fixtures without copying it. Dependency failures are failures, never skipped tests.
The CI workflow runs local and published modes, plus released mode on root tags.

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
