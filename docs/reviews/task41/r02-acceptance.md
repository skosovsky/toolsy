# R02 / D17 independent acceptance

Scope: five equally weighted criteria in the execution ledger; implementation
diff against `4dec511`. Bare hostnames match exactly; leading-dot entries match
descendants only. Deny matches take precedence before DNS/dial. Migration covers
both apex and descendant blacklist entries explicitly.

## Verification

The original baseline archive (`58085005`) fails the new overlap/syntax fixtures
on behavioral assertions, including identical suffix and broad allow/narrow deny;
its public dial attempts DNS for a blocked overlap. These initial fixtures compile
against the original API. Later initial-URL/redirect diagnostic assertions were
added to the current regression suite and were not part of that baseline run.

Parent verification passes uncached `go test -race -count=1` for httptool, agents,
OpenAPI, GraphQL, web, document and MCP. golangci-lint 2.14.0 reports zero issues
for the changed httptool/web modules. Whitespace checks pass. Logs are retained
alongside this report, with tabs expanded and trailing whitespace removed.

## Reviewer A — r02_acceptance_a

**100%, accepted**, each criterion 20/20; no unresolved detected defects.
Independently inspected the full diff including the new regression file and
migration, repeated uncached race checks for all seven modules and targeted lint.
Confirmed runtime deny precedence, consistent exact/suffix matching, positive
controls, blank allowlist rejection, pre-resolution denial and retained R01
redirect protections. Did not modify repository files or read the other verdict.

## Reviewer B — r02_acceptance_b

**100%, accepted**, each criterion 20/20; no unresolved detected defects.
Independently inspected the full diff, repeated seven-module race checks and
targeted lint, and reran original overlap failures. Its own resolver-counter probe
observed zero DNS calls for denied exact/suffix/narrow/normalized/blank-policy
admissions. It also checked transport slice snapshots, allowed siblings and
initial-URL/redirect consistency. Did not modify repository files or read the
other verdict.

## Benchmark and limits

The retained benchmark source measures admission with 1, 32 and 256 entries,
outside configuration construction. All nine cases pass with zero allocations.
This single local run ranges from 121 ns for a one-entry denial to 47.6 us for a
256-entry exact allow after scanning both lists. Matching remains linear; these
numbers are observations, not a statistical performance comparison. The source
can be copied into the module as a temporary `_test.go` file to reproduce it.

Standalone blacklist URL validation is not a complete DNS syntax validator;
SafeDialTransport rejects malformed policy hosts before resolution. Arbitrary
DNS/IDNA aliases, live interoperability and future R07/D34 scope are not certified.
Percentages measure the five stated criteria, not universal bug freedom.
