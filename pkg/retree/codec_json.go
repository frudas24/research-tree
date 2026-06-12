package retree

import "encoding/json"

// MarshalNodeJSON serializes a node using indented JSON for readability.
func MarshalNodeJSON(n *Node) ([]byte, error) {
	return json.MarshalIndent(n, "", "  ")
}

// UnmarshalNodeJSON parses a node from JSON.
func UnmarshalNodeJSON(b []byte) (*Node, error) {
	var n Node
	if err := json.Unmarshal(b, &n); err != nil {
		return nil, err
	}
	return &n, nil
}
