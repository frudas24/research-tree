package retree

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"testing"
)

// TestConcurrentAddTagsDoesNotLoseUpdate verifies locked read-modify-write.
func TestConcurrentAddTagsDoesNotLoseUpdate(t *testing.T) {
	s := mustInit(t, StorageJSON)
	n := &Node{Frontmatter: Frontmatter{Title: "concurrent tags"}}
	mustNoErr(t, s.CreateNode(n))

	var wg sync.WaitGroup
	errCh := make(chan error, 2)
	for _, tag := range []string{"agent-a", "agent-b"} {
		tag := tag
		wg.Add(1)
		go func() {
			defer wg.Done()
			errCh <- s.AddTags(n.ID, tag)
		}()
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		mustNoErr(t, err)
	}

	got, err := s.GetNode(n.ID)
	mustNoErr(t, err)
	for _, tag := range []string{"agent-a", "agent-b"} {
		if !slices.Contains(got.Tags, tag) {
			t.Fatalf("lost concurrent tag %q: %v", tag, got.Tags)
		}
	}
}

// TestUpdateNodeRejectsStaleRevisionAndRefreshesSuccessfulCaller verifies CAS semantics.
func TestUpdateNodeRejectsStaleRevisionAndRefreshesSuccessfulCaller(t *testing.T) {
	s := mustInit(t, StorageJSON)
	n := &Node{Frontmatter: Frontmatter{Title: "revision"}}
	mustNoErr(t, s.CreateNode(n))

	a, err := s.GetNode(n.ID)
	mustNoErr(t, err)
	b, err := s.GetNode(n.ID)
	mustNoErr(t, err)

	a.Body = "first"
	mustNoErr(t, s.UpdateNode(a))
	if a.Revision != 2 {
		t.Fatalf("successful caller revision not refreshed: got %d want 2", a.Revision)
	}
	b.Body = "stale"
	if err := s.UpdateNode(b); !errors.Is(err, ErrConflict) {
		t.Fatalf("expected ErrConflict for stale revision, got %v", err)
	}
	got, err := s.GetNode(n.ID)
	mustNoErr(t, err)
	if got.Body != "first" {
		t.Fatalf("stale update overwrote newer state: %q", got.Body)
	}
}

// TestForceDeleteClearsPrimaryParent verifies forced orphaning remains valid.
func TestForceDeleteClearsPrimaryParent(t *testing.T) {
	s := mustInit(t, StorageJSON)
	parent := &Node{Frontmatter: Frontmatter{Title: "parent"}}
	mustNoErr(t, s.CreateNode(parent))
	pp := parent.ID
	child := &Node{Frontmatter: Frontmatter{Title: "child", Parents: []NodeID{parent.ID}}}
	child.PrimaryParent = &pp
	mustNoErr(t, s.CreateNode(child))

	mustNoErr(t, s.DeleteNode(parent.ID, true))
	got, err := s.GetNode(child.ID)
	mustNoErr(t, err)
	if len(got.Parents) != 0 || got.PrimaryParent != nil {
		t.Fatalf("forced delete left invalid primary parent: parents=%v primary=%v", got.Parents, got.PrimaryParent)
	}
	if _, err := s.QueryNodes(Filter{}); err != nil {
		t.Fatalf("store must audit after forced delete: %v", err)
	}
}

// TestRelationsAllowHistoricalOrForwardTargetsButIndexMustMatchExactly preserves history safely.
func TestRelationsAllowHistoricalOrForwardTargetsButIndexMustMatchExactly(t *testing.T) {
	s := mustInit(t, StorageJSON)
	n := &Node{Frontmatter: Frontmatter{Title: "forward relation"}}
	n.Relations = []Relation{{Type: RelInspiredBy, Target: 9999, Note: "historical/forward"}}
	mustNoErr(t, s.CreateNode(n))
	if _, err := s.QueryNodes(Filter{}); err != nil {
		t.Fatalf("forward/historical relation target should remain valid: %v", err)
	}

	// Existing-but-incomplete derived indexes are corruption, not an empty set.
	mustNoErr(t, os.WriteFile(s.relationsPath(), nil, 0o644))
	if _, err := s.QueryNodes(Filter{}); err == nil {
		t.Fatal("expected audit failure when relations index silently drops a relation")
	}
}

