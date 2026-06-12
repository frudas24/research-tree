package retree

import (
	"fmt"
	"sort"
)

// Graph is an in-memory DAG projection with parent and child indexes.
type Graph struct {
	Nodes    map[NodeID]*Node
	Parents  map[NodeID][]NodeID // child -> parents
	Children map[NodeID][]NodeID // parent -> children
}

// NewGraph builds an empty graph.
func NewGraph() *Graph {
	return &Graph{
		Nodes:    map[NodeID]*Node{},
		Parents:  map[NodeID][]NodeID{},
		Children: map[NodeID][]NodeID{},
	}
}

// AddNode inserts a new node after validation and DAG checks.
func (g *Graph) AddNode(n *Node) error { return g.addNode(n, true) }

// addNode inserts a new node, optionally checking that parents exist.
func (g *Graph) addNode(n *Node, checkParentExists bool) error {
	if g == nil {
		return fmt.Errorf("%w: nil graph", ErrInvalidNode)
	}
	if n == nil {
		return fmt.Errorf("%w: nil", ErrInvalidNode)
	}
	if n.ID == 0 {
		return fmt.Errorf("%w: id required", ErrInvalidNode)
	}
	if _, ok := g.Nodes[n.ID]; ok {
		return fmt.Errorf("%w: %d", ErrDuplicateID, n.ID)
	}
	if err := ValidateNode(n); err != nil {
		return err
	}
	for _, pid := range n.Parents {
		if _, ok := g.Nodes[pid]; !ok && checkParentExists {
			return fmt.Errorf("%w: parent %d not found", ErrInvalidNode, pid)
		}
	}
	if g.WouldCreateCycle(n.ID, n.Parents) {
		return fmt.Errorf("%w: add node %d", ErrCycleDetected, n.ID)
	}

	g.Nodes[n.ID] = CloneNode(n)
	g.Parents[n.ID] = uniqueSortedIDs(n.Parents)
	for _, pid := range g.Parents[n.ID] {
		g.Children[pid] = uniqueSortedIDs(append(g.Children[pid], n.ID))
	}
	if _, ok := g.Children[n.ID]; !ok {
		g.Children[n.ID] = nil
	}
	return nil
}

// RemoveNode deletes a node. If force is false, nodes with children are rejected.
func (g *Graph) RemoveNode(id NodeID, force bool) error {
	if _, ok := g.Nodes[id]; !ok {
		return ErrNotFound
	}
	children := append([]NodeID(nil), g.Children[id]...)
	if len(children) > 0 && !force {
		return fmt.Errorf("%w: %d", ErrHasChildren, id)
	}

	for _, pid := range g.Parents[id] {
		g.Children[pid] = removeID(g.Children[pid], id)
	}
	if force {
		for _, cid := range children {
			g.Parents[cid] = removeID(g.Parents[cid], id)
			if cn, ok := g.Nodes[cid]; ok {
				cn.Parents = removeID(cn.Parents, id)
			}
		}
	}
	delete(g.Nodes, id)
	delete(g.Parents, id)
	delete(g.Children, id)
	return nil
}

// UpdateNode replaces metadata and parent edges of an existing node.
func (g *Graph) UpdateNode(id NodeID, n *Node) error {
	if _, ok := g.Nodes[id]; !ok {
		return ErrNotFound
	}
	if n == nil {
		return fmt.Errorf("%w: nil", ErrInvalidNode)
	}
	candidate := CloneNode(n)
	candidate.ID = id
	if err := ValidateNode(candidate); err != nil {
		return err
	}
	for _, pid := range candidate.Parents {
		if _, ok := g.Nodes[pid]; !ok {
			return fmt.Errorf("%w: parent %d not found", ErrInvalidNode, pid)
		}
	}
	if g.WouldCreateCycle(id, candidate.Parents) {
		return fmt.Errorf("%w: update node %d", ErrCycleDetected, id)
	}

	oldParents := append([]NodeID(nil), g.Parents[id]...)
	for _, pid := range oldParents {
		g.Children[pid] = removeID(g.Children[pid], id)
	}

	g.Nodes[id] = candidate
	g.Parents[id] = uniqueSortedIDs(candidate.Parents)
	for _, pid := range g.Parents[id] {
		g.Children[pid] = uniqueSortedIDs(append(g.Children[pid], id))
	}
	if _, ok := g.Children[id]; !ok {
		g.Children[id] = nil
	}
	return nil
}

// GetNode returns a deep copy of the node.
func (g *Graph) GetNode(id NodeID) (*Node, error) {
	n, ok := g.Nodes[id]
	if !ok {
		return nil, ErrNotFound
	}
	return CloneNode(n), nil
}

