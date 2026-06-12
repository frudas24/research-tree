package cmds

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

// newAlertCmd constructs the "alert" subcommand.
func newAlertCmd(opts *RootOptions) *cobra.Command {
	cmd := &cobra.Command{Use: "alert", Short: "Branch warning alerts"}
	cmd.AddCommand(newAlertListCmd(opts))
	cmd.AddCommand(newAlertAckCmd(opts))
	return cmd
}

// newAlertListCmd constructs the "alert list" subcommand.
func newAlertListCmd(opts *RootOptions) *cobra.Command {
	var agent string
	var onlyUnacked bool
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List warnings",
		RunE: func(cmd *cobra.Command, args []string) error {
			store, err := openStore(opts)
			if err != nil {
				return err
			}
			warnings, err := store.ListBranchWarnings(agent, onlyUnacked)
			if err != nil {
				return err
			}
			if opts.OutputJSON {
				return printMaybeJSON(cmd, true, warnings, "")
			}
			lines := make([]string, 0, len(warnings))
			for _, w := range warnings {
				state := "pending"
				if w.AckedAt != nil {
					state = "ack"
				}
				lines = append(lines, fmt.Sprintf("%s | %s | %s", w.ID, state, w.Message))
			}
			return printMaybeJSON(cmd, false, nil, strings.Join(lines, "\n"))
		},
	}
	cmd.Flags().StringVar(&agent, "agent", "", "Filter by agent")
	cmd.Flags().BoolVar(&onlyUnacked, "only-unacked", false, "Only show unacknowledged warnings")
	return cmd
}

// newAlertAckCmd constructs the "alert ack" subcommand.
func newAlertAckCmd(opts *RootOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "ack <warning-id>",
		Short: "Acknowledge warning",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			store, err := openStore(opts)
			if err != nil {
				return err
			}
			if err := store.AckBranchWarning(args[0]); err != nil {
				return err
			}
			return printMaybeJSON(cmd, opts.OutputJSON, map[string]any{"ack": args[0]}, fmt.Sprintf("acknowledged %s", args[0]))
		},
	}
	return cmd
}
