package toolsy

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"hash"
	"iter"
	"slices"
)

// RegistryViewSpec describes a durable capability view over a root registry.
type RegistryViewSpec struct {
	ToolNames         []string
	RequiredToolNames []string
	Reason            string
	Owner             string
	Policy            Policy
}

// RegistryViewSnapshot is the serializable identity of a registry view.
type RegistryViewSnapshot struct {
	ID                string   `json:"id"`
	ToolNames         []string `json:"tool_names"`
	RequiredToolNames []string `json:"required_tool_names"`
	ManifestDigest    string   `json:"manifest_digest"`
	Reason            string   `json:"reason"`
	Owner             string   `json:"owner"`
}

// RegistryView is a first-class capability object over a registry subset.
type RegistryView struct {
	reg      *Registry
	snapshot RegistryViewSnapshot
}

// View creates a durable capability view with its own policy layered over the root registry policy.
func (r *Registry) View(spec RegistryViewSpec) (*RegistryView, error) {
	if spec.ToolNames == nil {
		spec.ToolNames = r.ToolNames()
	}
	subset, err := r.subsetWithPolicy(nil, spec.ToolNames...)
	if err != nil {
		return nil, err
	}
	if len(spec.RequiredToolNames) > 0 {
		manifestSet, manifestErr := subset.ManifestSet()
		if manifestErr != nil {
			return nil, manifestErr
		}
		if contractErr := ValidateManifestContract(manifestSet, spec.RequiredToolNames); contractErr != nil {
			return nil, contractErr
		}
	}
	snapshot, err := newRegistryViewSnapshot(spec, subset)
	if err != nil {
		return nil, err
	}
	subset.opts.policy = composePolicies(subset.opts.policy, spec.Policy)
	subset.opts.view = snapshot
	return &RegistryView{reg: subset, snapshot: snapshot}, nil
}

// RestoreView recreates a view from a previously saved snapshot.
func (r *Registry) RestoreView(snapshot RegistryViewSnapshot, policy Policy) (*RegistryView, error) {
	if snapshot.ID == "" {
		return nil, newRegistryViewSnapshotMismatchError("missing id", "id")
	}
	if snapshot.ManifestDigest == "" {
		return nil, newRegistryViewSnapshotMismatchError("missing manifest digest", "manifest_digest")
	}
	view, err := r.View(RegistryViewSpec{
		ToolNames:         snapshot.ToolNames,
		RequiredToolNames: snapshot.RequiredToolNames,
		Reason:            snapshot.Reason,
		Owner:             snapshot.Owner,
		Policy:            policy,
	})
	if err != nil {
		return nil, err
	}
	if snapshot.ManifestDigest != view.snapshot.ManifestDigest {
		return nil, newRegistryViewSnapshotMismatchError("manifest digest mismatch", "manifest_digest")
	}
	if snapshot.ID != view.snapshot.ID {
		return nil, newRegistryViewSnapshotMismatchError("id mismatch", "id")
	}
	return view, nil
}

// Snapshot returns the durable view identity.
func (v *RegistryView) Snapshot() RegistryViewSnapshot {
	if v == nil {
		return RegistryViewSnapshot{}
	}
	out := v.snapshot
	out.ToolNames = append([]string(nil), v.snapshot.ToolNames...)
	out.RequiredToolNames = append([]string(nil), v.snapshot.RequiredToolNames...)
	return out
}

// ToolNames returns the tools visible through the view.
func (v *RegistryView) ToolNames() []string {
	if v == nil || v.reg == nil {
		return nil
	}
	return v.reg.ToolNames()
}

// ManifestSet returns the manifest set visible through the view.
func (v *RegistryView) ManifestSet() (ManifestSet, error) {
	if v == nil || v.reg == nil {
		return ManifestSet{}, nil
	}
	return v.reg.ManifestSet()
}

// ValidateManifestContract validates required tools against this view.
func (v *RegistryView) ValidateManifestContract(requiredNames []string) error {
	if len(requiredNames) == 0 {
		requiredNames = v.Snapshot().RequiredToolNames
	}
	ms, err := v.ManifestSet()
	if err != nil {
		return err
	}
	return ValidateManifestContract(ms, requiredNames)
}

// Execute runs a call through the view.
func (v *RegistryView) Execute(ctx context.Context, call ToolCall, yield func(Chunk) error) error {
	if v == nil || v.reg == nil {
		return NewRegistryStateError()
	}
	return v.reg.Execute(ctx, call, yield)
}

