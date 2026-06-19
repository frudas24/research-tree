package cmds

import (
	"fmt"
	"sort"
	"strings"

	"github.com/frudas24/research-tree/pkg/retree"
	"github.com/spf13/cobra"
)

// newTreeCmd constructs the "tree" subcommand.
func newTreeCmd(opts *RootOptions) *cobra.Command {
	var depth int
	var status, claimStatus, evidenceStatus, evidenceCause string
	var flat, activeOnly, showRelations bool
	cmd := &cobra.Command{
		Use:   "tree [id]",
		Short: "Render tree view",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			store, err := openStore(opts)
			if err != nil {
				return err
			}
			cc := newColorizer(opts.ColorMode)
			nodes, err := store.QueryNodes(retree.Filter{
				Status:         parseNodeStatus(status),
				ClaimStatus:    parseClaimStatus(claimStatus),
				EvidenceStatus: parseEvidenceStatus(evidenceStatus),
				EvidenceCause:  parseEvidenceCause(evidenceCause),
			})
			if err != nil {
				return err
			}
			nodesByID := make(map[retree.NodeID]*retree.Node, len(nodes))
			for _, n := range nodes {
				nodesByID[n.ID] = n
			}
			children := map[retree.NodeID][]retree.NodeID{}
			for _, n := range nodes {
				structuralParents := n.Parents
				if n.PrimaryParent != nil {
					structuralParents = []retree.NodeID{*n.PrimaryParent}
				}
				for _, p := range structuralParents {
					if _, ok := nodesByID[p]; ok {
						children[p] = append(children[p], n.ID)
					}
				}
			}
			for id := range children {
				sort.Slice(children[id], func(i, j int) bool { return children[id][i] < children[id][j] })
			}

			var roots []retree.NodeID
			if len(args) == 1 {
				id, err := parseNodeID(args[0])
				if err != nil {
					return err
				}
				roots = []retree.NodeID{id}
			} else {
				for _, n := range nodes {
					isRoot := true
					structuralParents := n.Parents
					if n.PrimaryParent != nil {
						structuralParents = []retree.NodeID{*n.PrimaryParent}
					}
					for _, p := range structuralParents {
						if _, ok := nodesByID[p]; ok {
							isRoot = false
							break
						}
					}
					if isRoot {
						roots = append(roots, n.ID)
					}
				}
				sort.Slice(roots, func(i, j int) bool { return roots[i] < roots[j] })
			}

			// Pre-compute which branches have active nodes (for --active-only)
			hasActive := map[retree.NodeID]bool{}
			if activeOnly {
				var dfs func(id retree.NodeID) bool
				dfs = func(id retree.NodeID) bool {
					if v, ok := hasActive[id]; ok {
						return v
					}
					n, ok := nodesByID[id]
					if !ok {
						hasActive[id] = false
						return false
					}
					active := n.Status == retree.StatusActive
					for _, c := range children[id] {
						if dfs(c) {
							active = true
						}
					}
					hasActive[id] = active
					return active
				}
				for _, n := range nodes {
					dfs(n.ID)
				}
			}

			if opts.OutputJSON {
				edges := make([]map[string]retree.NodeID, 0)
				for p, cs := range children {
					for _, c := range cs {
						edges = append(edges, map[string]retree.NodeID{"from": p, "to": c})
					}
				}
				return printMaybeJSON(cmd, true, map[string]any{"roots": roots, "nodes": nodes, "edges": edges}, "")
			}

			lines := make([]string, 0)
			rendered := map[retree.NodeID]bool{}
			onPath := map[retree.NodeID]bool{}
			var walk func(id retree.NodeID, prefix string, d int)
			walk = func(id retree.NodeID, prefix string, d int) {
				n, ok := nodesByID[id]
				if !ok {
					return
				}
				if onPath[id] {
					lines = append(lines, prefix+fmt.Sprintf("%04d ...cycle-cut...", id))
					return
				}
				if rendered[id] {
					lines = append(lines, prefix+fmt.Sprintf("%04d ...ref-cut...", id))
					return
				}
				// Skip branch if --active-only and no active descendants
				if activeOnly && !hasActive[id] {
					return
				}
				onPath[id] = true
				rendered[id] = true
				title := cc.golden(n.MilestoneClass, titleWithVerdict(n))
				evidence := evidenceIcon(n)
				line := fmt.Sprintf("%04d | %s | %s | %s | [%s] %s", n.ID, cc.status(n.Status, statusIcon(n.Status)), cc.outcomeColor(n.Outcome, outcomeIcon(n)), evidence, n.Agent, title)
				if flat {
					lines = append(lines, strings.TrimSpace(line))
				} else {
					lines = append(lines, prefix+strings.TrimSpace(line))
					if showRelations && len(n.Relations) > 0 {
						relParts := make([]string, 0, len(n.Relations))
						for _, rel := range n.Relations {
							piece := fmt.Sprintf("%s:%04d", rel.Type, rel.Target)
							if rel.Note != "" {
								piece += fmt.Sprintf("(%s)", rel.Note)
							}
							relParts = append(relParts, piece)
						}
						lines = append(lines, prefix+"  ↬ "+strings.Join(relParts, ", "))
					}
				}
				if depth > 0 && d >= depth {
					return
				}
				for _, c := range children[id] {
					if activeOnly && !hasActive[c] {
						continue
					}
					np := prefix
					if !flat {
						np += "  "
					}
					walk(c, np, d+1)
				}
				delete(onPath, id)
			}
			for _, r := range roots {
				if activeOnly && !hasActive[r] {
					continue
				}
				walk(r, "", 1)
			}
			return printMaybeJSON(cmd, false, nil, strings.Join(lines, "\n"))
		},
	}
	cmd.Flags().IntVar(&depth, "depth", 0, "Max depth (0 = unlimited)")
	cmd.Flags().StringVar(&status, "status", "", "Filter by status")
	cmd.Flags().StringVar(&claimStatus, "claim-status", "", "Filter by claim status")
	cmd.Flags().StringVar(&evidenceStatus, "evidence-status", "", "Filter by evidence status")
	cmd.Flags().StringVar(&evidenceCause, "evidence-cause", "", "Filter by evidence cause")
	cmd.Flags().BoolVar(&flat, "flat", false, "Flat output")
	cmd.Flags().BoolVar(&activeOnly, "active-only", false, "Only show branches with active nodes")
	cmd.Flags().BoolVar(&showRelations, "show-relations", false, "Show non-structural relation hints inline in expanded tree mode")
	return cmd
}

