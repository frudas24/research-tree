# CLI Reference

## Global flags

| Flag | Default | Description |
|------|---------|-------------|
| `--research-root` | `.research` | Path to research root |
| `--json` | false | Emit structured JSON output |
| `--color` | auto | Color mode: `always`, `never`, `auto` |

## Storage model

`rt init` defaults to binary storage for production. JSON remains available for
debugging, inspection, and git-friendly diffs.

```bash
rt init                              # binary format (default)
rt init --storage-format json        # JSON format
rt init --force                      # reinitialize and create emergency backup
```

Environment variables:

| Variable | Default | Description |
|----------|---------|-------------|
| `RESEARCH_ROOT` | unset | Full explicit root path |
| `RESEARCH_TREE_ROOT` | unset | Base directory; root becomes `$RESEARCH_TREE_ROOT/.research` |
| `RESEARCH_TREE_FORMAT` | `bin` | Storage format: `bin` or `json` |

## Node lifecycle

### `rt node create`

```bash
rt node create --title "Baseline"
rt node create --title "Child" --parents 1,2
rt node create --title "Child" --parents "Baseline"
rt node create --title "Scoped claim" --scope "mistral-q4km ctx=2048 greedy"
rt node create --title "Active task" --exit-criteria "close after 3 reproducible seeds"
rt node create --title "Run note" --body-file ./notes.md
rt node create --title "Done node" --status done --outcome success
rt node create --title "Candidate" --parents 1,2 --primary-parent 1
rt node create --title "Candidate" --relation compares_against:2
rt node create --title "Contaminated run" --evidence-status poisoned --evidence-cause base_snapshot --evidence-scope "qwen@remoto32d raw prompts"
```

- `--parents` accepts numeric IDs or unique title substrings.
- `--primary-parent` must reference one of the node's DAG parents.
- `--relation` adds typed cross-links such as `depends_on`, `compares_against`, `inspired_by`, and `aggregates`.
- `--edit` opens `$EDITOR` and takes precedence over `--body` / `--body-file`.
- `status=done` requires terminal outcome: `success`, `failure`, or `inconclusive`.

### `rt node show`

```bash
rt node show 1
rt node show 1 --view summary
rt node show 1 --agent
rt node show 1 --json
```

`--agent` renders a compact handoff-oriented projection for another agent.
Human views in `show`, `list`, and `tree` may append verdict badges such as
`[failure]`, `[inconclusive]`, or `[superseded]` to improve scanability.
`show` also renders `scope`, `exit criteria`, `continued by`, `superseded by`,
`primary parent`, typed `relations`, and the latest parsed `run-meta` block when present.
When available, the latest run is sourced from structured `runs` and then
rendered in human-readable form.

### `rt node edit`

```bash
rt node edit 4 --status paused
rt node edit 4 --claim-status validated
rt node edit 4 --scope "llama-8b q4_k_m ctx=4096"
rt node edit 4 --exit-criteria "close when throughput benchmark is replicated"
rt node edit 4 --parents 3
rt node edit 4 --continued-by 7
rt node edit 4 --superseded-by 9
rt node edit 4 --add-parents "Fallback parent"
rt node edit 4 --rm-parents 2
rt node edit 4 --add-tags "gpu,bench"
rt node edit 4 --rm-tags "draft"
rt node edit 4 --append-body "New note"
rt node edit 4 --relation compares_against:9
rt node edit 4 --add-relation inspired_by:7
rt node edit 4 --rm-relation inspired_by:7
rt node edit 4 --evidence-status suspect --evidence-cause prompt_surface
```

- `--parents` replaces the full parent set.
- `--add-parents` and `--rm-parents` perform atomic parent edits.
- `--continued-by` and `--superseded-by` add semantic continuity links distinct from DAG parents.
- `--relation` replaces the relation set; `--add-relation` / `--rm-relation` mutate it atomically.
- `--evidence-status` / `--evidence-cause` model evidence hygiene separately from truth/failure.
- if a node ends up with multiple structural parents and no `primary_parent`, CLI emits a warning and `rt doctor lineage` will flag it.
- Parent resolution accepts IDs or unique title substrings.
- Cycle creation is rejected.

### `rt node poison` / `rt node revalidate`

```bash
rt node poison 36 --cause base_snapshot --scope "qwen@remoto32d" --reason "base snapshot corrupted" --by 394
rt node revalidate 36 --by 394
```

Use these when the experiment happened but should not be treated as clean doctrine.

