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
result := timetool.ComputeCurrent(loc)
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

## Configuration & Security

- **WithLocation(loc):** Sets the timezone for the "local" field in `time_current` and for **date arithmetic in `time_calculate`** (base_date is interpreted in this zone, then days/hours are added). Default is system local (`time.Local`); fallback to UTC if local is not available.
- **Date arithmetic:** DST-safe: addition is done in the configured location via `time.AddDate` and `time.Add`, so wall-clock time is preserved across DST boundaries.

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
