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
