# R01 independent acceptance

Baseline: `58085005dc7e4a0747b2a57af2ef2f62ba2c3750`. Scope: R01 and the five
criteria in [execution ledger](../task41-progress.md), each worth 20%. Final scope
includes httptool, agents, OpenAPI, GraphQL, web, document and MCP: all direct
consumers of the shared helper.

Implementation: remote redirects allow only same-origin GET/HEAD; whitelist read
redirects strip Authorization/Cookie/Proxy-Authorization on origin change. Other
initial methods never redirect, including HTTP method rewriting. Every refused
redirect has an outer nonretryable `CodeRemoteExecution` error, an inspectable
`RedirectError` and any original validation cause. Original effects are not
declared rolled back. Public migration is in `docs/migration-task41.md`.

## Baseline and implementation evidence

New shared-policy and public agents/OpenAPI/GraphQL tests were run before changing
production source, with `GOCACHE=/tmp/toolsy-review-gocache go test -run
'Test(RemoteRedirect|AllowedRedirect|RedirectSafety|OriginNormalizes|RPCRejects|PublicTool.*Redirect|PublicIntrospectionRejects)'
-count=1 ./...` in each module. All four commands failed on behavioral assertions,
without build failures. Logs are retained alongside this report. Classification
and stream-read positive tests were added during remediation; their current API
assertions were not part of the original baseline run.

Final parent verification:

```sh
GOCACHE=/tmp/toolsy-review-gocache go test -race -count=1 \
  ./toolkits/httptool/... ./agents/... ./contracts/openapi/... ./contracts/graphql/... \
  ./toolkits/web/... ./toolkits/document/... ./mcp/...
GOCACHE=/tmp/toolsy-review-gocache \
  GOLANGCI_LINT_CACHE=/tmp/toolsy-task41/lint-cache \
  golangci-lint run --allow-serial-runners \
  ./toolkits/httptool/... ./agents/... ./contracts/openapi/... ./contracts/graphql/... \
  ./toolkits/web/... ./toolkits/document/... ./mcp/...
git diff 58085005dc7e4a0747b2a57af2ef2f62ba2c3750 --check
```

Results: all seven module suites PASS with race, golangci-lint 2.14.0 reports
0 issues, whitespace check PASS. Final test/lint logs are retained here.

## Initial four-module reviewer A — r01_acceptance_a

Final: **100%, accepted**, 20/20 for each of the five criteria; no unresolved
detected defects. Independently repeated uncached race tests and lint in all four
modules. The initial review identified correctable URL/SSRF redirect failures
and lint defects (80%, rejected). After changes, the original userinfo probe for
both callbacks returned RedirectError=true, outer REMOTE_EXECUTION and
ClientCorrectable=false. Re-reviewed the complete updated diff and documentation.
Did not implement or mutate repository files and did not read reviewer B's verdict.

## Initial four-module reviewer B — r01_acceptance_b

Final: **100%, accepted**, 20/20 for each of the five criteria; no unresolved
detected defects. Independently repeated uncached race tests and lint in all four
modules. Initial lint failure rejected the diff at 80%. The final independent
wire probe checked 140 combinations of two policies, seven methods, five
statuses and two origin scenarios with race detection. Target counters proved
no effectful second dispatch; read headers followed the selected policy. Every
refusal exposed nonretryable/noncorrectable CodeRemoteExecution and RedirectError.
Validated retained diagnostic causes and agent create's unknown outcome. Did not
implement or mutate repository files and did not read reviewer A's verdict.

## Final seven-module reviewer A — r01_final_a

Final: **100%, accepted**, all five criteria 20/20. Independently repeated
uncached race tests and lint over all seven direct modules, reviewed the entire
baseline diff and current migration. Its initial expanded review rejected the
diff at 90%: document/HTTP toolkits masked outer classification and MCP's
method-changing guard bypassed RedirectError. The repeated public probes now
return REMOTE_EXECUTION, retry=false, correction=false, RedirectError=true.
Its independent MCP overlay passes all 301/302/303/307/308 cases. No remaining
identified defect. Did not implement changes, modify the repository or read
previous acceptance reports/the other final reviewer's verdict.

## Final seven-module reviewer B — r01_final_b

Final: **100%, accepted**, all five criteria 20/20. Independently repeated
uncached race tests and lint over all seven direct modules. Its initial expanded
review rejected the diff at 90% for consumer classification and test coverage.
Repeated document public probe now preserves REMOTE_EXECUTION. Its independent
wire probe covers 30 unsafe-method/status combinations, GET/HEAD credential
positives and the bounded redirect loop. It independently reran the MCP baseline
307/308 fixture (targetCalls=1) and confirmed the fixed regression passes. No
remaining identified defect. Did not implement changes, modify the repository or
read previous acceptance reports/the other final reviewer's verdict.

## Supplemental evidence and limits

MCP's public same-origin body-replay regression was run in a `git archive` of the
baseline under `/tmp/toolsy-task41/r01-original`. Only the new counter fixture was
copied; API assertions using the newly added RedirectError were removed to allow
compilation against the original API. Both 307/308 cases failed because the
second endpoint received one request. The current fixture also verifies typed
classification for 301/302/303. Baseline log is retained here. Retained log files
expand tabs and strip trailing whitespace; messages are unchanged. Original raw
logs remain under `/tmp/toolsy-task41`.

One earlier seven-module run under concurrent load exceeded an existing devZero
read test's 200 ms threshold by 3 ms. No body/read implementation changed in R01;
subsequent uncached parent and independent suites passed. This timing-sensitive
fixture is recorded as a verification limitation rather than hidden.

These are local HTTP fixtures, shared-policy checks and source review. Subdomain
and HTTPS downgrade/upgrade are checked by shared-policy fixtures; real wire
probes use local origins with different ports. Live interoperability, arbitrary
host callbacks, the full 24-module workspace and remaining task41 scope are not
certified here. Percentages measure the five criteria, not universal bug freedom.
