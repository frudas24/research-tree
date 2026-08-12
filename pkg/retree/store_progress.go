package retree

import (
	"fmt"
	"sort"
)

// UmbrellaProgress is a derived, never-persisted summary of an umbrella's
// children. Every child is counted once per umbrella even when it has multiple
// parents, because the children set is built from the umbrella's own parent links.
type UmbrellaProgress struct {
	DirectChildren   int      `json:"direct_children"`
	WorkChildren     int      `json:"work_children"`
	UmbrellaChildren int      `json:"umbrella_children"`
	Active           int      `json:"active"`
	Done             int      `json:"done"`
	Paused           int      `json:"paused"`
	ActionableLeaves []NodeID `json:"actionable_leaves"`
}

// DerivedProgress computes the derived progress summary for an umbrella node.
// It is read-only and returns an error for non-umbrella nodes.
func (s *Store) DerivedProgress(id NodeID) (*UmbrellaProgress, error) {
	g, err := s.loadGraph()
	if err != nil {
		return nil, err
	}
	n, ok := g.Nodes[id]
	if !ok {
		return nil, ErrNotFound
	}
	if !n.IsUmbrella() {
		return nil, fmt.Errorf("%w: node %d is not an umbrella", ErrInvalidNode, id)
	}

	prog := &UmbrellaProgress{}
	children := g.GetChildren(id)
	prog.DirectChildren = len(children)
	for _, cid := range children {
		c, ok := g.Nodes[cid]
		if !ok {
			continue
		}
		if c.IsUmbrella() {
			prog.UmbrellaChildren++
		} else {
			prog.WorkChildren++
		}
		switch c.Status {
		case StatusActive:
			prog.Active++
		case StatusDone:
			prog.Done++
		case StatusPaused:
			prog.Paused++
		}
	}

	// Actionable leaves: active work-kind descendants without children of their
	// own. Paused and done leaves are deferred/resolved and not actionable now.
	// Self is excluded by GetDescendants.
	for _, did := range g.GetDescendants(id) {
		d, ok := g.Nodes[did]
		if !ok || d.IsUmbrella() {
			continue
		}
		if len(g.GetChildren(did)) > 0 {
			continue
		}
		if d.Status != StatusActive {
			continue
		}
		prog.ActionableLeaves = append(prog.ActionableLeaves, did)
	}
	sort.Slice(prog.ActionableLeaves, func(i, j int) bool { return prog.ActionableLeaves[i] < prog.ActionableLeaves[j] })
	return prog, nil
}
