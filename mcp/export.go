package mcp

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"mime"
	"strings"
	"sync"

	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/skosovsky/toolsy"
	"github.com/skosovsky/toolsy/internal/jsonschemax"
)

const exportJSONKind = "export JSON"

// ExportConfig defines one immutable authority/catalog boundary. The host uses
// a separate SDK Server per tenant/scope via its HTTP handler or STDIO connection.
type ExportConfig struct {
	Names          []string
	Implementation *sdk.Implementation
	ServerOptions  *sdk.ServerOptions
	// Resolve authenticates from transport context, never from arguments/_meta.
	Resolve func(context.Context, sdk.Request) (toolsy.CallContext, error)
	// Session, if provided, must be built from this same view. The factory is a
	// trusted host port, not model data, and must preserve current policy/replay.
	Session func(context.Context, *toolsy.RegistryView) (*toolsy.Session, error)
	// Project is required for independent streams or attached controls. It must
	// explicitly preserve audience boundaries and never serialize grants/effects.
	Project func(context.Context, toolsy.ToolManifest, []toolsy.Chunk) (*sdk.CallToolResult, error)
	// Control maps host approval/pause outcomes into an MCP workflow. It is not
	// an approval engine. Responses must be verified by current core policy.
	Control func(context.Context, *sdk.CallToolRequest, error) (*sdk.CallToolResult, error)
	// ErrorMapper explicitly projects host-approved protocol diagnostics. A nil
	// mapping retains the safe wrapped error; raw causes must never be published.
	ErrorMapper    func(context.Context, sdk.Request, error) *jsonrpc.Error
	MaxResultBytes int
	MaxChunks      int
}

// Export publishes an explicit selection from a scoped RegistryView through
// Server.AddTool. No handler directly invokes a domain Tool.Execute.
//
//nolint:gocognit // Registration validates one immutable catalog and installs its scoped handlers.
func Export(view *toolsy.RegistryView, config ExportConfig) (*sdk.Server, error) {
	if view == nil || config.Names == nil || config.Resolve == nil || config.Implementation == nil {
		return nil, errors.New("mcp export requires scope, explicit names, implementation and trusted resolver")
	}
	if config.MaxResultBytes == 0 {
		config.MaxResultBytes = defaultSnapshotBytes
	}
	if config.MaxChunks == 0 {
		config.MaxChunks = 1024
	}
	if config.MaxResultBytes < 1 || config.MaxChunks < 1 {
		return nil, errors.New("mcp export limits must be positive")
	}
	selected, err := selectExportManifests(view, config)
	if err != nil {
		return nil, err
	}
	server := sdk.NewServer(config.Implementation, config.ServerOptions)
	// Authenticate discovery and direct invocation against the same host scope.
	server.AddReceivingMiddleware(func(next sdk.MethodHandler) sdk.MethodHandler {
		return func(ctx context.Context, method string, req sdk.Request) (sdk.Result, error) {
			if method != "tools/list" && method != toolCallMethod {
				return next(ctx, method, req)
			}
			if _, err := config.Resolve(ctx, req); err != nil {
				return nil, mapExportError(ctx, req, &BridgeError{Kind: "export authority", Cause: err}, config)
			}
			result, err := next(ctx, method, req)
			if err != nil {
				return nil, mapExportError(ctx, req, err, config)
			}
			return result, nil
		}
	})
	for _, manifest := range selected {
		input, err := json.Marshal(manifest.Parameters)
		if err != nil {
			return nil, err
		}
		var output any
		if manifest.OutputSchema != nil {
			raw, err := json.Marshal(manifest.OutputSchema)
			if err != nil {
				return nil, err
			}
			output = json.RawMessage(raw)
		}
		// Raw AddTool avoids generic applySchema/default numeric conversion:
		// https://github.com/modelcontextprotocol/go-sdk/issues/1201
		// https://github.com/modelcontextprotocol/go-sdk/pull/1302
		server.AddTool(
			&sdk.Tool{ //nolint:exhaustruct_v5 // Optional payload fields and zero-value synchronization are deliberate.
				Name:         manifest.Name,
				Description:  manifest.Description,
				InputSchema:  json.RawMessage(input),
				OutputSchema: output,
			},
			func(ctx context.Context, req *sdk.CallToolRequest) (*sdk.CallToolResult, error) {
				return exportCall(ctx, view, config, manifest, req)
			},
		)
	}
	return server, nil
}

func selectExportManifests(view *toolsy.RegistryView, config ExportConfig) ([]toolsy.ToolManifest, error) {
	manifests, err := view.ManifestSet()
	if err != nil {
		return nil, err
	}
	selected := make([]toolsy.ToolManifest, 0, len(config.Names))
	seen := map[string]bool{}
	for _, name := range config.Names {
		manifest, ok := manifests.Manifest(name)
		if !ok || seen[name] {
			return nil, errors.New("mcp export selection is absent or duplicated")
		}
		seen[name] = true
		if validationErr := validateExportManifest(manifest, config); validationErr != nil {
			return nil, validationErr
		}
		selected = append(selected, manifest)
	}
	return selected, nil
}

