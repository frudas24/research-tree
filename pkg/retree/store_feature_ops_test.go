package retree

import (
	"os"
	"path/filepath"
	"testing"
)

// TestCreateNodeWithFeatureRollsBackOnPersistFailure verifies the composite
// operation restores next_id and removes the node if persistence fails after
// the node file write but before the feature payload is committed.
func TestCreateNodeWithFeatureRollsBackOnPersistFailure(t *testing.T) {
	s := mustInit(t, StorageJSON)
	parent := &Node{Frontmatter: Frontmatter{Title: "parent", Status: StatusActive}}
	mustNoErr(t, s.CreateNode(parent))

	if err := os.RemoveAll(s.edgesPath()); err != nil && !os.IsNotExist(err) {
		t.Fatalf("remove edges path: %v", err)
	}
	if err := os.MkdirAll(s.edgesPath(), 0o755); err != nil {
		t.Fatalf("block edges file path: %v", err)
	}

	child := &Node{Frontmatter: Frontmatter{Title: "child", Status: StatusActive, Parents: []NodeID{parent.ID}}}
	if err := s.CreateNodeWithFeature(child, "NewFeature", RoleImplementation, true, parent.ID); err == nil {
		t.Fatal("expected composite create to fail")
	}

	if next := s.NextID(); next != 2 {
		t.Fatalf("expected next_id rollback to 2, got %d", next)
	}
	if _, err := s.GetNode(2); err == nil {
		t.Fatal("expected created node to be rolled back")
	}
	if _, err := os.Stat(filepath.Join(s.nodesDir(), "0002.json")); !os.IsNotExist(err) {
		t.Fatalf("expected node file rollback, got %v", err)
	}
	features, err := s.ListFeatures()
	mustNoErr(t, err)
	if len(features) != 0 {
		t.Fatalf("expected feature rollback, got %+v", features)
	}
}
