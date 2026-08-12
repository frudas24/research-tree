package retree

import (
	"sort"
	"time"
)

const (
	// HotspotPendingChildWeight is the multiplier applied to pending children.
	HotspotPendingChildWeight = 5
	// HotspotInconclusiveOutcomeBonus is added when the node outcome is inconclusive.
	HotspotInconclusiveOutcomeBonus = 5
	// HotspotFormulaDescription documents the deterministic hotspot calculation.
	HotspotFormulaDescription = "hotness = pending_children*5 + age_days + inconclusive_bonus (bonus=5 when outcome=inconclusive)"
)

// NodeSummary is a lightweight view of a node for dashboard and query outputs.
// Full details are available via GetNode.
type NodeSummary struct {
	ID              NodeID         `json:"id"`
	Title           string         `json:"title"`
	Kind            NodeKind       `json:"kind,omitempty"`
	Status          NodeStatus     `json:"status"`
	Outcome         Outcome        `json:"outcome,omitempty"`
	ClaimStatus     ClaimStatus    `json:"claim_status"`
	Agent           string         `json:"agent"`
	Scope           string         `json:"scope,omitempty"`
	Tags            []string       `json:"tags,omitempty"`
	Revision        uint64         `json:"revision"`
	MilestoneClass  MilestoneClass `json:"milestone_class,omitempty"`
	MilestoneKind   MilestoneKind  `json:"milestone_kind,omitempty"`
	MilestoneReason string         `json:"milestone_reason,omitempty"`
	Parents         []NodeID       `json:"parents,omitempty"`
	Children        []NodeID       `json:"children,omitempty"`
}

// HotspotSummary describes one high-attention node in the status dashboard.
type HotspotSummary struct {
	ID                NodeID         `json:"id"`
	Title             string         `json:"title"`
	Status            NodeStatus     `json:"status"`
	Outcome           Outcome        `json:"outcome"`
	ClaimStatus       ClaimStatus    `json:"claim_status"`
	MilestoneClass    MilestoneClass `json:"milestone_class,omitempty"`
	MilestoneKind     MilestoneKind  `json:"milestone_kind,omitempty"`
	MilestoneReason   string         `json:"milestone_reason,omitempty"`
	Agent             string         `json:"agent"`
	PendingChildren   int            `json:"pending_children"`
	AgeDays           int            `json:"age_days"`
	PendingWeight     int            `json:"pending_weight"`
	InconclusiveBonus int            `json:"inconclusive_bonus"`
	Hotness           int            `json:"hotness"`
}

// UmbrellaPressureEntry is a derived count-based pressure signal for one
// umbrella. Unlike work hotspots it deliberately avoids the age/hotness formula:
// umbrellas are programs, not experiments, so pressure is unresolved work.
type UmbrellaPressureEntry struct {
	ID         NodeID `json:"id"`
	Title      string `json:"title"`
	Agent      string `json:"agent"`
	Active     int    `json:"active"`     // direct active children
	Paused     int    `json:"paused"`     // direct paused children
	Unresolved int    `json:"unresolved"` // descendants (excluding self) not done
}

// StatusSummary is the stable dashboard contract for CLI and ABI consumers.
// Existing keys (total/active/done/paused/warnings/agent) are kept for compatibility.
// active/done/paused and hotspots contain work nodes only; umbrellas are reported
// separately so programs never distort actionable-work metrics.
type StatusSummary struct {
	Total                int                            `json:"total"`
	Active               []NodeSummary                  `json:"active"`
	Done                 []NodeSummary                  `json:"done"`
	Paused               []NodeSummary                  `json:"paused"`
	Warnings             []BranchWarning                `json:"warnings"`
	Agent                string                         `json:"agent"`
	StatusCounts         map[NodeStatus]int             `json:"status_counts"`
	ClaimStatusCounts    map[ClaimStatus]int            `json:"claim_status_counts"`
	OutcomeCounts        map[Outcome]int                `json:"outcome_counts"`
	RunValidityCounts    map[string]int                 `json:"run_validity_counts"`
	Matrix               map[NodeStatus]map[Outcome]int `json:"matrix"`
	HotspotFormula       string                         `json:"hotspot_formula"`
	Hotspots             []HotspotSummary               `json:"hotspots"`
	WorkStatusCounts     map[NodeStatus]int             `json:"work_status_counts"`
	UmbrellaStatusCounts map[NodeStatus]int             `json:"umbrella_status_counts"`
	UmbrellaActive       []NodeSummary                  `json:"umbrella_active,omitempty"`
	UmbrellaDone         []NodeSummary                  `json:"umbrella_done,omitempty"`
	UmbrellaPaused       []NodeSummary                  `json:"umbrella_paused,omitempty"`
	UmbrellaPressure     []UmbrellaPressureEntry        `json:"umbrella_pressure,omitempty"`
}

