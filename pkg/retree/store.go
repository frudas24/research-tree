package retree

import "time"

// Store is the primary interface to a research-tree root on disk.
// It provides CRUD for nodes, graph queries, filtering, artifact management, and tagging.
type Store struct {
	rootPath string
	format   StorageFormat
}

// Open opens an existing research-root at rootPath.
func Open(rootPath string) (*Store, error) {
	return openStore(rootPath)
}

// Init creates a new research-root at rootPath.
func Init(rootPath string, format StorageFormat) (*Store, error) {
	return initStore(rootPath, format)
}

// CreateNode assigns an ID and writes a new node to disk.
func (s *Store) CreateNode(n *Node) error {
	return s.createNode(n)
}

// GetNode retrieves a node by ID.
func (s *Store) GetNode(id NodeID) (*Node, error) {
	return s.getNode(id)
}

// UpdateNode overwrites an existing node.
func (s *Store) UpdateNode(n *Node) error {
	return s.updateNode(n)
}

// DeleteNode removes a node. If force is false and the node has children, it fails.
func (s *Store) DeleteNode(id NodeID, force bool) error {
	return s.deleteNode(id, force)
}

// GetChildren returns the direct children of a node.
func (s *Store) GetChildren(id NodeID) ([]NodeID, error) {
	g, err := s.loadGraph()
	if err != nil {
		return nil, err
	}
	if _, ok := g.Nodes[id]; !ok {
		return nil, ErrNotFound
	}
	return g.GetChildren(id), nil
}

// GetParents returns the direct parents of a node.
func (s *Store) GetParents(id NodeID) ([]NodeID, error) {
	g, err := s.loadGraph()
	if err != nil {
		return nil, err
	}
	if _, ok := g.Nodes[id]; !ok {
		return nil, ErrNotFound
	}
	return g.GetParents(id), nil
}

// GetAncestors returns all ancestors (DFS upward).
func (s *Store) GetAncestors(id NodeID) ([]NodeID, error) {
	g, err := s.loadGraph()
	if err != nil {
		return nil, err
	}
	if _, ok := g.Nodes[id]; !ok {
		return nil, ErrNotFound
	}
	return g.GetAncestors(id), nil
}

// GetDescendants returns all descendants (DFS downward).
func (s *Store) GetDescendants(id NodeID) ([]NodeID, error) {
	g, err := s.loadGraph()
	if err != nil {
		return nil, err
	}
	if _, ok := g.Nodes[id]; !ok {
		return nil, ErrNotFound
	}
	return g.GetDescendants(id), nil
}

// GetRoots returns nodes with no parents.
func (s *Store) GetRoots() ([]NodeID, error) {
	g, err := s.loadGraph()
	if err != nil {
		return nil, err
	}
	return g.GetRoots(), nil
}

// GetLeaves returns nodes with no children.
func (s *Store) GetLeaves() ([]NodeID, error) {
	g, err := s.loadGraph()
	if err != nil {
		return nil, err
	}
	out := make([]NodeID, 0)
	for id := range g.Nodes {
		if len(g.GetChildren(id)) == 0 {
			out = append(out, id)
		}
	}
	return uniqueSortedIDs(out), nil
}

// ListNodes returns node IDs matching the filter.
func (s *Store) ListNodes(f Filter) ([]NodeID, error) {
	nodes, err := s.QueryNodes(f)
	if err != nil {
		return nil, err
	}
	ids := make([]NodeID, 0, len(nodes))
	for _, n := range nodes {
		ids = append(ids, n.ID)
	}
	return ids, nil
}

// QueryNodes returns full nodes matching the filter.
func (s *Store) QueryNodes(f Filter) ([]*Node, error) {
	all, err := s.loadAllNodes()
	if err != nil {
		return nil, err
	}
	filtered := make([]*Node, 0, len(all))
	for _, n := range all {
		if !matchesFilter(n, f) {
			continue
		}
		filtered = append(filtered, CloneNode(n))
	}
	sortNodes(filtered, f.SortBy, f.Order)
	if f.Offset > 0 {
		if f.Offset >= len(filtered) {
			return []*Node{}, nil
		}
		filtered = filtered[f.Offset:]
	}
	if f.Limit > 0 && len(filtered) > f.Limit {
		filtered = filtered[:f.Limit]
	}
	return filtered, nil
}

