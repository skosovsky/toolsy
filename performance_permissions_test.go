package toolsy

import "testing"

func TestMemoryPermissionsEmptyOwnership(t *testing.T) {
	// Arrange: empty visible permissions must not expose or retain a large backing array.
	backing := make([]Permission, 65536)
	backing[0] = "original"
	input := ToolManifest{Requirements: ToolRequirements{Permissions: backing[:0]}}
	// Act.
	snapshot := cloneManifestForPolicy(input)
	grown := snapshot.Requirements.Permissions
	grown = append(grown, "changed")
	// Assert: empty stays nonnil, capacity is detached, and policy append cannot touch source.
	if snapshot.Requirements.Permissions == nil || len(snapshot.Requirements.Permissions) != 0 ||
		cap(snapshot.Requirements.Permissions) != 0 {
		t.Fatal("empty permissions retain source capacity or lose nonnil type")
	}
	if backing[0] != "original" || grown[0] != "changed" {
		t.Fatal("policy permissions mutate source")
	}
}

func TestMemoryPermissionsNilAndPopulated(t *testing.T) {
	// Arrange.
	backing := []Permission{"original", "unused"}
	input := ToolRequirements{Permissions: backing[:1]}
	// Act.
	snapshot := cloneRequirements(input)
	snapshot.Permissions[0] = "changed"
	nilSnapshot := cloneRequirements(ToolRequirements{})
	// Assert: populated copy is independent and nil remains nil.
	if backing[0] != "original" || len(snapshot.Permissions) != 1 || cap(snapshot.Permissions) != 1 {
		t.Fatal("populated permissions ownership lost")
	}
	if nilSnapshot.Permissions != nil {
		t.Fatal("nil permissions changed")
	}
}
