package main

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/frudas24/research-tree/pkg/retree"
)

// TestBridgeMergeUpdate verifies that partial updates work correctly:
// sending only {id, status} should merge with the existing node, not
// overwrite other fields like title.
func TestBridgeMergeUpdate(t *testing.T) {
	root := filepath.Join(t.TempDir(), "research")
	s, err := retree.Init(root, retree.StorageJSON)
	if err != nil {
		t.Fatalf("init: %v", err)
	}

	// Create a node with full data
	n1 := &retree.Node{Frontmatter: retree.Frontmatter{
		Title:  "original title",
		Status: retree.StatusActive,
		Agent:  "test",
		Tags:   []string{"alpha", "beta"},
	}}
	n1.Body = "original body content"
	if err := s.CreateNode(n1); err != nil {
		t.Fatalf("create: %v", err)
	}

	// Simulate partial update via JSON merge (what the bridge does)
	partialJSON := `{"id":1,"status":"done","claim_status":"validated"}`

	// This is the exact logic from retree_update_node in the bridge
	var partial map[string]any
	if err := json.Unmarshal([]byte(partialJSON), &partial); err != nil {
		t.Fatal(err)
	}
	idFloat := partial["id"].(float64)
	id := retree.NodeID(idFloat)

	existing, err := s.GetNode(id)
	if err != nil {
		t.Fatal(err)
	}
	existingBytes, _ := json.Marshal(existing)
	var merged map[string]any
	json.Unmarshal(existingBytes, &merged)
	for k, v := range partial {
		merged[k] = v
	}
	mergedBytes, _ := json.Marshal(merged)

	var n retree.Node
	if err := json.Unmarshal(mergedBytes, &n); err != nil {
		t.Fatal(err)
	}
	if err := s.UpdateNode(&n); err != nil {
		t.Fatalf("update failed: %v", err)
	}

	// Verify: title and body should be preserved, status and claim changed
	got, err := s.GetNode(1)
	if err != nil {
		t.Fatal(err)
	}
	if got.Title != "original title" {
		t.Fatalf("title should be preserved, got %q", got.Title)
	}
	if got.Body != "original body content" {
		t.Fatalf("body should be preserved, got %q", got.Body)
	}
	if got.Tags == nil || got.Tags[0] != "alpha" {
		t.Fatalf("tags should be preserved, got %v", got.Tags)
	}
	if got.Status != retree.StatusDone {
		t.Fatalf("status should be updated to done, got %s", got.Status)
	}
	if got.ClaimStatus != retree.ClaimValidated {
		t.Fatalf("claim_status should be updated, got %s", got.ClaimStatus)
	}
}

func TestBridgeMergeUpdatePreservesMilestoneFields(t *testing.T) {
	root := filepath.Join(t.TempDir(), "research")
	s, err := retree.Init(root, retree.StorageJSON)
	if err != nil {
		t.Fatalf("init: %v", err)
	}

	n1 := &retree.Node{Frontmatter: retree.Frontmatter{
		Title:           "golden title",
		Status:          retree.StatusActive,
		MilestoneClass:  retree.MilestoneGolden,
		MilestoneKind:   retree.MilestoneKindChampion,
		MilestoneReason: "best current lineage artifact",
	}}
	if err := s.CreateNode(n1); err != nil {
		t.Fatalf("create: %v", err)
	}

	partialJSON := `{"id":1,"status":"done","outcome":"success"}`
	var partial map[string]any
	if err := json.Unmarshal([]byte(partialJSON), &partial); err != nil {
		t.Fatal(err)
	}
	idFloat := partial["id"].(float64)
	id := retree.NodeID(idFloat)
	existing, err := s.GetNode(id)
	if err != nil {
		t.Fatal(err)
	}
	existingBytes, _ := json.Marshal(existing)
	var merged map[string]any
	json.Unmarshal(existingBytes, &merged)
	for k, v := range partial {
		merged[k] = v
	}
	mergedBytes, _ := json.Marshal(merged)
	var n retree.Node
	if err := json.Unmarshal(mergedBytes, &n); err != nil {
		t.Fatal(err)
	}
	if err := s.UpdateNode(&n); err != nil {
		t.Fatalf("update failed: %v", err)
	}

	got, err := s.GetNode(1)
	if err != nil {
		t.Fatal(err)
	}
	if got.MilestoneClass != retree.MilestoneGolden || got.MilestoneKind != retree.MilestoneKindChampion || got.MilestoneReason != "best current lineage artifact" {
		t.Fatalf("milestone fields should be preserved, got %+v", got.Frontmatter)
	}
}

