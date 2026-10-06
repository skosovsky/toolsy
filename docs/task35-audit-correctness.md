# Task35 — независимый аудит корректности

Зафиксированный независимый snapshot предшествует последующему обновлению
Go/зависимостей по запросу от2026-10-05. Его проверки отдельно записаны в
task35-handoff.md; этот отчёт не заявляет новый post-refresh аудит.

## Current verdict — 2026-10-05

**Correctness PASS для текущего проверенного implemented whole candidate.** Root COR-001–007 и MCP-COR-001–008 закрыты независимо; открытых подтверждённых findings нет. Полные независимые root/MCP race suites прошли, финальные небольшие lint-only delta перепроверены. Последний fingerprint проверенного subset: `b034e4f6763e8c1671ac27bc00b7affdaf7df4efd622dcf2de37f44d965e0c28`.

MCP clear-break migration авторизована и выполнена, current full package собирается и проходит race. Старый preserved-MCP build conflict ниже — **историческое evidence**, не нынешний blocker. Production аудитором не изменялась; отдельно по явному поручению добавлена custom-pending regression с RED→fix verification. Независимость от completeness auditor сохранена. На текущем коде основной агент подтвердил terminal PASS: all-module `make test` (93922), all-module `make lint` (87586, 0 issues; Go1.26.3, тот же pinned runner/rules, isolated caches и --allow-parallel-runners, без исключения модулей), preflight (90040) и `git diff --check`. Эти gate результаты предоставлены основным агентом, не выданы за независимые запуски аудитора. Correctness verdict не заменяет независимую полноту и не разрешает release/issue closure.

## Historical first execution audit

Следующие findings и baseline ограничения относятся к первоначальным snapshot и сохранены как история. Их current closure/final evidence находятся в последних разделах отчёта.

## Scope и evidence

Прочитано полное `.cursor/docs/task35.md`, текущие prepared boundary, typed/policy/raw/stream builders, registry execution, override/async paths, ResultCache/JSONResultCodec, OperationProfile, MemoryOperationStore, reconciliation port, durable filejournal adapter и соответствующие regression/fault tests. Независимость от аудитора полноты сохранена: его отчёт не читался. Production code и repository tests аудитором не изменялись.

Четыре сценария воспроизведены через публичный API внешней Go-программой `/private/tmp/toolsy-audit-proof.go` командой `go run /private/tmp/toolsy-audit-proof.go` (exit 0). Это adversarial evidence ошибок, не принятие полной suite. Пользовательские untracked MCP-файлы не менялись. Workspace MCP build conflict и отсутствие финального all-module gate остаются ограничениями; аудит не подтверждает protocol profiles условных карточек, которые не реализованы. Race/lint PASS предыдущего запуска не использовался как доказательство отсутствия ошибок.

## COR-001 / P1 — внешний policy wrapper обходит внутреннюю typed policy на replay и binds не те args

Точки: `policy_tool.go:112–130` (профиль вызывается до `spec.Tool.Execute`, внутренний профиль отключается); `typed_tool.go:155–170` (внутренние binder/policy выполняются только при invocation); `result_cache.go:85–86` (hit пропускает invocation).

Публичная поддерживаемая композиция `NewPolicyTool(NewTypedTool(...))` имеет две preparation boundaries. Внешний binder нормализует value в `approved`; внутренний binder меняет его на `different`. Профиль — следовательно approval/operation binding — получает `approved`, а actual handler исполняет `different`. При cache/operation replay внутренняя typed policy вообще не выполняется. После fill, даже если внутренняя policy отозвана, внешний wrapper возвращает cached result с nil error. Это повтор BUG-001 через композицию и нарушение binding конкретного действия в TLS-001/002.

Воспроизведённый вывод:

```text
profile authorized canonical={"value":"approved"}
actual handler args=different
nested binding err=<nil>
inner revoked yet replay result="different"
inner revoked replay err=<nil>
```

Regression: завернуть typed tool с изменяющим args binder и счётчиком policy в policy wrapper; каждый разрешённый fill/replay должен пройти все применимые policies, а snapshot профиля должен соответствовать actual handler args. Revoke внутренней policy → denial и zero leaked chunks. Repeat для ResultCache/OperationProfile, direct/registry/Session и async background. Если такую композицию нельзя обеспечить минимальным seam, construction/configuration защищённого profile должна fail closed до side effect; нельзя молча заявлять защищённую capability.

## COR-002 / P1 — cache replay обходит текущий delivery/audience policy wrapper

Точки: `policy_tool.go:73–76` (manifest не включает delivery/audience/metadata spec); `policy_tool.go:123–125` (applyPolicyToolEnvelope находится внутри continuation); `result_cache.go:135–162` (key содержит manifest/input/view, но не текущую envelope policy); `result_cache.go:172–192` (replay доставляет старый envelope).