// AddArtifact registers an artifact reference on a node.
func (s *Store) AddArtifact(id NodeID, a Artifact) error {
	if err := ValidateArtifact(a); err != nil {
		return err
	}
	n, err := s.GetNode(id)
	if err != nil {
		return err
	}
	n.Artifacts = append(n.Artifacts, a)
	n.Modified = time.Now().UTC()
	return s.UpdateNode(n)
}

// RemoveArtifact removes artifact references matching the non-empty fields of the matcher.
func (s *Store) RemoveArtifact(id NodeID, matcher Artifact) error {
	n, err := s.GetNode(id)
	if err != nil {
		return err
	}
	filtered := n.Artifacts[:0]
	removed := false
	for _, a := range n.Artifacts {
		if artifactMatches(a, matcher) {
			removed = true
			continue
		}
		filtered = append(filtered, a)
	}
	if !removed {
		return nil
	}
	n.Artifacts = filtered
	n.Modified = time.Now().UTC()
	return s.UpdateNode(n)
}

// EmbedArtifact copies a local file into the research root and registers it.
func (s *Store) EmbedArtifact(id NodeID, localPath string, description string) error {
	return s.embedArtifact(id, localPath, description)
}

// CreateResource adds a new resource inventory record.
func (s *Store) CreateResource(r Resource) error {
	return s.createResource(r)
}

// UpdateResource updates an existing resource inventory record.
func (s *Store) UpdateResource(r Resource) error {
	return s.updateResource(r)
}

// DeleteResource removes a resource when it has no active leases.
func (s *Store) DeleteResource(id string) error {
	return s.deleteResource(id)
}

// GetResource returns one resource by ID.
func (s *Store) GetResource(id string) (*Resource, error) {
	return s.getResource(id)
}

// ListResources returns all known resources.
func (s *Store) ListResources() ([]Resource, error) {
	return s.listResources()
}

// ClaimResource creates an active lease from a node to a resource.
func (s *Store) ClaimResource(lease ResourceLease) error {
	return s.claimResource(lease)
}

// ReleaseResource removes one active lease.
func (s *Store) ReleaseResource(nodeID NodeID, resourceID string) error {
	return s.releaseResource(nodeID, resourceID)
}

// ListResourceLeases returns all active leases.
func (s *Store) ListResourceLeases() ([]ResourceLease, error) {
	return s.listResourceLeases()
}

// ListResourceEvents returns all historical resource occupancy events.
func (s *Store) ListResourceEvents() ([]ResourceEvent, error) {
	return s.listResourceEvents()
}

// GetResourceEvents returns historical occupancy events for one resource.
func (s *Store) GetResourceEvents(resourceID string) ([]ResourceEvent, error) {
	return s.getResourceEvents(resourceID)
}

// GetNodeResourceLeases returns active leases held by a node.
func (s *Store) GetNodeResourceLeases(nodeID NodeID) ([]ResourceLease, error) {
	return s.getNodeResourceLeases(nodeID)
}

// AddTags adds one or more tags to a node.
func (s *Store) AddTags(id NodeID, tags ...string) error {
	n, err := s.GetNode(id)
	if err != nil {
		return err
	}
	n.Tags = append(n.Tags, tags...)
	n.Tags = uniqueStrings(n.Tags)
	n.Modified = time.Now().UTC()
	return s.UpdateNode(n)
}

// RemoveTags removes one or more tags from a node.
func (s *Store) RemoveTags(id NodeID, tags ...string) error {
	n, err := s.GetNode(id)
	if err != nil {
		return err
	}
	for _, tag := range tags {
		n.Tags = removeString(n.Tags, tag)
	}
	n.Modified = time.Now().UTC()
	return s.UpdateNode(n)
}

// AddParents adds one or more parent edges to a node without replacing existing parents.
func (s *Store) AddParents(id NodeID, parents ...NodeID) error {
	n, err := s.GetNode(id)
	if err != nil {
		return err
	}
	n.Parents = uniqueSortedIDs(append(n.Parents, parents...))
	n.Modified = time.Now().UTC()
	return s.UpdateNode(n)
}

// RemoveParents removes one or more parent edges from a node.
func (s *Store) RemoveParents(id NodeID, parents ...NodeID) error {
	n, err := s.GetNode(id)
	if err != nil {
		return err
	}
	toRemove := make(map[NodeID]struct{}, len(parents))
	for _, pid := range parents {
		toRemove[pid] = struct{}{}
	}
	filtered := n.Parents[:0]
	for _, pid := range n.Parents {
		if _, drop := toRemove[pid]; drop {
			continue
		}
		filtered = append(filtered, pid)
	}
	n.Parents = uniqueSortedIDs(filtered)
	n.Modified = time.Now().UTC()
	return s.UpdateNode(n)
}