// TestMissingRelationsIndexIsRebuiltFromNodes verifies deterministic repair.
func TestMissingRelationsIndexIsRebuiltFromNodes(t *testing.T) {
	s := mustInit(t, StorageJSON)
	n := &Node{Frontmatter: Frontmatter{Title: "relation source"}}
	n.Relations = []Relation{{Type: RelDependsOn, Target: 42}}
	mustNoErr(t, s.CreateNode(n))
	mustNoErr(t, os.Remove(s.relationsPath()))

	if _, err := s.QueryNodes(Filter{}); err != nil {
		t.Fatalf("missing rebuildable relations index should repair: %v", err)
	}
	rels, err := s.ListRelations(n.ID)
	mustNoErr(t, err)
	if len(rels) != 1 || rels[0].Target != 42 {
		t.Fatalf("relations repair lost authoritative relation: %+v", rels)
	}
}

// TestNodeJSONRejectsUnknownFields verifies persisted nodes fail closed.
func TestNodeJSONRejectsUnknownFields(t *testing.T) {
	if _, err := UnmarshalNodeJSON([]byte(`{"title":"x","unknown_field":true}`)); err == nil {
		t.Fatal("unknown node JSON field must fail closed")
	}
}

// TestOpenRecoversInterruptedBinaryPair verifies crash recovery on Open.
func TestOpenRecoversInterruptedBinaryPair(t *testing.T) {
	root := filepath.Join(t.TempDir(), "tree")
	s, err := Init(root, StorageBIN)
	mustNoErr(t, err)
	n := &Node{Frontmatter: Frontmatter{Title: "binary recovery"}}
	mustNoErr(t, s.CreateNode(n))

	mustNoErr(t, os.Remove(s.nodesIdxPath()))
	mustNoErr(t, os.WriteFile(s.binaryDirtyPath(), []byte("interrupted\n"), 0o644))

	reopened, err := Open(root)
	mustNoErr(t, err)
	got, err := reopened.GetNode(n.ID)
	mustNoErr(t, err)
	if got.Title != n.Title {
		t.Fatalf("binary recovery returned wrong node: %+v", got)
	}
	if _, err := os.Stat(reopened.binaryDirtyPath()); !os.IsNotExist(err) {
		t.Fatalf("binary dirty marker should be cleared after recovery: %v", err)
	}
}

// TestEmbedArtifactNeverOverwritesExistingPayload verifies collision safety.
func TestEmbedArtifactNeverOverwritesExistingPayload(t *testing.T) {
	s := mustInit(t, StorageJSON)
	n := &Node{Frontmatter: Frontmatter{Title: "artifact"}}
	mustNoErr(t, s.CreateNode(n))

	d1 := t.TempDir()
	d2 := t.TempDir()
	p1 := filepath.Join(d1, "result.bin")
	p2 := filepath.Join(d2, "result.bin")
	mustNoErr(t, os.WriteFile(p1, []byte("first"), 0o644))
	mustNoErr(t, os.WriteFile(p2, []byte("second"), 0o644))
	mustNoErr(t, s.EmbedArtifact(n.ID, p1, "first"))
	if err := s.EmbedArtifact(n.ID, p2, "second"); err == nil {
		t.Fatal("expected duplicate embedded artifact basename to fail instead of overwrite")
	}
	stored := filepath.Join(s.artifactsDir(), "0001", "result.bin")
	b, err := os.ReadFile(stored)
	mustNoErr(t, err)
	if string(b) != "first" {
		t.Fatalf("existing embedded artifact was overwritten: %q", b)
	}
}

