# toolsy-gen input contract

The manifest `parameters` object is the sole validation contract. Generated wrappers
publish that exact schema and delegate all validation to `NewProxyTool` and the
shared bounded `internal/jsonschemax` engine. Generated DTOs do not contain a
second `Validate` method. `required` means key presence, never nonempty values.

Supported dialect: JSON Schema 2020-12 (implicit or explicit). The root must be
an object with declared properties. Every top-level property requires a nonempty description. Properties support string, integer, boolean,
and arrays of those primitives. A top-level property may declare a two-member
type union of one supported type and `null`; it is represented by
`json.RawMessage`, distinguishing omitted (`nil`), null (`null` bytes), and value.
Nested objects, nested arrays, references, composition and unknown keywords are
rejected during generation. Keyword support is explicit: type, description,
properties, required, additionalProperties (boolean only), items, enum, const,
default, examples, title, string minLength/maxLength/pattern/format (date-time),
integer minimum/maximum/exclusiveMinimum/exclusiveMaximum/multipleOf,
array minItems/maxItems/uniqueItems and root minProperties/maxProperties.
Keywords must occur at their applicable schema locations. Defaults are metadata;
they are never inserted into omitted arguments.

## Normative DTO mapping and presence

This table describes the current generator. Required/optional refers to membership
in the root `required` list. The source schema validates presence and nullability
before a generated handler receives the DTO; Go pointer/slice shapes do not
replace that validation. Empty/zero values are accepted unless source constraints
reject them. Date-time remains a schema annotation on string, with no separate Go
decoder constraint. Defaults are never applied.

| Schema location/type | Required top-level field | Optional top-level field | Accepted value/presence |
| --- | --- | --- | --- |
| Root `object` | Named `<Name>Input` struct | Not applicable | Object required; no nullable/scalar/array root; includes `RawJSON` |
| `string`, also `format: date-time` | `string` | `*string` | Required omission rejected; optional omitted is nil; present string retained; nonnullable null rejected |
| `integer` | `*json.Number` | `*json.Number` | Required omission rejected; optional omitted is nil; present zero/nonzero number is nonnil and exact lexeme retained; null rejected |
| `boolean` | `*bool` | `*bool` | Required omission rejected; optional omitted is nil; present false is nonnil; null rejected |
| `array` of supported primitives | `*[]T` | `[]T` | Required omission rejected; present `[]` gives nonnil pointer/empty slice; optional omitted slice nil, present `[]` nonnil empty; null rejected |
| Top-level `[supported type, null]` union, including arrays | `json.RawMessage` | `json.RawMessage` | Required omission rejected; optional omission nil; explicit null is bytes `null`; concrete value retains JSON value and numeric lexemes |
| Array item `string`/date-time | `T = string` | Same | Value items; nullable items unsupported |
| Array item `integer` | `T = json.Number` | Same | Exact numeric lexeme; null rejected |
| Array item `boolean` | `T = bool` | Same | Value false retained; null rejected |
| Nested object/array, nullable array item, standalone null or other unions | Unsupported | Unsupported | Generation fails before output installation |

Both union orders are accepted (`["null", "string"]` or `["string", "null"]`);
exactly one supported concrete type plus null is allowed, at top-level properties
only. Integer JSON values must be mathematically integral according to the shared
schema validator (for example `1.0` can validate); `json.Number` retains the source
lexeme without promising `Int64` conversion for arbitrarily large values. Hosts
choose their domain conversion and handle conversion errors.

Every generated input includes `RawJSON` (excluded from JSON encoding), retaining
the complete accepted argument, including undeclared properties when allowed.
Nullable fields preserve omission/null/value and numeric lexemes; whitespace
inside decoded nullable values can normalize. `RawJSON` also permits complete
presence inspection. DTO re-encoding is not an authoritative argument snapshot:
optional slices with `omitempty` can omit a present empty array, and optional raw
nullable fields have no `omitempty` and can encode omitted nil as null. Use the
validated original `RawJSON` when exact argument presence matters.

