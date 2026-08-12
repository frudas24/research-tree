package retree

import (
	"os"
	"path/filepath"
	"testing"
)

// TestCreateNodeWithFeatureRollsBackOnPersistFailure verifies the composite
// operation restores next_id, nodes, and derived indexes if persistence fails
// after edges.jsonl is published but before relations.jsonl completes.
func TestCreateNodeWithFeatureRollsBackOnPersistFailure(t *testing.T) {
	s := mustInit(t, StorageJSON)
	parent := &Node{Frontmatter: Frontmatter{Title: "parent", Status: StatusActive}}
	mustNoErr(t, s.CreateNode(parent))

	beforeEdges, err := os.ReadFile(s.edgesPath())
	mustNoErr(t, err)
	beforeRelations, err := os.ReadFile(s.relationsPath())
	mustNoErr(t, err)

	relationsTmp := s.relationsPath() + ".tmp"
	if err := os.RemoveAll(relationsTmp); err != nil && !os.IsNotExist(err) {
		t.Fatalf("remove relations tmp path: %v", err)
	}
	if err := os.MkdirAll(relationsTmp, 0o755); err != nil {
		t.Fatalf("block relations tmp file path: %v", err)
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
	afterEdges, err := os.ReadFile(s.edgesPath())
	mustNoErr(t, err)
	if string(afterEdges) != string(beforeEdges) {
		t.Fatalf("expected edges rollback, before=%q after=%q", string(beforeEdges), string(afterEdges))
	}
	afterRelations, err := os.ReadFile(s.relationsPath())
	mustNoErr(t, err)
	if string(afterRelations) != string(beforeRelations) {
		t.Fatalf("expected relations rollback, before=%q after=%q", string(beforeRelations), string(afterRelations))
	}
	features, err := s.ListFeatures()
	mustNoErr(t, err)
	if len(features) != 0 {
		t.Fatalf("expected feature rollback, got %+v", features)
	}
}