// TestStableBinaryReadRetriesAcrossGenerationChange verifies the seqlock does
// not return a read that overlapped a complete writer publication.
func TestStableBinaryReadRetriesAcrossGenerationChange(t *testing.T) {
	s := mustInit(t, StorageBIN)
	n := &Node{Frontmatter: Frontmatter{Title: "generation"}}
	mustNoErr(t, s.CreateNode(n))

	calls := 0
	got, err := withStableBinaryRead(s, func() (NodeID, error) {
		calls++
		if calls == 1 {
			mustNoErr(t, s.writeBinaryGeneration("overlapping-writer"))
		}
		return n.ID, nil
	})
	mustNoErr(t, err)
	if got != n.ID || calls != 2 {
		t.Fatalf("unstable generation was not retried: id=%d calls=%d", got, calls)
	}
}

// TestBinaryIndexPublishFailureIsRecoverableCommit verifies a failure after
// nodes.bin publication is classified as committed and recoverable in-place.
func TestBinaryIndexPublishFailureIsRecoverableCommit(t *testing.T) {
	s := mustInit(t, StorageBIN)
	first := &Node{Frontmatter: Frontmatter{Title: "first"}}
	mustNoErr(t, s.CreateNode(first))
	second := &Node{Frontmatter: Frontmatter{ID: 2, Title: "second"}}
	ApplyNodeDefaults(second, nowUTC())

	mustNoErr(t, os.Remove(s.nodesIdxPath()))
	mustNoErr(t, os.Mkdir(s.nodesIdxPath(), 0o755))
	err := s.writeAllNodesBIN([]*Node{first, second})
	if !errors.Is(err, ErrDerivedState) {
		t.Fatalf("post-BIN-commit index failure must be recoverable, got %v", err)
	}
	if !authoritativeCommitSucceeded(err) {
		t.Fatal("post-BIN-commit error was misclassified as pre-commit")
	}
	mustNoErr(t, os.Remove(s.nodesIdxPath()))
	mustNoErr(t, s.withLock("recover_probe", func() error { return nil }))
	got, err := s.GetNode(second.ID)
	mustNoErr(t, err)
	if got.Title != second.Title {
		t.Fatalf("recovered wrong binary node: %+v", got)
	}
}

// TestBinaryGenerationFailureRecoversImmediately verifies a final generation
// write failure does not leave the current Store instance blocked by dirty state.
func TestBinaryGenerationFailureRecoversImmediately(t *testing.T) {
	s := mustInit(t, StorageBIN)
	first := &Node{Frontmatter: Frontmatter{Title: "first"}}
	mustNoErr(t, s.CreateNode(first))
	second := &Node{Frontmatter: Frontmatter{ID: 2, Title: "second"}}
	ApplyNodeDefaults(second, nowUTC())

	calls := 0
	err := s.writeAllNodesBINWithGenerationWriter([]*Node{first, second}, func(string) error {
		calls++
		return errors.New("injected generation failure")
	})
	if err != nil {
		t.Fatalf("recoverable generation failure returned error: %v", err)
	}
	if calls != 1 {
		t.Fatalf("unexpected injected generation writes: %d", calls)
	}
	if _, err := os.Stat(s.binaryDirtyPath()); !os.IsNotExist(err) {
		t.Fatalf("dirty marker survived immediate recovery: %v", err)
	}
	got, err := s.GetNode(second.ID)
	mustNoErr(t, err)
	if got.Title != second.Title {
		t.Fatalf("generation recovery returned wrong node: %+v", got)
	}
}

// TestConcurrentBinaryReadersObserveCompleteGenerations stress-tests lock-free
// reads while a writer repeatedly publishes complete BIN/IDX generations.
func TestConcurrentBinaryReadersObserveCompleteGenerations(t *testing.T) {
	s := mustInit(t, StorageBIN)
	current := &Node{Frontmatter: Frontmatter{Title: "stress"}}
	mustNoErr(t, s.CreateNode(current))
	nodeID := current.ID

	const readers = 8
	stop := make(chan struct{})
	errCh := make(chan error, readers)
	var wg sync.WaitGroup
	for range readers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				node, err := s.GetNode(nodeID)
				if err != nil {
					errCh <- err
					return
				}
				if node.ID != nodeID || node.Title != "stress" {
					errCh <- fmt.Errorf("mixed binary generation: %+v", node)
					return
				}
			}
		}()
	}
	for i := 0; i < 100; i++ {
		next := CloneNode(current)
		next.Body = fmt.Sprintf("generation-%d", i)
		next.Revision++
		next.Modified = nowUTC()
		mustNoErr(t, s.withLock("binary_stress", func() error {
			return s.writeAllNodesBIN([]*Node{next})
		}))
		current = next
	}
	close(stop)
	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Fatalf("concurrent binary read: %v", err)
	}
}

