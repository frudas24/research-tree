package cmds

import (
	"fmt"
	"strings"

	"github.com/frudas24/research-tree/pkg/retree"
	"github.com/spf13/cobra"
)

// newFeatureCmd constructs the "feature" subcommand group.
func newFeatureCmd(opts *RootOptions) *cobra.Command {
	cmd := &cobra.Command{Use: "feature", Short: "Feature lineage operations"}
	cmd.AddCommand(newFeatureCreateCmd(opts))
	cmd.AddCommand(newFeatureListCmd(opts))
	cmd.AddCommand(newFeatureShowCmd(opts))
	cmd.AddCommand(newFeatureLinkCmd(opts))
	return cmd
}

// newFeatureCreateCmd constructs the "feature create" subcommand.
func newFeatureCreateCmd(opts *RootOptions) *cobra.Command {
	var fromNode string
	cmd := &cobra.Command{
		Use:   "create <name>",
		Short: "Create a feature",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			store, err := openStore(opts)
			if err != nil {
				return err
			}
			fromID, err := parseNodeID(fromNode)
			if err != nil {
				return fmt.Errorf("--from-node: %w", err)
			}
			f, err := store.CreateFeature(strings.TrimSpace(args[0]), fromID)
			if err != nil {
				return err
			}
			return printMaybeJSON(cmd, opts.OutputJSON, f, fmt.Sprintf("created feature %s: %s", f.ID, f.Name))
		},
	}
	cmd.Flags().StringVar(&fromNode, "from-node", "", "RT node that proposed this feature (required)")
	_ = cmd.MarkFlagRequired("from-node")
	return cmd
}

// newFeatureListCmd constructs the "feature list" subcommand.
func newFeatureListCmd(opts *RootOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List all features",
		RunE: func(cmd *cobra.Command, args []string) error {
			store, err := openStore(opts)
			if err != nil {
				return err
			}
			features, err := store.ListFeatures()
			if err != nil {
				return err
			}
			if opts.OutputJSON {
				return printMaybeJSON(cmd, true, features, "")
			}
			if len(features) == 0 {
				return printMaybeJSON(cmd, false, nil, "(no features)")
			}
			cc := newColorizer(opts.ColorMode)
			var b strings.Builder
			for _, f := range features {
				label := string(f.Status)
				if label == "active" {
					label = cc.status(retree.StatusActive, label)
				}
				fmt.Fprintf(&b, "%s %s [%s]", f.ID, f.Name, label)
				if f.CurrentNode != 0 {
					fmt.Fprintf(&b, "  current: %04d", f.CurrentNode)
				}
				fmt.Fprintf(&b, "  nodes: %d\n", len(f.Nodes))
			}
			return printMaybeJSON(cmd, false, nil, b.String())
		},
	}
	return cmd
}

// newFeatureShowCmd constructs the "feature show" subcommand.
func newFeatureShowCmd(opts *RootOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "show <id|name>",
		Short: "Show feature details",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			store, err := openStore(opts)
			if err != nil {
				return err
			}
			f, err := store.GetFeature(strings.TrimSpace(args[0]))
			if err != nil {
				return err
			}
			if opts.OutputJSON {
				return printMaybeJSON(cmd, true, f, "")
			}
			cc := newColorizer(opts.ColorMode)
			var b strings.Builder
			statusLabel := string(f.Status)
			if f.Status == retree.FeatureActive {
				statusLabel = cc.status(retree.StatusActive, statusLabel)
			}
			fmt.Fprintf(&b, "%s %s [%s]\n", f.ID, f.Name, statusLabel)
			fmt.Fprintf(&b, "  slug: %s\n", f.Slug)
			fmt.Fprintf(&b, "  created from: %04d\n", f.CreatedFrom)
			if f.CurrentNode != 0 {
				mode := f.CurrentNodeMode
				if mode == "" {
					mode = "derived"
				}
				fmt.Fprintf(&b, "  current node: %04d (%s)\n", f.CurrentNode, mode)
			}
			if len(f.Nodes) > 0 {
				fmt.Fprintf(&b, "  nodes:\n")
				for _, n := range f.Nodes {
					fmt.Fprintf(&b, "    %04d  %s\n", n.NodeID, n.Role)
				}
			}
			return printMaybeJSON(cmd, false, nil, b.String())
		},
	}
	return cmd
}

// newFeatureLinkCmd constructs the "feature link" subcommand.
func newFeatureLinkCmd(opts *RootOptions) *cobra.Command {
	var role string
	cmd := &cobra.Command{
		Use:   "link <feature-id> <node-id>",
		Short: "Link a node to a feature",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			store, err := openStore(opts)
			if err != nil {
				return err
			}
			nodeID, err := parseNodeID(args[1])
			if err != nil {
				return fmt.Errorf("node-id: %w", err)
			}
			featRole := retree.FeatureNodeRole(strings.TrimSpace(role))
			if featRole == "" {
				featRole = retree.RoleImplementation
			}
			if err := store.LinkNodeToFeature(args[0], nodeID, featRole); err != nil {
				return err
			}
			return printMaybeJSON(cmd, opts.OutputJSON,
				map[string]any{"feature": args[0], "node": nodeID, "role": featRole},
				fmt.Sprintf("linked node %04d to feature %s as %s", nodeID, args[0], featRole))
		},
	}
	cmd.Flags().StringVar(&role, "role", "implementation", "Node role within feature")
	return cmd
}