// TestBridgeCreateAndQuery verifies create + query + graph traversal
// through the Store API (the bridge wraps this).
func TestBridgeCreateAndQuery(t *testing.T) {
	root := filepath.Join(t.TempDir(), "research")
	s, err := retree.Init(root, retree.StorageJSON)
	if err != nil {
		t.Fatalf("init: %v", err)
	}

	// Create nodes
	a := &retree.Node{Frontmatter: retree.Frontmatter{Title: "root", Agent: "x", Tags: []string{"root-tag"}}}
	b := &retree.Node{Frontmatter: retree.Frontmatter{Title: "child", Parents: []retree.NodeID{1}, Agent: "x"}}
	c := &retree.Node{Frontmatter: retree.Frontmatter{Title: "grandchild", Parents: []retree.NodeID{2}}}
	s.CreateNode(a)
	s.CreateNode(b)
	s.CreateNode(c)

	// Filter by agent
	ids, _ := s.ListNodes(retree.Filter{Agent: "x"})
	if len(ids) != 2 {
		t.Fatalf("agent filter: expected 2, got %d", len(ids))
	}

	// Filter by title_contains (the bug that was fixed)
	ids, _ = s.ListNodes(retree.Filter{TitleContains: "child"})
	if len(ids) != 2 || ids[0] != 2 || ids[1] != 3 {
		t.Fatalf("title_contains filter: expected [2,3], got %v", ids)
	}

	// Graph: ancestors of grandchild
	anc, _ := s.GetAncestors(3)
	if len(anc) != 2 || anc[0] != 1 || anc[1] != 2 {
		t.Fatalf("ancestors of 3: expected [1,2], got %v", anc)
	}

	// Status query
	s.UpdateNode(&retree.Node{Frontmatter: retree.Frontmatter{ID: 2, Title: "child", Status: retree.StatusDone, ClaimStatus: retree.ClaimProvisional}})
	active, _ := s.ListNodes(retree.Filter{Status: retree.StatusActive})
	if len(active) != 2 {
		t.Fatalf("active filter: expected 2, got %d", len(active))
	}
}

// mustNoErr fails the test on unexpected error.
func mustNoErr(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
}

// TestBridgeReparenting verifies reparenting across subtrees and cycle rejection.
func TestBridgeReparenting(t *testing.T) {
	root := filepath.Join(t.TempDir(), "research")
	s, err := retree.Init(root, retree.StorageJSON)
	if err != nil {
		t.Fatalf("init: %v", err)
	}
	a := &retree.Node{Frontmatter: retree.Frontmatter{Title: "A"}}
	b := &retree.Node{Frontmatter: retree.Frontmatter{Title: "B", Parents: []retree.NodeID{1}}}
	c := &retree.Node{Frontmatter: retree.Frontmatter{Title: "C"}}
	d := &retree.Node{Frontmatter: retree.Frontmatter{Title: "D", Parents: []retree.NodeID{3}}}
	mustNoErr(t, s.CreateNode(a))
	mustNoErr(t, s.CreateNode(b))
	mustNoErr(t, s.CreateNode(c))
	mustNoErr(t, s.CreateNode(d))

	b2, _ := s.GetNode(2)
	b2.Parents = []retree.NodeID{3}
	mustNoErr(t, s.UpdateNode(b2))
	b2, _ = s.GetNode(2)
	if len(b2.Parents) != 1 || b2.Parents[0] != 3 {
		t.Fatalf("reparent failed: parents=%v", b2.Parents)
	}

	c3, _ := s.GetNode(3)
	c3.Parents = []retree.NodeID{4}
	if err := s.UpdateNode(c3); err == nil {
		t.Fatal("expected cycle error, got nil")
	}
}

