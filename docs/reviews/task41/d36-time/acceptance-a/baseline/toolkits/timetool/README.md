# Toolsy: Time Toolkit (timetool)

**Description:** Gives the agent a sense of time: current time (UTC and local) and DST-safe date arithmetic (add days/hours) so the LLM does not hallucinate dates.

## Installation

```bash
go get github.com/skosovsky/toolsy/toolkits/timetool
```

**Dependencies:** stdlib only (`time`); requires `github.com/skosovsky/toolsy` (core).

## Available tools

| Tool             | Description                                     | Input                                                        |
| ---------------- | ----------------------------------------------- | ------------------------------------------------------------ |
| `time_current`   | Get current time in UTC and local with weekday  | `{}`                                                         |
| `time_calculate` | Add days or hours to a date (DST-safe), RFC3339 | `{"base_date": "string", "add_days": int, "add_hours": int}` |

Result: `time_current` returns `{"utc": "...", "local": "...", "weekday": "...", "unix": N}`. `time_calculate` returns `{"result": "RFC3339", "weekday": "..."}`.

## Library mode

```go
loc, _ := time.LoadLocation("Europe/Moscow")
result, err := timetool.ComputeCurrent(loc)
if err != nil { /* location cannot represent an RFC3339 instant */ }
// result.UTC, result.Local, result.Weekday, result.Unix
```

## IoC (host customization)

```go
	timetool.AsTools(
		timetool.WithLocationProvider(func(ctx context.Context, env *toolsy.RunEnv) (*time.Location, error) {
			// resolve timezone from session state
			return time.LoadLocation("Europe/Moscow")
		}),
		timetool.WithResultFormatter(func(r timetool.CurrentResult) (any, error) {
			return map[string]any{"server_time": r.UTC}, nil
		}),
		timetool.WithCalculateResultFormatter(func(r timetool.CalculateResult) (any, error) {
			return map[string]any{"when": r.Result}, nil
		}),
		timetool.WithHostResultValidator(func(v any) error { return nil }),
	)
```

## Time and bounds contract

Both tools use one host location resolver. `WithLocationProvider` overrides `WithLocation` on every invocation (including calculate); provider errors and `(nil, nil)` are returned as errors, never passed to `time.In`. The default location is `time.Local`, falling back to UTC. `ComputeCurrent(nil)` is a separate library convenience and uses UTC.

`base_date` uses the strict RFC3339 subset supported by Go calendar validation: four-digit year, two-digit time fields, uppercase T/Z, dot fractional separator with 1–9 fractional digits, and numeric offsets below 24 hours with minutes below 60. Leap seconds are unsupported; excess fractional precision is rejected rather than discarded. Valid numeric zero offsets and trailing-zero fractions are accepted. `base_date` is an RFC3339 instant: its explicit numeric offset determines the instant, rather than being replaced by the host zone. Calculation first converts that instant to the resolved host zone, then applies calendar `add_days` with `time.AddDate`, then elapsed `add_hours` with `time.Add`. Days preserve calendar wall time across ordinary DST transitions; hours are exact elapsed durations and can change wall time. For nonexistent or ambiguous local times `AddDate` follows Go's timezone normalization (the choice of offset in an ambiguity is not guaranteed). The result uses the host zone's offset at the resulting instant. RFC3339Nano retains fractional seconds. Historical offsets containing seconds and fixed offsets outside ±23:59 cannot represent the same instant in RFC3339 and return validation errors in both tools and `ComputeCurrent`.

Hour duration multiplication is checked before conversion to `time.Duration`; calendar input is bounded and calendar/intermediate/final results must stay within RFC3339 years 0000–9999. Overflow returns a validation error instead of wraparound. `base_date` is limited to 64 bytes. There is no collection, source reader, or continuation interface.

`WithMaxWireBytes(n)` bounds both default and host-formatted complete JSON after escaping. Default: 64 KiB; zero selects the finite default; negative values reject construction. Overlimit output is rejected with no truncation, including multibyte and escaped strings. Host formatters remain responsible for their own computation/allocation bounds and business DTOs; the wire limit does not bound arbitrary callback allocations. Host validators run before marshaling. The tools are read-only and confer no authority for subsequent actions.

## Quick start

```go
package main

import (
	"github.com/skosovsky/toolsy"
	"github.com/skosovsky/toolsy/toolkits/timetool"
)

func main() {
	builder := toolsy.NewRegistryBuilder()

	tools, err := timetool.AsTools()
	if err != nil {
		panic(err)
	}
	for _, tool := range tools {
		builder.Add(tool)
	}
}
```


Nil options reject construction. Host ports and callbacks are borrowed; the host
owns their lifetime and synchronization. See the [shared constructor and ownership
contract](../README.md#constructor-configuration-and-ownership) for option snapshots
and the distinction between configuration containers and mutable host ports.
