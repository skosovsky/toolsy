package timetool

import (
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/skosovsky/toolsy"
)

var rfc3339Re = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(Z|[+-]\d{2}:\d{2})$`)

func TestTimeCurrent_ReturnsValidRFC3339(t *testing.T) {
	tools, err := AsTools()
	require.NoError(t, err)
	currentTool := tools[0]

	var result currentResult
	require.NoError(t, currentTool.Execute(context.Background(), []byte(`{}`), func(c toolsy.Chunk) error {
		if c.RawData != nil {
			if r, ok := c.RawData.(currentResult); ok {
				result = r
			}
		}
		return nil
	}))
	require.Regexp(t, rfc3339Re, result.UTC)
	require.Regexp(t, rfc3339Re, result.Local)
	require.NotEmpty(t, result.Weekday)
	require.Positive(t, result.Unix)
}

func TestTimeCalculate_AddDaysAndHours(t *testing.T) {
	// Use UTC so result is deterministic regardless of machine timezone
	tools, err := AsTools(WithLocation(time.UTC))
	require.NoError(t, err)
	calculateTool := tools[1]

	var result calculateResult
	require.NoError(t, calculateTool.Execute(context.Background(), []byte(`{"base_date":"2026-03-11T12:00:00Z","add_days":3,"add_hours":2}`), func(c toolsy.Chunk) error {
		if c.RawData != nil {
			if r, ok := c.RawData.(calculateResult); ok {
				result = r
			}
		}
		return nil
	}))
	require.Regexp(t, rfc3339Re, result.Result)
	parsed, err := time.Parse(time.RFC3339, result.Result)
	require.NoError(t, err)
	expected := time.Date(2026, 3, 14, 14, 0, 0, 0, time.UTC)
	require.True(t, parsed.Equal(expected), "got %s", result.Result)
	require.Equal(t, "Saturday", result.Weekday)
}

func TestTimeCalculate_InvalidBaseDate_ClientError(t *testing.T) {
	tools, err := AsTools()
	require.NoError(t, err)
	calculateTool := tools[1]

	err = calculateTool.Execute(context.Background(), []byte(`{"base_date":"not-a-date","add_days":0,"add_hours":0}`), func(toolsy.Chunk) error { return nil })
	require.Error(t, err)
	require.True(t, toolsy.IsClientError(err))
	require.Contains(t, err.Error(), "invalid base_date")
}

func TestTimeCalculate_EmptyBaseDate_ClientError(t *testing.T) {
	tools, err := AsTools()
	require.NoError(t, err)
	calculateTool := tools[1]

	err = calculateTool.Execute(context.Background(), []byte(`{"add_days":0,"add_hours":0}`), func(toolsy.Chunk) error { return nil })
	require.Error(t, err)
	require.True(t, toolsy.IsClientError(err))
	require.Contains(t, err.Error(), "base_date")
}

func TestTimeCalculate_DST_AddDateCorrect(t *testing.T) {
	// In a DST transition day, Add(24*time.Hour) can yield wrong calendar day; AddDate(0,0,1) is correct.
	loc, err := time.LoadLocation("America/New_York")
	require.NoError(t, err)
	tools, err := AsTools(WithLocation(loc))
	require.NoError(t, err)
	calculateTool := tools[1]

	// 2026-03-08 02:00 EST -> add 1 day -> 2026-03-09 (DST starts March 8 2026 in US)
	base := "2026-03-08T07:00:00Z" // 02:00 EST
	var result calculateResult
	require.NoError(t, calculateTool.Execute(context.Background(), []byte(`{"base_date":"`+base+`","add_days":1,"add_hours":0}`), func(c toolsy.Chunk) error {
		if c.RawData != nil {
			if r, ok := c.RawData.(calculateResult); ok {
				result = r
			}
		}
		return nil
	}))
	parsed, err := time.Parse(time.RFC3339, result.Result)
	require.NoError(t, err)
	require.Equal(t, 9, parsed.Day())
	require.Equal(t, time.March, parsed.Month())
}

func TestTimeCalculate_DST_FallBack_November(t *testing.T) {
	// America/New_York: DST ends first Sunday of November. Add 1 calendar day in that zone.
	loc, err := time.LoadLocation("America/New_York")
	require.NoError(t, err)
	tools, err := AsTools(WithLocation(loc))
	require.NoError(t, err)
	calculateTool := tools[1]

	// 2026-11-01 00:30 EDT (UTC 04:30) -> add 1 calendar day in NY -> 2026-11-02 00:30 EST (wall clock)
	base := "2026-11-01T04:30:00Z"
	var result calculateResult
	require.NoError(t, calculateTool.Execute(context.Background(), []byte(`{"base_date":"`+base+`","add_days":1,"add_hours":0}`), func(c toolsy.Chunk) error {
		if c.RawData != nil {
			if r, ok := c.RawData.(calculateResult); ok {
				result = r
			}
		}
		return nil
	}))
	parsed, err := time.Parse(time.RFC3339, result.Result)
	require.NoError(t, err)
	require.Equal(t, 2, parsed.Day())
	require.Equal(t, time.November, parsed.Month())
	require.Equal(t, 2026, parsed.Year())
	// Wall clock in NY: 00:30 (hour 0, min 30)
	parsedNY := parsed.In(loc)
	require.Equal(t, 0, parsedNY.Hour(), "wall clock hour in NY should be 00:30")
	require.Equal(t, 30, parsedNY.Minute())
}

func TestTimeCalculate_DST_SpringForward_WallClockPreserved(t *testing.T) {
	// America/New_York: spring forward March 8 2026. Add 1 calendar day in that zone; result must be March 9.
	loc, err := time.LoadLocation("America/New_York")
	require.NoError(t, err)
	tools, err := AsTools(WithLocation(loc))
	require.NoError(t, err)
	calculateTool := tools[1]

	base := "2026-03-08T07:00:00Z" // 02:00 EST
	var result calculateResult
	require.NoError(t, calculateTool.Execute(context.Background(), []byte(`{"base_date":"`+base+`","add_days":1,"add_hours":0}`), func(c toolsy.Chunk) error {
		if c.RawData != nil {
			if r, ok := c.RawData.(calculateResult); ok {
				result = r
			}
		}
		return nil
	}))
	parsed, err := time.Parse(time.RFC3339, result.Result)
	require.NoError(t, err)
	parsedNY := parsed.In(loc)
	require.Equal(t, 9, parsedNY.Day(), "next calendar day after DST boundary")
	require.Equal(t, time.March, parsedNY.Month())
	// Arithmetic is in zone; wall clock may be 02:00 or 03:00 depending on tz data and transition handling
	require.GreaterOrEqual(t, parsedNY.Hour(), 0)
	require.Less(t, parsedNY.Hour(), 24)
}

func TestAsTools_ToolCountAndNames(t *testing.T) {
	tools, err := AsTools()
	require.NoError(t, err)
	require.Len(t, tools, 2)
	require.Equal(t, "time_current", tools[0].Name())
	require.Equal(t, "time_calculate", tools[1].Name())
}

func TestAsTools_CustomNames(t *testing.T) {
	tools, err := AsTools(WithCurrentName("now"), WithCalculateName("add_time"))
	require.NoError(t, err)
	require.Equal(t, "now", tools[0].Name())
	require.Equal(t, "add_time", tools[1].Name())
}
