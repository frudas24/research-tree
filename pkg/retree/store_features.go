package retree

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"slices"
	"sort"
	"strings"
)

var slugRegexp = regexp.MustCompile(`[^a-z0-9]+`)

// Slugify normalizes a name into a URL-friendly slug.
func Slugify(name string) string {
	s := strings.ToLower(strings.TrimSpace(name))
	s = slugRegexp.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	return s
}

// nextFeatureID reads and bumps the next_id counter. Caller must save the payload.
func (s *Store) nextFeatureID(payload *featurePayload) string {
	id := payload.NextID
	payload.NextID++
	return fmt.Sprintf("f%04d", id)
}

// featurePayload is the top-level structure of features.json.
type featurePayload struct {
	NextID   int        `json:"next_id"`
	Features []*Feature `json:"features"`
}

// loadFeaturePayload reads features.json.
func (s *Store) loadFeaturePayload() (*featurePayload, error) {
	b, err := os.ReadFile(s.featuresPath())
	if err != nil {
		return nil, err
	}
	var p featurePayload
	if err := json.Unmarshal(b, &p); err != nil {
		return nil, err
	}
	if p.Features == nil {
		p.Features = []*Feature{}
	}
	return &p, nil
}

// saveFeaturePayload writes features.json atomically.
func (s *Store) saveFeaturePayload(p *featurePayload) error {
	b, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	tmp := s.featuresPath() + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, s.featuresPath())
}

// resolveFeatureID resolves a feature spec (id, slug, or name) to a canonical id.
func (s *Store) resolveFeatureID(spec string) (string, error) {
	spec = strings.TrimSpace(spec)
	if strings.HasPrefix(spec, "f") {
		return spec, nil
	}
	payload, err := s.loadFeaturePayload()
	if err != nil {
		return "", err
	}
	norm := Slugify(spec)
	for _, f := range payload.Features {
		if f.Slug == norm || Slugify(f.Name) == norm {
			return f.ID, nil
		}
	}
	return "", fmt.Errorf("feature %q not found", spec)
}

// CreateFeature creates a new feature and returns it.
func (s *Store) CreateFeature(name string, createdFrom NodeID) (*Feature, error) {
	return s.createFeature(name, createdFrom)
}

func (s *Store) createFeature(name string, createdFrom NodeID) (*Feature, error) {
	var created *Feature
	err := s.withLock("create_feature", func() error {
		if err := s.ensureFeaturesLayout(); err != nil {
			return err
		}
		if _, err := s.GetNode(createdFrom); err != nil {
			return fmt.Errorf("created_from node %d: %w", createdFrom, ErrNotFound)
		}
		slug := Slugify(name)
		if slug == "" {
			return &FeatureError{msg: "slug is empty after normalization"}
		}
		payload, lerr := s.loadFeaturePayload()
		if lerr != nil {
			return lerr
		}
		for _, f := range payload.Features {
			if f.Slug == slug {
				return &FeatureError{msg: fmt.Sprintf("feature slug %q already exists", slug)}
			}
		}
		id := s.nextFeatureID(payload)
		f := &Feature{
			ID:          id,
			Name:        strings.TrimSpace(name),
			Slug:        slug,
			Status:      FeatureActive,
			CreatedFrom: createdFrom,
		}
		if err := ValidateFeature(f); err != nil {
			return err
		}
		payload.Features = append(payload.Features, f)
		if serr := s.saveFeaturePayload(payload); serr != nil {
			return serr
		}
		created = f
		return nil
	})
	if err != nil {
		return nil, err
	}
	return created, nil
}

// GetFeature retrieves a feature by id, slug, or name.
func (s *Store) GetFeature(spec string) (*Feature, error) {
	id, err := s.resolveFeatureID(spec)
	if err != nil {
		return nil, err
	}
	payload, err := s.loadFeaturePayload()
	if err != nil {
		return nil, err
	}
	for _, f := range payload.Features {
		if f.ID == id {
			return f, nil
		}
	}
	return nil, ErrNotFound
}

