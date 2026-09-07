package retree

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"
)

const (
	embedStagingPrefix     = ".embed-"
	embedStagingSuffix     = ".tmp"
	embedTransactionPrefix = ".embed-transaction-"
)

// embedTransaction journals one payload-to-node publication in progress.
type embedTransaction struct {
	NodeID       NodeID `json:"node_id"`
	ArtifactPath string `json:"artifact_path"`
}

// createNode assigns an ID, applies defaults, and persists a new node.
func (s *Store) createNode(n *Node) error {
	if n == nil {
		return fmt.Errorf("%w: nil", ErrInvalidNode)
	}
	return s.withLock("create_node", func() error {
		if err := s.ensureSnapshotCatalogHealthy(); err != nil {
			return err
		}
		g, err := s.loadGraph()
		if err != nil {
			return err
		}
		next, err := s.readNextID()
		if err != nil {
			return err
		}
		n.ID = next
		ApplyNodeDefaults(n, nowUTC())
		if err := g.AddNode(n); err != nil {
			return err
		}
		if err := s.writeNextID(next + 1); err != nil {
			return err
		}
		if err := s.persistGraphDelta(g, map[NodeID]struct{}{n.ID: {}}, nil); !authoritativeCommitSucceeded(err) {
			return err
		}
		s.bestEffortSnapshot("create_node")
		return nil
	})
}

// mutateNodeLocked applies a read-modify-write mutation to the latest node while
// the store lock is held. This is the primitive used by additive helpers such
// as AddTags/AddParents so two agents cannot lose each other's updates between
// an unlocked GetNode and a later UpdateNode.
func (s *Store) mutateNode(id NodeID, operation string, mutate func(*Node) error) error {
	if id == 0 {
		return fmt.Errorf("%w: id required", ErrInvalidNode)
	}
	return s.withLock(operation, func() error {
		if err := s.ensureSnapshotCatalogHealthy(); err != nil {
			return err
		}
		g, err := s.loadGraph()
		if err != nil {
			return err
		}
		existing, err := g.GetNode(id)
		if err != nil {
			return err
		}
		candidate := CloneNode(existing)
		if err := mutate(candidate); err != nil {
			return err
		}
		candidate.Created = existing.Created
		candidate.Modified = nowUTC()
		candidate.Revision = existing.Revision + 1
		ApplyNodeDefaults(candidate, candidate.Created)
		// Validate the complete logical mutation before producing history or
		// touching durable node state.
		if err := g.UpdateNode(id, candidate); err != nil {
			return err
		}
		if err := s.saveNodeHistory(existing); err != nil {
			return err
		}
		if err := s.persistGraphDelta(g, map[NodeID]struct{}{id: {}}, nil); !authoritativeCommitSucceeded(err) {
			return err
		}
		s.bestEffortSnapshot(operation)
		return nil
	})
}

