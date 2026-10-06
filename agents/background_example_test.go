package agents_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"

	"github.com/skosovsky/toolsy"
	"github.com/skosovsky/toolsy/agents"
)

func ExampleAsBackgroundTool() {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"task_id":"remote-1","artifacts":[]}`))
	}))
	defer server.Close()
	client := agents.NewClient(server.URL, agents.WithAllowPrivateIPs(true))
	tool, err := agents.AsBackgroundTool("start", "Start remote work", []byte(`{"type":"object"}`), client)
	if err != nil {
		panic(err)
	}
	// A host-owned store. Production hosts choose persistence and status retrieval.
	references := make(map[string]agents.AcceptedTaskReference)
	err = tool.Execute(
		context.Background(),
		toolsy.NewRunEnv(nil),
		toolsy.ToolInput{ArgsJSON: []byte(`{}`)},
		func(chunk toolsy.Chunk) error {
			var reference agents.AcceptedTaskReference
			if decodeErr := json.Unmarshal(chunk.Data, &reference); decodeErr != nil {
				return decodeErr
			}
			references[reference.TaskID] = reference
			return nil
		},
	)
	if err != nil {
		panic(err)
	}
	fmt.Println(references["remote-1"].Accepted)
	// Output: true
}
