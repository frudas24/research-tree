package retree

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
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
