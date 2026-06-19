// Package retree provides the core data model and storage engine for research-tree,
// a tool for mapping scientific/research work as a directed acyclic graph (DAG).
package retree

import "time"

// SchemaVersion is the on-disk schema version.
type SchemaVersion uint16

const (
	// CurrentSchemaVersion is the schema version produced by this binary.
	CurrentSchemaVersion SchemaVersion = 1
)

// StorageFormat selects the persistence codec.
type StorageFormat string

const (
	StorageJSON StorageFormat = "json"
	StorageBIN  StorageFormat = "bin"
)

// NodeID is a unique sequential identifier within a research root.
type NodeID uint64

// NodeStatus represents the activity state of a research node.
type NodeStatus string

const (
	StatusActive NodeStatus = "active"
	StatusDone   NodeStatus = "done"
	StatusPaused NodeStatus = "paused"
)

// Outcome captures the result of completed work.
type Outcome string

const (
	OutcomeUnset        Outcome = "unset"
	OutcomeSuccess      Outcome = "success"
	OutcomeFailure      Outcome = "failure"
	OutcomeInconclusive Outcome = "inconclusive"
)

// ClaimStatus represents epistemic confidence and validity over time.
type ClaimStatus string

const (
	ClaimProvisional ClaimStatus = "provisional"
	ClaimValidated   ClaimStatus = "validated"
	ClaimInvalidated ClaimStatus = "invalidated"
	ClaimSuperseded  ClaimStatus = "superseded"
)

// EvidenceStatus captures whether a node's observed evidence is trustworthy.
// This is intentionally separate from outcome and claim status.
type EvidenceStatus string

const (
	EvidenceClean       EvidenceStatus = "clean"
	EvidenceSuspect     EvidenceStatus = "suspect"
	EvidencePoisoned    EvidenceStatus = "poisoned"
	EvidenceRevalidated EvidenceStatus = "revalidated"
)

// EvidenceCause identifies the dominant reason evidence became unreliable.
type EvidenceCause string

const (
	EvidenceCauseNone          EvidenceCause = ""
	EvidenceCauseBaseSnapshot  EvidenceCause = "base_snapshot"
	EvidenceCauseToolchain     EvidenceCause = "toolchain"
	EvidenceCauseExporter      EvidenceCause = "exporter"
	EvidenceCauseDataset       EvidenceCause = "dataset"
	EvidenceCausePromptSurface EvidenceCause = "prompt_surface"
	EvidenceCauseRuntimeEnv    EvidenceCause = "runtime_env"
	EvidenceCauseUnknown       EvidenceCause = "unknown"
)

// MilestoneClass marks frontier-significant nodes without conflating them with status/outcome.
type MilestoneClass string

const (
	MilestoneNone   MilestoneClass = ""
	MilestoneGolden MilestoneClass = "golden"
)

// MilestoneKind refines why a milestone matters within its lineage.
type MilestoneKind string

const (
	MilestoneKindNone         MilestoneKind = ""
	MilestoneKindChampion     MilestoneKind = "champion"
	MilestoneKindBreakthrough MilestoneKind = "breakthrough"
	MilestoneKindPivot        MilestoneKind = "pivot"
)

// ArtifactMode specifies how an artifact is stored.
type ArtifactMode string

const (
	ArtifactPath     ArtifactMode = "path"
	ArtifactEmbedded ArtifactMode = "embedded"
)

// Artifact is a reference to a file or dataset produced by a research node.
type Artifact struct {
	Mode        ArtifactMode `json:"mode" yaml:"mode"`
	Host        string       `json:"host,omitempty" yaml:"host,omitempty"`
	Path        string       `json:"path" yaml:"path"`
	Description string       `json:"description,omitempty" yaml:"desc,omitempty"`
	SizeBytes   int64        `json:"size_bytes,omitempty" yaml:"size_bytes,omitempty"`
}

// ResourceKind classifies a resource for scheduling and reporting.
type ResourceKind string