Заполнить shared cache с policy wrapper AudienceModel. Затем заменить wrapper над тем же base tool на AudienceInternal, сохраняя subject/scope/manifest и explicit partition. Текущая policy разрешает execution, но требует internal delivery. Hit пропускает applyPolicyToolEnvelope и выдаёт AudienceModel. Known library envelope restrictions отсутствуют также в PreparedCall: partition callback не может автоматически привязать ключ к этим настройкам.

Воспроизведённый вывод:

```text
configured=model delivered=model replay=<nil>
configured=internal delivered=model replay=true
audience err=<nil>
```

Regression: shared store с model outcome → текущий wrapper internal → replay никогда не выдаёт model/user envelope. Аналогично сменить DeliveryClass/EnvelopeMetadata и проверить актуальный контракт. Решение должно учитывать одновременно сохранение полного outcome и недопустимость ослабления current delivery policy; host partition не должен неявно отвечать за библиотечные скрытые поля wrapper. Проверить operation replay тоже: он использует ту же replay helper и wrapper placement.

## COR-003 / P2 — buffered outcome не изолирует mutable Envelope.Result

Точки: `result_cache.go:195–210` (cloneResultChunk); `envelope.go:117–125` (Result остаётся `in.Result`); `stream_contract.go:161–162` и `operation_profile.go:293–295` используют тот же snapshot; `result_codec.go:64–71` сериализует изменённый Envelope.Result.

Producer передаёт Chunk с Data и TypedResult/Envelope.Result map `state=validated`; после успешного capture меняет исходную map в `changed after capture`, затем завершает producer успешно. TypedResult/Data буфер изолирует, Envelope.Result — нет. Получатель свежего и replay outcome получает два противоречащих типизированных значения; producer-owned mutation после validation попадает в durable outcome. При конкурентной mutation возможна также data race на codec чтении.

Воспроизведённый вывод:

```text
fresh typed=map[state:validated] envelope=map[state:changed after capture]
fresh err=<nil>
replay typed=map[state:validated] envelope=map[state:changed after capture]
```

Regression: nested mutable BYOT pointer/map/slice в Envelope.Result, отдельно от TypedResult, изменить после yield candidate; terminal/cached/journal fresh/replay envelope сохраняет исходный snapshot. Добавить race case с синхронизацией producer mutation после capture. Текущий TestCompleteResultSnapshotIsolatesNestedTypedEffects не содержит Envelope и поэтому не покрывает ошибку.

## COR-004 / P2 — ResultCache забывает consumer/control abort и объявляет success

Точки: `result_cache.go:90–96` (non-result yield error не присваивается captureErr); `result_cache.go:113–132` (nil producer error позволяет persistence/delivery). Для comparison OperationCapture и terminal stream имеют sticky failure.

Producer посылает progress, игнорирует ошибку consumer, затем посылает один result и возвращает nil. Consumer отказался от progress; ResultCache всё равно записывает и delivers successful terminal. Следующий вызов успешно replay этот result. Аналогичная форма игнорирования pause/control не блокируется capture cache, хотя документация обещает не завершать pending calls. Никакой автоматический rollback внешнего эффекта не требуется, но abort/pending нельзя превратить в cache success.

Воспроизведённый вывод:

```text
success delivered after abort
aborted producer err=<nil>
replay exists after consumer abort
```

Regression: producer намеренно игнорирует callback error на progress/control; ResultCache сохраняет первое failure/control outcome, последующие yield не доставляются как success, store не получает completed entry. Добавить cancellation после non-result callback, swallowed PauseSignal и async collected-output cap case. Отдельно сохранять уже persisted completed operation при поздней delivery failure — это иной правильный контракт.

## Проверенные без подтверждённой находки участки

В просмотренном коде MemoryOperationStore предоставляет единый mutex boundary grant+claim; namespace/scope/intent binding conflict, consumed grants, issuer/expiry и explicit AllowRecovery проверяются до нового dispatch. Used attempt IDs сохраняются, поэтому ABA attempt reuse запрещён. Finish/Resolve fenced binding+attempt, completed delivery failure не откатывает state, lease expiry не даёт слепого retry. OperationProfile повторно проверяет clock/approval expiry после claim acknowledgement и превращает поздние failures в unknown. ReconcileOperation — explicit trusted-host callback и CAS на inspected attempt, не модельная authority. Filejournal сериализует independent handles flock, sync temp→rename→directory и не выдаёт dispatch на uncertain write. Эти observations не отменяют найденные upstream composition/envelope gaps и не являются доказательством абсолютного отсутствия иных bugs.

## Следующая проверка

