# Testing

## Running tests

```bash
# All tests
make test

# Race detector
make test-race

# Specific package
go test -v ./pkg/retree/ -count=1

# Specific test
go test -v -run TestE2ESimulator ./pkg/retree/ -count=1

# CLI tests
go test -v ./cmd/rt/cmds/ -count=1
```

## Test structure

```
pkg/retree/
├── model_node_test.go        # Node validation, defaults, deep copy, JSON codec
├── graph_memory_test.go      # DAG operations, cycle detection, traversal, filtering
├── store_integration_test.go # CRUD flows, concurrency, snapshots, migration, filters
├── store_abi_test.go         # Public ABI: open queries, tags, embed artifact
├── codec_bin_test.go         # Binary codec: roundtrips, enums, errors, header
└── store_e2e_test.go         # E2E simulator: 16 scenarios with invariants

cmd/rt/cmds/
└── root_test.go              # CLI integration: init, CRUD, tree modes, concurrency
```

## Test patterns

### Integration tests (`store_integration_test.go`)

Use `t.TempDir()` to isolate the filesystem. Each test initializes
a fresh store with `mustInit`. Cover the public contract of the Store.

### ABI tests (`store_abi_test.go`)

Validate that the public ABI works from the perspective of an external
consumer: `Init` → `Open` → queries → filters.

### Unit tests (`model_node_test.go`, `graph_memory_test.go`, `codec_bin_test.go`)

Testean componentes individuales sin dependencia de disco.

### CLI tests (`root_test.go`)

Ejecutan el comando Cobra completo via `root.Execute()` con buffer de salida
capturado. Validan flags, JSON output, y flujos multi-comando.

### E2E simulator (`store_e2e_test.go`)

Simulates a real research workflow with 16 sequential scenarios that
share state. Each scenario produces pass/fail. At the end, generates an
auditable JSON artifact. See [e2e-simulator.md](e2e-simulator.md).

## Concurrency tests

Both `store_integration_test.go` and `root_test.go` include concurrency tests:
multiple goroutines creating nodes simultaneously, verifying
no ID collisions occur.

## What is tested

| Area | Coverage |
|------|----------|
| Node validation | All rules, edge cases |
| DAG operations | Add, remove, update, cycle detection, traversal |
| Storage CRUD | Create, get, update, delete, concurrency |
| Filters | Status, claim, tag, agent, title, limit, combined |
| Tags | Add, remove, idempotency, dedup |
| Artifacts | Path mode, embed, disk persistence |
| Claims | Invalidate, warnings, ack, idempotency |
| Snapshots | Creation, retention, restore |
| Migration | JSON→BIN→JSON roundtrip |
| Binary codec | All fields, enums, errors, header |
| CLI | All commands, all flags, JSON output |
| Unicode | Titles, tags, bodies |
| Empty store | Edge cases (0 nodes) |
| Invariants | ID uniqueness, referential integrity, cycles, edge consistency |
