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

// TestParseOptionalBool verifies strict boolean parsing with rejection of unknown values.
func TestParseOptionalBool(t *testing.T) {
	for _, tt := range []struct {
		raw     string
		want    *bool
		wantErr bool
	}{
		{"", nil, false},
		{"  ", nil, false},
		{"true", boolPtr(true), false},
		{"TRUE", boolPtr(true), false},
		{"1", boolPtr(true), false},
		{"yes", boolPtr(true), false},
		{"false", boolPtr(false), false},
		{"0", boolPtr(false), false},
		{"no", boolPtr(false), false},
		{"banana", nil, true},
		{"maybe", nil, true},
		{"2", nil, true},
		{"y", nil, true},
	} {
		got, err := parseOptionalBool(tt.raw)
		if tt.wantErr {
			if err == nil {
				t.Errorf("parseOptionalBool(%q): expected error, got %v", tt.raw, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("parseOptionalBool(%q): unexpected error: %v", tt.raw, err)
			continue
		}
		if tt.want == nil {
			if got != nil {
				t.Errorf("parseOptionalBool(%q): want nil, got %v", tt.raw, got)
			}
			continue
		}
		if got == nil || *got != *tt.want {
			t.Errorf("parseOptionalBool(%q): want %v, got %v", tt.raw, *tt.want, got)
		}
	}
}

// TestParseOptionalBoolAdversarial verifies that a previously-silent invalid
// value (which coerced to false and inverted the filter) now errors.
func TestParseOptionalBoolAdversarial(t *testing.T) {
	if _, err := parseOptionalBool("banana"); err == nil {
		t.Fatalf("invalid boolean must be rejected, not coerced to false")
	}
}

// TestSplitCommandArgs verifies editor command lines resolve to binary plus flags.
func TestSplitCommandArgs(t *testing.T) {
	for _, tt := range []struct {
		in   string
		want []string
	}{
		{"", nil},
		{"   ", nil},
		{"vi", []string{"vi"}},
		{"code --wait", []string{"code", "--wait"}},
		{"vim -u NONE -f", []string{"vim", "-u", "NONE", "-f"}},
		{`"/my editor" -f`, []string{"/my editor", "-f"}},
		{"nano --syntax=markdown", []string{"nano", "--syntax=markdown"}},
	} {
		got := splitCommandArgs(tt.in)
		if len(got) != len(tt.want) {
			t.Errorf("splitCommandArgs(%q): want %v, got %v", tt.in, tt.want, got)
			continue
		}
		for i := range got {
			if got[i] != tt.want[i] {
				t.Errorf("splitCommandArgs(%q): want %v, got %v", tt.in, tt.want, got)
				break
			}
		}
	}
}

// boolPtr returns a pointer to v for test assertions.
func boolPtr(v bool) *bool { return &v }