После исправлений повторить четыре adversarial scenarios, добавить repository regression tests, выполнить обязательные gates и провести независимый final-state аудит. До этого correctness acceptance закрывать нельзя.

## Промежуточная перепроверка policy composition

Публичный proof повторно выполнен после делегирования профиля deepest ExecutePrepared и добавления policyEnvelopeProfile. Source проверки: `policy_tool.go`, `policy_prepared_composition_test.go`, `prepared_execution.go`. Это targeted review, не final acceptance.

- Исправленная часть COR-001 подтверждена: profile snapshot теперь `different`, как actual handler args; revoked inner policy возвращает `POLICY_DENIED` без replay.
- Но outer policy по-прежнему вызывается до inner normalization. Добавленная outer ACL разрешает исключительно `approved`, явно запрещая `different`; вызов печатает `outer ACL authorizes=approved`, затем `actual handler args=different`, и возвращает nil. Все policies выполнены, но не все разрешили фактически исполненное действие. COR-001 остаётся открытой в части final binding/authorization всех decorators.
- Исправление COR-002 работает в направлении model → internal, но расширяет сохранённый private outcome при обратной смене wrapper: `configured=internal delivered=internal replay=<nil>`, затем `configured=model delivered=model replay=true`, nil error. Unconditional `applyPolicyToolEnvelope` на replay не удовлетворяет требованию never expands audience. Нужна совместимость/intersection текущего delivery contract и сохранённого envelope либо explicit fail-closed несовместимость; нельзя трактовать смену wrapper как право переадресовать сохранённый internal результат модели.
- Внешний proof попутно подтвердил исправления COR-003/004: envelope и TypedResult остаются `validated`; consumer abort возвращается и cache entry отсутствует. Это targeted evidence, не замена их repository/race coverage.

Минимальный безопасный контракт composition: downstream normalization не может менять действие после того, как decorator authorizes его args. Возможны два решения: (а) выполнить все bind/normalize, затем каждую применимую typed policy/validator над actual final canonical values (с явным BYOT преобразованием и отказом unsupported conversion); (б) policy decorator, рассчитанный на собственный canonical snapshot, fail closed на downstream binding/context mismatch до profile и side effect. Произвольный второй normalization нельзя молча считать безопасным лишь потому, что deepest policy выполнена. Отдельно гарантировать соответствие bound.Value и forwarded bound.Raw, а также identity/scope, если внутренний CallContext изменяется. Для opaque private proof полей необходим immutable host-owned контракт; JSON round-trip не должен уничтожать proof ради такой проверки.

## Вторая targeted перепроверка

После добавления preparedChecks и conservative replay audience intersection внешний original proof теперь даёт outer ACL `different` → POLICY_DENIED до handler, а stored internal/current model остаётся internal. **COR-001/002 targeted scenarios закрыты** на этом snapshot. Это не final acceptance всей задачи.

Отдельный hostile BYOT/context proof `/private/tmp/toolsy-final-snapshot-proof.go` (exit 0) подтвердил, что outer policy видит final-bob вместо промежуточного outer-alice, сохраняет private final-host-proof из deepest typed binder и не может изменить handler map через request.Args/BoundArgs.Value. Output:

```text
outer sees subject=final-bob proof=final-host-proof value=real
handler subject=final-bob proof=final-host-proof value=real
final snapshot error=<nil>
```

Документированный unsupported case: если final typed value не содержит необходимого private proof, JSON fallback его не изобретает; policy должна fail closed. Это явное ограничение composition, не утверждение, что private fields можно восстановить из JSON.

### COR-005 / P2 — invocation-local prepared checks наследуются unrelated nested вызовом

Точки текущего snapshot: `runenv.go:115–118` копирует preparedChecks в cloneForExecute; `prepared_execution.go:85–98` не удаляет завершённые checks перед handler dispatch; `registry.go` принимает ToolCall.Env и клонирует его для следующего invocation. PolicyEnvelopeProfile также остаётся wrapped в handler env и требует проверки invocation scoping.

Внешний proof `/private/tmp/toolsy-nested-policy-proof.go`, exit 0: parent tool с policy wrapper/schema `{action: required}` успешно проходит own check. Handler передаёт RunEnv в childReg.Execute для независимого child tool/schema `{}`. Child invocation наследует parent check и неожиданно валидирует `{}` по parent schema. Child handler не вызывается, parent завершается validation failure:

```text
outer policy tool=parent args={Action:outer}
nested child error=VALIDATION_FAILED: validating root: required: missing properties: ["action"]
parent error=VALIDATION_FAILED: validating root: required: missing properties: ["action"]
```

