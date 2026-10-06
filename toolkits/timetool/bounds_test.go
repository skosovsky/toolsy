package timetool

import (
	"context"
	"encoding/json"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/skosovsky/toolsy"
)

func TestDynamicCalendarLocationAndDST(t *testing.T) {
	loc, err := time.LoadLocation("America/New_York")
	require.NoError(t, err)
	calls := 0
	tools, err := AsTools(
		WithLocation(time.UTC),
		WithLocationProvider(
			func(context.Context, *toolsy.RunEnv) (*time.Location, error) { calls++; return loc, nil },
		),
	)
	require.NoError(t, err)
	for _, index := range []int{0, 1} {
		input := []byte(`{}`)
		if index == 1 {
			input = []byte(`{"base_date":"2026-03-07T17:00:00Z","add_days":1,"add_hours":0}`)
		}
		err = tools[index].Execute(
			context.Background(),
			toolsy.NewRunEnv(nil),
			toolsy.ToolInput{ArgsJSON: input},
			func(c toolsy.Chunk) error {
				if index == 1 {
					var out CalculateResult
					require.NoError(t, json.Unmarshal(c.Data, &out))
					require.Equal(t, "2026-03-08T12:00:00-04:00", out.Result)
				}
				return nil
			},
		)
		require.NoError(t, err)
	}
	require.Equal(t, 2, calls)
}

func TestTimeNilLocationAndArithmeticBounds(t *testing.T) {
	tools, err := AsTools(
		WithLocationProvider(func(context.Context, *toolsy.RunEnv) (*time.Location, error) {
			return nil, nil //nolint:nilnil // regression fixture for a malformed host provider
		}),
	)
	require.NoError(t, err)
	for _, tool := range tools {
		err = tool.Execute(
			context.Background(),
			toolsy.NewRunEnv(nil),
			toolsy.ToolInput{ArgsJSON: []byte(`{"base_date":"2026-03-07T17:00:00Z"}`)},
			func(toolsy.Chunk) error { t.Fatal("unexpected output"); return nil },
		)
		require.ErrorIs(t, err, toolsy.ErrValidation)
	}
	for _, args := range []calculateArgs{
		{BaseDate: "2026-01-01T00:00:00Z", AddDays: math.MaxInt},
		{BaseDate: "2026-01-01T00:00:00Z", AddHours: math.MaxInt},
		{BaseDate: "9999-12-31T23:00:00Z", AddHours: 1},
		{BaseDate: "0000-01-01T00:00:00Z", AddDays: -1},
	} {
		_, err = doCalculate(time.UTC, args)
		require.ErrorIs(t, err, toolsy.ErrValidation)
	}
}

func TestTimeCustomFormatterWireBounds(t *testing.T) {
	tools, err := AsTools(
		WithMaxWireBytes(100),
		WithResultFormatter(func(CurrentResult) (any, error) { return strings.Repeat("<", 40), nil }),
	)
	require.NoError(t, err)
	err = tools[0].Execute(
		context.Background(),
		toolsy.NewRunEnv(nil),
		toolsy.ToolInput{ArgsJSON: []byte(`{}`)},
		func(toolsy.Chunk) error { t.Fatal("oversize output"); return nil },
	)
	require.ErrorIs(t, err, toolsy.ErrValidation)
}

func TestRFC3339InstantFidelity(t *testing.T) {
	// Arrange: RFC3339 cannot encode Kolkata's historical seconds offset.
	historical, err := time.LoadLocation("Asia/Kolkata")
	require.NoError(t, err)
	for _, loc := range []*time.Location{historical, time.FixedZone("invalid", 25*3600)} {
		// Act.
		_, err = doCalculate(loc, calculateArgs{BaseDate: "1900-01-01T00:00:00Z"})
		// Assert.
		require.ErrorIs(t, err, toolsy.ErrValidation)
	}
	_, err = ComputeCurrent(time.FixedZone("invalid", 25*3600))
	require.ErrorIs(t, err, toolsy.ErrValidation)
	// Arrange: fractional seconds must survive a zero-delta operation.
	base := "2026-01-01T00:00:00.123456789Z"
	// Act.
	out, err := doCalculate(time.UTC, calculateArgs{BaseDate: base})
	// Assert.
	require.NoError(t, err)
	require.Equal(t, base, out.Result)
}

func TestStrictRFC3339Input(t *testing.T) {
	for _, base := range []string{
		"2026-01-01T00:00:00+24:00",
		"2026-01-01T00:00:00+00:60",
		"2026-01-01T00:00:00,123Z",
		"2026-01-01T0:00:00Z",
		"2026-01-01T00:00:00.1234567891Z",
	} {
		// Arrange / Act.
		_, err := doCalculate(time.UTC, calculateArgs{BaseDate: base})
		// Assert.
		require.ErrorIs(t, err, toolsy.ErrValidation, base)
	}
	for _, base := range []string{"2026-01-01T00:00:00+00:00", "2026-01-01T00:00:00.123000000Z"} {
		// Arrange / Act.
		out, err := doCalculate(time.UTC, calculateArgs{BaseDate: base})
		// Assert: formatting may remove trailing zeros but the instant is identical.
		require.NoError(t, err, base)
		expected, err := time.Parse(time.RFC3339Nano, base)
		require.NoError(t, err)
		actual, err := time.Parse(time.RFC3339Nano, out.Result)
		require.NoError(t, err)
		require.True(t, expected.Equal(actual))
	}
}
