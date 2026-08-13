package retree

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestScanAndRepairLegacyDoneUnsetOutcomesJSON verifies legacy done+unset JSON
// stores can be scanned and explicitly repaired without weakening strict loads.
func TestScanAndRepairLegacyDoneUnsetOutcomesJSON(t *testing.T) {
	s := kindStore(t)
	dir := s.nodesDir()

	legacy := &Node{Frontmatter: Frontmatter{
		SchemaVersion: CurrentSchemaVersion,
		ID:            1,
		Title:         "legacy",
		Kind:          "",
		Status:        StatusDone,
		Outcome:       OutcomeUnset,
		ClaimStatus:   ClaimProvisional,
	}}
	b, err := json.MarshalIndent(legacy, "", "  ")
	if err != nil {
		t.Fatalf("marshal legacy: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "0001.json"), append(b, '\n'), 0o644); err != nil {
		t.Fatalf("write legacy node: %v", err)
	}

	if _, err := s.QueryNodes(Filter{}); !errors.Is(err, ErrInvalidNode) {
		t.Fatalf("strict query must reject legacy store, got %v", err)
	}

	report, err := s.ScanLegacyDoneUnsetOutcomes()
	if err != nil {
		t.Fatalf("scan legacy outcomes: %v", err)
	}
	if len(report.Issues) != 1 || report.Issues[0].ID != 1 {
		t.Fatalf("unexpected scan report: %+v", report)
	}

	report, err = s.RepairLegacyDoneUnsetOutcomes(map[NodeID]Outcome{1: OutcomeInconclusive})
	if err != nil {
		t.Fatalf("repair legacy outcomes: %v", err)
	}
	if len(report.Repaired) != 1 || report.Repaired[0] != 1 {
		t.Fatalf("unexpected repair report: %+v", report)
	}

	nodes, err := s.QueryNodes(Filter{})
	if err != nil {
		t.Fatalf("strict query after repair: %v", err)
	}
	if len(nodes) != 1 || nodes[0].Outcome != OutcomeInconclusive {
		t.Fatalf("unexpected repaired nodes: %+v", nodes)
	}
}

// TestRepairLegacyDoneUnsetRequiresFullExplicitCoverage verifies repair stays
// defensive and refuses partial or non-terminal mappings.
func TestRepairLegacyDoneUnsetRequiresFullExplicitCoverage(t *testing.T) {
	s := kindStore(t)
	for _, id := range []NodeID{1, 2} {
		legacy := &Node{Frontmatter: Frontmatter{
			SchemaVersion: CurrentSchemaVersion,
			ID:            id,
			Title:         fmt.Sprintf("legacy-%d", id),
			Status:        StatusDone,
			Outcome:       OutcomeUnset,
			ClaimStatus:   ClaimProvisional,
		}}
		b, err := json.MarshalIndent(legacy, "", "  ")
		if err != nil {
			t.Fatalf("marshal legacy %d: %v", id, err)
		}
		name := filepath.Join(s.nodesDir(), fmt.Sprintf("%04d.json", id))
		if err := os.WriteFile(name, append(b, '\n'), 0o644); err != nil {
			t.Fatalf("write legacy %d: %v", id, err)
		}
	}

	if _, err := s.RepairLegacyDoneUnsetOutcomes(map[NodeID]Outcome{1: OutcomeSuccess}); !errors.Is(err, ErrInvalidNode) {
		t.Fatalf("partial repair must fail, got %v", err)
	}
	if _, err := s.RepairLegacyDoneUnsetOutcomes(map[NodeID]Outcome{1: OutcomeUnset, 2: OutcomeFailure}); !errors.Is(err, ErrInvalidNode) {
		t.Fatalf("unset repair outcome must fail, got %v", err)
	}
}

// TestRepairLegacyDoneUnsetOutcomesFailsEarlyOnCorruptSidecar verifies the
// repair path still audits sidecars before mutating storage.
func TestRepairLegacyDoneUnsetOutcomesFailsEarlyOnCorruptSidecar(t *testing.T) {
	s := kindStore(t)
	legacy := &Node{Frontmatter: Frontmatter{
		SchemaVersion: CurrentSchemaVersion,
		ID:            1,
		Title:         "legacy",
		Status:        StatusDone,
		Outcome:       OutcomeUnset,
		ClaimStatus:   ClaimProvisional,
	}}
	b, err := json.MarshalIndent(legacy, "", "  ")
	if err != nil {
		t.Fatalf("marshal legacy: %v", err)
	}
	if err := os.WriteFile(filepath.Join(s.nodesDir(), "0001.json"), append(b, '\n'), 0o644); err != nil {
		t.Fatalf("write legacy node: %v", err)
	}
	if err := os.Remove(s.featuresPath()); err != nil {
		t.Fatalf("remove features.json: %v", err)
	}
	if err := os.Mkdir(s.featuresPath(), 0o755); err != nil {
		t.Fatalf("replace features.json with dir: %v", err)
	}

	if _, err := s.RepairLegacyDoneUnsetOutcomes(map[NodeID]Outcome{1: OutcomeSuccess}); err == nil {
		t.Fatal("repair must fail on corrupt sidecar")
	}

	got, err := s.ScanLegacyDoneUnsetOutcomes()
	if err != nil {
		t.Fatalf("scan after failed repair: %v", err)
	}
	if len(got.Issues) != 1 || got.Issues[0].ID != 1 {
		t.Fatalf("legacy node should remain unrepaired after failed sidecar audit: %+v", got)
	}
}

// TestRepairLegacyDoneUnsetPreSnapshotRestores verifies the pre-repair
// snapshot can be restored even though it contains legacy done+unset nodes.
func TestRepairLegacyDoneUnsetPreSnapshotRestores(t *testing.T) {
	s := kindStore(t)
	legacy := &Node{Frontmatter: Frontmatter{
		SchemaVersion: CurrentSchemaVersion,
		ID:            1,
		Title:         "legacy",
		Status:        StatusDone,
		Outcome:       OutcomeUnset,
		ClaimStatus:   ClaimProvisional,
	}}
	b, err := json.MarshalIndent(legacy, "", "  ")
	if err != nil {
		t.Fatalf("marshal legacy: %v", err)
	}
	if err := os.WriteFile(filepath.Join(s.nodesDir(), "0001.json"), append(b, '\n'), 0o644); err != nil {
		t.Fatalf("write legacy node: %v", err)
	}

	report, err := s.RepairLegacyDoneUnsetOutcomes(map[NodeID]Outcome{1: OutcomeFailure})
	if err != nil {
		t.Fatalf("repair legacy outcomes: %v", err)
	}
	if len(report.Repaired) != 1 {
		t.Fatalf("unexpected repair report: %+v", report)
	}

	snaps, err := s.ListSnapshots()
	if err != nil {
		t.Fatalf("list snapshots: %v", err)
	}
	var preID string
	for _, snap := range snaps {
		if strings.Contains(snap.Operation, "repair_legacy_outcomes_pre") {
			preID = snap.ID
			break
		}
	}
	if preID == "" {
		t.Fatalf("expected repair_legacy_outcomes_pre snapshot, got %+v", snaps)
	}

	if err := s.RestoreSnapshot(preID); err != nil {
		t.Fatalf("restore pre-repair snapshot: %v", err)
	}

	if _, err := s.QueryNodes(Filter{}); !errors.Is(err, ErrInvalidNode) {
		t.Fatalf("strict query after restoring legacy snapshot must fail, got %v", err)
	}
	got, err := s.ScanLegacyDoneUnsetOutcomes()
	if err != nil {
		t.Fatalf("scan restored pre snapshot: %v", err)
	}
	if len(got.Issues) != 1 || got.Issues[0].ID != 1 {
		t.Fatalf("restored snapshot must recover legacy issue, got %+v", got)
	}
}
