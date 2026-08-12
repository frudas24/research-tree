package cmds

import (
	"fmt"

	"github.com/frudas24/research-tree/pkg/retree"
	"github.com/spf13/cobra"
)

// newStorageCmd constructs the "storage" subcommand.
func newStorageCmd(opts *RootOptions) *cobra.Command {
	cmd := &cobra.Command{Use: "storage", Short: "Storage operations"}
	cmd.AddCommand(newStorageMigrateCmd(opts))
	cmd.AddCommand(newStorageReindexCmd(opts))
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
