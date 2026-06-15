package cmds

import (
	"fmt"
	"sort"
	"strings"

	"github.com/frudas24/research-tree/pkg/retree"
	"github.com/spf13/cobra"
)

type linkEntry struct {
	From      NodeIDJSON `json:"from"`
	To        NodeIDJSON `json:"to"`
	Type      string     `json:"type"`
	Note      string     `json:"note,omitempty"`
	FromTitle string     `json:"from_title"`
	ToTitle   string     `json:"to_title"`
}

// NodeIDJSON marshals node IDs as plain JSON numbers for edge payloads.
type NodeIDJSON uint64

// MarshalJSON encodes the node ID as a base-10 JSON number.
func (n NodeIDJSON) MarshalJSON() ([]byte, error) {
	return []byte(fmt.Sprintf("%d", n)), nil
}

// newLinksCmd constructs the "links" subcommand for a flat DAG edge view.
func newLinksCmd(opts *RootOptions) *cobra.Command {
	var relType string
	cmd := &cobra.Command{
		Use:   "links",
		Short: "Show all graph edges (parents + relations)",
		Long: `Show a flat list of all edges in the research graph, including both
parent-child edges and typed relations (compares_against, inspired_by, etc.).

Use --type to filter by a specific relation type.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			store, err := openStore(opts)
			if err != nil {
				return err
			}
			cc := newColorizer(opts.ColorMode)

			// Collect all nodes for title lookups
			all, err := store.QueryNodes(retree.Filter{})
			if err != nil {
				return err
			}
			titleMap := make(map[retree.NodeID]string, len(all))
			for _, n := range all {
				titleMap[n.ID] = n.Title
			}

			var entries []linkEntry

			// Parent edges
			for _, n := range all {
				for _, p := range n.Parents {
					entries = append(entries, linkEntry{
						From:      NodeIDJSON(p),
						To:        NodeIDJSON(n.ID),
						Type:      "parent",
						FromTitle: titleMap[p],
						ToTitle:   titleMap[n.ID],
					})
				}
			}

			// Typed relations
			relAll, err := store.ListAllRelations()
			if err != nil {
				return err
			}
			for _, r := range relAll {
				entries = append(entries, linkEntry{
					From:      NodeIDJSON(r.From),
					To:        NodeIDJSON(r.Relation.Target),
					Type:      string(r.Relation.Type),
					Note:      r.Relation.Note,
					FromTitle: titleMap[r.From],
					ToTitle:   titleMap[r.Relation.Target],
				})
			}

			// Sort: by type, then from, then to
			sort.Slice(entries, func(i, j int) bool {
				if entries[i].Type != entries[j].Type {
					return entries[i].Type < entries[j].Type
				}
				if entries[i].From != entries[j].From {
					return entries[i].From < entries[j].From
				}
				return entries[i].To < entries[j].To
			})

			// Filter by type if requested
			if relType != "" {
				filtered := entries[:0]
				for _, e := range entries {
					if e.Type == relType {
						filtered = append(filtered, e)
					}
				}
				entries = filtered
			}

			if opts.OutputJSON {
				return printMaybeJSON(cmd, true, entries, "")
			}

			lines := make([]string, 0, len(entries))
			for _, e := range entries {
				typeLabel := e.Type
				note := ""
				if e.Note != "" {
					note = fmt.Sprintf(" (%s)", e.Note)
				}
				lines = append(lines, fmt.Sprintf("%04d %s %04d  %-20s %s%s",
					e.From, cc.golden("", "→"),
					e.To, typeLabel,
					cc.golden("", truncateTitle(e.FromTitle+note, 50)),
					" → "+truncateTitle(e.ToTitle, 40),
				))
			}
			if len(lines) == 0 {
				return printMaybeJSON(cmd, false, nil, "(no links)")
			}
			return printMaybeJSON(cmd, false, nil, strings.Join(lines, "\n"))
		},
	}
	cmd.Flags().StringVar(&relType, "type", "", "Filter by relation type (parent, depends_on, compares_against, inspired_by, aggregates)")
	return cmd
}

// truncateTitle bounds a title for narrow list displays.
func truncateTitle(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}
