package retree

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// TestInitCreatesLayoutAndOpen verifies init creates the layout and open succeeds.
func TestInitCreatesLayoutAndOpen(t *testing.T) {
	root := t.TempDir()
	s, err := Init(filepath.Join(root, "research"), StorageJSON)
	if err != nil {
		t.Fatalf("init: %v", err)
	}
	for _, p := range []string{s.metaPath(), s.nodesDir(), s.historyDir(), s.nextIDPath(), s.edgesPath()} {
		if _, err := os.Stat(p); err != nil {
			t.Fatalf("expected path exists %s: %v", p, err)
		}
	}
	opened, err := Open(s.rootPath)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if opened.StorageFormat() != StorageJSON {
		t.Fatalf("format mismatch: %v", opened.StorageFormat())
	}
}

// TestOpenFailsWhenMissing verifies open fails for nonexistent root.
func TestOpenFailsWhenMissing(t *testing.T) {
	_, err := Open(filepath.Join(t.TempDir(), "missing"))
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

// TestOpenFailsWithUnsupportedSchema verifies open fails with unsupported schema.
func TestOpenFailsWithUnsupportedSchema(t *testing.T) {
	s := mustInit(t, StorageJSON)
	meta := map[string]any{
		"schema_version": 999,
		"storage_format": "json",
		"created_at":     time.Now().UTC().Format(time.RFC3339),
	}
	b, err := json.Marshal(meta)
	mustNoErr(t, err)
	mustNoErr(t, os.WriteFile(s.metaPath(), b, 0o644))
	_, err = Open(s.rootPath)
	if !errors.Is(err, ErrUnsupportedSchema) {
		t.Fatalf("expected ErrUnsupportedSchema, got %v", err)
	}
}

// TestCreateGetUpdateDeleteFlowJSON verifies the full CRUD lifecycle in JSON mode.
func TestCreateGetUpdateDeleteFlowJSON(t *testing.T) {
	s := mustInit(t, StorageJSON)
	n1 := &Node{Frontmatter: Frontmatter{Title: "root", Tags: []string{"kd"}}}
	if err := s.CreateNode(n1); err != nil {
		t.Fatalf("create root: %v", err)
	}
	if n1.ID != 1 {
		t.Fatalf("expected id=1 got=%d", n1.ID)
	}
	n2 := &Node{Frontmatter: Frontmatter{Title: "child", Parents: []NodeID{n1.ID}, Status: StatusActive}}
	if err := s.CreateNode(n2); err != nil {
		t.Fatalf("create child: %v", err)
	}
	if n2.ID != 2 {
		t.Fatalf("expected id=2 got=%d", n2.ID)
	}

	n2g, err := s.GetNode(2)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	before := n2g.Modified
	time.Sleep(5 * time.Millisecond)
	n2g.Status = StatusDone
	n2g.Parents = nil
	if err := s.UpdateNode(n2g); err != nil {
		t.Fatalf("update: %v", err)
	}
	n2u, err := s.GetNode(2)
	if err != nil {
		t.Fatalf("get updated: %v", err)
	}
	if !n2u.Modified.After(before) {
		t.Fatalf("expected modified to advance")
	}
	if len(n2u.Parents) != 0 {
		t.Fatalf("expected parents cleared")
	}

	if err := s.DeleteNode(1, false); err != nil {
		t.Fatalf("delete parent should succeed after unlink: %v", err)
	}
	if _, err := s.GetNode(1); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected root gone, got %v", err)
	}
}

// TestDeleteWithoutForceFailsWithChildren verifies delete fails when node has children.
func TestDeleteWithoutForceFailsWithChildren(t *testing.T) {
	s := mustInit(t, StorageJSON)
	a := &Node{Frontmatter: Frontmatter{Title: "a"}}
	mustNoErr(t, s.CreateNode(a))
	b := &Node{Frontmatter: Frontmatter{Title: "b", Parents: []NodeID{a.ID}}}
	mustNoErr(t, s.CreateNode(b))
	if err := s.DeleteNode(a.ID, false); !errors.Is(err, ErrHasChildren) {
		t.Fatalf("expected ErrHasChildren, got %v", err)
	}
	mustNoErr(t, s.DeleteNode(a.ID, true))
	bn, err := s.GetNode(b.ID)
	mustNoErr(t, err)
	if len(bn.Parents) != 0 {
		t.Fatalf("expected child orphaned")
	}
}

