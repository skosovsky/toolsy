# Toolsy: Time Toolkit (timetool)

**Description:** Gives the agent a sense of time: current time (UTC and local) and DST-safe date arithmetic (add days/hours) so the LLM does not hallucinate dates.

## Installation

```bash
go get github.com/skosovsky/toolsy/toolkits/timetool
```

**Dependencies:** stdlib only (`time`); requires `github.com/skosovsky/toolsy` (core).

## Available tools

| Tool            | Description                                      | Input                                                                 |
|-----------------|--------------------------------------------------|-----------------------------------------------------------------------|
| `time_current`  | Get current time in UTC and local with weekday   | `{}`                                                                  |
| `time_calculate`| Add days or hours to a date (DST-safe), RFC3339  | `{"base_date": "string", "add_days": int, "add_hours": int}`          |

Result: `time_current` returns `{"utc": "...", "local": "...", "weekday": "...", "unix": N}`. `time_calculate` returns `{"result": "RFC3339", "weekday": "..."}`.

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
	reg := toolsy.NewRegistry()

	tools, err := timetool.AsTools()
	if err != nil {
		panic(err)
	}
	for _, tool := range tools {
		reg.Register(tool)
	}
}
```