// TestBridgeReparentRevision verifies that reparenting increments revision.
func TestBridgeReparentRevision(t *testing.T) {
	root := filepath.Join(t.TempDir(), "research")
	s, err := retree.Init(root, retree.StorageJSON)
	if err != nil {
		t.Fatalf("init: %v", err)
	}
	a := &retree.Node{Frontmatter: retree.Frontmatter{Title: "A"}}
	b := &retree.Node{Frontmatter: retree.Frontmatter{Title: "B", Parents: []retree.NodeID{1}}}
	mustNoErr(t, s.CreateNode(a))
	mustNoErr(t, s.CreateNode(b))

	b2, _ := s.GetNode(2)
	revBefore := b2.Revision
	b2.Parents = nil
	mustNoErr(t, s.UpdateNode(b2))
	b2, _ = s.GetNode(2)
	if b2.Revision <= revBefore {
		t.Fatalf("revision not incremented: %d → %d", revBefore, b2.Revision)
	}
}

// TestBridgeLoadTopoOrder verifies loadGraph supports lower IDs with higher-ID parents.
func TestBridgeLoadTopoOrder(t *testing.T) {
	root := filepath.Join(t.TempDir(), "research")
	s, err := retree.Init(root, retree.StorageJSON)
	if err != nil {
		t.Fatalf("init: %v", err)
	}
	// Two independent roots: A and B
	c := &retree.Node{Frontmatter: retree.Frontmatter{Title: "C"}}
	d := &retree.Node{Frontmatter: retree.Frontmatter{Title: "D"}}
	mustNoErr(t, s.CreateNode(c))
	mustNoErr(t, s.CreateNode(d))

	// Make C (ID=1) a child of D (ID=2) — lower ID depends on higher
	// This would fail before the topological loading fix
	e1, _ := s.GetNode(1)
	e1.Parents = []retree.NodeID{2}
	mustNoErr(t, s.UpdateNode(e1))

	s2, err := retree.Open(root)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	children, _ := s2.GetChildren(2)
	parents, _ := s2.GetParents(1)
	if len(parents) != 1 || parents[0] != 2 {
		t.Fatalf("parent after reload: %v", parents)
	}
	if len(children) != 1 || children[0] != 1 {
		t.Fatalf("child after reload: %v", children)
	}
}

// TestBridgeResourceLeases verifies inventory claims and auto-release through the store/bridge contract.
func TestBridgeResourceLeases(t *testing.T) {
	root := filepath.Join(t.TempDir(), "research")
	s, err := retree.Init(root, retree.StorageJSON)
	if err != nil {
		t.Fatalf("init: %v", err)
	}
	node := &retree.Node{Frontmatter: retree.Frontmatter{Title: "gpu run"}}
	mustNoErr(t, s.CreateNode(node))
	mustNoErr(t, s.CreateResource(retree.Resource{
		ID:           "gpu-node-0",
		Label:        "gpu-node-0 gpu0",
		Kind:         retree.ResourceGPU,
		Endpoint:     "10.0.0.14",
		EndpointKind: retree.EndpointIP,
		Enabled:      true,
		Capacity:     1,
	}))
	mustNoErr(t, s.ClaimResource(retree.ResourceLease{
		ResourceID: "gpu-node-0",
		NodeID:     node.ID,
		Mode:       retree.LeaseExclusive,
		ClaimedBy:  "bridge-test",
	}))
	leases, err := s.GetNodeResourceLeases(node.ID)
	if err != nil {
		t.Fatalf("leases: %v", err)
	}
	if len(leases) != 1 || leases[0].ResourceID != "gpu-node-0" {
		t.Fatalf("unexpected leases: %+v", leases)
	}
	fresh, _ := s.GetNode(node.ID)
	fresh.Status = retree.StatusDone
	fresh.Outcome = retree.OutcomeSuccess
	mustNoErr(t, s.UpdateNode(fresh))
	leases, err = s.GetNodeResourceLeases(node.ID)
	if err != nil {
		t.Fatalf("leases after close: %v", err)
	}
	if len(leases) != 0 {
		t.Fatalf("expected auto-release after done, got %+v", leases)
	}
	events, err := s.GetResourceEvents("gpu-node-0")
	if err != nil {
		t.Fatalf("events: %v", err)
	}
	if len(events) < 2 {
		t.Fatalf("expected resource events, got %+v", events)
	}
}
