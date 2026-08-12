package retree

import "fmt"

// createNodeWithFeature creates a node and links it to a feature as one store
// mutation, avoiding partial persistence when either side of the composite
// operation fails.
func (s *Store) createNodeWithFeature(n *Node, featureSpec string, role FeatureNodeRole, autoCreate bool, createdFrom NodeID) error {
	if n == nil {
		return fmt.Errorf("%w: nil", ErrInvalidNode)
	}
	if role == "" {
		role = RoleImplementation
	}
	if !containsFeatureRole(role) {
		return &FeatureError{msg: "unknown node role: " + string(role)}
	}
	return s.withLock("create_node_with_feature", func() error {
		if err := s.ensureSnapshotCatalogHealthy(); err != nil {
			return err
		}
		if err := s.ensureFeaturesLayout(); err != nil {
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

		payload, err := s.loadFeaturePayload()
		if err != nil {
			return err
		}
		target, err := s.prepareFeatureLink(payload, featureSpec, autoCreate, createdFrom)
		if err != nil {
			return err
		}
		if err := linkNodeInFeature(target, n.ID, role); err != nil {
			return err
		}
		if err := s.writeNextID(next + 1); err != nil {
			return err
		}
		if err := s.persistGraphDelta(g, map[NodeID]struct{}{n.ID: {}}, nil); err != nil {
			if rollbackErr := s.rollbackCreatedPrimaryState(next, n.ID); rollbackErr != nil {
				return fmt.Errorf("persist node graph: %w; rollback node failed: %v", err, rollbackErr)
			}
			return err
		}
		if err := s.saveFeaturePayload(payload); err != nil {
			rollbackErr := s.rollbackCreatedNode(next, n.ID)
			if rollbackErr != nil {
				return fmt.Errorf("save feature payload: %w; rollback node failed: %v", err, rollbackErr)
			}
			return err
		}
		s.bestEffortSnapshot("create_node")
		return nil
	})
}

// rollbackCreatedPrimaryState removes the just-created node from the primary
// node store and restores next_id without depending on sidecar regeneration.
func (s *Store) rollbackCreatedPrimaryState(previousNext NodeID, createdID NodeID) error {
	nodes, err := s.loadAllNodes()
	if err != nil {
		return err
	}
	filtered := make([]*Node, 0, len(nodes))
	for _, node := range nodes {
		if node.ID == createdID {
			continue
		}
		filtered = append(filtered, node)
	}
	if s.format == StorageJSON {
		if err := s.writeAllNodesJSONDelta(filtered, nil, []NodeID{createdID}); err != nil {
			return err
		}
	} else {
		if err := s.writeAllNodesBIN(filtered); err != nil {
			return err
		}
	}
	return s.writeNextID(previousNext)
}

// rollbackCreatedNode restores the pre-create next_id and removes the just-created node.
func (s *Store) rollbackCreatedNode(previousNext NodeID, createdID NodeID) error {
	g, err := s.loadGraph()
	if err != nil {
		return err
	}
	if err := g.RemoveNode(createdID, false); err != nil && err != ErrNotFound {
		return err
	}
	if err := s.persistGraphDelta(g, nil, []NodeID{createdID}); err != nil {
		return err
	}
	return s.writeNextID(previousNext)
}

// prepareFeatureLink resolves or creates the target feature in-memory.
func (s *Store) prepareFeatureLink(payload *featurePayload, spec string, autoCreate bool, createdFrom NodeID) (*Feature, error) {
	id, feature := resolveFeatureInPayload(payload, spec)
	if feature != nil {
		return feature, nil
	}
	if !autoCreate {
		return nil, fmt.Errorf("feature %q not found", spec)
	}
	if createdFrom == 0 {
		return nil, &FeatureError{msg: "created_from node required for feature creation"}
	}
	if _, err := s.getNode(createdFrom); err != nil {
		return nil, fmt.Errorf("created_from node %d: %w", createdFrom, ErrNotFound)
	}
	slug := Slugify(spec)
	if slug == "" {
		return nil, &FeatureError{msg: "slug is empty after normalization"}
	}
	if _, dup := resolveFeatureInPayload(payload, slug); dup != nil {
		return nil, &FeatureError{msg: fmt.Sprintf("feature slug %q already exists", slug)}
	}
	if id != "" {
		return nil, &FeatureError{msg: fmt.Sprintf("feature %q not found", spec)}
	}
	f := &Feature{
		ID:          s.nextFeatureID(payload),
		Name:        spec,
		Slug:        slug,
		Status:      FeatureActive,
		CreatedFrom: createdFrom,
	}
	if err := ValidateFeature(f); err != nil {
		return nil, err
	}
	payload.Features = append(payload.Features, f)
	return f, nil
}

// resolveFeatureInPayload resolves a feature spec without touching disk again.
func resolveFeatureInPayload(payload *featurePayload, spec string) (string, *Feature) {
	spec = Slugify(spec)
	for _, f := range payload.Features {
		if f.ID == spec || f.Slug == spec || Slugify(f.Name) == spec {
			return f.ID, f
		}
	}
	return "", nil
}

// linkNodeInFeature mutates one feature payload in-memory.
func linkNodeInFeature(target *Feature, nodeID NodeID, role FeatureNodeRole) error {
	for i, n := range target.Nodes {
		if n.NodeID == nodeID {
			target.Nodes[i].Role = role
			target.maybeResolveCurrentNode()
			return ValidateFeature(target)
		}
	}
	target.Nodes = append(target.Nodes, FeatureLinkedNode{NodeID: nodeID, Role: role})
	target.maybeResolveCurrentNode()
	return ValidateFeature(target)
}

// containsFeatureRole reports whether role is one of the allowed feature node roles.
func containsFeatureRole(role FeatureNodeRole) bool {
	return slicesContainsFeatureRole(role)
}
