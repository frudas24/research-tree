package retree

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// createNode assigns an ID, applies defaults, and persists a new node.
func (s *Store) createNode(n *Node) error {
	if n == nil {
		return fmt.Errorf("%w: nil", ErrInvalidNode)
	}
	return s.withLock("create_node", func() error {
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
		if err := s.persistGraph(g); err != nil {
			return err
		}
		return s.createSnapshot("create_node")
	})
}

// updateNode persists modifications to an existing node.
func (s *Store) updateNode(n *Node) error {
	if n == nil || n.ID == 0 {
		return fmt.Errorf("%w: id required", ErrInvalidNode)
	}
	return s.withLock("update_node", func() error {
		g, err := s.loadGraph()
		if err != nil {
			return err
		}
		existing, err := g.GetNode(n.ID)
		if err != nil {
			return err
		}
		candidate := CloneNode(n)
		if candidate.Created.IsZero() {
			candidate.Created = existing.Created
		}
		candidate.Modified = nowUTC()
		candidate.Revision = existing.Revision + 1
		ApplyNodeDefaults(candidate, candidate.Created)
		if err := s.saveNodeHistory(existing); err != nil {
			return err
		}
		if err := g.UpdateNode(n.ID, candidate); err != nil {
			return err
		}
		if err := s.persistGraph(g); err != nil {
			return err
		}
		if candidate.Status == StatusDone || candidate.Status == StatusPaused {
			action := ResourceEventAutoReleaseDone
			if candidate.Status == StatusPaused {
				action = ResourceEventAutoReleasePause
			}
			if err := s.releaseNodeResourcesUnlocked(candidate.ID, action); err != nil {
				return err
			}
		}
		return s.createSnapshot("update_node")
	})
}

// deleteNode removes a node, optionally forcing orphan of children.
func (s *Store) deleteNode(id NodeID, force bool) error {
	return s.withLock("delete_node", func() error {
		g, err := s.loadGraph()
		if err != nil {
			return err
		}
		if err := g.RemoveNode(id, force); err != nil {
			return err
		}
		if err := s.persistGraph(g); err != nil {
			return err
		}
		if err := s.releaseNodeResourcesUnlocked(id, ResourceEventAutoReleaseDelete); err != nil {
			return err
		}
		return s.createSnapshot("delete_node")
	})
}

// migrateStorageFormat converts between json and binary storage formats.
func (s *Store) migrateStorageFormat(target StorageFormat) error {
	if target != StorageJSON && target != StorageBIN {
		return fmt.Errorf("%w: invalid format %q", ErrInvalidNode, target)
	}
	if target == s.format {
		return nil
	}
	return s.withLock("migrate_storage_format", func() error {
		nodes, err := s.loadAllNodes()
		if err != nil {
			return err
		}
		old := s.format
		s.format = target
		g := NewGraph()
		for _, n := range nodes {
			if err := g.AddNode(n); err != nil {
				s.format = old
				return err
			}
		}
		if err := s.createSnapshot("migrate_pre"); err != nil {
			s.format = old
			return err
		}
		if err := s.persistGraph(g); err != nil {
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
		}
		return s.createSnapshot("migrate_post")
	})
}

// embedArtifact copies a local file into the research root and registers it.
func (s *Store) embedArtifact(id NodeID, localPath string, description string) error {
	finfo, err := os.Stat(localPath)
	if err != nil {
		return err
	}
	dstDir := filepath.Join(s.artifactsDir(), fmt.Sprintf("%04d", id))
	if err := os.MkdirAll(dstDir, 0o755); err != nil {
		return err
	}
	base := filepath.Base(localPath)
	dstPath := filepath.Join(dstDir, base)
	if err := copyFile(localPath, dstPath); err != nil {
		return err
	}
	artifact := Artifact{
		Mode:        ArtifactEmbedded,
		Path:        filepath.ToSlash(filepath.Join("artifacts", fmt.Sprintf("%04d", id), base)),
		Description: description,
		SizeBytes:   finfo.Size(),
	}
	return s.AddArtifact(id, artifact)
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
		if err := s.persistGraph(g); err != nil {
			return err
		}
		if err := s.generateWarningsForInvalidation(g, target); err != nil {
			return err
		}
		return s.createSnapshot("invalidate_claim")
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
			ID:            fmt.Sprintf("warn_%d_%04d", time.Now().Unix(), id),
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
