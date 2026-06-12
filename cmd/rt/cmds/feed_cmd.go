package cmds

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/frudas24/research-tree/pkg/retree"
	"github.com/spf13/cobra"
)

type feedEntry struct {
	ID              retree.NodeID         `json:"id"`
	Title           string                `json:"title"`
	Status          retree.NodeStatus     `json:"status"`
	Outcome         retree.Outcome        `json:"outcome"`
	ClaimStatus     retree.ClaimStatus    `json:"claim_status"`
	MilestoneClass  retree.MilestoneClass `json:"milestone_class,omitempty"`
	MilestoneKind   retree.MilestoneKind  `json:"milestone_kind,omitempty"`
	MilestoneReason string                `json:"milestone_reason,omitempty"`
	Agent           string                `json:"agent"`
	Tags            []string              `json:"tags,omitempty"`
	Timestamp       time.Time             `json:"timestamp"`
	Basis           string                `json:"basis"`
}

// newFeedCmd constructs the "feed" subcommand for chronological global activity views.
func newFeedCmd(opts *RootOptions) *cobra.Command {
	var days, hours, limit int
	var by, agent, tag, status string
	cmd := &cobra.Command{
		Use:   "feed",
		Short: "Show chronological research activity feed",
		Long:  `Show a chronological feed of nodes ordered by created or modified time.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			store, err := openStore(opts)
			if err != nil {
				return err
			}
			cc := newColorizer(opts.ColorMode)
			filter := retree.Filter{
				Agent:  strings.TrimSpace(agent),
				Tag:    strings.TrimSpace(tag),
				Status: parseNodeStatus(status),
			}
			all, err := store.QueryNodes(filter)
			if err != nil {
				return err
			}
			entries := buildFeedEntries(all, by, days, hours, limit, time.Now().UTC())
			if opts.OutputJSON {
				return printMaybeJSON(cmd, true, entries, "")
			}
			lines := make([]string, 0, len(entries))
			for _, e := range entries {
				statusLabel := string(e.Status)
				if e.Outcome != retree.OutcomeUnset && e.Outcome != "" {
					statusLabel = fmt.Sprintf("%s | %s", e.Status, e.Outcome)
				}
				title := titleWithVerdict(&retree.Node{Frontmatter: retree.Frontmatter{
					Title:           e.Title,
					Status:          e.Status,
					Outcome:         e.Outcome,
					ClaimStatus:     e.ClaimStatus,
					MilestoneClass:  e.MilestoneClass,
					MilestoneKind:   e.MilestoneKind,
					MilestoneReason: e.MilestoneReason,
				}})
				title = cc.golden(e.MilestoneClass, title)
				lines = append(lines, fmt.Sprintf("%04d %s [%s] {%s} %s [%s]",
					e.ID,
					e.Timestamp.Format("2006-01-02 15:04"),
					statusLabel,
					e.Basis,
					title,
					e.Agent,
				))
			}
			return printMaybeJSON(cmd, false, nil, strings.Join(lines, "\n"))
		},
	}
	cmd.Flags().StringVar(&by, "by", "modified", "Chronology basis: created|modified")
	cmd.Flags().IntVar(&days, "days", 0, "Days back (0 = disabled)")
	cmd.Flags().IntVar(&hours, "hours", 0, "Hours back (0 = disabled)")
	cmd.Flags().IntVar(&limit, "limit", 50, "Max entries")
	cmd.Flags().StringVar(&agent, "agent", "", "Filter by agent")
	cmd.Flags().StringVar(&tag, "tag", "", "Filter by tag")
	cmd.Flags().StringVar(&status, "status", "", "Filter by status: active|done|paused")
	return cmd
}

// buildFeedEntries converts node summaries into chronologically sorted feed entries.
func buildFeedEntries(nodes []*retree.Node, basis string, days int, hours int, limit int, now time.Time) []feedEntry {
	useCreated := strings.EqualFold(strings.TrimSpace(basis), "created")
	cutoff := time.Time{}
	if hours > 0 {
		cutoff = now.Add(-time.Duration(hours) * time.Hour)
	} else if days > 0 {
		cutoff = now.Add(-time.Duration(days) * 24 * time.Hour)
	}
	entries := make([]feedEntry, 0, len(nodes))
	for _, n := range nodes {
		ts := n.Modified
		basisName := "modified"
		if useCreated {
			ts = n.Created
			basisName = "created"
		}
		if ts.IsZero() {
			continue
		}
		if !cutoff.IsZero() && ts.Before(cutoff) {
			continue
		}
		entries = append(entries, feedEntry{
			ID:              n.ID,
			Title:           n.Title,
			Status:          n.Status,
			Outcome:         n.Outcome,
			ClaimStatus:     n.ClaimStatus,
			MilestoneClass:  n.MilestoneClass,
			MilestoneKind:   n.MilestoneKind,
			MilestoneReason: n.MilestoneReason,
			Agent:           n.Agent,
			Tags:            append([]string(nil), n.Tags...),
			Timestamp:       ts,
			Basis:           basisName,
		})
	}
	sort.Slice(entries, func(i, j int) bool {
		if !entries[i].Timestamp.Equal(entries[j].Timestamp) {
			return entries[i].Timestamp.After(entries[j].Timestamp)
		}
		return entries[i].ID > entries[j].ID
	})
	if limit > 0 && len(entries) > limit {
		entries = entries[:limit]
	}
	return entries
}