### `rt doctor lineage`

```bash
rt doctor lineage
rt doctor lineage --strict
```

This is the stricter architectural audit for lineage hygiene. It flags:

- nodes with multiple structural parents but no `primary_parent`
- nodes mixing multiple parents with matrix-style `relations`
- poisoned nodes still acting as structural ancestors
- revalidated nodes that still remain structural hubs

### `rt doctor evidence`

```bash
rt doctor evidence
```

This focuses on evidence hygiene rather than graph shape. It flags:

- poisoned nodes without clean reruns
- active nodes depending on poisoned ancestors
- doctrine/report nodes built on poisoned ancestors
- revalidated bookkeeping inconsistencies

### `rt node close`

Strict helper for terminal closure.

```bash
rt node close 42 --outcome success
rt node close 42 --outcome failure --append-body "OOM on layer 18"
```

Moving a node to `done` or `paused` releases all active resource leases held by that node.

### `rt node logrun`

Append normalized execution metadata and optionally attach the outdir as an artifact.
The command writes both:

- a structured `runs[]` entry
- optionally, with `--project-body`, a single latest markdown `run-meta` block in the body

```bash
rt resource claim 42 gpu-node-0 --by codex --note "benchmark lane"
rt node logrun 42 \
  --resource-id gpu-node-0 \
  --endpoint 10.0.0.14 \
  --endpoint-kind ip \
  --cmd "python train.py --seed 7" \
  --outdir /tmp/run_7 \
  --seed 7 \
  --eta 2h \
  --cost "$3" \
  --note "baseline"
rt node logrun 42 --cmd "python bench.py" --project-body
rt node logrun 42 --cmd "python bench.py" --valid=false --invalid-reason "wrong target"
```

`runs[]` is the authoritative store. `--project-body` is for cases where you
explicitly want the latest run mirrored into editorial markdown.

Rules:

- `resource_id` is the canonical hardware reference inside RT.
- `endpoint` is the technical run target and must be a real IP or DNS name.
- nicknames such as `gpu-node-0` belong in resource `label`, not in run `endpoint`, unless they are valid DNS in your environment.
- if the node has not claimed the resource, `logrun --resource-id ...` fails unless you explicitly pass `--allow-unleased-resource`.

### `rt node link`

Link commit and/or artifact metadata in one step.

```bash
rt node link 42 --commit auto --repo .
rt node link 42 --commit none --artifact /tmp/report.json --host gpu-node-0 --artifact-desc report
```

### `rt node delete`

```bash
rt node delete 7
rt node delete 7 --force
```

### `rt node list`

```bash
rt node list
rt node list --status active --agent researcher
rt node list --claim-status invalidated
rt node list --evidence-status poisoned
rt node list --evidence-cause base_snapshot
rt node list --outcome failure
rt node list --tag kd
rt node list --tags-all a,c
rt node list --tags-any kd,gpu
rt node list --title-contains "sparse"
rt node list --scope-contains "ctx=2048"
rt node list --body-contains "WebSocket"
rt node list --continued-by 42
rt node list --superseded-by 91
rt node list --has-artifact true
rt node list --sort-by modified --order desc --offset 10 --limit 20
rt node list --json
```

### `rt node history`

```bash
rt node history 5
rt node history 5 --json
```

### `rt node diff`

```bash
rt node diff 5 --rev-a 2
rt node diff 5 --rev-a 2 --rev-b 4
rt node diff 5 --rev-a 2 --rev-b 4 --json
```

`--rev-b` defaults to the current revision. The diff is semantic rather than
raw-file oriented: it compares structured node fields such as `status`,
`claim_status`, `scope`, `tags`, `artifacts`, `runs`, and `body`.

### `rt node ancestors` / `rt node descendants`

```bash
rt node ancestors 50
rt node descendants 4 --json
```

### `rt node invalidate`

```bash
rt node invalidate 2 --by 5 --reason "overfitting detected"
```

### `rt node import`

```bash
rt node import --file ./nodes.json
```

Expected payload:

```json
{"nodes":[{"title":"n1","status":"active","parents":[1]}]}
```

### `rt node batch`

```bash
rt node batch --filter-status active --set-status paused
rt node batch --filter-agent codex --set-claim-status validated
rt node batch --filter-tag draft --set-agent researcher
```

## Resource coordination

### `rt resource add`

