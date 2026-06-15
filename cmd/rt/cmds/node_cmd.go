package cmds

import (
	"fmt"
	"strings"
	"time"

	"github.com/frudas24/research-tree/pkg/retree"
	"github.com/spf13/cobra"
)

// newNodeCmd constructs the "node" subcommand group.
func newNodeCmd(opts *RootOptions) *cobra.Command {
	cmd := &cobra.Command{Use: "node", Short: "Node operations"}
	cmd.AddCommand(newNodeCreateCmd(opts))
	cmd.AddCommand(newNodeShowCmd(opts))
	cmd.AddCommand(newNodeEditCmd(opts))
	cmd.AddCommand(newNodeCloseCmd(opts))
	cmd.AddCommand(newNodeLogRunCmd(opts))
	cmd.AddCommand(newNodeLinkCmd(opts))
	cmd.AddCommand(newNodeDeleteCmd(opts))
	cmd.AddCommand(newNodeListCmd(opts))
	cmd.AddCommand(newNodeInvalidateCmd(opts))
	cmd.AddCommand(newNodeHistoryCmd(opts))
	cmd.AddCommand(newNodeDiffCmd(opts))
	cmd.AddCommand(newNodeAncestorsCmd(opts))
	cmd.AddCommand(newNodeDescendantsCmd(opts))
	cmd.AddCommand(newNodeImportCmd(opts))
	cmd.AddCommand(newNodeBatchCmd(opts))
	return cmd
}

