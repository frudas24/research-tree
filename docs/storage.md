# Storage Engine

## Overview

All persistence is file-based within `.research/`. No external database. No network dependencies.

## Path resolution

| Variable | Default | Override |
|----------|---------|----------|
| Research root | `.research/` | `--research-root`, or `RESEARCH_ROOT`, or `RESEARCH_TREE_ROOT/.research` |

## File layout

```
.research/
├── meta.json              # Store metadata
├── nodes/                 # JSON mode: one file per node
│   ├── 0001.json
│   └── 0002.json
├── nodes.bin              # Binary mode: RTND header + concatenated nodes
├── nodes.idx              # Direct access index
├── edges.jsonl            # Edges in JSONL format
├── next_id                # Atomic counter
├── lock                   # Lockfile for writes
├── snapshots/             # Recovery snapshots
│   ├── snapshot_*.tar.gz
│   └── manifest.json
├── history/               # Per-node history (previous revisions)
│   └── nodes/
│       └── NNNN/
│           └── revNNNN_timestamp.{json|bin}
├── alerts.jsonl           # Invalidation warnings
├── agents.json            # Agent registry
└── artifacts/             # Embedded files
    └── 0001/
        └── metrics.json
```

## JSON format

Each node is stored as `nodes/NNNN.json` with readable indentation. Ideal
for debugging, human inspection, and version control (git diff friendly).

Edges are stored in `edges.jsonl`:
```jsonl
{"from":1,"to":2}
{"from":1,"to":3}
```

## Binary format

See [binary-codec.md](binary-codec.md) for complete specification.

`nodes.bin` file with `RTND` header + version. Each node encoded in
binary format with length prefixes. Index in `nodes.idx` for direct
access by NodeID with CRC32 checksum.

## Lockfile

- **Acquisition:** `O_CREATE | O_EXCL` (kernel-level atomic)
- **Retry:** 100ms interval, 10s timeout
- **Stale takeover:** locks > 30s are considered orphaned and reclaimed
- **Content:** PID, host, timestamp, operation, owner

## Snapshots

- Automatically created after each mutation (create, update, delete, invalidate, migrate)
- Format: `tar.gz` of the `.research/` directory (excluding `snapshots/`)
- SHA-256 hash for integrity verification
- Retention: last 3 snapshots, older ones are deleted
- Restore: decompresses snapshot to temporary directory, replaces root content

## Atomicity

All writes follow the pattern:
1. Write to `.tmp`
2. `os.Rename(tmp, final)` — atomic on the same filesystem

This applies to: `meta.json`, `nodes.bin`, `nodes.idx`, `edges.jsonl`,
`next_id`, `alerts.jsonl`, `agents.json`, `manifest.json`, and individual
JSON node files.

## Edge index

`edges.jsonl` is a derived index that regenerates from the graph on each
`persistGraph`. Can be manually rebuilt with `s.RegenerateEdges()`.
Each line is a JSON object with `from` and `to` (NodeID).
