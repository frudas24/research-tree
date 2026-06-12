package cmds

import (
	"fmt"
	"sort"
	"strings"

	"github.com/frudas24/research-tree/pkg/retree"
	"github.com/spf13/cobra"
)

// newMermaidCmd constructs the "mermaid" subcommand.
func newMermaidCmd(opts *RootOptions) *cobra.Command {
	var direction string
	cmd := &cobra.Command{
		Use:   "mermaid [id]",
		Short: "Export tree as Mermaid flowchart",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			store, err := openStore(opts)
			if err != nil {
				return err
			}
			nodes, err := store.QueryNodes(retree.Filter{})
			if err != nil {
				return err
			}
			nodesByID := make(map[retree.NodeID]*retree.Node, len(nodes))
			for _, n := range nodes {
				nodesByID[n.ID] = n
			}
			children := map[retree.NodeID][]retree.NodeID{}
			for _, n := range nodes {
				for _, p := range n.Parents {
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
				if _, ok := nodesByID[id]; !ok {
					return fmt.Errorf("node %04d not found", id)
				}
				roots = []retree.NodeID{id}
			} else {
				for _, n := range nodes {
					isRoot := true
					for _, p := range n.Parents {
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

			dir := "TD"
			switch direction {
			case "LR", "RL", "BT", "TD":
				dir = direction
			}

			var b strings.Builder
			fmt.Fprintf(&b, "flowchart %s\n", dir)

			visited := map[retree.NodeID]bool{}
			var walk func(id retree.NodeID)
			walk = func(id retree.NodeID) {
				if visited[id] {
					return
				}
				visited[id] = true
				for _, cid := range children[id] {
					walk(cid)
				}
			}
			for _, r := range roots {
				walk(r)
			}

			// Node definitions
			for _, n := range nodes {
				if !visited[n.ID] {
					continue
				}
				label := escapeMermaid(n.Title)
				status := string(n.Status)
				outcome := string(n.Outcome)
				short := fmt.Sprintf("%s | %s", status, outcome)
				if n.Outcome == retree.OutcomeUnset {
					short = status
				}
				fmt.Fprintf(&b, "  N%d[\"%s\\n(%s)\"]\n", n.ID, label, short)
			}

			// Edges
			emitted := map[retree.NodeID]bool{}
			walk = func(id retree.NodeID) {
				if emitted[id] {
					return
				}
				emitted[id] = true
				for _, cid := range children[id] {
					fmt.Fprintf(&b, "  N%d --> N%d\n", id, cid)
					walk(cid)
				}
			}
			for _, r := range roots {
				walk(r)
			}

			return printMaybeJSON(cmd, false, nil, b.String())
		},
	}
	cmd.Flags().StringVar(&direction, "dir", "TD", "Direction: TD|LR|RL|BT")
	return cmd
}

// escapeMermaid escapes special chars for Mermaid syntax.
func escapeMermaid(s string) string {
	s = strings.ReplaceAll(s, "\"", "#quot;")
	s = strings.ReplaceAll(s, "(", "&#40;")
	s = strings.ReplaceAll(s, ")", "&#41;")
	return s
}
