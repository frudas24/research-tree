package cmds

import (
	"fmt"
	"strings"

	"github.com/frudas24/research-tree/pkg/retree"
	"github.com/spf13/cobra"
)

// newGoldenCmd constructs the "golden" subcommand as a human-friendly wrapper
// over the canonical milestone query.
func newGoldenCmd(opts *RootOptions) *cobra.Command {
	var kind, status, claimStatus, agent, sortBy, order string
	var limit int
	var verbose bool

	cmd := &cobra.Command{
		Use:   "golden",
		Short: "List golden milestone nodes",
		Long: `List golden milestone nodes using the canonical milestone metadata.

This is a human-oriented wrapper over:
  rt node list --milestone-class golden

It does not define a separate storage or query model.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			store, err := openStore(opts)
			if err != nil {
				return err
			}
			cc := newColorizer(opts.ColorMode)
			nodes, err := store.QueryNodes(retree.Filter{
				MilestoneClass: retree.MilestoneGolden,
				MilestoneKind:  retree.MilestoneKind(strings.TrimSpace(kind)),
				Status:         parseNodeStatus(status),
				ClaimStatus:    parseClaimStatus(claimStatus),
				Agent:          strings.TrimSpace(agent),
				SortBy:         strings.TrimSpace(sortBy),
				Order:          strings.TrimSpace(order),
				Limit:          limit,
			})
			if err != nil {
				return err
			}
			if opts.OutputJSON {
				return printMaybeJSON(cmd, true, nodes, "")
			}
			lines := make([]string, 0, len(nodes)*2+1)
			for _, n := range nodes {
				title := cc.golden(n.MilestoneClass, titleWithVerdict(n))
				kindLabel := "golden"
				if n.MilestoneKind != "" {
					kindLabel += "/" + string(n.MilestoneKind)
				}
				lines = append(lines, fmt.Sprintf("%04d | %s | %s | %s | %s",
					n.ID, kindLabel, n.Status, n.ClaimStatus, title,
				))
				if verbose && strings.TrimSpace(n.MilestoneReason) != "" {
					lines = append(lines, "  reason: "+n.MilestoneReason)
				}
			}
			if len(lines) == 0 {
				lines = append(lines, "no golden nodes found")
			}
			return printMaybeJSON(cmd, false, nil, strings.Join(lines, "\n"))
		},
	}

	cmd.Flags().StringVar(&kind, "kind", "", "Filter by golden kind: champion|breakthrough|pivot")
	cmd.Flags().StringVar(&status, "status", "", "Filter by status")
	cmd.Flags().StringVar(&claimStatus, "claim-status", "", "Filter by claim status")
	cmd.Flags().StringVar(&agent, "agent", "", "Filter by agent")
	cmd.Flags().StringVar(&sortBy, "sort-by", "modified", "Sort by: id|created|modified|title")
	cmd.Flags().StringVar(&order, "order", "desc", "Sort order: asc|desc")
	cmd.Flags().IntVar(&limit, "limit", 50, "Maximum rows")
	cmd.Flags().BoolVar(&verbose, "verbose", false, "Show milestone reasons")
	return cmd
}
