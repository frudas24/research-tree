package retree

import (
	"testing"
	"time"
)

// TestBuildStatusSummaryCounts verifies status/claim/outcome counters and matrix rows.
func TestBuildStatusSummaryCounts(t *testing.T) {
	now := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	nodes := []*Node{
		{Frontmatter: Frontmatter{ID: 1, Title: "a", Status: StatusActive, ClaimStatus: ClaimProvisional, Outcome: OutcomeUnset, Created: now.AddDate(0, 0, -2), MilestoneClass: MilestoneGolden, MilestoneKind: MilestoneKindBreakthrough, MilestoneReason: "frontier moved"}},
		{Frontmatter: Frontmatter{ID: 2, Title: "b", Status: StatusDone, ClaimStatus: ClaimValidated, Outcome: OutcomeSuccess, Parents: []NodeID{1}, Created: now.AddDate(0, 0, -5)}},
		{Frontmatter: Frontmatter{ID: 3, Title: "c", Status: StatusPaused, ClaimStatus: ClaimInvalidated, Outcome: OutcomeInconclusive, Parents: []NodeID{1}, Created: now.AddDate(0, 0, -1)}},
	}
	warnings := []BranchWarning{{ID: "w1", RootCauseNode: 2, ImpactedNode: 3}}
	summary := BuildStatusSummary(nodes, warnings, StatusBuildOptions{Agent: "opus", HotspotLimit: 10, Now: now})

	if summary.Total != 3 {
		t.Fatalf("total=%d", summary.Total)
	}
	if summary.StatusCounts[StatusActive] != 1 || summary.StatusCounts[StatusDone] != 1 || summary.StatusCounts[StatusPaused] != 1 {
		t.Fatalf("status_counts=%v", summary.StatusCounts)
	}
	if summary.ClaimStatusCounts[ClaimValidated] != 1 || summary.ClaimStatusCounts[ClaimInvalidated] != 1 {
		t.Fatalf("claim_status_counts=%v", summary.ClaimStatusCounts)
	}
	if summary.OutcomeCounts[OutcomeSuccess] != 1 || summary.OutcomeCounts[OutcomeInconclusive] != 1 || summary.OutcomeCounts[OutcomeUnset] != 1 {
		t.Fatalf("outcome_counts=%v", summary.OutcomeCounts)
	}
	if summary.Matrix[StatusDone][OutcomeSuccess] != 1 {
		t.Fatalf("matrix done/success=%d", summary.Matrix[StatusDone][OutcomeSuccess])
	}
	if summary.Matrix[StatusPaused][OutcomeInconclusive] != 1 {
		t.Fatalf("matrix paused/inconclusive=%d", summary.Matrix[StatusPaused][OutcomeInconclusive])
	}
	if summary.Agent != "opus" {
		t.Fatalf("agent=%q", summary.Agent)
	}
	if len(summary.Warnings) != 1 {
		t.Fatalf("warnings=%d", len(summary.Warnings))
	}
	if len(summary.Active) != 1 || summary.Active[0].MilestoneClass != MilestoneGolden || summary.Active[0].MilestoneKind != MilestoneKindBreakthrough || summary.Active[0].MilestoneReason != "frontier moved" {
		t.Fatalf("active milestone summary mismatch: %+v", summary.Active)
	}
}

// TestSummarizeNodesChildren verifies child edges are derived from parent links.
func TestSummarizeNodesChildren(t *testing.T) {
	nodes := []*Node{
		{Frontmatter: Frontmatter{ID: 1, Title: "root", Status: StatusActive}},
		{Frontmatter: Frontmatter{ID: 2, Title: "child-a", Status: StatusActive, Parents: []NodeID{1}}},
		{Frontmatter: Frontmatter{ID: 3, Title: "child-b", Status: StatusActive, Parents: []NodeID{1}}},
	}
	summaries := SummarizeNodes(nodes)
	if len(summaries) != 3 {
		t.Fatalf("summaries=%d", len(summaries))
	}
	if len(summaries[0].Children) != 2 {
		t.Fatalf("root children=%v", summaries[0].Children)
	}
}

// TestFilterWarningsByNodeSet verifies warning filtering by node membership.
func TestFilterWarningsByNodeSet(t *testing.T) {
	warnings := []BranchWarning{
		{ID: "w1", RootCauseNode: 1, ImpactedNode: 2},
		{ID: "w2", RootCauseNode: 3, ImpactedNode: 4},
	}
	set := map[NodeID]struct{}{2: {}, 9: {}}
	filtered := FilterWarningsByNodeSet(warnings, set)
	if len(filtered) != 1 || filtered[0].ID != "w1" {
		t.Fatalf("filtered=%v", filtered)
	}
}

// TestBuildStatusSummaryHotspotLimit verifies deterministic hotspot ordering and cap.
func TestBuildStatusSummaryHotspotLimit(t *testing.T) {
	now := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	nodes := []*Node{
		{Frontmatter: Frontmatter{ID: 1, Title: "a", Status: StatusActive, Created: now.AddDate(0, 0, -1)}},
		{Frontmatter: Frontmatter{ID: 2, Title: "b", Status: StatusActive, Created: now.AddDate(0, 0, -10)}},
		{Frontmatter: Frontmatter{ID: 3, Title: "c", Status: StatusActive, Created: now.AddDate(0, 0, -3), Parents: []NodeID{2}}},
	}
	summary := BuildStatusSummary(nodes, nil, StatusBuildOptions{Now: now, HotspotLimit: 2})
	if len(summary.Hotspots) != 2 {
		t.Fatalf("hotspots=%d", len(summary.Hotspots))
	}
	if summary.Hotspots[0].ID != 2 {
		t.Fatalf("expected node 2 first hotspot, got %d", summary.Hotspots[0].ID)
	}
}
