# Research Tree — Documentation

Standalone tool for mapping scientific and research work as a directed acyclic
graph (DAG). Each node represents a unit of research
(idea, experiment, decision) connected by parentage edges.

## Quick Start

```bash
# Initialize a research root
rt init

# Create nodes
rt node create --title "KD baseline" --agent researcher --tags kd,baseline
rt node create --title "KD sparse experiment" --parents 1 --agent researcher

# View the tree
rt tree

# Dashboard
rt status

# Filter
rt node list --status active --agent researcher
```

## Documentation by domain

| Document | Domain |
|-----------|--------|
| [architecture.md](architecture.md) | Architecture, design principles, project structure |
| [abi.md](abi.md) | Public API of `pkg/retree` — all Store operations |
| [data-model.md](data-model.md) | Nodes, enums, types, filters, validation |
| [storage.md](storage.md) | Storage engine, formats, lockfile, snapshots |
| [binary-codec.md](binary-codec.md) | Binary format specification (RTND v1) |
| [cli.md](cli.md) | Complete CLI command reference |
| [testing.md](testing.md) | How to run tests, test structure |
| [e2e-simulator.md](e2e-simulator.md) | E2E simulator: scenarios, invariants, auditable artifact |
| [development.md](development.md) | Development workflow, Makefile, conventions |

## Requirements

- Go 1.21+
- No external dependencies (Cobra only for CLI)


## Release policy

Research Tree is published as-is, with no guaranteed support commitment. See `../SUPPORT.md`.