// TestUpdateRejectsCycle verifies update rejects cycles.
func TestUpdateRejectsCycle(t *testing.T) {
	s := mustInit(t, StorageJSON)
	a := &Node{Frontmatter: Frontmatter{Title: "a"}}
	b := &Node{Frontmatter: Frontmatter{Title: "b", Parents: []NodeID{1}}}
	mustNoErr(t, s.CreateNode(a))
	b.Parents = []NodeID{a.ID}
	mustNoErr(t, s.CreateNode(b))
	na, err := s.GetNode(a.ID)
	mustNoErr(t, err)
	na.Parents = []NodeID{b.ID}
	err = s.UpdateNode(na)
	if !errors.Is(err, ErrCycleDetected) {
		t.Fatalf("expected ErrCycleDetected, got %v", err)
	}
}

// TestConcurrentCreateNoDuplicateIDs verifies concurrent node creation produces unique IDs.
func TestConcurrentCreateNoDuplicateIDs(t *testing.T) {
	s := mustInit(t, StorageJSON)
	const n = 20
	wg := sync.WaitGroup{}
	wg.Add(n)
	errs := make(chan error, n)
	for i := 0; i < n; i++ {
		i := i
		go func() {
			defer wg.Done()
			err := s.CreateNode(&Node{Frontmatter: Frontmatter{Title: "n" + string(rune('a'+i%26))}})
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent create error: %v", err)
		}
	}
	ids, err := s.ListNodes(Filter{})
	mustNoErr(t, err)
	if len(ids) != n {
		t.Fatalf("expected %d nodes, got %d", n, len(ids))
	}
	for i := 1; i <= n; i++ {
		if ids[i-1] != NodeID(i) {
			t.Fatalf("expected sequential ids, got %v", ids)
		}
	}
}

// TestListNodesFilters verifies list filters work correctly.
func TestListNodesFilters(t *testing.T) {
	s := mustInit(t, StorageJSON)
	n1 := &Node{Frontmatter: Frontmatter{Title: "KD alpha", Status: StatusActive, Tags: []string{"kd"}, Agent: "researcher", Scope: "mistral ctx=2048", MilestoneClass: MilestoneGolden, MilestoneKind: MilestoneKindChampion, MilestoneReason: "current lineage champion"}, Body: "alpha body with flask"}
	n2 := &Node{Frontmatter: Frontmatter{Title: "beta", Status: StatusDone, Tags: []string{"other"}, Agent: "opus", ClaimStatus: ClaimValidated}, Body: "beta body with websocket"}
	mustNoErr(t, s.CreateNode(n1))
	mustNoErr(t, s.CreateNode(n2))

	ids, err := s.ListNodes(Filter{Status: StatusActive})
	mustNoErr(t, err)
	if len(ids) != 1 || ids[0] != n1.ID {
		t.Fatalf("status filter mismatch: %v", ids)
	}
	ids, err = s.ListNodes(Filter{Tag: "kd"})
	mustNoErr(t, err)
	if len(ids) != 1 || ids[0] != n1.ID {
		t.Fatalf("tag filter mismatch: %v", ids)
	}
	ids, err = s.ListNodes(Filter{ClaimStatus: ClaimValidated})
	mustNoErr(t, err)
	if len(ids) != 1 || ids[0] != n2.ID {
		t.Fatalf("claim filter mismatch: %v", ids)
	}
	ids, err = s.ListNodes(Filter{TitleContains: "kd"})
	mustNoErr(t, err)
	if len(ids) != 1 || ids[0] != n1.ID {
		t.Fatalf("title filter mismatch: %v", ids)
	}
	ids, err = s.ListNodes(Filter{BodyContains: "flask"})
	mustNoErr(t, err)
	if len(ids) != 1 || ids[0] != n1.ID {
		t.Fatalf("body filter mismatch: %v", ids)
	}
	ids, err = s.ListNodes(Filter{ScopeContains: "ctx=2048"})
	mustNoErr(t, err)
	if len(ids) != 1 || ids[0] != n1.ID {
		t.Fatalf("scope filter mismatch: %v", ids)
	}
	ids, err = s.ListNodes(Filter{MilestoneClass: MilestoneGolden})
	mustNoErr(t, err)
	if len(ids) != 1 || ids[0] != n1.ID {
		t.Fatalf("milestone_class filter mismatch: %v", ids)
	}
	ids, err = s.ListNodes(Filter{MilestoneKind: MilestoneKindChampion})
	mustNoErr(t, err)
	if len(ids) != 1 || ids[0] != n1.ID {
		t.Fatalf("milestone_kind filter mismatch: %v", ids)
	}
}

