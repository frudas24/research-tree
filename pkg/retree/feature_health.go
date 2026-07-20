package retree

import (
	"fmt"
	"sort"
	"strings"
)

// healthSeverity maps DerivedHealth to a numeric severity for ordering.
var healthSeverity = map[DerivedHealth]int{
	HealthClean:    0,
	HealthWarning:  1,
	HealthUnmoored: 2,
	HealthDegraded: 3,
}

// healthOrder: degraded > unmoored > warning > clean
func worstHealth(a, b DerivedHealth) DerivedHealth {
	if healthSeverity[a] >= healthSeverity[b] {
		return a
	}
	return b
}

// FeatureHealthReport is the structured output of a feature health check.
type FeatureHealthReport struct {
	FeatureID   string          `json:"feature_id"`
	FeatureName string          `json:"feature_name"`
	Status      FeatureStatus   `json:"status"`
	Health      DerivedHealth   `json:"health"`
	Issues      []string        `json:"issues"`
	Timeline    []TimelineEntry `json:"timeline,omitempty"`
}

// TimelineEntry is one node's contribution to the feature timeline.
type TimelineEntry struct {
	NodeID NodeID          `json:"node_id"`
	Role   FeatureNodeRole `json:"role"`
	Title  string          `json:"title"`
	Status NodeStatus      `json:"status"`
}

// ComputeFeatureHealth computes derived_health for a feature.
// It checks linked nodes (local) and propagates health through dependency edges.
func (s *Store) ComputeFeatureHealth(spec string) (*FeatureHealthReport, error) {
	edges, err := s.ListAllFeatureEdges()
	if err != nil {
		return nil, fmt.Errorf("compute feature health: read feature edges: %w", err)
	}
	degradedMemo := make(map[string]bool)
	degradedInProgress := make(map[string]bool)
	return s.computeFeatureHealthWithDeps(spec, edges, degradedMemo, degradedInProgress)
}

func (s *Store) computeFeatureHealthWithDeps(spec string, edges []FeatureEdge, degradedMemo map[string]bool, degradedInProgress map[string]bool) (*FeatureHealthReport, error) {
	f, err := s.GetFeature(spec)
	if err != nil {
		return nil, err
	}
	report, err := s.computeLocalFeatureHealth(f.ID)
	if err != nil {
		return nil, err
	}
	for _, e := range edges {
		// Check unmoored
		if _, nerr := s.GetNode(e.CreatedFrom); nerr != nil {
			if e.From == f.ID || e.To == f.ID {
				report.Health = worstHealth(report.Health, HealthUnmoored)
				report.Issues = append(report.Issues,
					fmt.Sprintf("edge %s -[%s]-> %s is unmoored (node %04d not found)",
						e.From, e.Type, e.To, e.CreatedFrom))
			}
			continue
		}

		if e.Type == EdgeDependsOn && e.From == f.ID {
			degraded, err := s.computeDependsOnDegraded(e.To, edges, degradedMemo, degradedInProgress)
			if err != nil {
				return nil, err
			}
			if degraded {
				report.Health = worstHealth(report.Health, HealthDegraded)
				report.Issues = append(report.Issues,
					fmt.Sprintf("depends_on %s which is degraded", e.To))
			}
		}

		// Collaboration warns only when the directly connected feature is degraded.
		if e.Type == EdgeCollaboratesWith && (e.From == f.ID || e.To == f.ID) {
			other := e.To
			if e.To == f.ID {
				other = e.From
			}
			degraded, err := s.computeDependsOnDegraded(other, edges, degradedMemo, degradedInProgress)
			if err != nil {
				return nil, err
			}
			if degraded {
				report.Health = worstHealth(report.Health, HealthWarning)
				report.Issues = append(report.Issues,
					fmt.Sprintf("collaborates_with %s which is degraded → warning", other))
			}
		}

		// Supersedes: report retirement candidate
		if e.Type == EdgeSupersedes && e.To == f.ID {
			report.Issues = append(report.Issues,
				fmt.Sprintf("superseded by %s — consider retiring (from node %04d)", e.From, e.CreatedFrom))
		}
	}
	return report, nil
}

func (s *Store) computeDependsOnDegraded(spec string, edges []FeatureEdge, memo map[string]bool, inProgress map[string]bool) (bool, error) {
	f, err := s.GetFeature(spec)
	if err != nil {
		return false, err
	}
	if degraded, ok := memo[f.ID]; ok {
		return degraded, nil
	}
	if inProgress[f.ID] {
		return false, fmt.Errorf("compute feature health: cycle detected at feature %s", f.ID)
	}
	inProgress[f.ID] = true

	report, err := s.computeLocalFeatureHealth(f.ID)
	if err != nil {
		delete(inProgress, f.ID)
		return false, err
	}
	degraded := report.Health == HealthDegraded
	for _, e := range edges {
		if e.Type != EdgeDependsOn || e.From != f.ID {
			continue
		}
		childDegraded, err := s.computeDependsOnDegraded(e.To, edges, memo, inProgress)
		if err != nil {
			delete(inProgress, f.ID)
			return false, err
		}
		if childDegraded {
			degraded = true
			break
		}
	}
	delete(inProgress, f.ID)
	memo[f.ID] = degraded
	return degraded, nil
}

