package grpc

const (
	defaultMaxResponseBytes     = 512 * 1024
	defaultMaxDiscoveryServices = 256
	defaultMaxDescriptorFiles   = 512
	defaultMaxDiscoveryBytes    = 8 << 20
)

// Options configures the gRPC reflector and executor.
type Options struct {
	Services         []string // pre-fetch allowlist of service full names; empty = all
	MaxResponseBytes int
	// Discovery caps are inclusive; zero selects finite defaults, negative rejects.
	MaxDiscoveryServices int // all listed entries, including excluded and duplicate names
	MaxDescriptorFiles   int // received descriptor blobs, including duplicates/dependencies
	MaxDiscoveryBytes    int // aggregate protobuf response size, including list/envelopes/unknowns
}

func (o *Options) maxResponseBytes() int {
	if o != nil && o.MaxResponseBytes > 0 {
		return o.MaxResponseBytes
	}
	return defaultMaxResponseBytes
}
