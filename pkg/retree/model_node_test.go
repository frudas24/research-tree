package retree

import (
	"errors"
	"testing"
	"time"
)

// TestApplyNodeDefaults verifies default values are applied correctly.
func TestApplyNodeDefaults(t *testing.T) {
	n := &Node{Frontmatter: Frontmatter{Title: "t"}}
	now := time.Date(2026, 5, 31, 10, 0, 0, 0, time.UTC)
	ApplyNodeDefaults(n, now)
	if n.SchemaVersion != CurrentSchemaVersion {
		t.Fatalf("schema default mismatch: %v", n.SchemaVersion)
	}
	if n.Status != StatusActive {
		t.Fatalf("status default mismatch: %v", n.Status)
	}
	if n.ClaimStatus != ClaimProvisional {
		t.Fatalf("claim default mismatch: %v", n.ClaimStatus)
	}
	if !n.Created.Equal(now) || !n.Modified.Equal(now) {
		t.Fatalf("time defaults mismatch: created=%v modified=%v", n.Created, n.Modified)
	}
}

// TestValidateNodeRejectsMissingTitle verifies title validation.
func TestValidateNodeRejectsMissingTitle(t *testing.T) {
	n := &Node{}
	ApplyNodeDefaults(n, time.Now())
	err := ValidateNode(n)
	if !errors.Is(err, ErrInvalidNode) {
		t.Fatalf("expected ErrInvalidNode, got %v", err)
	}
}

// TestValidateNodeInvalidStatus verifies invalid status rejection.
func TestValidateNodeInvalidStatus(t *testing.T) {
	n := &Node{Frontmatter: Frontmatter{Title: "x", Status: "weird"}}
	ApplyNodeDefaults(n, time.Now())
	n.Status = "weird"
	err := ValidateNode(n)
	if !errors.Is(err, ErrInvalidStatus) {
		t.Fatalf("expected ErrInvalidStatus, got %v", err)
	}
}

// TestValidateNodeInvalidClaimStatus verifies invalid claim status rejection.
func TestValidateNodeInvalidClaimStatus(t *testing.T) {
	n := &Node{Frontmatter: Frontmatter{Title: "x"}}
	ApplyNodeDefaults(n, time.Now())
	n.ClaimStatus = "wrong"
	err := ValidateNode(n)
	if !errors.Is(err, ErrInvalidClaimStatus) {
		t.Fatalf("expected ErrInvalidClaimStatus, got %v", err)
	}
}

// TestValidateNodeGoldenRequiresReason verifies golden milestones must include a reason.
func TestValidateNodeGoldenRequiresReason(t *testing.T) {
	n := &Node{Frontmatter: Frontmatter{Title: "golden", MilestoneClass: MilestoneGolden}}
	ApplyNodeDefaults(n, time.Now())
	err := ValidateNode(n)
	if !errors.Is(err, ErrInvalidNode) {
		t.Fatalf("expected ErrInvalidNode for golden without reason, got %v", err)
	}
}

// TestValidateNodeRejectsDanglingMilestoneKind verifies milestone_kind without class is rejected.
func TestValidateNodeRejectsDanglingMilestoneKind(t *testing.T) {
	n := &Node{Frontmatter: Frontmatter{Title: "dangling", MilestoneKind: MilestoneKindChampion}}
	ApplyNodeDefaults(n, time.Now())
	err := ValidateNode(n)
	if !errors.Is(err, ErrInvalidNode) {
		t.Fatalf("expected ErrInvalidNode for milestone_kind without class, got %v", err)
	}
}

// TestValidateNodeAcceptsGoldenMilestone verifies a valid golden milestone passes validation.
func TestValidateNodeAcceptsGoldenMilestone(t *testing.T) {
	n := &Node{Frontmatter: Frontmatter{
		Title:           "golden",
		MilestoneClass:  MilestoneGolden,
		MilestoneKind:   MilestoneKindBreakthrough,
		MilestoneReason: "moved the bottleneck from runtime to distillation",
	}}
	ApplyNodeDefaults(n, time.Now())
	if err := ValidateNode(n); err != nil {
		t.Fatalf("expected valid golden milestone, got %v", err)
	}
}

