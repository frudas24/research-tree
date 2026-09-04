package retree

import (
	"errors"
	"fmt"
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

// TestRollbackCreatedPrimaryStateAcceptsLateBinaryFailure verifies rollback
// continues restoring auxiliary state after its BIN mutation has committed.
func TestRollbackCreatedPrimaryStateAcceptsLateBinaryFailure(t *testing.T) {
	s := mustInit(t, StorageBIN)
	first := &Node{Frontmatter: Frontmatter{Title: "first"}}
	second := &Node{Frontmatter: Frontmatter{Title: "second", Parents: []NodeID{1}}}
	mustNoErr(t, s.CreateNode(first))
	second.Parents = []NodeID{first.ID}

	edges, err := captureFileSnapshot(s.edgesPath())
	mustNoErr(t, err)
	relations, err := captureFileSnapshot(s.relationsPath())
	mustNoErr(t, err)
	previousNext, err := s.readNextID()
	mustNoErr(t, err)
	mustNoErr(t, s.CreateNode(second))

	writeThenFailDerived := func(nodes []*Node) error {
		if err := s.writeAllNodesBIN(nodes); err != nil {
			return err
		}
		return fmt.Errorf("%w: injected after binary rollback commit", ErrDerivedState)
	}
	if err := s.rollbackCreatedPrimaryStateWithBINWriter(previousNext, second.ID, edges, relations, writeThenFailDerived); err != nil {
		t.Fatalf("committed binary rollback reported failure: %v", err)
	}
	if _, err := s.GetNode(second.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("rolled-back node remains visible: %v", err)
	}
	next, err := s.readNextID()
	mustNoErr(t, err)
	if next != previousNext {
		t.Fatalf("next_id not restored: got %d want %d", next, previousNext)
	}
}