// ExecuteIter runs one view-scoped tool call and returns an iterator over chunks.
func (v *RegistryView) ExecuteIter(ctx context.Context, call ToolCall) iter.Seq2[Chunk, error] {
	return func(yield func(Chunk, error) bool) {
		ctxChild, cancel := context.WithCancel(ctx)
		defer cancel()
		var consumerStopped bool

		err := v.Execute(ctxChild, call, func(c Chunk) error {
			if consumerStopped {
				return context.Canceled
			}
			if !yield(c, nil) {
				consumerStopped = true
				cancel()
				return context.Canceled
			}
			return nil
		})

		if !consumerStopped && err != nil && !isContextInterrupt(err) {
			yield(Chunk{}, err)
		}
	}
}

// NewSession creates a session bound to this view's registry.
func (v *RegistryView) NewSession(opts ...SessionOption) (*Session, error) {
	if v == nil || v.reg == nil {
		return nil, NewRegistryStateError()
	}
	return NewSession(v.reg, opts...)
}

// Shutdown delegates lifecycle control to the shared root runtime state.
func (v *RegistryView) Shutdown(ctx context.Context) error {
	if v == nil || v.reg == nil {
		return NewRegistryStateError()
	}
	return v.reg.Shutdown(ctx)
}

func newRegistryViewSnapshot(spec RegistryViewSpec, reg *Registry) (RegistryViewSnapshot, error) {
	names := append([]string(nil), spec.ToolNames...)
	slices.Sort(names)
	required := append([]string(nil), spec.RequiredToolNames...)
	slices.Sort(required)
	manifestDigest, err := registryManifestDigest(reg)
	if err != nil {
		return RegistryViewSnapshot{}, err
	}
	snapshot := RegistryViewSnapshot{
		ID:                "",
		ToolNames:         names,
		RequiredToolNames: required,
		ManifestDigest:    manifestDigest,
		Reason:            spec.Reason,
		Owner:             spec.Owner,
	}
	id, err := registryViewSnapshotID(snapshot)
	if err != nil {
		return RegistryViewSnapshot{}, err
	}
	snapshot.ID = id
	return snapshot, nil
}

func registryViewSnapshotID(snapshot RegistryViewSnapshot) (string, error) {
	snapshot.ID = ""
	payload, err := json.Marshal(snapshot)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:]), nil
}

func registryManifestDigest(reg *Registry) (string, error) {
	manifestSet, err := reg.ManifestSet()
	if err != nil {
		return "", err
	}
	h := sha256.New()
	for _, name := range manifestSet.Names() {
		manifest, ok := manifestSet.Manifest(name)
		if !ok {
			continue
		}
		if err := writeManifestDigest(h, manifest); err != nil {
			return "", err
		}
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func writeManifestDigest(h hash.Hash, manifest ToolManifest) error {
	fmt.Fprintf(
		h,
		"%s\x00%s\x00%s\x00%t\x00%t\x00%t\x00%t\x00%s\x00%t\x00%s\x00",
		manifest.Name,
		manifest.Description,
		manifest.Version,
		manifest.ReadOnly,
		manifest.RequiresConfirmation,
		manifest.Dangerous,
		manifest.Idempotent,
		manifest.CompletionPolicy,
		manifest.Requirements.NeedsSession,
		manifest.Requirements.MemoryAccess,
	)
	if err := writeDigestJSON(h, manifest.Tags); err != nil {
		return err
	}
	if err := writeDigestJSON(h, manifest.Parameters); err != nil {
		return err
	}
	if err := writeDigestJSON(h, manifest.OutputSchema); err != nil {
		return err
	}
	return writeDigestJSON(h, manifest.Requirements.Permissions)
}

func writeDigestJSON(h hash.Hash, v any) error {
	payload, err := json.Marshal(v)
	if err != nil {
		return err
	}
	_, _ = h.Write(payload)
	_, _ = h.Write([]byte{0})
	return nil
}

func cloneRegistryViewSnapshot(snapshot RegistryViewSnapshot) RegistryViewSnapshot {
	out := snapshot
	out.ToolNames = append([]string(nil), snapshot.ToolNames...)
	out.RequiredToolNames = append([]string(nil), snapshot.RequiredToolNames...)
	return out
}

func newRegistryViewSnapshotMismatchError(reason string, fixableArgs ...string) *ToolError {
	return &ToolError{
		Code:        CodeToolsContractMissing,
		Reason:      "registry view snapshot " + reason,
		Retryable:   false,
		FixableArgs: append([]string(nil), fixableArgs...),
		SafeMessage: "",
		Err:         errors.New("registry view snapshot " + reason),
	}
}
