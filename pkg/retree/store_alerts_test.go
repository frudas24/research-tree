package retree

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestRewriteAlertsAtomic verifies rewriteAlerts produces exact content with
// no leftover temp file, including the empty-warning case.
func TestRewriteAlertsAtomic(t *testing.T) {
	root := t.TempDir()
	s, err := Init(filepath.Join(root, "research"), StorageJSON)
	if err != nil {
		t.Fatalf("init: %v", err)
	}
	path := s.alertsPath()

	warnings := []BranchWarning{
		{ID: "w1", Agent: "a", RootCauseNode: 1, ImpactedNode: 2, Severity: "warning", Message: "m1"},
		{ID: "w2", Agent: "a", RootCauseNode: 3, ImpactedNode: 2, Severity: "warning", Message: "m2"},
	}
	if err := rewriteAlerts(path, warnings); err != nil {
		t.Fatalf("rewrite: %v", err)
	}
	got := readAllLines(t, path)
	if len(got) != 2 {
		t.Fatalf("want 2 lines, got %d: %v", len(got), got)
	}
	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Fatalf("temp file must be renamed away: %v", err)
	}

	if err := rewriteAlerts(path, nil); err != nil {
		t.Fatalf("rewrite empty: %v", err)
	}
	if got := readAllLines(t, path); len(got) != 0 {
		t.Fatalf("want 0 lines after empty rewrite, got %d", len(got))
	}
}

// TestWarningIDsUniqueAcrossInvalidations verifies that two different root
// causes invalidating the same impacted node in the same second produce
// distinct warning IDs, so acknowledging one cannot acknowledge the other.
func TestWarningIDsUniqueAcrossInvalidations(t *testing.T) {
	root := t.TempDir()
	s, err := Init(filepath.Join(root, "research"), StorageJSON)
	if err != nil {
		t.Fatalf("init: %v", err)
	}

	// A and C both parent active node B (agent "a").
	a := &Node{Frontmatter: Frontmatter{Title: "root A", Agent: "a"}}
	if err := s.CreateNode(a); err != nil {
		t.Fatalf("create A: %v", err)
	}
	c := &Node{Frontmatter: Frontmatter{Title: "root C", Agent: "a"}}
	if err := s.CreateNode(c); err != nil {
		t.Fatalf("create C: %v", err)
	}
	b := &Node{Frontmatter: Frontmatter{Title: "impacted B", Agent: "a", Parents: []NodeID{a.ID, c.ID}}}
	if err := s.CreateNode(b); err != nil {
		t.Fatalf("create B: %v", err)
	}

	r1 := &Node{Frontmatter: Frontmatter{Title: "refuter 1"}}
	if err := s.CreateNode(r1); err != nil {
		t.Fatalf("create R1: %v", err)
	}
	r2 := &Node{Frontmatter: Frontmatter{Title: "refuter 2"}}
	if err := s.CreateNode(r2); err != nil {
		t.Fatalf("create R2: %v", err)
	}

	// Two invalidations in the same second (nanosecond IDs guarantee uniqueness).
	if err := s.InvalidateClaim(a.ID, r1.ID, "broken A"); err != nil {
		t.Fatalf("invalidate A: %v", err)
	}
	if err := s.InvalidateClaim(c.ID, r2.ID, "broken C"); err != nil {
		t.Fatalf("invalidate C: %v", err)
	}

	warnings, err := s.ListBranchWarnings("a", true)
	if err != nil {
		t.Fatalf("list warnings: %v", err)
	}
	if len(warnings) != 2 {
		t.Fatalf("want 2 warnings, got %d: %+v", len(warnings), warnings)
	}
	if warnings[0].ID == warnings[1].ID {
		t.Fatalf("warning IDs must be unique, both %q", warnings[0].ID)
	}

	// Adversarial: acking one must leave the other unacknowledged.
	if err := s.AckBranchWarning(warnings[0].ID); err != nil {
		t.Fatalf("ack: %v", err)
	}
	remaining, err := s.ListBranchWarnings("a", true)
	if err != nil {
		t.Fatalf("list remaining: %v", err)
	}
	if len(remaining) != 1 {
		t.Fatalf("want 1 unacked warning, got %d", len(remaining))
	}
	if remaining[0].ID != warnings[1].ID {
		t.Fatalf("wrong warning survived ack: %q vs %q", remaining[0].ID, warnings[1].ID)
	}
}

func readAllLines(t *testing.T, path string) []string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var out []string
	for _, line := range strings.Split(strings.TrimRight(string(b), "\n"), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		out = append(out, line)
	}
	return out
}
