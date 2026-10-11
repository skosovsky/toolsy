# Заменить собственный MCP SDK интеграцией с официальным Go SDK

Статус: **IN PROGRESS — миграция и PKG-01–PKG-08 реализованы; финальные проверки и независимое review расширенного scope выполняются.**
Дата: 11 октября 2026.

## Цель

Удалить самостоятельную реализацию MCP-протокола из `github.com/skosovsky/toolsy/mcp` и заменить её адаптером над `github.com/modelcontextprotocol/go-sdk/mcp`. Сохранить пользовательские возможности в пределах закреплённого SDK, явно описать его ограничения и сохранить обязательные границы исполнения. Добавить отдельное серверное направление для публикации инструментов toolsy через официальный SDK.

Причина: поддержка собственного JSON-RPC, wire DTO, discovery lifecycle, STDIO и HTTP/SSE дублирует официальный SDK. Полезность отдельных правил исполнения не оправдывает отдельный протокольный стек. Не переносить каждое историческое ограничение в новую обёртку только ради существующих тестов.

Задача разрешает реализацию после отдельного запуска. Создание этого файла не запускает миграцию, публикацию, GitHub issue или goal.

## Решение пользователя: контракт зависимости (11 октября 2026)

Этот раздел имеет приоритет над исходными требованиями точности и upstream-blocker ниже.
Пользователь изменил активный goal: использовать официальный опубликованный SDK без форка и принять **явно документированные и воспроизведённые ограничения SDK**. Ограничения чисел, raw-представления, неизвестных полей и публичных lifecycle hooks отделяются от дефектов адаптера. Подтверждённые characterization-тесты, комментарии со ссылками на существующие issues/PR (либо воспроизведение без выдуманного issue), документация и upgrade checks закрывают требование учёта ограничения, но не означают исправления SDK. Не обещать lossless migration.

Обязательные execution contracts — authorization, approval, current scope, replay, single-attempt execution, различимость outcome/uncertainty — остаются обязательными. Незаявленное отбрасывание данных адаптером, обход policy, повтор эффекта, гонка или неисправный lifecycle адаптера остаются blockers. SDK владеет протоколом и его validation; адаптер не создаёт парсер, обходной transport/Connection, fork или дублирующий runtime. Legacy удаляется после замещающих проверок. Финальная приёмка: 100% обновлённого обязательного scope и correctness APPROVE двумя независимыми reviewer на одном snapshot.

