# Независимая приёмка A — task41 row27 D10/D11

База: 38314b41d1b164f3537b2603177c310c404578da. Проверен текущий tracked/untracked diff, включая operation_config_test.go. Peer verdict не читался. Repo не изменялся, commit не выполнялся; дополнительные fixtures/overlays только /tmp.

| Явный критерий раздела27, вес20% | D10 | D11 |
|---|---:|---:|
| 1. Named config, exported configuration error, caller/example migration |20%|20%|
| 2. Nil/typednil/callback/issuer/lease/cap validation, snapshot, host ownership |20%|20%|
| 3. Atomic dispatch/fence/replay/reconciliation boundary, original-grant recovery invariant |20%|20%|
| 4. AAA matrix, public baseline/current probe, recovery/no-effect checks, race/lint/examples |20%|20%|
| 5. Contract/migration/example consistency, audited new intent/current approval/duplicate safety, no new core dependency |20%|20%|
| **Полнота собственной приёмки A** |**100%**|**100%**|

Вердикт A: D10 принят; D11 принят. Выявленных неустранённых ошибок нет. Это собственная независимая приёмка; общий двух-reviewer gate устанавливает ведущая после отдельного заключения B.

Основания:
- operation_profile.go:74-127: named OperationProfileConfig, errors.Is sentinel с inspectable reason; nil/typednil ports, nil callbacks, issuer/lease/cap; value capture; no host callback on construction. isNilValue проверен в runenv.go:232.
- operation_config_test.go:14-89: AAA invalid-field matrix и snapshot, zero/default/positive caps, prepare/clock callbacks не вызываются при конструкции.
- operation_config_test.go:92-151: valid/expired original, fresh-grant denial without consuming new grant, stale old finish denied. :153-257: real registry/profile handler stays1 after fresh/expired denials.
- operation_store.go:251-275 retains binding/issuer/expiry/AllowRecovery/consumed-original checks. Source SHA256 operation_store.go, operation_contract.go, operation_reconcile.go exactly equals baseline; all retained baseline SHA records verified.
- AST remaining-seven-argument search returned null; current root, human/mail callers and executable example compile.
- docs/execution-contract.md:43-85 and docs/migration-task41.md:729-760 reviewed after final documentation clarification: keyed constructor fields; original grant remains immutable; new intent requires trusted absence/idempotency evidence, authenticated audit linkage, current authority, fresh bound approval and explicit downstream duplicate safety. Unknown/lease expiry/model/fresh approval alone do not permit redispatch.

Самостоятельно повторённые проверки:
1. GOCACHE=/tmp/toolsy-review-gocache go test -race ./... at root: PASS, including filejournal/exectool and examples/approval_journal; slowest internal/toolsygen48.484s.
2. Same full affected module race human/mail: PASS1.959s/1.933s.
3. Pinned /opt/homebrew/bin/golangci-lint run --allow-parallel-runners ./..., independent lint cache root/human/mail: each0 issues.
4. Current retained public typednil overlay plus TestOperationProfileConfiguration/TestRecovery/TestOperationStore/TestReconcile -race -count=3: PASS2.174s; -list separately confirms TestD10PublicTypedNilProfileProbe discovered.
5. Baseline extracted via git archive38314b4; same external public fixture with canonical /private/tmp overlay: exactly two expected behavioral FAIL, typed nil store/codec accepted; no compile failure. Initial noncanonical /tmp overlay yielded no-tests and was corrected, not counted as evidence.
6. Independent temporary AAA bounds overlay: cap2 exact display/result succeeds; display3 refuses before handler, encoded-result3 fails after one handler; default1MiB exact boundaries succeed and +1 reject. -race -count=3 PASS2.052s. Initial assertion expected model error text; corrected to inspect underlying OperationError via errors.As, consistent with intentional internal-error sanitization.
7. Three independent go run CLI processes challenge→approve→restart replay: PASS; no challenge receipt, final physical receipt count1. Directory /tmp/toolsy-task41/d10-d11-a-journal-1tfklqnp.
8. git diff --check: PASS.

Ограничения: local fixtures/ports; external authorization, downstream idempotency, distributed storage conformance не сертифицированы. Host lifetime/synchronization and provenance authentication остаются explicit host obligations. Нет требования добавить scheduler, grant-refresh API или domain DTO.
