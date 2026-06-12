package cmds

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/frudas24/research-tree/pkg/retree"
	"github.com/spf13/cobra"
)

// newInitCmd constructs the "init" subcommand.
func newInitCmd(opts *RootOptions) *cobra.Command {
	var storageFormat string
	var force bool
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize a research root",
		Long: `Initialize a new research root at --research-root.

Use --force to reinitialize an existing root. This creates a safety
snapshot to the system temp directory before wiping.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			format := retree.StorageFormat(storageFormat)

			if force {
				if err := forceInit(opts.ResearchRoot, format); err != nil {
					return err
				}
				return printMaybeJSON(cmd, opts.OutputJSON, map[string]any{
					"root":   opts.ResearchRoot,
					"format": format,
				}, fmt.Sprintf("reinitialized %s (%s)", opts.ResearchRoot, format))
			}

			s, err := retree.Init(opts.ResearchRoot, format)
			if err != nil {
				return err
			}
			return printMaybeJSON(cmd, opts.OutputJSON, map[string]any{
				"root":   opts.ResearchRoot,
				"format": s.StorageFormat(),
			}, fmt.Sprintf("initialized %s (%s)", opts.ResearchRoot, s.StorageFormat()))
		},
	}
	cmd.Flags().StringVar(&storageFormat, "storage-format", defaultStorageFormat(), "Storage format: json|bin")
	cmd.Flags().BoolVar(&force, "force", false, "Reinitialize existing root (creates safety snapshot)")
	return cmd
}

// defaultStorageFormat resolves the default storage format from environment.
// Priority:
// 1) RESEARCH_TREE_FORMAT if set to json/bin
// 2) bin (production default)
func defaultStorageFormat() string {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("RESEARCH_TREE_FORMAT"))) {
	case string(retree.StorageJSON):
		return string(retree.StorageJSON)
	case string(retree.StorageBIN):
		return string(retree.StorageBIN)
	default:
		return string(retree.StorageBIN)
	}
}

// forceInit saves a safety snapshot outside the research root, then
// wipes and reinitializes.
func forceInit(rootPath string, format retree.StorageFormat) error {
	metaPath := filepath.Join(rootPath, "meta.json")
	if _, err := os.Stat(metaPath); err == nil {
		// Existing root detected — try to snapshot it first
		s, err := retree.Open(rootPath)
		if err == nil {
			snaps, _ := s.ListSnapshots()
			if len(snaps) > 0 {
				fmt.Fprintf(os.Stderr, "rt: safety snapshot saved to history/ (latest: %s)\n", snaps[0].ID)
			}
		}
		// Create external emergency backup
		backupDir, err := os.MkdirTemp("", "rt-init-backup-*")
		if err == nil {
			if err := copyDirContents(rootPath, backupDir); err == nil {
				fmt.Fprintf(os.Stderr, "rt: emergency backup saved to %s\n", backupDir)
			}
		}
		// Wipe
		if err := os.RemoveAll(rootPath); err != nil {
			return fmt.Errorf("force init: cannot remove existing root: %w", err)
		}
	}
	_, err := retree.Init(rootPath, format)
	return err
}

// copyDirContents copies all entries from src to dst (shallow copy of children).
func copyDirContents(src, dst string) error {
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	for _, e := range entries {
		srcPath := filepath.Join(src, e.Name())
		dstPath := filepath.Join(dst, e.Name())
		if e.IsDir() {
			if err := copyDirTree(srcPath, dstPath); err != nil {
				return err
			}
		} else {
			data, err := os.ReadFile(srcPath)
			if err != nil {
				return err
			}
			if err := os.MkdirAll(dst, 0o755); err != nil {
				return err
			}
			if err := os.WriteFile(dstPath, data, 0o644); err != nil {
				return err
			}
		}
	}
	return nil
}

// copyDirTree recursively copies a directory tree.
func copyDirTree(src, dst string) error {
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return err
	}
	for _, e := range entries {
		srcPath := filepath.Join(src, e.Name())
		dstPath := filepath.Join(dst, e.Name())
		if e.IsDir() {
			if err := copyDirTree(srcPath, dstPath); err != nil {
				return err
			}
		} else {
			data, err := os.ReadFile(srcPath)
			if err != nil {
				return err
			}
			if err := os.WriteFile(dstPath, data, 0o644); err != nil {
				return err
			}
		}
	}
	return nil
}
