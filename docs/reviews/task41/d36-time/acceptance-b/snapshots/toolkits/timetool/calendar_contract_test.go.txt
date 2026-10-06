package timetool_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"
	_ "time/tzdata"

	"github.com/stretchr/testify/require"

	"github.com/skosovsky/toolsy"
	"github.com/skosovsky/toolsy/toolkits/timetool"
)

func TestCalendarDaysAndElapsedHoursAcrossDST(t *testing.T) {
	for _, fixture := range []struct {
		name, base, expected string
		days, hours          int
		elapsedHours         int
	}{
		{"spring day", "2026-03-07T17:00:00Z", "2026-03-08T12:00:00-04:00", 1, 0, 23},
		{"spring duration", "2026-03-07T17:00:00Z", "2026-03-08T13:00:00-04:00", 0, 24, 24},
		{"fall day", "2026-10-31T16:00:00Z", "2026-11-01T12:00:00-05:00", 1, 0, 25},
		{"fall duration", "2026-10-31T16:00:00Z", "2026-11-01T11:00:00-05:00", 0, 24, 24},
		{"spring reverse day", "2026-03-08T16:00:00Z", "2026-03-07T12:00:00-05:00", -1, 0, -23},
		{"spring reverse duration", "2026-03-08T16:00:00Z", "2026-03-07T11:00:00-05:00", 0, -24, -24},
		{"fall reverse day", "2026-11-01T17:00:00Z", "2026-10-31T12:00:00-04:00", -1, 0, -25},
		{"fall reverse duration", "2026-11-01T17:00:00Z", "2026-10-31T13:00:00-04:00", 0, -24, -24},
		{"days before hours", "2026-03-07T06:30:00Z", "2026-03-08T04:30:00-04:00", 1, 2, 26},
		{"base offset instant", "2026-03-07T22:30:00.123456789+05:30", "2026-03-07T12:00:00.123456789-05:00", 0, 0, 0},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			// Arrange: use a known host zone; base's own offset still defines its instant.
			loc, err := time.LoadLocation("America/New_York")
			require.NoError(t, err)
			tools, err := timetool.AsTools(timetool.WithLocation(loc))
			require.NoError(t, err)
			input, err := json.Marshal(
				map[string]any{"base_date": fixture.base, "add_days": fixture.days, "add_hours": fixture.hours},
			)
			require.NoError(t, err)
			var result timetool.CalculateResult
			// Act.
			err = tools[1].Execute(
				t.Context(),
				toolsy.NewRunEnv(nil),
				toolsy.ToolInput{ArgsJSON: input},
				func(chunk toolsy.Chunk) error {
					return json.Unmarshal(chunk.Data, &result)
				},
			)
			// Assert exact wall time, offset, elapsed duration, and weekday.
			require.NoError(t, err)
			require.Equal(t, fixture.expected, result.Result)
			base, err := time.Parse(time.RFC3339Nano, fixture.base)
			require.NoError(t, err)
			actual, err := time.Parse(time.RFC3339Nano, result.Result)
			require.NoError(t, err)
			require.Equal(t, time.Duration(fixture.elapsedHours)*time.Hour, actual.Sub(base))
			require.Equal(t, actual.In(loc).Weekday().String(), result.Weekday)
		})
	}
}

func TestLocationProviderResolvesBothToolsOnEveryCall(t *testing.T) {
	// Arrange: the host callback can return a different location on each invocation.
	calls := 0
	tools, err := timetool.AsTools(timetool.WithLocation(time.UTC), timetool.WithLocationProvider(
		func(context.Context, *toolsy.RunEnv) (*time.Location, error) {
			calls++
			return time.FixedZone("host", calls*3600), nil
		}))
	require.NoError(t, err)
	for _, index := range []int{0, 1, 0, 1} {
		input := []byte(`{}`)
		if index == 1 {
			input = []byte(`{"base_date":"2026-01-01T00:00:00Z","add_days":0,"add_hours":0}`)
		}
		var result map[string]any
		// Act.
		err = tools[index].Execute(
			t.Context(),
			toolsy.NewRunEnv(nil),
			toolsy.ToolInput{ArgsJSON: input},
			func(chunk toolsy.Chunk) error {
				return json.Unmarshal(chunk.Data, &result)
			},
		)
		// Assert: both tools resolve afresh, not a constructor-time cached zone.
		require.NoError(t, err)
		field := "local"
		if index == 1 {
			field = "result"
		}
		stamp, ok := result[field].(string)
		require.True(t, ok)
		value, err := time.Parse(time.RFC3339Nano, stamp)
		require.NoError(t, err)
		_, offset := value.Zone()
		require.Equal(t, calls*3600, offset)
	}
	require.Equal(t, 4, calls)
}

func TestLocationProviderFailureHasNoStaticFallback(t *testing.T) {
	// Arrange: static UTC is available, but an explicit provider failure is terminal.
	failure := errors.New("host timezone unavailable")
	calls, outputs := 0, 0
	tools, err := timetool.AsTools(timetool.WithLocation(time.UTC), timetool.WithLocationProvider(
		func(context.Context, *toolsy.RunEnv) (*time.Location, error) { calls++; return nil, failure }))
	require.NoError(t, err)
	for index, input := range []string{`{}`, `{"base_date":"2026-01-01T00:00:00Z","add_days":0,"add_hours":0}`} {
		// Act.
		err = tools[index].Execute(t.Context(), toolsy.NewRunEnv(nil), toolsy.ToolInput{ArgsJSON: []byte(input)},
			func(toolsy.Chunk) error { outputs++; return nil })
		// Assert.
		require.ErrorIs(t, err, failure)
	}
	require.Equal(t, 2, calls)
	require.Zero(t, outputs)
}
