package agents

import (
	"strings"
	"testing"
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