// updateNode persists modifications to an existing node. A non-zero Revision
// is an optimistic concurrency precondition: stale snapshots fail instead of
// silently overwriting newer work. Revision zero remains accepted for legacy
// callers that intentionally perform an unconditional full replacement.
func (s *Store) updateNode(n *Node) error {
	if n == nil || n.ID == 0 {
		return fmt.Errorf("%w: id required", ErrInvalidNode)
	}
	return s.withLock("update_node", func() error {
		if err := s.ensureSnapshotCatalogHealthy(); err != nil {
			return err
		}
		g, err := s.loadGraph()
		if err != nil {
			return err
		}
		existing, err := g.GetNode(n.ID)
		if err != nil {
			return err
		}
		if n.Revision != 0 && n.Revision != existing.Revision {
			return fmt.Errorf("%w: node %d expected revision %d, current revision %d", ErrConflict, n.ID, n.Revision, existing.Revision)
		}
		candidate := CloneNode(n)
		if candidate.Created.IsZero() {
			candidate.Created = existing.Created
		}
		candidate.Modified = nowUTC()
		candidate.Revision = existing.Revision + 1
		ApplyNodeDefaults(candidate, candidate.Created)
		// Graph.UpdateNode is the final semantic validator (parents, relations,
		// cycles). Do it before history or any durable side effect.
		if err := g.UpdateNode(n.ID, candidate); err != nil {
			return err
		}

		// Resource leases are coordination state, not best-effort decoration.
		// For transitions away from active, release them first while retaining
		// an in-memory rollback copy. A crash in this tiny window can only leave
		// an active node without a lease (safe under-allocation), never a done
		// node that still blocks capacity.
		var (
			oldLeases       []ResourceLease
			leaseEvents     []ResourceEvent
			leasesRewritten bool
		)
		if candidate.Status == StatusDone || candidate.Status == StatusPaused {
			oldLeases, err = s.readLeases()
			if err != nil {
				return err
			}
			filtered, events := filterNodeLeases(oldLeases, candidate.ID, func() ResourceEventAction {
				if candidate.Status == StatusPaused {
					return ResourceEventAutoReleasePause
				}
				return ResourceEventAutoReleaseDone
			}())
			leaseEvents = events
			if len(filtered) != len(oldLeases) {
				if err := s.writeLeases(filtered); err != nil {
					return err
				}
				leasesRewritten = true
			}
		}

		if err := s.saveNodeHistory(existing); err != nil {
			if leasesRewritten {
				_ = s.writeLeases(oldLeases)
			}
			return err
		}
		persistErr := s.persistGraphDelta(g, map[NodeID]struct{}{n.ID: {}}, nil)
		if !authoritativeCommitSucceeded(persistErr) {
			if leasesRewritten {
				_ = s.writeLeases(oldLeases)
			}
			return persistErr
		}
		updated := CloneNode(candidate)
		*n = *updated
		for _, event := range leaseEvents {
			_ = s.appendResourceEvent(event)
		}
		s.bestEffortSnapshot("update_node")
		return nil
	})
}

// deleteNode removes a node. Force deletion orphans structural children while
// preserving typed relations as historical/unmoored references by design.
func (s *Store) deleteNode(id NodeID, force bool) error {
	return s.withLock("delete_node", func() error {
		if err := s.ensureSnapshotCatalogHealthy(); err != nil {
			return err
		}
		g, err := s.loadGraph()
		if err != nil {
			return err
		}
		if _, ok := g.Nodes[id]; !ok {
			return ErrNotFound
		}

		// Every child whose authoritative payload is changed by force deletion
		// must be rewritten so parent/primary_parent stay coherent.
		dirty := make(map[NodeID]struct{})
		if force {
			for _, cid := range g.GetChildren(id) {
				dirty[cid] = struct{}{}
			}
		}
		if err := g.RemoveNode(id, force); err != nil {
			return err
		}
		for dirtyID := range dirty {
			if node := g.Nodes[dirtyID]; node != nil {
				if err := ValidateNode(node); err != nil {
					return fmt.Errorf("post-delete node %d: %w", dirtyID, err)
				}
			}
		}
		if err := validateGraphReferentialIntegrity(g); err != nil {
			return err
		}

		oldLeases, err := s.readLeases()
		if err != nil {
			return err
		}
		filteredLeases, leaseEvents := filterNodeLeases(oldLeases, id, ResourceEventAutoReleaseDelete)
		leasesRewritten := len(filteredLeases) != len(oldLeases)
		if leasesRewritten {
			if err := s.writeLeases(filteredLeases); err != nil {
				return err
			}
		}
		persistErr := s.persistGraphDelta(g, dirty, []NodeID{id})
		if !authoritativeCommitSucceeded(persistErr) {
			if leasesRewritten {
				_ = s.writeLeases(oldLeases)
			}
			return persistErr
		}
		for _, event := range leaseEvents {
			_ = s.appendResourceEvent(event)
		}
		s.bestEffortSnapshot("delete_node")
		return nil
	})
}

