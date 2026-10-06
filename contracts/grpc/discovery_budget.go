package grpc

import (
	"errors"
	"fmt"

	reflectionpb "google.golang.org/grpc/reflection/grpc_reflection_v1"
	"google.golang.org/protobuf/proto"
)

// ErrDiscoveryLimit identifies a discovery quota refusal; no tools are published.
var ErrDiscoveryLimit = errors.New("grpc: discovery limit exceeded")

type discoveryBudget struct {
	maxServices int
	maxFiles    int
	maxBytes    int
	files       int
	bytes       int
}

func newDiscoveryBudget(opts Options) (*discoveryBudget, error) {
	if opts.MaxDiscoveryServices < 0 || opts.MaxDescriptorFiles < 0 || opts.MaxDiscoveryBytes < 0 ||
		opts.MaxResponseBytes < 0 {
		return nil, errors.New("grpc: negative limits are invalid")
	}
	budget := &discoveryBudget{
		maxServices: opts.MaxDiscoveryServices,
		maxFiles:    opts.MaxDescriptorFiles,
		maxBytes:    opts.MaxDiscoveryBytes,
		files:       0,
		bytes:       0,
	}
	if budget.maxServices == 0 {
		budget.maxServices = defaultMaxDiscoveryServices
	}
	if budget.maxFiles == 0 {
		budget.maxFiles = defaultMaxDescriptorFiles
	}
	if budget.maxBytes == 0 {
		budget.maxBytes = defaultMaxDiscoveryBytes
	}
	return budget, nil
}

func (b *discoveryBudget) response(response *reflectionpb.ServerReflectionResponse) error {
	size := proto.Size(response)
	if size > b.maxBytes-b.bytes {
		return discoveryLimit("bytes", b.maxBytes)
	}
	b.bytes += size
	return nil
}

func (b *discoveryBudget) file() error {
	if b.files >= b.maxFiles {
		return discoveryLimit("descriptor blobs", b.maxFiles)
	}
	b.files++
	return nil
}

func discoveryLimit(kind string, limit int) error {
	return fmt.Errorf("%w: %s cap %d", ErrDiscoveryLimit, kind, limit)
}