Regression: parent policy/binder count ровно один, разрешённый nested child с другими args успешно исполняется через scoped executor с тем же host context/budget, но не наследует parent argument validators/envelope transforms/manifest. Child собственная policy и profile продолжают выполняться. Повторить no-profile/cache/operation, direct nested Tool.Execute, registry ToolCall.Env и async. Не очищать host execution profile/Session budget целиком ради исправления: consume/reset только invocation-local decorator state перед handler, сохранять текущие host restrictions.

## Последний targeted verdict: COR-001–005 scenarios исправлены

Текущий `registry.go` после cloneForExecute очищает invocation-local preparedChecks и снимает только policyEnvelopeProfile decorators, сохраняя underlying host profile. Повторно запущен именно новый snapshot, не старый report. `/private/tmp/toolsy-nested-policy-proof.go` (exit 0) теперь печатает parent policy один раз, child handler ran и nil errors у child/parent. **COR-005 для scoped Registry/Session entry point закрыта**.

Независимые focused проверки:

- `go test -race ./... -run 'TestPolicyPreparationDoesNotLeakIntoNestedExecutor|TestPolicyComposition' -count=1` — PASS.
- `go test -race ./... -run 'TestResultCache|TestCompleteResultSnapshot|TestPreparedSnapshot' -count=1` — PASS; включает новые swallowed PauseSignal/cancellation/consumer abort tests.

Минимальный контракт direct entry point: RunEnv принадлежит одной invocation; новый direct Tool.Execute должен получить fresh invocation-owned environment с explicitly supplied trusted context/profile. Inherited handler environment само по себе не является scoped executor и не должно использоваться для raw nested bypass. Документирование этого ограничения необходимо; произвольная raw direct nesting с унаследованными parent decorators/manifest здесь не проверена и не объявляется поддержанной. Настоящий nested integration использует переданный Registry/view/Session executor, который повторяет собственные policy/budget checks.

Это targeted verdict конкретных fixes и regression scope. Он не заменяет final-state audit всех активных требований, полные workspace gates и проверку sync/async/nested integration шире этих fixtures.

## Broad final-state candidate audit

Production code/tests/другие документы при этой проверке заморожены; аудитор менял только этот отчёт и external proof scripts. Отчёт completeness не читался. Дополнительно проверены current raw-nesting guard и caller RunEnv cloning, scoped nested Session/view/policy/budget/approval integration, два external-package BYOT consumer на одном Tool, actual concurrent approved resumes, canonical input/secret freshness/provider end-marker fixtures, terminal+operation integration, обычный CLI approval_journal и его trusted-local-host authentication assumptions. Task35 и активные contracts перечитаны; deferred protocol/backend profiles не объявляются реализованными или security-audited.

### COR-006 / P1 — пользовательский metadata overlay стирает library replay marker и расширяет audience

Точки candidate snapshot: `policy_tool.go:189–194` (profile-side transform), `policy_tool.go:210–225` (audience restriction зависит от metadata marker, затем maps.Copy overlay его стирает), внешний wrapper yield выполняет transform повторно; `result_cache.go:201–208` устанавливает library marker перед replay delivery. OperationProfile использует ту же replay helper.

Публичный сценарий: shared ResultCache, policy wrapper первоначально AudienceInternal, затем current wrapper AudienceModel; оба разрешены и имеют тот же base manifest/host partition. В `ToolPolicySpec.EnvelopeMetadata` допустимо передать `{toolsy.CacheReplayMetadata: false}`. На replay первый transform ещё видит true и оставляет internal, но overlay записывает false. Следующий transform трактует outcome как fresh и выставляет model. Consumer получает private payload для модели и false replay marker. Reducer по документированному marker contract может также повторно применить сохранённые effects. Ни дополнительный grant, ни handler dispatch для этого не требуются.

Внешний `/private/tmp/toolsy-audit-proof.go` изменён только добавлением указанного metadata overlay; `go run` exit 0. Evidence:

```text
configured=internal delivered=internal replay=false
audience err=<nil>
configured=model delivered=model replay=false
audience err=<nil>
```

Regression: cache и operation profiles, single и nested policy wrappers, stored internal → current model/user с reserved metadata key false/non-bool; либо explicit construction failure до execution, либо outcome остаётся internal, а authoritative replay marker=true. Host effect reducer применяет effect только на fresh delivery. Runtime overlays не могут сбрасывать библиотечный marker; API должен документировать и защищать reserved key, а не доверять совпадению пользовательского имени. Проверить также fresh outcomes с forged marker и формально зафиксировать, кто имеет право его устанавливать.

### Выполненная независимая проверка

