package retree

import (
	"fmt"
	"sort"
)

// LegacyOutcomeIssue describes one legacy node that is done but still carries
// an unset outcome and therefore cannot pass normal validation anymore.
type LegacyOutcomeIssue struct {
	ID     NodeID     `json:"id"`
	Title  string     `json:"title"`
	Status NodeStatus `json:"status"`
	Kind   NodeKind   `json:"kind"`
}

// RepairLegacyOutcomeReport reports discovered and optionally repaired legacy
// done+unset nodes.
type RepairLegacyOutcomeReport struct {
	Issues   []LegacyOutcomeIssue `json:"issues"`
	Repaired []NodeID             `json:"repaired,omitempty"`
}

// ScanLegacyDoneUnsetOutcomes lists legacy nodes that need explicit terminal
// outcomes before the store can pass normal validation.
func (s *Store) ScanLegacyDoneUnsetOutcomes() (*RepairLegacyOutcomeReport, error) {
	nodes, err := s.loadAllNodesAllowLegacyDoneUnset()
	if err != nil {
		return nil, err
	}
	report := &RepairLegacyOutcomeReport{Issues: collectLegacyOutcomeIssues(nodes)}
	return report, nil
}

// RepairLegacyDoneUnsetOutcomes applies explicit terminal outcomes to every
// legacy done+unset node in the store. The fixes map must cover every issue.
func (s *Store) RepairLegacyDoneUnsetOutcomes(fixes map[NodeID]Outcome) (*RepairLegacyOutcomeReport, error) {
	report, err := s.ScanLegacyDoneUnsetOutcomes()
	if err != nil {
		return nil, err
	}
	if len(report.Issues) == 0 {
		return report, nil
	}
	if len(fixes) == 0 {
		return nil, fmt.Errorf("%w: no fixes supplied for legacy done+unset nodes", ErrInvalidNode)
	}
	return s.repairLegacyDoneUnsetOutcomes(report.Issues, fixes)
}

// repairLegacyDoneUnsetOutcomes applies a fully explicit outcome mapping under
// lock, snapshots the store, and rewrites it back through the strict validator.
func (s *Store) repairLegacyDoneUnsetOutcomes(issues []LegacyOutcomeIssue, fixes map[NodeID]Outcome) (*RepairLegacyOutcomeReport, error) {
	issueSet := make(map[NodeID]LegacyOutcomeIssue, len(issues))
	for _, issue := range issues {
		issueSet[issue.ID] = issue
	}
	for id, outcome := range fixes {
		if _, ok := issueSet[id]; !ok {
			return nil, fmt.Errorf("%w: node %d is not a legacy done+unset issue", ErrInvalidNode, id)
		}
		if outcome == OutcomeUnset {
			return nil, fmt.Errorf("%w: node %d repair outcome must be terminal", ErrInvalidNode, id)
		}
	}
	for id := range issueSet {
		if _, ok := fixes[id]; !ok {
			return nil, fmt.Errorf("%w: missing explicit outcome for legacy node %d", ErrInvalidNode, id)
		}
	}

	report := &RepairLegacyOutcomeReport{Issues: issues}
	err := s.withLock("repair_legacy_outcomes", func() error {
		if err := s.ensureSnapshotCatalogHealthy(); err != nil {
			return err
		}
		nodes, err := s.loadAllNodesAllowLegacyDoneUnset()
		if err != nil {
			return err
		}
		g := NewGraph()
		for _, n := range nodes {
			if outcome, ok := fixes[n.ID]; ok && n.Status == StatusDone && normalizeOutcome(n.Outcome) == OutcomeUnset {
				n.Outcome = outcome
				report.Repaired = append(report.Repaired, n.ID)
			}
			if err := ValidateNode(n); err != nil {
				return fmt.Errorf("legacy outcome repair for node %d: %w", n.ID, err)
			}
			if err := g.addNode(n, false); err != nil {
				return err
			}
		}
		if err := validateGraphReferentialIntegrity(g); err != nil {
			return err
		}
		if err := s.createSnapshot("repair_legacy_outcomes_pre"); err != nil {
			return err
		}
		if err := s.persistGraph(g); err != nil {
			return err
		}
		s.bestEffortSnapshot("repair_legacy_outcomes_post")
		sort.Slice(report.Repaired, func(i, j int) bool { return report.Repaired[i] < report.Repaired[j] })
		return nil
	})
	if err != nil {
		return nil, err
	}
	return report, nil
}

// collectLegacyOutcomeIssues extracts the done+unset nodes that must be
// repaired before the store can pass normal validation.
func collectLegacyOutcomeIssues(nodes []*Node) []LegacyOutcomeIssue {
	issues := make([]LegacyOutcomeIssue, 0)
	for _, n := range nodes {
		if normalizeStatus(n.Status) == StatusDone && normalizeOutcome(n.Outcome) == OutcomeUnset {
			issues = append(issues, LegacyOutcomeIssue{
				ID:     n.ID,
				Title:  n.Title,
				Status: normalizeStatus(n.Status),
				Kind:   effectiveKind(n),
			})
		}
	}
	sort.Slice(issues, func(i, j int) bool { return issues[i].ID < issues[j].ID })
	return issues
}
