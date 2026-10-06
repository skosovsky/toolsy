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

Integers use `json.Number` to preserve the original integer without int64 overflow.
Date-time uses string: its annotation does not add an independent Go decoder
constraint. Optional primitives use pointers, arrays preserve nil versus empty.
Every generated input includes `RawJSON` (excluded from JSON encoding) retaining
the complete accepted argument, including undeclared properties when allowed.
Generated field name `RawJSON` is reserved. Property names must be representable exactly by Go JSON tags: commas, quotes, backticks, backslashes, control characters and other unsupported tag symbols are rejected. Nullable fields and raw arguments
permit the host to inspect exact presence without conversion loss.

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
