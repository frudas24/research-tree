# Architecture

## Design principles

1. **Standalone.** Does not depend on git or external services. If git exists, it uses it
   for metadata; if not, works the same.
2. **Minimalism in writes.** Creating a node requires only `title`. Everything else
   is optional.
3. **ABI-first.** The primary interface is programmatic (`pkg/retree`). CLI and
   UI are consumers of that ABI.
4. **Concurrent and durable.** Atomic writes, lockfile with timeout and
   recovery. Lock-free reads.
5. **Dual codec.** JSON for debug/human inspection. Binary for production.
6. **Strict DAG.** No write operation may introduce cycles.
7. **Impact alerts.** If an ancestor is invalidated, agents with active descendant
   branches receive explicit warning.

## Project structure

```
research-tree/
├── cmd/rt/                  # CLI entry point
│   ├── main.go              # Entry point
│   └── cmds/                # Cobra subcommands
│       ├── root.go          # Root command + global flags
│       ├── init_cmd.go      # rt init
│       ├── node_cmd.go      # rt node {create,show,edit,delete,list,invalidate}
│       ├── tree_cmd.go      # rt tree
│       ├── status_cmd.go    # rt status
│       ├── artifact_cmd.go  # rt artifact {add,embed}
│       ├── tag_cmd.go       # rt tag {add,rm}
│       ├── alert_cmd.go     # rt alert {list,ack}
│       ├── storage_cmd.go   # rt storage migrate
│       ├── recovery_cmd.go  # rt recovery {list,restore}
│       ├── helpers.go       # ID parsing, CSV, body
│       └── root_test.go     # CLI integration tests
├── pkg/retree/              # Core library (public ABI)
│   ├── types.go             # Node, enums, Filter, BranchWarning
│   ├── model_node.go        # ValidateNode, ApplyNodeDefaults, CloneNode
│   ├── codec_json.go        # JSON marshal/unmarshal
│   ├── codec_bin.go         # Binary marshal/unmarshal (RTND v1)
│   ├── graph_memory.go      # In-memory DAG with cycle detection
│   ├── errors.go            # Sentinel errors
│   ├── store.go             # Public Store API
│   ├── store_meta.go        # Init, Open, meta.json, next_id
│   ├── store_nodes.go       # Load/persist JSON and BIN, edges index
│   ├── store_lock.go        # Lockfile with retry + stale takeover
│   ├── store_ops.go         # createNode, updateNode, deleteNode, migrate, embed, invalidate
│   ├── store_snapshot.go    # Snapshot tar.gz, retention, restore
│   ├── store_alerts.go      # Branch warnings (append-only JSONL)
│   ├── store_paths.go       # Path helpers for .research/ layout
│   ├── store_utils.go       # Filters, string/ID utilities
│   ├── *_test.go            # Unit + integration + E2E tests
│   └── store_e2e_test.go    # E2E simulator (16 scenarios)
├── third_party/
│   ├── commentlint/         # Doc comment linter (in-tree)
│   └── golangci-lint/       # golangci-lint reference
├── docs/                    # Documentation
├── TODO/                    # Historical implementation specs (0000–0005)
├── Makefile                 # Build pipeline
├── .golangci.yml            # Linter configuration
└── go.mod / go.sum
```

## Data flow

```
CLI (cmd/rt) ──► Store (pkg/retree) ──► Graph (in-memory) ──► Disk (.research/)
                      │                        │
                      ▼                        ▼
                 Lockfile               Cycle detection
                 Snapshots              Edge validation
                 Alerts                 Invariant checks
```

## Storage layout

```
.research/
├── meta.json              # schema_version, storage_format, created_at
├── nodes/                 # JSON mode: 0001.json, 0002.json...
├── nodes.bin              # Binary mode: RTND header + encoded nodes
├── nodes.idx              # Index: {NodeID: {offset, length, CRC32}}
├── edges.jsonl            # Edge index
├── next_id                # Atomic ID counter
├── lock                   # Lockfile (PID + timestamp + operation)
├── snapshots/             # Snapshot archives + manifest
│   ├── snapshot_*.tar.gz
│   └── manifest.json
├── history/               # Per-node revision history
│   └── nodes/
│       └── NNNN/
│           └── revNNNN_timestamp.{json|bin}
├── alerts.jsonl           # Branch warnings (append-only)
├── agents.json            # Agent registry
└── artifacts/             # Embedded artifacts
```

## Concurrency model

- **Writes:** lockfile (O_EXCL) + bounded retry (100ms × 10s timeout) + stale takeover (30s)
- **Reads:** lock-free
- **Atomicity:** write to `.tmp` → `os.Rename` for all persistent state
- **Snapshots:** automatic tar.gz after each mutation, rolling retention of 3
