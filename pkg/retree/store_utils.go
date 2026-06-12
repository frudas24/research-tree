package retree

import (
	"sort"
	"strings"
	"time"
)

// uniqueStrings returns a sorted, deduplicated copy of the input slice.
func uniqueStrings(in []string) []string {
	m := make(map[string]struct{}, len(in))
	for _, s := range in {
		if s == "" {
			continue
		}
		m[s] = struct{}{}
	}
	out := make([]string, 0, len(m))
	for s := range m {
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}

// removeString returns a copy of the slice with all occurrences of needle removed.
func removeString(in []string, needle string) []string {
	out := make([]string, 0, len(in))
	for _, s := range in {
		if s != needle {
			out = append(out, s)
		}
	}
	return out
}

// matchesFilter reports whether a node matches the given filter criteria.
func matchesFilter(n *Node, f Filter) bool {
	if f.Status != "" && n.Status != f.Status {
		return false
	}
	if f.ClaimStatus != "" && n.ClaimStatus != f.ClaimStatus {
		return false
	}
	if f.Outcome != "" && n.Outcome != f.Outcome {
		return false
	}
	if f.Tag != "" {
		found := false
		for _, tag := range n.Tags {
			if tag == f.Tag {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	if len(f.TagsAll) > 0 {
		for _, needle := range uniqueStrings(f.TagsAll) {
			found := false
			for _, tag := range n.Tags {
				if tag == needle {
					found = true
					break
				}
			}
			if !found {
				return false
			}
		}
	}
	if len(f.TagsAny) > 0 {
		foundAny := false
		for _, needle := range uniqueStrings(f.TagsAny) {
			for _, tag := range n.Tags {
				if tag == needle {
					foundAny = true
					break
				}
			}
			if foundAny {
				break
			}
		}
		if !foundAny {
			return false
		}
	}
	if f.Agent != "" && n.Agent != f.Agent {
		return false
	}
	if f.TitleContains != "" && !strings.Contains(strings.ToLower(n.Title), strings.ToLower(f.TitleContains)) {
		return false
	}
	if f.ScopeContains != "" && !strings.Contains(strings.ToLower(n.Scope), strings.ToLower(f.ScopeContains)) {
		return false
	}
	if f.BodyContains != "" && !strings.Contains(strings.ToLower(n.Body), strings.ToLower(f.BodyContains)) {
		return false
	}
	if f.ContinuedBy != 0 && !containsNodeID(n.ContinuedBy, f.ContinuedBy) {
		return false
	}
	if f.SupersededBy != 0 && !containsNodeID(n.SupersededBy, f.SupersededBy) {
		return false
	}
	if !f.CreatedAfter.IsZero() && n.Created.Before(f.CreatedAfter) {
		return false
	}
	if !f.CreatedBefore.IsZero() && n.Created.After(f.CreatedBefore) {
		return false
	}
	if f.HasArtifact != nil && (len(n.Artifacts) > 0) != *f.HasArtifact {
		return false
	}
	if f.MilestoneClass != "" && n.MilestoneClass != f.MilestoneClass {
		return false
	}
	if f.MilestoneKind != "" && n.MilestoneKind != f.MilestoneKind {
		return false
	}
	return true
}

// containsNodeID reports whether ids contains target.
func containsNodeID(ids []NodeID, target NodeID) bool {
	for _, id := range ids {
		if id == target {
			return true
		}
	}
	return false
}

// sortNodes applies deterministic ordering for query results.
func sortNodes(nodes []*Node, sortBy string, order string) {
	key := strings.ToLower(strings.TrimSpace(sortBy))
	if key == "" {
		key = "id"
	}
	desc := strings.EqualFold(strings.TrimSpace(order), "desc")

	sort.SliceStable(nodes, func(i, j int) bool {
		less := compareNodes(nodes[i], nodes[j], key)
		greater := compareNodes(nodes[j], nodes[i], key)
		if !less && !greater {
			return false
		}
		if desc {
			return greater
		}
		return less
	})
}

// compareNodes compares two nodes for the requested sort key.
func compareNodes(a, b *Node, key string) bool {
	switch key {
	case "created":
		return compareTimes(a.Created, b.Created, a.ID, b.ID)
	case "modified":
		return compareTimes(a.Modified, b.Modified, a.ID, b.ID)
	case "title":
		at, bt := strings.ToLower(a.Title), strings.ToLower(b.Title)
		if at == bt {
			return a.ID < b.ID
		}
		return at < bt
	case "id":
		fallthrough
	default:
		return a.ID < b.ID
	}
}

// compareTimes compares timestamps with node ID as deterministic tie-breaker.
func compareTimes(a, b time.Time, aID, bID NodeID) bool {
	if a.Equal(b) {
		return aID < bID
	}
	return a.Before(b)
}
