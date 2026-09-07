package cmds

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// TestCLIReindexRejectsCorruptPayload preserves the index on a failed scan.
func TestCLIReindexRejectsCorruptPayload(t *testing.T) {
	root := filepath.Join(t.TempDir(), "research")
	if _, err := runCLI(t, "--research-root", root, "init", "--storage-format", "bin"); err != nil {
		t.Fatal(err)
	}
	indexPath := filepath.Join(root, "nodes.idx")
	before, err := os.ReadFile(indexPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "nodes.bin"), []byte("invalid payload"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := runCLI(t, "--research-root", root, "storage", "reindex"); err == nil {
		t.Fatal("reindex accepted corrupt binary")
	}
	after, err := os.ReadFile(indexPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("failed recovery replaced the index")
	}
}

// TestCLIReindexWithoutUsableIndex exercises recovery through a fresh CLI open.
func TestCLIReindexWithoutUsableIndex(t *testing.T) {
	for _, corrupt := range []bool{false, true} {
		root := filepath.Join(t.TempDir(), "research")
		t.Setenv("RESEARCH_TREE_FORMAT", "bin")
		for _, args := range [][]string{{"init"}, {"node", "create", "--title", "survivor"}} {
			if _, err := runCLI(t, append([]string{"--research-root", root}, args...)...); err != nil {
				t.Fatal(err)
			}
		}
		idx := filepath.Join(root, "nodes.idx")
		if corrupt {
			if err := os.WriteFile(idx, []byte("broken"), 0o600); err != nil {
				t.Fatal(err)
			}
		} else if err := os.Remove(idx); err != nil {
			t.Fatal(err)
		}
		if _, err := runCLI(t, "--research-root", root, "storage", "reindex"); err != nil {
			t.Fatal(err)
		}
		if _, err := runCLI(t, "--research-root", root, "node", "show", "1"); err != nil {
			t.Fatal(err)
		}
	}
}
