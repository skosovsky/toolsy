package web

// scrapeWireJSONOverhead estimates JSON envelope bytes for {"markdown":"..."}.
const scrapeWireJSONOverhead = 18

// scrapeContentByteCap returns the HTML/markdown content limit derived from the wire byte budget.
// Wire truncation (with textprocessor.TruncationSuffix) applies only on final JSON marshal (tool path).
func scrapeContentByteCap(maxWireBytes int) int {
	if maxWireBytes <= scrapeWireJSONOverhead {
		return maxWireBytes
	}
	return maxWireBytes - scrapeWireJSONOverhead
}