// TestQueryNodesCompoundFiltersSortAndOffset verifies compound filters,
// sorting, and pagination behavior.
func TestQueryNodesCompoundFiltersSortAndOffset(t *testing.T) {
	s := mustInit(t, StorageJSON)
	mustNoErr(t, s.CreateNode(&Node{Frontmatter: Frontmatter{
		Title:  "gamma",
		Status: StatusDone, Outcome: OutcomeSuccess, Tags: []string{"a", "b"}, Agent: "x",
	}}))
	time.Sleep(5 * time.Millisecond)
	mustNoErr(t, s.CreateNode(&Node{Frontmatter: Frontmatter{
		Title:  "alpha",
		Status: StatusDone, Outcome: OutcomeSuccess, Tags: []string{"a", "c"}, Agent: "x",
	}, Artifacts: []Artifact{{Mode: ArtifactEmbedded, Path: "artifacts/0002/x.txt"}}}))
	time.Sleep(5 * time.Millisecond)
	mustNoErr(t, s.CreateNode(&Node{Frontmatter: Frontmatter{
		Title:  "beta",
		Status: StatusDone, Outcome: OutcomeFailure, Tags: []string{"c"}, Agent: "y",
	}}))

	hasArtifact := true
	nodes, err := s.QueryNodes(Filter{
		Status:      StatusDone,
		Outcome:     OutcomeSuccess,
		TagsAll:     []string{"a", "c"},
		HasArtifact: &hasArtifact,
	})
	mustNoErr(t, err)
	if len(nodes) != 1 || nodes[0].Title != "alpha" {
		t.Fatalf("compound filter mismatch: %+v", nodes)
	}

	nodes, err = s.QueryNodes(Filter{
		TagsAny: []string{"b", "z"},
		SortBy:  "title",
		Order:   "asc",
	})
	mustNoErr(t, err)
	if len(nodes) != 1 || nodes[0].Title != "gamma" {
		t.Fatalf("tags_any mismatch: %+v", nodes)
	}

	nodes, err = s.QueryNodes(Filter{SortBy: "title", Order: "asc", Offset: 1, Limit: 1})
	mustNoErr(t, err)
	if len(nodes) != 1 || nodes[0].Title != "beta" {
		t.Fatalf("sort/offset mismatch: %+v", nodes)
	}

	nodes, err = s.QueryNodes(Filter{SortBy: "created", Order: "desc", Limit: 1})
	mustNoErr(t, err)
	if len(nodes) != 1 || nodes[0].Title != "beta" {
		t.Fatalf("created desc mismatch: %+v", nodes)
	}

	mustNoErr(t, s.CreateNode(&Node{Frontmatter: Frontmatter{
		Title:        "continuity",
		ContinuedBy:  []NodeID{1},
		SupersededBy: []NodeID{2},
	}}))
	nodes, err = s.QueryNodes(Filter{ContinuedBy: 1})
	mustNoErr(t, err)
	if len(nodes) != 1 || nodes[0].Title != "continuity" {
		t.Fatalf("continued_by filter mismatch: %+v", nodes)
	}
	nodes, err = s.QueryNodes(Filter{SupersededBy: 2})
	mustNoErr(t, err)
	if len(nodes) != 1 || nodes[0].Title != "continuity" {
		t.Fatalf("superseded_by filter mismatch: %+v", nodes)
	}
}

