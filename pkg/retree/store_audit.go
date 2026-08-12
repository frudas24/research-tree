package retree

import (
	"fmt"
	"slices"
)

// auditStore validates that all persisted store sidecars are structurally and
// semantically consistent with the node set before a caller trusts the store.
func (s *Store) auditStore() error {
	g, err := s.loadGraph()
	if err != nil {
		return err
	}
	if err := s.auditRelations(g); err != nil {
		return err
	}
	features, err := s.auditFeatures(g)
	if err != nil {
		return err
	}
	resources, err := s.auditResources()
	if err != nil {
		return err
	}
	if err := s.auditLeases(g, resources); err != nil {
		return err
	}
	if err := s.auditFeatureEdges(features, g); err != nil {
		return err
	}
	if err := s.auditWarnings(g); err != nil {
		return err
	}
	if err := s.auditResourceEvents(g, resources); err != nil {
		return err
	}
	return nil
}

// auditRelations validates relations.jsonl against the loaded node set.
func (s *Store) auditRelations(g *Graph) error {
	lines, err := s.readRelationsLines()
	if err != nil {
		return err
	}
	for _, rl := range lines {
		if _, ok := g.Nodes[rl.From]; !ok {
			return fmt.Errorf("%w: relation source %d not found", ErrInvalidNode, rl.From)
		}
		if _, ok := g.Nodes[rl.To]; !ok {
			return fmt.Errorf("%w: relation target %d not found", ErrInvalidNode, rl.To)
		}
		if !slices.Contains(validRelationTypes, rl.Type) {
			return fmt.Errorf("%w: unknown relation type %q", ErrInvalidNode, rl.Type)
		}
	}
	return nil
}

// auditFeatures validates features.json and returns features indexed by ID.
func (s *Store) auditFeatures(g *Graph) (map[string]*Feature, error) {
	payload, err := s.loadFeaturePayload()
	if err != nil {
		return nil, err
	}
	byID := make(map[string]*Feature, len(payload.Features))
	slugs := make(map[string]string, len(payload.Features))
	for _, f := range payload.Features {
		if err := ValidateFeature(f); err != nil {
			return nil, err
		}
		if prev, dup := byID[f.ID]; dup {
			return nil, fmt.Errorf("%w: duplicate feature id %s (%s and %s)", ErrDuplicateID, f.ID, prev.Name, f.Name)
		}
		if prevID, dup := slugs[f.Slug]; dup {
			return nil, fmt.Errorf("%w: duplicate feature slug %s (%s and %s)", ErrDuplicateID, f.Slug, prevID, f.ID)
		}
		slugs[f.Slug] = f.ID
		linked := make(map[NodeID]struct{}, len(f.Nodes))
		for _, ln := range f.Nodes {
			if _, dup := linked[ln.NodeID]; dup {
				return nil, fmt.Errorf("%w: duplicate linked node %d in feature %s", ErrDuplicateID, ln.NodeID, f.ID)
			}
			linked[ln.NodeID] = struct{}{}
		}
		byID[f.ID] = f
	}
	return byID, nil
}

// auditResources validates resources.json and returns resources indexed by ID.
func (s *Store) auditResources() (map[string]Resource, error) {
	list, err := s.readResources()
	if err != nil {
		return nil, err
	}
	byID := make(map[string]Resource, len(list))
	for _, resource := range list {
		if err := ValidateResource(resource); err != nil {
			return nil, err
		}
		if _, dup := byID[resource.ID]; dup {
			return nil, fmt.Errorf("%w: duplicate resource id %s", ErrDuplicateID, resource.ID)
		}
		byID[resource.ID] = resource
	}
	return byID, nil
}

// auditLeases validates leases.json against nodes and resources.
func (s *Store) auditLeases(g *Graph, resources map[string]Resource) error {
	leases, err := s.readLeases()
	if err != nil {
		return err
	}
	seen := make(map[string]struct{}, len(leases))
	for _, lease := range leases {
		if err := ValidateLease(lease); err != nil {
			return err
		}
		if _, ok := g.Nodes[lease.NodeID]; !ok {
			return fmt.Errorf("lease node %d: %w", lease.NodeID, ErrNotFound)
		}
		if _, ok := resources[lease.ResourceID]; !ok {
			return fmt.Errorf("lease resource %s: %w", lease.ResourceID, ErrNotFound)
		}
		key := fmt.Sprintf("%s:%d", lease.ResourceID, lease.NodeID)
		if _, dup := seen[key]; dup {
			return fmt.Errorf("%w: duplicate lease %s", ErrDuplicateID, key)
		}
		seen[key] = struct{}{}
	}
	return nil
}

// auditFeatureEdges validates feature_edges.jsonl against features and nodes.
func (s *Store) auditFeatureEdges(features map[string]*Feature, g *Graph) error {
	edges, err := s.loadFeatureEdges()
	if err != nil {
		return err
	}
	seen := make(map[string]struct{}, len(edges))
	for _, edge := range edges {
		if err := ValidateFeatureEdge(&edge); err != nil {
			return err
		}
		if _, ok := features[edge.From]; !ok {
			return fmt.Errorf("feature edge from %s: %w", edge.From, ErrNotFound)
		}
		if _, ok := features[edge.To]; !ok {
			return fmt.Errorf("feature edge to %s: %w", edge.To, ErrNotFound)
		}
		key := fmt.Sprintf("%s:%s:%s", edge.From, edge.To, edge.Type)
		if _, dup := seen[key]; dup {
			return fmt.Errorf("%w: duplicate feature edge %s", ErrDuplicateID, key)
		}
		seen[key] = struct{}{}
	}
	return nil
}

// auditWarnings validates persisted branch warnings against the node set.
func (s *Store) auditWarnings(g *Graph) error {
	warnings, err := readJSONLines[BranchWarning](s.alertsPath())
	if err != nil {
		return err
	}
	for _, warning := range warnings {
		if warning.ID == "" {
			return fmt.Errorf("%w: warning id required", ErrInvalidNode)
		}
		if warning.RootCauseNode == 0 {
			return fmt.Errorf("%w: warning root cause node required", ErrInvalidNode)
		}
		if warning.ImpactedNode == 0 {
			return fmt.Errorf("%w: warning impacted node required", ErrInvalidNode)
		}
	}
	return nil
}

// auditResourceEvents validates historical resource events against nodes and resources.
func (s *Store) auditResourceEvents(g *Graph, resources map[string]Resource) error {
	events, err := readJSONLines[ResourceEvent](s.resourceEventsPath())
	if err != nil {
		return err
	}
	for _, event := range events {
		if event.ResourceID == "" {
			return fmt.Errorf("%w: resource event resource_id required", ErrInvalidResource)
		}
		if event.NodeID == 0 {
			return fmt.Errorf("%w: resource event node_id required", ErrInvalidResource)
		}
		if !slices.Contains(validLeaseModes, event.Mode) && event.Mode != "" {
			return fmt.Errorf("%w: resource event mode=%q", ErrInvalidResource, event.Mode)
		}
		_ = resources
		_ = g
	}
	return nil
}
