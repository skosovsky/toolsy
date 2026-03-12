package document

import "unicode/utf8"

const truncateSuffix = "\n[Truncated]"

// truncateUTF8 truncates s to at most maxBytes at a rune boundary and appends truncateSuffix.
// Use for safe output limiting (UTF-8 safe, no broken runes).
func truncateUTF8(s string, maxBytes int) string {
	if len(s) <= maxBytes {
		return s
	}
	need := maxBytes - len(truncateSuffix)
	if need <= 0 {
		return truncateSuffix
	}
	n := 0
	for _, r := range s {
		rn := utf8.RuneLen(r)
		if n+rn > need {
			return s[:n] + truncateSuffix
		}
		n += rn
	}
	return s
}