func validateExportManifest(manifest toolsy.ToolManifest, config ExportConfig) error {
	if manifest.StreamSemantics == toolsy.StreamIndependent && config.Project == nil {
		return &BridgeError{Kind: "independent stream requires explicit projection", Cause: ErrUnsupported}
	}
	if manifest.RequiresConfirmation && config.Control == nil {
		return &BridgeError{Kind: "confirmation requires host control mapping", Cause: ErrUnsupported}
	}
	if manifest.Parameters == nil || manifest.Parameters["type"] != "object" {
		return ErrUnsupported
	}
	if _, err := jsonschemax.Compile(manifest.Parameters); err != nil {
		return &BridgeError{Kind: "input schema", Cause: err}
	}
	if manifest.OutputSchema != nil {
		if _, err := jsonschemax.Compile(manifest.OutputSchema); err != nil {
			return &BridgeError{Kind: "output schema", Cause: err}
		}
	}
	return nil
}

func exportCall(ctx context.Context, view *toolsy.RegistryView, config ExportConfig,
	manifest toolsy.ToolManifest, req *sdk.CallToolRequest) (*sdk.CallToolResult, error) {
	identity, err := config.Resolve(ctx, req)
	if err != nil {
		return nil, &BridgeError{Kind: "export authority", Cause: err}
	}
	// Always replace model/remote call metadata with host authority and local IDs.
	call := toolsy.ToolCall{ //nolint:exhaustruct_v5 // Optional payload fields and zero-value synchronization are deliberate.
		ToolName:    manifest.Name,
		CallContext: identity,
		Input: toolsy.ToolInput{ //nolint:exhaustruct_v5 // Optional payload fields and zero-value synchronization are deliberate.
			CallID:   "mcp-export-" + rand.Text(),
			ArgsJSON: bytes.Clone(req.Params.Arguments),
		},
	}
	collector := &exportCollector{ //nolint:exhaustruct_v5 // Optional payload fields and zero-value synchronization are deliberate.
		config: config,
		req:    req,
	}
	if config.Session != nil {
		session, sessionErr := config.Session(ctx, view)
		if sessionErr != nil {
			return nil, &BridgeError{Kind: "export session", Cause: sessionErr}
		}
		if session == nil {
			return nil, ErrUnsupported
		}
		err = session.Execute(ctx, call, func(chunk toolsy.Chunk) error { return collector.accept(ctx, chunk) })
	} else {
		err = view.Execute(ctx, call, func(chunk toolsy.Chunk) error { return collector.accept(ctx, chunk) })
	}
	collector.mu.Lock()
	defer collector.mu.Unlock()
	if collector.sticky != nil {
		return nil, &BridgeError{Kind: "export delivery", Cause: collector.sticky}
	}
	if err != nil {
		var pending *toolsy.PendingApprovalError
		if config.Control != nil && (toolsy.IsControlError(err) || errors.As(err, &pending)) {
			result, controlErr := config.Control(ctx, req, err)
			if controlErr != nil {
				return nil, &BridgeError{Kind: "export control", Cause: controlErr}
			}
			return validateExportResult(manifest, result)
		}
		return nil, &BridgeError{Kind: "export execution", Cause: err}
	}
	if config.Project != nil {
		result, projectionErr := config.Project(ctx, manifest, collector.chunks)
		if projectionErr != nil {
			return nil, &BridgeError{Kind: "export projection", Cause: projectionErr}
		}
		return validateExportResult(manifest, result)
	}
	result, projectionErr := defaultExportProjection(collector.chunks)
	if projectionErr != nil {
		return nil, projectionErr
	}
	return validateExportResult(manifest, result)
}

type exportCollector struct {
	mu     sync.Mutex
	config ExportConfig
	req    *sdk.CallToolRequest
	chunks []toolsy.Chunk
	total  int
	sticky error
}

//nolint:nestif,gocognit // Progress and bounded terminal delivery share one sticky collector state.
func (c *exportCollector) accept(ctx context.Context, chunk toolsy.Chunk) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.sticky != nil {
		return c.sticky
	}
	if chunk.Event == toolsy.EventProgress {
		if token := c.req.Params.GetProgressToken(); token != nil {
			params := &sdk.ProgressNotificationParams{ //nolint:exhaustruct_v5 // Optional payload fields and zero-value synchronization are deliberate.
				ProgressToken: token,
			}
			if chunk.Progress != nil {
				params.Message = chunk.Progress.Message
				if chunk.Progress.Current != nil {
					params.Progress = *chunk.Progress.Current
				}
				if chunk.Progress.Total != nil {
					params.Total = *chunk.Progress.Total
				}
			}
			c.sticky = c.req.Session.NotifyProgress(ctx, params)
		}
		return c.sticky
	}
	c.total += len(chunk.Data)
	if len(c.chunks) >= c.config.MaxChunks || c.total > c.config.MaxResultBytes {
		c.sticky = &BridgeError{Kind: "export result limit", Cause: ErrAdmission}
		return c.sticky
	}
	if chunk.Event == toolsy.EventControl && c.config.Control != nil {
		c.chunks = append(c.chunks, chunk)
		return nil
	}
	if c.config.Project == nil &&
		(chunk.Event != toolsy.EventResult || len(chunk.Controls) != 0 || chunk.Control != nil) {
		c.sticky = &BridgeError{Kind: "unmapped control", Cause: ErrUnsupported}
		return c.sticky
	}
	chunk.Data = bytes.Clone(chunk.Data)
	c.chunks = append(c.chunks, chunk)
	return nil
}