// filterNodeLeases returns a copy of leases without nodeID and the historical
// events that should be appended after the authoritative mutation commits.
func filterNodeLeases(leases []ResourceLease, nodeID NodeID, action ResourceEventAction) ([]ResourceLease, []ResourceEvent) {
	filtered := make([]ResourceLease, 0, len(leases))
	events := make([]ResourceEvent, 0)
	for _, lease := range leases {
		if lease.NodeID != nodeID {
			filtered = append(filtered, lease)
			continue
		}
		events = append(events, ResourceEvent{
			ResourceID: lease.ResourceID,
			NodeID:     lease.NodeID,
			Action:     action,
			Mode:       lease.Mode,
			ClaimedBy:  lease.ClaimedBy,
			Note:       lease.Note,
			Timestamp:  nowUTC(),
		})
	}
	return filtered, events
}

// migrateStorageFormat converts between json and binary storage formats.
func (s *Store) migrateStorageFormat(target StorageFormat) error {
	if target != StorageJSON && target != StorageBIN {
		return fmt.Errorf("%w: invalid format %q", ErrInvalidNode, target)
	}
	return s.withLock("migrate_storage_format", func() error {
		if target == s.format {
			return nil
		}
		if err := s.ensureSnapshotCatalogHealthy(); err != nil {
			return err
		}
		// Migrate must be transparent on historical stores: legacy done+unset
		// nodes are preserved as-is (repair-outcomes remains the documented
		// follow-up), so load and insert with the legacy-tolerant paths.
		nodes, err := s.loadAllNodesAllowLegacyDoneUnset()
		if err != nil {
			return err
		}
		old := s.format
		s.format = target
		g := NewGraph()
		for _, n := range nodes {
			// A node may legally reference a parent with a higher ID, and
			// nodes are inserted in ascending ID order, so defer the
			// parent-existence check until the whole graph is assembled.
			if err := g.addNodeAssumeValid(n, false); err != nil {
				s.format = old
				return err
			}
		}
		for _, n := range nodes {
			for _, pid := range n.Parents {
				if _, ok := g.Nodes[pid]; !ok {
					s.format = old
					return fmt.Errorf("%w: parent %d not found", ErrInvalidNode, pid)
				}
			}
		}
		if err := s.createSnapshot("migrate_pre"); err != nil {
			s.format = old
			return err
		}
		if err := s.persistGraph(g); !authoritativeCommitSucceeded(err) {
			s.format = old
			return err
		}
		meta, err := s.readMeta()
		if err != nil {
			s.format = old
			return err
		}
		meta.StorageFormat = target
		if err := s.writeMeta(meta); err != nil {
			s.format = old
			return err
		}
		// Clean up old-format artifacts
		if old == StorageJSON {
			_ = os.RemoveAll(s.nodesDir())
		} else {
			_ = os.Remove(s.nodesBinPath())
			_ = os.Remove(s.nodesIdxPath())
			_ = os.Remove(s.binaryGenerationPath())
		}
		s.bestEffortSnapshot("migrate_post")
		return nil
	})
}

