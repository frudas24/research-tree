package cmds

import (
	"fmt"
	"strings"

	"github.com/frudas24/research-tree/pkg/retree"
	"github.com/spf13/cobra"
)

type lintIssue struct {
	NodeID   retree.NodeID `json:"node_id"`
	Title    string        `json:"title"`
	Severity string        `json:"severity"`
	Message  string        `json:"message"`
}

// newLintCmd constructs the "lint" subcommand for hygiene auditing.
func newLintCmd(opts *RootOptions) *cobra.Command {
	var maxParents int
	cmd := &cobra.Command{
		Use:   "lint",
		Short: "Audit research graph hygiene",
		Long: `Scan all nodes for common hygiene issues:

- Nodes with too many parents (--max-parents, default 4)
- Primary parent not found in parents list
- Orphan relations (target node doesn't exist)
- Duplicate relations (same type+target pair)
- Isolated nodes (no parents and no children)
- Nodes failing structural validation`,
		RunE: func(cmd *cobra.Command, args []string) error {
			store, err := openStore(opts)
			if err != nil {
				return err
			}
			all, err := store.QueryNodes(retree.Filter{})
			if err != nil {
				return err
			}

			// Build indexes
			allIDs := make(map[retree.NodeID]*retree.Node, len(all))
			for _, n := range all {
				allIDs[n.ID] = n
			}

			issues := make([]lintIssue, 0)

			for _, n := range all {
				// Structural validation
				if err := retree.ValidateNode(n); err != nil {
					issues = append(issues, lintIssue{
						NodeID:   n.ID,
						Title:    n.Title,
						Severity: "error",
						Message:  fmt.Sprintf("validation failed: %v", err),
					})
				}

				// Too many parents
				if len(n.Parents) >= maxParents {
					issues = append(issues, lintIssue{
						NodeID:   n.ID,
						Title:    n.Title,
						Severity: "warning",
						Message:  fmt.Sprintf("has %d parents (threshold: %d) — consider using typed relations instead", len(n.Parents), maxParents),
					})
				}

				// Primary parent not in parents
				if n.PrimaryParent != nil {
					found := false
					for _, p := range n.Parents {
						if p == *n.PrimaryParent {
							found = true
							break
						}
					}
					if !found {
						issues = append(issues, lintIssue{
							NodeID:   n.ID,
							Title:    n.Title,
							Severity: "error",
							Message:  fmt.Sprintf("primary_parent=%d is not in parents list", *n.PrimaryParent),
						})
					}
				}

				// Orphan relations
				for _, rel := range n.Relations {
					if _, ok := allIDs[rel.Target]; !ok {
						issues = append(issues, lintIssue{
							NodeID:   n.ID,
							Title:    n.Title,
							Severity: "warning",
							Message:  fmt.Sprintf("relation %s:%d targets non-existent node", rel.Type, rel.Target),
						})
					}
				}

				// Duplicate relations
				seen := make(map[string]struct{})
				for _, rel := range n.Relations {
					key := fmt.Sprintf("%s:%d", rel.Type, rel.Target)
					if _, ok := seen[key]; ok {
						issues = append(issues, lintIssue{
							NodeID:   n.ID,
							Title:    n.Title,
							Severity: "warning",
							Message:  fmt.Sprintf("duplicate relation %s:%d", rel.Type, rel.Target),
						})
					}
					seen[key] = struct{}{}
				}

				// Isolated nodes (no parents, no children, not a root of a large tree)
				hasChildren := false
				for _, other := range all {
					for _, p := range other.Parents {
						if p == n.ID {
							hasChildren = true
							break
						}
					}
					if hasChildren {
						break
					}
				}
				if len(n.Parents) == 0 && !hasChildren && len(all) > 1 {
					issues = append(issues, lintIssue{
						NodeID:   n.ID,
						Title:    n.Title,
						Severity: "info",
						Message:  "isolated node: no parents and no children",
					})
				}
			}

			// Branch warnings from active invalidations
			warnings, err := store.ListBranchWarnings("", true)
			if err != nil {
				return err
			}
			for _, w := range warnings {
				issues = append(issues, lintIssue{
					NodeID:   w.ImpactedNode,
					Title:    fmt.Sprintf("(impacted by %04d)", w.RootCauseNode),
					Severity: "warning",
					Message:  w.Message,
				})
			}

			if opts.OutputJSON {
				return printMaybeJSON(cmd, true, issues, "")
			}

			// Human output grouped by severity
			if len(issues) == 0 {
				return printMaybeJSON(cmd, false, nil, "✓ No hygiene issues found.")
			}

			var b strings.Builder
			errCount, warnCount, infoCount := 0, 0, 0
			for _, iss := range issues {
				switch iss.Severity {
				case "error":
					errCount++
				case "warning":
					warnCount++
				case "info":
					infoCount++
				}
			}
			fmt.Fprintf(&b, "Issues: %d errors, %d warnings, %d info\n\n", errCount, warnCount, infoCount)

			// Sort by severity then ID
			sevOrder := map[string]int{"error": 0, "warning": 1, "info": 2}
			sorted := make([]lintIssue, len(issues))
			copy(sorted, issues)
			for i := range sorted {
				for j := i + 1; j < len(sorted); j++ {
					si, sj := sevOrder[sorted[i].Severity], sevOrder[sorted[j].Severity]
					if si > sj || (si == sj && sorted[i].NodeID > sorted[j].NodeID) {
						sorted[i], sorted[j] = sorted[j], sorted[i]
					}
				}
			}

			for _, iss := range sorted {
				sevLabel := strings.ToUpper(iss.Severity[:1])
				fmt.Fprintf(&b, "%s %04d %s: %s\n", sevLabel, iss.NodeID, iss.Title, iss.Message)
			}
			return printMaybeJSON(cmd, false, nil, b.String())
		},
	}
	cmd.Flags().IntVar(&maxParents, "max-parents", 4, "Warn when a node has this many or more parents")
	return cmd
}
