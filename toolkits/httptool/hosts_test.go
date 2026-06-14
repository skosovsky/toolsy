package httptool

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMatchHost(t *testing.T) {
	t.Parallel()
	tests := []struct {
		host  string
		entry string
		want  bool
	}{
		{"api.slack.com", ".slack.com", true},
		{"hooks.slack.com", ".slack.com", true},
		{"slack.com", ".slack.com", false},
		{"evil-slack.com", ".slack.com", false},
		{"api.evil.com", ".evil.com", true},
		{"evil.com", ".evil.com", false},
		{"api.example.com", "api.example.com", true},
		{"sub.api.example.com", "api.example.com", true},
		{"notapi.example.com", "api.example.com", false},
	}
	for _, tt := range tests {
		t.Run(tt.host+"_"+tt.entry, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, MatchHost(tt.host, tt.entry))
		})
	}
}

func TestHostBlocked(t *testing.T) {
	t.Parallel()
	assert.True(t, HostBlocked("api.evil.com", []string{".evil.com"}))
	assert.False(t, HostBlocked("good.com", []string{".evil.com"}))
}
