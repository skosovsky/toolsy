package toolsy

import (
	"errors"
	"fmt"
	"slices"
)

// sessionConfiguration is immutable after publication. All entry points capture
// one value; callbacks execute after the capture, without a configuration lock.
type sessionConfiguration struct {
	registry *Registry
	binding  SessionBinding
}

func (s *Session) executionConfiguration() sessionConfiguration {
	if configuration := s.configuration.Load(); configuration != nil {
		return *configuration
	}
	return sessionConfiguration{}
}

// SessionBinding describes the registry/view/state schema boundary a session is bound to.
type SessionBinding struct {
	View              RegistryViewSnapshot `json:"view"`
	ToolNames         []string             `json:"tool_names"`
	ManifestDigest    string               `json:"manifest_digest"`
	PolicyDigest      string               `json:"policy_digest,omitempty"`
	StateSchemaDigest string               `json:"state_schema_digest,omitempty"`
}

// SessionCheckpoint is a state+binding checkpoint, not a full workflow continuation.
// It excludes RunPolicy, step limits/counts, dependencies and external effects.
// Hosts restore current authority and durable budgets explicitly.
type SessionCheckpoint struct {
	Binding  SessionBinding  `json:"binding"`
	Snapshot SessionSnapshot `json:"snapshot"`
}

// Binding returns the current execution binding for this session.
func (s *Session) Binding() SessionBinding {
	if s == nil {
		return SessionBinding{}
	}
	return cloneSessionBinding(s.executionConfiguration().binding)
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
		Binding:  snap.Binding(),
		Snapshot: snap,
	}, nil
}

// Rebind atomically moves this session to a compatible registry/view without rebuilding state.
// In-flight calls keep their captured registry; subsequent calls see the published
// configuration. Registry/manifest callbacks run without a configuration lock.
func (s *Session) Rebind(reg *Registry) error {
	if s == nil {
		return NewValidationError("session is nil")
	}
	target, err := newSessionBinding(reg, s.opts)
	if err != nil {
		return err
	}
	replacement := &sessionConfiguration{registry: reg, binding: target}
	for {
		current := s.configuration.Load()
		var binding SessionBinding
		if current != nil {
			binding = current.binding
		}
		if compatibilityErr := validateSessionBindingCompatible(binding, target); compatibilityErr != nil {
			return compatibilityErr
		}
		if s.configuration.CompareAndSwap(current, replacement) {
			return nil
		}
	}
}

// NewSessionFromCheckpoint creates a session and imports a state+binding checkpoint.
// The supplied options define current authority/budget; previous RunPolicy and
// execution counts are not restored. Codec registration is finalized by NewSession
// even if subsequent checkpoint validation/hydration fails.
func NewSessionFromCheckpoint(reg *Registry, checkpoint SessionCheckpoint, opts ...SessionOption) (*Session, error) {
	sess, err := NewSession(reg, opts...)
	if err != nil {
		return nil, err
	}
	if err := validateSessionBindingCompatible(checkpoint.Binding, sess.executionConfiguration().binding); err != nil {
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
