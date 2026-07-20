// Package cmds implements the CLI commands for the research-tree tool.
package cmds

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/frudas24/research-tree/pkg/retree"
	"github.com/spf13/cobra"
)

// Version set at build time via -ldflags.
var Version = "dev"

// RootOptions holds global CLI flags.
type RootOptions struct {
	ResearchRoot string
	OutputJSON   bool
	ColorMode    string
}

// NewRootCmd constructs the root command tree.
func NewRootCmd() *cobra.Command {
	opts := &RootOptions{}
	cmd := &cobra.Command{
		Use:     "rt",
		Short:   "Research tree CLI",
		Version: Version,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	cmd.PersistentFlags().StringVar(&opts.ResearchRoot, "research-root", defaultResearchRoot(), "Path to research root")
	cmd.PersistentFlags().BoolVar(&opts.OutputJSON, "json", false, "Emit JSON output")
	cmd.PersistentFlags().StringVar(&opts.ColorMode, "color", "auto", "Color mode: always|never|auto")

	cmd.AddCommand(newInitCmd(opts))
	cmd.AddCommand(newNodeCmd(opts))
	cmd.AddCommand(newArtifactCmd(opts))
	cmd.AddCommand(newTagCmd(opts))
	cmd.AddCommand(newRecoveryCmd(opts))
	cmd.AddCommand(newStorageCmd(opts))
	cmd.AddCommand(newAlertCmd(opts))
	cmd.AddCommand(newTreeCmd(opts))
	cmd.AddCommand(newStatusCmd(opts))
	cmd.AddCommand(newDestroyCmd(opts))
	cmd.AddCommand(newMermaidCmd(opts))
	cmd.AddCommand(newChangesCmd(opts))
	cmd.AddCommand(newTimelineCmd(opts))
	cmd.AddCommand(newFeedCmd(opts))
	cmd.AddCommand(newResourceCmd(opts))
	cmd.AddCommand(newGoldenCmd(opts))
	cmd.AddCommand(newLinksCmd(opts))
	cmd.AddCommand(newLintCmd(opts))
	cmd.AddCommand(newFeatureCmd(opts))
	cmd.AddCommand(newDoctorCmd(opts))
	return cmd
}

// printMaybeJSON writes either JSON or human-readable output to the command's stdout.
func printMaybeJSON(cmd *cobra.Command, asJSON bool, v any, human string) error {
	if asJSON {
		b, err := json.MarshalIndent(v, "", "  ")
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(cmd.OutOrStdout(), string(b))
		return err
	}
	_, err := fmt.Fprintln(cmd.OutOrStdout(), strings.TrimSpace(human))
	return err
}

// openStore opens the research root from CLI options.
func openStore(opts *RootOptions) (*retree.Store, error) {
	return retree.Open(opts.ResearchRoot)
}

// defaultResearchRoot resolves the default research-root path from environment.
// Priority:
// 1) RESEARCH_ROOT (full explicit path)
// 2) RESEARCH_TREE_ROOT/.research (workspace-like base directory)
// 3) .research in current working directory
func defaultResearchRoot() string {
	if explicit := strings.TrimSpace(os.Getenv("RESEARCH_ROOT")); explicit != "" {
		return explicit
	}
	if base := strings.TrimSpace(os.Getenv("RESEARCH_TREE_ROOT")); base != "" {
		return filepath.Join(base, ".research")
	}
	return ".research"
}