const (
	ResourceMachine ResourceKind = "machine"
	ResourceGPU     ResourceKind = "gpu"
	ResourceCPUSlot ResourceKind = "cpu-slot"
	ResourceOther   ResourceKind = "other"
)

// EndpointKind states how a resource endpoint should be interpreted.
type EndpointKind string

const (
	EndpointNone EndpointKind = "none"
	EndpointIP   EndpointKind = "ip"
	EndpointDNS  EndpointKind = "dns"
)

// LeaseMode controls whether a resource claim is exclusive or shared.
type LeaseMode string

const (
	LeaseExclusive LeaseMode = "exclusive"
	LeaseShared    LeaseMode = "shared"
)

// ResourceSpec holds stable, low-churn hardware notes for a resource.
type ResourceSpec struct {
	OS          string `json:"os,omitempty" yaml:"os,omitempty"`
	CPU         string `json:"cpu,omitempty" yaml:"cpu,omitempty"`
	RAMGB       int    `json:"ram_gb,omitempty" yaml:"ram_gb,omitempty"`
	GPU         string `json:"gpu,omitempty" yaml:"gpu,omitempty"`
	VRAMGB      int    `json:"vram_gb,omitempty" yaml:"vram_gb,omitempty"`
	StorageHint string `json:"storage_hint,omitempty" yaml:"storage_hint,omitempty"`
}

// Resource is a schedulable piece of hardware or capacity slice.
type Resource struct {
	ID           string       `json:"id" yaml:"id"`
	Label        string       `json:"label" yaml:"label"`
	Endpoint     string       `json:"endpoint,omitempty" yaml:"endpoint,omitempty"`
	EndpointKind EndpointKind `json:"endpoint_kind,omitempty" yaml:"endpoint_kind,omitempty"`
	Kind         ResourceKind `json:"kind" yaml:"kind"`
	Tags         []string     `json:"tags,omitempty" yaml:"tags,omitempty"`
	Enabled      bool         `json:"enabled" yaml:"enabled"`
	Maintenance  bool         `json:"maintenance,omitempty" yaml:"maintenance,omitempty"`
	Capacity     int          `json:"capacity,omitempty" yaml:"capacity,omitempty"`
	Spec         ResourceSpec `json:"spec,omitempty" yaml:"spec,omitempty"`
	Created      time.Time    `json:"created,omitempty" yaml:"created,omitempty"`
	Modified     time.Time    `json:"modified,omitempty" yaml:"modified,omitempty"`
}

// ResourceLease is an active node->resource occupancy claim.
type ResourceLease struct {
	ResourceID string    `json:"resource_id" yaml:"resource_id"`
	NodeID     NodeID    `json:"node_id" yaml:"node_id"`
	Mode       LeaseMode `json:"mode" yaml:"mode"`
	ClaimedBy  string    `json:"claimed_by,omitempty" yaml:"claimed_by,omitempty"`
	Note       string    `json:"note,omitempty" yaml:"note,omitempty"`
	ClaimedAt  time.Time `json:"claimed_at" yaml:"claimed_at"`
}

// ResourceEventAction captures lease lifecycle changes for auditability.
type ResourceEventAction string

const (
	ResourceEventClaim             ResourceEventAction = "claim"
	ResourceEventRelease           ResourceEventAction = "release"
	ResourceEventAutoReleaseDone   ResourceEventAction = "auto_release_done"
	ResourceEventAutoReleasePause  ResourceEventAction = "auto_release_paused"
	ResourceEventAutoReleaseDelete ResourceEventAction = "auto_release_delete"
)

// ResourceEvent records historical occupancy changes for one resource.
type ResourceEvent struct {
	ResourceID string              `json:"resource_id" yaml:"resource_id"`
	NodeID     NodeID              `json:"node_id" yaml:"node_id"`
	Action     ResourceEventAction `json:"action" yaml:"action"`
	Mode       LeaseMode           `json:"mode,omitempty" yaml:"mode,omitempty"`
	ClaimedBy  string              `json:"claimed_by,omitempty" yaml:"claimed_by,omitempty"`
	Note       string              `json:"note,omitempty" yaml:"note,omitempty"`
	Reason     string              `json:"reason,omitempty" yaml:"reason,omitempty"`
	Timestamp  time.Time           `json:"timestamp" yaml:"timestamp"`
}

