package toolsy

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"strconv"
	"testing"
	"time"
)

func performanceTool(b *testing.B, kind string, customBinder bool) Tool {
	b.Helper()
	raw, err := os.ReadFile("testdata/performance/groups.payload.json")
	if err != nil {
		b.Fatal(err)
	}
	schema, err := performanceSchema(b, "output")
	if err != nil {
		b.Fatal(err)
	}
	var tool Tool
	switch kind {
	case "proxy":
		tool, err = NewProxyTool(
			"groups",
			"groups",
			[]byte(`{"type":"object"}`),
			func(_ context.Context, _ *RunEnv, _ []byte, y func(Chunk) error) error {
				return y(Chunk{Event: EventResult, MimeType: MimeTypeJSON, Data: raw})
			},
			WithOutputSchema(schema),
		)
	case "dynamic":
		tool, err = NewDynamicToolFromSpec(
			DynamicToolSpec{
				Name:    "groups",
				Schema:  MapSchemaProvider{"type": "object"},
				Options: []ToolOption{WithOutputSchema(schema)},
				Handler: func(_ context.Context, _ *RunEnv, _ map[string]any, y func(Chunk) error) error {
					return y(Chunk{Event: EventResult, MimeType: MimeTypeJSON, Data: raw})
				},
			},
		)
	case "typed":
		spec := TypedToolSpec[NoSubject, NoScope, struct{}, json.RawMessage, string]{
			Name:    "groups",
			Options: []ToolOption{WithOutputSchema(schema)},
			Handler: func(context.Context, TypedCallContext[NoSubject, NoScope], *RunEnv, ValidatedArgs[struct{}]) (ToolResult[json.RawMessage, string], error) {
				return NewToolResult[json.RawMessage, string](json.RawMessage(raw)), nil
			},
		}
		if customBinder {
			spec.ArgsBinder = func(_ context.Context, req ArgsBindRequest) (ValidatedArgs[struct{}], error) {
				var value struct{}
				bindErr := json.Unmarshal(req.Input.ArgsJSON, &value)
				return ValidatedArgs[struct{}]{
					Value:    value,
					Raw:      req.Input.ArgsJSON,
					Metadata: map[string]any{"binder": true},
				}, bindErr
			}
		}
		tool, err = NewTypedTool(spec)
	}
	if err != nil {
		b.Fatal(err)
	}
	return tool
}

//nolint:gocognit // The fixed benchmark cross-product keeps each policy/executor fixture independent.
func BenchmarkPerformanceExecute(b *testing.B) {
	allow := PolicyFunc(func(context.Context, PolicyRequest) Decision { return AllowDecision() })
	for _, kind := range []string{"proxy", "typed", "dynamic"} {
		for _, path := range []string{"direct", "registry", "view", "session"} {
			for _, pol := range []string{"off", "on", "composed"} {
				if path == "direct" && pol != "off" {
					continue
				}
				b.Run(kind+"/"+path+"/"+pol, func(b *testing.B) {
					tool := performanceTool(b, kind, false)
					builder := NewRegistryBuilder()
					if pol == "on" {
						builder = NewRegistryBuilder(WithPolicy("allow", allow))
					}
					if pol == "composed" {
						builder = NewRegistryBuilder(WithPolicy("composed", composePolicies(allow, allow)))
					}
					registry, err := builder.Add(tool).Build()
					if err != nil {
						b.Fatal(err)
					}
					execute := registry.Execute
					var boundSession *Session
					if path == "view" {
						view, err := registry.View(RegistryViewSpec{ToolNames: []string{"groups"}})
						if err != nil {
							b.Fatal(err)
						}
						execute = view.Execute
					}
					if path == "session" {
						session, err := NewSession(registry)
						if err != nil {
							b.Fatal(err)
						}
						execute = session.Execute
						boundSession = session
					}
					if path == "direct" {
						execute = func(ctx context.Context, call ToolCall, y func(Chunk) error) error {
							return tool.Execute(ctx, call.Env, call.Input, y)
						}
					}
					ctx := context.Background()
					input := ToolInput{ArgsJSON: []byte(`{}`)}
					y := func(Chunk) error { return nil }
					b.ReportAllocs()
					b.ResetTimer()
					for range b.N {
						if err := execute(
							ctx,
							ToolCall{ToolName: "groups", Input: input, Env: NewRunEnv(boundSession)},
							y,
						); err != nil {
							b.Fatal(err)
						}
					}
				})
			}
		}
	}
}

