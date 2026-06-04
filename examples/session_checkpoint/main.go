// Package main demonstrates Session state Export/Import with StateTypeRegistry.
package main

import (
	"encoding/json"
	"fmt"
	"log"

	"github.com/skosovsky/toolsy"
)

// Host-local types (stand in for domain.ExecutionContext, ReferenceRegistry, etc.).
type executionContext struct {
	TraceID string `json:"trace_id"`
	Step    int    `json:"step"`
}

type referenceRegistry struct {
	IDs []string `json:"ids"`
}

const (
	stateKeyExecutionContext = "execution_context"
	stateKeyReferences       = "references"
)

// Host pattern (e.g. kosmify): global registry is a host choice, not toolsy.
//
//	var GlobalStateRegistry = toolsy.NewStateTypeRegistry()
//
//	func init() {
//	    GlobalStateRegistry.Register(tool.StateKeyExecutionContext, domain.ExecutionContext{})
//	    GlobalStateRegistry.Register(tool.StateKeyReferences, domain.ReferenceRegistry{})
//	}
//
//	func resume(reg *toolsy.Registry, payload Checkpoint) {
//	    session, _ := toolsy.NewSession(reg,
//	        toolsy.WithRunPolicy(policy),
//	        toolsy.WithStateTypeRegistry(GlobalStateRegistry),
//	    )
//	    _ = session.Import(payload.SessionData)
//	    env := toolsy.NewRunEnv(session)
//	    // Put per-process deps, then session.Execute(..., ToolCall{Env: env}, ...)
//	}

func main() {
	types := toolsy.NewStateTypeRegistry()
	types.Register(stateKeyExecutionContext, executionContext{})
	types.Register(stateKeyReferences, referenceRegistry{})

	session, err := toolsy.NewSession(nil, toolsy.WithStateTypeRegistry(types))
	if err != nil {
		log.Fatal(err)
	}

	toolsy.SetSessionState(session, stateKeyExecutionContext, executionContext{TraceID: "trace-1", Step: 1})
	toolsy.SetSessionState(session, stateKeyReferences, referenceRegistry{IDs: []string{"a", "b"}})

	raw, err := json.Marshal(session.Export())
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("checkpoint:", string(raw))

	var payload map[string]any
	if unmarshalErr := json.Unmarshal(raw, &payload); unmarshalErr != nil {
		log.Fatal(unmarshalErr)
	}

	resumed, err := toolsy.NewSession(nil, toolsy.WithStateTypeRegistry(types))
	if err != nil {
		log.Fatal(err)
	}
	if err := resumed.Import(payload); err != nil {
		log.Fatal(err)
	}

	ctx, ok := toolsy.GetSessionState[executionContext](resumed, stateKeyExecutionContext)
	if !ok {
		log.Fatal("execution_context missing after import")
	}
	refs, ok := toolsy.GetSessionState[referenceRegistry](resumed, stateKeyReferences)
	if !ok {
		log.Fatal("references missing after import")
	}

	// Tools read the same state via RunEnv after checkpoint restore.
	env := toolsy.NewRunEnv(resumed)
	ctxViaEnv, ok := toolsy.GetState[executionContext](env, stateKeyExecutionContext)
	if !ok {
		log.Fatal("execution_context missing via RunEnv after import")
	}
	if ctxViaEnv.TraceID != ctx.TraceID {
		log.Fatal("RunEnv GetState mismatch")
	}

	fmt.Printf("restored trace=%s step=%d refs=%v\n", ctx.TraceID, ctx.Step, refs.IDs)
}