func validateExportResult(manifest toolsy.ToolManifest, result *sdk.CallToolResult) (*sdk.CallToolResult, error) {
	if result == nil {
		return nil, &BridgeError{Kind: "nil export result", Cause: ErrUnsupported}
	}
	if result.IsError || manifest.OutputSchema == nil || result.InputRequests != nil {
		return result, nil
	}
	raw, err := json.Marshal(result.StructuredContent)
	if err != nil {
		return nil, &BridgeError{Kind: exportJSONKind, Cause: err}
	}
	value, err := jsonschemax.Decode(raw)
	if err != nil {
		return nil, &BridgeError{Kind: exportJSONKind, Cause: err}
	}
	schema, err := jsonschemax.Compile(manifest.OutputSchema)
	if err == nil {
		err = schema.Validate(value)
	}
	if err != nil {
		return nil, &BridgeError{
			Kind:  "export result contract",
			Cause: toolsy.NewInternalError(&toolsy.ResultContractError{Kind: "schema_mismatch", Cause: err}),
		}
	}
	return result, nil
}

func defaultExportProjection(chunks []toolsy.Chunk) (*sdk.CallToolResult, error) {
	if len(chunks) != 1 {
		return nil, &BridgeError{Kind: "export requires one terminal result", Cause: ErrUnsupported}
	}
	chunk := chunks[0]
	if chunk.Envelope != nil && chunk.Envelope.Audience != "" && chunk.Envelope.Audience != toolsy.AudienceModel {
		return nil, &BridgeError{Kind: "export audience", Cause: ErrUnsupported}
	}
	result := &sdk.CallToolResult{ //nolint:exhaustruct_v5 // Optional payload fields and zero-value synchronization are deliberate.
		Content: []sdk.Content{},
		IsError: chunk.IsError,
	}
	if chunk.EmptyResult || chunk.Noop {
		return result, nil
	}
	if chunk.IsError {
		result.Content = []sdk.Content{
			&sdk.TextContent{ //nolint:exhaustruct_v5 // Optional payload fields and zero-value synchronization are deliberate.
				Text: "tool execution failed",
			},
		}
		return result, nil
	}
	mediaType, _, err := mime.ParseMediaType(chunk.MimeType)
	if err != nil {
		return nil, &BridgeError{Kind: "export MIME", Cause: err}
	}
	switch {
	case mediaType == toolsy.MimeTypeJSON || strings.HasSuffix(mediaType, "+json"):
		if _, err := jsonschemax.Decode(chunk.Data); err != nil {
			return nil, &BridgeError{Kind: exportJSONKind, Cause: err}
		}
		result.StructuredContent = json.RawMessage(bytes.Clone(chunk.Data))
	case strings.HasPrefix(mediaType, "text/"):
		result.Content = []sdk.Content{
			&sdk.TextContent{ //nolint:exhaustruct_v5 // Optional payload fields and zero-value synchronization are deliberate.
				Text: string(chunk.Data),
			},
		}
	case strings.HasPrefix(mediaType, "image/"):
		result.Content = []sdk.Content{
			&sdk.ImageContent{ //nolint:exhaustruct_v5 // Optional payload fields and zero-value synchronization are deliberate.
				Data:     bytes.Clone(chunk.Data),
				MIMEType: mediaType,
			},
		}
	case strings.HasPrefix(mediaType, "audio/"):
		result.Content = []sdk.Content{
			&sdk.AudioContent{ //nolint:exhaustruct_v5 // Optional payload fields and zero-value synchronization are deliberate.
				Data:     bytes.Clone(chunk.Data),
				MIMEType: mediaType,
			},
		}
	default:
		return nil, &BridgeError{Kind: "export binary requires explicit projection", Cause: ErrUnsupported}
	}
	return result, nil
}

func mapExportError(ctx context.Context, req sdk.Request, err error, config ExportConfig) error {
	if config.ErrorMapper == nil {
		return err
	}
	mapped := config.ErrorMapper(ctx, req, err)
	if mapped == nil {
		return err
	}
	// SDK v1.8.0 drops Data from wrapped jsonrpc.Errors. Return a fresh public
	// SDK error containing only explicitly host-approved diagnostics.
	if len(mapped.Data) > config.MaxResultBytes || len(mapped.Message) > config.MaxResultBytes-len(mapped.Data) ||
		(len(mapped.Data) != 0 && !json.Valid(mapped.Data)) {
		return &BridgeError{Kind: "export error projection", Cause: ErrUnsupported}
	}
	return &jsonrpc.Error{Code: mapped.Code, Message: mapped.Message, Data: bytes.Clone(mapped.Data)}
}