// GitCommit references a git commit associated with the node.
type GitCommit struct {
	Hash    string `json:"hash" yaml:"hash"`
	Message string `json:"message,omitempty" yaml:"message,omitempty"`
}

// RunRecord is a structured execution record attached to a node.
type RunRecord struct {
	Timestamp     time.Time    `json:"timestamp" yaml:"timestamp"`
	ResourceID    string       `json:"resource_id,omitempty" yaml:"resource_id,omitempty"`
	Endpoint      string       `json:"endpoint,omitempty" yaml:"endpoint,omitempty"`
	EndpointKind  EndpointKind `json:"endpoint_kind,omitempty" yaml:"endpoint_kind,omitempty"`
	Host          string       `json:"host,omitempty" yaml:"host,omitempty"`
	Command       string       `json:"command,omitempty" yaml:"command,omitempty"`
	OutDir        string       `json:"outdir,omitempty" yaml:"outdir,omitempty"`
	Seed          string       `json:"seed,omitempty" yaml:"seed,omitempty"`
	ETA           string       `json:"eta,omitempty" yaml:"eta,omitempty"`
	Cost          string       `json:"cost,omitempty" yaml:"cost,omitempty"`
	Note          string       `json:"note,omitempty" yaml:"note,omitempty"`
	Valid         *bool        `json:"valid,omitempty" yaml:"valid,omitempty"`
	InvalidReason string       `json:"invalid_reason,omitempty" yaml:"invalid_reason,omitempty"`
}

// RelationType classifies a typed cross-edge between research nodes.
type RelationType string

const (
	RelDependsOn       RelationType = "depends_on"
	RelComparesAgainst RelationType = "compares_against"
	RelInspiredBy      RelationType = "inspired_by"
	RelAggregates      RelationType = "aggregates"
)

// Relation is a typed, informational cross-edge between nodes.
// Unlike Parents, relations do NOT participate in DAG cycle enforcement —
// they are purely descriptive (comparison, inspiration, aggregation).
type Relation struct {
	Type   RelationType `json:"type" yaml:"type"`
	Target NodeID       `json:"target" yaml:"target"`
	Note   string       `json:"note,omitempty" yaml:"note,omitempty"`
}

// Frontmatter holds the structured metadata of a research node.
type Frontmatter struct {
	SchemaVersion   SchemaVersion  `json:"schema_version" yaml:"schema_version"`
	ID              NodeID         `json:"id" yaml:"id"`
	Title           string         `json:"title" yaml:"title"`
	Status          NodeStatus     `json:"status" yaml:"status"`
	ClaimStatus     ClaimStatus    `json:"claim_status,omitempty" yaml:"claim_status,omitempty"`
	EvidenceStatus  EvidenceStatus `json:"evidence_status,omitempty" yaml:"evidence_status,omitempty"`
	EvidenceCause   EvidenceCause  `json:"evidence_cause,omitempty" yaml:"evidence_cause,omitempty"`
	EvidenceScope   string         `json:"evidence_scope,omitempty" yaml:"evidence_scope,omitempty"`
	Scope           string         `json:"scope,omitempty" yaml:"scope,omitempty"`
	ExitCriteria    string         `json:"exit_criteria,omitempty" yaml:"exit_criteria,omitempty"`
	Parents         []NodeID       `json:"parents,omitempty" yaml:"parents,omitempty"`
	ContinuedBy     []NodeID       `json:"continued_by,omitempty" yaml:"continued_by,omitempty"`
	SupersededBy    []NodeID       `json:"superseded_by,omitempty" yaml:"superseded_by,omitempty"`
	Agent           string         `json:"agent,omitempty" yaml:"agent,omitempty"`
	Tags            []string       `json:"tags,omitempty" yaml:"tags,omitempty"`
	Created         time.Time      `json:"created,omitempty" yaml:"created,omitempty"`
	Modified        time.Time      `json:"modified,omitempty" yaml:"modified,omitempty"`
	Outcome         Outcome        `json:"outcome,omitempty" yaml:"outcome,omitempty"`
	Revision        uint64         `json:"revision" yaml:"revision"`
	MilestoneClass  MilestoneClass `json:"milestone_class,omitempty" yaml:"milestone_class,omitempty"`
	MilestoneKind   MilestoneKind  `json:"milestone_kind,omitempty" yaml:"milestone_kind,omitempty"`
	MilestoneReason string         `json:"milestone_reason,omitempty" yaml:"milestone_reason,omitempty"`
	Relations       []Relation     `json:"relations,omitempty" yaml:"relations,omitempty"`
	PrimaryParent   *NodeID        `json:"primary_parent,omitempty" yaml:"primary_parent,omitempty"`
}

