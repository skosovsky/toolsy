package main

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/skosovsky/toolsy/adapters/sandbox/starlark"
	"github.com/skosovsky/toolsy/exectool"
)

func TestPolicyRecipeExactOutput(t *testing.T) {
	// Arrange.
	ctx, cancel := context.WithTimeout(context.Background(), callerBudget)
	defer cancel()
	var out bytes.Buffer
	// Act.
	err := run(ctx, `print("bounded")`, &out)
	// Assert.
	require.NoError(t, err)
	var result exectool.RunResult
	require.NoError(t, json.Unmarshal(bytes.TrimSpace(out.Bytes()), &result))
	require.Equal(t, "bounded\n", result.Stdout)
	require.Empty(t, result.Stderr)
	require.Zero(t, result.ExitCode)
}

func TestPolicyRecipeCallerCancellation(t *testing.T) {
	// Arrange.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var out bytes.Buffer
	// Act.
	err := run(ctx, `print("must not run")`, &out)
	// Assert.
	require.ErrorIs(t, err, context.Canceled)
	require.Empty(t, out.String())
}

func TestPolicyRecipeStepsAreNotCallerTimeout(t *testing.T) {
	// Arrange: background context has no deadline, finite host instruction policy remains.
	var out bytes.Buffer
	code := "def work():\n    total=0\n    for i in range(100000):\n        total+=i\n    return total\nwork()"
	// Act.
	err := run(context.Background(), code, &out)
	// Assert: do not confuse step exhaustion with elapsed deadline or guest exit.
	require.ErrorIs(t, err, starlark.ErrStepLimit)
	require.ErrorIs(t, err, exectool.ErrSandboxFailure)
	require.NotErrorIs(t, err, exectool.ErrTimeout)
	require.NotErrorIs(t, err, context.DeadlineExceeded)
	require.Empty(t, out.String())
}