- Broad focused `go test -race ./... -run 'TestPrepared|TestPolicyComposition|TestPolicyPreparation|TestResultCache|TestOperation|TestConcurrentApproved|TestNestedSupplied|TestTwoOrdinaryBYOT|TestCanonical|TestTerminalStream|TestOrdinaryHost' -count=1` — PASS. Это реально покрытые suites, не claim о всех module tests.
- `go test -race ./adapters/execution/filejournal -count=1` — PASS, включая interprocess claim и abrupt external-commit/crash recovery.
- `go test -race . -run 'TestApproval|TestHostProviderCompletionBoundaryFixture|TestPreparedHandlerEnv|TestDirectPreparedCalls' -count=1` — PASS.
- `git diff --check` — PASS.
- COR-006 воспроизводится несмотря на эти зелёные проверки: соответствующей regression в frozen snapshot нет.

Raw direct reuse of protected handler env теперь fail closed до built-in binding/handler; independent direct built-in calls clone invocation state и не меняют caller env. Registry/Session начинают новый invocation, сбрасывают только parent decorator/dispatch state и сохраняют underlying trusted profile. Current integrations явно передают supplied Session и host context, не получают root registry из RunEnv. Ordinary approval_journal deliberately fixed identity/CLI approve допустимы только для trusted local operator; README правильно запрещает трактовать это как remote authentication и не выполняет автоматическое reconciliation по локальному файлу.

В повторно просмотренных atomic claim/fencing/reconciliation/crash, context/argument snapshot, terminal-buffer и sample-host путях новых самостоятельных P0–P3 не подтверждено, кроме COR-006. Это конечный просмотр конкретного candidate, не абсолютное доказательство отсутствия всех ошибок. Root lint/race результаты основного агента не заменяют собственную проверку и обязательный all-module gate. Final acceptance отклонена до исправления COR-006 и финальных обоих audits/gates.

## Повторный final-state audit после COR-006 fix

COR-006 независимо перепроверена на следующем frozen candidate. `validatePolicyToolSpec` отказывает при любом presence reserved key; public `NewPolicyToolFromSpec` делегирует в ту же проверку. External `/private/tmp/toolsy-reserved-metadata-proof.go`, exit 0: false/true/nil/string values дают nil Tool, explicit reserved metadata error и zero handler calls. Runtime overlay дополнительно удаляет reserved key из private copy перед merge. Новые defense-in-depth и nested cache/operation effect-reducer tests входят в full root race.

Собственный `go test -race ./... -count=1` — PASS по всему root module, включая filejournal, subprocess crash/concurrency, обычный approval CLI, examples и codegen. Это не all-module make test: отдельный MCP baseline conflict не считается зелёным gate. Повторные original public scenarios COR-001–005 также больше не воспроизводятся: final args deny/revoked inner deny, audience internal, snapshot stable, aborted cache miss.

### COR-007 / P2 — JSONResultCodec молча округляет JSON numbers в dynamic typed payload

Точка: `result_codec.go:90–92` (`json.Unmarshal` full wire result), последующее восстановление TypedResult/Envelope.Result. Любая interface-bearing JSON value (`map[string]any`, `R=any`, metadata, interface-bearing effects) декодируется через float64, несмотря на успешный encode exact standard `json.Number`.

Публичный codec proof `/private/tmp/toolsy-codec-number-proof.go`, exit 0: R=`map[string]any`, значение `{"n": json.Number("9007199254740993")}`, Data=`{"n":9007199254740993}`. EncodeResult и DecodeResult завершаются без ошибки. После decode TypedResult повторно marshals в `{"n":9007199254740992}`, Envelope.Result содержит то же округлённое значение, тогда как Data/Envelope.Raw остаются exact original bytes:

```text
original raw={"n":9007199254740993} typed={"n":9007199254740993} replay raw={"n":9007199254740993} typed={"n":9007199254740992} decoded type=float64 err=<nil>
```

Это стандартный JSON-shaped payload с json.Number, а не private proof/non-JSON type, требующий другого codec. Полный outcome становится противоречивым, подтверждённое значение silently changes. При больших account/record IDs неправильный TypedResult может использоваться обычным host consumer. Нормативные input-number fixtures не покрывают output codec recovery.

Regression: exact >2^53 numbers и nested decimals в dynamic result/envelope/effects/metadata; codec roundtrip и actual cache/operation replay должны сохранять числовое значение либо explicit fail unsupported before completed persistence/delivery. Concrete typed integer/float/pointer codecs продолжат сохранять declared types. Зафиксировать ограниченный dynamic JSON representation contract (например json.Number для interface numbers), не обещать восстановление arbitrary concrete Go types из interface JSON; такие host types требуют custom codec. JSON decoder должен reject trailing extra values, если меняется Unmarshal на Decoder.

COR-006 закрыта; новый candidate отклонён только новой подтверждённой COR-007 в просмотренном implemented execution scope плюс независимо обязательный workspace gate. После исправления требуется новая проверка обоими аудиторами.

## Итоговая независимая перепроверка после COR-007 fix

