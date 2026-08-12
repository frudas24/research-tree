package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/frudas24/research-tree/pkg/retree"
)

// newTestStore initializes a JSON store in a temp dir and returns it with
// its root path so tests can break the store's on-disk state.
func newTestStore(t *testing.T) (*retree.Store, string) {
	t.Helper()
	root := t.TempDir()
	path := filepath.Join(root, "research")
	s, err := retree.Init(path, retree.StorageJSON)
	if err != nil {
		t.Fatalf("init: %v", err)
	}
	return s, path
}

// createTestGraph seeds parent/child/done nodes for handler tests.
func createTestGraph(t *testing.T, s *retree.Store) {
	t.Helper()
	parent := &retree.Node{Frontmatter: retree.Frontmatter{Title: "parent", Status: retree.StatusActive}}
	if err := s.CreateNode(parent); err != nil {
		t.Fatalf("create parent: %v", err)
	}
	child := &retree.Node{Frontmatter: retree.Frontmatter{Title: "child", Status: retree.StatusActive, Parents: []retree.NodeID{parent.ID}}}
	if err := s.CreateNode(child); err != nil {
		t.Fatalf("create child: %v", err)
	}
	done := &retree.Node{Frontmatter: retree.Frontmatter{Title: "done", Status: retree.StatusDone, Outcome: retree.OutcomeSuccess, Parents: []retree.NodeID{parent.ID}}}
	if err := s.CreateNode(done); err != nil {
		t.Fatalf("create done: %v", err)
	}
}

// TestGraphHandlerReturnsPayload verifies /graph serves the DAG projection.
func TestGraphHandlerReturnsPayload(t *testing.T) {
	s, _ := newTestStore(t)
	createTestGraph(t, s)

	srv := httptest.NewServer(newMux(s))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/graph")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if got := resp.Header.Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("unexpected CORS header on /graph: %q", got)
	}
	var payload GraphPayload
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if payload.Total != 3 {
		t.Fatalf("total = %d, want 3", payload.Total)
	}
	if len(payload.Edges) != 2 {
		t.Fatalf("edges = %d, want 2", len(payload.Edges))
	}
	var parentNode *GraphNode
	for i := range payload.Nodes {
		if payload.Nodes[i].Title == "parent" {
			parentNode = &payload.Nodes[i]
		}
	}
	if parentNode == nil {
		t.Fatalf("parent node missing from payload")
	}
	if parentNode.PendingChildren != 1 {
		t.Fatalf("pending children = %d, want 1 (done child excluded)", parentNode.PendingChildren)
	}
}

// TestNodeHandlerReturnsDetail verifies /node serves full detail.
func TestNodeHandlerReturnsDetail(t *testing.T) {
	s, _ := newTestStore(t)
	createTestGraph(t, s)
	nodes, err := s.QueryNodes(retree.Filter{TitleContains: "parent"})
	if err != nil || len(nodes) != 1 {
		t.Fatalf("query parent: %v nodes=%d", err, len(nodes))
	}

	srv := httptest.NewServer(newMux(s))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/node?id=" + itoa(nodes[0].ID))
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if got := resp.Header.Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("unexpected CORS header on /node: %q", got)
	}
	var detail NodeDetail
	if err := json.NewDecoder(resp.Body).Decode(&detail); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if detail.Title != "parent" {
		t.Fatalf("title = %q, want parent", detail.Title)
	}
	if len(detail.Children) != 2 {
		t.Fatalf("children = %d, want 2", len(detail.Children))
	}
}

// TestGraphHandlerSurfacesStoreErrors verifies storage failures are reported
// as 500 instead of a silent empty graph.
func TestGraphHandlerSurfacesStoreErrors(t *testing.T) {
	s, path := newTestStore(t)
	createTestGraph(t, s)
	// Break the store: JSON mode reads nodes from nodes/.
	if err := os.RemoveAll(filepath.Join(path, "nodes")); err != nil {
		t.Fatalf("remove nodes dir: %v", err)
	}

	srv := httptest.NewServer(newMux(s))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/graph")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500 (storage error must not render as empty graph)", resp.StatusCode)
	}
}

// TestNodeHandlerDistinguishesNotFoundFromStorageError verifies a broken
// store returns 500 while a truly missing node returns 404.
func TestNodeHandlerDistinguishesNotFoundFromStorageError(t *testing.T) {
	s, path := newTestStore(t)
	createTestGraph(t, s)

	srv := httptest.NewServer(newMux(s))
	defer srv.Close()

	// Missing node id -> 404.
	resp, err := http.Get(srv.URL + "/node?id=9999")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("missing node status = %d, want 404", resp.StatusCode)
	}

	// Broken store -> 500, not 404.
	if err := os.RemoveAll(filepath.Join(path, "nodes")); err != nil {
		t.Fatalf("remove nodes dir: %v", err)
	}
	resp, err = http.Get(srv.URL + "/node?id=1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("broken store status = %d, want 500", resp.StatusCode)
	}
}

// TestComputeHotnessParity verifies the graph server hotness matches the
// shared formula for the same inputs.
func TestComputeHotnessParity(t *testing.T) {
	for _, tt := range []struct {
		pending, age, bonus, want int
	}{
		{0, 0, 0, 0},
		{2, 3, 0, 2*retree.HotspotPendingChildWeight + 3},
		{1, 10, retree.HotspotInconclusiveOutcomeBonus, retree.HotspotPendingChildWeight + 10 + retree.HotspotInconclusiveOutcomeBonus},
	} {
		got := retree.ComputeHotness(tt.pending, tt.age, tt.bonus)
		if got != tt.want {
			t.Fatalf("ComputeHotness(%d,%d,%d) = %d, want %d", tt.pending, tt.age, tt.bonus, got, tt.want)
		}
	}
}

// itoa renders a NodeID as a decimal string.
func itoa(id retree.NodeID) string {
	return fmt.Sprintf("%d", id)
}
