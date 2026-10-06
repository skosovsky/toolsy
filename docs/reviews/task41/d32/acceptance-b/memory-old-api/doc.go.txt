// Package memory exposes a bounded session scratchpad backed by host StateStore.
// One Scratchpad instance must be the sole writer per session state key; its cancellable admission gate
// serializes local calls but does not provide distributed atomicity.
package memory