// computeLocalFeatureHealth computes health from a feature's own nodes only, no edge propagation.
func (s *Store) computeLocalFeatureHealth(spec string) (*FeatureHealthReport, error) {
	f, err := s.GetFeature(spec)
	if err != nil {
		return nil, err
	}
	report := &FeatureHealthReport{
		FeatureID:   f.ID,
		FeatureName: f.Name,
		Status:      f.Status,
		Health:      HealthClean,
		Issues:      make([]string, 0),
	}
	for _, ln := range f.Nodes {
		n, nerr := s.GetNode(ln.NodeID)
		if nerr != nil {
			report.Issues = append(report.Issues, fmt.Sprintf("linked node %04d not found", ln.NodeID))
			continue
		}
		if n.EvidenceStatus == EvidencePoisoned {
			switch ln.Role {
			case RoleImplementation, RoleFix, RoleDecision, RoleRegression:
				report.Health = worstHealth(report.Health, HealthDegraded)
				report.Issues = append(report.Issues,
					fmt.Sprintf("node %04d (%s/%s) is poisoned → degraded", ln.NodeID, ln.Role, truncTitle(n.Title)))
			case RoleBenchmark, RoleExperiment:
				report.Health = worstHealth(report.Health, HealthWarning)
				report.Issues = append(report.Issues,
					fmt.Sprintf("node %04d (%s/%s) is poisoned → warning", ln.NodeID, ln.Role, truncTitle(n.Title)))
			default:
				report.Issues = append(report.Issues,
					fmt.Sprintf("node %04d (%s/%s) is poisoned (info only)", ln.NodeID, ln.Role, truncTitle(n.Title)))
			}
		}
	}
	return report, nil
}

// ComputeFeatureTimeline builds the timeline for a feature.
func (s *Store) ComputeFeatureTimeline(spec string) (*FeatureHealthReport, error) {
	f, err := s.GetFeature(spec)
	if err != nil {
		return nil, err
	}
	report := &FeatureHealthReport{
		FeatureID:   f.ID,
		FeatureName: f.Name,
		Status:      f.Status,
		Timeline:    make([]TimelineEntry, 0, len(f.Nodes)),
	}

	// Sort nodes by ID for chronological order
	sorted := make([]FeatureLinkedNode, len(f.Nodes))
	copy(sorted, f.Nodes)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].NodeID < sorted[j].NodeID })

	for _, ln := range sorted {
		entry := TimelineEntry{NodeID: ln.NodeID, Role: ln.Role}
		if n, nerr := s.GetNode(ln.NodeID); nerr == nil {
			entry.Title = n.Title
			entry.Status = n.Status
		}
		report.Timeline = append(report.Timeline, entry)
	}

	return report, nil
}

// ComputeAllFeatureHealth reports health for all features.
func (s *Store) ComputeAllFeatureHealth() ([]*FeatureHealthReport, error) {
	features, err := s.ListFeatures()
	if err != nil {
		return nil, err
	}
	edges, err := s.ListAllFeatureEdges()
	if err != nil {
		return nil, fmt.Errorf("compute all feature health: read feature edges: %w", err)
	}
	degradedMemo := make(map[string]bool)
	degradedInProgress := make(map[string]bool)
	reports := make([]*FeatureHealthReport, 0, len(features))
	for _, f := range features {
		r, err := s.computeFeatureHealthWithDeps(f.ID, edges, degradedMemo, degradedInProgress)
		if err != nil {
			return nil, err
		}
		reports = append(reports, r)
	}
	return reports, nil
}

func truncTitle(s string) string {
	if len(s) > 40 {
		return s[:37] + "..."
	}
	return s
}

// DocLine is a helper to build CLI output lines.
func (r *FeatureHealthReport) DocLines() string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s %s [%s]  health: %s\n", r.FeatureID, r.FeatureName, r.Status, r.Health)
	for _, iss := range r.Issues {
		fmt.Fprintf(&b, "  • %s\n", iss)
	}
	if len(r.Timeline) > 0 {
		fmt.Fprintf(&b, "  timeline:\n")
		for _, t := range r.Timeline {
			fmt.Fprintf(&b, "    %04d %-16s %s [%s]\n", t.NodeID, t.Role, truncTitle(t.Title), t.Status)
		}
	}
	return b.String()
}
