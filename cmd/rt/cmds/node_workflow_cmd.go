package cmds

import (
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/frudas24/research-tree/pkg/retree"
	"github.com/spf13/cobra"
)

// newNodeCloseCmd adds a strict close helper: done + mandatory outcome.
func newNodeCloseCmd(opts *RootOptions) *cobra.Command {
	var outcome, appendBody string
	cmd := &cobra.Command{
		Use:   "close <id>",
		Short: "Close node as done with mandatory outcome",
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
			n, err := store.GetNode(id)
			if err != nil {
				return err
			}
			n.Status = retree.StatusDone
			if strings.TrimSpace(outcome) != "" {
				n.Outcome = parseOutcome(outcome)
			}
			if err := validateTerminalOutcome(n.Status, n.Outcome); err != nil {
				return err
			}
			if appendBody != "" {
				n.Body = appendMarkdownBlock(n.Body, appendBody)
			}
			n.Modified = time.Now().UTC()
			if err := store.UpdateNode(n); err != nil {
				return err
			}
			return printMaybeJSON(cmd, opts.OutputJSON, n, fmt.Sprintf("closed node %04d (%s)", n.ID, n.Outcome))
		},
	}
	cmd.Flags().StringVar(&outcome, "outcome", "", "Outcome: success|failure|inconclusive (required if currently unset)")
	cmd.Flags().StringVar(&appendBody, "append-body", "", "Optional close note appended to body")
	return cmd
}

