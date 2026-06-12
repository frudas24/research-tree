package cmds

import (
	"fmt"

	"github.com/frudas24/research-tree/pkg/retree"
	"github.com/spf13/cobra"
)

// newArtifactCmd constructs the "artifact" subcommand.
func newArtifactCmd(opts *RootOptions) *cobra.Command {
	cmd := &cobra.Command{Use: "artifact", Short: "Artifact operations"}
	cmd.AddCommand(newArtifactAddCmd(opts))
	cmd.AddCommand(newArtifactRemoveCmd(opts))
	cmd.AddCommand(newArtifactEmbedCmd(opts))
	return cmd
}

// newArtifactAddCmd constructs the "artifact add" subcommand.
func newArtifactAddCmd(opts *RootOptions) *cobra.Command {
	var mode, host, path, desc string
	cmd := &cobra.Command{
		Use:   "add <id>",
		Short: "Add artifact reference",
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
			a := retree.Artifact{Mode: retree.ArtifactMode(mode), Host: host, Path: path, Description: desc}
			if err := store.AddArtifact(id, a); err != nil {
				return err
			}
			return printMaybeJSON(cmd, opts.OutputJSON, a, fmt.Sprintf("artifact added to %04d", id))
		},
	}
	cmd.Flags().StringVar(&mode, "mode", "path", "Artifact mode: path|embedded")
	cmd.Flags().StringVar(&host, "host", "", "Artifact host (required for mode=path)")
	cmd.Flags().StringVar(&path, "path", "", "Artifact path")
	cmd.Flags().StringVar(&desc, "desc", "", "Artifact description")
	_ = cmd.MarkFlagRequired("path")
	return cmd
}

// newArtifactEmbedCmd constructs the "artifact embed" subcommand.
func newArtifactEmbedCmd(opts *RootOptions) *cobra.Command {
	var filePath, desc string
	cmd := &cobra.Command{
		Use:   "embed <id>",
		Short: "Embed artifact file",
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
			if err := store.EmbedArtifact(id, filePath, desc); err != nil {
				return err
			}
			return printMaybeJSON(cmd, opts.OutputJSON, map[string]any{"id": id, "file": filePath}, fmt.Sprintf("artifact embedded into %04d", id))
		},
	}
	cmd.Flags().StringVar(&filePath, "file", "", "File to embed")
	cmd.Flags().StringVar(&desc, "desc", "", "Artifact description")
	_ = cmd.MarkFlagRequired("file")
	return cmd
}

// newArtifactRemoveCmd constructs the "artifact remove" subcommand.
func newArtifactRemoveCmd(opts *RootOptions) *cobra.Command {
	var mode, host, path, desc string
	cmd := &cobra.Command{
		Use:   "remove <id>",
		Short: "Remove artifact reference",
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
			a := retree.Artifact{Mode: retree.ArtifactMode(mode), Host: host, Path: path, Description: desc}
			if err := store.RemoveArtifact(id, a); err != nil {
				return err
			}
			return printMaybeJSON(cmd, opts.OutputJSON, a, fmt.Sprintf("artifact removed from %04d", id))
		},
	}
	cmd.Flags().StringVar(&mode, "mode", "", "Artifact mode to match: path|embedded")
	cmd.Flags().StringVar(&host, "host", "", "Artifact host to match")
	cmd.Flags().StringVar(&path, "path", "", "Artifact path to match")
	cmd.Flags().StringVar(&desc, "desc", "", "Artifact description to match")
	_ = cmd.MarkFlagRequired("path")
	return cmd
}