// TestAtomicTagAndParentOperations verifies additive/removal tag and parent APIs.
func TestAtomicTagAndParentOperations(t *testing.T) {
	s := mustInit(t, StorageJSON)
	mustNoErr(t, s.CreateNode(&Node{Frontmatter: Frontmatter{Title: "root"}}))
	mustNoErr(t, s.CreateNode(&Node{Frontmatter: Frontmatter{Title: "alt"}}))
	mustNoErr(t, s.CreateNode(&Node{Frontmatter: Frontmatter{Title: "child", Parents: []NodeID{1}, Tags: []string{"x"}}}))

	mustNoErr(t, s.AddTags(3, "x", "y", "z"))
	mustNoErr(t, s.RemoveTags(3, "x"))
	n, err := s.GetNode(3)
	mustNoErr(t, err)
	if strings.Join(n.Tags, ",") != "y,z" {
		t.Fatalf("unexpected tags after atomic ops: %v", n.Tags)
	}

	mustNoErr(t, s.AddParents(3, 2))
	n, err = s.GetNode(3)
	mustNoErr(t, err)
	if len(n.Parents) != 2 || n.Parents[0] != 1 || n.Parents[1] != 2 {
		t.Fatalf("unexpected parents after add: %v", n.Parents)
	}

	mustNoErr(t, s.RemoveParents(3, 1))
	n, err = s.GetNode(3)
	mustNoErr(t, err)
	if len(n.Parents) != 1 || n.Parents[0] != 2 {
		t.Fatalf("unexpected parents after remove: %v", n.Parents)
	}
}

// TestRemoveArtifact verifies artifact removal by matcher fields.
func TestRemoveArtifact(t *testing.T) {
	s := mustInit(t, StorageJSON)
	mustNoErr(t, s.CreateNode(&Node{Frontmatter: Frontmatter{Title: "artifacts"}}))
	mustNoErr(t, s.AddArtifact(1, Artifact{Mode: ArtifactPath, Host: "local", Path: "/tmp/a", Description: "keep"}))
	mustNoErr(t, s.AddArtifact(1, Artifact{Mode: ArtifactPath, Host: "gpu-node-0", Path: "/tmp/b", Description: "drop"}))
	mustNoErr(t, s.RemoveArtifact(1, Artifact{Mode: ArtifactPath, Host: "gpu-node-0", Path: "/tmp/b"}))
	n, err := s.GetNode(1)
	mustNoErr(t, err)
	if len(n.Artifacts) != 1 || n.Artifacts[0].Host != "local" {
		t.Fatalf("unexpected artifacts after remove: %+v", n.Artifacts)
	}
}

// TestRegenerateEdges verifies edge regeneration from graph.
func TestRegenerateEdges(t *testing.T) {
	s := mustInit(t, StorageJSON)
	a := &Node{Frontmatter: Frontmatter{Title: "a"}}
	mustNoErr(t, s.CreateNode(a))
	b := &Node{Frontmatter: Frontmatter{Title: "b", Parents: []NodeID{a.ID}}}
	mustNoErr(t, s.CreateNode(b))
	mustNoErr(t, os.Remove(s.edgesPath()))
	mustNoErr(t, s.RegenerateEdges())
	data, err := os.ReadFile(s.edgesPath())
	mustNoErr(t, err)
	if !strings.Contains(string(data), `{"from":1,"to":2}`) {
		t.Fatalf("edges missing, got: %s", data)
	}
}

