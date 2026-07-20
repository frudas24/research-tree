package retree

import (
	"strings"
	"testing"
)

func poisonedNode(title string) *Node {
	n := &Node{Frontmatter: Frontmatter{
		Title:          title,
		EvidenceStatus: EvidencePoisoned,
		EvidenceCause:  EvidenceCauseToolchain,
	}}
	n.PoisonReason = "test poison"
	return n
}

// TestDerivedHealthClean verifies health stays clean when no issues exist.
func TestDerivedHealthClean(t *testing.T) {
	s := mustInit(t, StorageJSON)
	root := &Node{Frontmatter: Frontmatter{Title: "root"}}
	mustNoErr(t, s.CreateNode(root))

	f, err := s.CreateFeature("Clean Feature", root.ID)
	mustNoErr(t, err)
	mustNoErr(t, s.LinkNodeToFeature(f.ID, root.ID, RoleImplementation))

	report, err := s.ComputeFeatureHealth(f.ID)
	mustNoErr(t, err)
	if report.Health != HealthClean {
		t.Fatalf("expected clean, got %s", report.Health)
	}
}

// TestDerivedHealthDegradedFromPoisonedNode verifies poisoned implementation degrades the feature.
func TestDerivedHealthDegradedFromPoisonedNode(t *testing.T) {
	s := mustInit(t, StorageJSON)
	root := &Node{Frontmatter: Frontmatter{Title: "root"}}
	bad := poisonedNode("impl")
	mustNoErr(t, s.CreateNode(root))
	mustNoErr(t, s.CreateNode(bad))

	f, err := s.CreateFeature("Poisoned Impl", root.ID)
	mustNoErr(t, err)
	mustNoErr(t, s.LinkNodeToFeature(f.ID, bad.ID, RoleImplementation))

	report, err := s.ComputeFeatureHealth(f.ID)
	mustNoErr(t, err)
	if report.Health != HealthDegraded {
		t.Fatalf("expected degraded, got %s", report.Health)
	}
}

// TestDependsOnPropagatesDegraded verifies depends_on propagates degraded health.
func TestDependsOnPropagatesDegraded(t *testing.T) {
	s := mustInit(t, StorageJSON)
	root := &Node{Frontmatter: Frontmatter{Title: "root"}}
	bad := poisonedNode("bad impl")
	mustNoErr(t, s.CreateNode(root))
	mustNoErr(t, s.CreateNode(bad))

	fA, err := s.CreateFeature("Dep A", root.ID)
	mustNoErr(t, err)
	mustNoErr(t, s.LinkNodeToFeature(fA.ID, bad.ID, RoleImplementation))
	fB, err := s.CreateFeature("Dep B", root.ID)
	mustNoErr(t, err)
	mustNoErr(t, s.LinkNodeToFeature(fB.ID, root.ID, RoleImplementation))
	mustNoErr(t, s.RelateFeatures(fB.ID, fA.ID, EdgeDependsOn, root.ID))

	report, err := s.ComputeFeatureHealth(fB.ID)
	mustNoErr(t, err)
	if report.Health != HealthDegraded {
		t.Fatalf("expected degraded via depends_on, got %s", report.Health)
	}
}

// TestCollaboratesWithPropagatesWarning verifies collaborates_with propagates warning.
func TestCollaboratesWithPropagatesWarning(t *testing.T) {
	s := mustInit(t, StorageJSON)
	root := &Node{Frontmatter: Frontmatter{Title: "root"}}
	bad := poisonedNode("bad")
	mustNoErr(t, s.CreateNode(root))
	mustNoErr(t, s.CreateNode(bad))

	fA, err := s.CreateFeature("Degraded A", root.ID)
	mustNoErr(t, err)
	mustNoErr(t, s.LinkNodeToFeature(fA.ID, bad.ID, RoleImplementation))
	fB, err := s.CreateFeature("Clean B", root.ID)
	mustNoErr(t, err)
	mustNoErr(t, s.LinkNodeToFeature(fB.ID, root.ID, RoleImplementation))
	mustNoErr(t, s.RelateFeatures(fB.ID, fA.ID, EdgeCollaboratesWith, root.ID))

	report, err := s.ComputeFeatureHealth(fB.ID)
	mustNoErr(t, err)
	if report.Health != HealthWarning {
		t.Fatalf("expected warning via collaborates_with, got %s", report.Health)
	}
}

