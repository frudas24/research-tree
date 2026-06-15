package cmds

import (
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"

	"github.com/frudas24/research-tree/pkg/retree"
)

// appendMarkdownBlock appends markdown content with exactly one separating
// newline when both sides are non-empty and neither already provides it.
func appendMarkdownBlock(body string, appendBody string) string {
	if appendBody == "" {
		return body
	}
	if body == "" {
		return appendBody
	}
	if strings.HasSuffix(body, "\n") || strings.HasPrefix(appendBody, "\n") {
		return body + appendBody
	}
	return body + "\n" + appendBody
}

// upsertRunMetaBlock rewrites the body so it contains at most one latest
// run-meta block, preserving the surrounding editorial content.
func upsertRunMetaBlock(body string, block string) string {
	base := stripRunMetaBlocks(body)
	base = strings.TrimRight(base, "\n")
	if strings.TrimSpace(block) == "" {
		return base
	}
	if base == "" {
		return strings.TrimLeft(block, "\n")
	}
	return base + "\n\n" + strings.TrimLeft(block, "\n")
}

// stripRunMetaBlocks removes all embedded run-meta yaml projections from a body.
func stripRunMetaBlocks(body string) string {
	if body == "" {
		return ""
	}
	lines := strings.Split(body, "\n")
	kept := make([]string, 0, len(lines))
	for i := 0; i < len(lines); i++ {
		line := lines[i]
		if !strings.HasPrefix(line, "### run-meta ") {
			kept = append(kept, line)
			continue
		}
		i++
		if i < len(lines) && lines[i] == "```yaml" {
			for i++; i < len(lines); i++ {
				if lines[i] == "```" {
					break
				}
			}
		}
	}
	out := strings.Join(kept, "\n")
	for strings.Contains(out, "\n\n\n") {
		out = strings.ReplaceAll(out, "\n\n\n", "\n\n")
	}
	return out
}

// parseNodeID parses a string into a NodeID.
func parseNodeID(s string) (retree.NodeID, error) {
	u, err := strconv.ParseUint(strings.TrimSpace(s), 10, 64)
	if err != nil {
		return 0, err
	}
	return retree.NodeID(u), nil
}

// resolveParents resolves a comma-separated parent spec to NodeIDs.
// Each token is first tried as a numeric ID. If that fails, it performs
// a case-insensitive substring search across all existing node titles.
// Ambiguous matches (multiple nodes matching the same substring) are
// rejected with an error listing the candidates.
func resolveParents(store *retree.Store, csv string) ([]retree.NodeID, error) {
	csv = strings.TrimSpace(csv)
	if csv == "" {
		return nil, nil
	}
	tokens := strings.Split(csv, ",")
	out := make([]retree.NodeID, 0, len(tokens))
	for _, tok := range tokens {
		tok = strings.TrimSpace(tok)
		if tok == "" {
			continue
		}
		// Try numeric first
		if id, err := parseNodeID(tok); err == nil {
			out = append(out, retree.NodeID(id))
			continue
		}
		// Fuzzy match by title substring
		ids, err := findNodesByTitle(store, tok)
		if err != nil {
			return nil, fmt.Errorf("parent %q: %w", tok, err)
		}
		out = append(out, ids...)
	}
	return out, nil
}

// findNodesByTitle performs a case-insensitive substring search across
// all node titles and returns matching IDs. Returns an error if zero or
// more than one match is found.
func findNodesByTitle(store *retree.Store, needle string) ([]retree.NodeID, error) {
	all, err := store.QueryNodes(retree.Filter{})
	if err != nil {
		return nil, err
	}
	needle = strings.ToLower(strings.TrimSpace(needle))
	var matches []retree.NodeID
	for _, n := range all {
		if strings.Contains(strings.ToLower(n.Title), needle) {
			matches = append(matches, n.ID)
		}
	}
	if len(matches) == 0 {
		return nil, fmt.Errorf("no node title contains %q", needle)
	}
	if len(matches) > 1 {
		titles := make([]string, len(matches))
		for i, id := range matches {
			n, _ := store.GetNode(id)
			if n != nil {
				titles[i] = fmt.Sprintf("%04d %s", id, n.Title)
			} else {
				titles[i] = fmt.Sprintf("%04d", id)
			}
		}
		return nil, fmt.Errorf("ambiguous: %q matches %d nodes: %s", needle, len(matches), strings.Join(titles, ", "))
	}
	return matches, nil
}

