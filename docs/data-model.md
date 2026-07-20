# Data Model

## Node

A node is the atomic unit of research. It can represent a complete line of
work, an experiment, a design decision, or a pivot.

### Fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `title` | string | **Yes** | Descriptive title of the node |
| `id` | NodeID (uint64) | Auto | Sequential ID assigned on creation |
| `status` | NodeStatus | Default: `active` | Lifecycle status |
| `claim_status` | ClaimStatus | Default: `provisional` | Epistemic confidence |
| `evidence_status` | EvidenceStatus | Default: `clean` | Reliability of the observed evidence |
| `evidence_cause` | EvidenceCause | No | Dominant contamination cause if evidence is not clean |
| `evidence_scope` | string | No | Scope boundary of contamination (`host/model/surface/...`) |
| `scope` | string | No | Explicit scope of the claim or result |
| `exit_criteria` | string | No | Explicit criterion for closing the node |
| `parents` | []NodeID | No | Parent node IDs (DAG edges) |
| `continued_by` | []NodeID | No | Nodes that continue this line of work |
| `superseded_by` | []NodeID | No | Nodes that replace this conclusion |
| `agent` | string | No | Identifier of responsible agent |
| `tags` | []string | No | Flexible categorization |
| `created` | time.Time | Auto | Creation timestamp |
| `modified` | time.Time | Auto | Last modification timestamp |
| `commits` | []GitCommit | No | Associated git commits |
| `runs` | []RunRecord | No | Structured runs (host/cmd/outdir/seed/eta/cost/note/valid) |
| `artifacts` | []Artifact | No | Produced artifacts |
| `invalidated_by` | []NodeID | No | Nodes that refute this claim |
| `invalidation_reason` | string | No | Reason for invalidation |
| `poisoned_by` | []NodeID | No | Nodes/events that explain the contamination |
| `revalidated_by` | []NodeID | No | Clean reruns that supersede contaminated evidence |
| `poison_reason` | string | No | Mandatory narrative reason when `evidence_status=poisoned` |
| `milestone_class` | string | No | Class of structural milestone (`golden`) |
| `milestone_kind` | string | No | Optional subtype (`champion`, `breakthrough`, `pivot`) |
| `milestone_reason` | string | No | Brief mandatory reason for golden milestones |
| `body` | string | No | Free Markdown body |

### Activity status

| Value | Meaning |
|-------|---------|
| `active` | Working on this right now |
| `done` | Work is finished (no longer active) |
| `paused` | Work is temporarily on hold |

### Outcome (result)

| Value | Meaning |
|-------|---------|
| `unset` | No result yet (default for active) |
| `success` | Completed successfully |
| `failure` | Did not work, dead end |
| `inconclusive` | Results ambiguous |

Combined: `done`+`success` = finished well. `done`+`failure` = dead end.

### Claim status

```

### Evidence status

```
clean ─────────► suspect ───────► poisoned ───────► revalidated
```

This dimension is intentionally separate from claim truth:

- `claim_status=invalidated` means the **idea/result** was refuted.
- `evidence_status=poisoned` means the **measurement substrate** was unreliable.

Typical poison causes:

- corrupted base snapshot
- broken exporter/toolchain
- bad prompt surface used as judge
- runtime environment mismatch
provisional ──► validated
     └───────► invalidated (requires invalidated_by)
     └───────► superseded
```

### Artifact modes

- **path:** external reference (`host:path`). For large files (>1MB).
- **embedded:** local copy within `.research/artifacts/`. For small files.

### Validation rules

- `title` cannot be empty
- `status` must be one of the valid values
- `claim_status` must be one of the valid values
- If `claim_status=invalidated`, `invalidated_by` cannot be empty
- `parents` cannot contain ID 0
- `continued_by` cannot contain ID 0
- `superseded_by` cannot contain ID 0
- `artifacts[].mode=path` requires non-empty `host`
- `artifacts[].path` is mandatory
- `milestone_class=golden` requires non-empty `milestone_reason`
- `milestone_kind` can only be used when `milestone_class` is present
- `evidence_status=poisoned` requires non-empty `poison_reason`
- `evidence_status=revalidated` requires non-empty `revalidated_by`