// TestValidateNodeInvalidatedRequiresInvalidatedBy verifies invalidated claim requires invalidated_by.
func TestValidateNodeInvalidatedRequiresInvalidatedBy(t *testing.T) {
	n := &Node{Frontmatter: Frontmatter{Title: "x"}}
	ApplyNodeDefaults(n, time.Now())
	n.ClaimStatus = ClaimInvalidated
	err := ValidateNode(n)
	if !errors.Is(err, ErrInvalidNode) {
		t.Fatalf("expected ErrInvalidNode, got %v", err)
	}
}

// TestValidateArtifactRules verifies artifact validation rules.
func TestValidateArtifactRules(t *testing.T) {
	if err := ValidateArtifact(Artifact{Mode: ArtifactPath, Path: "/tmp/x", Host: "h"}); err != nil {
		t.Fatalf("expected valid artifact, got %v", err)
	}
	if err := ValidateArtifact(Artifact{Mode: ArtifactPath, Path: "/tmp/x"}); !errors.Is(err, ErrInvalidArtifact) {
		t.Fatalf("expected ErrInvalidArtifact for missing host, got %v", err)
	}
	if err := ValidateArtifact(Artifact{Mode: ArtifactEmbedded, Path: "artifacts/1/a"}); err != nil {
		t.Fatalf("expected embedded artifact valid, got %v", err)
	}
}

// TestUnmarshalNodeJSONMinimumAndFull verifies minimal and full JSON unmarshal.
func TestUnmarshalNodeJSONMinimumAndFull(t *testing.T) {
	minimal := []byte(`{"title":"only title"}`)
	n, err := UnmarshalNodeJSON(minimal)
	if err != nil {
		t.Fatalf("unmarshal minimal: %v", err)
	}
	ApplyNodeDefaults(n, time.Now())
	if err := ValidateNode(n); err != nil {
		t.Fatalf("validate minimal: %v", err)
	}

	full := []byte(`{
	  "schema_version": 1,
	  "id": 23,
	  "title": "full",
	  "status": "done",
	  "claim_status": "validated", "outcome": "success",
	  "milestone_class": "golden",
	  "milestone_kind": "breakthrough",
	  "milestone_reason": "teacher substrate compressed by orders of magnitude",
	  "scope": "mistral-q4km ctx=2048 greedy",
	  "exit_criteria": "close after 3 seeds",
	  "parents": [1,2],
	  "continued_by": [24],
	  "superseded_by": [25],
	  "agent": "a",
	  "tags": ["x"],
	  "created": "2026-05-31T10:00:00Z",
	  "modified": "2026-05-31T11:00:00Z",
	  "commits": [{"hash":"abc"}],
	  "artifacts": [{"mode":"embedded","path":"artifacts/23/m.json"}],
	  "body": "notes"
	}`)
	n, err = UnmarshalNodeJSON(full)
	if err != nil {
		t.Fatalf("unmarshal full: %v", err)
	}
	if err := ValidateNode(n); err != nil {
		t.Fatalf("validate full: %v", err)
	}
}

// TestCloneNodeDeepCopy verifies CloneNode produces a deep copy.
func TestCloneNodeDeepCopy(t *testing.T) {
	n := &Node{
		Frontmatter: Frontmatter{Title: "a", Parents: []NodeID{1}, ContinuedBy: []NodeID{2}, SupersededBy: []NodeID{3}, Tags: []string{"x"}},
		Commits:     []GitCommit{{Hash: "abc"}},
		Runs:        []RunRecord{{Host: "local", Command: "go test ./..."}},
		Artifacts:   []Artifact{{Mode: ArtifactEmbedded, Path: "p"}},
	}
	cpy := CloneNode(n)
	cpy.Parents[0] = 9
	cpy.ContinuedBy[0] = 8
	cpy.SupersededBy[0] = 7
	cpy.Tags[0] = "y"
	cpy.Commits[0].Hash = "def"
	cpy.Runs[0].Host = "gpu-node-0"
	if n.Parents[0] == 9 || n.ContinuedBy[0] == 8 || n.SupersededBy[0] == 7 || n.Tags[0] == "y" || n.Commits[0].Hash == "def" || n.Runs[0].Host == "gpu-node-0" {
		t.Fatal("expected deep copy")
	}
}

