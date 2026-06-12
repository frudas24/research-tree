# E2E Simulator

Comprehensive integration test (`store_e2e_test.go`) that simulates a realistic ML research
workflow and produces an auditable JSON artifact.

## Running

```bash
go test -v -run TestE2ESimulator ./pkg/retree/ -count=1
```

## Scenarios (16)

| # | Scenario | What it validates |
|---|----------|-------------------|
| S01 | Init + basic CRUD | Store creation, node creation, update with commits/body |
| S02 | Graph queries | GetRoots, GetChildren, GetAncestors, GetDescendants, GetLeaves |
| S03 | Diamond dependency | Node with 2 parents (semantic merge) |
| S04 | Tags + artifacts | AddTags/RemoveTags idempotency, AddArtifact, EmbedArtifact |
| S05 | Filtering | Status, Agent, TitleContains, Tag, Limit, GetActiveAgents, QueryNodes |
| S06 | Invalidation cascade | InvalidateClaim, warnings, AckBranchWarning, idempotency |
| S07 | Cycle prevention | Update creates cycle → error, missing parent → error |
| S08 | Delete + force | Without force fails with children, force orphans children |
| S09 | Storage migration | JSON→BIN→JSON roundtrip with data integrity |
| S10 | Snapshots + recovery | ListSnapshots, retention ≤3, RestoreSnapshot functional |
| S11 | Concurrency | 10 goroutines creating nodes, no ID collisions |
| S12 | Edge cases | Unicode, empty store operations |
| S13 | Agent resolution | ResolveAgentName for known and unknown agents |
| S14 | JSON↔BIN equivalence | Field-level comparison via json.Marshal, double roundtrip |
| S15 | Native BIN format | Init with BIN, full CRUD and graph operations |
| S16 | DAG invariants | 8 checks: ID uniqueness, referential integrity, cycle-free, edge index, schema, claim, ValidateNode |

## Invariants checked (S16)

1. **ID uniqueness** — no duplicate NodeIDs
2. **ID monotonicity** — all IDs < NextID
3. **Parent referential integrity** — every parent reference resolves to an existing node
4. **Cycle-free** — WouldCreateCycle returns false for all existing edges
5. **Edge index consistency** — edges.jsonl matches node parents exactly
6. **Schema version** — all nodes have CurrentSchemaVersion
7. **Claim status** — invalidated nodes have non-empty invalidated_by
8. **Per-node validation** — ValidateNode passes for every node

## Audit artifact

The test writes `e2e_audit_report.json` (~6KB) containing:

```json
{
  "generated_at": "2026-05-31T...",
  "scenarios": [
    {"name": "S01: init + basic CRUD", "passed": true, "duration_ms": 5},
    ...
  ],
  "summary": {
    "total": 16,
    "passed": 16,
    "failed": 0,
    "final_node_count": 25
  },
  "final_graph_state": {
    "roots": [1],
    "leaves": [4, 5, ...],
    "active_nodes": [...],
    "pending_warnings": 0
  }
}
```
