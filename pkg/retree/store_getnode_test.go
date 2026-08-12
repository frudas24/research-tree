package retree

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// TestGetNodeDirectMatchesScan verifies direct node reads agree with the
// full-store scan in both storage formats.
func TestGetNodeDirectMatchesScan(t *testing.T) {
	for _, format := range []StorageFormat{StorageJSON, StorageBIN} {
		t.Run(string(format), func(t *testing.T) {
			root := t.TempDir()
			s, err := Init(filepath.Join(root, "research"), format)
			if err != nil {
				t.Fatalf("init: %v", err)
			}
			for _, title := range []string{"A", "B", "C"} {
				n := &Node{Frontmatter: Frontmatter{Title: title, Status: StatusActive, Agent: "x", Tags: []string{"t"}}}
				if err := s.CreateNode(n); err != nil {
					t.Fatalf("create %s: %v", title, err)
				}
			}
			all, err := s.QueryNodes(Filter{})
			if err != nil {
				t.Fatalf("query: %v", err)
			}
			for _, scan := range all {
				direct, err := s.GetNode(scan.ID)
				if err != nil {
					t.Fatalf("direct get %d: %v", scan.ID, err)
				}
				if direct.Title != scan.Title || direct.Agent != scan.Agent {
					t.Fatalf("direct get %d diverges: %+v vs %+v", scan.ID, direct, scan)
				}
				if len(direct.Tags) != len(scan.Tags) {
					t.Fatalf("tags diverged for %d", scan.ID)
				}
			}
		})
	}
}

// TestGetNodeMissingIsNotFound verifies a valid store returns ErrNotFound for
// an absent node in both formats.
func TestGetNodeMissingIsNotFound(t *testing.T) {
	for _, format := range []StorageFormat{StorageJSON, StorageBIN} {
		t.Run(string(format), func(t *testing.T) {
			root := t.TempDir()
			s, err := Init(filepath.Join(root, "research"), format)
			if err != nil {
				t.Fatalf("init: %v", err)
			}
			if _, err := s.GetNode(12345); !errors.Is(err, ErrNotFound) {
				t.Fatalf("want ErrNotFound, got %v", err)
			}
		})
	}
}

// TestGetNodeJSONIDMismatchAdversarial verifies a node file whose content ID
// disagrees with its filename is rejected instead of returned.
func TestGetNodeJSONIDMismatchAdversarial(t *testing.T) {
	root := t.TempDir()
	s, err := Init(filepath.Join(root, "research"), StorageJSON)
	if err != nil {
		t.Fatalf("init: %v", err)
	}
	// Craft 0007.json whose payload claims node 9.
	evil := &Node{Frontmatter: Frontmatter{ID: 9, Title: "impostor", Status: StatusActive}}
	b, err := MarshalNodeJSON(evil)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	path := filepath.Join(s.nodesDir(), "0007.json")
	if err := os.WriteFile(path, append(b, '\n'), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, err := s.GetNode(7); err == nil {
		t.Fatalf("ID mismatch must be rejected")
	}
}