// Node is a unit of research: an idea, experiment, or decision point.
type Node struct {
	Frontmatter
	Commits            []GitCommit `json:"commits,omitempty" yaml:"commits,omitempty"`
	Runs               []RunRecord `json:"runs,omitempty" yaml:"runs,omitempty"`
	Artifacts          []Artifact  `json:"artifacts,omitempty" yaml:"artifacts,omitempty"`
	InvalidatedBy      []NodeID    `json:"invalidated_by,omitempty" yaml:"invalidated_by,omitempty"`
	InvalidationReason string      `json:"invalidation_reason,omitempty" yaml:"invalidation_reason,omitempty"`
	PoisonedBy         []NodeID    `json:"poisoned_by,omitempty" yaml:"poisoned_by,omitempty"`
	RevalidatedBy      []NodeID    `json:"revalidated_by,omitempty" yaml:"revalidated_by,omitempty"`
	PoisonReason       string      `json:"poison_reason,omitempty" yaml:"poison_reason,omitempty"`
	Body               string      `json:"body,omitempty" yaml:"-"`
}

// Filter narrows ListNodes / QueryNodes results.
type Filter struct {
	Status         NodeStatus     `json:"status,omitempty"`
	ClaimStatus    ClaimStatus    `json:"claim_status,omitempty"`
	EvidenceStatus EvidenceStatus `json:"evidence_status,omitempty"`
	EvidenceCause  EvidenceCause  `json:"evidence_cause,omitempty"`
	Outcome        Outcome        `json:"outcome,omitempty"`
	Tag            string         `json:"tag,omitempty"`
	TagsAll        []string       `json:"tags_all,omitempty"`
	TagsAny        []string       `json:"tags_any,omitempty"`
	Agent          string         `json:"agent,omitempty"`
	TitleContains  string         `json:"title_contains,omitempty"`
	ScopeContains  string         `json:"scope_contains,omitempty"`
	BodyContains   string         `json:"body_contains,omitempty"`
	ContinuedBy    NodeID         `json:"continued_by,omitempty"`
	SupersededBy   NodeID         `json:"superseded_by,omitempty"`
	HasArtifact    *bool          `json:"has_artifact,omitempty"`
	MilestoneClass MilestoneClass `json:"milestone_class,omitempty"`
	MilestoneKind  MilestoneKind  `json:"milestone_kind,omitempty"`
	CreatedAfter   time.Time      `json:"created_after,omitempty"`
	CreatedBefore  time.Time      `json:"created_before,omitempty"`
	SortBy         string         `json:"sort_by,omitempty"`
	Order          string         `json:"order,omitempty"`
	Offset         int            `json:"offset,omitempty"`
	Limit          int            `json:"limit,omitempty"`
}

// BranchWarning is a persisted warning generated when an invalidated ancestor
// impacts active descendant work.
type BranchWarning struct {
	ID            string
	Agent         string
	RootCauseNode NodeID
	ImpactedNode  NodeID
	Severity      string
	Message       string
	CreatedAt     time.Time
	AckedAt       *time.Time
}
