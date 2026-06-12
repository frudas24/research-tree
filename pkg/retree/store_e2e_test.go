package retree

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"
)

// e2eReport is the auditable artifact produced by the E2E simulation.
type e2eReport struct {
	GeneratedAt string        `json:"generated_at"`
	Scenarios   []e2eScenario `json:"scenarios"`
	Summary     e2eSummary    `json:"summary"`
	GraphState  e2eGraphState `json:"final_graph_state"`
}

type e2eScenario struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Passed      bool   `json:"passed"`
	Error       string `json:"error,omitempty"`
	DurationMs  int64  `json:"duration_ms"`
}

type e2eSummary struct {
	Total     int `json:"total"`
	Passed    int `json:"passed"`
	Failed    int `json:"failed"`
	NodeCount int `json:"final_node_count"`
}

type e2eGraphState struct {
	Roots       []NodeID      `json:"roots"`
	Leaves      []NodeID      `json:"leaves"`
	ActiveNodes []e2eNodeInfo `json:"active_nodes"`
	Warnings    int           `json:"pending_warnings"`
}

type e2eNodeInfo struct {
	ID          NodeID      `json:"id"`
	Title       string      `json:"title"`
	Status      NodeStatus  `json:"status"`
	ClaimStatus ClaimStatus `json:"claim_status"`
	Parents     []NodeID    `json:"parents"`
	Agent       string      `json:"agent"`
	Tags        []string    `json:"tags"`
}

