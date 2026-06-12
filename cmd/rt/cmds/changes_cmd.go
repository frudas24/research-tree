package cmds

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/frudas24/research-tree/pkg/retree"
	"github.com/spf13/cobra"
)

// newChangesCmd constructs the "changes" subcommand.
func newChangesCmd(opts *RootOptions) *cobra.Command {
	var since, limit int
	cmd := &cobra.Command{
		Use:   "changes",
		Short: "Show recent node changes",
		Long:  `Show recently modified nodes. Use --since for hours, --limit for max rows.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			store, err := openStore(opts)
			if err != nil {
				return err
			}
			all, err := store.QueryNodes(retree.Filter{})
			if err != nil {
				return err
			}

			cutoff := time.Now().UTC().Add(-time.Duration(since) * time.Hour)
			type entry struct {
				ID       retree.NodeID
				Title    string
				Modified time.Time
				Status   string
			}
			var entries []entry
			for _, n := range all {
				if since > 0 && n.Modified.Before(cutoff) {
					continue
				}
				entries = append(entries, entry{n.ID, n.Title, n.Modified, string(n.Status)})
			}
			sort.Slice(entries, func(i, j int) bool { return entries[i].Modified.After(entries[j].Modified) })
			if limit > 0 && len(entries) > limit {
				entries = entries[:limit]
			}

			if opts.OutputJSON {
				return printMaybeJSON(cmd, true, entries, "")
			}
			var lines []string
			for _, e := range entries {
				lines = append(lines, fmt.Sprintf("%04d %s [%s] %s", e.ID, e.Modified.Format("01-02 15:04"), e.Status, e.Title))
			}
			return printMaybeJSON(cmd, false, nil, strings.Join(lines, "\n"))
		},
	}
	cmd.Flags().IntVar(&since, "since", 0, "Hours back (0 = all time)")
	cmd.Flags().IntVar(&limit, "limit", 20, "Max entries")
	return cmd
}
