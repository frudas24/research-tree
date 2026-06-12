package retree

import (
	"errors"
	"reflect"
	"testing"
	"time"
)

// mkNode creates a test node with the given id, title, and optional parents.
func mkNode(id NodeID, title string, parents ...NodeID) *Node {
	n := &Node{Frontmatter: Frontmatter{ID: id, Title: title, Parents: parents}}
	ApplyNodeDefaults(n, time.Date(2026, 5, 31, 10, 0, 0, 0, time.UTC))
	return n
}

// TestGraphAddGetRoundtrip verifies basic add/get roundtrip.
func TestGraphAddGetRoundtrip(t *testing.T) {
	g := NewGraph()
	n1 := mkNode(1, "root")
	if err := g.AddNode(n1); err != nil {
		t.Fatalf("add root: %v", err)
	}
	n2 := mkNode(2, "child", 1)
	if err := g.AddNode(n2); err != nil {
		t.Fatalf("add child: %v", err)
	}
	got, err := g.GetNode(2)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Title != "child" || !reflect.DeepEqual(got.Parents, []NodeID{1}) {
		t.Fatalf("unexpected node: %+v", got)
	}
}

// TestGraphRejectsMissingParent verifies adding a node with a missing parent fails.
func TestGraphRejectsMissingParent(t *testing.T) {
	g := NewGraph()
	err := g.AddNode(mkNode(2, "x", 99))
	if !errors.Is(err, ErrInvalidNode) {
		t.Fatalf("expected ErrInvalidNode, got %v", err)
	}
}

// TestGraphDetectCycleOnUpdateAtoBtoA verifies cycle detection on update.
func TestGraphDetectCycleOnUpdateAtoBtoA(t *testing.T) {
	g := NewGraph()
	mustAdd(t, g, mkNode(1, "a"))
	mustAdd(t, g, mkNode(2, "b", 1))
	n1 := mkNode(1, "a", 2)
	err := g.UpdateNode(1, n1)
	if !errors.Is(err, ErrCycleDetected) {
		t.Fatalf("expected cycle error, got %v", err)
	}
}

// TestGraphAncestorsAndDescendants verifies ancestor and descendant traversal.
func TestGraphAncestorsAndDescendants(t *testing.T) {
	g := NewGraph()
	mustAdd(t, g, mkNode(1, "n1"))
	mustAdd(t, g, mkNode(2, "n2", 1))
	mustAdd(t, g, mkNode(3, "n3", 2))
	mustAdd(t, g, mkNode(4, "n4", 1))

	if got := g.GetAncestors(3); !reflect.DeepEqual(got, []NodeID{1, 2}) {
		t.Fatalf("ancestors mismatch: %v", got)
	}
	if got := g.GetDescendants(1); !reflect.DeepEqual(got, []NodeID{2, 3, 4}) {
		t.Fatalf("descendants mismatch: %v", got)
	}
}

// TestGraphRoots verifies root node detection.
func TestGraphRoots(t *testing.T) {
	g := NewGraph()
	mustAdd(t, g, mkNode(1, "n1"))
	mustAdd(t, g, mkNode(2, "n2"))
	mustAdd(t, g, mkNode(3, "n3", 1))
	if got := g.GetRoots(); !reflect.DeepEqual(got, []NodeID{1, 2}) {
		t.Fatalf("roots mismatch: %v", got)
	}
}

// TestGraphListByStatusAndClaimStatus verifies filtering by status and claim status.
func TestGraphListByStatusAndClaimStatus(t *testing.T) {
	g := NewGraph()
	n1 := mkNode(1, "n1")
	n1.Status = StatusActive
	n1.ClaimStatus = ClaimProvisional
	mustAdd(t, g, n1)

	n2 := mkNode(2, "n2")
	n2.Status = StatusDone
	n2.ClaimStatus = ClaimValidated
	mustAdd(t, g, n2)

	if got := g.ListByStatus(StatusActive); !reflect.DeepEqual(got, []NodeID{1}) {
		t.Fatalf("status mismatch: %v", got)
	}
	if got := g.ListByClaimStatus(ClaimValidated); !reflect.DeepEqual(got, []NodeID{2}) {
		t.Fatalf("claim status mismatch: %v", got)
	}
}

// TestGraphRemoveNodeWithAndWithoutForce verifies normal and force node removal.
func TestGraphRemoveNodeWithAndWithoutForce(t *testing.T) {
	g := NewGraph()
	mustAdd(t, g, mkNode(1, "n1"))
	mustAdd(t, g, mkNode(2, "n2", 1))
	if err := g.RemoveNode(1, false); !errors.Is(err, ErrHasChildren) {
		t.Fatalf("expected ErrHasChildren, got %v", err)
	}
	if err := g.RemoveNode(1, true); err != nil {
		t.Fatalf("force remove: %v", err)
	}
	if parents := g.GetParents(2); len(parents) != 0 {
		t.Fatalf("expected orphan child, got parents=%v", parents)
	}
}

// TestGraphAtRiskDescendants verifies at-risk descendant detection.
func TestGraphAtRiskDescendants(t *testing.T) {
	g := NewGraph()
	mustAdd(t, g, mkNode(1, "n1"))
	n2 := mkNode(2, "n2", 1)
	n2.Status = StatusActive
	mustAdd(t, g, n2)
	n3 := mkNode(3, "n3", 2)
	n3.Status = StatusDone
	mustAdd(t, g, n3)
	if got := g.GetAtRiskDescendants(1); !reflect.DeepEqual(got, []NodeID{2}) {
		t.Fatalf("at-risk mismatch: %v", got)
	}
}

// mustAdd adds a node to the graph, failing the test on error.
func mustAdd(t *testing.T, g *Graph, n *Node) {
	t.Helper()
	if err := g.AddNode(n); err != nil {
		t.Fatalf("add node %d: %v", n.ID, err)
	}
}
