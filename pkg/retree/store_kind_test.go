package retree

import (
	"errors"
	"path/filepath"
	"testing"
)

// kindStore initializes a fresh JSON store for kind tests.
func kindStore(t *testing.T) *Store {
	t.Helper()
	s, err := Init(filepath.Join(t.TempDir(), "research"), StorageJSON)
	if err != nil {
		t.Fatalf("init: %v", err)
	}
	return s
}

// mustCreate creates a node, failing the test on error, and returns its ID.
func mustCreate(t *testing.T, s *Store, n *Node) NodeID {
	t.Helper()
	if err := s.CreateNode(n); err != nil {
		t.Fatalf("create %q: %v", n.Title, err)
	}
	return n.ID
}

// TestFilterKindSeparatesWorkAndUmbrella verifies kind filtering, including
// legacy nodes (no kind) matching the work filter.
func TestFilterKindSeparatesWorkAndUmbrella(t *testing.T) {
	s := kindStore(t)
	work := mustCreate(t, s, &Node{Frontmatter: Frontmatter{Title: "w", Status: StatusActive}})
	umb := mustCreate(t, s, &Node{Frontmatter: Frontmatter{Title: "u", Kind: NodeKindUmbrella, Status: StatusActive}})
	// Legacy-style node persisted without a kind (defaults normalize to work).
	legacy := mustCreate(t, s, &Node{Frontmatter: Frontmatter{Title: "legacy", Status: StatusActive}})

	works, err := s.QueryNodes(Filter{Kind: NodeKindWork})
	if err != nil {
		t.Fatalf("query work: %v", err)
	}
	if len(works) != 2 {
		t.Fatalf("want 2 work nodes (incl. legacy), got %d", len(works))
	}
	seen := map[NodeID]bool{}
	for _, n := range works {
		seen[n.ID] = true
	}
	if !seen[work] || !seen[legacy] {
		t.Fatalf("work filter must include legacy node: %v", seen)
	}
	if seen[umb] {
		t.Fatalf("work filter must exclude umbrella")
	}

	umbrellas, err := s.QueryNodes(Filter{Kind: NodeKindUmbrella})
	if err != nil {
		t.Fatalf("query umbrella: %v", err)
	}
	if len(umbrellas) != 1 || umbrellas[0].ID != umb {
		t.Fatalf("want exactly the umbrella, got %+v", umbrellas)
	}
}

// TestQueryNodesRejectsInvalidKindFilter verifies core callers do not silently
// interpret an invalid kind filter as an empty result set.
func TestQueryNodesRejectsInvalidKindFilter(t *testing.T) {
	s := kindStore(t)
	mustCreate(t, s, &Node{Frontmatter: Frontmatter{Title: "w", Status: StatusActive}})
	if _, err := s.QueryNodes(Filter{Kind: NodeKind("banana")}); !errors.Is(err, ErrInvalidNode) {
		t.Fatalf("expected ErrInvalidNode for invalid kind filter, got %v", err)
	}
}

// TestDerivedProgressNestedAndMultiParent verifies umbrella progress counts
// every child once even with multiple parents and recurses into nested umbrellas.
func TestDerivedProgressNestedAndMultiParent(t *testing.T) {
	s := kindStore(t)
	root := mustCreate(t, s, &Node{Frontmatter: Frontmatter{Title: "umbrella", Kind: NodeKindUmbrella, Status: StatusActive}})
	nested := mustCreate(t, s, &Node{Frontmatter: Frontmatter{Title: "nested umbrella", Kind: NodeKindUmbrella, Status: StatusActive, Parents: []NodeID{root}}})
	// A work child with TWO parents (root + nested) — counted once in root's set.
	shared := mustCreate(t, s, &Node{Frontmatter: Frontmatter{Title: "shared work", Status: StatusActive, Parents: []NodeID{root, nested}}})
	doneWork := mustCreate(t, s, &Node{Frontmatter: Frontmatter{Title: "done work", Status: StatusDone, Outcome: OutcomeSuccess, Parents: []NodeID{root}}})
	pausedWork := mustCreate(t, s, &Node{Frontmatter: Frontmatter{Title: "paused work", Status: StatusPaused, Parents: []NodeID{nested}}})
	// Actionable leaf under shared work.
	leaf := mustCreate(t, s, &Node{Frontmatter: Frontmatter{Title: "leaf", Status: StatusActive, Parents: []NodeID{shared}}})
	// Done leaf (not actionable).
	mustCreate(t, s, &Node{Frontmatter: Frontmatter{Title: "done leaf", Status: StatusDone, Outcome: OutcomeFailure, Parents: []NodeID{shared}}})

	prog, err := s.DerivedProgress(root)
	if err != nil {
		t.Fatalf("derived progress: %v", err)
	}
	if prog.DirectChildren != 3 {
		t.Fatalf("direct children = %d, want 3 (nested, shared, doneWork; pausedWork is nested's child)", prog.DirectChildren)
	}
	if prog.WorkChildren != 2 || prog.UmbrellaChildren != 1 {
		t.Fatalf("work=%d umbrella=%d, want 2/1", prog.WorkChildren, prog.UmbrellaChildren)
	}
	// Status counts cover ALL direct children by status (umbrellas included):
	// nested(active) + shared(active) + doneWork(done).
	if prog.Active != 2 || prog.Done != 1 || prog.Paused != 0 {
		t.Fatalf("active=%d done=%d paused=%d, want 2/1/0", prog.Active, prog.Done, prog.Paused)
	}
	// Actionable leaves: leaf (active) only; pausedWork is a leaf but paused;
	// doneWork and done leaf are resolved.
	if len(prog.ActionableLeaves) != 1 || prog.ActionableLeaves[0] != leaf {
		t.Fatalf("actionable leaves = %v, want [%d]", prog.ActionableLeaves, leaf)
	}
	for _, id := range prog.ActionableLeaves {
		if id == doneWork || id == pausedWork {
			t.Fatalf("resolved/deferred node %d must never be actionable", id)
		}
	}
}

