# Generated input presence

This local, effect-free example starts with a complete [manifest](presence.json),
generates [DTO/handler/factory](presence_gen.go), supplies a [handler](main.go),
and executes the tool through its public API. The
[normative table](../../docs/generator-contract.md#normative-dto-mapping-and-presence)
is the reference for required/optional fields, nullable unions and array items.

From the repository root:

```sh
go run ./cmd/toolsy-gen ./examples/generated_presence/presence.json
go run ./examples/generated_presence
go test -race ./examples/generated_presence
```

Alternatively, `go generate ./examples/generated_presence` invokes the same local
CLI. Generate only the intended manifest path; a directory input recursively scans
manifests there. In your own module, install the CLI from the same chosen release
as core (`go install github.com/skosovsky/toolsy/cmd/toolsy-gen@vX.Y.Z`), then run
`toolsy-gen /path/to/presence.json`. Replace the version placeholder explicitly;
no release is published by this example.

Expected output:

```text
text="" integer=9007199254740993 active=false tags=0 optionalTextOmitted=true nullable=null
```

The manifest intentionally contains all supported top-level type/presence variants;
required fields are supplied even when empty/zero. Nullable required fields use
explicit null. Optional fields remain absent; the schema's optional string default
is metadata and is not inserted. The public tests assert omitted versus explicit
null, empty slices versus nil, false/zero pointers and exact integer item lexemes.
They also prove invalid omission/null/item types never dispatch the handler.
`RawJSON` is the authoritative accepted argument representation, including allowed
unknown keys; serializing a DTO again can change presence.

Factories have no hidden authority or automatic retry. Add current host policy,
identity and approval at the normal execution boundary. This example has no
external effects and uses synchronous execution. For streaming and explicit host
async completion, see [generated_stream](../generated_stream/README.md).