// TestSnapshotsRetentionAndRestore verifies snapshot retention and restore.
func TestSnapshotsRetentionAndRestore(t *testing.T) {
	s := mustInit(t, StorageJSON)
	for i := 0; i < 5; i++ {
		mustNoErr(t, s.CreateNode(&Node{Frontmatter: Frontmatter{Title: "n" + string(rune('a'+i))}}))
	}
	snaps, err := s.ListSnapshots()
	mustNoErr(t, err)
	if len(snaps) > 3 {
		t.Fatalf("expected <=3 snapshots, got %d", len(snaps))
	}
	if len(snaps) == 0 {
		t.Fatalf("expected snapshots")
	}
	// Restore latest should be safe and keep store operable.
	mustNoErr(t, s.RestoreSnapshot(snaps[0].ID))
	_, err = s.ListNodes(Filter{})
	mustNoErr(t, err)
}

// TestMigrateJSONToBINAndBack verifies storage format migration roundtrip.
func TestMigrateJSONToBINAndBack(t *testing.T) {
	s := mustInit(t, StorageJSON)
	n := &Node{Frontmatter: Frontmatter{Title: "node"}}
	mustNoErr(t, s.CreateNode(n))
	mustNoErr(t, s.MigrateStorageFormat(StorageBIN))
	if s.StorageFormat() != StorageBIN {
		t.Fatalf("expected bin format")
	}
	got, err := s.GetNode(n.ID)
	mustNoErr(t, err)
	if got.Title != n.Title {
		t.Fatalf("roundtrip mismatch")
	}
	mustNoErr(t, s.MigrateStorageFormat(StorageJSON))
	if s.StorageFormat() != StorageJSON {
		t.Fatalf("expected json format")
	}
}

// TestLockStaleIsReclaimed verifies stale locks are reclaimed.
func TestLockStaleIsReclaimed(t *testing.T) {
	s := mustInit(t, StorageJSON)
	stale := "pid: 1\nhost: \"h\"\ntimestamp: \"2000-01-01T00:00:00Z\"\noperation: \"x\"\nowner: \"y\"\n"
	mustNoErr(t, os.WriteFile(s.lockPath(), []byte(stale), 0o644))
	mustNoErr(t, s.CreateNode(&Node{Frontmatter: Frontmatter{Title: "ok"}}))
}

// TestInvalidateClaimWarningsAndAck verifies claim invalidation, warnings, and ack.
func TestInvalidateClaimWarningsAndAck(t *testing.T) {
	s := mustInit(t, StorageJSON)
	a := &Node{Frontmatter: Frontmatter{Title: "ancestor", Agent: "researcher", Status: StatusDone}}
	mustNoErr(t, s.CreateNode(a))
	ref := &Node{Frontmatter: Frontmatter{Title: "refuter", Status: StatusDone}}
	mustNoErr(t, s.CreateNode(ref))
	active := &Node{Frontmatter: Frontmatter{Title: "active", Parents: []NodeID{a.ID}, Agent: "opus", Status: StatusActive}}
	mustNoErr(t, s.CreateNode(active))

	mustNoErr(t, s.InvalidateClaim(a.ID, ref.ID, "bad assumption"))
	warnings, err := s.ListBranchWarnings("opus", true)
	mustNoErr(t, err)
	if len(warnings) != 1 {
		t.Fatalf("expected 1 warning, got %d", len(warnings))
	}
	mustNoErr(t, s.AckBranchWarning(warnings[0].ID))
	warnings, err = s.ListBranchWarnings("opus", true)
	mustNoErr(t, err)
	if len(warnings) != 0 {
		t.Fatalf("expected 0 unacked warnings, got %d", len(warnings))
	}
}

