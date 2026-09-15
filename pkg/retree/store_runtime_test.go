package retree

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestSeparateRuntimePreservesHistoryAndRejectsOldHandle(t *testing.T) {
	root := filepath.Join(t.TempDir(), "research")
	old, err := Init(root, StorageBIN)
	if err != nil {
		t.Fatal(err)
	}
	n := &Node{Frontmatter: Frontmatter{Title: "finding"}, Body: "preserve"}
	if err = old.CreateNode(n); err != nil {
		t.Fatal(err)
	}
	if err = SeparateRuntime(root); err != nil {
		t.Fatal(err)
	}
	if err = SeparateRuntime(root); err != nil {
		t.Fatal(err)
	}
	if _, err = old.GetNode(n.ID); !errors.Is(err, ErrStaleStore) {
		t.Fatalf("old handle: %v", err)
	}
	s, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	got, err := s.GetNode(n.ID)
	if err != nil || got.Body != "preserve" {
		t.Fatal(got, err)
	}
	if filepath.Dir(s.lockPath()) != filepath.Join(root, ".state") {
		t.Fatal(s.lockPath())
	}
	if _, err = os.Stat(filepath.Join(root, "history")); err != nil {
		t.Fatal(err)
	}
}

func TestSeparatedRuntimeSurvivesSnapshotRestore(t *testing.T) {
	root := filepath.Join(t.TempDir(), "research")
	if err := SeparateRuntime(root); err != nil {
		t.Fatal(err)
	}
	s, err := Init(root, StorageBIN)
	if err != nil {
		t.Fatal(err)
	}
	snapshots, err := s.ListSnapshots()
	if err != nil || len(snapshots) == 0 {
		t.Fatal(err)
	}
	n := &Node{Frontmatter: Frontmatter{Title: "after snapshot"}}
	if err = s.CreateNode(n); err != nil {
		t.Fatal(err)
	}
	if err = s.RestoreSnapshot(snapshots[0].ID); err != nil {
		t.Fatal(err)
	}
	if _, err = Open(root); err != nil {
		t.Fatal(err)
	}
	if runtimeDirectory(root) == root {
		t.Fatal("restore reverted runtime layout")
	}
	if _, err = s.GetNode(n.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("snapshot was not restored: %v", err)
	}
}

func TestInterruptedRuntimeCopyIsReconciled(t *testing.T) {
	root := filepath.Join(t.TempDir(), "research")
	s, err := Init(root, StorageBIN)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.CreateNode(&Node{Frontmatter: Frontmatter{Title: "finding"}}); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(root, ".state")
	if err = os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(dir, ".nodes.generation"), []byte("interrupted"), 0600); err != nil {
		t.Fatal(err)
	}
	if err = SeparateRuntime(root); err != nil {
		t.Fatal(err)
	}
	if _, err = Open(root); err != nil {
		t.Fatal(err)
	}
}
