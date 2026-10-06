// Package rag exposes one host-owned knowledge retriever as a bounded toolsy tool.
// Retrieval routing, deduplication, fallback and retry policy belong to the host.
// Host IoC is available through WithResultFormatter and WithHostResultValidator.
package rag