Последний frozen candidate перечитан независимо, включая public builders и profiles, wrapper/current policy composition, caller env isolation/raw nested guard, codecs/replay/envelopes, atomic approval+claim/used fencing IDs, unknown outcome/crash/reconciliation/expiry, terminal-stream composition и ordinary host examples. Отчёт полноты не читался. Production/tests и пользовательские MCP-файлы не менялись; только этот отчёт и external proof scripts. Проверка не распространяет claims на deferred protocol/backend profiles.

Execution snapshot fingerprint: `e9a1c0562ef7cd40972a10ec1fd8e68b5c8449f76df04c67f0ebd47878f0653e`, получен через SHA-256 sorted-by-shell file checksum stream для root `*.go`, filejournal `*.go`, approval_journal/stream_terminal `*.go` и `testdata/execution/*.json`. Сам отчёт не включён; это идентификация проверенного execution candidate, не digest всего репозитория.

COR-007 fix сохраняет concrete declared field types и использует decoder.UseNumber для dynamic JSON/interface numbers. Second Decode требует io.EOF: null/object/invalid suffix rejected, не ослаблен one-document contract. Новые tests проверяют exact >2^53, nested decimal/exponent lexemes, result/envelope/effects/metadata, int64 и actual Session cache/operation replay, а не только codec helper.

Независимый `/private/tmp/toolsy-codec-number-proof.go` exit 0 теперь даёт:

```text
original raw={"n":9007199254740993} typed={"n":9007199254740993} replay raw={"n":9007199254740993} typed={"n":9007199254740993} decoded type=json.Number err=<nil>
```

Остальные external proofs повторно запущены на том же candidate, exit 0:

- `toolsy-audit-proof.go`: outer final args denial и inner revoke denial, stored internal remains internal/replay=true, stable envelope snapshot, consumer abort без cache entry.
- `toolsy-reserved-metadata-proof.go`: public FromSpec rejects false/true/nil/string reserved key, Tool=nil, handler calls=0.
- `toolsy-nested-policy-proof.go`: parent policy once, child handler executes, child/parent nil error.
- `toolsy-final-snapshot-proof.go`: final host context/private proof preserved, hostile Args/BoundArgs map mutation does not affect handler.

Собственный независимый **полный** `go test -race ./... -count=1` — PASS на final numeric candidate; includes filejournal interprocess/crash, examples/approval_journal, all root packages/codegen и COR regression tests. `git diff --check` — PASS. Наличие зелёных тестов само по себе не использовалось как отсутствие bugs: рассмотренные bypass/ordering/replay/crash invariants сверялись с кодом и adversarial public scenarios.

Итог по findings:

| Finding | Severity | Финальный статус |
|---|---|---|
| COR-001 inner replay authorization/final argument mismatch | P1 | fixed, independently verified |
| COR-002 stale/current audience expansion | P1 | fixed, independently verified |
| COR-003 mutable Envelope.Result snapshot | P2 | fixed, independently verified |
| COR-004 swallowed consumer/control abort | P2 | fixed, independently verified |
| COR-005 parent prepared checks leaking into nested executor | P2 | fixed, independently verified |
| COR-006 reserved replay marker overlay | P1 | fixed, independently verified |
| COR-007 dynamic JSON numeric rounding | P2 | fixed, independently verified |

Незакрытых подтверждённых implementation findings в проверенном выполненном scope нет. Ограничения сохраняются: host authenticates identity/approver and supplies policy/dependency freshness; private immutable/dynamic arbitrary Go types require explicit host contracts/custom codec; local journal not distributed exactly-once; actual nested consumers must use scoped executor; deferred cards not audited as implemented. Возможность иных неизвестных bugs не отрицается.

Общий task acceptance **по-прежнему не достигнут**: preserved user MCP baseline blocks обязательные all-module gates. Correctness result не подменяет аудит полноты, не делает deferred implemented и не разрешает release/issue closure. Root scope PASS не выдаётся за all-module PASS.

## Fresh whole-candidate review after authorized MCP migration — 2026-10-05

Предыдущий verdict выше исторический и не сертифицирует новый кандидат. Перечитаны task35, requirements matrix (не отчёт completeness), текущие public execution/migration contracts и code/diff: prepared boundary, built-in/wrapper/direct/nested Registry/Session environment, result cache/codec/replay metadata/audience, approval/claim/store/reconciliation/fencing, filejournal local durability/locks, terminal stream/consumer/provider completion, ordinary approval/terminal examples, изменённые MCP client/peer/stdio/POST SSE/routing/subscription paths. Host-authentication and immutable opaque data contracts проверялись как явно ограниченные порты; deferred profiles не выдаются за реализованные.

Независимые full tests текущего кандидата:

