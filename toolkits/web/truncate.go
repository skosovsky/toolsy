package web

import "github.com/skosovsky/toolsy/internal/format"

// scrapeWireJSONOverhead estimates JSON envelope bytes for {"markdown":"..."}.
const scrapeWireJSONOverhead = 18

// scrapeContentByteCap returns the HTML/markdown content limit derived from the wire byte budget.
// Wire truncation (with textprocessor.TruncationSuffix) applies only on final JSON marshal (tool path).
func scrapeContentByteCap(maxWireBytes int) int {
	return format.WireContentCap(maxWireBytes, scrapeWireJSONOverhead)
}