// TestE2ESimulator runs a comprehensive end-to-end simulation of realistic
// ML research workflows and produces an auditable JSON artifact.
func TestE2ESimulator(t *testing.T) {
	root := t.TempDir()
	researchRoot := filepath.Join(root, "research")

	report := &e2eReport{
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
	}
	scenarios := make([]e2eScenario, 0)

	// --- Scenario 1: Init and basic CRUD ---
	scenarios = append(scenarios, runScenario("S01: init + basic CRUD", func() error {
		s, err := Init(researchRoot, StorageJSON)
		if err != nil {
			return fmt.Errorf("init: %w", err)
		}

		// Create root node
		baseline := &Node{Frontmatter: Frontmatter{
			Title:  "KD baseline establecido",
			Status: StatusDone,
			Agent:  "researcher",
			Tags:   []string{"kd", "baseline"},
		}}
		if err := s.CreateNode(baseline); err != nil {
			return fmt.Errorf("create baseline: %w", err)
		}
		if baseline.ID != 1 {
			return fmt.Errorf("expected id=1, got %d", baseline.ID)
		}

		// Create child experiments
		sparse := &Node{Frontmatter: Frontmatter{
			Title:   "KD sparse top-k k=128",
			Status:  StatusActive,
			Parents: []NodeID{baseline.ID},
			Agent:   "researcher",
			Tags:    []string{"kd", "sparse", "experimento"},
		}}
		if err := s.CreateNode(sparse); err != nil {
			return fmt.Errorf("create sparse: %w", err)
		}
		if sparse.ID != 2 {
			return fmt.Errorf("expected id=2, got %d", sparse.ID)
		}

		hybrid := &Node{Frontmatter: Frontmatter{
			Title:   "KD hybrid sparse+dense",
			Status:  StatusActive,
			Parents: []NodeID{baseline.ID},
			Agent:   "opus",
			Tags:    []string{"kd", "hybrid", "experimento"},
		}}
		if err := s.CreateNode(hybrid); err != nil {
			return fmt.Errorf("create hybrid: %w", err)
		}

		// Update sparse → done with artifacts
		sparse.Status = StatusDone
		sparse.ClaimStatus = ClaimValidated
		sparse.Body = "## Results\nk=128 achieves 0.82 recall with 40% sparsity."
		sparse.Commits = []GitCommit{
			{Hash: "abc123", Message: "implement sparse top-k"},
			{Hash: "def456", Message: "fix k=128 reshape"},
		}
		if err := s.UpdateNode(sparse); err != nil {
			return fmt.Errorf("update sparse: %w", err)
		}

		// Get and verify
		got, err := s.GetNode(sparse.ID)
		if err != nil {
			return fmt.Errorf("get sparse: %w", err)
		}
		if got.Status != StatusDone || got.ClaimStatus != ClaimValidated || got.Body == "" {
			return fmt.Errorf("sparse state mismatch: status=%s claim=%s body_len=%d", got.Status, got.ClaimStatus, len(got.Body))
		}

		return nil
	}))

	// --- Scenario 2: Graph queries and navigation ---
	scenarios = append(scenarios, runScenario("S02: graph queries + navigation", func() error {
		s, err := Open(researchRoot)
		if err != nil {
			return fmt.Errorf("open: %w", err)
		}

		// Roots
		roots, err := s.GetRoots()
		if err != nil {
			return err
		}
		if len(roots) != 1 || roots[0] != 1 {
			return fmt.Errorf("expected root=[1], got %v", roots)
		}

		// Children of baseline
		children, err := s.GetChildren(1)
		if err != nil {
			return err
		}
		if len(children) != 2 {
			return fmt.Errorf("expected 2 children, got %d", len(children))
		}

		// Ancestors of hybrid (should be just baseline)
		anc, err := s.GetAncestors(3)
		if err != nil {
			return err
		}
		if len(anc) != 1 || anc[0] != 1 {
			return fmt.Errorf("expected ancestors=[1], got %v", anc)
		}

		// Descendants of baseline (should be sparse + hybrid)
		desc, err := s.GetDescendants(1)
		if err != nil {
			return err
		}
		if len(desc) != 2 {
			return fmt.Errorf("expected 2 descendants, got %d", len(desc))
		}

		// Leaves
		leaves, err := s.GetLeaves()
		if err != nil {
			return err
		}
		if len(leaves) != 2 {
			return fmt.Errorf("expected 2 leaves, got %d", len(leaves))
		}

		return nil
	}))

	// --- Scenario 3: Diamond dependency (merge) ---
	scenarios = append(scenarios, runScenario("S03: diamond dependency (merge)", func() error {
		s, err := Open(researchRoot)
		if err != nil {
			return fmt.Errorf("open: %w", err)
		}

		// Create a merge node with both sparse and hybrid as parents
		merged := &Node{Frontmatter: Frontmatter{
			Title:   "Ensemble KD sparse+hybrid",
			Status:  StatusActive,
			Parents: []NodeID{2, 3},
			Agent:   "researcher",
			Tags:    []string{"kd", "ensemble"},
		}}
		if err := s.CreateNode(merged); err != nil {
			return fmt.Errorf("create merge: %w", err)
		}
		if merged.ID != 4 {
			return fmt.Errorf("expected id=4, got %d", merged.ID)
		}

		// Verify parents
		parents, err := s.GetParents(4)
		if err != nil {
			return err
		}
		if len(parents) != 2 {
			return fmt.Errorf("expected 2 parents, got %d", len(parents))
		}

		// Ancestors of merge should be [1, 2, 3]
		anc, err := s.GetAncestors(4)
		if err != nil {
			return err
		}
		if len(anc) != 3 {
			return fmt.Errorf("expected 3 ancestors, got %d: %v", len(anc), anc)
		}

		return nil
	}))

	// --- Scenario 4: Tags and artifacts ---
	scenarios = append(scenarios, runScenario("S04: tags + artifacts", func() error {
		s, err := Open(researchRoot)
		if err != nil {
			return fmt.Errorf("open: %w", err)
		}

		// Add tags idempotently
		if err := s.AddTags(1, "ml", "ml", "kd"); err != nil {
			return fmt.Errorf("add tags: %w", err)
		}
		n1, _ := s.GetNode(1)
		expected := []string{"baseline", "kd", "ml"}
		if !stringSlicesEqual(n1.Tags, expected) {
			return fmt.Errorf("tags mismatch: got %v want %v", n1.Tags, expected)
		}

		// Remove tags
		if err := s.RemoveTags(1, "ml"); err != nil {
			return fmt.Errorf("remove tags: %w", err)
		}
		n1, _ = s.GetNode(1)
		expected = []string{"baseline", "kd"}
		if !stringSlicesEqual(n1.Tags, expected) {
			return fmt.Errorf("tags after rm mismatch: %v", n1.Tags)
		}

		// Add path-mode artifact
		a := Artifact{Mode: ArtifactPath, Host: "gpu-node-0", Path: "/tmp/model.bin", Description: "model weights", SizeBytes: 6920601}
		if err := s.AddArtifact(2, a); err != nil {
			return fmt.Errorf("add artifact: %w", err)
		}

		// Embed artifact
		tmpFile := filepath.Join(t.TempDir(), "metrics.json")
		if err := os.WriteFile(tmpFile, []byte(`{"loss":28.79,"step":25}`), 0o644); err != nil {
			return err
		}
		if err := s.EmbedArtifact(2, tmpFile, "training metrics"); err != nil {
			return fmt.Errorf("embed artifact: %w", err)
		}

		n2, _ := s.GetNode(2)
		if len(n2.Artifacts) != 2 {
			return fmt.Errorf("expected 2 artifacts, got %d", len(n2.Artifacts))
		}
		// Verify embedded file exists on disk
		emb := n2.Artifacts[1]
		if emb.Mode != ArtifactEmbedded {
			return fmt.Errorf("expected embedded mode, got %s", emb.Mode)
		}
		diskPath := filepath.Join(researchRoot, filepath.FromSlash(emb.Path))
		if _, err := os.Stat(diskPath); err != nil {
			return fmt.Errorf("embedded file missing: %w", err)
		}

		return nil
	}))

	// --- Scenario 5: Filtering ---
	scenarios = append(scenarios, runScenario("S05: filtering + querying", func() error {
		s, err := Open(researchRoot)
		if err != nil {
			return fmt.Errorf("open: %w", err)
		}

		// Filter by status
		active, err := s.ListNodes(Filter{Status: StatusActive})
		if err != nil {
			return err
		}
		if len(active) != 2 {
			return fmt.Errorf("expected 2 active nodes, got %d", len(active))
		}

		// Filter by agent
		researcherNodes, err := s.ListNodes(Filter{Agent: "researcher"})
		if err != nil {
			return err
		}
		if len(researcherNodes) != 3 {
			return fmt.Errorf("expected 3 researcher nodes, got %d", len(researcherNodes))
		}

		// Combined filter: baseline + sparse both done+researcher
		doneResearcher, err := s.QueryNodes(Filter{Status: StatusDone, Agent: "researcher"})
		if err != nil {
			return err
		}
		if len(doneResearcher) != 2 {
			return fmt.Errorf("expected 2 done+researcher nodes, got %d", len(doneResearcher))
		}

		// Title contains "sparse": nodes 2, 3, 4
		sparseNodes, err := s.ListNodes(Filter{TitleContains: "sparse"})
		if err != nil {
			return err
		}
		if len(sparseNodes) != 3 {
			return fmt.Errorf("expected 3 nodes with 'sparse', got %d", len(sparseNodes))
		}

		// Filter by tag
		tagged, err := s.ListNodes(Filter{Tag: "ensemble"})
		if err != nil {
			return err
		}
		if len(tagged) != 1 || tagged[0] != 4 {
			return fmt.Errorf("expected [4] for tag=ensemble, got %v", tagged)
		}

		// Limit
		limited, err := s.ListNodes(Filter{Limit: 1})
		if err != nil {
			return err
		}
		if len(limited) != 1 {
			return fmt.Errorf("expected 1 with limit, got %d", len(limited))
		}

		// Active agents
		agents, err := s.GetActiveAgents()
		if err != nil {
			return err
		}
		if len(agents) != 2 {
			return fmt.Errorf("expected 2 agents, got %v", agents)
		}

		return nil
	}))

	// --- Scenario 6: Invalidation cascade ---
	scenarios = append(scenarios, runScenario("S06: invalidation cascade + warnings", func() error {
		s, err := Open(researchRoot)
		if err != nil {
			return fmt.Errorf("open: %w", err)
		}

		// Create refuter node
		refuter := &Node{Frontmatter: Frontmatter{
			Title:  "Refutation: k=128 overfits small datasets",
			Status: StatusDone,
			Agent:  "opus",
		}}
		if err := s.CreateNode(refuter); err != nil {
			return fmt.Errorf("create refuter: %w", err)
		}

		// Invalidate the sparse node (id=2) claim
		if err := s.InvalidateClaim(2, refuter.ID, "overfitting detected on k<1000"); err != nil {
			return fmt.Errorf("invalidate claim: %w", err)
		}

		// Verify node 2 is now invalidated
		n2, _ := s.GetNode(2)
		if n2.ClaimStatus != ClaimInvalidated {
			return fmt.Errorf("expected invalidated, got %s", n2.ClaimStatus)
		}
		if len(n2.InvalidatedBy) != 1 || n2.InvalidatedBy[0] != refuter.ID {
			return fmt.Errorf("invalidated_by mismatch: %v", n2.InvalidatedBy)
		}

		// Check warnings generated for active descendants (node 4 = ensemble, active)
		warnings, err := s.ListBranchWarnings("researcher", true)
		if err != nil {
			return err
		}
		if len(warnings) == 0 {
			return fmt.Errorf("expected warnings for researcher agent")
		}

		// Ack warning
		if err := s.AckBranchWarning(warnings[0].ID); err != nil {
			return fmt.Errorf("ack warning: %w", err)
		}
		warnings, _ = s.ListBranchWarnings("researcher", true)
		if len(warnings) != 0 {
			return fmt.Errorf("expected 0 unacked after ack, got %d", len(warnings))
		}

		// Verify idempotent invalidation
		if err := s.InvalidateClaim(2, refuter.ID, "overfitting detected on k<1000"); err != nil {
			return fmt.Errorf("idempotent invalidate: %w", err)
		}

		return nil
	}))

	// --- Scenario 7: Cycle prevention ---
	scenarios = append(scenarios, runScenario("S07: cycle prevention", func() error {
		s, err := Open(researchRoot)
		if err != nil {
			return fmt.Errorf("open: %w", err)
		}

		// Try to make baseline depend on ensemble (would create cycle)
		baseline, _ := s.GetNode(1)
		baseline.Parents = []NodeID{4}
		err = s.UpdateNode(baseline)
		if err == nil {
			return fmt.Errorf("expected cycle error, got nil")
		}
		if !strings.Contains(err.Error(), "cycle") {
			return fmt.Errorf("expected cycle error, got: %v", err)
		}

		// Invalid parent reference should fail
		badNode := &Node{Frontmatter: Frontmatter{
			Title:   "orphan test",
			Parents: []NodeID{9999},
		}}
		err = s.CreateNode(badNode)
		if err == nil {
			return fmt.Errorf("expected error for missing parent")
		}

		return nil
	}))

	// --- Scenario 8: Delete with/without force ---
	scenarios = append(scenarios, runScenario("S08: delete + force delete", func() error {
		s, err := Open(researchRoot)
		if err != nil {
			return fmt.Errorf("open: %w", err)
		}

		// Create temporary parent-child for delete testing
		tmpParent := &Node{Frontmatter: Frontmatter{Title: "tmp parent"}}
		if err := s.CreateNode(tmpParent); err != nil {
			return err
		}
		tmpChild := &Node{Frontmatter: Frontmatter{
			Title:   "tmp child",
			Parents: []NodeID{tmpParent.ID},
		}}
		if err := s.CreateNode(tmpChild); err != nil {
			return err
		}

		// Delete without force should fail
		err = s.DeleteNode(tmpParent.ID, false)
		if err == nil {
			return fmt.Errorf("expected ErrHasChildren")
		}

		// Force delete should succeed and orphan child
		if err := s.DeleteNode(tmpParent.ID, true); err != nil {
			return fmt.Errorf("force delete: %w", err)
		}
		orphan, _ := s.GetNode(tmpChild.ID)
		if len(orphan.Parents) != 0 {
			return fmt.Errorf("expected orphaned child, got parents=%v", orphan.Parents)
		}

		return nil
	}))

	// --- Scenario 9: Storage migration JSON ↔ BIN ---
	scenarios = append(scenarios, runScenario("S09: storage migration JSON ↔ BIN", func() error {
		s, err := Open(researchRoot)
		if err != nil {
			return fmt.Errorf("open: %w", err)
		}

		if s.StorageFormat() != StorageJSON {
			return fmt.Errorf("expected JSON, got %s", s.StorageFormat())
		}

		// Migrate to binary
		if err := s.MigrateStorageFormat(StorageBIN); err != nil {
			return fmt.Errorf("migrate to bin: %w", err)
		}
		if s.StorageFormat() != StorageBIN {
			return fmt.Errorf("expected BIN after migration, got %s", s.StorageFormat())
		}

		// Verify data accessible after migration
		n, err := s.GetNode(1)
		if err != nil {
			return fmt.Errorf("get after bin migration: %w", err)
		}
		if n.Title != "KD baseline establecido" {
			return fmt.Errorf("data corruption after migration: title=%q", n.Title)
		}

		// Migrate back
		if err := s.MigrateStorageFormat(StorageJSON); err != nil {
			return fmt.Errorf("migrate back to json: %w", err)
		}

		// Verify again
		n, _ = s.GetNode(1)
		if n.Title != "KD baseline establecido" || n.Status != StatusDone {
			return fmt.Errorf("data corruption after roundtrip: %+v", n.Frontmatter)
		}

		return nil
	}))

	// --- Scenario 10: Snapshots and recovery ---
	scenarios = append(scenarios, runScenario("S10: snapshots + recovery", func() error {
		s, err := Open(researchRoot)
		if err != nil {
			return fmt.Errorf("open: %w", err)
		}

		snaps, err := s.ListSnapshots()
		if err != nil {
			return err
		}
		if len(snaps) == 0 {
			return fmt.Errorf("expected snapshots")
		}
		if len(snaps) > 3 {
			return fmt.Errorf("retention exceeded: %d snapshots", len(snaps))
		}

		// Restore the latest snapshot
		latest := snaps[0]
		if err := s.RestoreSnapshot(latest.ID); err != nil {
			return fmt.Errorf("restore: %w", err)
		}

		// After restore, store should be functional
		ids, err := s.ListNodes(Filter{})
		if err != nil {
			return fmt.Errorf("list after restore: %w", err)
		}
		if len(ids) < 4 {
			return fmt.Errorf("expected >=4 nodes after restore, got %d", len(ids))
		}

		return nil
	}))

	// --- Scenario 11: Concurrent operations ---
	scenarios = append(scenarios, runScenario("S11: concurrent operations", func() error {
		s, err := Open(researchRoot)
		if err != nil {
			return fmt.Errorf("open: %w", err)
		}

		const n = 10
		var wg sync.WaitGroup
		wg.Add(n)
		errs := make(chan error, n)
		for i := 0; i < n; i++ {
			go func(idx int) {
				defer wg.Done()
				err := s.CreateNode(&Node{Frontmatter: Frontmatter{
					Title: fmt.Sprintf("concurrent-%d", idx),
					Tags:  []string{"concurrent"},
				}})
				errs <- err
			}(i)
		}
		wg.Wait()
		close(errs)
		for err := range errs {
			if err != nil {
				return fmt.Errorf("concurrent create: %w", err)
			}
		}

		// Verify no ID collisions
		ids, _ := s.ListNodes(Filter{Tag: "concurrent"})
		if len(ids) != n {
			return fmt.Errorf("expected %d concurrent nodes, got %d", n, len(ids))
		}

		return nil
	}))

	// --- Scenario 12: Edge cases (unicode, empty store, special chars) ---
	scenarios = append(scenarios, runScenario("S12: edge cases (unicode, empty operations)", func() error {
		s, err := Open(researchRoot)
		if err != nil {
			return fmt.Errorf("open: %w", err)
		}

		// Unicode in title and body
		uniNode := &Node{Frontmatter: Frontmatter{
			Title: "実験: 日本語テスト 🧪",
			Tags:  []string{"unicode", "実験"},
		}}
		uniNode.Body = "## 結果\n精度は 95% 以上でした。\n```\nα → β → γ\n```"
		if err := s.CreateNode(uniNode); err != nil {
			return fmt.Errorf("create unicode: %w", err)
		}
		got, _ := s.GetNode(uniNode.ID)
		if got.Title != uniNode.Title || got.Body != uniNode.Body {
			return fmt.Errorf("unicode roundtrip failed")
		}

		// Empty store operations (these should not panic)
		emptyRoot := filepath.Join(t.TempDir(), "empty-research")
		emptyStore, err := Init(emptyRoot, StorageJSON)
		if err != nil {
			return fmt.Errorf("init empty: %w", err)
		}
		ids, _ := emptyStore.ListNodes(Filter{})
		if len(ids) != 0 {
			return fmt.Errorf("expected 0 nodes in empty store")
		}
		roots, _ := emptyStore.GetRoots()
		if len(roots) != 0 {
			return fmt.Errorf("expected 0 roots in empty store")
		}
		leaves, _ := emptyStore.GetLeaves()
		if len(leaves) != 0 {
			return fmt.Errorf("expected 0 leaves in empty store")
		}
		_, err = emptyStore.GetNode(1)
		if err == nil {
			return fmt.Errorf("expected ErrNotFound on empty store")
		}
		agents, _ := emptyStore.GetActiveAgents()
		if len(agents) != 0 {
			return fmt.Errorf("expected 0 agents in empty store")
		}

		return nil
	}))

	// --- Scenario 13: Agent name resolution ---
	scenarios = append(scenarios, runScenario("S13: agent name resolution", func() error {
		s, err := Open(researchRoot)
		if err != nil {
			return fmt.Errorf("open: %w", err)
		}

		// Default agent.local exists from init
		name := s.ResolveAgentName("local")
		if name != "local" {
			return fmt.Errorf("expected 'local', got %q", name)
		}

		// Unknown agent returns the ID itself
		name = s.ResolveAgentName("nonexistent")
		if name != "nonexistent" {
			return fmt.Errorf("expected 'nonexistent', got %q", name)
		}

		return nil
	}))

	// --- Scenario 14: Exhaustive field-level JSON ↔ BIN equivalence ---
	scenarios = append(scenarios, runScenario("S14: exhaustive field-level JSON↔BIN equivalence", func() error {
		// Start fresh with JSON to isolate the comparison
		eqRoot := filepath.Join(t.TempDir(), "equivalence")
		sJSON, err := Init(eqRoot, StorageJSON)
		if err != nil {
			return fmt.Errorf("init json: %w", err)
		}

		// Create a node with every possible field populated
		full := &Node{Frontmatter: Frontmatter{
			Title:       "Full spectrum test node",
			Status:      StatusDone,
			ClaimStatus: ClaimValidated,
			Parents:     []NodeID{},
			Agent:       "equivalence-tester",
			Tags:        []string{"alpha", "beta", "gamma"},
		}}
		full.Commits = []GitCommit{
			{Hash: "a1b2c3d4e5f6a7b8c9d0e1f2", Message: "initial commit with long message"},
			{Hash: "ff", Message: ""},
		}
		full.Artifacts = []Artifact{
			{Mode: ArtifactPath, Host: "server.01", Path: "/data/models/large.bin", Description: "model file", SizeBytes: 12345678901},
			{Mode: ArtifactEmbedded, Path: "artifacts/0099/plot.png", Description: "", SizeBytes: 0},
		}
		full.InvalidatedBy = []NodeID{5, 10, 15}
		full.InvalidationReason = "Inconsistent results across seeds 42, 123, 999"
		full.Body = "## Section\nContent with `code` and **bold**.\n"
		if err := sJSON.CreateNode(full); err != nil {
			return fmt.Errorf("create full in json: %w", err)
		}

		// Also create minimal node and node with empty strings
		minimal := &Node{Frontmatter: Frontmatter{Title: "minimal"}}
		if err := sJSON.CreateNode(minimal); err != nil {
			return fmt.Errorf("create minimal: %w", err)
		}

		// Snapshot all JSON nodes for comparison
		jsonNodes := make(map[NodeID]*Node)
		all, _ := sJSON.QueryNodes(Filter{})
		for _, n := range all {
			jsonNodes[n.ID] = CloneNode(n)
		}

		// Migrate to BIN
		if err := sJSON.MigrateStorageFormat(StorageBIN); err != nil {
			return fmt.Errorf("migrate to bin: %w", err)
		}

		// Re-open and load all BIN nodes
		sBIN, err := Open(eqRoot)
		if err != nil {
			return fmt.Errorf("reopen bin: %w", err)
		}
		binNodes, _ := sBIN.QueryNodes(Filter{})

		// Exhaustive field-by-field comparison
		if len(binNodes) != len(jsonNodes) {
			return fmt.Errorf("node count mismatch: json=%d bin=%d", len(jsonNodes), len(binNodes))
		}
		for _, bn := range binNodes {
			jn, ok := jsonNodes[bn.ID]
			if !ok {
				return fmt.Errorf("node %d missing in json set", bn.ID)
			}

			// Serialize both to JSON for deep comparison
			jb, _ := json.Marshal(jn)
			bb, _ := json.Marshal(bn)
			if string(jb) != string(bb) {
				return fmt.Errorf("node %d field-level mismatch:\n  json: %s\n  bin:  %s", bn.ID, jb, bb)
			}
		}

		// Verify binary header exists and is valid before migrating back
		headerPath := filepath.Join(eqRoot, "nodes.bin")
		f, err := os.Open(headerPath)
		if err != nil {
			return fmt.Errorf("open nodes.bin: %w", err)
		}
		header := make([]byte, binHeaderSize)
		if _, err := f.Read(header); err != nil {
			f.Close()
			return fmt.Errorf("read header: %w", err)
		}
		f.Close()
		if _, err := ReadBinHeader(header); err != nil {
			return fmt.Errorf("invalid binary header: %w", err)
		}

		// Verify nodes/ directory was cleaned up (migration JSON→BIN removes it)
		if _, err := os.Stat(filepath.Join(eqRoot, "nodes")); !os.IsNotExist(err) {
			return fmt.Errorf("expected nodes/ to be removed after JSON→BIN migration")
		}

		// Migrate back to JSON and verify again (double roundtrip)
		if err := sBIN.MigrateStorageFormat(StorageJSON); err != nil {
			return fmt.Errorf("migrate back: %w", err)
		}
		sBack, _ := Open(eqRoot)
		backNodes, _ := sBack.QueryNodes(Filter{})
		for _, bn := range backNodes {
			jn, ok := jsonNodes[bn.ID]
			if !ok {
				return fmt.Errorf("node %d missing after double roundtrip", bn.ID)
			}
			jb, _ := json.Marshal(jn)
			bb, _ := json.Marshal(bn)
			if string(jb) != string(bb) {
				return fmt.Errorf("node %d mismatch after JSON→BIN→JSON roundtrip:\n  orig: %s\n  back: %s", bn.ID, jb, bb)
			}
		}

		return nil
	}))

	// --- Scenario 15: Native BIN format operations ---
	scenarios = append(scenarios, runScenario("S15: native BIN format operations", func() error {
		binRoot := filepath.Join(t.TempDir(), "native-bin")
		s, err := Init(binRoot, StorageBIN)
		if err != nil {
			return fmt.Errorf("init bin: %w", err)
		}
		if s.StorageFormat() != StorageBIN {
			return fmt.Errorf("expected BIN format")
		}

		// Full CRUD in native BIN
		a := &Node{Frontmatter: Frontmatter{
			Title:  "BIN-native root",
			Status: StatusActive,
			Agent:  "bin-agent",
			Tags:   []string{"bin", "native"},
		}}
		if err := s.CreateNode(a); err != nil {
			return fmt.Errorf("create in bin: %w", err)
		}
		b := &Node{Frontmatter: Frontmatter{
			Title:   "BIN-native child",
			Parents: []NodeID{a.ID},
			Status:  StatusActive,
		}}
		if err := s.CreateNode(b); err != nil {
			return fmt.Errorf("create child in bin: %w", err)
		}
		// Update
		b.Status = StatusDone
		b.ClaimStatus = ClaimValidated
		if err := s.UpdateNode(b); err != nil {
			return fmt.Errorf("update in bin: %w", err)
		}
		// Query
		got, err := s.GetNode(b.ID)
		if err != nil {
			return fmt.Errorf("get in bin: %w", err)
		}
		if got.Status != StatusDone || got.ClaimStatus != ClaimValidated {
			return fmt.Errorf("bin update mismatch")
		}
		// Filter
		active, _ := s.ListNodes(Filter{Status: StatusActive})
		if len(active) != 1 || active[0] != a.ID {
			return fmt.Errorf("bin filter mismatch: %v", active)
		}
		// Graph queries
		children, _ := s.GetChildren(a.ID)
		if len(children) != 1 || children[0] != b.ID {
			return fmt.Errorf("bin children mismatch")
		}
		// Delete
		if err := s.DeleteNode(b.ID, false); err != nil {
			return fmt.Errorf("delete in bin: %w", err)
		}
		// Verify binary file has valid header
		idxData, _ := os.ReadFile(filepath.Join(binRoot, "nodes.idx"))
		if len(idxData) == 0 {
			return fmt.Errorf("empty index file")
		}
		headerPath := filepath.Join(binRoot, "nodes.bin")
		f, _ := os.Open(headerPath)
		defer f.Close()
		header := make([]byte, binHeaderSize)
		f.Read(header)
		if _, err := ReadBinHeader(header); err != nil {
			return fmt.Errorf("bin header invalid: %w", err)
		}

		return nil
	}))

	// --- Scenario 16: DAG invariants and edge consistency ---
	scenarios = append(scenarios, runScenario("S16: DAG invariants + edge consistency", func() error {
		s, err := Open(researchRoot)
		if err != nil {
			return fmt.Errorf("open: %w", err)
		}
		return validateStoreInvariants(s)
	}))

	// Assemble report
	report.Scenarios = scenarios
	passed := 0
	failed := 0
	for _, sc := range scenarios {
		if sc.Passed {
			passed++
		} else {
			failed++
		}
	}

	// Final graph state
	s, _ := Open(researchRoot)
	allNodes, _ := s.QueryNodes(Filter{})
	roots, _ := s.GetRoots()
	leaves, _ := s.GetLeaves()
	warnings, _ := s.ListBranchWarnings("", true)

	activeInfos := make([]e2eNodeInfo, 0)
	for _, n := range allNodes {
		if n.Status == StatusActive {
			activeInfos = append(activeInfos, e2eNodeInfo{
				ID: n.ID, Title: n.Title, Status: n.Status,
				ClaimStatus: n.ClaimStatus, Parents: n.Parents,
				Agent: n.Agent, Tags: n.Tags,
			})
		}
	}
	sort.Slice(activeInfos, func(i, j int) bool { return activeInfos[i].ID < activeInfos[j].ID })

	report.Summary = e2eSummary{
		Total:     len(scenarios),
		Passed:    passed,
		Failed:    failed,
		NodeCount: len(allNodes),
	}
	report.GraphState = e2eGraphState{
		Roots:       roots,
		Leaves:      leaves,
		ActiveNodes: activeInfos,
		Warnings:    len(warnings),
	}

	// Write auditable artifact
	artifactPath := filepath.Join(t.TempDir(), "e2e_audit_report.json")
	b, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		t.Fatalf("marshal report: %v", err)
	}
	if err := os.WriteFile(artifactPath, b, 0o644); err != nil {
		t.Fatalf("write report: %v", err)
	}
	t.Logf("E2E audit report: %s (%d bytes)", artifactPath, len(b))

	if failed > 0 {
		t.Errorf("E2E: %d/%d scenarios failed", failed, len(scenarios))
		for _, sc := range scenarios {
			if !sc.Passed {
				t.Errorf("  FAIL %s: %s", sc.Name, sc.Error)
			}
		}
	}
}

