package retree

import (
	"fmt"
	"os"
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
	mustNoErr(t, s.LinkNodeToFeature(f.ID, child.ID, RoleFix))

	got, err := s.GetFeature(f.ID)
	mustNoErr(t, err)
	if got.CurrentNode != child.ID {
		t.Fatalf("expected current_node %d, got %d", child.ID, got.CurrentNode)
	}
	if got.CurrentNodeMode != "explicit" {
		t.Fatalf("expected explicit mode, got %s", got.CurrentNodeMode)
	}
}

// TestDependsOnPropagatesDegradedTransitively verifies degraded health travels across dependency chains.
func TestDependsOnPropagatesDegradedTransitively(t *testing.T) {
	s := mustInit(t, StorageJSON)
	root := &Node{Frontmatter: Frontmatter{Title: "root"}}
	bad := poisonedNode("bad impl")
	mustNoErr(t, s.CreateNode(root))
	mustNoErr(t, s.CreateNode(bad))

	fC, err := s.CreateFeature("Dep C", root.ID)
	mustNoErr(t, err)
	mustNoErr(t, s.LinkNodeToFeature(fC.ID, bad.ID, RoleImplementation))
	fB, err := s.CreateFeature("Dep B", root.ID)
	mustNoErr(t, err)
	mustNoErr(t, s.LinkNodeToFeature(fB.ID, root.ID, RoleImplementation))
	fA, err := s.CreateFeature("Dep A", root.ID)
	mustNoErr(t, err)
	mustNoErr(t, s.LinkNodeToFeature(fA.ID, root.ID, RoleImplementation))
	mustNoErr(t, s.RelateFeatures(fB.ID, fC.ID, EdgeDependsOn, root.ID))
	mustNoErr(t, s.RelateFeatures(fA.ID, fB.ID, EdgeDependsOn, root.ID))

	report, err := s.ComputeFeatureHealth(fA.ID)
	mustNoErr(t, err)
	if report.Health != HealthDegraded {
		t.Fatalf("expected transitive degraded via depends_on, got %s", report.Health)
	}
}

// TestCollaboratesWithDoesNotPropagateInfiniteWarnings verifies collaboration warnings stay direct.
func TestCollaboratesWithDoesNotPropagateInfiniteWarnings(t *testing.T) {
	s := mustInit(t, StorageJSON)
	root := &Node{Frontmatter: Frontmatter{Title: "root"}}
	bad := poisonedNode("bad impl")
	mustNoErr(t, s.CreateNode(root))
	mustNoErr(t, s.CreateNode(bad))

	fC, err := s.CreateFeature("C", root.ID)
	mustNoErr(t, err)
	mustNoErr(t, s.LinkNodeToFeature(fC.ID, bad.ID, RoleImplementation))
	fB, err := s.CreateFeature("B", root.ID)
	mustNoErr(t, err)
	mustNoErr(t, s.LinkNodeToFeature(fB.ID, root.ID, RoleImplementation))
	fA, err := s.CreateFeature("A", root.ID)
	mustNoErr(t, err)
	mustNoErr(t, s.LinkNodeToFeature(fA.ID, root.ID, RoleImplementation))
	mustNoErr(t, s.RelateFeatures(fB.ID, fC.ID, EdgeCollaboratesWith, root.ID))
	mustNoErr(t, s.RelateFeatures(fA.ID, fB.ID, EdgeCollaboratesWith, root.ID))

	report, err := s.ComputeFeatureHealth(fA.ID)
	mustNoErr(t, err)
	if report.Health != HealthClean {
		t.Fatalf("expected collaboration warning to stay direct, got %s", report.Health)
	}
}

// TestComputeFeatureHealthDetectsCycleDefensively verifies corrupt edge cycles fail loudly.
func TestComputeFeatureHealthDetectsCycleDefensively(t *testing.T) {
	s := mustInit(t, StorageJSON)
	root := &Node{Frontmatter: Frontmatter{Title: "root"}}
	mustNoErr(t, s.CreateNode(root))

	f1, err := s.CreateFeature("A", root.ID)
	mustNoErr(t, err)
	f2, err := s.CreateFeature("B", root.ID)
	mustNoErr(t, err)

	corrupt := fmt.Sprintf("{\"from\":\"%s\",\"to\":\"%s\",\"type\":\"depends_on\",\"created_from\":%d}\n{\"from\":\"%s\",\"to\":\"%s\",\"type\":\"depends_on\",\"created_from\":%d}\n",
		f1.ID, f2.ID, root.ID, f2.ID, f1.ID, root.ID)
	mustNoErr(t, os.WriteFile(s.featureEdgesPath(), []byte(corrupt), 0o644))

	_, err = s.ComputeFeatureHealth(f1.ID)
	if err == nil || !strings.Contains(err.Error(), "cycle detected") {
		t.Fatalf("expected defensive cycle error, got %v", err)
	}
}

// TestComputeFeatureHealthReturnsErrorOnCorruptEdges verifies malformed edge storage fails loudly.
func TestComputeFeatureHealthReturnsErrorOnCorruptEdges(t *testing.T) {
	s := mustInit(t, StorageJSON)
	root := &Node{Frontmatter: Frontmatter{Title: "root"}}
	mustNoErr(t, s.CreateNode(root))

	f, err := s.CreateFeature("A", root.ID)
	mustNoErr(t, err)
	mustNoErr(t, os.WriteFile(s.featureEdgesPath(), []byte("{not-json}\n"), 0o644))

	_, err = s.ComputeFeatureHealth(f.ID)
	if err == nil || !strings.Contains(err.Error(), "read feature edges") {
		t.Fatalf("expected corrupt edges error, got %v", err)
	}
}