// newNodeLogRunCmd appends normalized run metadata into node body and can
// attach run outdir as artifact.
func newNodeLogRunCmd(opts *RootOptions) *cobra.Command {
	var resourceID, endpoint, endpointKind, artifactHost, runCmd, outDir, seed, eta, cost, note, artifactDesc string
	var addArtifact bool
	var projectBody bool
	var allowUnleasedResource bool
	var valid bool
	var invalidReason string
	cmd := &cobra.Command{
		Use:   "logrun <id>",
		Short: "Append structured run metadata (host/cmd/outdir/seed/eta/cost)",
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
			n, err := store.GetNode(id)
			if err != nil {
				return err
			}
			if strings.TrimSpace(artifactDesc) == "" {
				artifactDesc = "run-outdir"
			}
			if strings.TrimSpace(resourceID) != "" {
				resource, err := store.GetResource(resourceID)
				if err != nil {
					return err
				}
				if strings.TrimSpace(endpoint) == "" {
					endpoint = resource.Endpoint
					endpointKind = string(resource.EndpointKind)
				}
				if !allowUnleasedResource {
					leases, err := store.GetNodeResourceLeases(id)
					if err != nil {
						return err
					}
					matched := false
					for _, lease := range leases {
						if lease.ResourceID == resourceID {
							matched = true
							break
						}
					}
					if !matched {
						return fmt.Errorf("resource %s is not currently claimed by node %04d", resourceID, id)
					}
				}
			}
			runTimestamp := time.Now().UTC()
			runRecord := retree.RunRecord{
				Timestamp:     runTimestamp,
				ResourceID:    resourceID,
				Endpoint:      endpoint,
				EndpointKind:  retree.EndpointKind(strings.TrimSpace(endpointKind)),
				Command:       runCmd,
				OutDir:        outDir,
				Seed:          seed,
				ETA:           eta,
				Cost:          cost,
				Note:          note,
				Valid:         &valid,
				InvalidReason: invalidReason,
			}
			n.Runs = append(n.Runs, runRecord)
			if projectBody {
				n.Body = upsertRunMetaBlock(n.Body, renderRunMetaBlock(runRecord))
			}

			if addArtifact && outDir != "" {
				if strings.TrimSpace(artifactHost) == "" {
					switch {
					case strings.TrimSpace(resourceID) != "":
						artifactHost = resourceID
					case strings.TrimSpace(endpoint) != "":
						artifactHost = endpoint
					default:
						artifactHost = "local"
					}
				}
				exists := false
				for _, a := range n.Artifacts {
					if a.Mode == retree.ArtifactPath && a.Host == artifactHost && a.Path == outDir {
						exists = true
						break
					}
				}
				if !exists {
					n.Artifacts = append(n.Artifacts, retree.Artifact{
						Mode:        retree.ArtifactPath,
						Host:        artifactHost,
						Path:        outDir,
						Description: artifactDesc,
					})
				}
			}

			n.Modified = time.Now().UTC()
			if err := store.UpdateNode(n); err != nil {
				return err
			}
			return printMaybeJSON(cmd, opts.OutputJSON, n, fmt.Sprintf("run metadata appended to %04d", n.ID))
		},
	}
	cmd.Flags().StringVar(&resourceID, "resource-id", "", "Claimed resource identifier used for this run")
	cmd.Flags().StringVar(&endpoint, "endpoint", "", "Technical endpoint used by the run (IP or DNS)")
	cmd.Flags().StringVar(&endpointKind, "endpoint-kind", string(retree.EndpointNone), "Endpoint kind: none|ip|dns")
	cmd.Flags().StringVar(&runCmd, "cmd", "", "Run command")
	cmd.Flags().StringVar(&outDir, "outdir", "", "Output dir / artifact path")
	cmd.Flags().StringVar(&seed, "seed", "", "Seed used in run")
	cmd.Flags().StringVar(&eta, "eta", "", "ETA or duration summary")
	cmd.Flags().StringVar(&cost, "cost", "", "Cost summary (hours/$/tokens)")
	cmd.Flags().StringVar(&note, "note", "", "Extra note")
	cmd.Flags().BoolVar(&valid, "valid", true, "Whether this run should be treated as valid evidence")
	cmd.Flags().StringVar(&invalidReason, "invalid-reason", "", "Why the run is invalid (used when --valid=false)")
	cmd.Flags().BoolVar(&addArtifact, "add-artifact", true, "Also attach --outdir as path artifact (if provided)")
	cmd.Flags().BoolVar(&projectBody, "project-body", false, "Also project the latest run into node body as a single run-meta block")
	cmd.Flags().BoolVar(&allowUnleasedResource, "allow-unleased-resource", false, "Allow logging a run against a resource not currently claimed by the node")
	cmd.Flags().StringVar(&artifactHost, "artifact-host", "", "Artifact host label for --outdir path artifacts")
	cmd.Flags().StringVar(&artifactDesc, "artifact-desc", "run-outdir", "Artifact description when --add-artifact")
	return cmd
}

// renderRunMetaBlock renders one canonical markdown run-meta block from a structured run.
func renderRunMetaBlock(run retree.RunRecord) string {
	var b strings.Builder
	b.WriteString("### run-meta ")
	b.WriteString(run.Timestamp.UTC().Format("2006-01-02 15:04:05"))
	b.WriteString(" UTC\n")
	b.WriteString("```yaml\n")
	if run.ResourceID != "" {
		fmt.Fprintf(&b, "resource_id: %q\n", run.ResourceID)
	}
	if run.Endpoint != "" {
		fmt.Fprintf(&b, "endpoint: %q\n", run.Endpoint)
	}
	if run.EndpointKind != "" && run.EndpointKind != retree.EndpointNone {
		fmt.Fprintf(&b, "endpoint_kind: %q\n", run.EndpointKind)
	}
	if run.Host != "" {
		fmt.Fprintf(&b, "legacy_host: %q\n", run.Host)
	}
	if run.Command != "" {
		fmt.Fprintf(&b, "cmd: %q\n", run.Command)
	}
	if run.OutDir != "" {
		fmt.Fprintf(&b, "outdir: %q\n", run.OutDir)
	}
	if run.Seed != "" {
		fmt.Fprintf(&b, "seed: %q\n", run.Seed)
	}
	if run.ETA != "" {
		fmt.Fprintf(&b, "eta: %q\n", run.ETA)
	}
	if run.Cost != "" {
		fmt.Fprintf(&b, "cost: %q\n", run.Cost)
	}
	if run.Note != "" {
		fmt.Fprintf(&b, "note: %q\n", run.Note)
	}
	if run.Valid != nil {
		fmt.Fprintf(&b, "valid: %t\n", *run.Valid)
	}
	if run.InvalidReason != "" {
		fmt.Fprintf(&b, "invalid_reason: %q\n", run.InvalidReason)
	}
	b.WriteString("```\n")
	return b.String()
}

