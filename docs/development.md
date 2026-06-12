# Development

## Quick start

```bash
# Clone
git clone https://gitlab.com/ai_world/agent/research-tree.git
cd research-tree

# Build (runs fmt → vet → tidy → commentlint → compile)
make build

# Run tests
make test

# Full pipeline
make build && make test
```

## Makefile targets

| Target | Description |
|--------|-------------|
| `make build` | Run all checks + compile binary to `build/rt` |
| `make check` | Run fmt + vet + tidy + commentlint + lint (no build) |
| `make fmt` | gofmt -s on all Go sources |
| `make vet` | go vet ./... |
| `make tidy` | go mod tidy + go mod verify |
| `make commentlint` | Enforce doc comments on all functions |
| `make lint` | golangci-lint (requires installation) |
| `make test` | go test -count=1 ./... |
| `make test-race` | go test -race -count=1 ./... |
| `make tools` | Install golangci-lint |
| `make clean` | Remove build/ directory |
| `make help` | List all targets |

## Build pipeline order

```
make build:
  1. gofmt -s (format all sources)
  2. go fmt ./... (format imports)
  3. go vet ./... (static analysis)
  4. go mod tidy + verify (module hygiene)
  5. commentlint (doc comments on all functions)
  6. golangci-lint (optional, warns if not installed)
  7. go build -gcflags 'all=-e' (compile, show all errors)
```

## Code conventions

- **Language:** Go (see go.mod for version)
- **Package:** `github.com/frudas24/research-tree/pkg/retree` is the public API
- **CLI:** `cmd/rt/main.go` uses Cobra
- **Tests:** `_test.go` files alongside the code (standard Go convention)
- **Doc comments:** Every function must have a doc comment (enforced by commentlint)
- **No external dependencies** beyond Cobra (and yaml.v3 for commentlint)
- **Zero-allocation** binary codec using only `encoding/binary`

## Commit discipline

- Small, logical commits — one concept per commit
- Commit messages in English
- Run `make build` before pushing

## Implementation history

| Phase | Commit | Description |
|-------|--------|-------------|
| 0001 | — | Data model: Node, enums, validation, DAG graph |
| 0002 | — | Storage engine: Init/Open, lockfile, snapshots, JSON/BIN |
| 0004 | — | Public ABI: Store API with integration tests |
| 0003 | — | CLI: Cobra commands with E2E tests |
| 0005 | — | Visualization: tree view, status dashboard |
| — | — | Binary codec: true RTND v1 format |
| — | — | E2E simulator: 16 scenarios with invariant validation |
