// Package history provides dependency-free helpers for semantic chat history truncation.
//
// The package is BYOT (Bring Your Own Type): callers provide their own message type T
// plus lightweight adapters (token counter, summarizer, and inspector).
//
// A successful semantic summary counts only the original history, its protected
// system prefix and the proposed summary. Mechanical fallback checks safe
// boundaries in order and reuses one candidate buffer. It deliberately does not
// assume additive or monotonic token counts: with a whole-snapshot tokenizer,
// fallback can still inspect a quadratic number of messages. Hosts own tokenizer
// caching, normalization and any provider-specific token accounting.
package history