- root `go test -race ./... -count=1` — PASS, включая filejournal, examples/approval_journal и all root packages/codegen.
- MCP **полный** `go test -race ./... -count=1` — PASS 20.921s, оба examples включены, без выбора GoFiles/исключения old tests.

Эти результаты не отменили finding ниже. All-module Makefile gates запускал основной агент; их final result пока не подменяется двумя module PASS. Исходные 197 миграционных сценариев и denominator не пересчитывались этим correctness review. Runtime/tests аудитором в whole-candidate review не изменялись; external public proof scripts и этот отчёт — единственные новые изменения.

### MCP-COR-008 — P2: consumer abort оставляет custom pending Await/request незавершённым

Code: `mcp/client.go:1081–1083` Await goroutine использует caller ctx; progress consumer-abort branches `1090–1093` и `1098–1101` отправляют best-effort notification, но не отменяют invocation-owned context (его нет). Required CompletionPendingRequest не означает optional CancellablePendingRequest. Поддерживаемый custom prepared request может корректно выполнить Completion hooks при terminal и honour Await(ctx), не реализуя optional local CancelPending. Сервер не обязан подтвердить best-effort cancellation; с caller context.Background остаются request/await goroutine до global Close либо произвольного remote response, хотя Tool.Execute уже вернул aborted.

Независимый публичный `go run -race /private/tmp/toolsy-mcp-custom-abort-proof.go` — exit0:

```text
streamAborted=true originalCause=true activeAwaitAfterReturn=1 completionHooks=2
```

Transport реализует современный PrepareRequest/Deliver/Abort, discovery/tools-list current DTO и required Completion; tools/call выдаёт progress и оставляет Await до ctx cancel или transport Close. Yield возвращает consumer error. Два hooks — discovery/list, tool completion отсутствует. Await корректно выбирает ctx.Done; отсутствие cleanup обусловлено использованием неотменённого caller ctx, не нарушением интерфейса custom implementation. Proof defer закрывает client после измерения, ресурсы не оставлены. Это local request/goroutine leak, не доказанный повтор эффекта или authorization bypass; не надо лечить глобальным закрытием транспорта.

Remedy contract: proxy invocation owns cancellable child context, cleanup его отменяет при каждом exit, включая consumer abort. Preserve original consumer cause + ErrStreamAborted, parent host context values, current bounded single-owner detached wire cancellation и exactly-once completion/retirement. Invocation cancellation не является rollback и не даёт retry permission. Не требовать global Client.Close для освобождения aborted per-request Await.

Regression: custom PreparedRequest implements Completion but deliberately hides optional CancellablePendingRequest; tools/call blocks Await until its ctx ends; progress consumer abort → Execute returns original cause+stream marker, Await ends/completion retires route without global Close, zero result/late progress and at most one cancellation notification. Repeat with background parent, parent cancellation and already-terminal progress draining; pre-Deliver/no-completion abort remains no wire send.

### Current verdict

Новое finding MCP-COR-008 **OPEN**; кандидат не принят несмотря на full root/MCP race PASS. В просмотренных других активных execution paths новых подтверждённых P0/P1/P2 не обнаружено; это не обещание отсутствия неизвестных дефектов. Closed COR-001–007 root и MCP-COR-001–007 воспроизводимые guards сохранены на нынешних путях. Stale `docs/migration-task35.md` Remaining paragraph ещё ссылается на preserved MCP build conflicts; синхронизация публичной документации требуется после финальных gate results.

После исправления нужны оба независимых свежих final reviews и full gates на новом frozen candidate. Issue/release/commit не выполнялись.

## Fresh final re-audit after MCP-COR-008 — 2026-10-05

MCP-COR-008 **closed, independently verified**. Required Completion without optional Cancellable/Delivery/Claim now uses invocation-owned child context for Prepare/Await and cancels it at every exit. Await worker no longer sends automatic cancellation independently of the invocation loop. Parent cancellation and consumer abort retain their separate result/cause, wire notifications remain single-owner and detached, already terminal progress/result consumer failure does not manufacture another cancellation. Retirement still serializes with enqueue and precedes release of Await; buffered accepted pre-terminal progress still drains before result.

Repository `mcp/custom_pending_abort_migration_test.go` was added under explicit parent authorization, proved RED before runtime fix, then independently passed with other progress/cancellation/terminal cases `-race -count=10` (2.097s). Custom pending hides all three optional cancellation/delivery/ownership interfaces; first request has no remote terminal and Await honours its context. Test checks bounded Await cleanup, completion hook, no active progress route/late delivery, original cause+stream marker, no global Close, at most one notification with actual request ID and a working subsequent call. No production code was modified by the auditor. External public race proof now prints:

```text
streamAborted=true originalCause=true activeAwaitAfterReturn=0 completionHooks=3
```

