package exectool

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/skosovsky/toolsy"
)

type execArgs struct {
	Language string            `json:"language"`
	Code     string            `json:"code"`
	Env      map[string]string `json:"env,omitempty"`
	Files    map[string]string `json:"files,omitempty"`
}

// New creates the generic exec_code tool backed by a specific sandbox.
func New(sandbox Sandbox, opts ...Option) (toolsy.Tool, error) {
	if sandbox == nil {
		return nil, errors.New("exectool: sandbox is nil")
	}

	var o options
	for _, opt := range opts {
		opt(&o)
	}
	applyDefaults(&o)
	if err := validateExecOptions(&o); err != nil {
		return nil, err
	}

	supported, err := resolveSupportedLanguages(sandbox, &o)
	if err != nil {
		return nil, err
	}

	handler := newExecHandler(sandbox, supported, o)
	return toolsy.NewDynamicTool(
		o.name,
		o.description,
		buildSchema(supported),
		handler,
		o.toolOptions...,
	)
}

func validateExecOptions(o *options) error {
	if o.timeout <= 0 {
		return errors.New(
			"exectool: execution timeout is required for safety (LLM-generated code). " +
				"Pass exectool.WithTimeout, e.g. exectool.WithTimeout(10 * time.Second)",
		)
	}
	return nil
}

func resolveSupportedLanguages(sandbox Sandbox, o *options) ([]string, error) {
	supported, err := normalizeLanguages(sandbox.SupportedLanguages())
	if err != nil {
		return nil, fmt.Errorf("exectool: supported languages: %w", err)
	}
	if len(o.allowedLanguages) == 0 {
		return supported, nil
	}
	allowed, err := normalizeLanguages(o.allowedLanguages)
	if err != nil {
		return nil, fmt.Errorf("exectool: allowed languages: %w", err)
	}
	supported = intersectLanguages(supported, allowed)
	if len(supported) == 0 {
		return nil, errors.New("exectool: allowed languages do not intersect sandbox capabilities")
	}
	return supported, nil
}

func newExecHandler(
	sandbox Sandbox,
	supported []string,
	o options,
) func(context.Context, toolsy.RunContext, []byte, func(toolsy.Chunk) error) error {
	return func(ctx context.Context, _ toolsy.RunContext, argsJSON []byte, yield func(toolsy.Chunk) error) error {
		var args execArgs
		if err := json.Unmarshal(argsJSON, &args); err != nil {
			return &toolsy.ClientError{
				Reason:    "json parse error: " + err.Error(),
				Retryable: false,
				Err:       nil,
			}
		}

		if strings.TrimSpace(args.Code) == "" {
			return &toolsy.ClientError{Reason: "code is required", Retryable: false, Err: toolsy.ErrValidation}
		}

		language := strings.TrimSpace(args.Language)
		if !containsLanguage(supported, language) {
			return fmt.Errorf("%w: %s", ErrUnsupportedLanguage, language)
		}

		timeout, err := effectiveTimeout(ctx, o.timeout)
		if err != nil {
			return err
		}

		req := RunRequest{
			Language: language,
			Code:     args.Code,
			Env:      cloneEnv(args.Env),
			Files:    encodeFiles(args.Files),
			Timeout:  timeout,
		}

		runCtx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()

		res, err := sandbox.Run(runCtx, req)
		if err != nil {
			if runCtx.Err() == context.DeadlineExceeded || errors.Is(err, context.DeadlineExceeded) {
				return fmt.Errorf("exectool: sandbox run: %w", ErrTimeout)
			}
			return fmt.Errorf("exectool: sandbox run: %w", err)
		}
		out, err := json.Marshal(res)
		if err != nil {
			return fmt.Errorf("exectool: marshal result: %w", err)
		}
		return yield(toolsy.Chunk{Event: toolsy.EventResult, Data: out, MimeType: toolsy.MimeTypeJSON})
	}
}

func buildSchema(languages []string) map[string]any {
	enum := make([]any, len(languages))
	for i, language := range languages {
		enum[i] = language
	}

	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"language": map[string]any{
				"type":        "string",
				"description": "Programming or scripting language to execute",
				"enum":        enum,
			},
			"code": map[string]any{
				"type":        "string",
				"description": "Source code to execute",
			},
			"env": map[string]any{
				"type":        "object",
				"description": "Optional environment variables passed into the sandbox",
				"additionalProperties": map[string]any{
					"type": "string",
				},
			},
			"files": map[string]any{
				"type":        "object",
				"description": "Optional UTF-8 text files materialized in the sandbox workspace",
				"additionalProperties": map[string]any{
					"type": "string",
				},
			},
		},
		"required":             []any{"language", "code"},
		"additionalProperties": false,
	}
}

func normalizeLanguages(languages []string) ([]string, error) {
	seen := make(map[string]struct{}, len(languages))
	normalized := make([]string, 0, len(languages))
	for _, language := range languages {
		trimmed := strings.TrimSpace(language)
		if trimmed == "" {
			return nil, errors.New("language names must be non-empty")
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		normalized = append(normalized, trimmed)
	}
	if len(normalized) == 0 {
		return nil, errors.New("at least one language must be configured")
	}
	sort.Strings(normalized)
	return normalized, nil
}

func intersectLanguages(supported, allowed []string) []string {
	allowSet := make(map[string]struct{}, len(allowed))
	for _, language := range allowed {
		allowSet[language] = struct{}{}
	}

	out := make([]string, 0, len(supported))
	for _, language := range supported {
		if _, ok := allowSet[language]; ok {
			out = append(out, language)
		}
	}
	return out
}

func containsLanguage(languages []string, target string) bool {
	return slices.Contains(languages, target)
}

func effectiveTimeout(ctx context.Context, configured time.Duration) (time.Duration, error) {
	timeout := configured
	if deadline, ok := ctx.Deadline(); ok {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return 0, ErrTimeout
		}
		if timeout <= 0 || remaining < timeout {
			timeout = remaining
		}
	}
	if timeout <= 0 {
		return 0, ErrTimeout
	}
	return timeout, nil
}

func cloneEnv(env map[string]string) map[string]string {
	if len(env) == 0 {
		return nil
	}
	cloned := make(map[string]string, len(env))
	maps.Copy(cloned, env)
	return cloned
}

func encodeFiles(files map[string]string) map[string][]byte {
	if len(files) == 0 {
		return nil
	}
	out := make(map[string][]byte, len(files))
	for name, content := range files {
		out[name] = []byte(content)
	}
	return out
}
