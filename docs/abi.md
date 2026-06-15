# ABI — Public API (`pkg/retree`)

`github.com/frudas24/research-tree/pkg/retree` is the primary programmatic ABI.
The CLI is a consumer of this package, not the other way around.

## Core types

### `Node`

```go
type Node struct {
    Frontmatter
    Commits            []GitCommit
    Runs               []RunRecord
    Artifacts          []Artifact
    InvalidatedBy      []NodeID
    InvalidationReason string
    Body               string
}

type Frontmatter struct {
    SchemaVersion SchemaVersion
    ID            NodeID
    Title         string
    Status        NodeStatus
    ClaimStatus   ClaimStatus
    Scope         string
    ExitCriteria  string
    Parents       []NodeID
    ContinuedBy   []NodeID
    SupersededBy  []NodeID
    Agent         string
    Tags          []string
    Created       time.Time
    Modified      time.Time
    Outcome       Outcome
    Revision       uint64
    MilestoneClass MilestoneClass
    MilestoneKind  MilestoneKind
    MilestoneReason string
}

type RunRecord struct {
    Timestamp time.Time
    Host      string
    Command   string
    OutDir    string
    Seed      string
    ETA       string
    Cost      string
    Note      string
    Valid     *bool
    InvalidReason string
}
```

### Enums

| Type | Values |
|------|--------|
| `NodeStatus` | `active`, `done`, `paused` |
| `Outcome` | `unset`, `success`, `failure`, `inconclusive` |
| `ClaimStatus` | `provisional`, `validated`, `invalidated`, `superseded` |
| `MilestoneClass` | `""`, `golden` |
| `MilestoneKind` | `""`, `champion`, `breakthrough`, `pivot` |
| `ArtifactMode` | `path`, `embedded` |
| `StorageFormat` | `json`, `bin` |

### `Filter`

```go
type Filter struct {
    Status        NodeStatus
    ClaimStatus   ClaimStatus
    Outcome       Outcome
    Tag           string
    TagsAll       []string
    TagsAny       []string
    Agent         string
    TitleContains string
    ScopeContains string
    BodyContains  string
    ContinuedBy   NodeID
    SupersededBy  NodeID
    HasArtifact    *bool
    MilestoneClass MilestoneClass
    MilestoneKind  MilestoneKind
    CreatedAfter   time.Time
    CreatedBefore time.Time
    SortBy        string
    Order         string
    Offset        int
    Limit         int
}
```

Semantics:

- `Tag` matches one exact tag.
- `TagsAll` requires all listed tags.
- `TagsAny` requires at least one listed tag.
- `BodyContains` is case-insensitive full-text substring matching on `Body`.
- `ScopeContains` is case-insensitive substring matching on `Scope`.
- `ContinuedBy` filters nodes whose `continued_by` contains the target node ID.
- `SupersededBy` filters nodes whose `superseded_by` contains the target node ID.
- `HasArtifact` filters by presence or absence of artifacts.
- `MilestoneClass` / `MilestoneKind` filter frontier-significant nodes without relying on tags.
- `SortBy` supports `id`, `title`, `created`, `modified`.
- `Order` supports `asc` and `desc`.

Milestone semantics:

- `MilestoneClass=golden` is the canonical frontier marker.
- `MilestoneKind` refines that marker as `champion`, `breakthrough`, or `pivot`.
- `MilestoneReason` is required for golden nodes and should explain why the node
  became frontier-significant.
- This axis is orthogonal to `Status`, `Outcome`, and `ClaimStatus`.

## Store lifecycle

```go
s, err := retree.Init(rootPath, retree.StorageBIN)
s, err := retree.Open(rootPath)
```

## CRUD and graph API

| Method | Signature / behavior |
|--------|-----------------------|
| `CreateNode` | `func (s *Store) CreateNode(n *Node) error` |
| `GetNode` | `func (s *Store) GetNode(id NodeID) (*Node, error)` |
| `UpdateNode` | `func (s *Store) UpdateNode(n *Node) error` |
| `DeleteNode` | `func (s *Store) DeleteNode(id NodeID, force bool) error` |
| `GetChildren` | direct children |
| `GetParents` | direct parents |
| `GetAncestors` | all ancestors |
| `GetDescendants` | all descendants |
| `GetRoots` | nodes without parents |
| `GetLeaves` | nodes without children |

Graph invariants:

