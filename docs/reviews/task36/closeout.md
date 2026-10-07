# Task36 closeout

Issue CLOSED: https://github.com/skosovsky/toolsy/issues/4

Author migration notification: https://github.com/skosovsky/toolsy/issues/4#issuecomment-6035235399

Published implementation: https://github.com/skosovsky/toolsy/tree/v0.19.0

@skosovsky Реализация готова: независимая приёмка полноты — 100% (9/9 AC); независимая проверка корректности не обнаружила незакрытых дефектов в проверенном scope. Core сохраняет BYOT и не получает зависимостей от callers, policy engines или continuation runners.

Опубликованный результат: [v0.19.0](https://github.com/skosovsky/toolsy/tree/v0.19.0). [Контракт host dispatch](https://github.com/skosovsky/toolsy/blob/v0.19.0/docs/host-dispatch-contract.md), [runnable recipe](https://github.com/skosovsky/toolsy/tree/v0.19.0/examples/host_dispatch), [реальная optional integration](https://github.com/skosovsky/toolsy/tree/v0.19.0/examples/host_dispatch_integration), [migration before/after](https://github.com/skosovsky/toolsy/blob/v0.19.0/docs/host-dispatch-migration.md), [приёмка и evidence](https://github.com/skosovsky/toolsy/tree/v0.19.0/docs/reviews/task36).

Что теперь изменить в application glue:

1. Вместо вызова только `name+args` и возврата `outcome.Result, err` формировать `recipe.Request[Subject, Scope, Activity]`. Копировать исходный native tool-call ID в `CallID`; `OperationID` выдаёт trusted host для отдельного intent, `AttemptID` — для попытки, `ActivityIdentity` хранится в continuation. Повторная доставка сохраняет intent/activity, но получает новый provider CallID. Одинаковые name+args разных intents не означают одну операцию.
2. Установить `recipe.Profile` в registry и получать декларации из `dispatcher.Manifests()` того же restricted view. Передавать source schema без повторного inference и округления чисел; subject/scope, policy/dependency fingerprints и grant поступают из trusted host.
3. Заменить собственный batch loop одним host execution owner. Разбирать полный `Result.Outcome`, `Err` и `Decision`; только `continue` допускает следующий последовательный эффект. Approval/pause/yield/halt, unknown/in-progress и fault останавливают dispatch; deny/correction/business failure не запускают автоматический retry. Модели передавать только `Result.Model`.
4. Approval выдавать для фактически подготовленного challenge. Любая правка args/attachments, subject/scope, tool/schema/view/policy/dependency/activity требует новой авторизации; текущие ограничения применяются и при replay.
5. После потери доставки использовать `Inspect` как host-only diagnostic, а completed-result получать через текущий авторизованный Session replay. Unknown/in-progress разрешать только trusted reconciliation по внешнему evidence, без повторного handler вслепую. Reducer применять лишь для fresh effects; для crash-safe reducer использовать atomic outbox на стороне приложения.
6. Убрать ручную установку `toolsy.replay_source`: fresh producers теперь отклоняют зарезервированную provenance. Перевести positional `Chunk{...}` literals на keyed fields. Настоящие child replay chunks пересылать без изменения payload/effects/envelope/controls; private invocation-local snapshot не сериализуется и не является approval credential.

Релиз выполнен штатным `make release-break`. Проверки: все модули `make lint` и `make test` с race; root/recipe race; local и published semantic modes; release artifact verification и clear-break preflight. После публикации отдельно проверены actual root consumer с `GOWORK=off`, без replace и копирования recipe, и adversarial public consumer.

Закрываю issue как completed. Fixtures доказывают описанные локальные durability/replay границы; exactly-once внешних эффектов и production distributed durability не заявлены.
