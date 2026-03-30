# toolsyotel

`toolsyotel` is an extension module for OpenTelemetry instrumentation around `toolsy`.

## Semantic truncation observability

When you use `history.ApplySemanticTruncation(...)`, forward its report to
`RecordSemanticTruncation(...)` to get a dedicated span:

```go
import (
	"context"

	"github.com/skosovsky/toolsy/history"
	"github.com/skosovsky/toolsy/ext/toolsyotel"
	"go.opentelemetry.io/otel/sdk/trace"
)

func truncateWithTracing(
	ctx context.Context,
	historyIn []MyMessage,
	counter history.TokenCounter[MyMessage],
	summarizer history.ContextSummarizer[MyMessage],
	inspector history.MessageInspector[MyMessage],
	tp *trace.TracerProvider,
) ([]MyMessage, error) {
	out, report, err := history.ApplySemanticTruncation(
		ctx,
		historyIn,
		12000,
		counter,
		summarizer,
		inspector,
		history.WithMinRecentMessages[MyMessage](2),
	)

	// Always emit semantic truncation telemetry, even when truncation falls back.
	toolsyotel.RecordSemanticTruncation(
		ctx,
		report,
		err,
		toolsyotel.WithTracerProvider(tp),
	)
	if err != nil {
		return nil, err
	}
	return out, nil
}
```

The emitted span name is `toolsy.semantic_truncation` and includes:

- `toolsy.truncation.tokens_before`
- `toolsy.truncation.tokens_after`
- `toolsy.truncation.messages_compressed_count`
- `toolsy.truncation.fallback_used`
- `toolsy.truncation.mechanical_used`
- `toolsy.truncation.summarization_applied`
