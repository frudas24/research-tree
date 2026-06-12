# commentlint

Simple Go linter that requires a documentation comment for **all functions** (exported or not). Designed to integrate with `golangci-lint`, but in this repository it runs as an independent step (`make commentlint`).

## Installation / prerequisites

- Go 1.24 or higher.
- This directory already contains the code; it requires no prior build (runs via `go run`).

## Usage

```bash
# check entire repo (respecting .golangci.yml)
go run ./third_party/commentlint ./...

# check only synapse packages
make commentlint internal/synapse/...
```

By default, all packages are checked (`./...`). Exclusions and limits defined in `.golangci.yml` are respected.

## Integration with `.golangci.yml`

`commentlint` reads the `issues` section to replicate GolangCI behavior:

```yaml
issues:
  max-issues-per-linter: 10   # report limit (0 = unlimited)
  exclude-dirs:
    - dist
    - vendor
    - internal/rpc/proto
  exclude-files:
    - ".*_grpc\\.pb\\.go"
    - ".*\\.pb\\.go"
```

If the file does not exist, default values are used (no exclusions or limit). Paths are considered relative to the repo root.

## Integration with the workflow

- The `make commentlint` target runs the linter before the usual lint.
- `make build` does not invoke it yet (to avoid noise). If you want to make it mandatory, add `commentlint` as a dependency of the `lint` or `build` target.

## Known limitations

- Not yet integrated as a real `golangci-lint` plugin (runs as a separate command).
- Does not ignore generated functions unless the file contains the standard header `// Code generated... DO NOT EDIT.`