// TestDerivedProgressRejectsWorkNode verifies the guard on non-umbrellas.
func TestDerivedProgressRejectsWorkNode(t *testing.T) {
	s := kindStore(t)
	w := mustCreate(t, s, &Node{Frontmatter: Frontmatter{Title: "w", Status: StatusActive}})
	if _, err := s.DerivedProgress(w); err == nil {
		t.Fatalf("DerivedProgress must reject a work node")
	}
	if _, err := s.DerivedProgress(9999); err != ErrNotFound {
		t.Fatalf("want ErrNotFound for missing node, got %v", err)
	}
}

// TestStatusSummarySeparatesUmbrellas verifies the legacy arrays still contain
// all nodes while work-only metrics and hotspots stay separated.
func TestStatusSummarySeparatesUmbrellas(t *testing.T) {
	s := kindStore(t)
	work := mustCreate(t, s, &Node{Frontmatter: Frontmatter{Title: "w", Status: StatusActive, Agent: "a"}})
	umb := mustCreate(t, s, &Node{Frontmatter: Frontmatter{Title: "u", Kind: NodeKindUmbrella, Status: StatusActive, Agent: "a"}})
	child := mustCreate(t, s, &Node{Frontmatter: Frontmatter{Title: "c", Status: StatusActive, Parents: []NodeID{umb}}})

	all, err := s.QueryNodes(Filter{})
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	sum := BuildStatusSummary(all, nil, StatusBuildOptions{Now: nowUTC()})

	if sum.Total != 3 {
		t.Fatalf("total = %d, want 3", sum.Total)
	}
	// Legacy active remains compatible and includes every active node.
	if len(sum.Active) != 3 {
		t.Fatalf("legacy active = %+v, want all 3 active nodes", sum.Active)
	}
	if len(sum.UmbrellaActive) != 1 || sum.UmbrellaActive[0].ID != umb {
		t.Fatalf("umbrella active = %+v, want %d", sum.UmbrellaActive, umb)
	}
	if sum.WorkStatusCounts[StatusActive] != 2 || sum.UmbrellaStatusCounts[StatusActive] != 1 {
		t.Fatalf("counts work=%v umbrella=%v", sum.WorkStatusCounts, sum.UmbrellaStatusCounts)
	}
	for _, h := range sum.Hotspots {
		if h.ID == umb {
			t.Fatalf("umbrella must never appear in work hotspots: %+v", h)
		}
	}
	if len(sum.UmbrellaPressure) != 1 {
		t.Fatalf("umbrella pressure = %+v, want 1 entry", sum.UmbrellaPressure)
	}
	pressure := sum.UmbrellaPressure[0]
	if pressure.ID != umb || pressure.Active != 1 || pressure.Paused != 0 || pressure.Unresolved != 1 {
		t.Fatalf("pressure = %+v, want active=1 unresolved=1", pressure)
	}
	if work == umb || child == umb || child == work {
		t.Fatalf("node ids collided")
	}
}

// TestDerivedProgressMultiParentCountedOnce verifies a child with two umbrella
// parents is counted once per umbrella.
func TestDerivedProgressMultiParentCountedOnce(t *testing.T) {
	s := kindStore(t)
	u1 := mustCreate(t, s, &Node{Frontmatter: Frontmatter{Title: "u1", Kind: NodeKindUmbrella, Status: StatusActive}})
	u2 := mustCreate(t, s, &Node{Frontmatter: Frontmatter{Title: "u2", Kind: NodeKindUmbrella, Status: StatusActive}})
	shared := mustCreate(t, s, &Node{Frontmatter: Frontmatter{Title: "shared", Status: StatusActive, Parents: []NodeID{u1, u2}}})

	p1, err := s.DerivedProgress(u1)
	if err != nil {
		t.Fatalf("u1: %v", err)
	}
	p2, err := s.DerivedProgress(u2)
	if err != nil {
		t.Fatalf("u2: %v", err)
	}
	if p1.DirectChildren != 1 || p1.WorkChildren != 1 {
		t.Fatalf("u1 progress = %+v, want 1 direct work child", p1)
	}
	if p2.DirectChildren != 1 || p2.WorkChildren != 1 {
		t.Fatalf("u2 progress = %+v, want 1 direct work child", p2)
	}
	if len(p1.ActionableLeaves) != 1 || p1.ActionableLeaves[0] != shared {
		t.Fatalf("u1 actionable leaves = %v, want [%d]", p1.ActionableLeaves, shared)
	}
}