// ListFeatures returns all features.
func (s *Store) ListFeatures() ([]*Feature, error) {
	payload, err := s.loadFeaturePayload()
	if err != nil {
		return nil, err
	}
	out := make([]*Feature, len(payload.Features))
	copy(out, payload.Features)
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

// LinkNodeToFeature links a node to a feature with the given role.
// Idempotent: if the node is already linked, the role is updated.
func (s *Store) LinkNodeToFeature(featureSpec string, nodeID NodeID, role FeatureNodeRole) error {
	fid, err := s.resolveFeatureID(featureSpec)
	if err != nil {
		return err
	}
	if _, err := s.GetNode(nodeID); err != nil {
		return fmt.Errorf("linked node %d: %w", nodeID, ErrNotFound)
	}
	if !slices.Contains(validFeatureNodeRoles, role) {
		return &FeatureError{msg: "unknown node role: " + string(role)}
	}
	return s.withLock("link_node_feature", func() error {
		payload, err := s.loadFeaturePayload()
		if err != nil {
			return err
		}
		var target *Feature
		for _, f := range payload.Features {
			if f.ID == fid {
				target = f
				break
			}
		}
		if target == nil {
			return fmt.Errorf("feature %s: %w", fid, ErrNotFound)
		}
		// Check existing link — update role if found
		for i, n := range target.Nodes {
			if n.NodeID == nodeID {
				if n.Role == role {
					return nil // already linked with same role
				}
				target.Nodes[i].Role = role
				target.maybeResolveCurrentNode()
				if err := ValidateFeature(target); err != nil {
					return err
				}
				return s.saveFeaturePayload(payload)
			}
		}
		// New link
		target.Nodes = append(target.Nodes, FeatureLinkedNode{NodeID: nodeID, Role: role})
		target.maybeResolveCurrentNode()
		if err := ValidateFeature(target); err != nil {
			return err
		}
		return s.saveFeaturePayload(payload)
	})
}

// resolveCurrentNode derives current_node from the latest linked node
// with role implementation, fix, or decision.
func (f *Feature) resolveCurrentNode() {
	var latest NodeID
	for _, n := range f.Nodes {
		if currentNodeRoles[n.Role] && n.NodeID > latest {
			latest = n.NodeID
		}
	}
	f.CurrentNode = latest
}

func (f *Feature) maybeResolveCurrentNode() {
	if f.CurrentNodeMode == "explicit" {
		return
	}
	f.CurrentNodeMode = "derived"
	f.resolveCurrentNode()
}

// SetFeatureStatus updates the feature status.
func (s *Store) SetFeatureStatus(featureSpec string, status FeatureStatus) error {
	fid, err := s.resolveFeatureID(featureSpec)
	if err != nil {
		return err
	}
	return s.withLock("set_feature_status", func() error {
		payload, err := s.loadFeaturePayload()
		if err != nil {
			return err
		}
		for _, f := range payload.Features {
			if f.ID == fid {
				f.Status = status
				if err := ValidateFeature(f); err != nil {
					return err
				}
				return s.saveFeaturePayload(payload)
			}
		}
		return ErrNotFound
	})
}

// SetFeatureCurrentNode sets the current_node explicitly.
func (s *Store) SetFeatureCurrentNode(featureSpec string, nodeID NodeID) error {
	fid, err := s.resolveFeatureID(featureSpec)
	if err != nil {
		return err
	}
	if _, err := s.GetNode(nodeID); err != nil {
		return fmt.Errorf("current node %d: %w", nodeID, ErrNotFound)
	}
	return s.withLock("set_feature_current", func() error {
		payload, err := s.loadFeaturePayload()
		if err != nil {
			return err
		}
		for _, f := range payload.Features {
			if f.ID == fid {
				f.CurrentNode = nodeID
				f.CurrentNodeMode = "explicit"
				if err := ValidateFeature(f); err != nil {
					return err
				}
				return s.saveFeaturePayload(payload)
			}
		}
		return ErrNotFound
	})
}

// FeatureExists reports whether a feature spec resolves to an existing feature.
func (s *Store) FeatureExists(spec string) bool {
	_, err := s.resolveFeatureID(spec)
	return err == nil
}

// ── Feature Edges ───────────────────────────────────────────────────────────

// saveFeatureEdges writes feature_edges.jsonl atomically.
func (s *Store) saveFeatureEdges(edges []FeatureEdge) error {
	var b strings.Builder
	for _, e := range edges {
		line, err := json.Marshal(e)
		if err != nil {
			return err
		}
		b.Write(line)
		b.WriteByte('\n')
	}
	tmp := s.featureEdgesPath() + ".tmp"
	if err := os.WriteFile(tmp, []byte(b.String()), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, s.featureEdgesPath())
}

// loadFeatureEdges reads all edges from feature_edges.jsonl.
func (s *Store) loadFeatureEdges() ([]FeatureEdge, error) {
	f, err := os.Open(s.featureEdgesPath())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()
	var out []FeatureEdge
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var e FeatureEdge
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, sc.Err()
}

// RelateFeatures creates a typed edge between two features.
// createdFrom must reference an existing RT node.
// Duplicate edges (same from, to, type) are rejected.
// depends_on and supersedes cycles are rejected.
func (s *Store) RelateFeatures(fromSpec, toSpec string, edgeType FeatureEdgeType, createdFrom NodeID) error {
	fidFrom, err := s.resolveFeatureID(fromSpec)
	if err != nil {
		return fmt.Errorf("from: %w", err)
	}
	fidTo, err := s.resolveFeatureID(toSpec)
	if err != nil {
		return fmt.Errorf("to: %w", err)
	}
	if fidFrom == fidTo {
		return &FeatureError{msg: "cannot relate feature to itself"}
	}
	if _, err := s.GetFeature(fidFrom); err != nil {
		return fmt.Errorf("from: %w", err)
	}
	if _, err := s.GetFeature(fidTo); err != nil {
		return fmt.Errorf("to: %w", err)
	}
	if _, err := s.GetNode(createdFrom); err != nil {
		return fmt.Errorf("created_from node %d: %w", createdFrom, ErrNotFound)
	}
	if !slices.Contains(validFeatureEdgeTypes, edgeType) {
		return &FeatureError{msg: "unknown edge type: " + string(edgeType)}
	}
	return s.withLock("relate_features", func() error {
		edges, err := s.loadFeatureEdges()
		if err != nil {
			return err
		}
		// Check duplicate
		for _, e := range edges {
			if e.From == fidFrom && e.To == fidTo && e.Type == edgeType {
				return &FeatureError{msg: fmt.Sprintf("edge %s->%s %s already exists", fidFrom, fidTo, edgeType)}
			}
		}
		// Cycle detection for depends_on and supersedes
		if edgeType == EdgeDependsOn || edgeType == EdgeSupersedes {
			edges = append(edges, FeatureEdge{From: fidFrom, To: fidTo, Type: edgeType, CreatedFrom: createdFrom})
			if hasFeatureCycle(edges, fidFrom, fidTo, edgeType) {
				return &FeatureError{msg: fmt.Sprintf("cycle detected: %s->%s %s", fidFrom, fidTo, edgeType)}
			}
			edges = edges[:len(edges)-1]
		}
		edges = append(edges, FeatureEdge{From: fidFrom, To: fidTo, Type: edgeType, CreatedFrom: createdFrom})
		return s.saveFeatureEdges(edges)
	})
}

// hasFeatureCycle checks whether adding edge from→to of the given type creates a cycle.
// Only checks depends_on and supersedes (not collaborates_with).
func hasFeatureCycle(edges []FeatureEdge, from, to string, edgeType FeatureEdgeType) bool {
	// Build adjacency: node → reachable via depends_on or supersedes
	graph := make(map[string][]string)
	for _, e := range edges {
		if e.Type == EdgeDependsOn || e.Type == EdgeSupersedes {
			graph[e.From] = append(graph[e.From], e.To)
		}
	}
	// DFS from `to` → if we can reach `from`, adding from→to creates a cycle
	visited := make(map[string]bool)
	var dfs func(n string) bool
	dfs = func(n string) bool {
		if n == from {
			return true
		}
		if visited[n] {
			return false
		}
		visited[n] = true
		for _, next := range graph[n] {
			if dfs(next) {
				return true
			}
		}
		return false
	}
	return dfs(to)
}

// UnrelateFeatures removes the edge with the given type between two features.
// The edge type is required to disambiguate when multiple edges exist.
func (s *Store) UnrelateFeatures(fromSpec, toSpec string, edgeType FeatureEdgeType) error {
	fidFrom, err := s.resolveFeatureID(fromSpec)
	if err != nil {
		return fmt.Errorf("from: %w", err)
	}
	fidTo, err := s.resolveFeatureID(toSpec)
	if err != nil {
		return fmt.Errorf("to: %w", err)
	}
	return s.withLock("unrelate_features", func() error {
		edges, err := s.loadFeatureEdges()
		if err != nil {
			return err
		}
		found := false
		filtered := make([]FeatureEdge, 0, len(edges))
		for _, e := range edges {
			if e.From == fidFrom && e.To == fidTo && e.Type == edgeType {
				found = true
				continue
			}
			filtered = append(filtered, e)
		}
		if !found {
			return &FeatureError{msg: fmt.Sprintf("edge %s->%s %s not found", fidFrom, fidTo, edgeType)}
		}
		return s.saveFeatureEdges(filtered)
	})
}

// ListFeatureEdges returns all edges for a feature (both incoming and outgoing).
func (s *Store) ListFeatureEdges(featureSpec string) ([]FeatureEdge, error) {
	fid, err := s.resolveFeatureID(featureSpec)
	if err != nil {
		return nil, err
	}
	edges, err := s.loadFeatureEdges()
	if err != nil {
		return nil, err
	}
	var out []FeatureEdge
	for _, e := range edges {
		if e.From == fid || e.To == fid {
			out = append(out, e)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Type != out[j].Type {
			return string(out[i].Type) < string(out[j].Type)
		}
		if out[i].From != out[j].From {
			return out[i].From < out[j].From
		}
		return out[i].To < out[j].To
	})
	return out, nil
}

// ListAllFeatureEdges returns every feature edge.
func (s *Store) ListAllFeatureEdges() ([]FeatureEdge, error) {
	return s.loadFeatureEdges()
}

// ── Impact & Graph ─────────────────────────────────────────────────────────

// FeatureImpact reports what depends on or collaborates with a feature.
type FeatureImpact struct {
	FeatureID    string   `json:"feature_id"`
	FeatureName  string   `json:"feature_name"`
	DependsOnUs  []string `json:"depends_on_us"`
	Collaborates []string `json:"collaborates_with_us"`
	WeDependOn   []string `json:"we_depend_on"`
}

// ComputeFeatureImpact analyzes what other features depend on or collaborate with this one.
func (s *Store) ComputeFeatureImpact(spec string) (*FeatureImpact, error) {
	f, err := s.GetFeature(spec)
	if err != nil {
		return nil, err
	}
	edges, err := s.ListAllFeatureEdges()
	if err != nil {
		return nil, err
	}
	impact := &FeatureImpact{
		FeatureID:   f.ID,
		FeatureName: f.Name,
	}
	for _, e := range edges {
		switch e.Type {
		case EdgeDependsOn:
			if e.To == f.ID {
				impact.DependsOnUs = append(impact.DependsOnUs, e.From)
			}
			if e.From == f.ID {
				impact.WeDependOn = append(impact.WeDependOn, e.To)
			}
		case EdgeCollaboratesWith:
			if e.From == f.ID {
				impact.Collaborates = append(impact.Collaborates, e.To)
			} else if e.To == f.ID {
				impact.Collaborates = append(impact.Collaborates, e.From)
			}
		}
	}
	sort.Strings(impact.DependsOnUs)
	sort.Strings(impact.Collaborates)
	sort.Strings(impact.WeDependOn)
	return impact, nil
}

// FeatureGraph is a subgraph of features connected by edges.
type FeatureGraph struct {
	Nodes []FeatureGraphNode `json:"nodes"`
	Edges []FeatureGraphEdge `json:"edges"`
}

// FeatureGraphNode is a node in a feature subgraph.
type FeatureGraphNode struct {
	ID     string        `json:"id"`
	Name   string        `json:"name"`
	Status FeatureStatus `json:"status"`
}

// FeatureGraphEdge is an edge in a feature subgraph.
type FeatureGraphEdge struct {
	From string          `json:"from"`
	To   string          `json:"to"`
	Type FeatureEdgeType `json:"type"`
}

// ComputeFeatureGraph builds the immediate subgraph around a feature.
func (s *Store) ComputeFeatureGraph(spec string) (*FeatureGraph, error) {
	f, err := s.GetFeature(spec)
	if err != nil {
		return nil, err
	}
	edges, err := s.ListAllFeatureEdges()
	if err != nil {
		return nil, err
	}

	nodeSet := map[string]bool{f.ID: true}
	var graphEdges []FeatureGraphEdge

	for _, e := range edges {
		if e.From == f.ID || e.To == f.ID {
			nodeSet[e.From] = true
			nodeSet[e.To] = true
			graphEdges = append(graphEdges, FeatureGraphEdge{
				From: e.From, To: e.To, Type: e.Type,
			})
		}
	}

	features, err := s.ListFeatures()
	if err != nil {
		return nil, err
	}
	nameMap := make(map[string]string, len(features))
	for _, feat := range features {
		nameMap[feat.ID] = feat.Name
	}

	var nodes []FeatureGraphNode
	for id := range nodeSet {
		name := nameMap[id]
		status := FeatureActive
		for _, feat := range features {
			if feat.ID == id {
				status = feat.Status
				break
			}
		}
		nodes = append(nodes, FeatureGraphNode{ID: id, Name: name, Status: status})
	}
	sort.Slice(nodes, func(i, j int) bool { return nodes[i].ID < nodes[j].ID })
	sort.Slice(graphEdges, func(i, j int) bool {
		if graphEdges[i].Type != graphEdges[j].Type {
			return string(graphEdges[i].Type) < string(graphEdges[j].Type)
		}
		return graphEdges[i].From < graphEdges[j].From
	})

	return &FeatureGraph{Nodes: nodes, Edges: graphEdges}, nil
}