### Golden milestones

Golden milestones are a **third semantic dimension** separate from:

- `status` (lifecycle)
- `outcome` (work result)
- `claim_status` (epistemic validity)

A golden node marks a boundary result or lineage reference. Typical cases:

- **`champion`**: best artifact or current operative result of a line
- **`breakthrough`**: broke a previous ceiling or turned intuition into real capability
- **`pivot`**: moved the bottleneck or reordered the roadmap

Should not be modeled with ad-hoc tags (`golden`, `canonical`, `source-of-truth`) or
only with prose in `body`. The canonical form is:

```json
{
  "milestone_class": "golden",
  "milestone_kind": "breakthrough",
  "milestone_reason": "Compressed teacher supervision ~3089x without losing pilot recovery"
}
```

In human CLI, the canonical query is:

```bash
rt node list --milestone-class golden
```

And there is a convenience shortcut:

```bash
rt golden
```

### Example (JSON)

```json
{
  "schema_version": 1,
  "id": 23,
  "title": "KD sparse top-k k=128 vs k=64",
  "status": "done",
  "claim_status": "validated",
  "evidence_status": "revalidated",
  "revalidated_by": [39],
  "scope": "mistral-q4km ctx=2048 greedy",
  "exit_criteria": "close after 3 seeds under stable variance",
  "parents": [14, 16],
  "continued_by": [24],
  "superseded_by": [25],
  "agent": "researcher",
  "tags": ["kd", "sparse", "experimento"],
  "created": "2026-05-30T14:00:00Z",
  "modified": "2026-05-30T15:30:00Z",
  "commits": [
    {"hash": "abc123", "message": "flag --top-k"},
    {"hash": "def456", "message": "fix reshape"}
  ],
  "artifacts": [
    {"mode": "path", "host": "gpu-node-0", "path": "/tmp/kd_sparse_t9k_k128", "size_bytes": 6920601},
    {"mode": "embedded", "path": "artifacts/0023/metrics.json"}
  ],
  "body": "## Hypothesis\nk=128 gives better coverage than k=64.\n"
}
```

---

## Feature Lineage

Features are living project entities stored in `features.json`. They group
related RT nodes and track the lifecycle of what is alive in the project.

### features.json

```json
{
  "next_id": 3,
  "features": [
    {
      "id": "f0001",
      "name": "Reinforcement Learning Bridge",
      "slug": "reinforcement-learning-bridge",
      "status": "active",
      "created_from": 41,
      "current_node": 68,
      "current_node_mode": "derived",
      "nodes": [
        {"node_id": 41, "role": "proposal"},
        {"node_id": 47, "role": "implementation"},
        {"node_id": 62, "role": "benchmark"},
        {"node_id": 68, "role": "fix"}
      ]
    }
  ]
}
```

### feature_edges.jsonl

```json
{"from":"f0002","to":"f0001","type":"collaborates_with","created_from":58}
{"from":"f0003","to":"f0001","type":"depends_on","created_from":61}
{"from":"f0008","to":"f0001","type":"supersedes","created_from":74}
```

### Feature status

| Status | Meaning |
|--------|---------|
| `active` | Feature is alive and maintained |
| `degraded` | Maintainer marked as degraded |
| `retired` | No longer in use |

### Derived health (computed, not stored)

| Health | Meaning |
|--------|---------|
| `clean` | No issues detected |
| `warning` | Collaborator degraded or benchmark poisoned |
| `degraded` | Implementation poisoned or depends_on degraded |
| `unmoored` | Edge has lost its evidence anchor |

### Edge types

| Type | Cycle | Propagation |
|------|-------|-------------|
| `depends_on` | Rejected | Degraded propagates |
| `collaborates_with` | Allowed | Warning only |
| `supersedes` | Rejected | Reports retirement candidate |

### Node roles within a feature

`proposal | implementation | experiment | benchmark | regression | fix | decision | documentation`

Only `implementation`, `fix`, and `decision` affect `current_node` derivation.
