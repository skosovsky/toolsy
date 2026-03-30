// Package history provides dependency-free helpers for semantic chat history truncation.
//
// The package is BYOT (Bring Your Own Type): callers provide their own message type T
// plus lightweight adapters (token counter, summarizer, and inspector).
package history
