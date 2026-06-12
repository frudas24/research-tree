package cmds

import (
	"fmt"
	"os/user"
	"sort"
	"strings"

	"github.com/frudas24/research-tree/pkg/retree"
	"github.com/spf13/cobra"
)

const (
	statusSectionOverview = "overview"
	statusSectionActive   = "active"
	statusSectionDone     = "done"
	statusSectionPaused   = "paused"
	statusSectionClaim    = "claim"
	statusSectionRuns     = "runs"
	statusSectionWarnings = "warnings"
	statusSectionHotspots = "hotspots"
	statusSectionMatrix   = "matrix"
)

// newStatusCmd constructs the "status" subcommand.
func newStatusCmd(opts *RootOptions) *cobra.Command {
	var agent string
	var mine bool
	var verbose bool
	var limit int
	var section string
	var tag string
	var scopeContains string
	var showMatrix bool

	cmd := &cobra.Command{
		Use:   "status",
		Short: "Status dashboard",
		Long: `Show research tree status with scalable summaries by default.

Use --verbose to print full node lists.
Use --section to focus on one or more sections (comma-separated).
Use --matrix to render status×outcome counts in text output.
Use --tag, --scope-contains, and --agent to narrow the scope.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			store, err := openStore(opts)
			cc := newColorizer(opts.ColorMode)
			if err != nil {
				return err
			}

			// Resolve agent filter.
			filterAgent := agent
			if mine {
				u, err := user.Current()
				if err == nil {
					filterAgent = u.Username
				}
			}

			filter := buildStatusFilter(filterAgent, tag, scopeContains)
			all, err := store.QueryNodes(filter)
			if err != nil {
				return err
			}
			warnings, _ := store.ListBranchWarnings(filterAgent, true)
			if tag != "" {
				idSet := make(map[retree.NodeID]struct{}, len(all))
				for _, n := range all {
					idSet[n.ID] = struct{}{}
				}
				warnings = retree.FilterWarningsByNodeSet(warnings, idSet)
			}

			summary := retree.BuildStatusSummary(all, warnings, retree.StatusBuildOptions{
				Agent:        filterAgent,
				HotspotLimit: 10,
			})

			if opts.OutputJSON {
				return printMaybeJSON(cmd, true, summary, "")
			}

			sections := parseSectionSet(section)
			return printMaybeJSON(cmd, false, nil, renderStatusText(cc, summary, statusRenderOptions{
				Agent:      filterAgent,
				Tag:        tag,
				Scope:      scopeContains,
				Verbose:    verbose,
				Limit:      limit,
				Sections:   sections,
				ShowMatrix: showMatrix,
			}))
		},
	}
	cmd.Flags().StringVar(&agent, "agent", "", "Filter by agent")
	cmd.Flags().BoolVar(&mine, "mine", false, "Show only your own nodes (OS username)")
	cmd.Flags().StringVar(&tag, "tag", "", "Filter by tag")
	cmd.Flags().StringVar(&scopeContains, "scope-contains", "", "Filter by scope substring")
	cmd.Flags().BoolVar(&verbose, "verbose", false, "Print detailed node lists")
	cmd.Flags().IntVar(&limit, "limit", 0, "Limit detailed rows per section (0 = no limit)")
	cmd.Flags().StringVar(&section, "section", "", "Comma-separated sections: overview,active,done,paused,claim,runs,warnings,hotspots,matrix")
	cmd.Flags().BoolVar(&showMatrix, "matrix", false, "Render status×outcome matrix in text output")
	return cmd
}

// statusRenderOptions controls text rendering behavior.
type statusRenderOptions struct {
	Agent      string
	Tag        string
	Scope      string
	Verbose    bool
	Limit      int
	Sections   map[string]struct{}
	ShowMatrix bool
}

// buildStatusFilter creates a node query filter from status flags.
func buildStatusFilter(agent string, tag string, scopeContains string) retree.Filter {
	f := retree.Filter{}
	if agent != "" {
		f.Agent = agent
	}
	if tag != "" {
		f.Tag = tag
	}
	if scopeContains != "" {
		f.ScopeContains = scopeContains
	}
	return f
}

// parseSectionSet parses a comma-separated section list.
func parseSectionSet(raw string) map[string]struct{} {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	set := map[string]struct{}{}
	for _, piece := range strings.Split(raw, ",") {
		name := strings.ToLower(strings.TrimSpace(piece))
		if name == "" {
			continue
		}
		set[name] = struct{}{}
	}
	return set
}

// shouldRenderSection returns whether one section should be rendered.
func shouldRenderSection(section string, selected map[string]struct{}) bool {
	if len(selected) == 0 {
		return true
	}
	_, ok := selected[section]
	return ok
}

// renderStatusText builds the human-readable status dashboard.
func renderStatusText(cc *colorizer, summary retree.StatusSummary, opts statusRenderOptions) string {
	lines := make([]string, 0, 64)
	header := fmt.Sprintf("Nodes: %d total | %d active | %d done | %d paused", summary.Total, len(summary.Active), len(summary.Done), len(summary.Paused))
	if opts.Agent != "" {
		header += fmt.Sprintf("  (agent: %s)", opts.Agent)
	}
	if opts.Tag != "" {
		header += fmt.Sprintf("  (tag: %s)", opts.Tag)
	}
	if opts.Scope != "" {
		header += fmt.Sprintf("  (scope: %s)", opts.Scope)
	}
	lines = append(lines, header)
	lines = append(lines, "")

	detailMode := opts.Verbose || len(opts.Sections) > 0 || summary.Total <= 20

	if shouldRenderSection(statusSectionOverview, opts.Sections) {
		roots, branching, leaves := summarizeActiveTopology(summary.Active)
		lines = append(lines, "Active Overview:")
		lines = append(lines, fmt.Sprintf("  roots: %d | with pending children: %d | leaves: %d", roots, branching, leaves))
		lines = append(lines, "")
	}

	if shouldRenderSection(statusSectionDone, opts.Sections) {
		doneOutcomes := countOutcomes(summary.Done)
		lines = append(lines, "Done Outcomes:")
		lines = append(lines, fmt.Sprintf("  success: %d | failure: %d | inconclusive: %d | unset: %d",
			doneOutcomes[retree.OutcomeSuccess],
			doneOutcomes[retree.OutcomeFailure],
			doneOutcomes[retree.OutcomeInconclusive],
			doneOutcomes[retree.OutcomeUnset],
		))
		lines = append(lines, "")
	}

	if shouldRenderSection(statusSectionPaused, opts.Sections) {
		pausedOutcomes := countOutcomes(summary.Paused)
		lines = append(lines, "Paused Outcomes:")
		lines = append(lines, fmt.Sprintf("  inconclusive: %d | unset: %d | success: %d | failure: %d",
			pausedOutcomes[retree.OutcomeInconclusive],
			pausedOutcomes[retree.OutcomeUnset],
			pausedOutcomes[retree.OutcomeSuccess],
			pausedOutcomes[retree.OutcomeFailure],
		))
		lines = append(lines, "")
	}

	if shouldRenderSection(statusSectionClaim, opts.Sections) {
		lines = append(lines, "Claim Status:")
		lines = append(lines, fmt.Sprintf("  validated: %d | invalidated: %d | provisional: %d | superseded: %d",
			summary.ClaimStatusCounts[retree.ClaimValidated],
			summary.ClaimStatusCounts[retree.ClaimInvalidated],
			summary.ClaimStatusCounts[retree.ClaimProvisional],
			summary.ClaimStatusCounts[retree.ClaimSuperseded],
		))
		lines = append(lines, "")
	}

	if shouldRenderSection(statusSectionRuns, opts.Sections) {
		lines = append(lines, "Run Validity:")
		lines = append(lines, fmt.Sprintf("  valid: %d | invalid: %d | unknown: %d",
			summary.RunValidityCounts["valid"],
			summary.RunValidityCounts["invalid"],
			summary.RunValidityCounts["unknown"],
		))
		lines = append(lines, "")
	}

	if shouldRenderSection(statusSectionHotspots, opts.Sections) {
		lines = append(lines, "Top Hotspots:")
		lines = append(lines, fmt.Sprintf("  formula: %s", summary.HotspotFormula))
		if len(summary.Hotspots) == 0 {
			lines = append(lines, "  none")
		} else {
			for _, h := range applyHotspotLimit(summary.Hotspots, opts.Limit) {
				title := h.Title
				if h.MilestoneClass == retree.MilestoneGolden {
					title = "★ " + title
					title = cc.golden(h.MilestoneClass, title)
				}
				lines = append(lines, fmt.Sprintf("  %04d hot=%d pending=%d*%d age=%dd bonus=%d %s [%s]",
					h.ID, h.Hotness, h.PendingChildren, h.PendingWeight, h.AgeDays, h.InconclusiveBonus, title, h.Agent,
				))
			}
		}
		lines = append(lines, "")
	}

	if shouldRenderSection(statusSectionWarnings, opts.Sections) {
		lines = append(lines, fmt.Sprintf("Warnings: %d unacked", len(summary.Warnings)))
		if detailMode && len(summary.Warnings) > 0 {
			warnings := append([]retree.BranchWarning(nil), summary.Warnings...)
			sort.Slice(warnings, func(i, j int) bool { return warnings[i].CreatedAt.Before(warnings[j].CreatedAt) })
			for _, w := range applyWarningLimit(warnings, opts.Limit) {
				lines = append(lines, fmt.Sprintf("  %s %s", w.ID, w.Message))
			}
		}
		lines = append(lines, "")
	}

	if opts.ShowMatrix || shouldRenderSection(statusSectionMatrix, opts.Sections) {
		if shouldRenderSection(statusSectionMatrix, opts.Sections) || len(opts.Sections) == 0 || opts.ShowMatrix {
			lines = append(lines, "Status x Outcome Matrix:")
			lines = append(lines, renderMatrixLine(summary.Matrix, retree.StatusActive))
			lines = append(lines, renderMatrixLine(summary.Matrix, retree.StatusDone))
			lines = append(lines, renderMatrixLine(summary.Matrix, retree.StatusPaused))
			lines = append(lines, "")
		}
	}

	if detailMode {
		if shouldRenderSection(statusSectionActive, opts.Sections) {
			appendNodeSection(&lines, cc, "Active:", summary.Active, opts.Limit)
		}
		if shouldRenderSection(statusSectionDone, opts.Sections) {
			appendNodeSection(&lines, cc, "Done (completed):", summary.Done, opts.Limit)
		}
		if shouldRenderSection(statusSectionPaused, opts.Sections) {
			appendNodeSection(&lines, cc, "Paused:", summary.Paused, opts.Limit)
		}
	}

	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return strings.Join(lines, "\n")
}

// summarizeActiveTopology returns active roots, branching nodes, and active leaves.
func summarizeActiveTopology(active []retree.NodeSummary) (roots int, branching int, leaves int) {
	for _, n := range active {
		if len(n.Parents) == 0 {
			roots++
		}
		if len(n.Children) > 0 {
			branching++
		} else {
			leaves++
		}
	}
	return roots, branching, leaves
}

// countOutcomes counts outcomes in a node summary slice.
func countOutcomes(nodes []retree.NodeSummary) map[retree.Outcome]int {
	counts := map[retree.Outcome]int{
		retree.OutcomeUnset:        0,
		retree.OutcomeSuccess:      0,
		retree.OutcomeFailure:      0,
		retree.OutcomeInconclusive: 0,
	}
	for _, n := range nodes {
		counts[n.Outcome]++
	}
	return counts
}

// renderMatrixLine renders one status row of the outcome matrix.
func renderMatrixLine(matrix map[retree.NodeStatus]map[retree.Outcome]int, status retree.NodeStatus) string {
	row := matrix[status]
	if row == nil {
		row = map[retree.Outcome]int{}
	}
	return fmt.Sprintf("  %-6s success=%d failure=%d inconclusive=%d unset=%d",
		status,
		row[retree.OutcomeSuccess],
		row[retree.OutcomeFailure],
		row[retree.OutcomeInconclusive],
		row[retree.OutcomeUnset],
	)
}

// appendNodeSection adds one detailed node section when requested.
func appendNodeSection(lines *[]string, cc *colorizer, title string, nodes []retree.NodeSummary, limit int) {
	if len(nodes) == 0 {
		return
	}
	*lines = append(*lines, title)
	for _, n := range applyNodeLimit(nodes, limit) {
		icon := cc.status(n.Status, iconForSummary(n))
		title := n.Title
		if n.MilestoneClass == retree.MilestoneGolden {
			title = "★ " + title
			title = cc.golden(n.MilestoneClass, title)
		}
		tagSuffix := ""
		if len(n.Tags) > 0 {
			tagSuffix = fmt.Sprintf(" tags=[%s]", strings.Join(n.Tags, ","))
		}
		*lines = append(*lines, fmt.Sprintf("%s %04d %s [%s]%s", icon, n.ID, title, n.Agent, tagSuffix))
	}
	if limit > 0 && len(nodes) > limit {
		*lines = append(*lines, fmt.Sprintf("  ... %d more", len(nodes)-limit))
	}
	*lines = append(*lines, "")
}

// iconForSummary maps a node summary to its display icon.
func iconForSummary(n retree.NodeSummary) string {
	if n.Status == retree.StatusActive {
		return "▶"
	}
	if n.Status == retree.StatusPaused {
		return "⏸"
	}
	switch n.Outcome {
	case retree.OutcomeFailure:
		return "✗"
	case retree.OutcomeInconclusive:
		return "⏸"
	default:
		return "✔"
	}
}

// applyNodeLimit truncates detailed node sections.
func applyNodeLimit(nodes []retree.NodeSummary, limit int) []retree.NodeSummary {
	if limit <= 0 || len(nodes) <= limit {
		return nodes
	}
	return nodes[:limit]
}

// applyHotspotLimit truncates hotspot rows.
func applyHotspotLimit(hotspots []retree.HotspotSummary, limit int) []retree.HotspotSummary {
	if limit <= 0 || len(hotspots) <= limit {
		return hotspots
	}
	return hotspots[:limit]
}

// applyWarningLimit truncates warning rows.
func applyWarningLimit(warnings []retree.BranchWarning, limit int) []retree.BranchWarning {
	if limit <= 0 || len(warnings) <= limit {
		return warnings
	}
	return warnings[:limit]
}
