# Toolsy toolkits

Eleven independent Go modules adapt host capabilities into tools. Providers,
connections, business DTOs, credentials, permissions and execution policy stay
with the host. Import only the modules the application needs.

| Module | Access and effect boundary | Limits and continuation |
| --- | --- | --- |
| [fstool](fstool/README.md) | Host-selected `os.Root`; root-relative access; exact write content | Source, wire, entry/name/scan bounds; real byte ranges and directory offsets, no snapshot promise |
| [mail](mail/README.md) | Host reader/sender; outgoing body unchanged or rejected before Send | Body, source, item, count and wire bounds; no backend cursor |
| [rag](rag/README.md) | Host DTO → retrieval unit → formatter; explicit source and unit identity | Provider count/item/source and actual wire bounds; no invented cursor |
| [web](web/README.md) | Host search provider; SSRF-safe scrape; extracted content is untrusted data | Provider and parser source/item/count/wire bounds; scrape retains final source URL |
| [document](document/README.md) | Local access disabled until host provides root/source; remote opt-in | Separate source/parser/item/count/wire bounds; PDF explicit opt-in, no hard in-process parser quota |
| [sqltool](sqltool/README.md) | Host `database/sql` connection and DB role; inspect allowlist is not execute ACL | Rows/cells/columns/tables/source/wire bounds; display truncation explicitly flagged, no fabricated pagination |
| [memory](memory/README.md) | Host session store; one writer through one toolkit instance | Finite facts/key/value/store/wire bounds; rejects over-budget state, never silently evicts |
| [prompts](prompts/README.md) | Trusted host provider returns instructions with optional source/version | Finite source/instructions/provenance/wire bounds; instructions unchanged or rejected |
| [httptool](httptool/README.md) | Allowed domains authorize egress; explicit exact origins authorize credentials | Request/read/wire bounds; cross-origin redirects cannot inherit credentials |
| [timetool](timetool/README.md) | One host location resolver; calendar days distinct from elapsed hours | Checked arithmetic and finite wire bounds, including host formatter output |
| [human](human/README.md) | Conversation pause is UX; actual action requires a bound host grant | Finite complete JSON pause payload, no text truncation or authorization fallback |

The module README is the executable contract's companion: it defines finite
defaults, host overrides, zero/negative semantics, DTOs and supported backend
capabilities. Bounds check actual returned data, including providers which ignore
requested limits. They cannot constrain allocations inside arbitrary host
callbacks, database drivers or third-party parsers; those boundaries are stated
explicitly. Hard CPU/memory isolation belongs in host-owned workers.

## Host output DTOs

`timetool`, `web`, `rag`, `sqltool` and `document` expose formatter and validator
ports. Formatters receive bounded module DTOs and return host-owned DTOs. When
both ports are configured, the validator sees the formatter output. The complete
serialized JSON is then checked against the wire budget, including escaping and
custom output. Oversize returns a validation error, never sliced JSON. A compact
host DTO may fit even when the module's default representation would not.

Web scrape formatters receive `ScrapeWireResult`, including `source_url`. RAG
formatters own provenance preservation for their custom DTOs. Source access and
retrieval content do not confer instruction authority. See
[result contracts](../docs/result-contract.md) for validation and delivery.

## Source bounds and side effects

Source reads, collection counts, item sizes and final wire sizes are separate
budgets. Explicit SQL display truncation differs from a source or final-wire
error. Mail, HTTP POST and filesystem writes do not shorten action arguments.
A response-limit failure after an external action does not roll that action back;
compose the core operation profile for bound approval, durable state and replay.
Confirmation hints do not replace host authentication or business policy.

Filesystem offsets are actual backend positions. Other injected ports do not
promise continuation and return explicit errors rather than inventing tokens.
Hosts can add their own backend-specific adapters without a universal artifact
store or a shared domain model.

## HTTP and execution policy

HTTP consumers reuse `httptool` safe transport with DNS/IP checks and pinning.
Private addresses require explicit host configuration. Toolkit credential origin
bindings are separate from egress allowlists; service auth in other protocol
adapters remains their own host contract.

Register tools with `NewRegistryBuilder().Add(tools...)`. Hosts provide deadlines,
permissions, scheduling, retries and external quotas. See
[resiliency](../examples/resiliency/main.go) and
[bound approval](human/bound_approval_test.go). No toolkit implements an agent
loop, business authorization service, prompt repository or distributed database.


## Constructor configuration and ownership

Every toolkit constructor rejects a nil `Option` with an error, including cleanup
wrappers and web library functions. Options apply in order; the last value wins.
Negative limits reject construction. Zero selects the documented finite default,
except human's explicit `WithMaxPayloadBytes(0)`, which remains an error because
that API requires a positive control-payload budget. Invalid configuration returns
nil constructed objects and nil cleanup functions, without calling host providers.
`memory.NewScratchpad` now returns `(*Scratchpad, error)`; `AsTools` still checks
whether a positive output budget can encode mandatory status responses.

Security containers are snapshots at option creation and independent copies at
each application: HTTP allowed hosts, credential origins and metadata headers,
web blocked hosts, and SQL schema-inspection table filters. Mutations after the
`With...` call do not affect the option or constructed tools. Do not mutate input
containers concurrently while creating the snapshot. SQL table filters restrict
schema inspection only; database privileges remain the query authority.

Providers, databases, root handles, stores, custom scrapers, functions and state
captured by closures are borrowed, not cloned. Keep them alive for executions and
provide safe synchronization for concurrent calls, or serialize calls in the host.
Snapshots do not make mutable provider internals thread-safe. Returned host data
must remain stable for toolkit processing; ownership rules for specific data are
in each module's contract. The HTTP pool's TLS configuration is cloned; referenced
certificates, trust roots and callback state must remain immutable for its lifetime.