// validateStoreInvariants checks critical invariants: ID uniqueness, no cycles, edge
// consistency, ID monotonicity, and parent referential integrity. Returns the first
// violation found or nil if all invariants hold.
func validateStoreInvariants(s *Store) error {
	all, err := s.QueryNodes(Filter{})
	if err != nil {
		return fmt.Errorf("query all: %w", err)
	}

	// 1. ID uniqueness
	seen := map[NodeID]bool{}
	for _, n := range all {
		if seen[n.ID] {
			return fmt.Errorf("invariant violated: duplicate ID %d", n.ID)
		}
		seen[n.ID] = true
	}

	// 2. ID monotonicity via NextID
	next := s.NextID()
	for _, n := range all {
		if n.ID >= next {
			return fmt.Errorf("invariant violated: node %d >= next_id %d", n.ID, next)
		}
	}

	// 3. Parent referential integrity: all parents must exist
	nodesByID := make(map[NodeID]*Node, len(all))
	for _, n := range all {
		nodesByID[n.ID] = n
	}
	for _, n := range all {
		for _, pid := range n.Parents {
			if _, ok := nodesByID[pid]; !ok {
				return fmt.Errorf("invariant violated: node %d references missing parent %d", n.ID, pid)
			}
		}
	}

	// 4. No cycles: loadGraph would reject them on construction, but verify here.
	g, err := s.loadGraph()
	if err != nil {
		return fmt.Errorf("load graph: %w", err)
	}
	for _, n := range g.Nodes {
		for _, pid := range n.Parents {
			if g.WouldCreateCycle(n.ID, []NodeID{pid}) {
				return fmt.Errorf("invariant violated: cycle detected through %d <- %d", pid, n.ID)
			}
		}
	}

	// 5. Edge index consistency: edges.jsonl must match node parents
	edgesPath := s.edgesPath()
	edgeData, err := os.ReadFile(edgesPath)
	if err != nil {
		return fmt.Errorf("read edges: %w", err)
	}
	edgeSet := map[string]bool{}
	for _, line := range strings.Split(strings.TrimSpace(string(edgeData)), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		edgeSet[line] = true
	}
	for _, n := range all {
		for _, pid := range n.Parents {
			expected := fmt.Sprintf(`{"from":%d,"to":%d}`, pid, n.ID)
			if !edgeSet[expected] {
				return fmt.Errorf("invariant violated: edge from=%d to=%d missing in edges.jsonl", pid, n.ID)
			}
		}
	}

	// 6. Schema version consistency
	for _, n := range all {
		if n.SchemaVersion != CurrentSchemaVersion {
			return fmt.Errorf("invariant violated: node %d has schema %d, want %d", n.ID, n.SchemaVersion, CurrentSchemaVersion)
		}
	}

	// 7. Claim status consistency: invalidated implies invalidated_by non-empty
	for _, n := range all {
		if n.ClaimStatus == ClaimInvalidated && len(n.InvalidatedBy) == 0 {
			return fmt.Errorf("invariant violated: node %d is invalidated but invalidated_by is empty", n.ID)
		}
	}

	// 8. ValidateNode for every node
	for _, n := range all {
		if err := ValidateNode(n); err != nil {
			return fmt.Errorf("invariant violated: node %d fails validation: %w", n.ID, err)
		}
	}

	return nil
}

// runScenario executes a scenario function and wraps it with timing and error capture.
func runScenario(name string, fn func() error) e2eScenario {
	start := time.Now()
	err := fn()
	elapsed := time.Since(start).Milliseconds()
	sc := e2eScenario{
		Name:       name,
		Passed:     err == nil,
		DurationMs: elapsed,
	}
	if err != nil {
		sc.Error = err.Error()
	}
	return sc
}

// stringSlicesEqual compares two sorted string slices.
func stringSlicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
