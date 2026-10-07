# Generated stream and explicit async host

From the repository root:

```sh
go run ./cmd/toolsy-gen ./examples/generated_stream/progress.yaml
go run ./examples/generated_stream
```

The [manifest](progress.yaml) generates an ordinary synchronous
[stream factory](progress_gen.go). The [host](main.go) explicitly wraps it with
`AsAsyncTool`, a background timeout, collection cap and completion callback, then
waits for completion and shuts down the registry. The returned accepted chunk is
scheduling acknowledgement; terminal output/error belongs to the callback.
The generated tool alone remains synchronous and caller cancellation attached.

The shared [generator contract](../../docs/generator-contract.md#streaming-and-host-async-composition)
describes progress/final buffering, errors, consumer stop and cooperative handlers.
The generated lifecycle fixtures in `internal/toolsygen/generator_test.go` compile
consumer modules and cover synchronous terminal/error/cancel and explicit async
completion. Unit coverage does not imply hard preemption of a blocking iterator.
