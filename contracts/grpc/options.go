package grpc

import (
	"google.golang.org/grpc"
)

const defaultMaxResponseBytes = 512 * 1024

// Options configures the gRPC reflector and executor.
type Options struct {
	DialOptions      []grpc.DialOption
	Services         []string // allowlist of service full names; empty = all
	MaxResponseBytes int
}

func (o *Options) maxResponseBytes() int {
	if o != nil && o.MaxResponseBytes > 0 {
		return o.MaxResponseBytes
	}
	return defaultMaxResponseBytes
}
