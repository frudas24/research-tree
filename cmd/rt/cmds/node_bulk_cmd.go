package cmds

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/frudas24/research-tree/pkg/retree"
	"github.com/spf13/cobra"
)

// newNodeImportCmd constructs the "node import" subcommand for batch
// importing nodes from a JSON file.
func newNodeImportCmd(opts *RootOptions) *cobra.Command {
	var filePath string
	cmd := &cobra.Command{
		Use:   "import",
		Short: "Import nodes from JSON file",
		Long:  `Import nodes from a JSON file. Format: {"nodes":[{"title":"...", "parents":[1], ...}]}`,
		RunE: func(cmd *cobra.Command, args []string) error {
			store, err := openStore(opts)
			if err != nil {
				return err
			}
			data, err := readBody(filePath)
			if err != nil {
				return err
			}
			var payload struct {
				Nodes []retree.Node `json:"nodes"`
			}
			if err := json.Unmarshal([]byte(data), &payload); err != nil {
				return err
			}
			var created []retree.NodeID
			for i := range payload.Nodes {
				payload.Nodes[i].ID = 0 // let store assign
				if err := store.CreateNode(&payload.Nodes[i]); err != nil {
					return fmt.Errorf("import node %d (title=%q): %w", i, payload.Nodes[i].Title, err)
				}
				created = append(created, payload.Nodes[i].ID)
			}
			return printMaybeJSON(cmd, opts.OutputJSON, map[string]any{"imported": len(created), "ids": created}, fmt.Sprintf("imported %d nodes", len(created)))
		},
	}
	cmd.Flags().StringVar(&filePath, "file", "", "JSON file to import")
	_ = cmd.MarkFlagRequired("file")
	return cmd
}

// newNodeBatchCmd constructs a callback for batch update via --filter.
func newNodeBatchCmd(opts *RootOptions) *cobra.Command {
	var filterStatus, filterAgent, filterTag, setStatus, setClaimStatus, setOutcome, setTitle, setBody, setAgent string
	cmd := &cobra.Command{
		Use:   "batch",
		Short: "Batch update nodes (use with --filter-*)",
		RunE: func(cmd *cobra.Command, args []string) error {
			store, err := openStore(opts)
			if err != nil {
				return err
			}
			f := retree.Filter{
				Status: parseNodeStatus(filterStatus),
				Agent:  filterAgent,
				Tag:    filterTag,
			}
			nodes, err := store.QueryNodes(f)
			if err != nil {
				return err
			}
			updated := 0
			for _, n := range nodes {
				if setTitle != "" {
					n.Title = setTitle
				}
				if setStatus != "" {
					n.Status = parseNodeStatus(setStatus)
				}
				if setClaimStatus != "" {
					n.ClaimStatus = parseClaimStatus(setClaimStatus)
				}
				if setOutcome != "" {
					n.Outcome = parseOutcome(setOutcome)
				}
				if setBody != "" {
					n.Body = setBody
				}
				if setAgent != "" {
					n.Agent = setAgent
				}
				if err := validateTerminalOutcome(n.Status, n.Outcome); err != nil {
					return fmt.Errorf("update node %d: %w", n.ID, err)
				}
				n.Modified = time.Now().UTC()
				if err := store.UpdateNode(n); err != nil {
					return fmt.Errorf("update node %d: %w", n.ID, err)
				}
				updated++
			}
			return printMaybeJSON(cmd, opts.OutputJSON, map[string]any{"updated": updated, "matched": len(nodes)}, fmt.Sprintf("updated %d of %d nodes", updated, len(nodes)))
		},
	}
	cmd.Flags().StringVar(&filterStatus, "filter-status", "", "Filter: status")
	cmd.Flags().StringVar(&filterAgent, "filter-agent", "", "Filter: agent")
	cmd.Flags().StringVar(&filterTag, "filter-tag", "", "Filter: tag")
	cmd.Flags().StringVar(&setStatus, "set-status", "", "Set status")
	cmd.Flags().StringVar(&setClaimStatus, "set-claim-status", "", "Set claim status")
	cmd.Flags().StringVar(&setOutcome, "set-outcome", "", "Set outcome: unset|success|failure|inconclusive")
	cmd.Flags().StringVar(&setTitle, "set-title", "", "Set title")
	cmd.Flags().StringVar(&setBody, "set-body", "", "Set body")
	cmd.Flags().StringVar(&setAgent, "set-agent", "", "Set agent")
	return cmd
}