// TestSupersedesReportsRetirementCandidate verifies supersedes reports but doesn't auto-retire.
func TestSupersedesReportsRetirementCandidate(t *testing.T) {
	s := mustInit(t, StorageJSON)
	root := &Node{Frontmatter: Frontmatter{Title: "root"}}
	mustNoErr(t, s.CreateNode(root))

	fOld, err := s.CreateFeature("Old", root.ID)
	mustNoErr(t, err)
	mustNoErr(t, s.LinkNodeToFeature(fOld.ID, root.ID, RoleImplementation))
	fNew, err := s.CreateFeature("New", root.ID)
	mustNoErr(t, err)
	mustNoErr(t, s.LinkNodeToFeature(fNew.ID, root.ID, RoleImplementation))
	mustNoErr(t, s.RelateFeatures(fNew.ID, fOld.ID, EdgeSupersedes, root.ID))

	report, err := s.ComputeFeatureHealth(fOld.ID)
	mustNoErr(t, err)
	found := false
	for _, iss := range report.Issues {
		if strings.Contains(iss, "superseded by") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected superseded report, got issues: %v", report.Issues)
	}
	got, _ := s.GetFeature(fOld.ID)
	if got.Status != FeatureActive {
		t.Fatalf("expected status active (not auto-retired), got %s", got.Status)
	}
}

// TestUnmooredEdgeDetected verifies edges with missing created_from nodes are flagged.
func TestUnmooredEdgeDetected(t *testing.T) {
	s := mustInit(t, StorageJSON)
	root := &Node{Frontmatter: Frontmatter{Title: "root"}}
	mustNoErr(t, s.CreateNode(root))

	f1, err := s.CreateFeature("F1", root.ID)
	mustNoErr(t, err)
	mustNoErr(t, s.LinkNodeToFeature(f1.ID, root.ID, RoleImplementation))
	f2, err := s.CreateFeature("F2", root.ID)
	mustNoErr(t, err)
	mustNoErr(t, s.LinkNodeToFeature(f2.ID, root.ID, RoleImplementation))
	mustNoErr(t, s.RelateFeatures(f1.ID, f2.ID, EdgeDependsOn, root.ID))

	mustNoErr(t, s.DeleteNode(root.ID, true))

	report, err := s.ComputeFeatureHealth(f1.ID)
	mustNoErr(t, err)
	found := false
	for _, iss := range report.Issues {
		if strings.Contains(iss, "unmoored") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected unmoored issue, got: %v", report.Issues)
	}
}

// TestCurrentNodeExplicitOverridesDerived verifies explicit current_node doesn't change via derivation.
func TestCurrentNodeExplicitOverridesDerived(t *testing.T) {
	s := mustInit(t, StorageJSON)
	root := &Node{Frontmatter: Frontmatter{Title: "root"}}
	child := &Node{Frontmatter: Frontmatter{Title: "child"}}
	mustNoErr(t, s.CreateNode(root))
	mustNoErr(t, s.CreateNode(child))

	f, err := s.CreateFeature("Feature", root.ID)
	mustNoErr(t, err)
	mustNoErr(t, s.LinkNodeToFeature(f.ID, root.ID, RoleDecision))
	mustNoErr(t, s.SetFeatureCurrentNode(f.ID, child.ID))

	got, err := s.GetFeature(f.ID)
	mustNoErr(t, err)
	if got.CurrentNode != child.ID {
		t.Fatalf("expected current_node %d, got %d", child.ID, got.CurrentNode)
	}
	if got.CurrentNodeMode != "explicit" {
		t.Fatalf("expected explicit mode, got %s", got.CurrentNodeMode)
	}
}
