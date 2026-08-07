package mcp

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/require"
)

func TestProtocolVersionErrorBoundsServerDiagnostic(t *testing.T) {
	// Arrange.
	selected := strings.Repeat("я", maxDiagnosticBytes*4)
	err := &ProtocolVersionError{Requested: ProtocolVersion, Selected: selected}

	// Act.
	message := err.Error()

	// Assert.
	require.Less(t, len(message), len(selected))
	require.NotContains(t, message, selected)
	require.True(t, utf8.ValidString(message))
}

func TestJSONRPCErrorRejectsFractionalCodeOnMarshal(t *testing.T) {
	// Arrange.
	wireErr := JSONRPCError{Code: JSONNumber("1.5"), Message: "invalid"}

	// Act.
	_, err := wireErr.MarshalJSON()

	// Assert.
	require.ErrorContains(t, err, "must be integer")
}
