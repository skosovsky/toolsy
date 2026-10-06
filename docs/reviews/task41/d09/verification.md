# D09 verification

Baseline commit/source hashes: baseline-sources.json. Production bytes were checked
against that commit before mutation. The retained public fixture uses reflection
only to support both void baseline and error-returning current mutation APIs.
All six baseline failures are behavioral assertions, without compile failure:
absent/unbound mutation target (four), accepted generic/stream nil handler (two).
Map a nonexistent root d09_probe_test.go to d09-mutation-probe_test.go.txt using a
Go overlay and run `go test -race -count=1 . -run TestD09PublicMutationAndHandlerProbe`.
Current original fixture passes; final count3 run PASS.

Current direct AAA contract tests cover nil/zero/unbound targets, empty keys,
INTERNAL/nonretryable/noncorrectable cause; shared cloned env/session storage,
optional typed non-nil reads, legal nil values and host-owned referenced BYOT values.
Constructor matrix covers generic/stream/proxy/dynamic/typed/policy-spec and
missing-schema precedence; policy wrapper rejects nil/typednil base tools.
Existing concurrent/reentry state tests now check mutation errors safely. A nil
Session.Execute Env never silently creates a bound session state target.

Root contract race PASS; root lint 0 issues; executable snapshot example PASS.
Full current root race PASS; all24-module `GOFLAGS=-count=1 make test` PASS.
The all-module root stage preceded the final dynamic nil-handler precedence/sample
addition; current full root-final race covers that final production source too.
Reviewer A then found two ignored explicit-type-argument mutation results in codec
tests. They now use require.NoError; targeted current race count3 PASS and pinned root lint 0 issues. Production code is unchanged by that review fix. Both A and B independently accepted the updated diff at100% (five20/20), no unresolved detected defects; see acceptance-a.md/acceptance-b.md.
No live external backend/service or arbitrary BYOT synchronization guarantee.
