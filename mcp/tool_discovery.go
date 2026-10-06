package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
)

// ToolDiscovery is one complete typed discovery. Page cache hints are not
// collapsed into a fabricated aggregate TTL. Descriptors are not permission grants.
type ToolDiscovery struct {
	Tools      []MCPTool
	Pages      []ToolsListResult
	Generation uint64
}

type toolDiscoveryBudget struct {
	limits       PaginationLimits
	bytes, items int
}

func newToolDiscoveryBudget(limits PaginationLimits) toolDiscoveryBudget {
	return toolDiscoveryBudget{limits: limits.normalized(), bytes: 0, items: 0}
}

func (b *toolDiscoveryBudget) accept(raw json.RawMessage) error {
	if len(raw) > b.limits.MaxBytes-b.bytes {
		return &InvalidPayloadError{
			Subject: toolsListResultSubject,
			Err:     errors.New("aggregate discovery byte limit exceeded"),
		}
	}
	if _, err := requireCompleteResult(raw, MethodToolsList, false); err != nil {
		return err
	}
	fields, err := decodeObjectFields(raw)
	if err != nil {
		return &InvalidPayloadError{Subject: toolsListResultSubject, Err: err}
	}
	var tools []json.RawMessage
	if err = json.Unmarshal(fields["tools"], &tools); err != nil {
		return &InvalidPayloadError{Subject: toolsListResultSubject, Err: err}
	}
	if len(tools) > b.limits.MaxItems-b.items {
		return &InvalidPayloadError{
			Subject: toolsListResultSubject,
			Err:     errors.New("aggregate discovery item limit exceeded"),
		}
	}
	b.bytes += len(raw)
	b.items += len(tools)
	return nil
}

func (c *Client) discoverToolCandidates(ctx context.Context) (ToolDiscovery, []toolBindingCandidate, error) {
	if err := c.requireCapability("tools"); err != nil {
		return ToolDiscovery{}, nil, err
	}
	generation := c.toolGeneration.Load()
	budget := newToolDiscoveryBudget(c.opts.Pagination)
	pages := make([]ToolsListResult, 0)
	fetch := func(ctx context.Context, cursor string) ([]toolBindingCandidate, string, error) {
		if current := c.toolGeneration.Load(); current != generation {
			return nil, "", staleError(InvalidationTools, generation, current)
		}
		result, candidates, err := c.listToolsPage(ctx, cursor, generation, &budget)
		if err != nil {
			return nil, "", err
		}
		pages = append(pages, result)
		return candidates, result.NextCursor, nil
	}
	all := make([]toolBindingCandidate, 0)
	seen := make(map[string]struct{})
	for candidate, err := range IterateCursorWithLimits(ctx, c.opts.Pagination, fetch) {
		if err != nil {
			return ToolDiscovery{}, nil, err
		}
		if _, duplicate := seen[candidate.descriptor.Name]; duplicate {
			return ToolDiscovery{}, nil, &InvalidPayloadError{
				Subject: toolsListResultSubject,
				Err:     fmt.Errorf("duplicate tool name %q across pages", candidate.descriptor.Name),
			}
		}
		seen[candidate.descriptor.Name] = struct{}{}
		all = append(all, candidate)
	}
	if err := ctx.Err(); err != nil {
		return ToolDiscovery{}, nil, err
	}
	if err := c.publishToolAuthority(ctx, all, generation); err != nil {
		return ToolDiscovery{}, nil, err
	}
	tools := make([]MCPTool, len(all))
	for i, candidate := range all {
		tools[i] = candidate.descriptor
	}
	return ToolDiscovery{Tools: tools, Pages: pages, Generation: generation}, all, nil
}
