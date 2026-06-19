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

func newDoctorCmd(opts *RootOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Higher-level structural diagnostics",
	}
	cmd.AddCommand(newDoctorLineageCmd(opts))
	return cmd
}

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