// TestInvalidateClaimIdempotentNoDuplicates verifies invalidation is idempotent.
func TestInvalidateClaimIdempotentNoDuplicates(t *testing.T) {
	s := mustInit(t, StorageJSON)
	a := &Node{Frontmatter: Frontmatter{Title: "ancestor", Status: StatusDone}}
	ref := &Node{Frontmatter: Frontmatter{Title: "refuter", Status: StatusDone}}
	child := &Node{Frontmatter: Frontmatter{Title: "active", Status: StatusActive, Agent: "researcher", Parents: []NodeID{1}}}
	mustNoErr(t, s.CreateNode(a))
	mustNoErr(t, s.CreateNode(ref))
	child.Parents = []NodeID{a.ID}
	mustNoErr(t, s.CreateNode(child))

	mustNoErr(t, s.InvalidateClaim(a.ID, ref.ID, "same"))
	mustNoErr(t, s.InvalidateClaim(a.ID, ref.ID, "same"))
	warnings, err := s.ListBranchWarnings("researcher", false)
	mustNoErr(t, err)
	if len(warnings) != 1 {
		t.Fatalf("expected one warning, got %d", len(warnings))
	}
}

// mustInit initializes a test store, failing the test on error.
func mustInit(t *testing.T, format StorageFormat) *Store {
	t.Helper()
	s, err := Init(filepath.Join(t.TempDir(), "research"), format)
	if err != nil {
		t.Fatalf("init: %v", err)
	}
	return s
}

// mustNoErr fails the test if err is non-nil.
func mustNoErr(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
}

// TestFilterJSONRoundtrip verifies that Filter struct JSON tags work
// correctly for bridge/FFI compatibility (snake_case ↔ Go struct).
func TestFilterJSONRoundtrip(t *testing.T) {
	s := mustInit(t, StorageJSON)
	n1 := &Node{Frontmatter: Frontmatter{Title: "alpha bug", Status: StatusActive, Agent: "test", Tags: []string{"kd"}}, Body: "contains flask mention"}
	n2 := &Node{Frontmatter: Frontmatter{Title: "beta done", Status: StatusDone, Agent: "test", Tags: []string{"ml"}}, Body: "contains websocket mention"}
	n3 := &Node{Frontmatter: Frontmatter{Title: "gamma active", Status: StatusActive, Agent: "other"}}
	mustNoErr(t, s.CreateNode(n1))
	mustNoErr(t, s.CreateNode(n2))
	mustNoErr(t, s.CreateNode(n3))

	// Simulate what the bridge does: marshal Filter to JSON, unmarshal back
	f := Filter{Status: StatusActive, TitleContains: "bug"}
	b, err := json.Marshal(f)
	mustNoErr(t, err)

	var f2 Filter
	mustNoErr(t, json.Unmarshal(b, &f2))

	ids, err := s.ListNodes(f2)
	mustNoErr(t, err)
	if len(ids) != 1 || ids[0] != n1.ID {
		t.Fatalf("Filter JSON roundtrip failed: expected [%d], got %v. JSON: %s", n1.ID, ids, b)
	}

	// Snake_case from TypeScript side
	tsJSON := `{"status":"active","title_contains":"bug"}`
	var f3 Filter
	mustNoErr(t, json.Unmarshal([]byte(tsJSON), &f3))
	ids, err = s.ListNodes(f3)
	mustNoErr(t, err)
	if len(ids) != 1 || ids[0] != n1.ID {
		t.Fatalf("Filter snake_case unmarshal failed: expected [%d], got %v", n1.ID, ids)
	}

	// Combined filters in snake_case
	tsJSON2 := `{"status":"active","agent":"test"}`
	var f4 Filter
	mustNoErr(t, json.Unmarshal([]byte(tsJSON2), &f4))
	ids, err = s.ListNodes(f4)
	mustNoErr(t, err)
	if len(ids) != 1 || ids[0] != n1.ID {
		t.Fatalf("Combined filter failed: expected [%d], got %v", n1.ID, ids)
	}

	// Body filter in snake_case
	tsJSON3 := `{"body_contains":"flask"}`
	var f5 Filter
	mustNoErr(t, json.Unmarshal([]byte(tsJSON3), &f5))
	ids, err = s.ListNodes(f5)
	mustNoErr(t, err)
	if len(ids) != 1 || ids[0] != n1.ID {
		t.Fatalf("Body filter failed: expected [%d], got %v", n1.ID, ids)
	}
}

