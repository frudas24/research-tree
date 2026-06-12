package retree

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// TestABIOpenLoadsExistingGraphAndQueries verifies the public ABI for open, graph queries, and filters.
func TestABIOpenLoadsExistingGraphAndQueries(t *testing.T) {
	root := filepath.Join(t.TempDir(), "research")
	s, err := Init(root, StorageJSON)
	mustNoErr(t, err)
	a := &Node{Frontmatter: Frontmatter{Title: "a", Status: StatusDone}}
	mustNoErr(t, s.CreateNode(a))
	b := &Node{Frontmatter: Frontmatter{
		Title:           "b",
		Status:          StatusDone,
		Parents:         []NodeID{a.ID},
		MilestoneClass:  MilestoneGolden,
		MilestoneKind:   MilestoneKindPivot,
		MilestoneReason: "moved the bottleneck elsewhere",
	}}
	mustNoErr(t, s.CreateNode(b))

	s2, err := Open(root)
	mustNoErr(t, err)
	children, err := s2.GetChildren(a.ID)
	mustNoErr(t, err)
	if !reflect.DeepEqual(children, []NodeID{b.ID}) {
		t.Fatalf("children mismatch: %v", children)
	}
	anc, err := s2.GetAncestors(b.ID)
	mustNoErr(t, err)
	if !reflect.DeepEqual(anc, []NodeID{a.ID}) {
		t.Fatalf("ancestors mismatch: %v", anc)
	}
	roots, err := s2.GetRoots()
	mustNoErr(t, err)
	if !reflect.DeepEqual(roots, []NodeID{a.ID}) {
		t.Fatalf("roots mismatch: %v", roots)
	}
	failed, err := s2.ListNodes(Filter{TitleContains: "b"})
	mustNoErr(t, err)
	if !reflect.DeepEqual(failed, []NodeID{b.ID}) {
		t.Fatalf("failed filter mismatch: %v", failed)
	}
	q, err := s2.QueryNodes(Filter{TitleContains: "b"})
	mustNoErr(t, err)
	if len(q) != 1 || q[0].ID != b.ID {
		t.Fatalf("query mismatch: %+v", q)
	}
	if q[0].MilestoneClass != MilestoneGolden || q[0].MilestoneKind != MilestoneKindPivot {
		t.Fatalf("milestone fields not preserved through ABI query path: %+v", q[0])
	}
}

// TestABITagsAndEmbedArtifact verifies the public ABI for tags and embed artifact.
func TestABITagsAndEmbedArtifact(t *testing.T) {
	s := mustInit(t, StorageJSON)
	n := &Node{Frontmatter: Frontmatter{Title: "n"}}
	mustNoErr(t, s.CreateNode(n))

	mustNoErr(t, s.AddTags(n.ID, "x", "x", "y"))
	mustNoErr(t, s.AddTags(n.ID, "y"))
	got, err := s.GetNode(n.ID)
	mustNoErr(t, err)
	if !reflect.DeepEqual(got.Tags, []string{"x", "y"}) {
		t.Fatalf("tag add idempotency mismatch: %v", got.Tags)
	}

	mustNoErr(t, s.RemoveTags(n.ID, "x", "x"))
	got, err = s.GetNode(n.ID)
	mustNoErr(t, err)
	if !reflect.DeepEqual(got.Tags, []string{"y"}) {
		t.Fatalf("tag remove idempotency mismatch: %v", got.Tags)
	}

	local := filepath.Join(t.TempDir(), "metrics.json")
	mustNoErr(t, os.WriteFile(local, []byte(`{"ok":true}`), 0o644))
	mustNoErr(t, s.EmbedArtifact(n.ID, local, "metrics"))
	got, err = s.GetNode(n.ID)
	mustNoErr(t, err)
	if len(got.Artifacts) != 1 || got.Artifacts[0].Mode != ArtifactEmbedded {
		t.Fatalf("embed artifact mismatch: %+v", got.Artifacts)
	}
	diskPath := filepath.Join(s.rootPath, filepath.FromSlash(got.Artifacts[0].Path))
	if _, err := os.Stat(diskPath); err != nil {
		t.Fatalf("embedded artifact missing on disk: %v", err)
	}
}