// StatusBuildOptions controls optional status summary behavior.
type StatusBuildOptions struct {
	Agent        string
	HotspotLimit int
	Now          time.Time
}

// SummarizeNodes converts a node list into lightweight summaries with
// bidirectional graph edges derived in O(n) from parent links.
func SummarizeNodes(nodes []*Node) []NodeSummary {
	childrenByParent := buildChildrenByParent(nodes)
	out := make([]NodeSummary, len(nodes))
	for i, n := range nodes {
		out[i] = summarizeNodeWithChildren(n, childrenByParent[n.ID])
	}
	return out
}

// BuildStatusSummary computes a scalable status payload from node metadata.
func BuildStatusSummary(nodes []*Node, warnings []BranchWarning, opts StatusBuildOptions) StatusSummary {
	now := opts.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}
	hotspotLimit := opts.HotspotLimit
	if hotspotLimit <= 0 {
		hotspotLimit = 10
	}

	statusCounts := map[NodeStatus]int{
		StatusActive: 0,
		StatusDone:   0,
		StatusPaused: 0,
	}
	claimCounts := map[ClaimStatus]int{
		ClaimProvisional: 0,
		ClaimValidated:   0,
		ClaimInvalidated: 0,
		ClaimSuperseded:  0,
	}
	outcomeCounts := map[Outcome]int{
		OutcomeUnset:        0,
		OutcomeSuccess:      0,
		OutcomeFailure:      0,
		OutcomeInconclusive: 0,
	}
	runValidityCounts := map[string]int{
		"valid":   0,
		"invalid": 0,
		"unknown": 0,
	}
	matrix := map[NodeStatus]map[Outcome]int{
		StatusActive: newOutcomeCounter(),
		StatusDone:   newOutcomeCounter(),
		StatusPaused: newOutcomeCounter(),
	}

	nodeStatus := buildNodeStatusMap(nodes)
	childrenByParent := buildChildrenByParent(nodes)
	workCounts := map[NodeStatus]int{
		StatusActive: 0,
		StatusDone:   0,
		StatusPaused: 0,
	}
	umbrellaCounts := map[NodeStatus]int{
		StatusActive: 0,
		StatusDone:   0,
		StatusPaused: 0,
	}
	active := make([]NodeSummary, 0)
	done := make([]NodeSummary, 0)
	paused := make([]NodeSummary, 0)
	umbrellaActive := make([]NodeSummary, 0)
	umbrellaDone := make([]NodeSummary, 0)
	umbrellaPaused := make([]NodeSummary, 0)
	hotspots := make([]HotspotSummary, 0)
	umbrellaPressure := make([]UmbrellaPressureEntry, 0)

	for _, n := range nodes {
		status := normalizeStatus(n.Status)
		claim := normalizeClaimStatus(n.ClaimStatus)
		outcome := normalizeOutcome(n.Outcome)
		children := childrenByParent[n.ID]
		summary := summarizeNodeWithChildren(n, children)
		summary.Status = status
		summary.ClaimStatus = claim
		summary.Outcome = outcome

		statusCounts[status]++
		claimCounts[claim]++
		outcomeCounts[outcome]++
		runValidityCounts[latestRunValidity(n)]++
		matrix[status][outcome]++

		switch status {
		case StatusDone:
			done = append(done, summary)
		case StatusPaused:
			paused = append(paused, summary)
		default:
			active = append(active, summary)
		}

		if n.IsUmbrella() {
			umbrellaCounts[status]++
			switch status {
			case StatusDone:
				umbrellaDone = append(umbrellaDone, summary)
			case StatusPaused:
				umbrellaPaused = append(umbrellaPaused, summary)
			default:
				umbrellaActive = append(umbrellaActive, summary)
			}
			continue
		}

		workCounts[status]++

		// Umbrellas never enter the work hotspot formula.
		pendingCount := countPendingChildren(nodeStatus, childrenByParent[n.ID])
		hotspot := summarizeHotspot(n, pendingCount, len(childrenByParent[n.ID]), now)
		if hotspot.Hotness > 0 {
			hotspots = append(hotspots, hotspot)
		}
	}

	for _, n := range nodes {
		if !n.IsUmbrella() {
			continue
		}
		entry := UmbrellaPressureEntry{ID: n.ID, Title: n.Title, Agent: n.Agent}
		for _, cid := range childrenByParent[n.ID] {
			switch nodeStatus[cid] {
			case StatusActive:
				entry.Active++
			case StatusPaused:
				entry.Paused++
			}
		}
		entry.Unresolved = unresolvedDescendants(nodeStatus, childrenByParent, n.ID)
		umbrellaPressure = append(umbrellaPressure, entry)
	}

	sortNodeSummaries(active)
	sortNodeSummaries(done)
	sortNodeSummaries(paused)
	sortNodeSummaries(umbrellaActive)
	sortNodeSummaries(umbrellaDone)
	sortNodeSummaries(umbrellaPaused)
	sortHotspots(hotspots)
	sortUmbrellaPressure(umbrellaPressure)
	if len(hotspots) > hotspotLimit {
		hotspots = hotspots[:hotspotLimit]
	}
	if warnings == nil {
		warnings = []BranchWarning{}
	}

	return StatusSummary{
		Total:                len(nodes),
		Active:               active,
		Done:                 done,
		Paused:               paused,
		Warnings:             warnings,
		Agent:                opts.Agent,
		StatusCounts:         statusCounts,
		ClaimStatusCounts:    claimCounts,
		OutcomeCounts:        outcomeCounts,
		RunValidityCounts:    runValidityCounts,
		Matrix:               matrix,
		HotspotFormula:       HotspotFormulaDescription,
		Hotspots:             hotspots,
		WorkStatusCounts:     workCounts,
		UmbrellaStatusCounts: umbrellaCounts,
		UmbrellaActive:       umbrellaActive,
		UmbrellaDone:         umbrellaDone,
		UmbrellaPaused:       umbrellaPaused,
		UmbrellaPressure:     umbrellaPressure,
	}
}

