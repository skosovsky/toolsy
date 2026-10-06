# D03 independent acceptance A

Baseline HEAD: c682e30. Reviewed current tracked diff and untracked human/example_test.go, human/review_contract_test.go. No implementation participation, delegation or access to acceptance B verdict.

## Criteria (20% each)

1. **20/20**: default tool request_human_review, kind human_review, public WithReviewName/WithReviewDescription. Legacy symbols removed without aliases. Old name appears only in negative test/migration history.
2. **20/20**: two data-only independent stream adapters remain. Review has no issuer, store, action dispatcher or authentication port. Clarification name/schema/payload remains unchanged. Default encoded payload cap 16KiB; host controls continuation.
3. **20/20**: AAA tests cover manifest and custom name/description, exact encoded JSON cap with inclusive 64KiB and rejection above it, pre-canceled review/clarification and consumer error. Bound operation fixture checks the exact new review payload, no grant -> pending/no dispatch, authenticated host binding issuance, exact original action dispatch once, replay and current policy revocation.
4. **20/20**: README/doc.go/migration explicitly distinguish conversational intent from authenticated approver, operation binding, policy fingerprint, issuer, expiry and consume-once authority. Actual protected-operation challenge and current policy remain required. ExampleAsTools executes and asserts new tool/kind plus pause without grant. Independent module race/lint pass.
5. **20/20**: explicit zero/negative/>MaxControlBytes cap and nil options fail. No delivery above core budget. Consumer errors retain original cause and suppress pause sentinel for both tools, including cancellation/deadline consumer failures; pre-cancellation delivers no control. No live legacy API usage. This is independent A acceptance; second-reviewer gate belongs to parent and was not used as evidence.

**Completeness: 100%. Verdict: accepted.**

## Independent checks

- In toolkits/human: GOCACHE=/tmp/toolsy-review-gocache go test -race -count=3 ./... -> PASS 1.674s (d03-a-race.log).
- In toolkits/human: GOCACHE=/tmp/toolsy-review-gocache GOLANGCI_LINT_CACHE=/tmp/toolsy-task41/d03-a-lintcache /opt/homebrew/bin/golangci-lint run --allow-parallel-runners ./... -> 0 issues (d03-a-lint.log).
- Reviewer-owned temporary overlay consumer probe, both tools x arbitrary/canceled/deadline consumer errors; authority fixture, pre-cancellation and executable example: go -C toolkits/human test -race -count=10 -overlay=/tmp/toolsy-task41/d03-a-overlay.json -run 'TestD03AcceptanceConsumer|TestConversationPauseDoesNotGrantAction|TestReviewIntentCancellationNeverIssuesControl|ExampleAsTools' ./... -> PASS 1.943s (d03-a-adversarial.log).
- git diff --check -> clean. Source search confirms removed approval-named options/default with only negative test reference.

Errors/regressions/unfulfilled criteria: none found. Limitations: these checks establish the stated contract, not universal absence of defects. Authenticated host issuance is demonstrated through the existing trusted in-memory operation fixture; no external human identity provider/UI is exercised or supplied by this thin toolkit. Final repository-wide checks remain separate goal work. No implementation files were changed by this reviewer.
