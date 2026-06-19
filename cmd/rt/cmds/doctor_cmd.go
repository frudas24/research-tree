package cmds

import (
	"fmt"
	"sort"
	"strings"

	"github.com/frudas24/research-tree/pkg/retree"
	"github.com/spf13/cobra"
)

type doctorIssue struct {
	NodeID   retree.NodeID `json:"node_id"`
	Title    string        `json:"title"`
	Severity string        `json:"severity"`
	Message  string        `json:"message"`
}

// newDoctorCmd constructs the "doctor" subcommand for higher-level structural diagnostics.
func newDoctorCmd(opts *RootOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Higher-level structural diagnostics",
	}
	cmd.AddCommand(newDoctorLineageCmd(opts))
	cmd.AddCommand(newDoctorEvidenceCmd(opts))
	return cmd
}

// newDoctorLineageCmd constructs the "doctor lineage" subcommand for structural parent hygiene.
func newDoctorLineageCmd(opts *RootOptions) *cobra.Command {
	var strict bool
	cmd := &cobra.Command{
		Use:   "lineage",
		Short: "Audit structural lineage vs matrix/relation misuse",
		Long: `Detect lineage structures that should probably be modeled with
primary_parent + relations instead of multiple structural parents.

This command is stricter than lint. It focuses on research hygiene:

- multiple parents without primary_parent
- multiple parents combined with comparison/aggregation relations
- poisoned nodes still used as structural ancestors
- revalidated nodes that still remain structural hubs
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			store, err := openStore(opts)
			if err != nil {
				return err
			}
			all, err := store.QueryNodes(retree.Filter{})
			if err != nil {
				return err
			}
			idSet := make(map[retree.NodeID]*retree.Node, len(all))
			children := make(map[retree.NodeID][]retree.NodeID)
			for _, n := range all {
				idSet[n.ID] = n
			}
			for _, n := range all {
				structuralParents := n.Parents
				if n.PrimaryParent != nil {
					structuralParents = []retree.NodeID{*n.PrimaryParent}
				}
				for _, p := range structuralParents {
					if _, ok := idSet[p]; ok {
						children[p] = append(children[p], n.ID)
					}
				}
			}

			issues := make([]doctorIssue, 0)
			for _, n := range all {
				if len(n.Parents) > 1 && n.PrimaryParent == nil {
					sev := "warning"
					if strict {
						sev = "error"
					}
					issues = append(issues, doctorIssue{
						NodeID:   n.ID,
						Title:    n.Title,
						Severity: sev,
						Message:  fmt.Sprintf("has %d structural parents but no primary_parent; use one structural parent and move matrix context to relations", len(n.Parents)),
					})
				}

				if len(n.Parents) > 1 && len(n.Relations) > 0 {
					hasMatrixLike := false
					for _, rel := range n.Relations {
						if rel.Type == retree.RelComparesAgainst || rel.Type == retree.RelAggregates || rel.Type == retree.RelInspiredBy {
							hasMatrixLike = true
							break
						}
					}
					if hasMatrixLike {
						issues = append(issues, doctorIssue{
							NodeID:   n.ID,
							Title:    n.Title,
							Severity: "warning",
							Message:  "mixes multiple structural parents with matrix-style relations; likely wants primary_parent + relations instead",
						})
					}
				}

				if n.EvidenceStatus == retree.EvidencePoisoned && len(children[n.ID]) > 0 {
					issues = append(issues, doctorIssue{
						NodeID:   n.ID,
						Title:    n.Title,
						Severity: "warning",
						Message:  fmt.Sprintf("poisoned node still acts as structural ancestor for %d child nodes", len(children[n.ID])),
					})
				}

				if n.EvidenceStatus == retree.EvidenceRevalidated && len(children[n.ID]) > 0 {
					issues = append(issues, doctorIssue{
						NodeID:   n.ID,
						Title:    n.Title,
						Severity: "info",
						Message:  fmt.Sprintf("revalidated node remains structural ancestor for %d child nodes; consider shifting doctrine to clean rerun branch", len(children[n.ID])),
					})
				}
			}

			if opts.OutputJSON {
				return printMaybeJSON(cmd, true, issues, "")
			}
			if len(issues) == 0 {
				return printMaybeJSON(cmd, false, nil, "✓ No lineage hygiene issues found.")
			}
			sort.SliceStable(issues, func(i, j int) bool {
				order := map[string]int{"error": 0, "warning": 1, "info": 2}
				if order[issues[i].Severity] != order[issues[j].Severity] {
					return order[issues[i].Severity] < order[issues[j].Severity]
				}
				return issues[i].NodeID < issues[j].NodeID
			})
			var b strings.Builder
			for _, iss := range issues {
				fmt.Fprintf(&b, "%s %04d %s: %s\n", strings.ToUpper(iss.Severity[:1]), iss.NodeID, iss.Title, iss.Message)
			}
			return printMaybeJSON(cmd, false, nil, b.String())
		},
	}
	cmd.Flags().BoolVar(&strict, "strict", false, "Treat multi-parent-without-primary-parent as error")
	return cmd
}

// newDoctorEvidenceCmd constructs the "doctor evidence" subcommand for poisoned/revalidated evidence hygiene.
func newDoctorEvidenceCmd(opts *RootOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "evidence",
		Short: "Audit poisoned/revalidated evidence hygiene",
		Long: `Detect evidence hygiene issues that should not be expressed as
claim invalidation alone.

This command flags:

- poisoned nodes without clean revalidation
- active nodes whose structural ancestors are poisoned
- nodes marked revalidated but without explicit clean replacement links
- doctrine-like nodes built on top of poisoned evidence
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			store, err := openStore(opts)
			if err != nil {
				return err
			}
			all, err := store.QueryNodes(retree.Filter{})
			if err != nil {
				return err
			}
			nodes := make(map[retree.NodeID]*retree.Node, len(all))
			children := make(map[retree.NodeID][]retree.NodeID)
			for _, n := range all {
				nodes[n.ID] = n
			}
			for _, n := range all {
				structuralParents := n.Parents
				if n.PrimaryParent != nil {
					structuralParents = []retree.NodeID{*n.PrimaryParent}
				}
				for _, p := range structuralParents {
					if _, ok := nodes[p]; ok {
						children[p] = append(children[p], n.ID)
					}
				}
			}

			isDoctrineLike := func(n *retree.Node) bool {
				if n == nil {
					return false
				}
				for _, tag := range n.Tags {
					switch tag {
					case "doctrine", "report", "summary", "baseline", "golden":
						return true
					}
				}
				return n.MilestoneClass == retree.MilestoneGolden || strings.Contains(strings.ToLower(n.Title), "doctrine") || strings.Contains(strings.ToLower(n.Title), "report")
			}

			hasPoisonedAncestor := func(id retree.NodeID) (bool, []retree.NodeID) {
				seen := map[retree.NodeID]struct{}{}
				var hits []retree.NodeID
				var walk func(retree.NodeID)
				walk = func(cur retree.NodeID) {
					if _, ok := seen[cur]; ok {
						return
					}
					seen[cur] = struct{}{}
					n := nodes[cur]
					if n == nil {
						return
					}
					for _, p := range n.Parents {
						parent := nodes[p]
						if parent == nil {
							continue
						}
						if parent.EvidenceStatus == retree.EvidencePoisoned {
							hits = append(hits, parent.ID)
						}
						walk(parent.ID)
					}
				}
				walk(id)
				sort.Slice(hits, func(i, j int) bool { return hits[i] < hits[j] })
				if len(hits) == 0 {
					return false, nil
				}
				uniq := make([]retree.NodeID, 0, len(hits))
				var last retree.NodeID
				for i, h := range hits {
					if i == 0 || h != last {
						uniq = append(uniq, h)
					}
					last = h
				}
				return true, uniq
			}

			issues := make([]doctorIssue, 0)
			for _, n := range all {
				if n.EvidenceStatus == retree.EvidencePoisoned && len(n.RevalidatedBy) == 0 {
					issues = append(issues, doctorIssue{
						NodeID:   n.ID,
						Title:    n.Title,
						Severity: "warning",
						Message:  "poisoned evidence has no clean revalidated_by replacement yet",
					})
				}
				if n.EvidenceStatus == retree.EvidenceRevalidated && len(n.RevalidatedBy) == 0 {
					issues = append(issues, doctorIssue{
						NodeID:   n.ID,
						Title:    n.Title,
						Severity: "error",
						Message:  "evidence marked revalidated but revalidated_by is empty",
					})
				}
				if n.Status == retree.StatusActive {
					if poisoned, anc := hasPoisonedAncestor(n.ID); poisoned {
						issues = append(issues, doctorIssue{
							NodeID:   n.ID,
							Title:    n.Title,
							Severity: "warning",
							Message:  fmt.Sprintf("active node depends structurally on poisoned ancestor(s): %v", anc),
						})
					}
				}
				if isDoctrineLike(n) {
					if poisoned, anc := hasPoisonedAncestor(n.ID); poisoned {
						issues = append(issues, doctorIssue{
							NodeID:   n.ID,
							Title:    n.Title,
							Severity: "warning",
							Message:  fmt.Sprintf("doctrine/report node is built on poisoned ancestor(s): %v", anc),
						})
					}
				}
				if n.EvidenceStatus == retree.EvidencePoisoned && len(children[n.ID]) == 0 && len(n.RevalidatedBy) > 0 {
					issues = append(issues, doctorIssue{
						NodeID:   n.ID,
						Title:    n.Title,
						Severity: "info",
						Message:  "poisoned node already has clean replacement and no structural descendants; eligible for archival cleanup",
					})
				}
			}

			if opts.OutputJSON {
				return printMaybeJSON(cmd, true, issues, "")
			}
			if len(issues) == 0 {
				return printMaybeJSON(cmd, false, nil, "✓ No evidence hygiene issues found.")
			}
			sort.SliceStable(issues, func(i, j int) bool {
				order := map[string]int{"error": 0, "warning": 1, "info": 2}
				if order[issues[i].Severity] != order[issues[j].Severity] {
					return order[issues[i].Severity] < order[issues[j].Severity]
				}
				return issues[i].NodeID < issues[j].NodeID
			})
			var b strings.Builder
			for _, iss := range issues {
				fmt.Fprintf(&b, "%s %04d %s: %s\n", strings.ToUpper(iss.Severity[:1]), iss.NodeID, iss.Title, iss.Message)
			}
			return printMaybeJSON(cmd, false, nil, b.String())
		},
	}
	return cmd
}