The fresh whole-candidate review was repeated after this fix/style cleanup, not treated as only a targeted closure. Current authorization/final binding, private host proof/exported snapshot boundaries, nested scoped Session/Registry budget/view, replay privacy/effect marker, codec precision, atomic grant+claim/fencing, unknown/crash/reconciliation, terminal stream/provider fence and MCP protocol/transport ownership were rechecked against contracts and the current code. No new confirmed implementation findings were found in the implemented scope. Deferred capability profiles remain outside implementation claims; custom transports must honour Await contexts and Completion ordering, not ignore those interfaces.

Fresh independent **full** suites, without selecting/excluding files:

- root `go test -race ./... -count=1` — PASS; filejournal interprocess/crash, ordinary approval host, terminal/profile fixtures and all root packages/codegen included.
- MCP `go test -race ./... -count=1` — PASS 19.663s, both examples included.
- `git diff --check` — PASS.

All public adversarial execution proofs rerun, exit0: current final outer ACL denial/inner revoke, stored internal audience never expanded, immutable envelope/cache consumer abort, public FromSpec reserved-key rejection, scoped nested child with parent policy once, final host context/opaque proof under hostile Args mutation, exact dynamic numeric replay. MCP public proofs also rerun: Start failure Close once/listener closed, all three typed list subjects+cap+cause with secret-safe bounded reason and original interrupt precedence, actual HTTP Connect typed discovery limit32, unfinished SSE EOF no success, custom pending abort cleanup. These checks supplement code review; green tests alone are not claimed as proof of bug absence.

Audited source/test/fixture checksum stream SHA-256: `6fec3f60f73dd3444b625455a923276e63daf340f3659e90fff407abe756e77b`, from root `*.go`, filejournal `*.go`, MCP `*.go`, ordinary approval/terminal examples and execution fixtures. This identifies the checked candidate subset, not a whole-repository digest.

**Correctness verdict: PASS for the reviewed implemented candidate; zero open confirmed findings.** Root COR-001–007 and migration MCP-COR-001–008 are closed; chronological evidence remains in this report and the separate interim report. This is not blanket security certification or implementation of deferred cards. Overall task acceptance additionally requires independent completeness and successful final all-module `make test`/`make lint` on this same candidate, which the main agent owns. No release, issue closure or commit is authorized by this verdict.

### Final lint-only delta recheck

После предыдущего PASS independently reviewed extraction `completeToolCallChunk`: generation stale check → complete/input-required discriminator → result builder остаются в прежнем порядке, после final error mapping и accepted-progress drain, до consumer delivery. Invocation child cancellation, completion retirement и single notification ownership не перемещены и не ослаблены. UTF-8 fixture helper сохраняет malformed octet/correlation и positive ASCII control; nonfatal HTTP-handler assertions, context-first test helpers, blank embedded fields и unused nolint removals не меняют runtime контракты. Удаление unused isSubresourceURI не убирает resourceSubscriptionAllows/normalized equality/descendant checks.

Повторный **полный** MCP `go test -race ./... -count=1` — PASS 23.572s, examples included. `git diff --check` — PASS. Обновлённый source/test/fixture checksum stream тем же способом: `ed525e8ab0cca3cbefb3a30f8d26ba7a983f9ea668408ea68122a91491e0d369`. Новых подтверждённых findings нет; correctness PASS сохраняется. All-module final Makefile gate results остаются отдельным обязательным evidence основного агента.

### Final type/header-only delta

Локальный finalResult вынесен в unexported toolCallFinalResult с теми же raw/error fields; channel capacity1, Await worker и все branches неизменны. HTTP assertion использует CanonicalHeaderKey перед Header.Get, сохраняя case-insensitive HTTP contract. Independent actual MCP package selected custom-abort/progress/StreamableHTTP tests `-race -count=1` PASS 2.863s; `git diff --check` PASS. Обновлённый fingerprint `a8d130ca6e6eeb8377f075254e1e0568fe8de8de962a8b6dfead8c07df86c4a9`. Correctness PASS остаётся; pending final workspace gates сверху явно отделены от аудита.

Последующая окончательная test-only правка заменяет redundant CanonicalHeaderKey на canonical literal `Header.Get("Mcp-Protocol-Version")` (`mcp/transport_http_test.go:53`). HTTP Header.Get остаётся case-insensitive, expected ProtocolVersion и runtime не меняются. Independent exact test `TestStreamableHTTP_JSONVersionIgnoresSessionAndNeverDeletes`, actual MCP package `-race -count=1`: PASS 1.283s. Current fingerprint `b034e4f6763e8c1671ac27bc00b7affdaf7df4efd622dcf2de37f44d965e0c28`; correctness PASS, новых findings нет.