// TestValidateNodeInvalidOutcome verifies invalid outcome rejection.
func TestValidateNodeInvalidOutcome(t *testing.T) {
	n := &Node{Frontmatter: Frontmatter{Title: "x", Outcome: "wrong"}}
	ApplyNodeDefaults(n, time.Now())
	n.Outcome = "wrong"
	err := ValidateNode(n)
	if !errors.Is(err, ErrInvalidNode) {
		t.Fatalf("expected ErrInvalidNode for bad outcome, got %v", err)
	}
}

// TestApplyNodeDefaultsSetsOutcome verifies outcome defaults to unset.
func TestApplyNodeDefaultsSetsOutcome(t *testing.T) {
	n := &Node{Frontmatter: Frontmatter{Title: "t"}}
	ApplyNodeDefaults(n, time.Now())
	if n.Outcome != OutcomeUnset {
		t.Fatalf("expected outcome unset, got %q", n.Outcome)
	}
}

// TestValidateNodeRejectsZeroSemanticLinks verifies semantic link IDs cannot be zero.
func TestValidateNodeRejectsZeroSemanticLinks(t *testing.T) {
	n := &Node{Frontmatter: Frontmatter{Title: "x", ContinuedBy: []NodeID{0}}}
	ApplyNodeDefaults(n, time.Now())
	err := ValidateNode(n)
	if !errors.Is(err, ErrInvalidNode) {
		t.Fatalf("expected ErrInvalidNode for zero continued_by id, got %v", err)
	}
}

// TestValidateResourceEndpointKinds verifies endpoint validation semantics.
func TestValidateResourceEndpointKinds(t *testing.T) {
	okIP := Resource{ID: "gpu0", Label: "gpu zero", Kind: ResourceGPU, Endpoint: "10.0.0.14", EndpointKind: EndpointIP, Capacity: 1, Enabled: true}
	if err := ValidateResource(okIP); err != nil {
		t.Fatalf("expected valid ip endpoint, got %v", err)
	}
	okDNS := Resource{ID: "gpu1", Label: "gpu one", Kind: ResourceGPU, Endpoint: "gpu03.int.lab", EndpointKind: EndpointDNS, Capacity: 1, Enabled: true}
	if err := ValidateResource(okDNS); err != nil {
		t.Fatalf("expected valid dns endpoint, got %v", err)
	}
	badNickname := Resource{ID: "gpu2", Label: "gpu two", Kind: ResourceGPU, Endpoint: "gpu-node-0", EndpointKind: EndpointDNS, Capacity: 1, Enabled: true}
	if err := ValidateResource(badNickname); !errors.Is(err, ErrInvalidResource) {
		t.Fatalf("expected invalid nickname endpoint, got %v", err)
	}
}

// TestValidateRunRecordEndpointKinds verifies run endpoint validation semantics.
func TestValidateRunRecordEndpointKinds(t *testing.T) {
	if err := ValidateRunRecord(RunRecord{Endpoint: "10.0.0.14", EndpointKind: EndpointIP}); err != nil {
		t.Fatalf("valid ip run endpoint: %v", err)
	}
	if err := ValidateRunRecord(RunRecord{Endpoint: "gpu03.int.lab", EndpointKind: EndpointDNS}); err != nil {
		t.Fatalf("valid dns run endpoint: %v", err)
	}
	if err := ValidateRunRecord(RunRecord{Endpoint: "gpu-node-0", EndpointKind: EndpointDNS}); !errors.Is(err, ErrInvalidNode) {
		t.Fatalf("expected invalid run dns endpoint, got %v", err)
	}
}