```bash
rt resource add \
  --id gpu-node-0 \
  --label "GPU Node 0" \
  --kind gpu \
  --endpoint 10.0.0.14 \
  --endpoint-kind ip \
  --tags cuda,24gb,ubuntu22 \
  --os ubuntu22.04 \
  --cpu "EPYC 7543" \
  --ram-gb 256 \
  --gpu "RTX 4090" \
  --vram-gb 24
```

Resource identity model:

- `id`: stable canonical reference used by nodes and runs
- `label`: human-facing nickname
- `endpoint`: technical target used for connection or execution
- `endpoint_kind`: `none | ip | dns`

### `rt resource claim` / `rt resource release`

```bash
rt resource claim 42 gpu-node-0 --by codex --note "gemma baseline"
rt resource release 42 gpu-node-0
```

### `rt resource list`

```bash
rt resource list
rt resource list --free
rt resource list --used
rt resource list --kind gpu --tag cuda
rt resource list --json
```

### `rt resource show`

```bash
rt resource show gpu-node-0
rt resource show gpu-node-0 --json
```

Shows:

- inventory/spec metadata
- active leases
- recent lease events

### `rt resource history`

```bash
rt resource history gpu-node-0
rt resource history gpu-node-0 --limit 50
rt resource history gpu-node-0 --json
```

The history stream records:

- `claim`
- `release`
- `auto_release_done`
- `auto_release_paused`
- `auto_release_delete`

### `rt resource report`

```bash
rt resource report
rt resource report --json
```

The report shows which resources are free, used, under maintenance, or disabled,
and which nodes currently hold active leases.

Error semantics:

- `resource disabled`: inventory entry exists but is not schedulable
- `resource in maintenance`: temporarily unavailable by operator intent
- `resource busy`: active node leases block the operation, and the error names the blocking nodes

## Graph and status views

### `rt tree`

```bash
rt tree
rt tree 14
rt tree --depth 2
rt tree --status active
rt tree --show-relations
rt tree --flat
rt tree --json
```

`--show-relations` adds inline hints for non-structural typed relations in
expanded tree mode, without turning them into DAG edges.

### `rt links`

```bash
rt links
rt links --type parent
rt links --type compares_against --json
```

Shows a flat edge view across both DAG parent links and typed relations.

### `rt lint`

```bash
rt lint
rt lint --max-parents 4
rt lint --json
```

Audits graph hygiene, including:

- oversized parent fan-in
- invalid `primary_parent`
- orphaned or duplicate typed relations
- isolated nodes
- active invalidation branch warnings

### `rt status`

Scalable dashboard by default. Detailed dumps are opt-in.

```bash
rt status
rt status --agent researcher
rt status --mine
rt status --tag docker
rt status --scope-contains "ctx=2048"
rt status --matrix
rt status --section active,claim
rt status --verbose --limit 50
rt status --json
```

JSON is the stable automation contract. Existing top-level fields remain, while
new aggregations are additive:

- `status_counts`
- `claim_status_counts`
- `outcome_counts`
- `run_validity_counts`
- `matrix`
- `hotspot_formula`
- `hotspots`

Hotspots are deterministic rather than heuristic black boxes:

- `hotness = pending_children*5 + age_days + inconclusive_bonus`
- `inconclusive_bonus = 5` only when `outcome=inconclusive`

Text output shows the same breakdown inline, for example:

```text
Top Hotspots:
  formula: hotness = pending_children*5 + age_days + inconclusive_bonus (bonus=5 when outcome=inconclusive)
  0050 hot=25 pending=5*5 age=0d bonus=0 ...
```

### `rt mermaid`

```bash
rt mermaid
rt mermaid 14
rt mermaid --dir LR
```

`rt mermaid 14` exports only the reachable subtree rooted at node `14`.

### `rt changes`

```bash
rt changes
rt changes --since 48 --limit 30
rt changes --json
```

### `rt timeline`

```bash
rt timeline
rt timeline --days 30 --limit 100
rt timeline --json
```

### `rt feed`

```bash
rt feed
rt feed --by created --limit 30
rt feed --hours 24 --status done
rt feed --days 7 --agent codex --tag benchmark
rt feed --json
```

`feed` is the native chronological global view. Unlike `timeline`, it is not
grouped by day and can pivot on either `created` or `modified` timestamps.

## Artifacts, tags, alerts, recovery

### `rt artifact`

```bash
rt artifact add 1 --mode path --host gpu-node-0 --path /tmp/model.bin
rt artifact embed 1 --file ./metrics.json --desc "training metrics"
rt artifact rm 1 --host gpu-node-0 --path /tmp/model.bin
```

