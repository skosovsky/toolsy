package web

import "github.com/skosovsky/toolsy/internal/format"

// scrapeWireJSONOverhead estimates JSON envelope bytes for {"markdown":"..."}.
const scrapeWireJSONOverhead = 18

// scrapeContentByteCap returns the HTML/markdown content limit derived from the wire byte budget.
// Final JSON encoding rejects results that exceed the complete wire budget.
func scrapeContentByteCap(maxWireBytes int) int {
	return format.WireContentCap(maxWireBytes, scrapeWireJSONOverhead)
}
