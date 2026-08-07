#!/bin/sh
set -eu

repo_root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
cd "$repo_root"

production_files=$(find mcp -maxdepth 1 -type f -name '*.go' ! -name '*_test.go' -print)
method_files=$(find mcp -maxdepth 1 -type f -name '*.go' ! -name '*_test.go' ! -name 'transport.go' -print)
if [ -z "$production_files" ]; then
    echo "task34 preflight: MCP production files not found" >&2
    exit 1
fi

require_pattern() {
    pattern=$1
    shift
    if ! rg -q -- "$pattern" "$@"; then
        echo "task34 preflight: required contract pattern missing: $pattern" >&2
        exit 1
    fi
}

forbid_pattern() {
    pattern=$1
    shift
    if rg -n -- "$pattern" "$@"; then
        echo "task34 preflight: forbidden legacy MCP production contract found" >&2
        exit 1
    fi
}

# Static clear-break gate. Documentation and negative fixtures are deliberately
# outside this production-only scan.
require_pattern 'ProtocolVersion[[:space:]]*=[[:space:]]*"2026-07-28"' mcp/protocol.go
forbid_pattern '2025-11-25|2025-06-18|2024-11-05' $production_files
forbid_pattern 'InitializeParams|InitializeResult|MethodInitialize|MethodInitialized|ErrSessionExpired|WithClientRoots|WithRoots|OnRequest[[:space:]]*\(' $production_files
# transport.go owns the explicit fail-closed denylist; the behavioral fixture
# below proves those literals are rejection data, not reachable legacy methods.
forbid_pattern '"initialize"|"notifications/initialized"|"roots/list"|"logging/setLevel"|"resources/(subscribe|unsubscribe)"|"ping"' $method_files
forbid_pattern 'Mcp-Session-Id|Last-Event-ID|http\.MethodGet|http\.MethodDelete' $production_files

# Behavioral proof for discovery, request metadata, POST-only routing,
# cancellation/provenance, strict fixtures and old-version rejection.
# TestTask34OfficialSchemaArtifactHasImmutableProvenance is included by the
# TestTask34 wildcard and recomputes the pinned official schema SHA-256.
test_pattern='^(TestConnectUsesStrictDiscoveryAndSelfDescribingMetadata|TestConnectRejectsOldOnlyServerWithoutFallback|TestRequestMeta.*|TestCurrentInertCapabilities.*|TestExtensionRegistry.*|TestCapabilityExtensions.*|TestProtocolBoundary.*|TestStrictDTOFamilies.*|TestResultUnion.*|TestOutboundExtraAndMeta.*|TestComputeSnapshotDigest.*|TestStreamableHTTPUsesPostOnlyRoutingAndSentinelHeaders|TestPreparedRequestRegistersSubscriptionBeforeImmediateACK|TestStreamableHTTPCancellationClosesOnlyOriginatingPOST|TestSSEProvenanceRejectsCrossRequestAndPreAcknowledgementNotifications|TestTask34.*)$'
(cd mcp && go test -run "$test_pattern" -count=1 .)
(cd internal/jsonschemax && go test -run '^(TestDecodeRejectsRecursiveDuplicateKeys|TestCompile.*)$' -count=1 .)

echo "task34 preflight: MCP 2026-07-28 clear-break contract passed"