// embedArtifact copies a local file into the research root and registers it as
// one locked logical mutation. The payload is staged to a unique temporary file
// and never truncates an existing embedded artifact.
func (s *Store) embedArtifact(id NodeID, localPath string, description string) error {
	finfo, err := os.Stat(localPath)
	if err != nil {
		return err
	}
	if !finfo.Mode().IsRegular() {
		return fmt.Errorf("%w: embedded artifact source must be a regular file", ErrInvalidArtifact)
	}
	return s.withLock("embed_artifact", func() error {
		if err := s.ensureSnapshotCatalogHealthy(); err != nil {
			return err
		}
		g, err := s.loadGraph()
		if err != nil {
			return err
		}
		existing, err := g.GetNode(id)
		if err != nil {
			return err
		}

		dstDir := filepath.Join(s.artifactsDir(), fmt.Sprintf("%04d", id))
		if err := os.MkdirAll(dstDir, 0o755); err != nil {
			return err
		}
		base := filepath.Base(localPath)
		dstPath := filepath.Join(dstDir, base)
		if _, err := os.Stat(dstPath); err == nil {
			return fmt.Errorf("%w: embedded artifact %q already exists", ErrInvalidArtifact, base)
		} else if !os.IsNotExist(err) {
			return err
		}
		tmp, err := os.CreateTemp(dstDir, embedStagingPrefix+"*"+embedStagingSuffix)
		if err != nil {
			return err
		}
		tmpPath := tmp.Name()
		cleanupTmp := true
		defer func() {
			_ = tmp.Close()
			if cleanupTmp {
				_ = os.Remove(tmpPath)
			}
		}()
		src, err := os.Open(localPath)
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(tmp, src)
		closeSrcErr := src.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeSrcErr != nil {
			return closeSrcErr
		}
		if err := tmp.Sync(); err != nil {
			return err
		}
		if err := tmp.Close(); err != nil {
			return err
		}

		artifact := Artifact{
			Mode:        ArtifactEmbedded,
			Path:        filepath.ToSlash(filepath.Join("artifacts", fmt.Sprintf("%04d", id), base)),
			Description: description,
			SizeBytes:   finfo.Size(),
		}
		if err := ValidateArtifact(artifact); err != nil {
			return err
		}
		candidate := CloneNode(existing)
		candidate.Artifacts = append(candidate.Artifacts, artifact)
		candidate.Modified = nowUTC()
		candidate.Revision = existing.Revision + 1
		ApplyNodeDefaults(candidate, candidate.Created)
		if err := g.UpdateNode(id, candidate); err != nil {
			return err
		}
		if err := s.saveNodeHistory(existing); err != nil {
			return err
		}
		txnPath, err := s.writeEmbedTransaction(embedTransaction{NodeID: id, ArtifactPath: artifact.Path})
		if err != nil {
			return err
		}
		clearTransaction := false
		defer func() {
			if clearTransaction {
				_ = os.Remove(txnPath)
			}
		}()
		if err := os.Rename(tmpPath, dstPath); err != nil {
			clearTransaction = true
			return err
		}
		cleanupTmp = false
		if err := s.persistGraphDelta(g, map[NodeID]struct{}{id: {}}, nil); !authoritativeCommitSucceeded(err) {
			if removeErr := os.Remove(dstPath); removeErr == nil || os.IsNotExist(removeErr) {
				clearTransaction = true
			}
			return err
		}
		clearTransaction = true
		s.bestEffortSnapshot("embed_artifact")
		return nil
	})
}

// writeEmbedTransaction records enough information to reconcile a crash
// between publishing an embedded payload and publishing its node metadata.
func (s *Store) writeEmbedTransaction(txn embedTransaction) (string, error) {
	token, err := newLockToken()
	if err != nil {
		return "", err
	}
	b, err := json.Marshal(txn)
	if err != nil {
		return "", err
	}
	path := filepath.Join(s.rootPath, embedTransactionPrefix+token+".json")
	tmp, err := os.CreateTemp(s.rootPath, embedTransactionPrefix+"*.json.tmp")
	if err != nil {
		return "", err
	}
	tmpPath := tmp.Name()
	cleanup := true
	defer func() {
		_ = tmp.Close()
		if cleanup {
			_ = os.Remove(tmpPath)
		}
	}()
	if err := tmp.Chmod(0o644); err != nil {
		return "", err
	}
	if _, err := tmp.Write(append(b, '\n')); err != nil {
		return "", err
	}
	if err := tmp.Sync(); err != nil {
		return "", err
	}
	if err := tmp.Close(); err != nil {
		return "", err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return "", err
	}
	cleanup = false
	return path, nil
}

