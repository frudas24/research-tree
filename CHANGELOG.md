# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Changed
- FFI bindings consolidated into `third_party/retree-bridge/` and regenerated to
  cover the full bridge ABI (resources, feature lineage, recovery, history, migration).
- JSON persistence is delta-based: only dirty nodes are written, removing the
  delete-all-then-rewrite crash window.
- `getNode` reads single nodes directly (JSON file or BIN index + CRC) instead of
  scanning the whole store.
- research-graph reports storage errors as 500, distinguishes them from 404, and
  shares the hotspot formula with the status dashboard.
- Added `rt storage reindex` to rebuild `nodes.idx` from `nodes.bin`.

### Fixed
- CLI boolean flags (`--has-artifact`) reject unknown values instead of silently
  coercing them to `false` and inverting the filter.
- `$EDITOR` values with arguments (e.g. `code --wait`) resolve to binary plus flags.
- Binary codec fails on strings exceeding the u16/u32 length prefix instead of
  silently truncating them on write.
- Warning IDs are unique even when two invalidations hit the same node in the same
  second; acknowledging one no longer acknowledges the other.
- Runs persist a canonical `endpoint_kind` (`none` instead of `""`); an endpoint
  supplied without a kind is rejected.
- Node history written before a storage-format migration remains readable.
- A missing `nodes.idx` fails loudly instead of silently presenting an empty graph
  that the next write would destroy.

### Security
- Snapshot restore rejects archive entries with `..` or absolute paths and skips
  symlinks, closing a tar-slip that could write outside the extraction root.
- research-graph binds to `127.0.0.1` by default instead of all interfaces.

### Docs
- Go version requirements aligned to 1.24; storage persistence trade-offs documented.
- commentlint gate is green (missing doc comments and formatting completed).

## [v0.3.3] - 2026-07-21

### Added
- `research-graph`: real-time Cytoscape DAG visualizer with node detail panel,
  filters, and search-to-node.
- Feature lineage (M1–M4): feature registry, typed feature edges, timeline,
  doctor, impact, and graph views.
- Feature lineage exposed through the C FFI bridge.

### Changed
- research-graph UI: enriched node detail panel, working filters, layout fixes.
- GitHub Actions updated for Node 24.

## [v0.3.2] - 2026-06-30

- Re-tag of v0.3.1 (points to the same commit); no changes.

## [v0.3.1] - 2026-06-30

### Added
- Evidence hygiene workflow: poison/revalidate nodes, evidence doctor, evidence
  cause and scope.
- Lineage doctor for structural parent hygiene; warnings on multi-parent nodes
  without a primary parent.
- Relation hints in the expanded tree view.

### Changed
- Binary decoder forward-compat for legacy payloads.

### Docs
- README polished for the HN launch; CLI screenshot and agent integration guide added.

## [v0.3.0] - 2026-06-15

### Added
- Typed relations (`depends_on`, `compares_against`, `inspired_by`, `aggregates`),
  primary parent, and `rt links` / `rt lint` commands.

## [v0.2.1] - 2026-06-12

### Fixed
- GoReleaser archives allow a different binary count when the FFI bridge is included.

## [v0.2.0] - 2026-06-12

### Changed
- golangci-lint enforced in `make build`; remaining lint issues resolved.
- Simplified GoReleaser configuration (separate CLI and bridge archives, single
  archive per platform).

## [v0.1.1] - 2026-06-12

### Changed
- Resolved all golangci-lint errors across the codebase.

## [v0.1.0] - 2026-06-12

### Added
- Initial release: DAG-based research tracking as a standalone Go tool.
  - CLI (`rt`): init, node CRUD, tree, status, artifacts, tags, recovery, alerts.
  - Storage in `.research/`: JSON and binary codecs, lockfile, per-node history,
    snapshots with retention.
  - C FFI bridge (`libretree.so` / Windows DLL) and GoReleaser packaging.

[Unreleased]: https://github.com/frudas24/research-tree/compare/v0.3.3...HEAD
[v0.3.3]: https://github.com/frudas24/research-tree/compare/v0.3.2...v0.3.3
[v0.3.2]: https://github.com/frudas24/research-tree/compare/v0.3.1...v0.3.2
[v0.3.1]: https://github.com/frudas24/research-tree/compare/v0.3.0...v0.3.1
[v0.3.0]: https://github.com/frudas24/research-tree/compare/v0.2.1...v0.3.0
[v0.2.1]: https://github.com/frudas24/research-tree/compare/v0.2.0...v0.2.1
[v0.2.0]: https://github.com/frudas24/research-tree/compare/v0.1.1...v0.2.0
[v0.1.1]: https://github.com/frudas24/research-tree/compare/v0.1.0...v0.1.1
[v0.1.0]: https://github.com/frudas24/research-tree/releases/tag/v0.1.0
