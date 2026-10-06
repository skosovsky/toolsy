# D10 / D11 independent acceptance B

Baseline: 38314b41d1b164f3537b2603177c310c404578da. Reviewed current tracked/untracked row27 diff read-only. Peer verdict was not read; repository files and Git state were not modified.

| Criterion (section27) | Completion | Evidence |
|---|---:|---|
| 1. Named config / caller migration / inspectable error | 20/20 | operation_profile.go:75,80,94,127; AST call-site scan returns null for remaining seven-argument calls; root and affected modules compile. |
| 2. Validation / snapshot / callback and reference ownership | 20/20 | operation_profile.go:98-127; operation_config_test.go:14,51 validates nil/typednil, callbacks, lease/cap, default/value snapshot/no callbacks. Inclusive display/result bounds remain existing `>` comparisons at operation_profile.go:197,305; host reference ownership explicit at :77. |
| 3. Retained atomic recovery boundary | 20/20 | operation_store.go:210-350 and operation_contract.go/operation_reconcile.go byte-identical baseline. Original consumed grant plus AllowRecovery/unexpired approval/fenced reconciliation retained. operation_config_test.go:92,153 covers valid/expired original, fresh denial, stale worker and physical handler counter. No retry scheduler/grant replacement. |
| 4. Reproduced public probes/tests/race/lint/examples | 20/20 | Same reflective public probe baseline FAIL store+codec assertions; current -race count3 PASS. Root -race ./... PASS, human/mail -race PASS. Root/human/mail pinned /opt/homebrew/bin/golangci-lint with independent caches and --allow-parallel-runners each 0 issues. Target config/recovery/probe -race count3 PASS. CLI challenge -> approval -> restart replay reports completed_operation; effects.jsonl exactly 1 line. |
| 5. Contract/migration/audited authorization/no new dependency | 20/20 | docs/execution-contract.md:43,56,64; docs/migration-task41.md:729; examples/approval_journal README/current constructor agree. Current authority, fresh bound approval, old provenance, verified absence/idempotency and uncertainty retention explicit. No domain DTO or core dependency added. |

Total: **100%**. D10 accepted by reviewer B; D11 accepted by reviewer B. Unresolved detected defects: **0**.

Independent verification: source SHA256 manifest matches git baseline for all four files; store/contract/reconcile current raw bytes equal baseline. Constructor is the only changed production logic. All real callers migrated (AST scan).

Limits: root broad race uses Go build/test cache where applicable; targeted count3 is uncached. Local fixtures and example do not prove external host authentication, downstream idempotency or distributed exactly-once effects. This report does not claim peer acceptance; the root must obtain the second independent100% before committing. No commit/publication performed.
