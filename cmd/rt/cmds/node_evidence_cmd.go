package cmds

import (
	"fmt"
	"strings"
	"time"

	"github.com/frudas24/research-tree/pkg/retree"
	"github.com/spf13/cobra"
)

// newNodePoisonCmd constructs the "node poison" subcommand for marking evidence as untrustworthy.
func newNodePoisonCmd(opts *RootOptions) *cobra.Command {
	var by, cause, scope, reason string
	cmd := &cobra.Command{
		Use:   "poison <id>",
		Short: "Mark node evidence as poisoned/contaminated",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			store, err := openStore(opts)
			if err != nil {
				return err
			}
			id, err := parseNodeID(args[0])
			if err != nil {
				return err
			}
			n, err := store.GetNode(id)
			if err != nil {
				return err
			}
			n.EvidenceStatus = retree.EvidencePoisoned
			if strings.TrimSpace(cause) != "" {
				n.EvidenceCause = parseEvidenceCause(cause)
			}
			if cmd.Flags().Changed("scope") {
				n.EvidenceScope = strings.TrimSpace(scope)
			}
			n.PoisonReason = strings.TrimSpace(reason)
			if strings.TrimSpace(by) != "" {
				poisonedBy, err := resolveParents(store, by)
				if err != nil {
					return err
				}
				n.PoisonedBy = uniqueNodeIDs(append(n.PoisonedBy, poisonedBy...))
			}
			n.Modified = time.Now().UTC()
			if err := store.UpdateNode(n); err != nil {
				return err
			}
			return printMaybeJSON(cmd, opts.OutputJSON, n, fmt.Sprintf("poisoned node %04d", n.ID))
		},
	}
	cmd.Flags().StringVar(&by, "by", "", "Node ID(s) or title substrings that explain/caused the contamination")
	cmd.Flags().StringVar(&cause, "cause", "", "Contamination cause: base_snapshot|toolchain|exporter|dataset|prompt_surface|runtime_env|unknown")
	cmd.Flags().StringVar(&scope, "scope", "", "Scope of contamination (e.g. host/model/surface)")
	cmd.Flags().StringVar(&reason, "reason", "", "Mandatory explanation of why the evidence is poisoned")
	_ = cmd.MarkFlagRequired("reason")
	return cmd
}

// newNodeRevalidateCmd constructs the "node revalidate" subcommand for clearing evidence suspicion.
func newNodeRevalidateCmd(opts *RootOptions) *cobra.Command {
	var by string
	cmd := &cobra.Command{
		Use:   "revalidate <id>",
		Short: "Mark node evidence as revalidated by clean rerun(s)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			store, err := openStore(opts)
			if err != nil {
				return err
			}
			id, err := parseNodeID(args[0])
			if err != nil {
				return err
			}
			n, err := store.GetNode(id)
			if err != nil {
				return err
			}
			revalidatedBy, err := resolveParents(store, by)
			if err != nil {
				return err
			}
			if len(revalidatedBy) == 0 {
				return fmt.Errorf("--by requires at least one node")
			}
			n.EvidenceStatus = retree.EvidenceRevalidated
			n.RevalidatedBy = uniqueNodeIDs(append(n.RevalidatedBy, revalidatedBy...))
			n.Modified = time.Now().UTC()
			if err := store.UpdateNode(n); err != nil {
				return err
			}
			return printMaybeJSON(cmd, opts.OutputJSON, n, fmt.Sprintf("revalidated node %04d", n.ID))
		},
	}
	cmd.Flags().StringVar(&by, "by", "", "Clean rerun node ID(s) or title substrings")
	_ = cmd.MarkFlagRequired("by")
	return cmd
}