// unresolvedDescendants counts every descendant of root (within the loaded
// node set, excluding root itself) whose status is not done.
func unresolvedDescendants(nodeStatus map[NodeID]NodeStatus, childrenByParent map[NodeID][]NodeID, root NodeID) int {
	seen := map[NodeID]bool{}
	count := 0
	stack := append([]NodeID(nil), childrenByParent[root]...)
	for len(stack) > 0 {
		cur := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if seen[cur] {
			continue
		}
		seen[cur] = true
		if st, ok := nodeStatus[cur]; ok && st != StatusDone {
			count++
		}
		stack = append(stack, childrenByParent[cur]...)
	}
	return count
}

// sortUmbrellaPressure orders pressure entries by node ID.
func sortUmbrellaPressure(entries []UmbrellaPressureEntry) {
	sort.Slice(entries, func(i, j int) bool { return entries[i].ID < entries[j].ID })
}

// FilterWarningsByNodeSet keeps warnings linked to nodes present in idSet.
func FilterWarningsByNodeSet(warnings []BranchWarning, idSet map[NodeID]struct{}) []BranchWarning {
	if len(idSet) == 0 || len(warnings) == 0 {
		return []BranchWarning{}
	}
	filtered := make([]BranchWarning, 0, len(warnings))
	for _, w := range warnings {
		if _, ok := idSet[w.ImpactedNode]; ok {
			filtered = append(filtered, w)
			continue
		}
		if _, ok := idSet[w.RootCauseNode]; ok {
			filtered = append(filtered, w)
		}
	}
	return filtered
}

// buildChildrenByParent constructs an adjacency map from parent links.
func buildChildrenByParent(nodes []*Node) map[NodeID][]NodeID {
	childrenByParent := map[NodeID][]NodeID{}
	for _, n := range nodes {
		for _, parent := range n.Parents {
			childrenByParent[parent] = append(childrenByParent[parent], n.ID)
		}
	}
	for parent := range childrenByParent {
		sort.Slice(childrenByParent[parent], func(i, j int) bool {
			return childrenByParent[parent][i] < childrenByParent[parent][j]
		})
	}
	return childrenByParent
}

// summarizeNodeWithChildren builds one lightweight node summary.
func summarizeNodeWithChildren(n *Node, children []NodeID) NodeSummary {
	return NodeSummary{
		ID:              n.ID,
		Title:           n.Title,
		Kind:            effectiveKind(n),
		Status:          normalizeStatus(n.Status),
		Outcome:         normalizeOutcome(n.Outcome),
		ClaimStatus:     normalizeClaimStatus(n.ClaimStatus),
		Agent:           n.Agent,
		Scope:           n.Scope,
		Tags:            append([]string(nil), n.Tags...),
		Revision:        n.Revision,
		MilestoneClass:  n.MilestoneClass,
		MilestoneKind:   n.MilestoneKind,
		MilestoneReason: n.MilestoneReason,
		Parents:         append([]NodeID(nil), n.Parents...),
		Children:        append([]NodeID(nil), children...),
	}
}

// latestRunValidity classifies the newest structured run.
func latestRunValidity(n *Node) string {
	if n == nil || len(n.Runs) == 0 {
		return "unknown"
	}
	last := n.Runs[len(n.Runs)-1]
	if last.Valid == nil {
		return "unknown"
	}
	if *last.Valid {
		return "valid"
	}
	return "invalid"
}

