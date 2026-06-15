package document

import "github.com/skosovsky/toolsy/internal/format"

// extractWireJSONOverhead estimates JSON envelope bytes for {"text":"..."}.
const extractWireJSONOverhead = 16

// contentByteCap returns the parser content limit derived from the wire byte budget.
// Wire truncation (with textprocessor.TruncationSuffix) applies only on final JSON marshal.
func contentByteCap(maxWireBytes int) int {
	return format.WireContentCap(maxWireBytes, extractWireJSONOverhead)
}
