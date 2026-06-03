package toolsy

import (
	"context"
	"mime"
	"strings"
	"unicode/utf8"

	"github.com/skosovsky/toolsy/textprocessor"
)

const defaultTruncationSuffix = "\n... [Output truncated. If you need more details, refine your query or use a specialized search tool.]"

// TruncationOption configures truncation middleware behavior.
type TruncationOption func(*truncationConfig)

type truncationConfig struct {
	suffix      string
	includeJSON bool
}

// WithTruncationSuffix overrides the suffix appended to truncated text.
func WithTruncationSuffix(suffix string) TruncationOption {
	return func(c *truncationConfig) {
		c.suffix = suffix
	}
}

// WithTruncationIncludeJSON enables truncation for application/json payloads.
func WithTruncationIncludeJSON(enable bool) TruncationOption {
	return func(c *truncationConfig) {
		c.includeJSON = enable
	}
}

// WithTruncation truncates large textual chunk payloads by rune count.
// By default it applies to text/plain and text/markdown payloads.
func WithTruncation(maxRunes int, opts ...TruncationOption) Middleware {
	cfg := truncationConfig{
		suffix:      defaultTruncationSuffix,
		includeJSON: false,
	}
	for _, opt := range opts {
		opt(&cfg)
	}
	return func(next Tool) Tool {
		return &truncationTool{
			toolBase: toolBase{next: next},
			maxRunes: maxRunes,
			cfg:      cfg,
		}
	}
}

type truncationTool struct {
	toolBase

	maxRunes int
	cfg      truncationConfig
}

func (t *truncationTool) Execute(
	ctx context.Context,
	run *RunEnv,
	input ToolInput,
	yield func(Chunk) error,
) error {
	if t.maxRunes <= 0 {
		return t.next.Execute(ctx, run, input, yield)
	}

	yieldWrapped := func(c Chunk) error {
		if len(c.Data) == 0 || !shouldTruncateMimeType(c.MimeType, t.cfg.includeJSON) {
			return yield(c)
		}
		if !utf8.Valid(c.Data) {
			return yield(c)
		}
		if utf8.RuneCount(c.Data) <= t.maxRunes {
			return yield(c)
		}
		c.Data = textprocessor.TruncateBytesByRunes(c.Data, t.maxRunes, t.cfg.suffix)
		return yield(c)
	}

	return t.next.Execute(ctx, run, input, yieldWrapped)
}

func shouldTruncateMimeType(mimeType string, includeJSON bool) bool {
	mediaType, _, err := mime.ParseMediaType(mimeType)
	if err != nil {
		return false
	}
	switch strings.ToLower(mediaType) {
	case "text/plain", "text/markdown":
		return true
	case "application/json":
		return includeJSON
	default:
		return false
	}
}