// reconcileArtifactTransactionsLocked removes orphaned payloads left by an
// interrupted embed, or only the journal when node metadata already committed.
// The caller must hold the store lock.
func (s *Store) reconcileArtifactTransactionsLocked(g *Graph) error {
	if err := s.cleanupArtifactStagingFilesLocked(g); err != nil {
		return err
	}
	entries, err := os.ReadDir(s.rootPath)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasPrefix(entry.Name(), embedTransactionPrefix) && strings.HasSuffix(entry.Name(), ".json.tmp") {
			if err := os.Remove(filepath.Join(s.rootPath, entry.Name())); err != nil && !os.IsNotExist(err) {
				return err
			}
			continue
		}
		if entry.IsDir() || !strings.HasPrefix(entry.Name(), embedTransactionPrefix) || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		journalPath := filepath.Join(s.rootPath, entry.Name())
		b, err := os.ReadFile(journalPath)
		if err != nil {
			return err
		}
		var txn embedTransaction
		if err := decodeJSONStrict(b, &txn); err != nil {
			return fmt.Errorf("invalid embed transaction %s: %w", entry.Name(), err)
		}
		cleanArtifactPath := filepath.ToSlash(filepath.Clean(filepath.FromSlash(txn.ArtifactPath)))
		expectedPrefix := filepath.ToSlash(filepath.Join("artifacts", fmt.Sprintf("%04d", txn.NodeID))) + "/"
		if txn.NodeID == 0 || cleanArtifactPath != txn.ArtifactPath || !strings.HasPrefix(cleanArtifactPath, expectedPrefix) || pathEscapesRoot(cleanArtifactPath) {
			return fmt.Errorf("invalid embed transaction %s: unsafe artifact path", entry.Name())
		}
		committed := false
		if node := g.Nodes[txn.NodeID]; node != nil {
			committed = slices.ContainsFunc(node.Artifacts, func(artifact Artifact) bool {
				return artifact.Mode == ArtifactEmbedded && artifact.Path == txn.ArtifactPath
			})
		}
		if !committed {
			payloadPath := filepath.Join(s.rootPath, filepath.FromSlash(cleanArtifactPath))
			if err := os.Remove(payloadPath); err != nil && !os.IsNotExist(err) {
				return err
			}
		}
		if err := os.Remove(journalPath); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

// cleanupArtifactStagingFilesLocked removes payload staging files left by a
// process that died before publishing an embed transaction. The caller must
// hold the store lock.
func (s *Store) cleanupArtifactStagingFilesLocked(g *Graph) error {
	// Published filenames are user-controlled and can resemble staging names.
	// Protect every registered path, including references from other nodes.
	registered := make(map[string]struct{})
	for _, node := range g.Nodes {
		for _, artifact := range node.Artifacts {
			registered[filepath.Clean(filepath.Join(s.rootPath, filepath.FromSlash(artifact.Path)))] = struct{}{}
		}
	}
	return filepath.WalkDir(s.artifactsDir(), func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			if os.IsNotExist(walkErr) {
				return nil
			}
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		if _, ok := registered[filepath.Clean(path)]; ok {
			return nil
		}
		name := entry.Name()
		if strings.HasPrefix(name, embedStagingPrefix) && strings.HasSuffix(name, embedStagingSuffix) {
			if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
				return err
			}
		}
		return nil
	})
}

// pathEscapesRoot rejects absolute and parent-traversing persisted paths.
func pathEscapesRoot(path string) bool {
	clean := filepath.Clean(filepath.FromSlash(path))
	return filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator))
}

// copyFile copies a file from src to dst.
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer func() { _ = out.Close() }()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Sync()
}

// resolveAgentName looks up the human-readable name for an agent ID.
func (s *Store) resolveAgentName(id string) (string, error) {
	b, err := os.ReadFile(s.agentsPath())
	if err != nil {
		return "", err
	}
	var payload map[string]map[string]string
	if err := json.Unmarshal(b, &payload); err != nil {
		return "", err
	}
	if v, ok := payload[id]; ok {
		return v["name"], nil
	}
	if v, ok := payload["agent."+id]; ok {
		return v["name"], nil
	}
	return "", nil
}