// parseRelationType maps a CLI string to a retree.RelationType.
func parseRelationType(s string) (retree.RelationType, error) {
	s = strings.TrimSpace(s)
	switch s {
	case "depends_on":
		return retree.RelDependsOn, nil
	case "compares_against":
		return retree.RelComparesAgainst, nil
	case "inspired_by":
		return retree.RelInspiredBy, nil
	case "aggregates":
		return retree.RelAggregates, nil
	default:
		return "", fmt.Errorf("unknown relation type %q (valid: depends_on, compares_against, inspired_by, aggregates)", s)
	}
}

// resolveRelations parses a comma-separated "type:target" specification.
// Each token has the form "compares_against:42" or "inspired_by:My Node Title".
func resolveRelations(store *retree.Store, csv string) ([]retree.Relation, error) {
	csv = strings.TrimSpace(csv)
	if csv == "" {
		return nil, nil
	}
	parts := strings.Split(csv, ",")
	out := make([]retree.Relation, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		colon := strings.IndexByte(part, ':')
		if colon < 1 {
			return nil, fmt.Errorf("invalid relation spec %q (expected type:target)", part)
		}
		relType, err := parseRelationType(part[:colon])
		if err != nil {
			return nil, err
		}
		targetSpec := strings.TrimSpace(part[colon+1:])
		if targetSpec == "" {
			return nil, fmt.Errorf("invalid relation spec %q: target required", part)
		}
		target, err := resolveParents(store, targetSpec)
		if err != nil {
			return nil, fmt.Errorf("relation target %q: %w", targetSpec, err)
		}
		if len(target) != 1 {
			return nil, fmt.Errorf("relation spec %q resolved to %d targets (expected 1)", targetSpec, len(target))
		}
		out = append(out, retree.Relation{Type: relType, Target: target[0]})
	}
	return out, nil
}

// parseCSVStrings parses a comma-separated string into sorted unique strings.
func parseCSVStrings(s string) []string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	sort.Strings(out)
	return out
}

