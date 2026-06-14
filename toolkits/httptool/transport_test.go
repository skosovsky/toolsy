package httptool

import (
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsPrivateIP(t *testing.T) {
	t.Parallel()
	assert.True(t, IsPrivateIP(net.ParseIP("127.0.0.1")))
	assert.True(t, IsPrivateIP(net.ParseIP("10.0.0.1")))
	assert.True(t, IsPrivateIP(net.ParseIP("192.168.1.1")))
	assert.False(t, IsPrivateIP(net.ParseIP("8.8.8.8")))
}

func TestDefaultLoopbackBlockedHosts(t *testing.T) {
	t.Parallel()
	blocked := DefaultLoopbackBlockedHosts()
	_, ok := blocked["localhost"]
	assert.True(t, ok)
	_, ok = blocked["127.0.0.1"]
	assert.True(t, ok)
}
