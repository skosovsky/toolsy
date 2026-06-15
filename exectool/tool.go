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

	supported, err := resolveSupportedLanguages(sandbox, &o)
	if err != nil {
		return nil, err
	}

	handler := newExecHandler(sandbox, supported)
	return toolsy.NewDynamicToolFromSpec(toolsy.DynamicToolSpec{ //nolint:exhaustruct // ValidateArgs optional
		Name:        o.name,
		Description: o.description,
		Schema:      toolsy.MapSchemaProvider(buildSchema(supported)),
		Handler:     handler,
		Options:     o.toolOptions,
	})
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
) func(context.Context, *toolsy.RunEnv, map[string]any, func(toolsy.Chunk) error) error {
	return func(ctx context.Context, _ *toolsy.RunEnv, decoded map[string]any, yield func(toolsy.Chunk) error) error {
		args, err := execArgsFromDecoded(decoded)
		if err != nil {
			return err
		}

		if strings.TrimSpace(args.Code) == "" {
			return toolsy.NewValidationError("code is required")
		}

		language := strings.TrimSpace(args.Language)
		if !containsLanguage(supported, language) {
			return toolsy.NewValidationError(
				fmt.Sprintf("unsupported language %q for this sandbox", language),
			)
		}

		req := RunRequest{
			Language: language,
			Code:     args.Code,
			Env:      cloneEnv(args.Env),
			Files:    encodeFiles(args.Files),
		}

		res, runErr := sandbox.Run(ctx, req)
		if runErr != nil {
			return mapExecError(runErr)
		}
		out, marshalErr := json.Marshal(res)
		if marshalErr != nil {
			return toolsy.NewInternalError(fmt.Errorf("exectool: marshal result: %w", marshalErr))
		}
		return yield(toolsy.Chunk{Event: toolsy.EventResult, Data: out, MimeType: toolsy.MimeTypeJSON})
	}
}

func execArgsFromDecoded(decoded map[string]any) (execArgs, error) {
	var args execArgs
	language, ok := decodedString(decoded, "language")
	if !ok {
		return execArgs{}, toolsy.NewValidationError("language is required")
	}
	args.Language = language

	code, ok := decodedString(decoded, "code")
	if !ok {
		return execArgs{}, toolsy.NewValidationError("code is required")
	}
	args.Code = code

	if env, ok := decodedStringMap(decoded, "env"); ok {
		args.Env = env
	}
	if files, ok := decodedStringMap(decoded, "files"); ok {
		args.Files = files
	}
	return args, nil
}

func decodedString(m map[string]any, key string) (string, bool) {
	v, ok := m[key]
	if !ok {
		return "", false
	}
	s, ok := v.(string)
	return s, ok
}

func decodedStringMap(m map[string]any, key string) (map[string]string, bool) {
	v, ok := m[key]
	if !ok || v == nil {
		return nil, false
	}
	raw, ok := v.(map[string]any)
	if !ok {
		return nil, false
	}
	out := make(map[string]string, len(raw))
	for k, val := range raw {
		s, ok := val.(string)
		if !ok {
			return nil, false
		}
		out[k] = s
	}
	return out, true
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

func mapExecError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, ErrUnsupportedLanguage) {
		return toolsy.NewValidationError(err.Error())
	}
	if errors.Is(err, context.Canceled) {
		return err
	}
	if errors.Is(err, ErrTimeout) || errors.Is(err, context.DeadlineExceeded) {
		return toolsy.NewTimeoutErrorFrom(err, true)
	}
	if mapped := toolsy.MapSandboxReadLimitError(err); mapped != nil {
		return mapped
	}
	return fmt.Errorf("exectool: sandbox run: %w", err)
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
