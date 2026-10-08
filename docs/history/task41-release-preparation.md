# Historical Task41 release preparation

This describes the former Go release implementation, replaced by the common
Bash release. For current commands and guarantees see [the runbook](../release/runbook.md).

## Release preparation (R05 / D37)

`make release-patch` and `make release-break` now build the host CLI and prepare
committed HEAD in a private clone. Linux and macOS are supported. The source
checkout must have no tracked changes. Untracked and ignored files remain local;
they never become candidate inputs. The source branch, index, files and refs are
preserved on success, failure and cancellation. Effective Git identity plus commit.gpgsign, gpg.format and user.signingkey
are copied into the private clone; hooks are disabled there. Other host Git
settings remain inherited from the host, so repository-specific signing programs
may need host configuration. Signing failure aborts before publication.

The inventory comes from all committed `go.mod` paths. An optional legacy module
list must equal that inventory. All module paths must follow the root module plus
the relative directory. Internal dependencies must form a DAG; cycles and missing
owned modules fail explicitly. Manifest edits use `go mod edit`, align owned
requirements and remove owned development replaces. External local replaces and
nonregular manifests/checksum files are rejected. Only expected `go.mod`/`go.sum`
files are staged. Before checkout, a private index and cached attribute check
reject attributes that transform checkout/archive bytes: filter, working-tree-encoding,
export-ignore, export-subst, ident, crlf, text and eol (absent or explicitly unset
attributes are allowed). Ordinary diff/merge/linguist metadata is supported. Host
global attributes are included in this check. Checkout uses LF and real symlinks.
Bootstrap applies the same attribute preflight before building its Git archive.

For each module, in dependency order, preparation tidies its release manifest,
creates the Go module ZIP with `x/mod/zip` (including inherited root LICENSE),
downloads that ZIP through an invocation-owned file proxy, and compiles its packages
and tests with `GOWORK=off` and `-mod=readonly`. Peer ZIP checksums are final before
preparing dependents. Each module has a ten-minute verification deadline. The CLI
then runs `make lint test` in the private checkout and the existing break preflight,
when present, with a thirty-minute deadline. Tracked changes from checks abort the
release. This verification checks the exact artifact graph; it does not execute
all dependency tests from their downloaded ZIPs.

The private module cache and GOPATH (including checksum-database state) are removed
on exit. Existing downloaded archives provide a read-only fallback before the
configured GOPROXY. Existing external GONOSUMDB policy is retained. If GONOPROXY
matches any owned module, verification sets that bypass to `none` and uses only
cached archives or direct VCS for external dependencies, preventing private module
names from reaching a public proxy. This case requires cached dependencies or
working VCS access; a proxy-only installation may fail explicitly. Existing Go
build/lint caches may still be reused.

To verify without publication, run
`bash scripts/release.sh -prepare-only patch`. The host wrapper archives committed runner code into a private bootstrap directory
and builds with
`GOWORK=off`, `-mod=readonly` and `-buildvcs=false`. SIGINT/SIGTERM cancel preparation
and terminate CLI command process groups before private-directory cleanup. The
bootstrap build has its own job process group; cancellation sends TERM, waits at
most two seconds, then sends KILL to that group and removes its directory.
Publication confirmation owns its input reader; Close must unblock Read.

The wrapper clears Git repository-selector environment variables and rejects
counted/serialized inline Git configuration before bootstrap; configure bootstrap
authentication in normal Git config or SSH facilities. The native CLI supports
counted authentication/identity configuration, strips counted repository, hook,
fsmonitor and checkout overrides, and rejects GIT_CONFIG_PARAMETERS. These
overrides cannot redirect its private index/worktree or change verified bytes.

Publication uses explicit root/submodule tag refspecs from this invocation, with
--atomic and --no-follow-tags; mirror configuration is disabled for that command.
Unrelated local tags never enter the train. Exactly one push destination is
required; fetch and push destinations may differ. Local and push-remote collisions
are checked before manifest preparation and again after confirmation. Existing
refs are never deleted or forced. A server without atomic push support or a
rejected tag fails the whole train; there is no sequential fallback.

Preflight cannot lock remote refs. Git arbitrates concurrent conflicting updates
at atomic push; an identical concurrent tag may be reported up-to-date. A transport
failure after the server accepted a push leaves publication outcome uncertain:
inspect the remote before retrying. Local cleanup does not roll back remote tags.
All release regressions use disposable local bare remotes; no production
publication is used for verification.