// invalidateClaim marks a claim as invalidated and propagates warnings.
func (s *Store) invalidateClaim(target NodeID, refuter NodeID, reason string) error {
	if reason == "" {
		reason = "invalidated"
	}
	return s.withLock("invalidate_claim", func() error {
		if err := s.ensureSnapshotCatalogHealthy(); err != nil {
			return err
		}
		g, err := s.loadGraph()
		if err != nil {
			return err
		}
		tn, ok := g.Nodes[target]
		if !ok {
			return ErrNotFound
		}
		if _, ok := g.Nodes[refuter]; !ok {
			return fmt.Errorf("%w: refuter not found", ErrInvalidNode)
		}
		// idempotent behavior for same tuple.
		for _, rid := range tn.InvalidatedBy {
			if rid == refuter && tn.ClaimStatus == ClaimInvalidated && tn.InvalidationReason == reason {
				return nil
			}
		}
		tn.ClaimStatus = ClaimInvalidated
		tn.InvalidatedBy = uniqueSortedIDs(append(tn.InvalidatedBy, refuter))
		tn.InvalidationReason = reason
		tn.Modified = nowUTC()
		if err := g.UpdateNode(target, tn); err != nil {
			return err
		}
		if _, err := s.listBranchWarnings("", false); err != nil {
			return err
		}
		if err := s.persistGraphDelta(g, map[NodeID]struct{}{target: {}}, nil); !authoritativeCommitSucceeded(err) {
			return err
		}
		s.bestEffortInvalidationWarnings(g, target)
		s.bestEffortSnapshot("invalidate_claim")
		return nil
	})
}

// generateWarningsForInvalidation creates branch warnings for active descendants.
func (s *Store) generateWarningsForInvalidation(g *Graph, rootCause NodeID) error {
	existing, err := s.listBranchWarnings("", false)
	if err != nil {
		return err
	}
	openKey := map[string]struct{}{}
	for _, w := range existing {
		if w.AckedAt != nil {
			continue
		}
		k := fmt.Sprintf("%d:%d:%s", w.RootCauseNode, w.ImpactedNode, w.Agent)
		openKey[k] = struct{}{}
	}
	impacted := g.GetAtRiskDescendants(rootCause)
	sort.Slice(impacted, func(i, j int) bool { return impacted[i] < impacted[j] })
	for _, id := range impacted {
		n := g.Nodes[id]
		agent := n.Agent
		if agent == "" {
			agent = "unassigned"
		}
		k := fmt.Sprintf("%d:%d:%s", rootCause, id, agent)
		if _, ok := openKey[k]; ok {
			continue
		}
		w := BranchWarning{
			// Nanosecond precision plus both endpoints keeps IDs unique even when
			// two different root causes hit the same impacted node in one second.
			ID:            fmt.Sprintf("warn_%d_%d_%04d", time.Now().UnixNano(), rootCause, id),
			Agent:         agent,
			RootCauseNode: rootCause,
			ImpactedNode:  id,
			Severity:      "warning",
			Message:       fmt.Sprintf("ancestor %04d invalidated impacts active node %04d", rootCause, id),
			CreatedAt:     nowUTC(),
		}
		if err := appendJSONLine(s.alertsPath(), w); err != nil {
			return err
		}
		openKey[k] = struct{}{}
	}
	return nil
}

// bestEffortInvalidationWarnings appends descendant warnings after the
// invalidation itself is durable, without turning a late sidecar failure into
// a false rollback signal for the mutation.
func (s *Store) bestEffortInvalidationWarnings(g *Graph, rootCause NodeID) {
	_ = s.generateWarningsForInvalidation(g, rootCause)
}
