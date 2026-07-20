package retree

import (
	"slices"
	"strings"
)

// FeatureStatus is the lifecycle state chosen by the maintainer.
type FeatureStatus string

const (
	FeatureActive   FeatureStatus = "active"
	FeatureDegraded FeatureStatus = "degraded"
	FeatureRetired  FeatureStatus = "retired"
)

var validFeatureStatuses = []FeatureStatus{
	FeatureActive,
	FeatureDegraded,
	FeatureRetired,
}

// DerivedHealth is the evidence-based health computed by RT at read time.
// It is NOT persisted — it is computed from linked nodes and edges.
type DerivedHealth string

const (
	HealthClean     DerivedHealth = "clean"
	HealthWarning   DerivedHealth = "warning"
	HealthDegraded  DerivedHealth = "degraded"
	HealthUnmoored  DerivedHealth = "unmoored"
)

// FeatureNodeRole classifies a node's relationship to a feature.
type FeatureNodeRole string

const (
	RoleProposal       FeatureNodeRole = "proposal"
	RoleImplementation FeatureNodeRole = "implementation"
	RoleExperiment     FeatureNodeRole = "experiment"
	RoleBenchmark      FeatureNodeRole = "benchmark"
	RoleRegression     FeatureNodeRole = "regression"
	RoleFix            FeatureNodeRole = "fix"
	RoleDecision       FeatureNodeRole = "decision"
	RoleDocumentation  FeatureNodeRole = "documentation"
)

var validFeatureNodeRoles = []FeatureNodeRole{
	RoleProposal,
	RoleImplementation,
	RoleExperiment,
	RoleBenchmark,
	RoleRegression,
	RoleFix,
	RoleDecision,
	RoleDocumentation,
}

// currentNodeRoles lists the roles that can move current_node.
var currentNodeRoles = map[FeatureNodeRole]bool{
	RoleImplementation: true,
	RoleFix:            true,
	RoleDecision:       true,
}

// FeatureLinkedNode is a node and its role within a feature.
type FeatureLinkedNode struct {
	NodeID NodeID          `json:"node_id"`
	Role   FeatureNodeRole `json:"role"`
}

// Feature is a living project entity that spans multiple RT nodes.
type Feature struct {
	ID               string              `json:"id"`
	Name             string              `json:"name"`
	Slug             string              `json:"slug"`
	Status           FeatureStatus       `json:"status"`
	CreatedFrom      NodeID              `json:"created_from"`
	CurrentNode      NodeID              `json:"current_node,omitempty"`
	CurrentNodeMode  string              `json:"current_node_mode,omitempty"` // "explicit" or "derived"
	Nodes            []FeatureLinkedNode  `json:"nodes"`
}

// FeatureEdge is a typed operational relationship between two features.
// Edge types are defined in M2; the type is used here for the data model.
type FeatureEdgeType string

const (
	EdgeDependsOn       FeatureEdgeType = "depends_on"
	EdgeCollaboratesWith FeatureEdgeType = "collaborates_with"
	EdgeSupersedes      FeatureEdgeType = "supersedes"
)

var validFeatureEdgeTypes = []FeatureEdgeType{
	EdgeDependsOn,
	EdgeCollaboratesWith,
	EdgeSupersedes,
}

// FeatureEdge is persisted in feature_edges.jsonl.
type FeatureEdge struct {
	From        string         `json:"from"`
	To          string         `json:"to"`
	Type        FeatureEdgeType `json:"type"`
	CreatedFrom NodeID          `json:"created_from"`
}

// ValidateFeature validates a Feature payload.
func ValidateFeature(f *Feature) error {
	if f == nil {
		return newInvalidFeatureError("nil")
	}
	if strings.TrimSpace(f.Name) == "" {
		return newInvalidFeatureError("name required")
	}
	if f.Slug == "" {
		return newInvalidFeatureError("slug required")
	}
	if !slices.Contains(validFeatureStatuses, f.Status) {
		return newInvalidFeatureError("unknown status: " + string(f.Status))
	}
	if f.CreatedFrom == 0 {
		return newInvalidFeatureError("created_from required")
	}
	for _, n := range f.Nodes {
		if n.NodeID == 0 {
			return newInvalidFeatureError("linked node id cannot be 0")
		}
		if !slices.Contains(validFeatureNodeRoles, n.Role) {
			return newInvalidFeatureError("unknown node role: " + string(n.Role))
		}
	}
	if f.CurrentNodeMode != "" && f.CurrentNodeMode != "explicit" && f.CurrentNodeMode != "derived" {
		return newInvalidFeatureError("current_node_mode must be explicit or derived")
	}
	return nil
}

func newInvalidFeatureError(msg string) error {
	return &FeatureError{msg: msg}
}

// FeatureError is returned for invalid feature data.
type FeatureError struct {
	msg string
}

func (e *FeatureError) Error() string {
	return "invalid feature: " + e.msg
}

// ValidateFeatureEdge validates a FeatureEdge payload.
func ValidateFeatureEdge(e *FeatureEdge) error {
	if e == nil {
		return newInvalidFeatureError("nil edge")
	}
	if e.From == "" {
		return newInvalidFeatureError("edge from required")
	}
	if e.To == "" {
		return newInvalidFeatureError("edge to required")
	}
	if !slices.Contains(validFeatureEdgeTypes, e.Type) {
		return newInvalidFeatureError("unknown edge type: " + string(e.Type))
	}
	if e.CreatedFrom == 0 {
		return newInvalidFeatureError("edge created_from required")
	}
	return nil
}