func BenchmarkPerformanceWrappers(b *testing.B) {
	for _, kind := range []string{"override", "logging", "binder", "prepared"} {
		b.Run(kind, func(b *testing.B) {
			tool := performanceTool(b, "typed", kind == "binder")
			if kind == "override" {
				tool = OverrideTool(tool, WithNewName("alias"))
			}
			if kind == "logging" {
				tool = WithLogging(slog.New(slog.DiscardHandler))(tool)
			}
			ctx := context.Background()
			input := ToolInput{ArgsJSON: []byte(`{}`)}
			yield := func(Chunk) error { return nil }
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				env := NewRunEnv(nil)
				if kind == "prepared" {
					env.preparedChecks = []func(context.Context, PreparedCall, any) error{
						func(context.Context, PreparedCall, any) error { return nil },
					}
				}
				if err := tool.Execute(ctx, env, input, yield); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

//nolint:gocognit // Cache and operation scenarios share the same fixed fixture and warm timer discipline.
func BenchmarkPerformanceProfiles(b *testing.B) {
	for _, mode := range []string{"cache-miss", "cache-hit", "operation-fresh", "operation-replay"} {
		b.Run(mode, func(b *testing.B) {
			tool, err := NewTool(
				"value",
				"value",
				func(context.Context, *RunEnv, struct{}) (string, error) { return "value", nil },
				WithIdempotent(),
			)
			if err != nil {
				b.Fatal(err)
			}
			var profile ExecutionProfile
			seq := 0
			if mode == "cache-miss" || mode == "cache-hit" {
				profile, err = NewResultCache(
					NewMemoryResultCacheStore(),
					func(context.Context, PreparedCall) (bool, error) { return true, nil },
					func(context.Context, PreparedCall) (string, error) {
						if mode == "cache-miss" {
							seq++
							return strconv.Itoa(seq), nil
						}
						return "fixed", nil
					},
					JSONResultCodec[string, string]{},
					0,
				)
			} else {
				profile, err = NewOperationProfile(
					OperationProfileConfig{
						Store:  NewMemoryOperationStore(),
						Issuer: "performance",
						Clock:  time.Now,
						Lease:  time.Minute,
						Codec:  JSONResultCodec[string, string]{},
						Prepare: func(_ context.Context, call PreparedCall) (OperationIntent, error) {
							id := "fixed"
							if mode == "operation-fresh" {
								seq++
								id = strconv.Itoa(seq)
							}
							return OperationIntent{
								Namespace:         "performance",
								Scope:             "scope",
								Subject:           "subject",
								OperationID:       id,
								AttemptID:         call.Input.CallID,
								PolicyFingerprint: "current",
								CanonicalDigest:   "fixed",
								CanonicalRules:    "json",
								DisplayJSON:       []byte(`{}`),
							}, nil
						},
					},
				)
			}
			if err != nil {
				b.Fatal(err)
			}
			reg, err := NewRegistryBuilder(WithExecutionProfile(profile)).Add(tool).Build()
			if err != nil {
				b.Fatal(err)
			}
			call := ToolCall{ToolName: "value", Input: ToolInput{CallID: "attempt", ArgsJSON: []byte(`{}`)}}
			yield := func(Chunk) error { return nil }
			if mode == "cache-hit" || mode == "operation-replay" {
				if err := reg.Execute(context.Background(), call, yield); err != nil {
					b.Fatal(err)
				}
			}
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				if err := reg.Execute(context.Background(), call, yield); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkPerformanceEnv(b *testing.B) {
	for _, populated := range []bool{false, true} {
		name := "empty"
		if populated {
			name = "populated"
		}
		b.Run(name, func(b *testing.B) {
			call := CallContext{}
			var attachments []Attachment
			if populated {
				call = NewCallContext(
					map[string]any{"user": "example"},
					map[string]any{"tenant": "example"},
					WithCallValue("nested", map[string]any{"values": []any{1, 2, 3}}),
				)
				attachments = []Attachment{{MimeType: MimeTypeText, Data: []byte("attachment")}}
			}
			env := NewRunEnv(nil)
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				_ = env.cloneForExecute(attachments, nil, call)
			}
		})
	}
}

func BenchmarkPerformanceStream(b *testing.B) {
	for _, terminal := range []bool{false, true} {
		name := "independent"
		opts := []ToolOption{WithIndependentStream()}
		if terminal {
			name = "terminal"
			opts = []ToolOption{WithTerminalStream(0), WithOutputSchema(map[string]any{"type": "string"})}
		}
		b.Run(name, func(b *testing.B) {
			tool, err := NewStreamTool(
				"stream",
				"stream",
				func(_ context.Context, _ *RunEnv, _ struct{}, yield func(Chunk) error) error {
					return yield(Chunk{Event: EventResult, MimeType: MimeTypeJSON, Data: []byte(`"value"`)})
				},
				opts...)
			if err != nil {
				b.Fatal(err)
			}
			input := ToolInput{ArgsJSON: []byte(`{}`)}
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				if err := tool.Execute(
					context.Background(),
					NewRunEnv(nil),
					input,
					func(Chunk) error { return nil },
				); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkPerformanceCold(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		_ = performanceTool(b, "proxy", false)
	}
}