// artifactMatches reports whether an artifact matches the non-zero matcher fields.
func artifactMatches(a Artifact, matcher Artifact) bool {
	if matcher.Mode != "" && a.Mode != matcher.Mode {
		return false
	}
	if matcher.Host != "" && a.Host != matcher.Host {
		return false
	}
	if matcher.Path != "" && a.Path != matcher.Path {
		return false
	}
	if matcher.Description != "" && a.Description != matcher.Description {
		return false
	}
	if matcher.SizeBytes != 0 && a.SizeBytes != matcher.SizeBytes {
		return false
	}
	return matcher.Mode != "" || matcher.Host != "" || matcher.Path != "" || matcher.Description != "" || matcher.SizeBytes != 0
}

// InvalidateClaim marks a previously accepted claim as invalidated and records
// the refuter node and rationale.
func (s *Store) InvalidateClaim(target NodeID, refuter NodeID, reason string) error {
	return s.invalidateClaim(target, refuter, reason)
}

// ListBranchWarnings returns warning events for an agent, optionally only
// pending (unacknowledged) ones.
func (s *Store) ListBranchWarnings(agent string, onlyUnacked bool) ([]BranchWarning, error) {
	return s.listBranchWarnings(agent, onlyUnacked)
}

// AckBranchWarning marks a warning as acknowledged.
func (s *Store) AckBranchWarning(warningID string) error {
	return s.ackBranchWarning(warningID)
}

// GetActiveNodes returns all nodes with status=active.
func (s *Store) GetActiveNodes() ([]*Node, error) {
	return s.QueryNodes(Filter{Status: StatusActive})
}

// GetActiveAgents returns agents that have active nodes.
func (s *Store) GetActiveAgents() ([]string, error) {
	nodes, err := s.GetActiveNodes()
	if err != nil {
		return nil, err
	}
	set := map[string]struct{}{}
	for _, n := range nodes {
		if n.Agent != "" {
			set[n.Agent] = struct{}{}
		}
	}
	out := make([]string, 0, len(set))
	for a := range set {
		out = append(out, a)
	}
	return uniqueStrings(out), nil
}

// NextID returns the next ID that would be assigned (without reserving it).
func (s *Store) NextID() NodeID {
	id, _ := s.readNextID()
	return id
}

// ResolveAgentName looks up a human-readable name from agents.json.
func (s *Store) ResolveAgentName(id string) string {
	name, err := s.resolveAgentName(id)
	if err != nil || name == "" {
		return id
	}
	return name
}

// StorageFormat returns the persistence codec currently used by the store.
func (s *Store) StorageFormat() StorageFormat {
	return s.format
}

// MigrateStorageFormat migrates persistent state between json and binary codecs.
func (s *Store) MigrateStorageFormat(target StorageFormat) error {
	return s.migrateStorageFormat(target)
}

// SnapshotMeta describes a recoverable historical snapshot.
type SnapshotMeta struct {
	ID        string `json:"id"`
	CreatedAt string `json:"created_at"`
	Operation string `json:"operation"`
	Hash      string `json:"hash"`
}

// ListSnapshots returns available historical snapshots.
func (s *Store) ListSnapshots() ([]SnapshotMeta, error) {
	return s.listSnapshots()
}

// RestoreSnapshot restores a historical snapshot by id.
func (s *Store) RestoreSnapshot(snapshotID string) error {
	return s.restoreSnapshot(snapshotID)
}

// GetNodeHistory returns all previous versions of a node, ordered oldest-first.
// Returns nil if no history exists.
func (s *Store) GetNodeHistory(id NodeID) ([]*Node, error) {
	return s.getNodeHistory(id)
}

// ListRelations returns all relations for a given node from the relations.jsonl index.
func (s *Store) ListRelations(id NodeID) ([]Relation, error) {
	return s.listRelations(id)
}

// ListAllRelations returns all relation edges across all nodes as (from, relation, target) triples.
func (s *Store) ListAllRelations() ([]struct {
	From     NodeID
	Relation Relation
}, error) {
	return s.listAllRelations()
}

// RegenerateRelations rebuilds the relations.jsonl index from stored node data.
func (s *Store) RegenerateRelations() error {
	return s.regenerateRelationsFromNodes()
}
