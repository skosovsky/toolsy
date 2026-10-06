# D34 SQL lexical subset evidence

Baseline signed 9b7ac3f. Spec/criteria: task41 ledger row35. Implementation only by
parent; both independent reviewers accepted 100%, five 20/20 criteria each,
no unresolved detected product errors. Rename ValidateReadOnlyQuery and file/
keyword helper to ValidateSelectLexicalSubset/select_subset/blockedStatementKeywords;
no old alias and no parser expansion. Algorithm retained; only identifiers, API
comments and misleading validation reasons change. Public tool names/metadata
stay under explicit host-owned restricted connection/routine authority.

Parent root full race count3 passes (internal/release113.374s, generator91.954s)
and pinned lint2.14.0 zeroissues. SQL full final race count3 passes3.137s plus local
host recipe1.591s, pinned lint zeroissues. Initial SQL example shadow findings
retained in sqltool-lint.log and fixed; no suppressions. example.log proves SELECT
and actual SQLite SQLITE_READONLY write denial from mode=ro fixture connection.

AAA lexical tests retain positive/rejected subset and explicitly exercise accepted
malformed grammar/SELECT functions/SELECT INTO, which a lexical usability filter
cannot certify. Private fixture driver registers observable host SELECT function;
query_only denies valid INSERT (SQLite code8) yet SELECT increments hostcounter.
This does not claim SQLite can execute SELECT INTO or malformed grammar; these
queries demonstrate filter acceptance alone, not database acceptance.

Own public host-effects probe compiled with exact baseline production snapshots
(including original readonly.go) using a Go overlay hiding renamed/current tests.
It passed race3 on baseline1.588s and current1.602s. This intentional passing
limitation demonstration substantiates naming/docs correction; it is not a claim
of a baseline-failing regression or new side-effect prevention. Snapshot/probe
sources retained as text. No live PostgreSQL/MySQL verification is claimed.

Supported driver aliases select schema inspection SQL, not dialect-aware lexers.
README/API/migration document quote/comment/token limitations, incomplete syntax,
borrowed DB/driver bounds, inspection-only table filters and host routine authority.


Additional deterministic lexer comparison: go/scanner token streams of exact
baseline and current implementation are identical after normalizing only two
renamed identifiers and three intended rejection-message literals. Comments are
excluded; all other tokens match. lexer-parity.log and comparison source retained.
This demonstrates that this row changes terminology/messages rather than inventing
a different SQL parser or expanding the accepted query domain.


Baseline harness identity is also executable: a dedicated old-API test references
ValidateReadOnlyQuery (absent from current production) and asserts its old semicolon
reason, alongside the public host-effect test. Both named tests visibly pass three
race runs in baseline-host-effects-identity.log, proving the exact old lexer was
compiled rather than accepting a no-tests/canonical-path overlay artifact.


Final independent A/B reports, raw logs, exact snapshots and own probe sources are
retained. A root tests/race (unchanged release excluded), SQL full race3 and root/SQL
lint pass; independent lexical corpus and actual public authority baseline/current
fixtures pass. B affected root race3 10.769s, SQL full race3 3.419s/host1.373s and
root/SQL lint pass; 10,024-query baseline/current decision parity race3 1.629s;
actual public authority current1.679s/baseline1.865s pass. B also verifies five
inspection dialect aliases share the same execute filter, inspection-only tables
and query_only DB denial vs host function effects. Scoped-out expensive unchanged
root packages are explicit; parent's full root race3 remains retained.

Initial independent harness failures are retained and excluded from product proof:
A quoting/cache issues; B vet missing virtual package directory, /tmp vs /private/tmp
baseline overlay no-tests run, and an initial command targeting root instead of SQL.
Corrected checks actually execute named probes; baseline passing behavior is a
limitation demonstration, not a claim of side-effect prevention or baseline FAIL.


Only substantive reports/logs/probes/affected baseline sources are archived. Lint
caches and unrelated files from the reviewer's disposable full baseline checkout
are omitted; exact commit plus affected snapshot hashes reproduce that checkout.
