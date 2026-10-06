# R03 independent acceptance

Scope: five equally weighted criteria in the execution ledger, current diff
against `d1619e8`. All validated input pipelines share existing lossless
jsonschemax Decode/Compile. Typed interface and dynamic numbers use json.Number;
concrete host types retain their declared semantics. Schema traversal preserves
property names and literal data while transforming actual schema nodes.

## Reviewer A — r03_acceptance_a

**100%, accepted**, all five criteria 20/20; no unresolved detected defects.
Independently inspected the complete final diff and new tests, ran uncached root
race plus targeted schema/numeric race after the final walker cleanup, lint
2.14.0 (zero issues), and whitespace checks. Its own overlay checks negative
int64 minimum, exclusiveMinimum, exact multipleOf, scientific integer enum and
nested typed interface numbers. Independently reran baseline numeric regressions
and confirmed original production files by SHA-256. No repository mutation or
access to the other reviewer's verdict.

## Reviewer B — r03_acceptance_b

**100%, accepted**, all five criteria 20/20; no unresolved detected defects.
Independently inspected the full final diff, reran uncached root race after the
walker cleanup and lint (zero issues). Its own overlay checks negative minimum,
const with exponent, exact multipleOf, max uint64, nested interfaces, escaped
nested duplicate keys and trailing documents. It exercised every listed walker
path including legacy tuple items, verifying schema IDs/strict transformations
and preservation of property id and const literals. Independently reran original
numeric regressions. No repository mutation or access to the other verdict.

## Baseline and parent verification

Initial numeric/structure regressions were run against the original archive
`58085005`. Behavioral failures show handler rounding, prepared snapshot rounding,
cache key collision, ignored bounds/enums and typed validation mismatch, without
build failures. Later typed interface, fractional, schema transformation and
exact structure-limit fixtures are current regressions, not baseline-certified.
Original dynamic constraints on property id were also lost by the old schema
walker; retaining that property's schema is necessary to enforce its bounds.

Parent final uncached root `go test -race -count=1 ./...` passes, including
filejournal, generator and executable examples; targeted lint reports zero
issues. Logs are retained alongside this report, with tabs expanded and trailing
whitespace stripped. An additional sequential uncached race sweep passes all 23 consumer modules;
its log is retained. Together with root this covers the 24-module workspace at
R03. The final goal gate must repeat verification after the remaining changes.

## Performance and limitations

One local Apple M1 Max run of the existing BenchmarkExecute observes baseline
22,729 ns/op, 13,099 B/op, 224 allocs/op versus current 30,672 ns/op, 14,626 B/op,
236 allocs/op. The extra bounded structural scan and exact validation have a
measurable cost. These single runs are not a statistical comparison; the
benchmark excludes construction and its loop ignores errors, while separate
execution regressions verify successful calls.

Duplicate keys, trailing documents, depth above 128 and more than 100,000 value
nodes are rejected; byte/string limits stay host-owned. Custom UnmarshalJSON and
explicit Go float fields may choose host representation semantics. Already
rounded host values cannot be recovered. Arbitrary schemas/live integrations,
other task41 rows and the final workspace gate are outside this acceptance.
Percentages describe criteria completion, not universal bug freedom.
