// Package bridgejson holds the pure-Go JSON payload parsing shared by the
// rt-bridge cgo exports and their tests.
//
// It deliberately carries no cgo constraint: cmd/rt-bridge/main.go imports
// "C", so it is excluded from the build when CGO_ENABLED=0, and logic that
// lives only in that file cannot be built, vetted or tested in a non-cgo
// build. Keeping it here lets CGO_ENABLED=0 vet and test the bridge package
// while the cgo build still uses the same code.
package bridgejson

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"

	"github.com/frudas24/research-tree/pkg/retree"
)

// ParsePartialNodeID extracts and validates the node ID of a partial node
// update payload. IDs that JSON cannot represent exactly as an integer are
// rejected rather than silently rounded.
func ParsePartialNodeID(partial map[string]json.RawMessage) (retree.NodeID, error) {
	raw, ok := partial["id"]
	if !ok {
		return 0, fmt.Errorf("id required in update payload")
	}
	var num json.Number
	if err := json.Unmarshal(raw, &num); err != nil {
		return 0, fmt.Errorf("id must be a JSON number")
	}
	if i, err := strconv.ParseInt(num.String(), 10, 64); err == nil {
		if i <= 0 {
			return 0, fmt.Errorf("id must be positive")
		}
		if i > 1<<53 {
			return 0, fmt.Errorf("id %q exceeds precise JSON integer range", num.String())
		}
		return retree.NodeID(i), nil
	}
	f, err := num.Float64()
	if err != nil || math.Trunc(f) != f || f <= 0 {
		return 0, fmt.Errorf("invalid id %q", num.String())
	}
	if f > float64(1<<53) {
		return 0, fmt.Errorf("id %q exceeds precise JSON integer range", num.String())
	}
	return retree.NodeID(uint64(f)), nil
}
