// Package document extracts text from host-authorized document sources and optional safe remote URLs.
// Source, parser and JSON wire budgets are independent; local access is disabled until the host
// selects a source provider or root. In-process PDF parsing requires explicit acceptance of
// third-party allocation and cancellation limitations.
package document
