# R13 / D20 / D21 / D22 verification

Baseline for this row: 22782ff. The public two-subscription regression fails for
both Close and parent cancellation on that baseline; it passes on the new code.
Late acknowledged/notification traffic for locally retired IDs leaves B live and
its generation unchanged. Malformed retired resource messages and malformed
messages during the active-to-retired transition still fail strictly.

Contracts: 64 active / 1024 tracked subscriptions / 1-minute retirement defaults;
no eviction on saturation. Transport defaults: 1 MiB raw frame, 16 MiB retained
outgoing bytes, 64 in-flight requests, unlimited cumulative traffic. Optional
lifetime bounds are explicit. Queue/count failures expose TransportLimitError;
frame/lifetime failures preserve ErrReadLimitExceeded. Raw LF and CRLF count
against incoming frame bounds.

Full discovery bounds 10000 items / 16 MiB aggregate raw bytes / 1000 pages /
1 MiB cursors by default, before accumulating/compiling out-of-budget descriptors.
ListToolsPage never publishes authority; typed DiscoverTools and proxy Discover
share the full collector. Every accepted input/output schema is compiled before
publication. Page TTL metadata remains available in ToolDiscovery.Pages.

Cancellation is checked inside the authority lock immediately before publication.
A synchronous atomic ReplaceToolHeaderBindings already started is the commit point;
later cancellation does not roll it back. This trusted facet must finish in bounded
time and cannot reenter Client methods. Mapper reentry remains supported. Native
HTTP Replace validates the replacement before swapping it. Generation changes
invalidate authority. The contract does not promise automatic transaction/rollback
for arbitrary host callbacks or arbitrary infinitely delayed wire messages.

Verification: affected full MCP race suite PASS (21.349s); final focused regressions
PASS count5 (3.237s); pinned golangci-lint2.14.0 reports 0 issues. Public real-stdio
regressions and synthetic HTTP SSE readers validate raw frame limits independently
of lifetime traffic. Old tests now assert typed errors instead of previous strings.

Benchmark (3 samples, 200ms; synthetic full discovery, not a stable latency estimate):
1 descriptor ~114432B/1712 allocations baseline versus ~135440B/2025 allocations;
32 descriptors ~2922200B/42428 allocations versus ~3507700B/50145 allocations.
The added input-schema compilation and aggregate predecode cost ~0.58 MiB and
~7717 allocations for 32 descriptors. No production performance guarantee inferred.
No go.mod/dependency change; unrelated tool-created go.work.sum drift was removed.

Initial acceptance found CRLF raw accounting, stdio Notify cause, cancellation
commit-boundary documentation, and canceled-route malformed validation gaps.
Those findings were fixed and permanent AAA regressions added. Initial verdicts
are superseded only by the independent final verdicts stored alongside this file.
