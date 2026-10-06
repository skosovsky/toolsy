# Toolsy: Time Toolkit (timetool)

**Description:** Current time (UTC and host local) and checked arithmetic: calendar days followed by elapsed hours.

## Installation

```bash
go get github.com/skosovsky/toolsy/toolkits/timetool
```

**Dependencies:** stdlib only (`time`); requires `github.com/skosovsky/toolsy` (core).

## Available tools

| Tool             | Description                                     | Input                                                        |
| ---------------- | ----------------------------------------------- | ------------------------------------------------------------ |
| `time_current`   | Get current time in UTC and local with weekday  | `{}`                                                         |
| `time_calculate` | Add calendar days then elapsed hours, RFC3339 | `{"base_date": "string", "add_days": int, "add_hours": int}` |

Result: `time_current` returns `{"utc": "...", "local": "...", "weekday": "...", "unix": N}`. `time_calculate` returns `{"result": "RFC3339", "weekday": "..."}`.

## Library mode

```go
loc, _ := time.LoadLocation("Europe/Moscow")
result, err := timetool.ComputeCurrent(loc)
if err != nil { /* location cannot represent an RFC3339 instant */ }
// result.UTC, result.Local, result.Weekday, result.Unix
```

## Host location and StateStore

`WithLocation` sets the local display zone **and** calculate's calendar zone.
`WithLocationProvider` resolves a zone for each invocation of either tool and
has priority over the static location. A provider error or nil location fails
without static fallback. Passing a nil provider disables that override.

Run `go run ./examples/host` from this module for a [complete host example](examples/host/main.go).
It reads a host-defined timezone key through `env.StateStore`, checks a finite
returned-byte budget, resolves a host-selected zone and supplies it explicitly.
The fixture store is local and immutable; it does not claim durable persistence.
The toolkit does not select a timezone key or automatically read session state.
Host callbacks own storage authorization, upstream bounds and context cooperation.

`RunEnv.StateStore` is the borrowed `Load`/`Save` persistence port supplied with
`toolsy.WithStateStore`. `GetSessionState`/`SetSessionState` use the separate
in-memory Session map. `SessionSnapshot` excludes external StateStore contents.
The memory toolkit uses this same explicit `run.StateStore` port for its scratchpad.

## Time and bounds contract

Both tools use one host location resolver. `WithLocationProvider` overrides `WithLocation` on every invocation (including calculate); provider errors and `(nil, nil)` are returned as errors, never passed to `time.In`. The default location is `time.Local`, falling back to UTC. `ComputeCurrent(nil)` is a separate library convenience and uses UTC.

`base_date` uses the strict RFC3339 subset supported by Go calendar validation: four-digit year, two-digit time fields, uppercase T/Z, dot fractional separator with 1–9 fractional digits, and numeric offsets below 24 hours with minutes below 60. Leap seconds are unsupported; excess fractional precision is rejected rather than discarded. Valid numeric zero offsets and trailing-zero fractions are accepted. `base_date` is an RFC3339 instant: its explicit numeric offset determines the instant, rather than being replaced by the host zone. Calculation first converts that instant to the resolved host zone, then applies calendar `add_days` with `time.AddDate`, then elapsed `add_hours` with `time.Add`. Days preserve calendar wall time across ordinary DST transitions; hours are exact elapsed durations and can change wall time. For nonexistent or ambiguous local times `AddDate` follows Go's timezone normalization (the choice of offset in an ambiguity is not guaranteed). The result uses the host zone's offset at the resulting instant. RFC3339Nano retains fractional seconds. Historical offsets containing seconds and fixed offsets outside ±23:59 cannot represent the same instant in RFC3339 and return validation errors in both tools and `ComputeCurrent`.

Hour duration multiplication is checked before conversion to `time.Duration`; calendar input is bounded and calendar/intermediate/final results must stay within RFC3339 years 0000–9999. Overflow returns a validation error instead of wraparound. `base_date` is limited to 64 bytes. There is no collection, source reader, or continuation interface.

`WithMaxWireBytes(n)` bounds both default and host-formatted complete JSON after escaping. Default: 64 KiB; zero selects the finite default; negative values reject construction. Overlimit output is rejected with no truncation, including multibyte and escaped strings. Host formatters remain responsible for their own computation/allocation bounds and business DTOs; the wire limit does not bound arbitrary callback allocations. Host validators run before marshaling. The tools are read-only and confer no authority for subsequent actions.

## DST examples

For America/New_York under the timezone rules used by Go:

| Base instant in local time | Delta | Result in local time | Elapsed |
| --- | --- | --- | --- |
| 2026-03-07 12:00 -05:00 | 1 calendar day | 2026-03-08 12:00 -04:00 | 23 hours |
| 2026-03-07 12:00 -05:00 | 24 elapsed hours | 2026-03-08 13:00 -04:00 | 24 hours |
| 2026-10-31 12:00 -04:00 | 1 calendar day | 2026-11-01 12:00 -05:00 | 25 hours |
| 2026-10-31 12:00 -04:00 | 24 elapsed hours | 2026-11-01 11:00 -05:00 | 24 hours |

Order matters: 2026-03-07 01:30 -05:00 plus one day then two hours yields
2026-03-08 04:30 -04:00. Adding hours before the day would yield 03:30 instead.
Rules come from the host/embedded Go timezone database, not a universal fixed DST
schedule. The tests and host example use embedded stdlib tzdata for availability.

Nil options reject construction. Host ports and callbacks are borrowed; the host
owns their lifetime and synchronization. See the [shared constructor and ownership
contract](../README.md#constructor-configuration-and-ownership) for option snapshots
and the distinction between configuration containers and mutable host ports.
