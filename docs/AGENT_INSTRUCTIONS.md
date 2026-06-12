# Research Tree — Agent Instructions

Canonical non-ABI runbook: `../AGENT_INSTRUCTIONS.md`.
This file is the extended reference with extra examples and migration guidance.

## Quick reference

```bash
rt status --json | jq '.active[] | {id, title, parents, children}'
rt status --json | jq '{total, status_counts, claim_status_counts, outcome_counts, run_validity_counts, hotspots}'
rt status --section done --limit 20
rt status --matrix
rt node ancestors 50 --json
rt node descendants 4 --json
rt node show 5 --json
rt node show 5 --agent --json
rt resource list --free --json
rt resource report --json
```

## Workflow for migrating research logs

### Phase 1: Initialize

```bash
rt init --force                    # fresh start (binary format, default)
rt init --force --storage-format json  # debug mode (human-readable)
```

Binary format is the default (2.6x smaller). Use `RESEARCH_TREE_FORMAT=json`
for human inspection or git-friendly diffs.

**Path resolution:** each project gets its own `.research/` directory.
Default is `$CWD/.research` (the repo you're working in).
Set `RESEARCH_TREE_ROOT=~/.agent` for a global cross-project tree.

### Phase 2: Create nodes block by block

Read one logical section of your research log at a time. Each block becomes one node.

```bash
# Root node (project context)
rt node create \
  --title "Project Setup — Baseline" \
  --agent researcher --tags "baseline,setup" --status done \
  --body '## Scope
Project: my-ml-project, Focus: initial baseline.
Remotes: gpu-node-0 (32GB), gpu-node-1 (64GB).'

# Child node
rt node create \
  --title "Critical Guardrails: No-Op, RAM, Greedy Split" \
  --parents "Project Setup" \
  --agent researcher --tags "guardrails" --status done \
  --body '## No-Op Guard
If emit_count==0 → NO-OP.'

# Experiment node with artifacts
rt node create \
  --title "Model partial strip: f=0.75, 0.78, 0.80" \
  --parents "Baseline" \
  --agent researcher --tags "experiment,model" --status active \
  --body '## Results
| f    | ctx1024 | ctx2048 |
|------|---------|---------|
| 0.75 | +0.24%  | +0.26%  |
| 0.78 | +0.27%  | +0.14%  |'

rt artifact add 5 --mode path --host gpu-node-0 --path "calibration/run_f078" --desc "best result"
```

When work moves from planning to actual hardware occupancy, claim the resource explicitly:

```bash
rt resource add --id gpu-node-0 --label "GPU Node 0" --kind gpu --endpoint 10.0.0.14 --endpoint-kind ip
rt resource claim 5 gpu-node-0 --by researcher --note "model run"
rt node logrun 5 --resource-id gpu-node-0 --endpoint 10.0.0.14 --endpoint-kind ip --cmd "python train.py ..."
rt resource history gpu-node-0 --json
```

### Phase 3: Mark progress as you work

As your research log moves forward, update node statuses:

```bash
rt node edit 4 --status done --outcome failure
rt node edit 5 --status done --claim-status validated
rt node edit 8 --status active
```

### Phase 4: Verify the tree

```bash
rt tree                              # visual DAG
rt status --mine                     # your active work
rt status                            # aggregate dashboard (scales to large trees)
rt status --verbose --limit 50       # detailed rows, bounded
rt status --section claim,hotspots   # focus specific sections
rt status --tag "mistral"            # scope by tag
rt node ancestors N --json           # trace lineage
```

## Key concepts

| Concept | How to use |
|---------|-----------|
| **Parents by title** | `--parents "SeedDelta"` matches by title substring, not numeric ID |
| **Reparent node** | `rt node edit <id> --parents "NewParent"` — move node between subtrees. Cycles rejected automatically with `cycle detected`. |
| **Body inline** | `--body 'markdown\ncontent'` — no temp files needed |
| **Artifacts** | Link external files (calibration outputs, model paths) to nodes |
| **Resources** | Explicit inventory of machines/GPUs/cpu-slots with stable `id`, human `label`, and technical `endpoint` |
| **Leases** | Active node→resource occupancy; released automatically on `done` or `paused` |
| **Tags** | Categorize: `seeddelta`, `experiment`, `guardrails`, `decision` |
| **Status flow** | active → done/paused |
| **Outcome** | unset/success/failure/inconclusive (typically meaningful with `status=done`) |
| **Claim status** | provisional → validated/invalidated/superseded |
| **Revision history** | Every edit saves the previous version. `rt node history N` |

## Status dashboard at scale

`rt status` now defaults to an aggregate dashboard designed for large trees.
Detailed node dumps are opt-in.

| Need | Command |
|------|---------|
| Fast summary (default) | `rt status` |
| Full detailed lists | `rt status --verbose` |
| Bounded detail | `rt status --verbose --limit 100` |
| Focus one section | `rt status --section active` |
| Show matrix status×outcome | `rt status --matrix` |
| Filter by tag | `rt status --tag docker` |
| Filter by agent | `rt status --agent researcher` / `rt status --mine` |

`--json` remains backward compatible and now includes additive fields:
`status_counts`, `claim_status_counts`, `outcome_counts`, `matrix`, `hotspots`.

## Status lifecycle

```
active ▶ — working on this
  → done ✔ — completed
    → done + success ✓ — worked
    → done + failure ✗ — did not work
    → done + inconclusive ? — ambiguous results
  → paused ⏸ — on hold
```

## Decision rules for migration

1. **Each logical section of the research log = one node.** Don't split paragraphs arbitrarily.
2. **Connect related concepts with parents.** If section B builds on section A, B is a child of A.
3. **Use tags for cross-cutting concerns.** `seeddelta`, `mistral`, `gemma`, `h2` etc.
4. **Mark as done what's confirmed, active what's in progress.** Be honest about status.
5. **Add artifacts for concrete outputs.** Calibration paths, model files, experiment directories.
6. **Don't skip data.** Every significant fact from the research log goes into a node body.
7. **Use `--body` inline for short bodies, `--edit` for long ones.**

## When to use each flag

| Situation | Flag |
|-----------|------|
| Short body (1-5 lines) | `--body 'content'` |
| Long body (multiple paragraphs) | `--edit` (opens $EDITOR) |
| Body from existing file | `--body-file path.md` |
| Parent known by title keyword | `--parents "keyword"` |
| Parent known by ID | `--parents 1,2,3` |
| Replace entire body on edit | `rt node edit N --body 'new'` |
| Append to body | `rt node edit N --append-body 'more'` |

## LLM consumption pattern

```bash
# 1. Scan
rt status --json | jq '{total, status_counts, claim_status_counts, outcome_counts, hotspots}'
rt status --json | jq '{active: [.active[] | {id,title}], done: [.done[] | {id,title,outcome}]}'

# 2. Navigate
rt node ancestors <id> --json
rt node descendants <id> --json

# 3. Deep dive
rt node show <id> --json | jq '{title, status, body, artifacts, revision, parents}'
```