- parent references must exist at mutation time
- cycles are rejected
- reparenting through `UpdateNode` preserves DAG validity
- on-disk loading may bypass parent existence checks only to restore already-persisted consistent data

## Query and mutation helpers

| Method | Behavior |
|--------|----------|
| `ListNodes(f Filter)` | returns matching IDs |
| `QueryNodes(f Filter)` | returns matching nodes |
| `AddTags(id, tags...)` | idempotent tag add |
| `RemoveTags(id, tags...)` | idempotent tag removal |
| `AddParents(id, parents...)` | additive parent edits |
| `RemoveParents(id, parents...)` | subtractive parent edits |
| `ListRelations(id)` | returns typed relation edges for one node |
| `ListAllRelations()` | returns all typed relation edges across the graph |
| `RegenerateRelations()` | rebuilds `relations.jsonl` from node data |
| `AddArtifact(id, a)` | attach path or embedded artifact |
| `RemoveArtifact(id, matcher)` | remove artifacts matching non-zero fields |
| `EmbedArtifact(id, localPath, desc)` | copy local file into store |

## Claim invalidation and branch warnings

| Method | Behavior |
|--------|----------|
| `InvalidateClaim(target, refuter, reason)` | marks claim invalid and records rationale |
| `ListBranchWarnings(agent, onlyUnacked)` | returns descendant warnings after invalidation |
| `AckBranchWarning(warningID)` | acknowledges a warning |

The model separates:

- `status`: execution lifecycle (`active`, `done`, `paused`)
- `outcome`: result quality (`success`, `failure`, `inconclusive`, `unset`)
- `claim_status`: epistemic validity (`provisional`, `validated`, `invalidated`, `superseded`)

Additional semantic fields:

- `scope`: bounded applicability of a claim or result
- `exit_criteria`: explicit closure condition for active work
- `continued_by`: operational continuation links
- `superseded_by`: semantic replacement links
- `relations`: typed cross-links such as comparison, inspiration, dependency, or aggregation
- `primary_parent`: designated canonical parent when a node has multiple DAG parents
- `runs`: structured execution records kept alongside the markdown audit trail
- `run_validity_counts`: additive dashboard counts for valid vs invalid vs unknown latest runs

## Status summaries

The package exposes scalable summary structures consumed by the CLI/FFI status dashboard.

Key properties:

- O(n) over node metadata
- additive JSON evolution
- separate counts for `status`, `outcome`, and `claim_status`
- optional hotspot ranking and status×outcome matrix

## Recovery and storage

| Method | Behavior |
|--------|----------|
| `ListSnapshots()` | lists recoverable snapshots |
| `RestoreSnapshot(snapshotID)` | restores a snapshot |
| `StorageFormat()` | returns active codec |
| `MigrateStorageFormat(target)` | migrates `json ↔ bin` |
| `RegenerateEdges()` | rebuilds edges from node graph |
| `RegenerateRelations()` | rebuilds relation edges from node graph |
| `NextID()` | previews next ID |

Operational notes:

- binary storage is the default production codec
- JSON remains useful for debugging and inspection
- snapshot retention keeps the latest three historical backups

## Sentinel errors

```go
var (
    ErrNotFound           = errors.New("not found")
    ErrUnsupportedSchema  = errors.New("unsupported schema")
    ErrInvalidNode        = errors.New("invalid node")
    ErrInvalidStatus      = errors.New("invalid status")
    ErrInvalidClaimStatus = errors.New("invalid claim status")
    ErrInvalidArtifact    = errors.New("invalid artifact")
    ErrDuplicateID        = errors.New("duplicate node id")
    ErrCycleDetected      = errors.New("cycle detected")
    ErrHasChildren        = errors.New("node has children")
)
```

## Example

```go
package main

import "github.com/frudas24/research-tree/pkg/retree"

func main() {
    s, _ := retree.Init(".research", retree.StorageBIN)

    n := &retree.Node{Frontmatter: retree.Frontmatter{
        Title: "KD baseline",
        Agent: "researcher",
        Tags:  []string{"kd"},
    }}
    _ = s.CreateNode(n)

    active, _ := s.QueryNodes(retree.Filter{
        Status: retree.StatusActive,
        Agent:  "researcher",
        SortBy: "modified",
        Order:  "desc",
    })

    _, _ = active, s.AddParents(n.ID, 2, 3)
}
```