// uniqueNodeIDs returns a sorted slice without zero values or duplicates.
func uniqueNodeIDs(in []retree.NodeID) []retree.NodeID {
	set := make(map[retree.NodeID]struct{}, len(in))
	out := make([]retree.NodeID, 0, len(in))
	for _, id := range in {
		if id == 0 {
			continue
		}
		if _, ok := set[id]; ok {
			continue
		}
		set[id] = struct{}{}
		out = append(out, id)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

// parseOptionalBool parses loose boolean strings and returns nil when unset.
func parseOptionalBool(raw string) *bool {
	raw = strings.TrimSpace(strings.ToLower(raw))
	if raw == "" {
		return nil
	}
	v := raw == "true" || raw == "1" || raw == "yes"
	if raw == "false" || raw == "0" || raw == "no" {
		return &v
	}
	return &v
}

// readBody reads the contents of a markdown body file.
func readBody(bodyFile string) (string, error) {
	if strings.TrimSpace(bodyFile) == "" {
		return "", nil
	}
	b, err := os.ReadFile(bodyFile)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// resolveBody returns the body content from inline text, a file, or an
// editor session (--edit flag). Priority: --edit > --body > --body-file.
func resolveBody(bodyInline, bodyFile string, useEditor bool) (string, error) {
	if useEditor {
		return editBody(bodyInline)
	}
	if bodyInline != "" {
		return bodyInline, nil
	}
	return readBody(bodyFile)
}

// editBody opens $EDITOR with the given initial content and returns the
// final text. Falls back to vi, then nano.
func editBody(initial string) (string, error) {
	tmp, err := os.CreateTemp("", "rt-body-*.md")
	if err != nil {
		return "", err
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	if initial != "" {
		if _, err := tmp.WriteString(initial); err != nil {
			return "", err
		}
	}
	_ = tmp.Close()

	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = os.Getenv("VISUAL")
	}
	if editor == "" {
		if _, err := exec.LookPath("vi"); err == nil {
			editor = "vi"
		} else if _, err := exec.LookPath("nano"); err == nil {
			editor = "nano"
		} else {
			return "", fmt.Errorf("no editor found: set $EDITOR or install vi/nano")
		}
	}

	cmd := exec.Command(editor, tmp.Name())
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("editor failed: %w", err)
	}

	b, err := os.ReadFile(tmp.Name())
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// RunMeta is the parsed structured metadata stored by `rt node logrun`.
type RunMeta struct {
	Timestamp     string `json:"timestamp,omitempty"`
	ResourceID    string `json:"resource_id,omitempty"`
	Endpoint      string `json:"endpoint,omitempty"`
	EndpointKind  string `json:"endpoint_kind,omitempty"`
	Host          string `json:"host,omitempty"`
	Command       string `json:"command,omitempty"`
	OutDir        string `json:"outdir,omitempty"`
	Seed          string `json:"seed,omitempty"`
	ETA           string `json:"eta,omitempty"`
	Cost          string `json:"cost,omitempty"`
	Note          string `json:"note,omitempty"`
	Valid         *bool  `json:"valid,omitempty"`
	InvalidReason string `json:"invalid_reason,omitempty"`
}

// latestRunMeta returns the latest structured run, falling back to markdown parsing.
func latestRunMeta(n *retree.Node) (*RunMeta, bool) {
	if n != nil && len(n.Runs) > 0 {
		last := n.Runs[len(n.Runs)-1]
		return &RunMeta{
			Timestamp:     last.Timestamp.UTC().Format("2006-01-02 15:04:05 MST"),
			ResourceID:    last.ResourceID,
			Endpoint:      last.Endpoint,
			EndpointKind:  string(last.EndpointKind),
			Host:          last.Host,
			Command:       last.Command,
			OutDir:        last.OutDir,
			Seed:          last.Seed,
			ETA:           last.ETA,
			Cost:          last.Cost,
			Note:          last.Note,
			Valid:         last.Valid,
			InvalidReason: last.InvalidReason,
		}, true
	}
	if n == nil {
		return nil, false
	}
	return parseLatestRunMetaBody(n.Body)
}

// parseLatestRunMetaBody extracts the last markdown run-meta block from a node body.
func parseLatestRunMetaBody(body string) (*RunMeta, bool) {
	const marker = "\n### run-meta "
	idx := strings.LastIndex(body, marker)
	if idx == -1 {
		if strings.HasPrefix(body, "### run-meta ") {
			idx = 0
		} else {
			return nil, false
		}
	} else {
		idx++
	}
	block := body[idx:]
	lines := strings.Split(block, "\n")
	if len(lines) == 0 || !strings.HasPrefix(lines[0], "### run-meta ") {
		return nil, false
	}
	meta := &RunMeta{Timestamp: strings.TrimSpace(strings.TrimPrefix(lines[0], "### run-meta "))}
	inCode := false
	for _, line := range lines[1:] {
		trimmed := strings.TrimSpace(line)
		if trimmed == "```yaml" {
			inCode = true
			continue
		}
		if !inCode {
			continue
		}
		if trimmed == "```" {
			break
		}
		key, value, ok := strings.Cut(trimmed, ":")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if unquoted, err := strconv.Unquote(value); err == nil {
			value = unquoted
		}
		switch key {
		case "resource_id":
			meta.ResourceID = value
		case "endpoint":
			meta.Endpoint = value
		case "endpoint_kind":
			meta.EndpointKind = value
		case "host":
			meta.Host = value
		case "legacy_host":
			meta.Host = value
		case "cmd":
			meta.Command = value
		case "outdir":
			meta.OutDir = value
		case "seed":
			meta.Seed = value
		case "eta":
			meta.ETA = value
		case "cost":
			meta.Cost = value
		case "note":
			meta.Note = value
		case "invalid_reason":
			meta.InvalidReason = value
		case "valid":
			v := strings.EqualFold(value, "true")
			meta.Valid = &v
		}
	}
	return meta, true
}

// verdictBadge returns a compact semantic badge for the node's visible verdict.
func verdictBadge(n *retree.Node) string {
	switch n.ClaimStatus {
	case retree.ClaimSuperseded:
		return "[superseded]"
	case retree.ClaimInvalidated:
		return "[invalidated]"
	}
	if n.Status == retree.StatusDone {
		switch n.Outcome {
		case retree.OutcomeFailure:
			return "[failure]"
		case retree.OutcomeInconclusive:
			return "[inconclusive]"
		}
	}
	return ""
}

// goldenBadge returns a ★ prefix for golden milestone nodes.
func goldenBadge(n *retree.Node) string {
	if n != nil && n.MilestoneClass == retree.MilestoneGolden {
		return "★"
	}
	return ""
}

// titleWithVerdict returns a display title enriched with the visible verdict badge.
func titleWithVerdict(n *retree.Node) string {
	parts := make([]string, 0, 3)
	if badge := goldenBadge(n); badge != "" {
		parts = append(parts, badge)
	}
	parts = append(parts, n.Title)
	if badge := verdictBadge(n); badge != "" {
		parts = append(parts, badge)
	}
	return strings.Join(parts, " ")
}

// summaryLine returns a compact human summary extracted from the body.
func summaryLine(body string) string {
	lines := strings.Split(body, "\n")
	inCode := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if strings.HasPrefix(trimmed, "### run-meta ") {
			continue
		}
		if trimmed == "```yaml" || trimmed == "```" {
			inCode = !inCode
			continue
		}
		if inCode {
			continue
		}
		return trimmed
	}
	return ""
}

// formatNodeHuman formats a node for human-readable display.
func formatNodeHuman(cc *colorizer, n *retree.Node, children []retree.NodeID, leases []retree.ResourceLease, full bool) string {
	var b strings.Builder
	title := titleWithVerdict(n)
	if cc != nil {
		title = cc.golden(n.MilestoneClass, title)
	}
	b.WriteString(fmt.Sprintf("%04d %s\n", n.ID, title))
	b.WriteString(fmt.Sprintf("  status: %s  claim: %s  agent: %s\n", n.Status, n.ClaimStatus, n.Agent))
	if n.MilestoneClass == retree.MilestoneGolden {
		line := "  milestone: golden"
		if n.MilestoneKind != "" {
			line += fmt.Sprintf(" / %s", n.MilestoneKind)
		}
		if n.MilestoneReason != "" {
			line += fmt.Sprintf(" — %s", n.MilestoneReason)
		}
		if cc != nil {
			line = cc.golden(n.MilestoneClass, line)
		}
		b.WriteString(line + "\n")
	}
	if n.Scope != "" {
		b.WriteString(fmt.Sprintf("  scope: %s\n", n.Scope))
	}
	if n.ExitCriteria != "" {
		b.WriteString(fmt.Sprintf("  exit criteria: %s\n", n.ExitCriteria))
	}
	if len(n.Tags) > 0 {
		b.WriteString(fmt.Sprintf("  tags:   %s\n", strings.Join(n.Tags, ", ")))
	}
	if len(n.Parents) > 0 {
		ids := make([]string, len(n.Parents))
		for i, p := range n.Parents {
			ids[i] = fmt.Sprintf("%04d", p)
		}
		b.WriteString(fmt.Sprintf("  parents:  %s\n", strings.Join(ids, ", ")))
	}
	if len(children) > 0 {
		ids := make([]string, len(children))
		for i, c := range children {
			ids[i] = fmt.Sprintf("%04d", c)
		}
		b.WriteString(fmt.Sprintf("  children: %s\n", strings.Join(ids, ", ")))
	}
	if len(n.ContinuedBy) > 0 {
		ids := make([]string, len(n.ContinuedBy))
		for i, c := range n.ContinuedBy {
			ids[i] = fmt.Sprintf("%04d", c)
		}
		b.WriteString(fmt.Sprintf("  continued by: %s\n", strings.Join(ids, ", ")))
	}
	if len(n.SupersededBy) > 0 {
		ids := make([]string, len(n.SupersededBy))
		for i, c := range n.SupersededBy {
			ids[i] = fmt.Sprintf("%04d", c)
		}
		b.WriteString(fmt.Sprintf("  superseded by: %s\n", strings.Join(ids, ", ")))
	}
	if n.PrimaryParent != nil {
		b.WriteString(fmt.Sprintf("  primary parent: %04d\n", *n.PrimaryParent))
	}
	if len(n.Relations) > 0 {
		lines := make([]string, len(n.Relations))
		for i, rel := range n.Relations {
			note := ""
			if rel.Note != "" {
				note = fmt.Sprintf(" (%s)", rel.Note)
			}
			lines[i] = fmt.Sprintf("%s:%04d%s", rel.Type, rel.Target, note)
		}
		b.WriteString(fmt.Sprintf("  relations: %s\n", strings.Join(lines, ", ")))
	}
	if len(leases) > 0 {
		b.WriteString("  resources:\n")
		for _, lease := range leases {
			line := fmt.Sprintf("    - %s [%s]", lease.ResourceID, lease.Mode)
			if lease.ClaimedBy != "" {
				line += fmt.Sprintf(" by %s", lease.ClaimedBy)
			}
			if lease.Note != "" {
				line += fmt.Sprintf(" — %s", lease.Note)
			}
			b.WriteString(line + "\n")
		}
	}
	if len(n.Commits) > 0 {
		b.WriteString(fmt.Sprintf("  commits: %d\n", len(n.Commits)))
	}
	if len(n.Artifacts) > 0 {
		b.WriteString(fmt.Sprintf("  artifacts: %d\n", len(n.Artifacts)))
		if full {
			for i, a := range n.Artifacts {
				mode := string(a.Mode)
				loc := a.Path
				if a.Mode == retree.ArtifactPath {
					loc = fmt.Sprintf("%s:%s", a.Host, a.Path)
				}
				detail := fmt.Sprintf("    %d. [%s] %s", i+1, mode, loc)
				if a.Description != "" {
					detail += fmt.Sprintf("  — %s", a.Description)
				}
				if a.SizeBytes > 0 {
					detail += fmt.Sprintf("  (%s)", formatBytes(a.SizeBytes))
				}
				b.WriteString(detail + "\n")
			}
		} else {
			b.WriteString("    use --view full for artifact details\n")
		}
	}
	if n.InvalidationReason != "" {
		b.WriteString(fmt.Sprintf("  invalidated by: %v — %s\n", n.InvalidatedBy, n.InvalidationReason))
	}
	if latestRun, ok := latestRunMeta(n); ok {
		b.WriteString("  latest run:\n")
		if latestRun.Timestamp != "" {
			b.WriteString(fmt.Sprintf("    timestamp: %s\n", latestRun.Timestamp))
		}
		if latestRun.ResourceID != "" {
			b.WriteString(fmt.Sprintf("    resource:  %s\n", latestRun.ResourceID))
		}
		if latestRun.Endpoint != "" {
			b.WriteString(fmt.Sprintf("    endpoint:  %s (%s)\n", latestRun.Endpoint, latestRun.EndpointKind))
		}
		if latestRun.Host != "" {
			b.WriteString(fmt.Sprintf("    legacy_host: %s\n", latestRun.Host))
		}
		if latestRun.Command != "" {
			b.WriteString(fmt.Sprintf("    cmd:       %s\n", latestRun.Command))
		}
		if latestRun.OutDir != "" {
			b.WriteString(fmt.Sprintf("    outdir:    %s\n", latestRun.OutDir))
		}
		if latestRun.Seed != "" {
			b.WriteString(fmt.Sprintf("    seed:      %s\n", latestRun.Seed))
		}
		if latestRun.ETA != "" {
			b.WriteString(fmt.Sprintf("    eta:       %s\n", latestRun.ETA))
		}
		if latestRun.Cost != "" {
			b.WriteString(fmt.Sprintf("    cost:      %s\n", latestRun.Cost))
		}
		if latestRun.Note != "" {
			b.WriteString(fmt.Sprintf("    note:      %s\n", latestRun.Note))
		}
		if latestRun.Valid != nil {
			b.WriteString(fmt.Sprintf("    valid:     %t\n", *latestRun.Valid))
		}
		if latestRun.InvalidReason != "" {
			b.WriteString(fmt.Sprintf("    invalid:   %s\n", latestRun.InvalidReason))
		}
	}
	b.WriteString(fmt.Sprintf("  revision: %d\n", n.Revision))
	b.WriteString(fmt.Sprintf("  created: %s  modified: %s\n", n.Created.Format("2006-01-02 15:04"), n.Modified.Format("2006-01-02 15:04")))
	if full && n.Body != "" {
		b.WriteString("\n─── body ───\n")
		b.WriteString(n.Body)
		if !strings.HasSuffix(n.Body, "\n") {
			b.WriteString("\n")
		}
		b.WriteString("────────────")
	} else if !full && n.Body != "" {
		b.WriteString("  body: present (use --view full)\n")
	}
	return b.String()
}

// formatNodeAgentView renders a compact handoff-oriented node view for agents.
func formatNodeAgentView(cc *colorizer, n *retree.Node, children []retree.NodeID, leases []retree.ResourceLease) string {
	var b strings.Builder
	title := titleWithVerdict(n)
	if cc != nil {
		title = cc.golden(n.MilestoneClass, title)
	}
	b.WriteString(fmt.Sprintf("%04d %s\n", n.ID, title))
	b.WriteString(fmt.Sprintf("  status: %s  outcome: %s  claim: %s\n", n.Status, n.Outcome, n.ClaimStatus))
	if n.MilestoneClass == retree.MilestoneGolden {
		line := "  milestone: golden"
		if n.MilestoneKind != "" {
			line += fmt.Sprintf(" / %s", n.MilestoneKind)
		}
		if n.MilestoneReason != "" {
			line += fmt.Sprintf(" — %s", n.MilestoneReason)
		}
		if cc != nil {
			line = cc.golden(n.MilestoneClass, line)
		}
		b.WriteString(line + "\n")
	}
	if n.Agent != "" {
		b.WriteString(fmt.Sprintf("  agent: %s\n", n.Agent))
	}
	if len(n.Tags) > 0 {
		b.WriteString(fmt.Sprintf("  tags: %s\n", strings.Join(n.Tags, ", ")))
	}
	if n.Scope != "" {
		b.WriteString(fmt.Sprintf("  scope: %s\n", n.Scope))
	}
	if n.ExitCriteria != "" {
		b.WriteString(fmt.Sprintf("  exit_criteria: %s\n", n.ExitCriteria))
	}
	if len(n.Parents) > 0 {
		ids := make([]string, len(n.Parents))
		for i, p := range n.Parents {
			ids[i] = fmt.Sprintf("%04d", p)
		}
		b.WriteString(fmt.Sprintf("  parents: %s\n", strings.Join(ids, ", ")))
	}
	if len(children) > 0 {
		ids := make([]string, len(children))
		for i, c := range children {
			ids[i] = fmt.Sprintf("%04d", c)
		}
		b.WriteString(fmt.Sprintf("  children: %s\n", strings.Join(ids, ", ")))
	}
	if len(n.ContinuedBy) > 0 {
		ids := make([]string, len(n.ContinuedBy))
		for i, c := range n.ContinuedBy {
			ids[i] = fmt.Sprintf("%04d", c)
		}
		b.WriteString(fmt.Sprintf("  continued_by: %s\n", strings.Join(ids, ", ")))
	}
	if len(n.SupersededBy) > 0 {
		ids := make([]string, len(n.SupersededBy))
		for i, c := range n.SupersededBy {
			ids[i] = fmt.Sprintf("%04d", c)
		}
		b.WriteString(fmt.Sprintf("  superseded_by: %s\n", strings.Join(ids, ", ")))
	}
	if len(leases) > 0 {
		items := make([]string, 0, len(leases))
		for _, lease := range leases {
			items = append(items, fmt.Sprintf("%s[%s]", lease.ResourceID, lease.Mode))
		}
		b.WriteString(fmt.Sprintf("  resources: %s\n", strings.Join(items, ", ")))
	}
	if latestRun, ok := latestRunMeta(n); ok {
		b.WriteString("  latest_run:\n")
		if latestRun.ResourceID != "" {
			b.WriteString(fmt.Sprintf("    resource_id: %s\n", latestRun.ResourceID))
		}
		if latestRun.Endpoint != "" {
			b.WriteString(fmt.Sprintf("    endpoint: %s\n", latestRun.Endpoint))
		}
		if latestRun.EndpointKind != "" {
			b.WriteString(fmt.Sprintf("    endpoint_kind: %s\n", latestRun.EndpointKind))
		}
		if latestRun.Host != "" {
			b.WriteString(fmt.Sprintf("    legacy_host: %s\n", latestRun.Host))
		}
		if latestRun.Command != "" {
			b.WriteString(fmt.Sprintf("    cmd: %s\n", latestRun.Command))
		}
		if latestRun.OutDir != "" {
			b.WriteString(fmt.Sprintf("    outdir: %s\n", latestRun.OutDir))
		}
		if latestRun.Seed != "" {
			b.WriteString(fmt.Sprintf("    seed: %s\n", latestRun.Seed))
		}
		if latestRun.Note != "" {
			b.WriteString(fmt.Sprintf("    note: %s\n", latestRun.Note))
		}
		if latestRun.Valid != nil {
			b.WriteString(fmt.Sprintf("    valid: %t\n", *latestRun.Valid))
		}
		if latestRun.InvalidReason != "" {
			b.WriteString(fmt.Sprintf("    invalid_reason: %s\n", latestRun.InvalidReason))
		}
	}
	body := strings.TrimSpace(n.Body)
	if body != "" {
		firstLine := summaryLine(body)
		if firstLine == "" {
			firstLine = strings.SplitN(body, "\n", 2)[0]
		}
		if len(firstLine) > 140 {
			firstLine = firstLine[:137] + "..."
		}
		b.WriteString(fmt.Sprintf("  summary: %s\n", firstLine))
	}
	if len(n.Artifacts) > 0 {
		b.WriteString(fmt.Sprintf("  artifacts: %d\n", len(n.Artifacts)))
	}
	if len(n.Commits) > 0 {
		b.WriteString(fmt.Sprintf("  commits: %d\n", len(n.Commits)))
	}
	return strings.TrimRight(b.String(), "\n")
}

// formatBytes returns a human-readable byte size string.
func formatBytes(n int64) string {
	if n < 1024 {
		return fmt.Sprintf("%dB", n)
	}
	if n < 1024*1024 {
		return fmt.Sprintf("%.1fKB", float64(n)/1024)
	}
	if n < 1024*1024*1024 {
		return fmt.Sprintf("%.1fMB", float64(n)/(1024*1024))
	}
	return fmt.Sprintf("%.1fGB", float64(n)/(1024*1024*1024))
}

// parseNodeStatus converts a string to NodeStatus.
func parseNodeStatus(s string) retree.NodeStatus { return retree.NodeStatus(strings.TrimSpace(s)) }

// parseClaimStatus converts a string to ClaimStatus.
func parseClaimStatus(s string) retree.ClaimStatus {
	return retree.ClaimStatus(strings.TrimSpace(s))
}

// colorizer applies ANSI colors when enabled.
type colorizer struct {
	enabled bool
}

// newColorizer creates a colorizer that applies ANSI escape codes when enabled.
func newColorizer(mode string) *colorizer {
	switch mode {
	case "always":
		return &colorizer{enabled: true}
	case "never":
		return &colorizer{enabled: false}
	default: // auto — check if stdout is a terminal
		fi, err := os.Stdout.Stat()
		return &colorizer{enabled: err == nil && (fi.Mode()&os.ModeCharDevice) != 0}
	}
}

// status applies the ANSI color for the given node status to the text.
func (c *colorizer) status(s retree.NodeStatus, text string) string {
	if !c.enabled {
		return text
	}
	switch s {
	case retree.StatusActive:
		return "\033[32m" + text + "\033[0m" // green
	case retree.StatusPaused:
		return "\033[33m" + text + "\033[0m" // yellow
	default:
		return text
	}
}

// outcomeColor applies the ANSI color for the given outcome.
func (c *colorizer) outcomeColor(o retree.Outcome, text string) string {
	if !c.enabled {
		return text
	}
	switch o {
	case retree.OutcomeSuccess:
		return "\033[36m" + text + "\033[0m" // cyan
	case retree.OutcomeFailure:
		return "\033[31m" + text + "\033[0m" // red
	case retree.OutcomeInconclusive:
		return "\033[35m" + text + "\033[0m" // magenta
	default:
		return text
	}
}

// golden applies a gold ANSI color to frontier-significant nodes.
func (c *colorizer) golden(class retree.MilestoneClass, text string) string {
	if !c.enabled || class != retree.MilestoneGolden {
		return text
	}
	return "\033[38;5;220m" + text + "\033[0m"
}

// parseOutcome converts a string to Outcome.
func parseOutcome(s string) retree.Outcome { return retree.Outcome(strings.TrimSpace(s)) }

// validateTerminalOutcome enforces stricter closure discipline:
// done nodes must declare a non-unset outcome.
func validateTerminalOutcome(status retree.NodeStatus, outcome retree.Outcome) error {
	if status != retree.StatusDone {
		return nil
	}
	if strings.TrimSpace(string(outcome)) == "" || outcome == retree.OutcomeUnset {
		return fmt.Errorf("status=done requires --outcome {success|failure|inconclusive}")
	}
	return nil
}
