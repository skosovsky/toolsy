package toolsy

import (
	"errors"
	"fmt"
	"slices"
)

// SessionBinding describes the registry/view/state schema boundary a session is bound to.
type SessionBinding struct {
	View              RegistryViewSnapshot `json:"view"`
	ToolNames         []string             `json:"tool_names"`
	ManifestDigest    string               `json:"manifest_digest"`
	PolicyDigest      string               `json:"policy_digest,omitempty"`
	StateSchemaDigest string               `json:"state_schema_digest,omitempty"`
}

// SessionCheckpoint carries both typed state and the execution binding needed to resume it.
type SessionCheckpoint struct {
	Binding  SessionBinding  `json:"binding"`
	Snapshot SessionSnapshot `json:"snapshot"`
}

// Binding returns the current execution binding for this session.
func (s *Session) Binding() SessionBinding {
	if s == nil {
		return SessionBinding{}
	}
	return cloneSessionBinding(s.binding)
}

// ExportCheckpoint exports state together with the binding required to resume it safely.
func (s *Session) ExportCheckpoint() (SessionCheckpoint, error) {
	if s == nil {
		return SessionCheckpoint{}, NewValidationError("session is nil")
	}
	snap, err := s.ExportSnapshot()
	if err != nil {
		return SessionCheckpoint{}, err
	}
	return SessionCheckpoint{
		Binding:  cloneSessionBinding(s.binding),
		Snapshot: snap,
	}, nil
}

// Rebind moves this session to a compatible registry or registry view without rebuilding state.
func (s *Session) Rebind(reg *Registry) error {
	if s == nil {
		return NewValidationError("session is nil")
	}
	target, err := newSessionBinding(reg, s.opts)
	if err != nil {
		return err
	}
	if err := validateSessionBindingCompatible(s.binding, target); err != nil {
		return err
	}
	s.reg = reg
	s.binding = target
	return nil
}

// NewSessionFromCheckpoint creates a session and imports a checkpoint after binding validation.
func NewSessionFromCheckpoint(reg *Registry, checkpoint SessionCheckpoint, opts ...SessionOption) (*Session, error) {
	sess, err := NewSession(reg, opts...)
	if err != nil {
		return nil, err
	}
	if err := validateSessionBindingCompatible(checkpoint.Binding, sess.binding); err != nil {
		return nil, err
	}
	if err := sess.ImportSnapshot(checkpoint.Snapshot); err != nil {
		return nil, err
	}
	return sess, nil
}

func newSessionBinding(reg *Registry, opts sessionOptions) (SessionBinding, error) {
	if reg == nil {
		return SessionBinding{
			View: RegistryViewSnapshot{
				ID:                "",
				ToolNames:         nil,
				RequiredToolNames: nil,
				ManifestDigest:    "",
				PolicyDigest:      "",
				Reason:            "",
				Owner:             "",
			},
			ToolNames:         nil,
			ManifestDigest:    "",
			PolicyDigest:      "",
			StateSchemaDigest: stateSchemaDigest(opts.codecRegistry),
		}, nil
	}
	digest, err := registryManifestDigest(reg)
	if err != nil {
		return SessionBinding{}, err
	}
	names := reg.ToolNames()
	slices.Sort(names)
	return SessionBinding{
		View:              cloneRegistryViewSnapshot(reg.opts.view),
		ToolNames:         names,
		ManifestDigest:    digest,
		PolicyDigest:      reg.opts.policyDigest,
		StateSchemaDigest: stateSchemaDigest(opts.codecRegistry),
	}, nil
}

func validateSessionBindingCompatible(want SessionBinding, got SessionBinding) error {
	if want.ManifestDigest != got.ManifestDigest {
		return newSessionBindingMismatchError("manifest digest mismatch", "manifest_digest")
	}
	if want.PolicyDigest != got.PolicyDigest {
		return newSessionBindingMismatchError("policy digest mismatch", "policy_digest")
	}
	if want.StateSchemaDigest != got.StateSchemaDigest {
		return newSessionBindingMismatchError("state schema digest mismatch", "state_schema_digest")
	}
	if want.View.ID != got.View.ID {
		return newSessionBindingMismatchError("view id mismatch", "view.id")
	}
	if !slices.Equal(want.ToolNames, got.ToolNames) {
		return newSessionBindingMismatchError("tool set mismatch", "tool_names")
	}
	return nil
}

func cloneSessionBinding(in SessionBinding) SessionBinding {
	out := in
	out.View = cloneRegistryViewSnapshot(in.View)
	out.ToolNames = append([]string(nil), in.ToolNames...)
	return out
}

func newSessionBindingMismatchError(reason string, fixableArgs ...string) *ToolError {
	return &ToolError{
		Code:        CodeToolsContractMissing,
		Reason:      "session binding " + reason,
		Retryable:   false,
		FixableArgs: append([]string(nil), fixableArgs...),
		SafeMessage: "",
		Err:         fmt.Errorf("toolsy: session binding %w", errors.New(reason)),
	}
}
