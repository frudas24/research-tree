package cmds

import (
	"fmt"

	"github.com/spf13/cobra"
)

// newTagCmd constructs the "tag" subcommand.
func newTagCmd(opts *RootOptions) *cobra.Command {
	cmd := &cobra.Command{Use: "tag", Short: "Tag operations"}
	cmd.AddCommand(newTagAddCmd(opts))
	cmd.AddCommand(newTagRmCmd(opts))
	return cmd
}

// newTagAddCmd constructs the "tag add" subcommand.
func newTagAddCmd(opts *RootOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "add <id> <tags>",
		Short: "Add comma-separated tags",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			store, err := openStore(opts)
			if err != nil {
				return err
			}
			id, err := parseNodeID(args[0])
			if err != nil {
				return err
			}
			if err := store.AddTags(id, parseCSVStrings(args[1])...); err != nil {
				return err
			}
			return printMaybeJSON(cmd, opts.OutputJSON, map[string]any{"id": id, "tags": parseCSVStrings(args[1])}, fmt.Sprintf("tags added to %04d", id))
		},
	}
	return cmd
}

// newTagRmCmd constructs the "tag rm" subcommand.
func newTagRmCmd(opts *RootOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "rm <id> <tags>",
		Short: "Remove comma-separated tags",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			store, err := openStore(opts)
			if err != nil {
				return err
			}
			id, err := parseNodeID(args[0])
			if err != nil {
				return err
			}
			if err := store.RemoveTags(id, parseCSVStrings(args[1])...); err != nil {
				return err
			}
			return printMaybeJSON(cmd, opts.OutputJSON, map[string]any{"id": id, "tags": parseCSVStrings(args[1])}, fmt.Sprintf("tags removed from %04d", id))
		},
	}
	return cmd
}