// TestFeatureCurrentNodeMustBeLinked rejects an existing but unrelated node.
func TestFeatureCurrentNodeMustBeLinked(t *testing.T) {
	s := mustInit(t, StorageJSON)
	linked := &Node{Frontmatter: Frontmatter{Title: "linked"}}
	unlinked := &Node{Frontmatter: Frontmatter{Title: "unlinked"}}
	mustNoErr(t, s.CreateNode(linked))
	mustNoErr(t, s.CreateNode(unlinked))
	feature, err := s.CreateFeature("current invariant", linked.ID)
	mustNoErr(t, err)
	mustNoErr(t, s.LinkNodeToFeature(feature.ID, linked.ID, RoleImplementation))
	if err := s.SetFeatureCurrentNode(feature.ID, unlinked.ID); err == nil {
		t.Fatal("unlinked node was accepted as feature current_node")
	}
}

// TestOpenReconcilesInterruptedEmbed removes an orphan published before node metadata.
func TestOpenReconcilesInterruptedEmbed(t *testing.T) {
	s := mustInit(t, StorageJSON)
	n := &Node{Frontmatter: Frontmatter{Title: "embed owner"}}
	mustNoErr(t, s.CreateNode(n))
	dir := filepath.Join(s.artifactsDir(), "0001")
	mustNoErr(t, os.MkdirAll(dir, 0o755))
	payload := filepath.Join(dir, "orphan.bin")
	mustNoErr(t, os.WriteFile(payload, []byte("orphan"), 0o644))
	journal, err := s.writeEmbedTransaction(embedTransaction{NodeID: n.ID, ArtifactPath: "artifacts/0001/orphan.bin"})
	mustNoErr(t, err)

	_, err = Open(s.rootPath)
	mustNoErr(t, err)
	if _, err := os.Stat(payload); !os.IsNotExist(err) {
		t.Fatalf("orphan payload survived reconciliation: %v", err)
	}
	if _, err := os.Stat(journal); !os.IsNotExist(err) {
		t.Fatalf("embed journal survived reconciliation: %v", err)
	}
}

// TestEmbedJournalIsAtomicAndOpenRemovesStaging verifies only complete
// journals become visible and crash-left payload staging files are reclaimed.
func TestEmbedJournalIsAtomicAndOpenRemovesStaging(t *testing.T) {
	s := mustInit(t, StorageJSON)
	n := &Node{Frontmatter: Frontmatter{Title: "embed staging"}}
	mustNoErr(t, s.CreateNode(n))
	dir := filepath.Join(s.artifactsDir(), "0001")
	mustNoErr(t, os.MkdirAll(dir, 0o755))
	payloadTmp := filepath.Join(dir, ".embed-orphan.tmp")
	mustNoErr(t, os.WriteFile(payloadTmp, []byte("partial"), 0o644))
	journalTmp := filepath.Join(s.rootPath, embedTransactionPrefix+"orphan.json.tmp")
	mustNoErr(t, os.WriteFile(journalTmp, []byte("{"), 0o644))

	journal, err := s.writeEmbedTransaction(embedTransaction{NodeID: n.ID, ArtifactPath: "artifacts/0001/final.bin"})
	mustNoErr(t, err)
	b, err := os.ReadFile(journal)
	mustNoErr(t, err)
	var txn embedTransaction
	if err := decodeJSONStrict(b, &txn); err != nil {
		t.Fatalf("published journal is incomplete: %v", err)
	}

	_, err = Open(s.rootPath)
	mustNoErr(t, err)
	for _, path := range []string{payloadTmp, journalTmp, journal} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("staging or journal file survived reconciliation at %s: %v", path, err)
		}
	}
}
