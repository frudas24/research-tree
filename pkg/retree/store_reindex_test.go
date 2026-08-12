package retree

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestRegenerateBinIndexRecoversLostIndex verifies nodes.idx can be rebuilt
// from nodes.bin after being deleted.
func TestRegenerateBinIndexRecoversLostIndex(t *testing.T) {
	root := t.TempDir()
	s, err := Init(filepath.Join(root, "research"), StorageBIN)
	if err != nil {
		t.Fatalf("init: %v", err)
	}
	for _, title := range []string{"A", "B", "C"} {
		n := &Node{Frontmatter: Frontmatter{Title: title, Status: StatusActive}}
		if err := s.CreateNode(n); err != nil {
			t.Fatalf("create %s: %v", title, err)
		}
	}
	if err := os.Remove(s.nodesIdxPath()); err != nil {
		t.Fatalf("remove index: %v", err)
	}

	// With the index gone, loading must fail loudly instead of returning an
	// empty graph (which would destroy nodes.bin on the next write).
	if _, err := s.GetNode(1); err == nil {
		t.Fatalf("expected loud failure when index is missing")
	}

	if err := s.RegenerateBinIndex(); err != nil {
		t.Fatalf("regenerate: %v", err)
	}
	all, err := s.QueryNodes(Filter{})
	if err != nil {
		t.Fatalf("query after regenerate: %v", err)
	}
	if len(all) != 3 {
		t.Fatalf("want 3 nodes after regenerate, got %d", len(all))
	}
	for _, n := range all {
		if n.Title == "" {
			t.Fatalf("node %d lost its payload after regenerate", n.ID)
		}
	}
}

// TestRegenerateBinIndexAfterMutation verifies the rebuilt index matches the
// writer's own index, including updated nodes (new offsets/lengths).
func TestRegenerateBinIndexAfterMutation(t *testing.T) {
	root := t.TempDir()
	s, err := Init(filepath.Join(root, "research"), StorageBIN)
	if err != nil {
		t.Fatalf("init: %v", err)
	}
	n := &Node{Frontmatter: Frontmatter{Title: "before", Status: StatusActive}}
	if err := s.CreateNode(n); err != nil {
		t.Fatalf("create: %v", err)
	}
	n.Status = StatusDone
	n.Outcome = OutcomeSuccess
	if err := s.UpdateNode(n); err != nil {
		t.Fatalf("update: %v", err)
	}

	original, err := os.ReadFile(s.nodesIdxPath())
	if err != nil {
		t.Fatalf("read original index: %v", err)
	}
	if err := s.RegenerateBinIndex(); err != nil {
		t.Fatalf("regenerate: %v", err)
	}
	rebuilt, err := os.ReadFile(s.nodesIdxPath())
	if err != nil {
		t.Fatalf("read rebuilt index: %v", err)
	}
	if string(original) != string(rebuilt) {
		t.Fatalf("rebuilt index diverges from writer index:\n%s\nvs\n%s", rebuilt, original)
	}
	got, err := s.GetNode(n.ID)
	if err != nil {
		t.Fatalf("get after regenerate: %v", err)
	}
	if got.Status != StatusDone {
		t.Fatalf("updated state lost: %s", got.Status)
	}
}

// TestRegenerateBinIndexDetectsCorruption verifies a structurally corrupted
// nodes.bin fails regeneration loudly instead of producing a wrong index.
func TestRegenerateBinIndexDetectsCorruption(t *testing.T) {
	root := t.TempDir()
	s, err := Init(filepath.Join(root, "research"), StorageBIN)
	if err != nil {
		t.Fatalf("init: %v", err)
	}
	for _, title := range []string{"A", "B", "C"} {
		n := &Node{Frontmatter: Frontmatter{Title: title, Status: StatusActive}}
		if err := s.CreateNode(n); err != nil {
			t.Fatalf("create %s: %v", title, err)
		}
	}

	// Truncate nodes.bin mid-payload: the last node's bytes are gone, so the
	// sequential scan must fail rather than silently skipping.
	binPath := s.nodesBinPath()
	fi, err := os.Stat(binPath)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	truncated := fi.Size() - 3
	if err := os.Truncate(binPath, truncated); err != nil {
		t.Fatalf("truncate: %v", err)
	}
	if err := s.RegenerateBinIndex(); err == nil {
		t.Fatalf("regeneration must fail on corrupted nodes.bin")
	}
}

// TestRegenerateBinIndexEmptyStore verifies an empty binary store
// regenerates cleanly.
func TestRegenerateBinIndexEmptyStore(t *testing.T) {
	root := t.TempDir()
	s, err := Init(filepath.Join(root, "research"), StorageBIN)
	if err != nil {
		t.Fatalf("init: %v", err)
	}
	if err := os.Remove(s.nodesIdxPath()); err != nil {
		t.Fatalf("remove index: %v", err)
	}
	if err := s.RegenerateBinIndex(); err != nil {
		t.Fatalf("regenerate empty: %v", err)
	}
	all, err := s.QueryNodes(Filter{})
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(all) != 0 {
		t.Fatalf("want 0 nodes, got %d", len(all))
	}
}

// TestRegenerateBinIndexRejectsJSONMode verifies the recovery path refuses
// to run on a JSON store.
func TestRegenerateBinIndexRejectsJSONMode(t *testing.T) {
	root := t.TempDir()
	s, err := Init(filepath.Join(root, "research"), StorageJSON)
	if err != nil {
		t.Fatalf("init: %v", err)
	}
	if err := s.RegenerateBinIndex(); err == nil {
		t.Fatalf("reindex must fail in json mode")
	} else if !strings.Contains(err.Error(), "bin mode") {
		t.Fatalf("unexpected error: %v", err)
	}
}
