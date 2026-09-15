package retree

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestSeparateRuntimePreservesHistoryAndRejectsOldHandle verifies that
// separation is idempotent, keeps research history readable through a freshly
// opened handle, and invalidates handles opened before the conversion.
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

// TestSeparatedRuntimeSurvivesSnapshotRestore verifies that restoring a
// snapshot captured before separation does not revert the runtime layout.
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

// TestInterruptedRuntimeCopyIsReconciled verifies that a partially written
// .state directory is repaired by a repeated SeparateRuntime call: sidecars the
// legacy root still holds are copied verbatim, and sidecars without a legacy
// source are dropped instead of surviving as stale runtime state.
func TestInterruptedRuntimeCopyIsReconciled(t *testing.T) {
	root := filepath.Join(t.TempDir(), "research")
	s, err := Init(root, StorageBIN)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.CreateNode(&Node{Frontmatter: Frontmatter{Title: "finding"}}); err != nil {
		t.Fatal(err)
	}
	legacy := make(map[string][]byte, len(runtimeSidecars))
	for _, name := range runtimeSidecars {
		b, readErr := os.ReadFile(filepath.Join(root, name))
		if readErr != nil && !os.IsNotExist(readErr) {
			t.Fatal(readErr)
		}
		legacy[name] = b
	}
	if len(legacy[".nodes.generation"]) == 0 {
		t.Fatal("expected the BIN publication to publish a legacy generation sidecar")
	}
	// Force the delete branch to be exercised: a freshly published store has no
	// pending derived-index marker, so .derived.dirty has no legacy source.
	if err = os.Remove(filepath.Join(root, ".derived.dirty")); err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	legacy[".derived.dirty"] = nil
	dir := filepath.Join(root, ".state")
	if err = os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	// Simulate the interrupted copy: every staged sidecar holds unusable content,
	// so the repeated conversion must overwrite the ones with a legacy source and
	// delete the others.
	for _, name := range runtimeSidecars {
		if err = os.WriteFile(filepath.Join(dir, name), []byte("interrupted"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err = SeparateRuntime(root); err != nil {
		t.Fatal(err)
	}
	for _, name := range runtimeSidecars {
		got, readErr := os.ReadFile(filepath.Join(dir, name))
		if legacy[name] == nil {
			if !os.IsNotExist(readErr) {
				t.Fatalf("%s: stale sidecar survived without a legacy source: %q %v", name, got, readErr)
			}
			continue
		}
		if readErr != nil {
			t.Fatalf("%s: %v", name, readErr)
		}
		if !bytes.Equal(got, legacy[name]) {
			t.Fatalf("%s: copied %q, want %q", name, got, legacy[name])
		}
	}
	if _, err = os.Stat(filepath.Join(dir, "ready")); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	wantGeneration := strings.TrimSpace(string(legacy[".nodes.generation"]))
	if got, genErr := reopened.readBinaryGeneration(); genErr != nil || got != wantGeneration {
		t.Fatalf("reopened generation = %q, %v; want %q", got, genErr, wantGeneration)
	}
}
