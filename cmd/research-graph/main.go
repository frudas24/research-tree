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
	mux.Handle("/cytoscape/", fs)

	// /graph endpoint: full DAG projection
	mux.HandleFunc("/graph", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Access-Control-Allow-Origin", "*")

		payload := buildGraphPayload(store)
		if err := json.NewEncoder(w).Encode(payload); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	})

	// /node endpoint: full detail for one node
	mux.HandleFunc("/node", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Access-Control-Allow-Origin", "*")

		idStr := r.URL.Query().Get("id")
		var id uint64
		if _, err := fmt.Sscanf(idStr, "%d", &id); err != nil || id == 0 {
			http.Error(w, "invalid id", http.StatusBadRequest)
			return
		}

		n, err := store.GetNode(retree.NodeID(id))
		if err != nil {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}

		detail := buildNodeDetail(store, n)
		if err := json.NewEncoder(w).Encode(detail); err != nil {
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

// ── DTOs ──

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
	Parents         []uint64 `json:"parents,omitempty"`
	PrimaryParent   *uint64  `json:"primary_parent,omitempty"`
}

type NodeDetail struct {
	ID                 uint64        `json:"id"`
	Title              string        `json:"title"`
	Status             string        `json:"status"`
	ClaimStatus        string        `json:"claim_status"`
	EvidenceStatus     string        `json:"evidence_status"`
	EvidenceCause      string        `json:"evidence_cause,omitempty"`
	EvidenceScope      string        `json:"evidence_scope,omitempty"`
	Outcome            string        `json:"outcome"`
	MilestoneClass     string        `json:"milestone_class,omitempty"`
	MilestoneKind      string        `json:"milestone_kind,omitempty"`
	MilestoneReason    string        `json:"milestone_reason,omitempty"`
	Agent              string        `json:"agent,omitempty"`
	Scope              string        `json:"scope,omitempty"`
	ExitCriteria       string        `json:"exit_criteria,omitempty"`
	Created            string        `json:"created,omitempty"`
	Modified           string        `json:"modified,omitempty"`
	Revision           uint64        `json:"revision"`
	Tags               []string      `json:"tags,omitempty"`
	Parents            []uint64      `json:"parents,omitempty"`
	Children           []uint64      `json:"children,omitempty"`
	PendingChildren    int           `json:"pending_children"`
	PrimaryParent      *uint64       `json:"primary_parent,omitempty"`
	ContinuedBy        []uint64      `json:"continued_by,omitempty"`
	SupersededBy       []uint64      `json:"superseded_by,omitempty"`
	InvalidatedBy      []uint64      `json:"invalidated_by,omitempty"`
	PoisonedBy         []uint64      `json:"poisoned_by,omitempty"`
	RevalidatedBy      []uint64      `json:"revalidated_by,omitempty"`
	InvalidationReason string        `json:"invalidation_reason,omitempty"`
	PoisonReason       string        `json:"poison_reason,omitempty"`
	Relations          []RelationDTO `json:"relations,omitempty"`
	RelationOf         []RelationDTO `json:"relation_of,omitempty"`
	RunsCount          int           `json:"runs_count"`
	ArtifactsCount     int           `json:"artifacts_count"`
	CommitsCount       int           `json:"commits_count"`
	Body               string        `json:"body,omitempty"`
	Hotness            int           `json:"hotness"`
}

type RelationDTO struct {
	Type   string `json:"type"`
	Target uint64 `json:"target"`
	Note   string `json:"note,omitempty"`
}

type GraphEdge struct {
	From uint64 `json:"from"`
	To   uint64 `json:"to"`
}

type GraphRelation struct {
	From   uint64 `json:"from"`
	Target uint64 `json:"target"`
	Type   string `json:"type"`
	Note   string `json:"note,omitempty"`
}

type GraphPayload struct {
	Nodes     []GraphNode     `json:"nodes"`
	Edges     []GraphEdge     `json:"edges"`
	Relations []GraphRelation `json:"relations"`
	Total     int             `json:"total"`
}

// ── Builders ──

func buildGraphPayload(store *retree.Store) GraphPayload {
	nodes, _ := store.QueryNodes(retree.Filter{SortBy: "id", Order: "asc"})

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

		hotness := pending * 5
		if !n.Created.IsZero() {
			ageDays := int(now.Sub(n.Created).Hours() / 24)
			hotness += ageDays
		}
		if n.Outcome == retree.OutcomeInconclusive {
			hotness += 5
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
			Children:        idsToU64(children),
			PendingChildren: pending,
			Hotness:         hotness,
			Tags:            n.Tags,
			Scope:           n.Scope,
			Parents:         idsToU64(n.Parents),
			PrimaryParent:   idPtrToU64(n.PrimaryParent),
		}
		graphNodes = append(graphNodes, gn)

		for _, pid := range n.Parents {
			graphEdges = append(graphEdges, GraphEdge{From: uint64(pid), To: uint64(n.ID)})
		}

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

func buildNodeDetail(store *retree.Store, n *retree.Node) NodeDetail {
	// Build children list
	nodes, _ := store.QueryNodes(retree.Filter{SortBy: "id", Order: "asc"})
	childrenByParent := map[retree.NodeID][]retree.NodeID{}
	statusByID := map[retree.NodeID]retree.NodeStatus{}
	for _, node := range nodes {
		statusByID[node.ID] = node.Status
		for _, pid := range node.Parents {
			childrenByParent[pid] = append(childrenByParent[pid], node.ID)
		}
	}
	children := childrenByParent[n.ID]
	pending := 0
	for _, cid := range children {
		if s, ok := statusByID[cid]; ok && s != retree.StatusDone {
			pending++
		}
	}

	now := time.Now().UTC()
	hotness := pending * 5
	if !n.Created.IsZero() {
		hotness += int(now.Sub(n.Created).Hours() / 24)
	}
	if n.Outcome == retree.OutcomeInconclusive {
		hotness += 5
	}

	// Relations pointing TO this node
	allNodes, _ := store.QueryNodes(retree.Filter{})
	var relationOf []RelationDTO
	for _, other := range allNodes {
		for _, rel := range other.Relations {
			if rel.Target == n.ID {
				relationOf = append(relationOf, RelationDTO{
					Type:   string(rel.Type),
					Target: uint64(other.ID),
					Note:   rel.Note,
				})
			}
		}
	}

	body := n.Body
	if len(body) > 2000 {
		body = body[:2000] + "\n... (truncated)"
	}

	detail := NodeDetail{
		ID:                 uint64(n.ID),
		Title:              n.Title,
		Status:             string(n.Status),
		ClaimStatus:        string(n.ClaimStatus),
		EvidenceStatus:     string(n.EvidenceStatus),
		EvidenceCause:      string(n.EvidenceCause),
		EvidenceScope:      n.EvidenceScope,
		Outcome:            string(n.Outcome),
		MilestoneClass:     string(n.MilestoneClass),
		MilestoneKind:      string(n.MilestoneKind),
		MilestoneReason:    n.MilestoneReason,
		Agent:              n.Agent,
		Scope:              n.Scope,
		ExitCriteria:       n.ExitCriteria,
		Created:            n.Created.Format(time.RFC3339),
		Modified:           n.Modified.Format(time.RFC3339),
		Revision:           n.Revision,
		Tags:               n.Tags,
		Parents:            idsToU64(n.Parents),
		Children:           idsToU64(children),
		PendingChildren:    pending,
		PrimaryParent:      idPtrToU64(n.PrimaryParent),
		ContinuedBy:        idsToU64(n.ContinuedBy),
		SupersededBy:       idsToU64(n.SupersededBy),
		InvalidatedBy:      idsToU64(n.InvalidatedBy),
		PoisonedBy:         idsToU64(n.PoisonedBy),
		RevalidatedBy:      idsToU64(n.RevalidatedBy),
		InvalidationReason: n.InvalidationReason,
		PoisonReason:       n.PoisonReason,
		RunsCount:          len(n.Runs),
		ArtifactsCount:     len(n.Artifacts),
		CommitsCount:       len(n.Commits),
		Body:               body,
		Hotness:            hotness,
	}

	// Outgoing relations
	for _, rel := range n.Relations {
		detail.Relations = append(detail.Relations, RelationDTO{
			Type:   string(rel.Type),
			Target: uint64(rel.Target),
			Note:   rel.Note,
		})
	}
	detail.RelationOf = relationOf

	return detail
}

// ── Helpers ──

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

func idsToU64(ids []retree.NodeID) []uint64 {
	out := make([]uint64, len(ids))
	for i, id := range ids {
		out[i] = uint64(id)
	}
	return out
}

func idPtrToU64(p *retree.NodeID) *uint64 {
	if p == nil {
		return nil
	}
	v := uint64(*p)
	return &v
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

func resolveUIDir() string {
	if _, err := os.Stat("ui/index.html"); err == nil {
		return "ui"
	}
	if exe, err := os.Executable(); err == nil {
		cand := filepath.Join(filepath.Dir(exe), "ui")
		if _, err := os.Stat(filepath.Join(cand, "index.html")); err == nil {
			return cand
		}
	}
	if gomod := os.Getenv("GOMOD"); gomod != "" {
		dir := filepath.Dir(gomod)
		cand := filepath.Join(dir, "ui")
		if _, err := os.Stat(filepath.Join(cand, "index.html")); err == nil {
			return cand
		}
	}
	return "ui"
}
