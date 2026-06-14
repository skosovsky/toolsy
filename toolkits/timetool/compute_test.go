package timetool

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestComputeCurrent_UTC(t *testing.T) {
	loc := time.FixedZone("Test", 3*3600)
	got := ComputeCurrent(loc)
	require.NotEmpty(t, got.UTC)
	require.NotEmpty(t, got.Local)
	require.NotEmpty(t, got.Weekday)
	require.Positive(t, got.Unix)

	parsedUTC, err := time.Parse(time.RFC3339, got.UTC)
	require.NoError(t, err)
	parsedLocal, err := time.Parse(time.RFC3339, got.Local)
	require.NoError(t, err)
	require.Equal(t, parsedUTC.Unix(), parsedLocal.Unix())
}

func TestComputeCurrent_NilLocationUsesUTC(t *testing.T) {
	got := ComputeCurrent(nil)
	require.NotEmpty(t, got.UTC)
	require.Equal(t, got.UTC, got.Local)
}

func TestComputeCurrent_DSTAwareLocation(t *testing.T) {
	loc, err := time.LoadLocation("America/New_York")
	require.NoError(t, err)
	got := ComputeCurrent(loc)
	parsedUTC, err := time.Parse(time.RFC3339, got.UTC)
	require.NoError(t, err)
	parsedLocal, err := time.Parse(time.RFC3339, got.Local)
	require.NoError(t, err)
	require.Equal(t, parsedUTC.Unix(), parsedLocal.Unix())
	_, off := parsedLocal.Zone()
	require.True(t, off == -4*3600 || off == -5*3600, "unexpected US Eastern offset: %d", off)
}
