package retree

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
)

// MarshalNodeJSON serializes a node using indented JSON for readability.
func MarshalNodeJSON(n *Node) ([]byte, error) {
	return json.MarshalIndent(n, "", "  ")
}

// decodeJSONStrict decodes exactly one JSON value and rejects unknown fields.
// Persisted RT payloads are schema-governed; silently ignoring a misspelled or
// future field would turn malformed state into apparently valid state.
func decodeJSONStrict(b []byte, v any) error {
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return err
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		if err == nil {
			return fmt.Errorf("unexpected trailing JSON value")
		}
		return err
	}
	return nil
}

// UnmarshalNodeJSON parses a node from schema-strict JSON.
func UnmarshalNodeJSON(b []byte) (*Node, error) {
	var n Node
	if err := decodeJSONStrict(b, &n); err != nil {
		return nil, err
	}
	return &n, nil
}
