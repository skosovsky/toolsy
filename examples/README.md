# Runnable host examples

Commands below run from the repository root on the source checkout. Root examples
use the core module; nested-module examples are marked separately. Add host policy,
identity and approval where external effects require them; examples do not grant
authority or promise production infrastructure guarantees.

| Example | Command | Purpose |
| --- | --- | --- |
| [run_call](run_call/main.go) | `go run ./examples/run_call` | Typed RunCall/ToolOutcome and validation errors |
| [calculator](calculator/main.go) | `go run ./examples/calculator` | Minimal typed result decoding |
| [session_snapshot](session_snapshot/main.go) | `go run ./examples/session_snapshot` | State codecs and state+binding snapshot, not workflow continuation |
| [approval_journal](approval_journal/README.md) | `go run ./examples/approval_journal -directory "$fixtureDir" -operation first-intent` | Bound operation approval, completed replay and unknown outcome |
| [contract_recovery](contract_recovery/main.go) | `go run ./examples/contract_recovery` | Post-handler contract failure is not argument repair authority |
| [host_event](host_event/main.go) | `go run ./examples/host_event` | Generic trusted host event with pause semantics |
| [stream_terminal](stream_terminal/main.go) | `go run ./examples/stream_terminal` | Progress versus terminal/error buffering |
| [streaming](streaming/main.go) | `go run ./examples/streaming` | Low-level synchronous chunk consumption |
| [full_agent](full_agent/main.go) | `go run ./examples/full_agent` | Local catalog, call parsing and batch chunk delivery |
| [nested_contract](nested_contract/main.go) | `go run ./examples/nested_contract` | Executable nested typed schema without JSON-string workaround |
| [generated_presence](generated_presence/README.md) | `go run ./examples/generated_presence` | Complete manifest/CLI/DTO/handler and presence semantics |
| [generated_stream](generated_stream/README.md) | `go run ./examples/generated_stream` | Generated synchronous stream plus explicit host async/callback/shutdown |
| [resiliency](resiliency/main.go) | `go -C examples/resiliency run .` | Nested module: external routery wrapper and host retry decisions |

The approval example needs an existing absolute directory owned by the host.
For a disposable local demonstration, set `fixtureDir="$(mktemp -d)"`, then use
the command above. It reports pending approval without an external receipt;
follow [the approval recipe](approval_journal/README.md) for explicit local approval
and replay. Never use a shared or untrusted directory.

Generate the two public examples before running them:

```sh
go run ./cmd/toolsy-gen ./examples/generated_presence/presence.json
go run ./cmd/toolsy-gen ./examples/generated_stream/progress.yaml
```

Use [generated_presence](generated_presence/README.md) for complete ordinary
manifest/CLI/handler setup, and [generated_stream](generated_stream/README.md) for
synchronous versus async ownership. Run `go test -race ./examples/...` for root
example fixtures; the nested resiliency module needs its own `go test` or `make test`.

## Nested toolkit recipes

| Recipe | Command | Host boundary |
| --- | --- | --- |
| [RAG routing/fallback](../toolkits/rag/examples/host/main.go) | `go -C toolkits/rag run ./examples/host` | Host strategy, no hidden retries |
| [SQL authority](../toolkits/sqltool/examples/host/main.go) | `go -C toolkits/sqltool run ./examples/host` | Restricted SQLite connection, lexical filter not authorization |
| [Timezone store](../toolkits/timetool/examples/host/main.go) | `go -C toolkits/timetool run ./examples/host` | Explicit bounded host StateStore resolver |

Current contracts, module installation/alignment and migration links are in the
[documentation index](../docs/README.md). Old task audit reports are indexed
[separately](../docs/history/README.md).