Generated field name `RawJSON` is reserved. Property names must be representable
exactly by Go JSON tags: commas, quotes, backticks, backslashes, control characters
and other unsupported tag symbols are rejected. Unknown input properties follow
the source `additionalProperties` policy and are not added as DTO fields.

The checked-in [presence manifest](../examples/generated_presence/presence.json),
[generated DTO/factory](../examples/generated_presence/presence_gen.go) and
[public execution fixtures](../examples/generated_presence/main_test.go) compile
all table mappings and exercise omission/null/empty/zero/items/exact numbers.
See [the complete CLI/handler example](../examples/generated_presence/README.md).

## Schema subset and nested inputs

Nested executable schemas belong in `NewTool`/`NewTypedTool` with BYO nested Go
types, or `NewDynamicToolFromSpec` with an explicit supported schema. Keep the object in
its input contract so validation occurs before handler dispatch. Do not turn a
structured object into JSON text inside a string to bypass generator limits;
that changes the visible schema and shifts validation past the input boundary.
The bounded flat generator subset remains unchanged. The runnable
[nested typed example](../examples/nested_contract/main.go) and its public tests
show an actual nested object and rejection before dispatch; run
`go run ./examples/nested_contract` from the checkout root.

Normative acceptance fixtures: required `""`, `[]`, `0`, `false` are accepted;
omitted required keys and nonnullable null are rejected; nullable explicit null
is accepted and remains distinguishable from omission. Adjacent minLength and
minItems constraints reject empty values. Unknown schema keywords and unsupported
nested schemas fail before any generated file is written. Limits are inherited
from the shared JSON parser/compiler (depth 128, 100,000 nodes) and generator file
byte cap. YAML accepts only JSON-compatible scalar tags, string mapping keys and finite numbers; aliases and custom tags are rejected. Numeric lexemes are preserved without float64 conversion. No external references are resolved. Handler and credentials remain
host responsibilities; a generated tool grants no business authority.


## Streaming and host async composition

stream:true generates a synchronous proxy with ExecuteStream iter.Seq2 handler.
All but the final successful part emit progress; the final part is a result. Empty
successful iteration emits an empty terminal result. Handler errors propagate with
any prior buffered part as progress, never as terminal success. Consumer errors stop
iteration; context checkpoints surround emitted parts and terminal completion.
The handler must cooperatively return when context is done; no iterator goroutine
is abandoned to pretend hard preemption. Invalid arguments fail before dispatch.

The host can wrap a generated tool using AsAsyncTool and explicitly configure
background timeout, collected-chunk cap and completion callback. The accepted chunk
acknowledges scheduling; terminal result/errors (including invalid args) belong to
WithOnComplete, not the caller's already-returned Execute. Existing async semantics
detach parent cancellation; configured timeout bounds cooperative background work.
Registry tracks accepted jobs and Shutdown waits for them. See the compiling/runnable
examples/generated_stream sample and generated-module lifecycle acceptance fixtures.

## Filesystem commit and recovery

The generator stages changed outputs before installation. Every finalize error or
observed cancellation rolls back installed targets and a current moved backup using
one best-effort recovery path. Earlier owned staging files are disposed on staging
failure. Primary, rollback and cleanup causes aggregate through errors.Join; typed
FileRecoveryError records target, backup/artifact path and failed action. Backup paths
are observations at failure: subsequent successful rollback may already have restored
them. Failed restore/remove diagnostics preserve recovery backups for host inspection.
Never interpret a leftover backup as automatically safe to overwrite a current target.

Once every output is installed and the final context check succeeds, backup disposal
is post-commit cleanup. FileRecoveryError.CommitComplete=true distinguishes those
errors; Generate returns the complete Files list alongside that diagnostic because
new outputs remain installed. Disposal failures aggregate across files, retaining
undeleted backups. No rollback is claimed after earlier backups were disposed.

Target paths require host exclusive-writer coordination during the run. File syscalls
remain synchronous and best-effort cleanup cannot guarantee crash-atomic multi-file
transactions, forced syscall interruption, concurrent-writer protection or recovery
from arbitrary external mutation. Process crashes can leave owned artifacts; use the
reported paths and actual filesystem state for recovery rather than blind retries.
