package cmds

import (
	"fmt"
	"slices"
	"strings"

	"github.com/frudas24/research-tree/pkg/retree"
	"github.com/spf13/cobra"
)

// newStorageCmd constructs the "storage" subcommand.
func newStorageCmd(opts *RootOptions) *cobra.Command {
	cmd := &cobra.Command{Use: "storage", Short: "Storage operations"}
	cmd.AddCommand(newStorageMigrateCmd(opts))
	cmd.AddCommand(newStorageReindexCmd(opts))
	cmd.AddCommand(newStorageRepairOutcomesCmd(opts))
	return cmd
}

// newStorageReindexCmd reconstructs nodes.idx from nodes.bin after the
// index was lost or corrupted.
func newStorageReindexCmd(opts *RootOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "reindex",
		Short: "Rebuild the binary node index from nodes.bin",
		Long: `Rebuild nodes.idx by scanning nodes.bin sequentially.

Use this to recover a binary-mode store whose index was lost or corrupted.
The store refuses to load (instead of silently appearing empty) while the
index is missing, so reindex is the recovery path.

Reindex only supports the current v2 binary payload layout. Legacy v1
stores remain readable with an intact index, but rebuilding their index is
intentionally rejected because the old concatenated payloads are not
delimited safely enough for forensic recovery.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			store, err := openStore(opts)
			if err != nil {
				return err
			}
			if store.StorageFormat() != retree.StorageBIN {
				return fmt.Errorf("reindex only applies to bin storage format (current: %s)", store.StorageFormat())
			}
			if err := store.RegenerateBinIndex(); err != nil {
				return err
			}
			return printMaybeJSON(cmd, opts.OutputJSON, map[string]any{"index": "rebuilt"}, "binary index rebuilt")
		},
	}
	return cmd
}

// newStorageMigrateCmd constructs the "storage migrate" subcommand.
func newStorageMigrateCmd(opts *RootOptions) *cobra.Command {
	var target string
	cmd := &cobra.Command{
		Use:   "migrate",
		Short: "Migrate storage format",
		RunE: func(cmd *cobra.Command, args []string) error {
			store, err := openStore(opts)
			if err != nil {
				return err
			}
			if err := store.MigrateStorageFormat(retree.StorageFormat(target)); err != nil {
				return err
			}
			return printMaybeJSON(cmd, opts.OutputJSON, map[string]any{"format": store.StorageFormat()}, fmt.Sprintf("storage migrated to %s", store.StorageFormat()))
		},
	}
	cmd.Flags().StringVar(&target, "to", string(retree.StorageBIN), "Target format: json|bin")
	return cmd
}

// newStorageRepairOutcomesCmd scans or repairs legacy done+unset nodes so old
// stores can be migrated without weakening normal validation.
func newStorageRepairOutcomesCmd(opts *RootOptions) *cobra.Command {
	var sets []string
	cmd := &cobra.Command{
		Use:   "repair-outcomes",
		Short: "Scan or repair legacy done nodes without terminal outcomes",
		Long: `Scan the store for legacy nodes persisted as status=done with outcome=unset.

Without --set, the command only reports the affected nodes.
With one or more --set ID=success|failure|inconclusive entries, it creates a
pre-repair snapshot, applies the explicit outcomes, and writes the repaired
store back under the normal strict validator.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			store, err := openStore(opts)
			if err != nil {
				return err
			}
			if len(sets) == 0 {
				report, err := store.ScanLegacyDoneUnsetOutcomes()
				if err != nil {
					return err
				}
				msg := "no legacy done+unset nodes found"
				if len(report.Issues) > 0 {
					msg = fmt.Sprintf("found %d legacy done+unset node(s)", len(report.Issues))
				}
				return printMaybeJSON(cmd, opts.OutputJSON, report, msg)
			}
			fixes, err := parseOutcomeRepairs(sets)
			if err != nil {
				return err
			}
			report, err := store.RepairLegacyDoneUnsetOutcomes(fixes)
			if err != nil {
				return err
			}
			return printMaybeJSON(cmd, opts.OutputJSON, report, fmt.Sprintf("repaired %d legacy node(s)", len(report.Repaired)))
		},
	}
	cmd.Flags().StringArrayVar(&sets, "set", nil, "Explicit repair mapping ID=success|failure|inconclusive (repeatable)")
	return cmd
}

// parseOutcomeRepairs parses repeated --set ID=outcome entries for the legacy
// outcome repair command, rejecting malformed or non-terminal assignments.
func parseOutcomeRepairs(entries []string) (map[retree.NodeID]retree.Outcome, error) {
	fixes := make(map[retree.NodeID]retree.Outcome, len(entries))
	for _, entry := range entries {
		left, right, ok := strings.Cut(strings.TrimSpace(entry), "=")
		if !ok || strings.TrimSpace(left) == "" || strings.TrimSpace(right) == "" {
			return nil, fmt.Errorf("invalid --set %q (want ID=success|failure|inconclusive)", entry)
		}
		id, err := parseNodeID(left)
		if err != nil {
			return nil, fmt.Errorf("invalid --set %q: %w", entry, err)
		}
		outcome := parseOutcome(right)
		if !slices.Contains([]retree.Outcome{retree.OutcomeSuccess, retree.OutcomeFailure, retree.OutcomeInconclusive}, outcome) {
			return nil, fmt.Errorf("invalid --set %q: outcome must be success, failure, or inconclusive", entry)
		}
		fixes[id] = outcome
	}
	return fixes, nil
}
