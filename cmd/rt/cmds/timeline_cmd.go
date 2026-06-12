package cmds

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/frudas24/research-tree/pkg/retree"
	"github.com/spf13/cobra"
)

// newTimelineCmd constructs the "timeline" subcommand.
func newTimelineCmd(opts *RootOptions) *cobra.Command {
	var days, limit int
	cmd := &cobra.Command{
		Use:   "timeline",
		Short: "Show timeline of node activity",
		Long:  `Show nodes ordered by modification time, grouped by day.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			store, err := openStore(opts)
			if err != nil {
				return err
			}
			all, err := store.QueryNodes(retree.Filter{})
			if err != nil {
				return err
			}
			cutoff := time.Now().UTC().Add(-time.Duration(days) * 24 * time.Hour)
			type entry struct {
				ID       retree.NodeID
				Title    string
				Modified time.Time
				Status   string
				Outcome  string
			}
			var entries []entry
			for _, n := range all {
				if n.Modified.IsZero() || n.Modified.Before(cutoff) {
					continue
				}
				entries = append(entries, entry{n.ID, n.Title, n.Modified, string(n.Status), string(n.Outcome)})
			}
			sort.Slice(entries, func(i, j int) bool { return entries[i].Modified.After(entries[j].Modified) })
			if limit > 0 && len(entries) > limit {
				entries = entries[:limit]
			}
			if opts.OutputJSON {
				return printMaybeJSON(cmd, true, entries, "")
			}

			var lines []string
			currentDay := ""
			for _, e := range entries {
				day := e.Modified.Format("2006-01-02")
				if day != currentDay {
					lines = append(lines, "─── "+day+" ───")
					currentDay = day
				}
				oc := e.Outcome
				statusLabel := e.Status
				if oc != "unset" && oc != "" {
					statusLabel = fmt.Sprintf("%s | %s", e.Status, oc)
				}
				lines = append(lines, fmt.Sprintf("  %04d %s [%s] %s", e.ID, e.Modified.Format("15:04"), statusLabel, e.Title))
			}
			return printMaybeJSON(cmd, false, nil, strings.Join(lines, "\n"))
		},
	}
	cmd.Flags().IntVar(&days, "days", 7, "Days back")
	cmd.Flags().IntVar(&limit, "limit", 50, "Max entries")
	return cmd
}
