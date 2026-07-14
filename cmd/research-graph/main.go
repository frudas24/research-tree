// Command research-graph serves a real-time DAG visualization for any research-tree root.
// It serves the Cytoscape UI and a /graph JSON endpoint from a .research directory.
//
// Usage:
//
//	research-graph --research-root ~/cardinal/.research --port 8080
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/frudas24/research-tree/pkg/retree"
)

func main() {
	researchRoot := flag.String("research-root", defaultResearchRoot(), "Path to .research directory")
	port := flag.Int("port", 8080, "HTTP port")
	flag.Parse()

	store, err := retree.Open(*researchRoot)
	if err != nil {
		log.Fatalf("open research root: %v", err)
	}

	uiDir := resolveUIDir()

	mux := http.NewServeMux()

	// Static UI files
	fs := http.FileServer(http.Dir(uiDir))
	mux.Handle("/assets/", fs)
	mux.Handle("/cytoscape/", fs) // if local cytoscape is vendored

	// /graph endpoint: full DAG projection
	mux.HandleFunc("/graph", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Access-Control-Allow-Origin", "*")

		payload := buildGraphPayload(store)
		if err := json.NewEncoder(w).Encode(payload); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	})

	// Serve index.html at root
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" || r.URL.Path == "/index.html" {
			http.ServeFile(w, r, filepath.Join(uiDir, "index.html"))
			return
		}
		http.NotFound(w, r)
	})

	addr := fmt.Sprintf(":%d", *port)
	log.Printf("research-graph serving %s on http://localhost%s", *researchRoot, addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatalf("serve: %v", err)
	}
}

// GraphNode is a lightweight DTO for the UI.
type GraphNode struct {
	ID              uint64   `json:"id"`
	Title           string   `json:"title"`
	Status          string   `json:"status"`
	ClaimStatus     string   `json:"claim_status"`
	EvidenceStatus  string   `json:"evidence_status"`
	Outcome         string   `json:"outcome"`
	MilestoneClass  string   `json:"milestone_class"`
	MilestoneKind   string   `json:"milestone_kind"`
	Agent           string   `json:"agent"`
	Children        []uint64 `json:"children"`
	PendingChildren int      `json:"pending_children"`
	Hotness         int      `json:"hotness,omitempty"`
	Tags            []string `json:"tags,omitempty"`
	Scope           string   `json:"scope,omitempty"`
}

// GraphEdge is a parent→child DAG edge.
type GraphEdge struct {
	From uint64 `json:"from"`
	To   uint64 `json:"to"`
}

// GraphRelation is a typed cross-edge.
type GraphRelation struct {
	From   uint64 `json:"from"`
	Target uint64 `json:"target"`
	Type   string `json:"type"`
	Note   string `json:"note,omitempty"`
}

// GraphPayload is the full graph response.
type GraphPayload struct {
	Nodes     []GraphNode     `json:"nodes"`
	Edges     []GraphEdge     `json:"edges"`
	Relations []GraphRelation `json:"relations"`
	Total     int             `json:"total"`
}

func buildGraphPayload(store *retree.Store) GraphPayload {
	nodes, _ := store.QueryNodes(retree.Filter{SortBy: "id", Order: "asc"})

	// Count children per node and pending children
	childrenByParent := map[retree.NodeID][]retree.NodeID{}
	for _, n := range nodes {
		for _, pid := range n.Parents {
			childrenByParent[pid] = append(childrenByParent[pid], n.ID)
		}
	}

	now := time.Now().UTC()
	graphNodes := make([]GraphNode, 0, len(nodes))
	graphEdges := make([]GraphEdge, 0)
	graphRelations := make([]GraphRelation, 0)

	for _, n := range nodes {
		children := childrenByParent[n.ID]
		pending := countPending(nodes, children)

		// Hotspot formula from pkg/retree/status_summary.go: hotness = pending*5 + age_days + inconclusive_bonus
		hotness := pending * 5
		if !n.Created.IsZero() {
			ageDays := int(now.Sub(n.Created).Hours() / 24)
			hotness += ageDays
		}
		if n.Outcome == retree.OutcomeInconclusive {
			hotness += 5
		}

		childrenU64 := make([]uint64, len(children))
		for i, c := range children {
			childrenU64[i] = uint64(c)
		}

		gn := GraphNode{
			ID:              uint64(n.ID),
			Title:           n.Title,
			Status:          string(n.Status),
			ClaimStatus:     string(n.ClaimStatus),
			EvidenceStatus:  string(n.EvidenceStatus),
			Outcome:         string(n.Outcome),
			MilestoneClass:  string(n.MilestoneClass),
			MilestoneKind:   string(n.MilestoneKind),
			Agent:           n.Agent,
			Children:        childrenU64,
			PendingChildren: pending,
			Hotness:         hotness,
			Tags:            n.Tags,
			Scope:           n.Scope,
		}
		graphNodes = append(graphNodes, gn)

		// Parent edges
		for _, pid := range n.Parents {
			graphEdges = append(graphEdges, GraphEdge{From: uint64(pid), To: uint64(n.ID)})
		}

		// Relation edges
		for _, rel := range n.Relations {
			graphRelations = append(graphRelations, GraphRelation{
				From:   uint64(n.ID),
				Target: uint64(rel.Target),
				Type:   string(rel.Type),
				Note:   rel.Note,
			})
		}
	}

	return GraphPayload{
		Nodes:     graphNodes,
		Edges:     graphEdges,
		Relations: graphRelations,
		Total:     len(nodes),
	}
}

func countPending(nodes []*retree.Node, children []retree.NodeID) int {
	statusByID := make(map[retree.NodeID]retree.NodeStatus, len(nodes))
	for _, n := range nodes {
		statusByID[n.ID] = n.Status
	}
	count := 0
	for _, cid := range children {
		if s, ok := statusByID[cid]; ok && s != retree.StatusDone {
			count++
		}
	}
	return count
}

func defaultResearchRoot() string {
	if explicit := strings.TrimSpace(os.Getenv("RESEARCH_ROOT")); explicit != "" {
		return explicit
	}
	if base := strings.TrimSpace(os.Getenv("RESEARCH_TREE_ROOT")); base != "" {
		return filepath.Join(base, ".research")
	}
	return ".research"
}

// resolveUIDir finds the ui/ directory relative to the binary or source tree.
func resolveUIDir() string {
	// Try relative to cwd first
	if _, err := os.Stat("ui/index.html"); err == nil {
		return "ui"
	}
	// Try relative to executable
	if exe, err := os.Executable(); err == nil {
		cand := filepath.Join(filepath.Dir(exe), "ui")
		if _, err := os.Stat(filepath.Join(cand, "index.html")); err == nil {
			return cand
		}
	}
	// Try GOPATH / module root heuristic
	if gomod := os.Getenv("GOMOD"); gomod != "" {
		dir := filepath.Dir(gomod)
		cand := filepath.Join(dir, "ui")
		if _, err := os.Stat(filepath.Join(cand, "index.html")); err == nil {
			return cand
		}
	}
	return "ui"
}
