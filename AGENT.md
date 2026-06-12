# AGENT.md — Research Tree

## What this project is

A standalone tool for mapping scientific/research work as a directed acyclic
graph (DAG). Instead of linear logs, research-tree stores work as nodes
(ideas, experiments, decisions) connected by parent-child edges.

> Operational runbook for agents: see `AGENT_INSTRUCTIONS.md`.

It was born from tracking multi-branch ML research where sequential log files
became unsustainable for navigating branches, pivots, and dead ends.

## How to work here

1. **Understand the data model first.** Read `pkg/retree/types.go` and
   `pkg/retree/model_node.go` — they define the canonical node structure.

2. **Implementation order:** data model → storage → ABI → CLI → visualization.
   This preserves the ABI-first principle and keeps CLI/UI as consumers of
   `pkg/retree`.

3. **Commit discipline:** small, logical commits. One concept per commit.
   Write commit messages in English.

4. **Verification gate:** `go build ./... && go vet ./...` before claiming
   any work done. Add tests in `_test.go` files alongside the code.

5. **Tests are not optional.** Every feature has tests. Tests live next to
   the code they test (standard Go convention).

## Project conventions

- **Language:** Go 1.21+ (no external dependencies beyond cobra; `yaml.v3`
  only if we keep a legacy Markdown importer).
- **Package:** `github.com/frudas24/research-tree/pkg/retree` is the public API.
- **CLI:** `cmd/rt/main.go` uses cobra for subcommands.
- **Storage:** `.research/` directory with `meta.json` (schema + format),
  `nodes.*` (json or binary codec), edge index, rolling history snapshots
  (last 3), `next_id`, and `lock` for concurrency.
- **Node format:** canonical struct (`json` for debug, binary codec for prod).
  Markdown frontmatter is optional/import-only.
- **Only required field:** `title`. Everything else (parents, status, tags,
  artifacts, commits) can be added later.
- **DAG invariant:** no write operation may introduce cycles.
- **Thread safety:** writes use lockfile + bounded retries + stale-lock takeover.
  Reads are lock-free.

## Reference

- `docs/` — architecture, CLI reference, data model, and development guides.
- `pkg/retree/` — core library: types, store, graph operations, and codecs.
- `cmd/rt/` — CLI implementation using cobra.