// newNodeCreateCmd constructs the "node create" subcommand.
func newNodeCreateCmd(opts *RootOptions) *cobra.Command {
	var title, status, claimStatus, milestoneClass, milestoneKind, milestoneReason, scope, exitCriteria, parentsCSV, continuedByCSV, supersededByCSV, agent, tagsCSV, bodyInline, bodyFile, relationsCSV, primaryParentStr string
	var outcome string
	var useEditor bool
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create node",
		Long: `Create a new research node.

The --parents flag accepts numeric IDs (1,2,3) or title substrings.
Title matching is case-insensitive and requires a unique match.

Body can be provided via --body (inline), --body-file (read from file),
or --edit (open $EDITOR). If --edit is set, the initial editor content
is taken from --body or --body-file if provided.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			store, err := openStore(opts)
			if err != nil {
				return err
			}
			parents, err := resolveParents(store, parentsCSV)
			if err != nil {
				return err
			}
			continuedBy, err := resolveParents(store, continuedByCSV)
			if err != nil {
				return err
			}
			supersededBy, err := resolveParents(store, supersededByCSV)
			if err != nil {
				return err
			}
			body, err := resolveBody(bodyInline, bodyFile, useEditor)
			if err != nil {
				return err
			}
			relations, err := resolveRelations(store, relationsCSV)
			if err != nil {
				return err
			}
			var primaryParent *retree.NodeID
			if strings.TrimSpace(primaryParentStr) != "" {
				pp, err := parseNodeID(primaryParentStr)
				if err != nil {
					return fmt.Errorf("--primary-parent: %w", err)
				}
				primaryParent = &pp
			}
			n := &retree.Node{
				Frontmatter: retree.Frontmatter{
					Title:           title,
					Status:          parseNodeStatus(status),
					ClaimStatus:     parseClaimStatus(claimStatus),
					MilestoneClass:  retree.MilestoneClass(strings.TrimSpace(milestoneClass)),
					MilestoneKind:   retree.MilestoneKind(strings.TrimSpace(milestoneKind)),
					MilestoneReason: strings.TrimSpace(milestoneReason),
					Scope:           scope,
					ExitCriteria:    exitCriteria,
					Outcome:         parseOutcome(outcome),
					Parents:         parents,
					ContinuedBy:     continuedBy,
					SupersededBy:    supersededBy,
					Agent:           agent,
					Tags:            parseCSVStrings(tagsCSV),
					Relations:       relations,
					PrimaryParent:   primaryParent,
				},
				Body: body,
			}
			if err := validateTerminalOutcome(n.Status, n.Outcome); err != nil {
				return err
			}
			if err := store.CreateNode(n); err != nil {
				return err
			}
			return printMaybeJSON(cmd, opts.OutputJSON, n, fmt.Sprintf("created node %04d", n.ID))
		},
	}
	cmd.Flags().StringVar(&title, "title", "", "Node title (required)")
	cmd.Flags().StringVar(&status, "status", "active", "Node status")
	cmd.Flags().StringVar(&claimStatus, "claim-status", "provisional", "Claim status")
	cmd.Flags().StringVar(&milestoneClass, "milestone-class", "", "Milestone class (e.g. golden)")
	cmd.Flags().StringVar(&milestoneKind, "milestone-kind", "", "Milestone kind (champion|breakthrough|pivot)")
	cmd.Flags().StringVar(&milestoneReason, "milestone-reason", "", "Required reason for golden milestones")
	cmd.Flags().StringVar(&scope, "scope", "", "Scope boundary for the claim or result")
	cmd.Flags().StringVar(&exitCriteria, "exit-criteria", "", "Explicit closure criteria for the node")
	cmd.Flags().StringVar(&outcome, "outcome", "", "Outcome: unset|success|failure|inconclusive")
	cmd.Flags().StringVar(&parentsCSV, "parents", "", "Comma-separated parent IDs or title substrings")
	cmd.Flags().StringVar(&continuedByCSV, "continued-by", "", "Comma-separated continuation node IDs or title substrings")
	cmd.Flags().StringVar(&supersededByCSV, "superseded-by", "", "Comma-separated superseding node IDs or title substrings")
	cmd.Flags().StringVar(&agent, "agent", "", "Node agent")
	cmd.Flags().StringVar(&tagsCSV, "tags", "", "Comma-separated tags")
	cmd.Flags().StringVar(&bodyInline, "body", "", "Body markdown (inline)")
	cmd.Flags().StringVar(&bodyFile, "body-file", "", "Body markdown file")
	cmd.Flags().BoolVar(&useEditor, "edit", false, "Open $EDITOR to write body")
	cmd.Flags().StringVar(&relationsCSV, "relation", "", "Add typed relation (type:target, e.g. compares_against:5)")
	cmd.Flags().StringVar(&primaryParentStr, "primary-parent", "", "Designate a primary parent ID")
	_ = cmd.MarkFlagRequired("title")
	return cmd
}

// newNodeShowCmd constructs the "node show" subcommand.
func newNodeShowCmd(opts *RootOptions) *cobra.Command {
	var view string
	var agentView bool
	cmd := &cobra.Command{
		Use:   "show <id>",
		Short: "Show node details",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			store, err := openStore(opts)
			if err != nil {
				return err
			}
			cc := newColorizer(opts.ColorMode)
			id, err := parseNodeID(args[0])
			if err != nil {
				return err
			}
			n, err := store.GetNode(id)
			if err != nil {
				return err
			}
			children, _ := store.GetChildren(id)
			leases, _ := store.GetNodeResourceLeases(id)
			if agentView {
				latestRun, _ := latestRunMeta(n)
				if opts.OutputJSON {
					return printMaybeJSON(cmd, true, map[string]any{
						"handoff_version":  "v1",
						"id":               n.ID,
						"title":            n.Title,
						"display_title":    titleWithVerdict(n),
						"status":           n.Status,
						"outcome":          n.Outcome,
						"claim_status":     n.ClaimStatus,
						"scope":            n.Scope,
						"exit_criteria":    n.ExitCriteria,
						"milestone_class":  n.MilestoneClass,
						"milestone_kind":   n.MilestoneKind,
						"milestone_reason": n.MilestoneReason,
						"agent":            n.Agent,
						"tags":             n.Tags,
						"parents":          n.Parents,
						"continued_by":     n.ContinuedBy,
						"superseded_by":    n.SupersededBy,
						"children":         children,
						"artifacts":        n.Artifacts,
						"commits":          n.Commits,
						"runs":             n.Runs,
						"active_resources": leases,
						"latest_run":       latestRun,
						"summary":          summaryLine(n.Body),
						"lineage": map[string]any{
							"parents":       n.Parents,
							"children":      children,
							"continued_by":  n.ContinuedBy,
							"superseded_by": n.SupersededBy,
						},
						"evidence": map[string]any{
							"commits":    n.Commits,
							"artifacts":  n.Artifacts,
							"runs":       n.Runs,
							"latest_run": latestRun,
							"resources":  leases,
						},
						"revision": n.Revision,
						"created":  n.Created,
						"modified": n.Modified,
					}, "")
				}
				return printMaybeJSON(cmd, false, nil, formatNodeAgentView(cc, n, children, leases))
			}
			full := !strings.EqualFold(view, "summary")
			if opts.OutputJSON && !full {
				return printMaybeJSON(cmd, true, map[string]any{
					"id":               n.ID,
					"title":            n.Title,
					"status":           n.Status,
					"outcome":          n.Outcome,
					"claim_status":     n.ClaimStatus,
					"scope":            n.Scope,
					"exit_criteria":    n.ExitCriteria,
					"milestone_class":  n.MilestoneClass,
					"milestone_kind":   n.MilestoneKind,
					"milestone_reason": n.MilestoneReason,
					"agent":            n.Agent,
					"tags":             n.Tags,
					"parents":          n.Parents,
					"continued_by":     n.ContinuedBy,
					"superseded_by":    n.SupersededBy,
					"children":         children,
					"runs":             n.Runs,
					"active_resources": leases,
					"artifacts":        len(n.Artifacts),
					"commits":          len(n.Commits),
					"revision":         n.Revision,
					"created":          n.Created,
					"modified":         n.Modified,
				}, "")
			}
			if opts.OutputJSON {
				return printMaybeJSON(cmd, true, map[string]any{
					"schema_version":      n.SchemaVersion,
					"id":                  n.ID,
					"title":               n.Title,
					"status":              n.Status,
					"claim_status":        n.ClaimStatus,
					"parents":             n.Parents,
					"continued_by":        n.ContinuedBy,
					"superseded_by":       n.SupersededBy,
					"tags":                n.Tags,
					"created":             n.Created,
					"modified":            n.Modified,
					"outcome":             n.Outcome,
					"revision":            n.Revision,
					"scope":               n.Scope,
					"exit_criteria":       n.ExitCriteria,
					"milestone_class":     n.MilestoneClass,
					"milestone_kind":      n.MilestoneKind,
					"milestone_reason":    n.MilestoneReason,
					"agent":               n.Agent,
					"commits":             n.Commits,
					"runs":                n.Runs,
					"artifacts":           n.Artifacts,
					"invalidated_by":      n.InvalidatedBy,
					"invalidation_reason": n.InvalidationReason,
					"body":                n.Body,
					"active_resources":    leases,
				}, "")
			}
			return printMaybeJSON(cmd, false, nil, formatNodeHuman(cc, n, children, leases, full))
		},
	}
	cmd.Flags().StringVar(&view, "view", "full", "View mode: summary|full")
	cmd.Flags().BoolVar(&agentView, "agent", false, "Compact handoff-oriented view for agents")
	return cmd
}

// newNodeEditCmd constructs the "node edit" subcommand.
func newNodeEditCmd(opts *RootOptions) *cobra.Command {
	var status, claimStatus, milestoneClass, milestoneKind, milestoneReason, scope, exitCriteria, addTags, rmTags, parentsCSV, addParentsCSV, rmParentsCSV, continuedByCSV, supersededByCSV, bodyInline, bodyFile, appendBody, relationsCSV, addRelationsCSV, rmRelationsCSV, primaryParentStr string
	var outcome string
	var useEditor bool
	cmd := &cobra.Command{
		Use:   "edit <id>",
		Short: "Edit node fields",
		Long: `Edit metadata and body of an existing node.

Body can be replaced via --body (inline), --body-file (read from file),
or --edit (open $EDITOR with current body). Use --append-body to add
text at the end instead of replacing.`,
		Args: cobra.ExactArgs(1),
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
			if parentsCSV != "" {
				resolved, err := resolveParents(store, parentsCSV)
				if err != nil {
					return err
				}
				n.Parents = resolved
			}
			if addParentsCSV != "" {
				resolved, err := resolveParents(store, addParentsCSV)
				if err != nil {
					return err
				}
				n.Parents = uniqueNodeIDs(append(n.Parents, resolved...))
			}
			if rmParentsCSV != "" {
				resolved, err := resolveParents(store, rmParentsCSV)
				if err != nil {
					return err
				}
				toRemove := make(map[retree.NodeID]struct{}, len(resolved))
				for _, pid := range resolved {
					toRemove[pid] = struct{}{}
				}
				filtered := make([]retree.NodeID, 0, len(n.Parents))
				for _, pid := range n.Parents {
					if _, drop := toRemove[pid]; drop {
						continue
					}
					filtered = append(filtered, pid)
				}
				n.Parents = uniqueNodeIDs(filtered)
			}
			if strings.TrimSpace(status) != "" {
				n.Status = parseNodeStatus(status)
			}
			if strings.TrimSpace(claimStatus) != "" {
				n.ClaimStatus = parseClaimStatus(claimStatus)
			}
			if cmd.Flags().Changed("milestone-class") {
				n.MilestoneClass = retree.MilestoneClass(strings.TrimSpace(milestoneClass))
			}
			if cmd.Flags().Changed("milestone-kind") {
				n.MilestoneKind = retree.MilestoneKind(strings.TrimSpace(milestoneKind))
			}
			if cmd.Flags().Changed("milestone-reason") {
				n.MilestoneReason = strings.TrimSpace(milestoneReason)
			}
			if strings.TrimSpace(scope) != "" {
				n.Scope = strings.TrimSpace(scope)
			}
			if strings.TrimSpace(exitCriteria) != "" {
				n.ExitCriteria = strings.TrimSpace(exitCriteria)
			}
			if strings.TrimSpace(outcome) != "" {
				n.Outcome = parseOutcome(outcome)
			}
			if continuedByCSV != "" {
				resolved, err := resolveParents(store, continuedByCSV)
				if err != nil {
					return err
				}
				n.ContinuedBy = uniqueNodeIDs(resolved)
			}
			if supersededByCSV != "" {
				resolved, err := resolveParents(store, supersededByCSV)
				if err != nil {
					return err
				}
				n.SupersededBy = uniqueNodeIDs(resolved)
			}
			// Relations
			if relationsCSV != "" {
				resolved, err := resolveRelations(store, relationsCSV)
				if err != nil {
					return err
				}
				n.Relations = resolved
			}
			if addRelationsCSV != "" {
				resolved, err := resolveRelations(store, addRelationsCSV)
				if err != nil {
					return err
				}
				n.Relations = append(n.Relations, resolved...)
			}
			if rmRelationsCSV != "" {
				resolved, err := resolveRelations(store, rmRelationsCSV)
				if err != nil {
					return err
				}
				toRemove := make(map[string]struct{}, len(resolved))
				for _, rel := range resolved {
					toRemove[fmt.Sprintf("%s:%d", rel.Type, rel.Target)] = struct{}{}
				}
				filtered := make([]retree.Relation, 0, len(n.Relations))
				for _, rel := range n.Relations {
					key := fmt.Sprintf("%s:%d", rel.Type, rel.Target)
					if _, drop := toRemove[key]; drop {
						continue
					}
					filtered = append(filtered, rel)
				}
				n.Relations = filtered
			}
			if cmd.Flags().Changed("primary-parent") {
				if strings.TrimSpace(primaryParentStr) == "" {
					n.PrimaryParent = nil
				} else {
					pp, err := parseNodeID(primaryParentStr)
					if err != nil {
						return fmt.Errorf("--primary-parent: %w", err)
					}
					n.PrimaryParent = &pp
				}
			}
			n.Tags = append(n.Tags, parseCSVStrings(addTags)...)
			toRm := map[string]struct{}{}
			for _, tag := range parseCSVStrings(rmTags) {
				toRm[tag] = struct{}{}
			}
			compacted := make([]string, 0, len(n.Tags))
			seen := map[string]struct{}{}
			for _, tag := range n.Tags {
				if _, drop := toRm[tag]; drop {
					continue
				}
				if _, ok := seen[tag]; ok {
					continue
				}
				seen[tag] = struct{}{}
				compacted = append(compacted, tag)
			}
			n.Tags = compacted

			// Body replacement: --edit takes priority, then --body, then --body-file
			switch {
			case useEditor:
				edited, err := editBody(n.Body)
				if err != nil {
					return err
				}
				n.Body = edited
			case bodyInline != "":
				n.Body = bodyInline
			case bodyFile != "":
				b, err := readBody(bodyFile)
				if err != nil {
					return err
				}
				n.Body = b
			case appendBody != "":
				n.Body = appendMarkdownBlock(n.Body, appendBody)
			}
			if err := validateTerminalOutcome(n.Status, n.Outcome); err != nil {
				return err
			}

			n.Modified = time.Now().UTC()
			if err := store.UpdateNode(n); err != nil {
				return err
			}
			return printMaybeJSON(cmd, opts.OutputJSON, n, fmt.Sprintf("updated node %04d", n.ID))
		},
	}
	cmd.Flags().StringVar(&status, "status", "", "New status")
	cmd.Flags().StringVar(&claimStatus, "claim-status", "", "New claim status")
	cmd.Flags().StringVar(&milestoneClass, "milestone-class", "", "Set milestone class (empty clears)")
	cmd.Flags().StringVar(&milestoneKind, "milestone-kind", "", "Set milestone kind (empty clears)")
	cmd.Flags().StringVar(&milestoneReason, "milestone-reason", "", "Set milestone reason (empty clears)")
	cmd.Flags().StringVar(&scope, "scope", "", "Set scope boundary")
	cmd.Flags().StringVar(&exitCriteria, "exit-criteria", "", "Set exit criteria")
	cmd.Flags().StringVar(&outcome, "outcome", "", "Outcome: unset|success|failure|inconclusive")
	cmd.Flags().StringVar(&addTags, "add-tags", "", "Comma-separated tags to add")
	cmd.Flags().StringVar(&rmTags, "rm-tags", "", "Comma-separated tags to remove")
	cmd.Flags().StringVar(&bodyInline, "body", "", "Replace body with inline markdown")
	cmd.Flags().StringVar(&parentsCSV, "parents", "", "Replace parents (comma-separated IDs or title substrings)")
	cmd.Flags().StringVar(&addParentsCSV, "add-parents", "", "Add parents (comma-separated IDs or title substrings)")
	cmd.Flags().StringVar(&rmParentsCSV, "rm-parents", "", "Remove parents (comma-separated IDs or title substrings)")
	cmd.Flags().StringVar(&continuedByCSV, "continued-by", "", "Replace continuation links (comma-separated IDs or title substrings)")
	cmd.Flags().StringVar(&supersededByCSV, "superseded-by", "", "Replace superseded-by links (comma-separated IDs or title substrings)")
	cmd.Flags().StringVar(&bodyFile, "body-file", "", "Replace body from file")
	cmd.Flags().BoolVar(&useEditor, "edit", false, "Open $EDITOR to edit body")
	cmd.Flags().StringVar(&appendBody, "append-body", "", "Text to append to body")
	cmd.Flags().StringVar(&relationsCSV, "relation", "", "Replace relations (type:target, e.g. compares_against:5)")
	cmd.Flags().StringVar(&addRelationsCSV, "add-relation", "", "Add relation (type:target)")
	cmd.Flags().StringVar(&rmRelationsCSV, "rm-relation", "", "Remove relation (type:target)")
	cmd.Flags().StringVar(&primaryParentStr, "primary-parent", "", "Set primary parent ID (empty clears)")
	return cmd
}

// newNodeDeleteCmd constructs the "node delete" subcommand.
func newNodeDeleteCmd(opts *RootOptions) *cobra.Command {
	var force bool
	cmd := &cobra.Command{
		Use:   "delete <id>",
		Short: "Delete node",
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
			if err := store.DeleteNode(id, force); err != nil {
				return err
			}
			return printMaybeJSON(cmd, opts.OutputJSON, map[string]any{"deleted": id, "force": force}, fmt.Sprintf("deleted node %04d", id))
		},
	}
	cmd.Flags().BoolVar(&force, "force", false, "Force delete and orphan children")
	return cmd
}

// newNodeListCmd constructs the "node list" subcommand.
func newNodeListCmd(opts *RootOptions) *cobra.Command {
	var status, claimStatus, outcome, milestoneClass, milestoneKind, tag, tagsAllCSV, tagsAnyCSV, agent, titleContains, scopeContains, bodyContains, sortBy, order, hasArtifact, continuedByRef, supersededByRef string
	var limit, offset int
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List nodes",
		RunE: func(cmd *cobra.Command, args []string) error {
			store, err := openStore(opts)
			if err != nil {
				return err
			}
			cc := newColorizer(opts.ColorMode)
			continuedBy := retree.NodeID(0)
			if strings.TrimSpace(continuedByRef) != "" {
				resolved, err := resolveParents(store, continuedByRef)
				if err != nil {
					return err
				}
				if len(resolved) != 1 {
					return fmt.Errorf("--continued-by requires exactly one resolved node, got %d", len(resolved))
				}
				continuedBy = resolved[0]
			}
			supersededBy := retree.NodeID(0)
			if strings.TrimSpace(supersededByRef) != "" {
				resolved, err := resolveParents(store, supersededByRef)
				if err != nil {
					return err
				}
				if len(resolved) != 1 {
					return fmt.Errorf("--superseded-by requires exactly one resolved node, got %d", len(resolved))
				}
				supersededBy = resolved[0]
			}
			ids, err := store.ListNodes(retree.Filter{
				Status:         parseNodeStatus(status),
				ClaimStatus:    parseClaimStatus(claimStatus),
				Outcome:        parseOutcome(outcome),
				MilestoneClass: retree.MilestoneClass(strings.TrimSpace(milestoneClass)),
				MilestoneKind:  retree.MilestoneKind(strings.TrimSpace(milestoneKind)),
				Tag:            tag,
				TagsAll:        parseCSVStrings(tagsAllCSV),
				TagsAny:        parseCSVStrings(tagsAnyCSV),
				Agent:          agent,
				TitleContains:  titleContains,
				ScopeContains:  scopeContains,
				BodyContains:   bodyContains,
				ContinuedBy:    continuedBy,
				SupersededBy:   supersededBy,
				HasArtifact:    parseOptionalBool(hasArtifact),
				SortBy:         sortBy,
				Order:          order,
				Offset:         offset,
				Limit:          limit,
			})
			if err != nil {
				return err
			}
			if opts.OutputJSON {
				return printMaybeJSON(cmd, true, ids, "")
			}
			var lines []string
			for _, id := range ids {
				n, _ := store.GetNode(id)
				if n == nil {
					continue
				}
				title := cc.golden(n.MilestoneClass, titleWithVerdict(n))
				lines = append(lines, fmt.Sprintf("%04d | %s | %s | %s | %s", n.ID, n.Status, n.ClaimStatus, title, n.Agent))
			}
			return printMaybeJSON(cmd, false, nil, strings.Join(lines, "\n"))
		},
	}
	cmd.Flags().StringVar(&status, "status", "", "Filter by status")
	cmd.Flags().StringVar(&claimStatus, "claim-status", "", "Filter by claim status")
	cmd.Flags().StringVar(&outcome, "outcome", "", "Filter by outcome")
	cmd.Flags().StringVar(&milestoneClass, "milestone-class", "", "Filter by milestone class")
	cmd.Flags().StringVar(&milestoneKind, "milestone-kind", "", "Filter by milestone kind")
	cmd.Flags().StringVar(&tag, "tag", "", "Filter by tag")
	cmd.Flags().StringVar(&tagsAllCSV, "tags-all", "", "Filter by requiring all tags (comma-separated)")
	cmd.Flags().StringVar(&tagsAnyCSV, "tags-any", "", "Filter by requiring any tag (comma-separated)")
	cmd.Flags().StringVar(&agent, "agent", "", "Filter by agent")
	cmd.Flags().StringVar(&titleContains, "title-contains", "", "Filter by title substring")
	cmd.Flags().StringVar(&scopeContains, "scope-contains", "", "Filter by scope substring")
	cmd.Flags().StringVar(&bodyContains, "body-contains", "", "Filter by body substring")
	cmd.Flags().StringVar(&continuedByRef, "continued-by", "", "Filter nodes whose continued_by contains this node ID or title substring")
	cmd.Flags().StringVar(&supersededByRef, "superseded-by", "", "Filter nodes whose superseded_by contains this node ID or title substring")
	cmd.Flags().StringVar(&hasArtifact, "has-artifact", "", "Filter by artifact presence: true|false")
	cmd.Flags().StringVar(&sortBy, "sort-by", "id", "Sort by: id|created|modified|title")
	cmd.Flags().StringVar(&order, "order", "asc", "Sort order: asc|desc")
	cmd.Flags().IntVar(&offset, "offset", 0, "Pagination offset")
	cmd.Flags().IntVar(&limit, "limit", 50, "Maximum rows")
	return cmd
}

// newNodeInvalidateCmd constructs the "node invalidate" subcommand.
func newNodeInvalidateCmd(opts *RootOptions) *cobra.Command {
	var by string
	var reason string
	cmd := &cobra.Command{
		Use:   "invalidate <id>",
		Short: "Invalidate claim and propagate warnings",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			store, err := openStore(opts)
			if err != nil {
				return err
			}
			target, err := parseNodeID(args[0])
			if err != nil {
				return err
			}
			refuter, err := parseNodeID(by)
			if err != nil {
				return err
			}
			if err := store.InvalidateClaim(target, refuter, reason); err != nil {
				return err
			}
			return printMaybeJSON(cmd, opts.OutputJSON, map[string]any{"invalidated": target, "by": refuter, "reason": reason}, fmt.Sprintf("invalidated node %04d by %04d", target, refuter))
		},
	}
	cmd.Flags().StringVar(&by, "by", "", "Refuter node ID")
	cmd.Flags().StringVar(&reason, "reason", "", "Invalidation reason")
	_ = cmd.MarkFlagRequired("by")
	return cmd
}

// newNodeHistoryCmd constructs the "node history" subcommand for viewing
// previous versions of a node.
func newNodeHistoryCmd(opts *RootOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "history <id>",
		Short: "Show edit history for a node",
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
			history, err := store.GetNodeHistory(id)
			if err != nil {
				return err
			}
			// Also show current version as the latest
			current, err := store.GetNode(id)
			if err != nil {
				return err
			}

			type historyEntry struct {
				Revision uint64 `json:"revision"`
				Modified string `json:"modified"`
				Title    string `json:"title"`
				Body     string `json:"body,omitempty"`
			}
			entries := make([]historyEntry, 0, len(history)+1)
			for _, h := range history {
				entries = append(entries, historyEntry{
					Revision: h.Revision,
					Modified: h.Modified.Format("2006-01-02 15:04"),
					Title:    h.Title,
					Body:     h.Body,
				})
			}
			entries = append(entries, historyEntry{
				Revision: current.Revision,
				Modified: current.Modified.Format("2006-01-02 15:04"),
				Title:    current.Title,
				Body:     current.Body,
			})

			if opts.OutputJSON {
				return printMaybeJSON(cmd, true, entries, "")
			}
			var lines []string
			lines = append(lines, fmt.Sprintf("History for %04d (%d versions):", id, len(entries)))
			for _, e := range entries {
				marker := "  "
				if e.Revision == current.Revision {
					marker = "▶ "
				}
				preview := e.Title
				if e.Body != "" {
					firstLine := strings.SplitN(e.Body, "\n", 2)[0]
					if len(firstLine) > 60 {
						firstLine = firstLine[:57] + "..."
					}
					preview += " — " + firstLine
				}
				lines = append(lines, fmt.Sprintf("%srev %d | %s | %s", marker, e.Revision, e.Modified, preview))
			}
			return printMaybeJSON(cmd, false, nil, strings.Join(lines, "\n"))
		},
	}
	return cmd
}

// newNodeAncestorsCmd constructs the "node ancestors" subcommand.
func newNodeAncestorsCmd(opts *RootOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "ancestors <id>",
		Short: "List ancestors of a node",
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
			ids, err := store.GetAncestors(id)
			if err != nil {
				return err
			}
			if opts.OutputJSON {
				nodes := make([]*retree.Node, 0, len(ids))
				for _, aid := range ids {
					n, _ := store.GetNode(aid)
					if n != nil {
						nodes = append(nodes, n)
					}
				}
				return printMaybeJSON(cmd, true, retree.SummarizeNodes(nodes), "")
			}
			var lines []string
			lines = append(lines, fmt.Sprintf("Ancestors of %04d (%d):", id, len(ids)))
			for _, aid := range ids {
				n, _ := store.GetNode(aid)
				if n != nil {
					lines = append(lines, fmt.Sprintf("  %s %04d %s", IconForNode(n), aid, n.Title))
				}
			}
			return printMaybeJSON(cmd, false, nil, strings.Join(lines, "\n"))
		},
	}
	return cmd
}

// newNodeDescendantsCmd constructs the "node descendants" subcommand.
func newNodeDescendantsCmd(opts *RootOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "descendants <id>",
		Short: "List descendants of a node",
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
			ids, err := store.GetDescendants(id)
			if err != nil {
				return err
			}
			if opts.OutputJSON {
				nodes := make([]*retree.Node, 0, len(ids))
				for _, did := range ids {
					n, _ := store.GetNode(did)
					if n != nil {
						nodes = append(nodes, n)
					}
				}
				return printMaybeJSON(cmd, true, retree.SummarizeNodes(nodes), "")
			}
			var lines []string
			lines = append(lines, fmt.Sprintf("Descendants of %04d (%d):", id, len(ids)))
			for _, did := range ids {
				n, _ := store.GetNode(did)
				if n != nil {
					lines = append(lines, fmt.Sprintf("  %s %04d %s", IconForNode(n), did, n.Title))
				}
			}
			return printMaybeJSON(cmd, false, nil, strings.Join(lines, "\n"))
		},
	}
	return cmd
}
