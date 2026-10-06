package sqltool

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestD32SchemaFilterSnapshot(t *testing.T) {
	// Arrange.
	tables := []string{"public"}
	option := WithAllowedTables(tables)
	tables[0] = "private"
	var first options
	option(&first)
	// Act.
	accepted := filterTablesByAllowlist([]string{"public", "private"}, first.allowedTables)
	// Assert.
	require.Equal(t, []string{"public"}, accepted)
	first.allowedTables[0] = "private"
	var second options
	option(&second)
	require.Equal(t, []string{"public"}, filterTablesByAllowlist([]string{"public", "private"}, second.allowedTables))
}
