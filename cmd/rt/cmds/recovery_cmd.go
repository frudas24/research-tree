package cmds

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

// newRecoveryCmd constructs the "recovery" subcommand.
func newRecoveryCmd(opts *RootOptions) *cobra.Command {
	cmd := &cobra.Command{Use: "recovery", Short: "Snapshot recovery"}
	cmd.AddCommand(newRecoveryListCmd(opts))
	cmd.AddCommand(newRecoveryRestoreCmd(opts))
	return cmd
}

// newRecoveryListCmd constructs the "recovery list" subcommand.
func newRecoveryListCmd(opts *RootOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List snapshots",
		RunE: func(cmd *cobra.Command, args []string) error {
			store, err := openStore(opts)
			if err != nil {
				return err
			}
			snaps, err := store.ListSnapshots()
			if err != nil {
				return err
			}
			if opts.OutputJSON {
				return printMaybeJSON(cmd, true, snaps, "")
			}
			lines := make([]string, 0, len(snaps))
			for _, s := range snaps {
				lines = append(lines, fmt.Sprintf("%s | %s | %s", s.ID, s.CreatedAt, s.Operation))
			}
			return printMaybeJSON(cmd, false, nil, strings.Join(lines, "\n"))
		},
	}
	return cmd
}

// newRecoveryRestoreCmd constructs the "recovery restore" subcommand.
func newRecoveryRestoreCmd(opts *RootOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "restore <snapshot-id>",
		Short: "Restore a snapshot",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			store, err := openStore(opts)
			if err != nil {
				return err
			}
			if err := store.RestoreSnapshot(args[0]); err != nil {
				return err
			}
			return printMaybeJSON(cmd, opts.OutputJSON, map[string]any{"restored": args[0]}, "snapshot restored")
		},
	}
	return cmd
}
