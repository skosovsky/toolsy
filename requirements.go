package toolsy

// MemoryAccess describes how a tool uses session in-memory state.
type MemoryAccess string

const (
	MemoryAccessNone      MemoryAccess = "none"
	MemoryAccessRead      MemoryAccess = "read"
	MemoryAccessReadWrite MemoryAccess = "readwrite"
)

// Permission is a host-defined capability label declared on a tool manifest.
type Permission string

// ToolRequirements holds typed declarative requirements for authorization and routing.
// Requirements are manifest metadata only; the registry does not enforce them at runtime.
// Hosts should enforce via middleware, [WithAuthorizer], or orchestrator policy.
type ToolRequirements struct {
	MemoryAccess MemoryAccess
	NeedsSession bool
	Permissions  []Permission
}

// cloneRequirements returns a defensive copy of requirements.
func cloneRequirements(r ToolRequirements) ToolRequirements {
	out := r
	if len(r.Permissions) > 0 {
		out.Permissions = append([]Permission(nil), r.Permissions...)
	}
	return out
}
