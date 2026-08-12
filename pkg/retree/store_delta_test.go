package retree

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// readNodeFile returns the raw on-disk JSON for a node file.
func readNodeFile(t *testing.T, s *Store, id NodeID) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(s.nodesDir(), fmt.Sprintf("%04d.json", id)))
	if err != nil {
		t.Fatalf("read node file %d: %v", id, err)
	}
	return b
}

// TestJSONDeltaWriteOnlyDirtyFile verifies updating one node leaves other
// node files byte-identical (delta persistence, not full rewrite).
func TestJSONDeltaWriteOnlyDirtyFile(t *testing.T) {
	root := t.TempDir()
	s, err := Init(filepath.Join(root, "research"), StorageJSON)
	if err != nil {
		t.Fatalf("init: %v", err)
	}
	a := &Node{Frontmatter: Frontmatter{Title: "A", Status: StatusActive}}
	if err := s.CreateNode(a); err != nil {
		t.Fatalf("create A: %v", err)
	}
	b := &Node{Frontmatter: Frontmatter{Title: "B", Status: StatusActive}}
	if err := s.CreateNode(b); err != nil {
		t.Fatalf("create B: %v", err)
	}

	before := readNodeFile(t, s, a.ID)
	b.Status = StatusDone
	b.Outcome = OutcomeSuccess
	if err := s.UpdateNode(b); err != nil {
		t.Fatalf("update B: %v", err)
	}
	after := readNodeFile(t, s, a.ID)
	if string(before) != string(after) {
		t.Fatalf("unrelated node A file must be untouched by updating B")
	}

	got, err := s.GetNode(b.ID)
	if err != nil {
		t.Fatalf("get B: %v", err)
	}
	if got.Status != StatusDone {
		t.Fatalf("B update lost: %s", got.Status)
	}
}

// TestJSONDeltaCreateDoesNotRewriteSiblings verifies creating a new node does
// not rewrite existing node files.
func TestJSONDeltaCreateDoesNotRewriteSiblings(t *testing.T) {
	root := t.TempDir()
	s, err := Init(filepath.Join(root, "research"), StorageJSON)
	if err != nil {
		t.Fatalf("init: %v", err)
	}
	a := &Node{Frontmatter: Frontmatter{Title: "A", Status: StatusActive}}
	if err := s.CreateNode(a); err != nil {
		t.Fatalf("create A: %v", err)
	}
	before := readNodeFile(t, s, a.ID)
	b := &Node{Frontmatter: Frontmatter{Title: "B", Status: StatusActive}}
	if err := s.CreateNode(b); err != nil {
		t.Fatalf("create B: %v", err)
	}
	if string(before) != string(readNodeFile(t, s, a.ID)) {
		t.Fatalf("creating B must not rewrite A")
	}
}

// TestJSONDeleteForceRewritesChildrenAndRemovesFile verifies a forced delete
// updates the orphaned children's files and removes the deleted node's file.
func TestJSONDeleteForceRewritesChildrenAndRemovesFile(t *testing.T) {
	root := t.TempDir()
	s, err := Init(filepath.Join(root, "research"), StorageJSON)
	if err != nil {
		t.Fatalf("init: %v", err)
	}
	a := &Node{Frontmatter: Frontmatter{Title: "A", Status: StatusActive}}
	if err := s.CreateNode(a); err != nil {
		t.Fatalf("create A: %v", err)
	}
	b := &Node{Frontmatter: Frontmatter{Title: "B", Status: StatusActive, Parents: []NodeID{a.ID}}}
	if err := s.CreateNode(b); err != nil {
		t.Fatalf("create B: %v", err)
	}
	c := &Node{Frontmatter: Frontmatter{Title: "C", Status: StatusActive, Parents: []NodeID{b.ID}}}
	if err := s.CreateNode(c); err != nil {
		t.Fatalf("create C: %v", err)
	}

	if err := s.DeleteNode(a.ID, true); err != nil {
		t.Fatalf("delete A force: %v", err)
	}
	if _, err := os.Stat(filepath.Join(s.nodesDir(), "0001.json")); !os.IsNotExist(err) {
		t.Fatalf("deleted node file must be removed: %v", err)
	}
	gotB, err := s.GetNode(b.ID)
	if err != nil {
		t.Fatalf("get B: %v", err)
	}
	if len(gotB.Parents) != 0 {
		t.Fatalf("B must be orphaned after force delete, parents=%v", gotB.Parents)
	}
	if _, err := s.GetNode(c.ID); err != nil {
		t.Fatalf("grandchild C must survive: %v", err)
	}
}

// TestJSONDeltaDiskMatchesGraph verifies after a mixed workload the JSON
// files on disk exactly match the in-memory graph (no stale or missing files).
func TestJSONDeltaDiskMatchesGraph(t *testing.T) {
	root := t.TempDir()
	s, err := Init(filepath.Join(root, "research"), StorageJSON)
	if err != nil {
		t.Fatalf("init: %v", err)
	}
	ids := make([]NodeID, 0, 5)
	for i := 0; i < 5; i++ {
		n := &Node{Frontmatter: Frontmatter{Title: titleFor(i), Status: StatusActive}}
		if err := s.CreateNode(n); err != nil {
			t.Fatalf("create %d: %v", i, err)
		}
		ids = append(ids, n.ID)
	}
	// Update one, delete another, invalidate a third.
	n2, err := s.GetNode(ids[1])
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	n2.Status = StatusDone
	n2.Outcome = OutcomeFailure
	if err := s.UpdateNode(n2); err != nil {
		t.Fatalf("update: %v", err)
	}
	if err := s.DeleteNode(ids[3], true); err != nil {
		t.Fatalf("delete: %v", err)
	}
	refuter, err := s.GetNode(ids[4])
	if err != nil {
		t.Fatalf("get refuter: %v", err)
	}
	if err := s.InvalidateClaim(ids[0], refuter.ID, "repro failed"); err != nil {
		t.Fatalf("invalidate: %v", err)
	}

	all, err := s.QueryNodes(Filter{})
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(all) != 4 {
		t.Fatalf("want 4 nodes, got %d", len(all))
	}
	entries, err := os.ReadDir(s.nodesDir())
	if err != nil {
		t.Fatalf("readdir: %v", err)
	}
	jsonFiles := 0
	for _, e := range entries {
		if !e.IsDir() && filepath.Ext(e.Name()) == ".json" {
			jsonFiles++
		}
	}
	if jsonFiles != 4 {
		t.Fatalf("want exactly 4 node files on disk, got %d", jsonFiles)
	}
}

// titleFor builds a deterministic node title for a test index.
func titleFor(i int) string {
	return "node-" + string(rune('A'+i))
}