v1.8.0 закрепляется как текущий официальный release-кандидат. Upstream structuredContent fix [#1239](https://github.com/modelcontextprotocol/go-sdk/pull/1239) влит после релиза; схемы/metadata отслеживаются в [#1310](https://github.com/modelcontextprotocol/go-sdk/issues/1310) / [#1311](https://github.com/modelcontextprotocol/go-sdk/pull/1311). Generic AddTool argument precision: [#1201](https://github.com/modelcontextprotocol/go-sdk/issues/1201) / [#1302](https://github.com/modelcontextprotocol/go-sdk/pull/1302); server adapter использует raw Server.AddTool. Для JSON-RPC ID отдельный issue не подтверждён, сохраняется локальное воспроизведение.

## Исходная база и доказательства

- Исследованный HEAD toolsy: `4f7eae6cc2393da259cf30a5535bef12c93d6e2c`. Перед реализацией проверить актуальный HEAD и delta, сохранить пользовательские изменения.
- Исследован официальный SDK `v1.8.0`, Origin `3f3b699b2b67e1ed033a63d6651671dab53c2d32`, module sum `h1:KIvahhYqwtbeniWVPs3TcXEA7b8jEtwfBpOTAI+Urx4=`. Проверка официального списка релизов и Go @latest подтвердила v1.8.0; версия закреплена в модуле и SDK-only fixture.
- Реализация baseline (до миграции): `mcp/README.md`, `mcp/client.go`, `mcp/protocol.go`, `mcp/peer.go`, `mcp/transport*.go`, `mcp/policy.go`, `mcp/tool_discovery.go`, `mcp/content.go`, `mcp/snapshot.go`, `mcp/extensions.go`.
- Контракты ядра: `docs/execution-contract.md`, `docs/result-contract.md`, `docs/control-contract.md`, `docs/policy-gates.md`, `docs/public-api-contracts.md`.
- Семь исследовательских тестов прошли на неизменённом SDK: импорт SDK tool в NewProxyTool; экспорт локального tool через Server.AddTool; manual/automatic MRTR; кеш и aliasing каталога; fallback версии; fidelity JSON; permissive result decoder. Это characterization и минимальный smoke, не полная приёмка миграции.
- Временный материал исследования: `/private/tmp/toolsy-sdk-research/probes/sdk_test.go`. Временный файл не является зависимостью задачи: ниже приведены входы и ожидаемые наблюдения для воспроизведения в репозитории.

Официальные источники:

- https://github.com/modelcontextprotocol/go-sdk/releases/tag/v1.8.0
- https://github.com/modelcontextprotocol/go-sdk/blob/v1.8.0/mcp/client.go
- https://github.com/modelcontextprotocol/go-sdk/blob/v1.8.0/mcp/server.go
- https://github.com/modelcontextprotocol/go-sdk/blob/v1.8.0/mcp/mrtr.go
- https://github.com/modelcontextprotocol/go-sdk/blob/v1.8.0/mcp/cache.go
- https://github.com/modelcontextprotocol/go-sdk/blob/v1.8.0/mcp/transport.go
- https://github.com/modelcontextprotocol/go-sdk/blob/v1.8.0/internal/jsonrpc2/messages.go

## Область работ

Обязательны два независимых направления в optional integration module:

1. **Client import:** SDK client/session → удалённые tools как `toolsy.Tool` → обычные RegistryView/Session execution paths.
2. **Server export:** SDK Server → scoped executor toolsy → доменные handlers. Это новая возможность, не существующий функционал старого клиента.

Предложить минимальное разделение файлов или подпакетов внутри `toolsy/mcp`; точные имена публичных API определить в спецификации. Не создавать собственный SDK-фасад поверх всего SDK.

Не входит: переписывание fb-mcp/tg-mcp/wa-mcp/zl-mcp, доменные политики мессенджеров, агентный runtime, UI подтверждений, собственные OAuth flow, tasks scheduler, публикация релиза и отправка upstream PR без отдельного поручения. Конкретные приложения могут служить read-only источником требований и локальных consumer fixtures.

## Архитектурный контракт

### Владение и зависимости

- Ядро toolsy не импортирует MCP SDK. Optional module зависит от ядра и официального SDK.
- SDK владеет JSON-RPC, транспортами, wire DTO, version negotiation, протокольными уведомлениями, заголовками и OAuth-инфраструктурой.
- Адаптер владеет отображением SDK tools/results/errors в toolsy и обратно. Предпочитать SDK-типы вместо второго набора MCP DTO. BYOT сохраняется для authenticated subject/scope и доменных данных.
- Host создаёт transport, конфигурирует SDK и владеет соединением. Заимствованную сессию адаптер не закрывает. Закрытие адаптера останавливает только его работу; закрытие SDK connection корректно завершает ожидающие вызовы адаптера.
- Нельзя ограничиться Attach к уже открытой session и одновременно обещать полную поддержку progress/invalidation. Предусмотреть composition API до Connect для callbacks/middleware и последующее связывание session.
- Обработчики host сохраняются: композиция без молчаливой перезаписи; порядок вызовов, ошибки и отмена документированы. Изоляция по session и уникальному progress token обязательна при совместном клиенте.
- Не копировать весь ClientOptions в собственные WithX. Использовать штатные options и узкие настройки адаптера.
- Никаких reflection/unsafe/private API SDK. Обёртки Connection не считать прозрачными автоматически: SDK использует непубличный sessionUpdated hook; проверить влияние wrapping на HTTP lifecycle и negotiation.

### Контракт исполнения

- Импортированный инструмент исполняется через существующий prepared execution path. Текущая authorization/policy применяется перед dispatch и replay. Адаптер не создаёт вторую систему approval/journal.
- Remote annotations — данные, не разрешение. Host явно задаёт execution classification. Опасность не означает автоматическое подтверждение без соответствующей policy.
- Remote result не может подделать внутренние effects/controls, approval proof, provenance или replay metadata.
- CallID приложения, JSON-RPC ID SDK, logical operation ID и progress token — разные идентичности; не смешивать их.
- Отмена, timeout, разрыв связи или ошибка проверки результата не доказывают отсутствие внешнего эффекта и не разрешают повторный dispatch.
- Ошибки transport/protocol, `isError`, schema validation, cancellation, unsupported capability и input-required остаются различимыми. Причины доступны через errors.Is/As; секреты не попадают в diagnostics.

## Матрица возможностей и назначение

Для каждой строки перед кодированием зафиксировать current API → target API → owner → invariant → executable evidence. Статусы: preserved, delegated, intentionally changed с обоснованием, blocked. Непроверенная строка не равна preserved.

| Возможность | Целевое размещение и требование |
| --- | --- |
| Discovery tools, pagination | SDK list APIs; адаптер создаёт согласованный набор tools, учитывает cursor loops, дубли, отмену и лимиты совокупной выдачи |
| Input/output schema | Сохранять полученную SDK схему, required/default/null/zero и dialect; numeric fidelity ограничена SDK и фиксируется тестами; не генерировать параллельную схему |
| Typed proxy tools | NewProxyTool либо существующий подходящий constructor; validation до dispatch и проверка результата до успешной доставки |
| Text/image/audio/resource_link/embedded resource | Сохранять порядок, MIME, bytes, annotations и типизированные SDK blocks; текст — только производная проекция |
| structuredContent | Сохранять SDK JSON значения; rounding и missing/null ограничения SDK документируются и тестируются; не stringify всё в текст |
| Resources и templates | Доступны через SDK; сохранить ресурс как tool там, где это текущая возможность |
| Prompts | SDK list/get; не дублировать весь prompt client ради прежних имён методов |
| Progress | SDK notifications → toolsy progress; изоляция параллельных calls, bounded delivery и terminal ordering |
| Cancellation | SDK cancellation и child contexts; consumer abort освобождает свою работу без закрытия общей session |
| MRTR/input_required | Явный промежуточный исход; single-attempt adapter не запускает скрытый цикл; host-controlled continuation сохраняет requestState/inputResponses |
| Invalidation/subscriptions | SDK notifications; определённые правила обновления imported registry, ошибок подписки и отсутствия subscription capability |
| Cache | Разделить discovery TTL SDK и operation/result cache toolsy; scope/connection identity не выводить из cacheScope |
| Snapshot identity | Явная версия и область хеширования; не называть digest SDK DTO прежним полным wire digest |
| Extensions и _meta | Сохранить поддерживаемые metadata/extension payload; неизвестное не активирует runtime; неподдерживаемая сохранность явно выявляется |
| HTTP authentication | SDK OAuth либо host HTTP client; сохранить безопасную диагностику, не добавлять скрытый business retry |
| HTTP routing headers | SDK отвечает за Mcp-* и x-mcp-header mapping; conformance fixture проверяет actual wire |
| Network/resource limits | Настройки SDK и host HTTPClient/httptool; лимиты очереди/параллелизма — слой admission, не новый transport |
| STDIO/process lifetime | SDK transport либо host supervisor; отдельно проверить process tree, stderr, limits и graceful/forced shutdown |
| Version negotiation | Штатная negotiation; exact-version restriction только при явной необходимости и документированном контракте |
| Server export | Отдельный адаптер с explicit schema, scoped execution, one terminal result и контролируемой delivery projection |

Исторические API можно удалить с ясной breaking migration. Потеря способности и изменение способа вызова — разные вещи. Нельзя незаметно снять существующую гарантию, объявив её «не нужна»: указать изменение, последствия и получить решение пользователя, если оно меняет согласованные требования.

## Подтверждённые ограничения SDK и необходимые решения

### A. Точность JSON и сохранность представления

На SDK v1.8.0 воспроизведено:

- `jsonrpc.DecodeMessage` → EncodeMessage для `{"jsonrpc":"2.0","id":9007199254740993,"method":"probe"}` выдаёт ID `9007199254740992`.
- Decode/encode CallToolResult с `structuredContent:{"id":9007199254740993}` выдаёт `9007199254740992`.
- Неизвестное верхнеуровневое поле `vendorField` результата исчезает при round-trip.

Проверить также числа в schema constraints, _meta, extension payload; missing/null, exponent forms, string-vs-number IDs. JSON-RPC ID и числа доменного результата тестировать отдельно.

Нельзя восстановить raw точность повторным json.Marshal уже декодированного DTO. Предпочтение: исправленный опубликованный SDK или upstream-compatible минимальное изменение raw/number handling. Ограничения неизменённого SDK документировать и воспроизвести; согласно обновлённому goal они не блокируют завершение. Не прятать fork в адаптер. Временный patch/fork требует явного решения, точного pin, diff, regression tests и условия удаления. Патч ID из tg-mcp — reference, не доказательство исправления structuredContent и остальных чисел.

### B. Cache и aliasing

Сервер с TTL 60000/private: два ListTools делают один network request. Изменение Description первого результата видно во втором; возвращается общий указатель.

Адаптер копирует изменяемый snapshot и не передаёт shared SDK state для мутации. Определить freshness, invalidation, смену identity и необходимость force-refresh; проверить публичный способ управления кешем. Не выключать cache путём молчаливой порчи ttlMs в переданных host данных. Недоступный bypass — API gap, а не повод копировать client.go.

### C. MRTR и versions

Сервер в первом ответе возвращает пустой InputRequests и requestState, во втором terminal. Default client вызывает handler дважды; Disabled:true — один раз с NeedsInput и сохранённым state. Использовать штатное отключение для single-attempt режима; отсутствие автопродолжения подтвердить тестом.

Сервер, поддерживающий только 2025-11-25, успешно подключается к клиенту, предпочитающему новейшую редакцию. ProtocolVersion — preference, не no-fallback guarantee. Capability gating не заменять сравнением строк версий там, где достаточно фактической capability.

### D. Strict decoding и расширения

Прямой decoder CallToolResult в v1.8.0 принимает missing resultType, unknown resultType и duplicate JSON key. Это не доказательство обхода всех проверок полного connection path. Повторить на actual wire и отделить обязательную защиту от старых ограничений. Повторная валидация SDK DTO не обнаружит уже потерянные duplicate keys и unknown fields.

### E. Progress и subscription lifetime

У старого клиента есть PrepareRequest/Deliver и OnComplete до Await, явный ACK/filter/subscription ID. Публичный API SDK не следует считать эквивалентным без тестов. Проверить callbacks до отправки, progress после terminal, buffered progress, consumer abort, late notifications, ACK, listener failure и shutdown. При реальном gap предпочесть небольшой upstream hook; не писать новый peer/JSON-RPC dispatcher.

### F. Supervisor и transport ownership

SDK CommandTransport завершает child process; это само по себе не доказывает завершение всех descendants. Если требуется прежняя гарантия дерева процессов, использовать host supervisor плюс SDK IO transport и проверить платформы. Настроить лимиты конкретного используемого transport: опция StdioTransport не означает её наличие у CommandTransport.

## Серверный адаптер

- Использовать `Server.AddTool` с явными input/output schemas и raw arguments; он оставляет domain validation вызывающей стороне. Generic AddTool не применять автоматически, если это меняет исходную schema или numeric/default semantics.
- Handler вызывает host-provided scoped RegistryView/Session executor. Прямой tool.Execute допустим только как тест минимального стыка, не как обход policy в production adapter.
- Authenticated subject/scope определяет host через транспортный контекст; модель не может передать себе права через arguments/_meta.
- Discovery и invocation согласованы по scope: скрытый tool недоступен прямым tools/call. Общий Server не должен раскрывать другой tenant catalog; выбрать и проверить штатный SDK механизм изоляции.
- Не публиковать весь mutable registry по умолчанию. Explicit export selection и повторная authorization перед фактическим dispatch обязательны.
- Отобразить one terminal result, empty result, soft error и protocol failure. Внутренние effects/controls/provenance не сериализуются произвольно клиенту.
- Для approval/pause использовать host mapping в поддерживаемый MCP workflow с привязкой к точным args/identity; inputResponses или текст модели не являются grant. Неподдерживаемую комбинацию отклонять до effects.
- Для independent multi-result streams, attachments и прочих конструкций без прямого MCP соответствия явно определить projection/aggregation либо отказ на регистрации. Не отбрасывать результаты молча.
- Progress использовать при наличии запроса/токена; transport delivery failure после effects оставляет корректную uncertainty и не вызывает resend.
- Поставить examples import/export и consumer fixtures без Telegram/Zalo credentials.

## План реализации

- [x] P0. Сверить HEAD, SDK release, правила репозитория и потребителей; составить inventory public API/тестов и выбрать версию SDK.
- [x] P1. Написать design/contract и полную матрицу покрытия: ownership, data fidelity, errors, capabilities, breaking migration и server export boundary.
- [x] P2. Перенести исследовательские reproductions в устойчивые AAA fixtures. Зафиксировать принятые ограничения SDK, upstream-ссылки и upgrade checks; закрыть дефекты адаптера без ложных гарантий fidelity.
- [x] P3. Реализовать client adapter на публичном SDK API с pre-Connect composition, schemas, results/errors, progress и lifecycle.
- [x] P4. Реализовать server adapter со scoped execution и согласованным discovery; проверить authority и projection отдельно от сетевого протокола.
- [x] P5. Перенести contract tests, добавить HTTP/STDIO interoperability, two-client concurrency, consumer cancellation, invalidation и admission/approval/replay tests.
- [x] P6. Удалить старые peer, transports, protocol DTO и codec implementations, лишние зависимости, obsolete fixtures/examples/docs. Не оставлять два runtime backend.
- [x] P7. Обновить public docs, module metadata, migration guide, runnable examples; выполнить все применимые checks и подготовить итоговый evidence report.
- [ ] P8. Запустить двух независимых review-субагентов: полнота в процентах и ошибки/регрессии. Исправить замечания, повторить затронутые проверки и получить оба финальных verdict на одном конечном snapshot.

Не удалять старую реализацию до появления необходимых замещающих проверок. Она может временно быть differential oracle, но не единственным источником истины: ожидаемое поведение берётся из нового контракта и MCP specification.

## Проверки

Все новые tests — Arrange–Act–Assert, без реальных аккаунтов и external effects.

- Import/export через настоящий SDK по HTTP и STDIO; in-memory smoke отдельно.
- Все content types, порядок блоков, MIME/binary payload, structured output object/array/scalar/null/missing, exact numeric fixtures.
- Input/output schema parity; bad input — zero dispatch; bad output после эффекта — no automatic repair/retry.
- isError vs protocol/network faults, error cause, cancellation и unsupported capability.
- MRTR terminal/input-required/manual continuation; исходные params не мутируются; отсутствуют скрытые дополнительные вызовы.
- Discovery pagination bounds/duplicates/cursor loops; snapshot ownership; cache isolation; invalidation до/во время вызова и replay.
- Concurrent calls/sessions, callback composition, progress ordering, consumer abort, disconnect/shutdown, bounded queues и cleanup.
- Server hidden tool direct call, cross-scope access, spoofed identity/grants, изменённые args после approval, replay после revoke.
- Registry policies/operation journal продолжают работать; повтор операции не приводит к повторному внешнему эффекту.
- SDK upgrade fixtures для обнаруженных blockers, включая точность ID и structuredContent отдельно; это characterization принятых ограничений, а не обязательное исправление SDK.
- Root и nested modules: `make test`, `make lint`, применимые integration/race/build targets; сверить фактический Makefile. Root `go test ./...` не покрывает nested modules.
- Consumer graph с GOWORK=off: не прятать зависимости и несовместимость под workspace replace. Обновление pin проверяется теми же regression fixtures.

## Независимая приёмка двумя субагентами

После реализации и обязательных проверок запустить **двух отдельных субагентов**. Оба получают эту задачу, контракты, фактический diff, доступ к исходникам и evidence проверок одного snapshot. Они не участвуют в реализации и не получают выводы друг друга до выдачи собственных отчётов. Отчёт основного агента не заменяет самостоятельное чтение кода и проверку evidence.

### Субагент 1 — полнота выполнения в процентах

- Разложить все обязательные требования задачи, матрицу возможностей и критерии завершения на проверяемые пункты со стабильными ID; зафиксировать знаменатель до оценки результата. Не считать только отмеченные автором чекбоксы.
- Для каждого пункта указать статус, ссылки на реализацию и проверки, недостающую работу. Статусы: выполнено и подтверждено, частично, отсутствует, заблокировано, не проверено.
- Рассчитать **полноту = 100 × число полностью выполненных и подтверждённых обязательных пунктов / общее число обязательных пунктов**. Показать числитель, знаменатель, процент и разбивку по client import, server export, fidelity, lifecycle, cleanup, tests и docs. Частичные, blocked и непроверенные пункты не засчитывать как выполненные.
- Исключение из scope допускается только по явному решению пользователя со ссылкой на него; нельзя уменьшать знаменатель ради 100%. Исторические необязательные ограничения не превращать в новые требования; их disposition всё равно должен быть зафиксирован.
- Отдельно перечислить все gaps и выдать verdict: COMPLETE только при 100% подтверждённой полноте обязательного scope; иначе INCOMPLETE с конкретными условиями завершения. Процент отражает покрытие требований, а не вероятность отсутствия ошибок.

### Субагент 2 — ошибки и регрессии

- Независимо проверить корректность кода, границы SDK/adapter/core, данные и схемы, ownership, конкурентность, cancellation/progress, subscriptions/cache, authorization/approval/replay, ошибки после внешнего эффекта и server scope isolation.
- Использовать adversarial contexts: недоверенный сервер, malformed payload, смена каталога/identity, late notifications, одновременные вызовы, consumer abort и неизвестный исход. Проверить, что тесты подтверждают заявленные гарантии, а не только повторяют реализацию.
- Каждый finding содержит severity, файл и строку, сценарий воспроизведения либо проверяемое обоснование, влияние и условие исправления. Предположения и непроверенные риски отделять от подтверждённых дефектов.
- Выдать APPROVE либо REQUEST_CHANGES с явным списком оставшихся дефектов и ограничений проверки. Отсутствие findings не считать доказательством полной безошибочности.

### Исправления и повторная проверка

Основной агент устраняет gaps и подтверждённые дефекты, запускает затронутые tests и возвращает изменения соответствующему reviewer. Оба reviewer подтверждают финальный snapshot: SHA, а при uncommitted changes — идентификатор проверенного diff. После содержательных правок прежние verdict нельзя выдавать за ревью нового состояния.

Завершение требует **100% полноты и APPROVE по корректности**, без неразрешённых подтверждённых дефектов в scope. Спорные замечания разрешать evidence, с фиксацией решения reviewer; не скрывать их. В итоговом отчёте привести оба независимых отчёта, их snapshot, список исправлений и результаты recheck. Подтверждённый дефект адаптера или execution contract оставляет задачу незавершённой. Принятое документированное ограничение SDK само по себе не блокирует приёмку.

## Критерии завершения

- [x] Единственный production MCP protocol engine — официальный SDK; собственные peer/JSON-RPC/HTTP/SSE/STDIO реализации удалены.
- [x] Core toolsy остаётся независимым от SDK; import/export находятся в optional module и имеют runnable examples.
- [x] Все текущие публичные возможности имеют disposition и проверяемое соответствие; нет незаявленных потерь адаптера; ограничения SDK отдельно документированы и воспроизведены; effects/control outcomes защищены.
- [x] Подтверждённые data-loss ограничения выбранного SDK документированы, тестируются и отделены от обязательных execution contracts; fork/обходов нет.
- [x] Server discovery/dispatch соблюдают один scope, authorization не выводится из model input, callbacks и shared sessions изолированы.
- [x] SDK negotiation/OAuth/subscriptions остаются доступны host без искусственного урезания адаптером.
- [x] Legacy cleanup завершён, tests не удалены ради green без заменяющего контракта, все обязательные checks PASS.
- [x] Документированы breaking changes, новый ownership, гарантии и ограничения; итоговый отчёт различает проверенное, непроверенное и blocked.
- [ ] Два независимых review-субагента проверили один финальный snapshot: полнота 100% с числителем/знаменателем, correctness APPROVE; замечания закрыты и recheck подтверждён.
- [x] Не выполнены незапрошенные publish/tag/push, GitHub issue/comment, изменения соседних приложений или permanent upstream fork.

## Goal на реализацию

Активный goal пользователя от 11 октября 2026 заменил исходный запрет завершения при ограничениях upstream: использовать официальный опубликованный SDK без форка, документировать и тестировать его ограничения, сохранить execution contracts и получить двойную независимую приёмку. Исходный текст goal удалён, чтобы не оставлять конфликтующий контракт.

## Дополнение от 11 октября 2026 — интеграция всех пакетов официального SDK

Это дополнение расширяет обязательный scope задачи. Новый goal в конце файла является актуальной формулировкой для реализации. При противоречии с более ранними формулировками действуют следующие решения: использовать официальный опубликованный SDK без форка; известные ограничения SDK документировать и проверять, а не устранять собственной реализацией протокола. Принятые upstream-ограничения не блокируют завершение сами по себе. Дефекты адаптера и нарушение обязательных execution contracts не относятся к этому исключению.

### Распределение ответственности между пакетами

Все четыре пакета принадлежат одному модулю `github.com/modelcontextprotocol/go-sdk` и выбираются согласованной версией этого модуля. Не требуется искусственно импортировать каждый пакет в production adapter: часть композиции принадлежит host и проверяется examples/consumer fixtures.

| Пакет | Использование | Что не реализуем повторно |
| --- | --- | --- |
| `mcp` | Client/Server, session, SDK transports, tools/results, resources/prompts, callbacks, negotiation; публичные API для обоих направлений адаптера | MCP lifecycle, wire DTO, SSE/STDIO/HTTP protocol stack |
| `jsonrpc` | Публичные structured error/message types там, где они необходимы; проверка сохранения error code/data/cause на границе SDK и adapter | Собственный peer, dispatcher, parser или transport только ради обхода SDK |
| `auth` | Host подключает штатный OAuthHandler для клиента и bearer middleware с TokenVerifier для HTTP-сервера; adapter совместим с этой композицией | Собственный OAuth client flow, refresh machinery, token verification framework |
| `oauthex` | SDK metadata/discovery/registration primitives и ProtectedResourceMetadata; преимущественно host configuration/examples | Свои OAuth metadata DTO, WWW-Authenticate parser, discovery и registration HTTP-клиенты |

Проверить API выбранного релиза при реализации. Исследованная база v1.8.0 содержит `auth.RequireBearerToken`, `auth.TokenVerifier`, `auth.TokenInfoFromContext`, `auth.ProtectedResourceMetadataHandler`, `auth.OAuthHandler` и `oauthex.ProtectedResourceMetadata`. Это точки композиции, а не требование оборачивать каждую из них новым API toolsy.

Источники:

- https://pkg.go.dev/github.com/modelcontextprotocol/go-sdk/mcp
- https://pkg.go.dev/github.com/modelcontextprotocol/go-sdk/jsonrpc
- https://pkg.go.dev/github.com/modelcontextprotocol/go-sdk/auth
- https://pkg.go.dev/github.com/modelcontextprotocol/go-sdk/oauthex
- https://github.com/modelcontextprotocol/go-sdk/tree/v1.8.0/auth

### Клиентская OAuth-композиция

- Приложение создаёт и конфигурирует штатный SDK OAuthHandler/transport. Adapter принимает SDK-композицию без удаления handler, замены HTTPClient или скрытого повторного создания connection.
- Приложение владеет token persistence, client credentials, пользовательским consent/browser callback, выбором authorization server и сетевыми настройками. Adapter не сохраняет токены и не запускает браузер самостоятельно.
- Проверить штатные challenge/discovery и refresh paths выбранного SDK, а также отказ авторизации, отмену и ошибку refresh. SDK-owned authorization round trips отделять от запрещённых adapter-owned повторных business dispatch. Не обещать отсутствие любых HTTP повторов при включённом OAuth; явно описать поведение SDK.
- Проверить число фактических вызовов бизнес-handler в успешном auth flow и при ошибках. При неизвестном исходе эффектов adapter не инициирует resend.
- OAuth/auth ошибки остаются доступными для обработки через SDK-типы либо сохраняемую cause; ошибки tool и MRTR не подменяются auth failure. Секреты не попадают в error text, log, snapshots или MCP content.
- Смена identity/credentials не должна оставлять imported catalog и scope от другой учётной записи. Зафиксировать безопасный lifecycle сессии/кеша при смене principal; не считать cacheScope авторизацией.

### Серверная auth-композиция

Рекомендуемая граница:

```text
HTTP request
  → SDK bearer middleware + host TokenVerifier
  → verified token information
  → host mapping в authenticated subject/scope
  → scoped RegistryView/Session executor toolsy
```

- `TokenVerifier` и domain mapping поставляет host. SDK middleware предоставляет результат проверки, но не определяет бизнес-права, tenant и правила approval.
- Metadata endpoint использует SDK `ProtectedResourceMetadataHandler` и `oauthex.ProtectedResourceMetadata`. Адреса/issuer/scopes задаёт host; adapter не становится authorization server.
- Валидный access token не означает разрешение любого инструмента. Проверки discovery и calls согласованы с scoped executor; видимость одного tenant не раскрывает каталог другого.
- Поддельные subject/scope/token claims в arguments или model-supplied `_meta` не перекрывают verified identity.
- Без токена, с неверным/истёкшим токеном либо недостаточными правами бизнес-handler не вызывается. Проверку issuer/audience/scopes реализует и конфигурирует host verifier согласно выбранной auth-схеме; не приписывать middleware автоматическую проверку всех этих условий.
- OAuth scopes и human approval — разные механизмы. OAuth consent не заменяет grant на конкретную операцию; inputResponses не выдаёт полномочий самостоятельно.
- HTTP auth применяется только в соответствующей HTTP-композиции. Для STDIO описать доверие к локальному процессу/host, не навязывать HTTP bearer flow этому transport.

### Обязательные примеры и проверки дополнения

Все fixtures используют synthetic credentials и локальные test servers, без реальных IdP, аккаунтов, браузера и внешней отправки сообщений.

- [x] PKG-01. В design/матрице отражены все четыре пакета и их owners; прямые импорты обусловлены использованием, core toolsy не получает MCP/auth dependencies.
- [x] PKG-02. Client example совмещает adapter, штатный SDK OAuthHandler и host transport settings. Integration fixture подтверждает штатный auth flow и отсутствие дополнительного бизнес-dispatch со стороны адаптера.
- [x] PKG-03. Проверены challenge/discovery, refresh success/failure, отказ и cancellation выбранной OAuth-композиции; token persistence остаётся у host, секреты не выдаются.
- [x] PKG-04. Server example использует SDK bearer middleware и protected resource metadata; verified identity преобразуется host в subject/scope и доходит до executor.
- [x] PKG-05. Negative tests подтверждают zero business dispatch без необходимых полномочий, изоляцию двух principals, невозможность подмены identity и обхода approval.
- [x] PKG-06. JSON-RPC/protocol errors сохраняют нужные code/data/cause при mapping, без самописного JSON-RPC backend и без утечки секретов.
- [x] PKG-07. STDIO import/export работает без HTTP OAuth; документированы ownership и различия trust boundaries.
- [x] PKG-08. Upstream limitations заведены с version pin, reproducer, ссылкой на issue/PR при наличии и условием перепроверки при upgrade. Characterization acceptance не выдаётся за исправление SDK.

Эти пункты входят в обязательный знаменатель completeness review. Корректность OAuth/identity composition отдельно проверяет второй reviewer. Принятое ограничение SDK допускается только с явным описанием влияния и evidence; оно не оправдывает ошибку интеграции или потерю данных, внесённую самим adapter.

### Уточнение известных upstream-проблем

Статусы ниже проверены 11 октября 2026; при реализации проверить повторно:

- `structuredContent`: issue https://github.com/modelcontextprotocol/go-sdk/issues/1236 закрыто; PR https://github.com/modelcontextprotocol/go-sdk/pull/1239 merged 16 сентября 2026, после релиза v1.8.0 от 14 сентября. Не считать наличие fix в main наличием fix в выбранном релизе.
- Schemas/metadata: https://github.com/modelcontextprotocol/go-sdk/issues/1310 и https://github.com/modelcontextprotocol/go-sdk/pull/1311 были открыты.
- Typed arguments: https://github.com/modelcontextprotocol/go-sdk/issues/1201 и https://github.com/modelcontextprotocol/go-sdk/pull/1302 были открыты. Это отдельный путь от низкоуровневого Server.AddTool.
- JSON-RPC ID: локальное воспроизведение есть; отдельный upstream issue проведённым поиском не найден. Не приписывать этот дефект issue о structuredContent.

Использовать официальный опубликованный релиз. Не закреплять unreleased main/pseudo-version как скрытый обход решения о релизной зависимости. Не создавать fork, vendor patch, replacement decoder или перехват тела ответа для восстановления потерянных чисел. Если исправление вошло в новый официальный релиз, обновить pin, fixtures и документацию ограничений.

## Новый goal на реализацию с учётом всех пакетов SDK

> Реализуй `.cursor/tasks/mcp-official-sdk-integration.md` в toolsy, включая дополнение по пакетам `mcp`, `jsonrpc`, `auth` и `oauthex`. Начни со сверки текущего HEAD, потребителей и актуальных официальных релизов. Работай Spec/Contract-First: актуализируй контракт и критерии задачи согласно решению использовать официальный опубликованный SDK без форка, принимая документированные ограничения зависимости.
>
> Удали собственную реализацию MCP-протокола и замени её интеграцией с `github.com/modelcontextprotocol/go-sdk`. Сохрани `toolsy/mcp` как optional module с независимыми client import и server export направлениями. Используй публичные SDK API и типы, сохрани BYOT и независимость core. Не создавай второй протокольный стек, собственный парсер, обходной transport или копию SDK options.
>
> Составь и закрой матрицу возможностей: tools, schemas, content, resources, prompts, progress, cancellation, subscriptions, MRTR, errors, lifecycle и auth composition. `mcp` используй для protocol/client/server; `jsonrpc` — для необходимых публичных error/message types без нового transport; `auth` и `oauthex` — через штатную host-композицию OAuth, bearer verification и resource metadata. Не требуй прямого импорта каждого пакета в adapter, если его место в examples или host. Поддержи pre-Connect composition callbacks и последующее binding session без перезаписи host handlers и без присвоения владения заимствованным connection.
>
> Сохрани обязательные authorization, approval, scope и replay contracts. В server export передавай проверенную identity через host mapping в scoped executor; не доверяй model arguments/_meta как источнику полномочий. Разделяй OAuth consent, access scopes и approval конкретной операции. Token storage, credentials, user interaction и выбор IdP принадлежат host. Проверь client/server OAuth-композицию на synthetic fixtures и отдельную STDIO-композицию без HTTP OAuth. Не добавляй собственные auth flows или скрытый повтор business dispatch.
>
> Выбери и закрепи официальный релиз SDK. Известные upstream-ограничения, включая точность чисел, отрази в документации, комментариях у затронутого кода и characterization-тестах с issue/PR и version pin; если issue не найдено, укажи reproducer без выдуманной ссылки. Не делай fork, vendor patch, replacement decoder или unreleased pin ради обхода SDK. Принятые upstream-ограничения сами по себе не блокируют завершение, но не обещай отсутствующие гарантии и не списывай дефекты adapter на SDK. Предусмотри перепроверку и снятие ограничений при обновлении официального релиза.
>
> Добавь AAA contract/regression/interoperability tests, runnable import/export и auth examples, проверки HTTP/STDIO, scopes, error mapping, cancellation, lifecycle и GOWORK=off consumer graph. Выполни применимые проверки root и nested modules. Удали legacy protocol runtime, лишние зависимости и устаревшую документацию; опиши breaking changes и ownership.
>
> После реализации запусти двух независимых субагентов: первый оценивает полноту обязательных требований, включая PKG-01–PKG-08, в процентах с фиксированным знаменателем и evidence; второй ищет ошибки и регрессии, включая auth/identity boundaries. Исправляй замечания и получи 100% полноты и correctness APPROVE на одном финальном snapshot. Принятые ограничения SDK показывай отдельно: их документирование и тестирование закрывает соответствующее требование, но не означает исправления upstream.
>
> Не публикуй релиз, не создавай внешние issues/PR и не меняй соседние MCP-приложения. Заверши отчётом о перенесённых возможностях, проверках, verdict обоих reviewers и ограничениях выбранной версии SDK.

## Историческая приёмка scope до дополнения PKG-01–PKG-08

До обновления goal независимые reviewer подтвердили полноту 40/40 = 100% и correctness APPROVE на одном candidate; четыре подтверждённых дефекта адаптера устранены и независимо воспроизведены повторно. Итоговые snapshot/evidence зафиксированы в [completeness review](../../docs/reviews/mcp-sdk-integration/completeness-review.md) и [correctness review](../../docs/reviews/mcp-sdk-integration/correctness-review.md).

PASS: make test и make lint для 27 модулей, integration/e2e, go mod verify, GOWORK=off consumers/artifacts, оба runnable examples, dependency isolation и diff check. Полный e2e candidate проверен на точной локальной temporary tracked-source копии; индекс исходного репозитория не изменён. Подробнее: [verification notes](../../docs/reviews/mcp-sdk-integration/verification-notes.md).

SDK v1.8.0 используется без fork и без protocol workaround. Numeric/raw/null/unknown-field, cache и lifecycle ограничения — принятые ограничения зависимости; отсутствие lossless гарантий отражено в migration docs и characterization tests. Публикаций и внешних изменений нет.

Обновлённый scope реализован, но финальная приёмка ещё выполняется: прежние verdict не распространяются на PKG-01–PKG-08. Фиксированный знаменатель нового reviewer — 48 (40 исходных + 8 новых требований).