### `rt tag`

```bash
rt tag add 1 "kd,ml,experiment"
rt tag rm 1 "ml"
```

### `rt alert`

```bash
rt alert list
rt alert list --agent researcher --only-unacked
rt alert ack warn_1717123456_0003
```

### `rt recovery`

```bash
rt recovery list
rt recovery restore snapshot_2026...
```

### `rt storage`

```bash
rt storage migrate --to bin
rt storage migrate --to json
```

### `rt destroy`

```bash
rt destroy
rt destroy --yes
```

`init --force` and `destroy` create an emergency backup in `/tmp` before wiping.

## Agent consumption pattern

Prefer structured navigation over parsing human text:

```bash
rt status --json
rt node list --body-contains "Flask" --json
rt node ancestors 50 --json
rt node descendants 4 --json
rt node show 5 --json
rt node history 5 --json
```

Suggested pattern:

1. Scan with `status --json`.
2. Narrow with `node list --tag/--body-contains/...`.
3. Traverse with `ancestors` / `descendants`.
4. Deep-dive with `show` and `history`.

---

## Feature Lineage

Features are living project entities that span multiple RT nodes. While
nodes capture events, features capture what is alive right now.

```
Nodes    = events / evidence / decisions       (fossil record)
Features = living entities of the project      (organism)
Edges    = operational impact between features (nervous system)
```

### `rt feature`

```bash
rt feature create "Neural Network Spike" --from-node 0007
rt feature list
rt feature list --json
rt feature show f0001
rt feature show f0001 --json
rt feature timeline f0001
rt feature timeline f0001 --json
```

### `rt feature link`

```bash
rt feature link f0001 0047 --role implementation
rt feature link "RL Bridge" 0048 --role benchmark
```

Linking a node to a feature is idempotent: re-linking updates the role.
Nodes can belong to multiple features. Roles: `proposal`, `implementation`,
`experiment`, `benchmark`, `regression`, `fix`, `decision`, `documentation`.

`current_node` is derived from the latest linked node with role
`implementation`, `fix`, or `decision`. If it was set explicitly, later
feature links do not overwrite it. Use `--feature` and
`--feature-role` on `rt node create` to link at creation time:

```bash
rt node create --title "RL baseline" --feature f0001 --feature-role experiment
rt node create --title "RL bridge" --feature "RL Bridge" --create-feature --feature-role implementation
```

### `rt feature relate`

```bash
rt feature relate f0002 f0001 --type depends_on --from-node 0058
rt feature relate f0003 f0001 --type collaborates_with --from-node 0062
rt feature relate f0008 f0001 --type supersedes --from-node 0074
rt feature unrelate f0002 f0001 --type depends_on
rt feature edges f0001
rt feature edges f0001 --json
```

Edge types: `depends_on`, `collaborates_with`, `supersedes`.
`--from-node` is required and must reference an existing RT node documenting
the decision. Duplicate edges are rejected. `depends_on` and `supersedes`
cycles are rejected; `collaborates_with` cycles are allowed.

For `supersedes`: `from` is the replacement, `to` is the replaced.

### `rt feature doctor`

```bash
rt feature doctor f0001
rt feature doctor --all
rt feature doctor f0001 --json
```

Computes `derived_health` at read time (never persisted):

| Condition | Health |
|-----------|--------|
| Linked node impl/fix/decision/regression poisoned | `degraded` |
| Linked node benchmark/experiment poisoned | `warning` |
| transitively `depends_on` a degraded feature | `degraded` |
| directly `collaborates_with` a degraded feature | `warning` |
| Edge with missing `created_from` node | `unmoored` |
| Superseded by another feature | retirement candidate reported |

Severity order: `degraded > unmoored > warning > clean`.

If RT cannot read `feature_edges.jsonl` cleanly, `rt feature doctor` returns an
error instead of a partial health report.

### `rt feature impact`

```bash
rt feature impact f0001
rt feature impact f0001 --json
```

Shows which features depend on, collaborate with, or are depended upon by
this feature.

### `rt feature graph`

```bash
rt feature graph f0001
rt feature graph f0001 --json
```

Shows the immediate subgraph (nodes + edges) around a feature.

### Feature status

`active | degraded | retired` — set by the maintainer via Store/API helpers.
`derived_health` is
evidence-based and computed, not manual.

Feature slugs are unique. Lookup accepts ID (`f0001`), slug, or name.
`rt node create --feature "Name" --create-feature` auto-creates if missing.