// GetChildren returns sorted direct children IDs.
func (g *Graph) GetChildren(id NodeID) []NodeID { return append([]NodeID(nil), g.Children[id]...) }

// GetParents returns sorted direct parent IDs.
func (g *Graph) GetParents(id NodeID) []NodeID { return append([]NodeID(nil), g.Parents[id]...) }

// GetAncestors returns all ancestors in deterministic order.
func (g *Graph) GetAncestors(id NodeID) []NodeID {
	visited := map[NodeID]bool{}
	var out []NodeID
	var dfs func(NodeID)
	dfs = func(cur NodeID) {
		for _, pid := range g.Parents[cur] {
			if visited[pid] {
				continue
			}
			visited[pid] = true
			out = append(out, pid)
			dfs(pid)
		}
	}
	dfs(id)
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

// GetDescendants returns all descendants in deterministic order.
func (g *Graph) GetDescendants(id NodeID) []NodeID {
	visited := map[NodeID]bool{}
	var out []NodeID
	var dfs func(NodeID)
	dfs = func(cur NodeID) {
		for _, cid := range g.Children[cur] {
			if visited[cid] {
				continue
			}
			visited[cid] = true
			out = append(out, cid)
			dfs(cid)
		}
	}
	dfs(id)
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

// GetRoots returns nodes without parents.
func (g *Graph) GetRoots() []NodeID {
	out := make([]NodeID, 0)
	for id := range g.Nodes {
		if len(g.Parents[id]) == 0 {
			out = append(out, id)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

// ListByStatus lists node IDs filtered by node status.
func (g *Graph) ListByStatus(status NodeStatus) []NodeID {
	out := make([]NodeID, 0)
	for id, n := range g.Nodes {
		if n.Status == status {
			out = append(out, id)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

// ListByClaimStatus lists node IDs filtered by claim status.
func (g *Graph) ListByClaimStatus(status ClaimStatus) []NodeID {
	out := make([]NodeID, 0)
	for id, n := range g.Nodes {
		if n.ClaimStatus == status {
			out = append(out, id)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

// ListByTag lists node IDs containing the requested tag.
func (g *Graph) ListByTag(tag string) []NodeID {
	out := make([]NodeID, 0)
	for id, n := range g.Nodes {
		for _, t := range n.Tags {
			if t == tag {
				out = append(out, id)
				break
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

// ListByAgent lists node IDs assigned to the same agent.
func (g *Graph) ListByAgent(agent string) []NodeID {
	out := make([]NodeID, 0)
	for id, n := range g.Nodes {
		if n.Agent == agent {
			out = append(out, id)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

// WouldCreateCycle reports whether assigning newParents to nodeID would create a cycle.
func (g *Graph) WouldCreateCycle(nodeID NodeID, newParents []NodeID) bool {
	for _, pid := range newParents {
		if pid == nodeID {
			return true
		}
		// New edge is pid -> nodeID. It creates a cycle when nodeID can
		// already reach pid through existing descendants.
		if g.hasPath(nodeID, pid) {
			return true
		}
	}
	return false
}

// GetAtRiskDescendants returns active descendants impacted by ancestor invalidation.
func (g *Graph) GetAtRiskDescendants(id NodeID) []NodeID {
	all := g.GetDescendants(id)
	out := make([]NodeID, 0, len(all))
	for _, did := range all {
		if n, ok := g.Nodes[did]; ok && n.Status == StatusActive {
			out = append(out, did)
		}
	}
	return out
}

// hasPath reports whether there is a path from 'from' to 'target'.
func (g *Graph) hasPath(from, target NodeID) bool {
	visited := map[NodeID]bool{}
	var dfs func(NodeID) bool
	dfs = func(cur NodeID) bool {
		if cur == target {
			return true
		}
		visited[cur] = true
		for _, next := range g.Children[cur] {
			if visited[next] {
				continue
			}
			if dfs(next) {
				return true
			}
		}
		return false
	}
	return dfs(from)
}

// uniqueSortedIDs returns a sorted, deduplicated copy of the ID slice.
func uniqueSortedIDs(in []NodeID) []NodeID {
	if len(in) == 0 {
		return nil
	}
	m := make(map[NodeID]struct{}, len(in))
	for _, id := range in {
		m[id] = struct{}{}
	}
	out := make([]NodeID, 0, len(m))
	for id := range m {
		out = append(out, id)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

// removeID returns a copy of the slice with all occurrences of needle removed.
func removeID(in []NodeID, needle NodeID) []NodeID {
	out := make([]NodeID, 0, len(in))
	for _, id := range in {
		if id != needle {
			out = append(out, id)
		}
	}
	return out
}