// ComputeHotness is the single source of truth for the hotspot formula.
// Both the status dashboard and the graph server must use it so the score
// never drifts between consumers.
func ComputeHotness(pendingChildren, ageDays, inconclusiveBonus int) int {
	return (pendingChildren * HotspotPendingChildWeight) + ageDays + inconclusiveBonus
}

// summarizeHotspot calculates a deterministic hotspot score per node.
func summarizeHotspot(n *Node, pendingCount, totalChildren int, now time.Time) HotspotSummary {
	ageDays := ageInDays(n.Created, now)
	outcome := normalizeOutcome(n.Outcome)
	inconclusiveBonus := 0
	if outcome == OutcomeInconclusive {
		inconclusiveBonus = HotspotInconclusiveOutcomeBonus
	}
	hotness := ComputeHotness(pendingCount, ageDays, inconclusiveBonus)
	return HotspotSummary{
		ID:                n.ID,
		Title:             n.Title,
		Status:            normalizeStatus(n.Status),
		Outcome:           outcome,
		ClaimStatus:       normalizeClaimStatus(n.ClaimStatus),
		MilestoneClass:    n.MilestoneClass,
		MilestoneKind:     n.MilestoneKind,
		MilestoneReason:   n.MilestoneReason,
		Agent:             n.Agent,
		PendingChildren:   pendingCount,
		AgeDays:           ageDays,
		PendingWeight:     HotspotPendingChildWeight,
		InconclusiveBonus: inconclusiveBonus,
		Hotness:           hotness,
	}
}

// sortNodeSummaries orders summaries by ID ascending.
func sortNodeSummaries(nodes []NodeSummary) {
	sort.Slice(nodes, func(i, j int) bool {
		return nodes[i].ID < nodes[j].ID
	})
}

// sortHotspots orders hotspots by score, then children, then age, then ID.
func sortHotspots(hotspots []HotspotSummary) {
	sort.Slice(hotspots, func(i, j int) bool {
		if hotspots[i].Hotness != hotspots[j].Hotness {
			return hotspots[i].Hotness > hotspots[j].Hotness
		}
		if hotspots[i].PendingChildren != hotspots[j].PendingChildren {
			return hotspots[i].PendingChildren > hotspots[j].PendingChildren
		}
		if hotspots[i].AgeDays != hotspots[j].AgeDays {
			return hotspots[i].AgeDays > hotspots[j].AgeDays
		}
		return hotspots[i].ID < hotspots[j].ID
	})
}

// normalizeStatus returns a safe status default.
func normalizeStatus(status NodeStatus) NodeStatus {
	switch status {
	case StatusDone, StatusPaused, StatusActive:
		return status
	default:
		return StatusActive
	}
}

// normalizeClaimStatus returns a safe claim-status default.
func normalizeClaimStatus(claim ClaimStatus) ClaimStatus {
	switch claim {
	case ClaimValidated, ClaimInvalidated, ClaimSuperseded, ClaimProvisional:
		return claim
	default:
		return ClaimProvisional
	}
}

// normalizeOutcome returns a safe outcome default.
func normalizeOutcome(outcome Outcome) Outcome {
	switch outcome {
	case OutcomeSuccess, OutcomeFailure, OutcomeInconclusive, OutcomeUnset:
		return outcome
	default:
		return OutcomeUnset
	}
}

// newOutcomeCounter creates an outcome counter initialized to zero.
func newOutcomeCounter() map[Outcome]int {
	return map[Outcome]int{
		OutcomeUnset:        0,
		OutcomeSuccess:      0,
		OutcomeFailure:      0,
		OutcomeInconclusive: 0,
	}
}

// ageInDays returns full UTC days between created and now.
func ageInDays(created, now time.Time) int {
	if created.IsZero() {
		return 0
	}
	delta := now.Sub(created)
	if delta < 0 {
		return 0
	}
	return int(delta.Hours() / 24)
}

// buildNodeStatusMap creates a lookup from NodeID to NodeStatus.
func buildNodeStatusMap(nodes []*Node) map[NodeID]NodeStatus {
	m := make(map[NodeID]NodeStatus, len(nodes))
	for _, n := range nodes {
		m[n.ID] = n.Status
	}
	return m
}

// countPendingChildren returns how many children are not done.
func countPendingChildren(statusMap map[NodeID]NodeStatus, childIDs []NodeID) int {
	count := 0
	for _, cid := range childIDs {
		s, ok := statusMap[cid]
		if ok && s != StatusDone {
			count++
		}
	}
	return count
}
