package cmds

import (
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"

	"github.com/frudas24/research-tree/pkg/retree"
	"github.com/spf13/cobra"
)

// nodeFieldDiff captures one changed node field between two revisions.
type nodeFieldDiff struct {
	Field  string `json:"field"`
	Before any    `json:"before"`
	After  any    `json:"after"`
}

// nodeRevisionDiff is the stable CLI/tool payload for revision comparisons.
type nodeRevisionDiff struct {
	ID            retree.NodeID   `json:"id"`
	RevisionA     uint64          `json:"revision_a"`
	RevisionB     uint64          `json:"revision_b"`
	ModifiedA     string          `json:"modified_a"`
	ModifiedB     string          `json:"modified_b"`
	ChangedFields []string        `json:"changed_fields"`
	Changes       []nodeFieldDiff `json:"changes"`
}

// newNodeDiffCmd constructs the "node diff" subcommand for comparing revisions.
func newNodeDiffCmd(opts *RootOptions) *cobra.Command {
	var revA uint64
	var revB uint64
	cmd := &cobra.Command{
		Use:   "diff <id>",
		Short: "Diff two revisions of a node",
		Long: `Compare two revisions of a node.

Use --rev-a to pick the baseline revision.
Use --rev-b to pick the target revision; if omitted, the current revision is used.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if revA == 0 {
				return fmt.Errorf("--rev-a is required")
			}
			store, err := openStore(opts)
			if err != nil {
				return err
			}
			id, err := parseNodeID(args[0])
			if err != nil {
				return err
			}
			versions, current, err := collectNodeVersions(store, id)
			if err != nil {
				return err
			}
			a, ok := versions[revA]
			if !ok {
				return fmt.Errorf("revision %d not found for node %04d", revA, id)
			}
			targetRev := revB
			if targetRev == 0 {
				targetRev = current.Revision
			}
			b, ok := versions[targetRev]
			if !ok {
				return fmt.Errorf("revision %d not found for node %04d", targetRev, id)
			}
			diff := buildNodeRevisionDiff(id, a, b)
			if opts.OutputJSON {
				return printMaybeJSON(cmd, true, diff, "")
			}
			return printMaybeJSON(cmd, false, nil, renderNodeRevisionDiff(diff))
		},
	}
	cmd.Flags().Uint64Var(&revA, "rev-a", 0, "Baseline revision")
	cmd.Flags().Uint64Var(&revB, "rev-b", 0, "Target revision (defaults to current)")
	return cmd
}

// collectNodeVersions returns all available revisions plus the current node.
func collectNodeVersions(store *retree.Store, id retree.NodeID) (map[uint64]*retree.Node, *retree.Node, error) {
	history, err := store.GetNodeHistory(id)
	if err != nil {
		return nil, nil, err
	}
	current, err := store.GetNode(id)
	if err != nil {
		return nil, nil, err
	}
	versions := make(map[uint64]*retree.Node, len(history)+1)
	for _, n := range history {
		versions[n.Revision] = n
	}
	versions[current.Revision] = current
	return versions, current, nil
}

// buildNodeRevisionDiff computes a structured semantic diff between two node revisions.
func buildNodeRevisionDiff(id retree.NodeID, a *retree.Node, b *retree.Node) nodeRevisionDiff {
	changes := make([]nodeFieldDiff, 0, 16)
	appendNodeFieldDiff(&changes, "title", a.Title, b.Title)
	appendNodeFieldDiff(&changes, "status", a.Status, b.Status)
	appendNodeFieldDiff(&changes, "outcome", a.Outcome, b.Outcome)
	appendNodeFieldDiff(&changes, "claim_status", a.ClaimStatus, b.ClaimStatus)
	appendNodeFieldDiff(&changes, "scope", a.Scope, b.Scope)
	appendNodeFieldDiff(&changes, "exit_criteria", a.ExitCriteria, b.ExitCriteria)
	appendNodeFieldDiff(&changes, "parents", a.Parents, b.Parents)
	appendNodeFieldDiff(&changes, "continued_by", a.ContinuedBy, b.ContinuedBy)
	appendNodeFieldDiff(&changes, "superseded_by", a.SupersededBy, b.SupersededBy)
	appendNodeFieldDiff(&changes, "agent", a.Agent, b.Agent)
	appendNodeFieldDiff(&changes, "tags", a.Tags, b.Tags)
	appendNodeFieldDiff(&changes, "invalidated_by", a.InvalidatedBy, b.InvalidatedBy)
	appendNodeFieldDiff(&changes, "invalidation_reason", a.InvalidationReason, b.InvalidationReason)
	appendNodeFieldDiff(&changes, "commits", a.Commits, b.Commits)
	appendNodeFieldDiff(&changes, "artifacts", a.Artifacts, b.Artifacts)
	appendNodeFieldDiff(&changes, "runs", a.Runs, b.Runs)
	appendNodeFieldDiff(&changes, "body", a.Body, b.Body)

	fields := make([]string, 0, len(changes))
	for _, change := range changes {
		fields = append(fields, change.Field)
	}
	sort.Strings(fields)
	return nodeRevisionDiff{
		ID:            id,
		RevisionA:     a.Revision,
		RevisionB:     b.Revision,
		ModifiedA:     a.Modified.Format("2006-01-02 15:04"),
		ModifiedB:     b.Modified.Format("2006-01-02 15:04"),
		ChangedFields: fields,
		Changes:       changes,
	}
}

// appendNodeFieldDiff appends a diff entry when the values differ.
func appendNodeFieldDiff(changes *[]nodeFieldDiff, field string, before any, after any) {
	if reflect.DeepEqual(before, after) {
		return
	}
	*changes = append(*changes, nodeFieldDiff{
		Field:  field,
		Before: cloneDiffValue(before),
		After:  cloneDiffValue(after),
	})
}

// cloneDiffValue normalizes slices so JSON/text rendering is stable and detached.
func cloneDiffValue(value any) any {
	switch v := value.(type) {
	case []string:
		return append([]string(nil), v...)
	case []retree.NodeID:
		return append([]retree.NodeID(nil), v...)
	case []retree.GitCommit:
		return append([]retree.GitCommit(nil), v...)
	case []retree.Artifact:
		return append([]retree.Artifact(nil), v...)
	case []retree.RunRecord:
		return append([]retree.RunRecord(nil), v...)
	default:
		return value
	}
}

// renderNodeRevisionDiff renders a concise human diff for CLI inspection.
func renderNodeRevisionDiff(diff nodeRevisionDiff) string {
	lines := []string{
		fmt.Sprintf("Diff for %04d rev %d -> rev %d:", diff.ID, diff.RevisionA, diff.RevisionB),
		fmt.Sprintf("  modified: %s -> %s", diff.ModifiedA, diff.ModifiedB),
	}
	if len(diff.Changes) == 0 {
		lines = append(lines, "  no semantic changes")
		return strings.Join(lines, "\n")
	}
	for _, change := range diff.Changes {
		lines = append(lines, fmt.Sprintf("  %s:", change.Field))
		lines = append(lines, fmt.Sprintf("    - %s", formatNodeDiffValue(change.Before)))
		lines = append(lines, fmt.Sprintf("    + %s", formatNodeDiffValue(change.After)))
	}
	return strings.Join(lines, "\n")
}

// formatNodeDiffValue renders one diff value on a single stable line.
func formatNodeDiffValue(value any) string {
	raw, err := json.Marshal(value)
	if err != nil {
		return fmt.Sprintf("%v", value)
	}
	s := string(raw)
	if len(s) > 160 {
		s = s[:157] + "..."
	}
	return s
}
