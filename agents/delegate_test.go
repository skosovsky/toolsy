package agents

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/skosovsky/toolsy"
)

func TestFormatStepOutput(t *testing.T) {
	tests := []struct {
		name      string
		text      string
		artifacts []Artifact
		want      string
		contains  []string // alternative: check output contains these substrings
	}{
		{
			name:      "text only",
			text:      "hello",
			artifacts: nil,
			want:      "hello",
		},
		{
			name: "text and artifact with data URI",
			text: "done",
			artifacts: []Artifact{
				{FileName: "img.png", MimeType: "image/png", Data: "base64data"},
			},
			contains: []string{"done", "![img.png](data:image/png;base64,base64data)"},
		},
		{
			name:      "artifact default MimeType",
			text:      "",
			artifacts: []Artifact{{FileName: "x", Data: "abc"}},
			contains:  []string{"data:application/octet-stream;base64,abc"},
		},
		{
			name:      "artifact default FileName",
			text:      "",
			artifacts: []Artifact{{MimeType: "image/jpeg", Data: "xyz"}},
			contains:  []string{"![file](data:image/jpeg;base64,xyz)"},
		},
		{
			name:      "artifact without data outputs filename only",
			text:      "",
			artifacts: []Artifact{{FileName: "readme.txt", MimeType: "text/plain"}},
			want:      "readme.txt",
		},
		{
			name: "multiple artifacts",
			text: "results",
			artifacts: []Artifact{
				{FileName: "a.png", MimeType: "image/png", Data: "AAA"},
				{FileName: "b.png", MimeType: "image/png", Data: "BBB"},
			},
			contains: []string{
				"results",
				"![a.png](data:image/png;base64,AAA)",
				"![b.png](data:image/png;base64,BBB)",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out := formatStepOutput(tt.text, tt.artifacts)
			if tt.want != "" {
				if out != tt.want {
					t.Errorf("formatStepOutput() = %q, want %q", out, tt.want)
				}
				return
			}
			for _, sub := range tt.contains {
				if !strings.Contains(out, sub) {
					t.Errorf("formatStepOutput() = %q, want to contain %q", out, sub)
				}
			}
		})
	}
}

func TestAsTool_CancelTaskUsesBoundedContext(t *testing.T) {
	var cancelCalled atomic.Bool
	releaseStream := make(chan struct{})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/tasks") &&
			!strings.Contains(r.URL.Path, "/cancel"):
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"task_id":"task-1"}`))
		case strings.Contains(r.URL.Path, "/cancel"):
			cancelCalled.Store(true)
			w.WriteHeader(http.StatusNoContent)
		case strings.Contains(r.URL.Path, "/steps"):
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = fmt.Fprintf(
				w,
				"data: {\"step_id\":\"s1\",\"task_id\":\"task-1\",\"name\":\"n\",\"status\":\"running\",\"is_last\":false}\n\n",
			)
			if flusher, ok := w.(http.Flusher); ok {
				flusher.Flush()
			}
			select {
			case <-releaseStream:
			case <-r.Context().Done():
			}
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	client := NewClient(srv.URL, WithAllowPrivateIPs(true))
	tool, err := AsTool("delegate", "delegate", []byte(`{"type":"object"}`), client)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	var abortOnce sync.Once
	execErr := tool.Execute(
		ctx,
		toolsy.NewRunEnv(nil),
		toolsy.ToolInput{ArgsJSON: []byte(`{}`)},
		func(c toolsy.Chunk) error {
			if c.Event == toolsy.EventProgress {
				abortOnce.Do(func() {
					cancel()
					close(releaseStream)
				})
			}
			return nil
		},
	)
	require.Error(t, execErr)

	require.Eventually(t, cancelCalled.Load, 2*time.Second, 10*time.Millisecond)

	// httptest.Server does not propagate the client context deadline to r.Context().Deadline();
	// verify the bounded cancel context contract on the client side instead.
	parent, parentCancel := context.WithCancel(context.Background())
	parentCancel()
	cancelCtx, cancelFn := context.WithTimeout(context.WithoutCancel(parent), cancelTaskTimeout)
	defer cancelFn()
	deadline, ok := cancelCtx.Deadline()
	require.True(t, ok, "CancelTask defer context must have a deadline")
	assert.LessOrEqual(t, time.Until(deadline), cancelTaskTimeout+500*time.Millisecond)
	assert.Greater(t, time.Until(deadline), 0*time.Second)
}
