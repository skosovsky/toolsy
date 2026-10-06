# D03 human review intent verification

Default request_human_review, payload human_review and WithReviewName/Description
replace approval-named conversational API without aliases. Actual ApprovalGrant /
OperationProfile authority remains separate and unchanged. Clarification payload
and callback/cancel behavior remain intact. Configuration1..MaxControlBytes enforces
that complete encodedJSON fits core's finite control delivery budget; default16KiB.

Parent humanmodule racecount3PASS2.238s and pinned humanlint0. Extra rootlint0
is recorded separately and is not labeled as modulelint. ExampleAsTools and
bound-action composition execute successfully. New AAA tests cover manifest,
custom options, exact64KiB/overlimits and cancellation. Existing escapedUnicode/
callback-failure/no-grant/hostgrant/replay/policyrevocation fixtures remain covered.
The protected action denies dispatch even after the review intent says user agreed.

Host owns identity authentication, exact bound operation challenge, grant issuance,
expiry, storage and current authorization. Review text/pause/reply has none of this
authority. The thin adapter has no issuer/store/action execution port. Old catalog
names, payloadkind and option APIs require explicit host migration.

Both independent reviewers accepted100% (five20% criteria), no unresolved detected
defects; reviewer-owned adversarial probe sources and logs are retained.
Limits: deterministic trusted-local operation store fixture, not live authentication
or remote grant persistence verification; no scheduler/durable continuation or
hardcallbackpreemption guarantee.