// statusIcon returns the pure activity status icon.
func statusIcon(s retree.NodeStatus) string {
	switch s {
	case retree.StatusActive:
		return "▶"
	case retree.StatusPaused:
		return "⏸"
	default:
		return "✔"
	}
}

// outcomeIcon returns just the outcome emoji.
func outcomeIcon(n *retree.Node) string {
	switch n.Outcome {
	case retree.OutcomeSuccess:
		return "✔"
	case retree.OutcomeFailure:
		return "✗"
	case retree.OutcomeInconclusive:
		return "⏸"
	default:
		return "·"
	}
}

// evidenceIcon returns the evidence hygiene icon for compact tree output.
func evidenceIcon(n *retree.Node) string {
	switch n.EvidenceStatus {
	case retree.EvidencePoisoned:
		return "☣"
	case retree.EvidenceRevalidated:
		return "♻"
	case retree.EvidenceSuspect:
		return "?"
	default:
		return "·"
	}
}

// IconForNode returns an emoji icon for a node combining status and outcome.
func IconForNode(n *retree.Node) string {
	if n.Status == retree.StatusActive {
		return "▶"
	}
	if n.Status == retree.StatusPaused {
		return "⏸"
	}
	// Status done: icon based on outcome
	switch n.Outcome {
	case retree.OutcomeSuccess:
		return "✔"
	case retree.OutcomeFailure:
		return "✗"
	case retree.OutcomeInconclusive:
		return "⏸"
	default:
		return "✔"
	}
}
