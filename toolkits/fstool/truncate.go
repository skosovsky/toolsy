package fstool

import "github.com/skosovsky/toolsy/internal/format"

// readWireJSONOverhead estimates JSON envelope bytes for {"content":"..."}.
const readWireJSONOverhead = 15

// readContentByteCap returns the file content limit derived from the wire byte budget.
func readContentByteCap(maxWireBytes int) int {
	return format.WireContentCap(maxWireBytes, readWireJSONOverhead)
}
