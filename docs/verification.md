# Repository verification

Make invokes standard Go commands and golangci-lint directly. Modules are discovered
from go.mod files, excluding hidden directories and vendor. All commands use
GOWORK=off. There is no aggregate check target or tool version validation target.

| Command | Scope |
|---|---|
| `make modules` | List all discovered development modules. |
| `make test` | Fresh ordinary race tests across all modules. |
| `make test-integration` | Files with integration build tag; execute TestIntegration… functions only. |
| `make test-e2e` | Files with e2e build tag; execute TestE2E… functions only. |
| `make test-live` | Files with live build tag; execute TestLive… functions only, including paid calls. |
| `make lint` | Formatting diff and lint without rewriting files. |
| `make fix` | Go fix, formatting and lint fixes; modifies files. |
| `make fuzz` | Every discovered fuzz function separately, 30 seconds per function. |
| `make bench` / `make cover` | Benchmarks / per-module coverage. |

Build tags alone do not exclude ordinary test files. Profile targets combine the tag
with a matching test-name prefix so a module without such tests executes none.
Use the same convention for new tests; each profile runs directly through Go too:

```sh
GOWORK=off go test -race -tags=integration -run '^TestIntegration' ./...
GOWORK=off go test -race -tags=e2e -run '^TestE2E' ./...
```

Recipes use tools from PATH and explicitly propagate command failures.
Tool versions are pinned in CI, not enforced by Make. CI and source release gates
run lint, fresh unit tests, integration and e2e sequentially.

## Toolsy prerequisites and modules

Every discovered module is tested and published, including both example modules.
Nested module paths must equal the root module path plus their repository directory.
`go.work` remains an optional development convenience; Make and release ignore it.

The host dispatch example has its own go.mod and pinned prompty/flowy/guardy
versions. Its local root replacement checks the current Toolsy implementation.
Semantic fixtures use integration / TestIntegration; no sibling checkout or
copied recipe is required.

Docker integration requires a local Linux Docker daemon with mandatory cgroup
controls, access to the test workspace, and these adapter runtime images:
`bash:5.2`, `node:22-alpine`, `python:3.11-alpine`. Pull them before running the
integration profile. Missing prerequisites fail the selected tests. These images
provide sandbox interpreters; there is no host Python/PDF prerequisite.

MCP revision and legacy-fallback guards, schema provenance, and protocol fixtures
are ordinary Go tests. There is no separate preflight runner.

Make/release contract tests reside in the root module's test-only internal/release
package. Release e2e tests use disposable source and bare repositories; artifact
tests resolve all modules through a temporary file proxy without development
replacements. They never publish to the production remote.

CI pins Go 1.27.1 and golangci-lint 2.14.0. Local commands use tools from PATH.
Benchmarks, fuzz campaigns and paid provider tests remain explicit separate commands.
Historical reports retain their original commands and receipts.

Infrastructure reference: ragy commit `7baab34bdde64f1f6b1b99e89ab66d0b072fc6c5`.
