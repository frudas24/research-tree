# AGENT_INSTRUCTIONS.md — Canonical Runbook for Non-ABI Agents

This is the canonical operational guide for agents that interact with
`research-tree` through the CLI instead of the Go ABI.

Extended reference material lives in `docs/AGENT_INSTRUCTIONS.md`.

## 1) Mission

Keep research tracking reliable, reproducible, and auditable.
`rt` is not a note app: it is a DAG of claims, experiments, and outcomes.

## 2) Hard rules

1. **One concept per commit.**
2. **Never close a node without terminal outcome.**
   `status=done` requires `outcome in {success,failure,inconclusive}`.
3. **Always link evidence.**
   Capture command, resource, endpoint, outdir/artifacts, and commit hash.
4. **Claim hardware explicitly before using it.**
   Nodes do not auto-pick machines. Use `resource claim` first, then `logrun`.
5. **Keep DAG semantics clean.**
   Parent = causal or epistemic dependency, not loose association.
6. **No silent rewrites of meaning.**
   If intent changes, create a child node, reparent explicitly, or append a note.
7. **Prefer structured scans before deep dives.**
   Use `status`, `list`, `ancestors`, `descendants`, then `show`.

## 3) Start-of-task checklist

```bash
git pull --ff-only
go test ./...
./build/rt status || true
```

If `./build/rt` is missing:

```bash
go build -o build/rt ./cmd/rt
```

## 4) Fast commands to inspect health

```bash
rt status
rt status --matrix
rt status --section active,claim
rt tree --depth 3
rt changes --since 72
rt timeline --days 14
rt feed --by modified --hours 24
rt node diff 42 --rev-a 3
rt resource report
rt resource history gpu-node-0
```

For machine-readable scans:

```bash
rt status --json
rt node list --json --body-contains "keyword"
rt node ancestors 42 --json
rt node descendants 42 --json
rt node show 42 --json
```

## 5) Required workflow for experiments

### A. Create or attach node

```bash
rt node create --title "RXX: short objective" --parents <id> --tags a,b,c
```

### B. Log run metadata

```bash
rt resource claim <id> gpu-node-0 --by codex --note "gemma baseline"
rt node logrun <id> \
  --resource-id gpu-node-0 \
  --endpoint 10.0.0.14 \
  --endpoint-kind ip \
  --cmd "python train.py ..." \
  --outdir /tmp/run_foo \
  --seed 123 \
  --eta "2h" \
  --cost "cpu:8h" \
  --note "baseline sparse-k128"
```

By default this updates structured `runs[]` and artifacts only. Use
`--project-body` only when you intentionally want the latest run mirrored into
markdown body as a human-facing note.

Rules:

- `resource_id` is the canonical hardware reference inside RT.
- `label` is human-facing inventory text.
- `endpoint` is the technical run target and must be a real IP or DNS name.
- nicknames like `gpu-node-0` belong in `label`, not `endpoint`, unless they are actually valid DNS in your environment.
- if the node has not claimed the resource, `logrun --resource-id ...` fails unless you pass `--allow-unleased-resource`.

### C. Link commit and artifact

```bash
rt node link <id> --commit auto --artifact /tmp/run_foo --host gpu-node-0
```

### D. Close node with terminal outcome

```bash
rt node close <id> --outcome success --append-body "Key metric: +3.2% t/s"
```

When a node moves to `done` or `paused`, all active resource leases for that node are released automatically.
If a resource operation fails with `resource busy`, the error names the blocking nodes and holders; release the lease by resolving those nodes, not by bypassing the lease model.

## 6) Parenting and branch discipline

- Parent must answer: **what prior claim, run, or decision does this depend on?**
- Avoid attaching new execution work to broad executive/root nodes when a recent technical parent exists.
- Reparent explicitly when branch meaning changes:

```bash
rt node edit <id> --parents <new_parent_id>
```

- Use additive or subtractive parent edits when the node genuinely depends on multiple branches:

```bash
rt node edit <id> --add-parents <id>
rt node edit <id> --rm-parents <id>
```

Cycles are rejected automatically.

## 7) Validation gate before claiming “done”

For code changes:

```bash
go test ./...
make build
```

For CLI behavior changes, include at least:

- one happy-path command
- one negative-path command
- JSON mode sanity where applicable

## 8) Compact context handoff pattern

For another agent, keep the handoff narrow and explicit:

```bash
rt status --json | jq '{total, status_counts, claim_status_counts, outcome_counts, run_validity_counts, hotspots}'
rt node show <id> --agent --json
rt node diff <id> --rev-a <old_rev> --json
rt node ancestors <id> --json
rt node descendants <id> --json
```

If the node set is large, narrow first:

```bash
rt node list --status active --agent <agent> --json
rt node list --tag <tag> --json
rt node list --body-contains "<term>" --json
rt resource list --free --json
rt resource report --json
```

## 9) Current semantic model

These are the current first-class axes. Do not invent new meanings in free text
when one of these already applies.

- `status`: `active | done | paused`
- `outcome`: `unset | success | failure | inconclusive`
- `claim_status`: `provisional | validated | invalidated | superseded`
- `resource inventory`: explicit machine/GPU/cpu-slot inventory
- `leases`: active node→resource occupancy
- `resource history`: historical occupancy transitions for audit

Interpretation:

- `done + success`: the work completed and produced a positive result
- `done + failure`: the work completed and refuted or failed operationally
- `done + inconclusive`: the work completed but evidence is ambiguous
- `paused`: not finished; blocked or deferred
- `claim_status=superseded`: a conclusion was replaced by a later one
- resource release depends on node lifecycle, not experiment outcome

## 10) Known current limits

Current CLI and model still do **not yet** have a separate top-level axis for:

- invalid-run semantics separate from `status`, `outcome`, and `claim_status`

However, invalid-run semantics **are** first-class at the structured run level:

- `runs[]`
- `RunRecord.Valid`
- `RunRecord.InvalidReason`
- `rt node logrun --valid=false --invalid-reason ...`
- `rt status --json` → `run_validity_counts`

Design follow-up for any future top-level axis remains tracked in:

- internal design notes (not published in this repo)

Until then:

- prefer `--scope` for bounded claims
- prefer `--exit-criteria` for active work that needs a crisp closure condition
- use `logrun` for reproducibility
- rely on structured `runs` as the source of truth; only use `--project-body` when a human-facing markdown mirror is intentionally needed
- use `claim_status=superseded` and `--superseded-by` explicitly
- use `--continued-by` when the next operational node is already known
- use `rt node diff <id> --rev-a N [--rev-b M]` when you need audit-grade change inspection between revisions

Current scope-writing discipline:

- name the setup boundary in the title when it matters materially
- include model, dataset, decode mode, hardware target, or parameter regime in `--scope`
- avoid universal wording like "works" when the claim only holds for one configuration

## 11) Pitfalls to avoid

- Marking `done` with `outcome=unset`
- Logging runs only in chat and not in node body/artifacts
- Logging hardware runs without first claiming the resource
- Ambiguous parent title matching when numeric IDs are available
- Treating parent edges as “related to” instead of true dependency
- Writing broad claims without stating setup, model, dataset, or decode conditions

## 12) Reference

- Conceptual context: `AGENT.md`
- Extended agent guide: `docs/AGENT_INSTRUCTIONS.md`
- User docs: `README.md`, `docs/cli.md`
- Core model/store: `pkg/retree/`
- CLI implementation: `cmd/rt/cmds/`
