package cmds

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/frudas24/research-tree/pkg/retree"
	"github.com/spf13/cobra"
)

// newDestroyCmd constructs the "destroy" subcommand for explicitly wiping
// a research root with confirmation and safety snapshot.
func newDestroyCmd(opts *RootOptions) *cobra.Command {
	var yes bool
	cmd := &cobra.Command{
		Use:   "destroy",
		Short: "Destroy the research root permanently",
		Long: `Permanently destroy the research root at --research-root.

This command requires --yes for non-interactive use.
It creates a safety snapshot to /tmp before destroying as a last resort.
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			root := opts.ResearchRoot
			metaPath := filepath.Join(root, "meta.json")
			if _, err := os.Stat(metaPath); os.IsNotExist(err) {
				return fmt.Errorf("no research root at %s", root)
			}

			// Confirm
			if !yes {
				fmt.Fprintf(cmd.ErrOrStderr(), "Destroy %s? [y/N]: ", root)
				var response string
				fmt.Scanln(&response)
				if response != "y" && response != "Y" {
					return fmt.Errorf("aborted")
				}
			}

			// Safety snapshot to external location
			s, err := retree.Open(root)
			if err == nil {
				snaps, _ := s.ListSnapshots()
				if len(snaps) > 0 {
					fmt.Fprintf(cmd.ErrOrStderr(), "rt: %d snapshots in history/ will be destroyed\n", len(snaps))
				}
			}

			backupDir, err := os.MkdirTemp("", "rt-destroy-backup-*")
			if err == nil {
				if err := copyDirContents(root, backupDir); err == nil {
					fmt.Fprintf(cmd.ErrOrStderr(), "rt: safety backup saved to %s\n", backupDir)
				}
			}

			if err := os.RemoveAll(root); err != nil {
				return fmt.Errorf("destroy failed: %w", err)
			}

			fmt.Fprintf(cmd.OutOrStdout(), "destroyed %s\n", root)
			return nil
		},
	}
	cmd.Flags().BoolVar(&yes, "yes", false, "Skip confirmation prompt")
	return cmd
}