// TestBinaryHistoryRoundtrip verifies that per-node edit history
// uses the binary codec when store format is BIN.
func TestBinaryHistoryRoundtrip(t *testing.T) {
	s := mustInit(t, StorageBIN)
	n := &Node{Frontmatter: Frontmatter{Title: "history test", Status: StatusActive}}
	mustNoErr(t, s.CreateNode(n))

	// Update twice to generate history entries
	n.Status = StatusDone
	mustNoErr(t, s.UpdateNode(n))
	n.Status = StatusDone
	mustNoErr(t, s.UpdateNode(n))

	// Check history
	history, err := s.GetNodeHistory(n.ID)
	mustNoErr(t, err)
	if len(history) == 0 {
		t.Fatal("expected history entries")
	}
	// Verify binary format: files should have .bin extension
	dir := filepath.Join(s.nodeHistoryDir(), fmt.Sprintf("%04d", n.ID))
	entries, err := os.ReadDir(dir)
	mustNoErr(t, err)
	for _, e := range entries {
		if !e.IsDir() && !strings.HasSuffix(e.Name(), ".bin") {
			t.Fatalf("expected .bin extension in binary mode, got %s", e.Name())
		}
	}
	// Verify content roundtrip
	for _, h := range history {
		if h.Title != "history test" {
			t.Fatalf("history title mismatch: %q", h.Title)
		}
	}
}

// TestSnapshotPreservesHistory verifies that snapshots include
// per-node edit history and restore preserves it.
func TestSnapshotPreservesHistory(t *testing.T) {
	s := mustInit(t, StorageJSON)
	n := &Node{Frontmatter: Frontmatter{Title: "snap history test", Status: StatusActive}}
	mustNoErr(t, s.CreateNode(n))

	// Generate history
	n.Status = StatusDone
	mustNoErr(t, s.UpdateNode(n))

	historyBefore, err := s.GetNodeHistory(n.ID)
	mustNoErr(t, err)
	if len(historyBefore) == 0 {
		t.Fatal("expected history before snapshot")
	}

	// Get latest snapshot
	snaps, err := s.ListSnapshots()
	mustNoErr(t, err)
	if len(snaps) == 0 {
		t.Fatal("expected snapshots")
	}

	// Verify snapshot contains history nodes
	snapPath := s.snapshotPath(snaps[0].ID)
	hasHistory := false
	f, err := os.Open(snapPath)
	mustNoErr(t, err)
	defer func() { _ = f.Close() }()
	gz, err := gzip.NewReader(f)
	mustNoErr(t, err)
	defer func() { _ = gz.Close() }()
	tr := tar.NewReader(gz)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		mustNoErr(t, err)
		if strings.Contains(h.Name, "history/nodes") {
			hasHistory = true
			break
		}
	}
	if !hasHistory {
		t.Fatal("snapshot does not contain history/nodes")
	}

	// Restore and verify history preserved
	mustNoErr(t, s.RestoreSnapshot(snaps[0].ID))
	historyAfter, err := s.GetNodeHistory(n.ID)
	mustNoErr(t, err)
	if len(historyAfter) != len(historyBefore) {
		t.Fatalf("history lost after restore: before=%d after=%d", len(historyBefore), len(historyAfter))
	}
}

// TestBinaryModeNoNodesDir verifies that binary mode does not create
// a nodes/ directory.
func TestBinaryModeNoNodesDir(t *testing.T) {
	s := mustInit(t, StorageBIN)
	if _, err := os.Stat(s.nodesDir()); !os.IsNotExist(err) {
		entries, _ := os.ReadDir(s.nodesDir())
		t.Fatalf("expected no nodes/ dir in binary mode, got %v", entries)
	}
	// Create a node and verify it goes to nodes.bin
	n := &Node{Frontmatter: Frontmatter{Title: "bin test"}}
	mustNoErr(t, s.CreateNode(n))
	got, err := s.GetNode(n.ID)
	mustNoErr(t, err)
	if got.Title != "bin test" {
		t.Fatal("roundtrip failed")
	}
}
