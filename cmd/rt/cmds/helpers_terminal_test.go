package cmds

import (
	"testing"

	"github.com/frudas24/research-tree/pkg/retree"
)

// TestValidateTerminalOutcome verifies the status/outcome guardrails.
func TestValidateTerminalOutcome(t *testing.T) {
	if err := validateTerminalOutcome(retree.StatusActive, retree.OutcomeUnset); err != nil {
		t.Fatalf("active should allow unset outcome: %v", err)
	}
	if err := validateTerminalOutcome(retree.StatusDone, retree.OutcomeUnset); err == nil {
		t.Fatalf("done should reject unset outcome")
	}
	if err := validateTerminalOutcome(retree.StatusDone, retree.OutcomeSuccess); err != nil {
		t.Fatalf("done+success should be valid: %v", err)
	}
}
