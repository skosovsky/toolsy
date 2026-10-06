# Conversation compaction migration (D01)

Generic conversation history policy has left toolsy. Removed APIs:
`github.com/skosovsky/toolsy/history` (including `ApplySemanticTruncation`,
`TokenCounter`, `ContextSummarizer`, `MessageInspector`,
`SemanticTruncationOption`, `WithMinRecentMessages`, `SemanticTruncationReport`),
`toolsyotel.RecordSemanticTruncation` and `examples/semantic_truncation`.
No compatibility shim or mandatory contexty dependency is retained.

Tool execution still accepts BYOT values. `historycodec` remains the strict raw
**tool-result transcript** format; `ResultCodec` remains typed cache/journal
persistence. Neither is a generic conversation retention policy.

## Source capability check

Inspected local contexty commit
`8416b7b9883a09a26e3fe740d02dd8f78bff0b7f`. Relevant root-package tests were run
under race: rolling summary, drop-head atomicity, budget/retention and pending
rounds. This identifies the checked source revision, not a published version
recommendation. Contexty supports these concerns independently of toolsy:

| Removed concern | Host/contexty composition | Semantic difference |
|---|---|---|
| Generic `[]T` plus inspector | Host projects to `contexty.Message` / typed parts | No generic inspector adapter or automatic lossless reverse projection. Preserve host identity, provenance, tool call IDs and source refs deliberately. |
| Whole-history token counter | `TokenEstimator`, `BudgetConfig.Budget` | Estimator also provides per-message estimates. Provider framing/reservations remain host-owned; an estimated count is not an exact tokenizer guarantee. |
| Leading system prefix | `RetentionPolicy.MessageIDs` (or explicit `Roles`) | Selecting all system roles protects more than the old leading prefix. Explicit stable message IDs reproduce the intended selection. Required messages must have IDs. |
| Summarize old history | `Summarizer.Summarize(SummaryRequest)` | Returns one semantic message rather than generic `[]T`; host supplies content/identity and validates projection semantics. |
| Recent minimum / call boundaries | `WithRollingSummary`, tool-round inspection | Complete rounds extend the recent tail; pending rounds stay protected. Recent tail is not silently trimmed below the minimum. |
| Silent mechanical fallback after summary failure | Host chooses a separate eviction recipe | Summary errors propagate. Oversized required/recent content fails; do not silently delete it. A host may explicitly retry with a different validated policy. |
| Truncation report / toolsy OTel helper | `BudgetResult.Decision`, contexty observer, host telemetry | No drop-in span/report compatibility; host owns telemetry, redaction and exporter. |

`NewDropHeadStrategy` provides tool-round-safe mechanical eviction. Do not promise
that an arbitrary non-monotonic whole-snapshot counter has identical old fallback
selection semantics. The migration deliberately changes this contract.

## Host-owned recipe

Add contexty to the **host's** module at a reviewed revision. The following
compiles against the checked source; it adds no dependency to toolsy modules.
Project host messages before invoking it, and persist summaries/retained message
identity in the host. Required prefix IDs should name only the intended protected
messages. Do not pass a fabricated token budget from an estimated provider window.

```go
package host

import (
    "context"

    "github.com/skosovsky/contexty"
)

func CompactConversation(
    ctx context.Context,
    messages []contexty.Message,
    protectedIDs []string,
    effectiveTokens int,
    recentMessages int,
    estimator contexty.TokenEstimator,
    summarizer contexty.Summarizer,
) (contexty.BudgetResult, error) {
    pipeline := contexty.NewBudgetPipeline(
        contexty.BudgetConfig{
            Budget: contexty.EffectiveInputBudget(effectiveTokens),
            Retention: contexty.RetentionPolicy{MessageIDs: protectedIDs},
            Summarizer: summarizer,
        },
        estimator,
        contexty.WithRollingSummary(contexty.RollingSummaryPolicy{
            Descriptor: contexty.Descriptor{ID: "host.conversation-summary", Revision: "1"},
            RecentMessages: recentMessages,
        }),
    )
    return pipeline.Apply(ctx, messages)
}
```

The summarizer and estimator must honor cancellation. Host policies decide whether
summary/provider failure can trigger another attempt, how many attempts are allowed
and whether any mechanical eviction is acceptable. Toolsy does not schedule these
attempts. This change provides no hard preemption, provider-token accuracy or
lossless conversion guarantee for arbitrary host message types.
