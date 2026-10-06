# Contract fixtures

`agent-protocol-v1.openapi.yml` is the upstream Agent Protocol v1 OpenAPI 3.0.1 specification pinned at [`ecbffe0b9e45bdd3ead76299af6d980a1132b32a`](https://github.com/agi-inc/agent-protocol/blob/ecbffe0b9e45bdd3ead76299af6d980a1132b32a/schemas/openapi.yml). Retrieved 2026-10-06. The upstream MIT license is retained in `agent-protocol-LICENSE`.

`normative-task.json` and `normative-step.json` instantiate the pinned required envelopes. `TestPinnedNormativeStatusFixture` reads the enum from the upstream fixture rather than guessing protocol vocabulary.

`toolsy-step-stream-v1.json` is the local bridge extension terminal table, used by `TestDelegateTerminalOutcomes`. The SSE transport, cancellation endpoint and failed/cancelled states in this extension are **not** upstream Agent Protocol v1 requirements. They are not A2A states.

Tests exercise local HTTP fixtures. No live remote Agent Protocol service was used; this is fixture conformance to the declared subset and extension, not live interoperability certification.
