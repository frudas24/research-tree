# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [v0.4.4] - 2026-08-13

### Fixed
- `rt storage migrate` is now transparent on legacy stores: historical
  `done + outcome=unset` nodes survive format migration instead of
  hard-failing the command, so `rt storage repair-outcomes` remains the
  documented follow-up.
- `rt storage migrate` now accepts nodes that reference a parent with a
  higher ID; parent-existence is validated after graph assembly so insertion
  order (ascending node ID) cannot reject legal edges.

### Tests
- Added CLI and store-level regression tests covering transparent legacy
  migration (json→bin and bin→json round trips) and higher-ID parent edges.

## [v0.4.3] - 2026-08-13

### Fixed
- Legacy outcome repair now preflights the full store with sidecar auditing
  while tolerating only historical `done + outcome=unset` nodes, so corrupt
  sidecars abort before any mutation or snapshot is written.
- Restoring `repair_legacy_outcomes_pre` snapshots now succeeds even though
  they intentionally preserve legacy `done + outcome=unset` history for
  recovery.

### Changed
- `StatusSummary` documentation now explicitly matches the compatibility
  contract: legacy `active/done/paused` arrays still include umbrellas, while
  work-only metrics live in `WorkStatusCounts`, `Hotspots`, and `Umbrella*`
  projections.
- README now documents the operational rule that restoring a legacy snapshot
  requires rerunning `rt storage repair-outcomes` before strict commands such
  as `rt status` will accept the store again.

## [v0.4.2] - 2026-08-12

### Fixed
- Store locks now carry an owner token and refresh/release only while they
  still own that token, preventing a resumed stale owner from overwriting or
  deleting a newer lock holder.
- Writers in the same process are additionally serialized per research root,
  closing intra-process races around lock handoff under heavy concurrent tests.
- Composite node+feature creation now rolls back derived sidecars as well as
  primary state, restoring `next_id`, node files, `edges.jsonl`, and
  `relations.jsonl` after mid-persist failures.
- Store lock handoff is now serialized under an OS-level guard lock on Unix and
  Windows, closing the stale-lock takeover race between observe/refresh/remove.
- Snapshot preflight now fails before mutations when `snapshots/` exists as a
  file instead of a directory.
- Snapshot restore no longer archives the internal `.lock.guard` file.
- Snapshot extraction rejects absolute archive paths portably, including POSIX
  absolute paths and Windows drive paths.
- History restore validation no longer keeps snapshot tarballs open across the
  restore path, fixing Windows rename failures during the history-preservation
  test.

## [v0.4.1] - 2026-08-12

### Fixed
- Active store locks now refresh their timestamp while work is in progress, so
  long-running operations cannot be reclaimed as stale by a second writer.
- `relations.jsonl` now preserves `Relation.Note`, and relation index reads stay
  backward-compatible with legacy entries that never stored a note.
- Binary index recovery (`rt storage reindex`) now runs under the store write
  lock and fails loudly on duplicate node IDs in `nodes.bin` instead of
  overwriting index entries silently.

### Changed
- `rt storage reindex` now explicitly supports only the current RTND v2 binary
  payload layout. Legacy RTND v1 stores remain readable with an intact
  `nodes.idx`, but rebuilding that index is intentionally rejected because the
  old concatenated payloads are not delimited safely enough for forensic
  recovery.

## [v0.4.0] - 2026-08-12

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

[Unreleased]: https://github.com/frudas24/research-tree/compare/v0.4.3...HEAD
[v0.4.3]: https://github.com/frudas24/research-tree/compare/v0.4.2...v0.4.3
[v0.4.2]: https://github.com/frudas24/research-tree/compare/v0.4.1...v0.4.2
[v0.4.1]: https://github.com/frudas24/research-tree/compare/v0.4.0...v0.4.1
[v0.4.0]: https://github.com/frudas24/research-tree/compare/v0.3.3...v0.4.0
[v0.3.3]: https://github.com/frudas24/research-tree/compare/v0.3.2...v0.3.3
[v0.3.2]: https://github.com/frudas24/research-tree/compare/v0.3.1...v0.3.2
[v0.3.1]: https://github.com/frudas24/research-tree/compare/v0.3.0...v0.3.1
[v0.3.0]: https://github.com/frudas24/research-tree/compare/v0.2.1...v0.3.0
[v0.2.1]: https://github.com/frudas24/research-tree/compare/v0.2.0...v0.2.1
[v0.2.0]: https://github.com/frudas24/research-tree/compare/v0.1.1...v0.2.0
[v0.1.1]: https://github.com/frudas24/research-tree/compare/v0.1.0...v0.1.1
[v0.1.0]: https://github.com/frudas24/research-tree/releases/tag/v0.1.0