// newNodeLinkCmd links node<->commit<->artifact in one step.
func newNodeLinkCmd(opts *RootOptions) *cobra.Command {
	var commit, message, repo, artifactPath, artifactHost, artifactDesc string
	cmd := &cobra.Command{
		Use:   "link <id>",
		Short: "Link current git commit and/or artifact to node",
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
			n, err := store.GetNode(id)
			if err != nil {
				return err
			}
			if strings.TrimSpace(repo) == "" {
				repo = "."
			}
			if strings.TrimSpace(commit) == "" {
				commit = "auto"
			}
			if commit != "none" {
				hash := commit
				msg := message
				if commit == "auto" || strings.EqualFold(commit, "head") {
					var err error
					hash, msg, err = gitHead(repo)
					if err != nil {
						return err
					}
					if message != "" {
						msg = message
					}
				}
				exists := false
				for _, c := range n.Commits {
					if strings.EqualFold(c.Hash, hash) {
						exists = true
						break
					}
				}
				if !exists {
					n.Commits = append(n.Commits, retree.GitCommit{Hash: hash, Message: msg})
				}
			}
			if strings.TrimSpace(artifactPath) != "" {
				if strings.TrimSpace(artifactHost) == "" {
					artifactHost = "local"
				}
				exists := false
				for _, a := range n.Artifacts {
					if a.Mode == retree.ArtifactPath && a.Host == artifactHost && a.Path == artifactPath {
						exists = true
						break
					}
				}
				if !exists {
					n.Artifacts = append(n.Artifacts, retree.Artifact{
						Mode:        retree.ArtifactPath,
						Host:        artifactHost,
						Path:        artifactPath,
						Description: artifactDesc,
					})
				}
			}
			n.Modified = time.Now().UTC()
			if err := store.UpdateNode(n); err != nil {
				return err
			}
			return printMaybeJSON(cmd, opts.OutputJSON, n, fmt.Sprintf("linked metadata into %04d", n.ID))
		},
	}
	cmd.Flags().StringVar(&commit, "commit", "auto", "Commit hash | head | auto | none")
	cmd.Flags().StringVar(&message, "message", "", "Optional commit message override")
	cmd.Flags().StringVar(&repo, "repo", ".", "Git repo path for --commit auto/head")
	cmd.Flags().StringVar(&artifactPath, "artifact", "", "Artifact path to attach")
	cmd.Flags().StringVar(&artifactHost, "host", "local", "Artifact host for --artifact")
	cmd.Flags().StringVar(&artifactDesc, "artifact-desc", "", "Artifact description")
	return cmd
}

// gitHead resolves HEAD hash and subject for a repository path.
func gitHead(repo string) (hash string, subject string, err error) {
	out, err := exec.Command("git", "-C", repo, "rev-parse", "HEAD").Output()
	if err != nil {
		return "", "", fmt.Errorf("git rev-parse HEAD failed: %w", err)
	}
	hash = strings.TrimSpace(string(out))
	sub, err := exec.Command("git", "-C", repo, "show", "-s", "--format=%s", hash).Output()
	if err != nil {
		return hash, "", fmt.Errorf("git show --format=%%s failed: %w", err)
	}
	subject = strings.TrimSpace(string(sub))
	return hash, subject, nil
}
