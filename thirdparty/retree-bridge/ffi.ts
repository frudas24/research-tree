/**
 * bun:ffi bindings to libretree.so.
 *
 * Research Tree — C ABI bridge for TypeScript consumption.
 *
 * Build the shared library first:
 *   make libretree.so
 *
 * All complex types cross the boundary as JSON. The caller must
 * call retree_free_string on every returned non-null char*.
 */

import { dlopen, FFIType, ptr } from "bun:ffi";
import { existsSync } from "fs";
import { join } from "path";

// ── Load shared library ─────────────────────────────────────

function resolveLibPath(): string {
  const candidates = [
    join(import.meta.dirname, "..", "..", "build", "libretree.so"),
    join(import.meta.dirname, "..", "..", "dist", "libretree.so"),
  ];
  for (const c of candidates) {
    if (existsSync(c)) {
      return c;
    }
  }
  throw new Error(
    "libretree.so not found. Build it: CGO_ENABLED=1 go build -buildmode=c-shared -o build/libretree.so ./cmd/rt-bridge/",
  );
}

const libPath = resolveLibPath();

const lib = dlopen(libPath, {
  // Lifecycle
  retree_init:        { args: [FFIType.cstring, FFIType.cstring], returns: FFIType.ptr },
  retree_open:        { args: [FFIType.cstring], returns: FFIType.ptr },
  retree_destroy:     { args: [FFIType.ptr] },
  retree_free_string: { args: [FFIType.ptr] },

  // Node CRUD
  retree_create_node: { args: [FFIType.ptr, FFIType.cstring], returns: FFIType.cstring },
  retree_get_node:    { args: [FFIType.ptr, FFIType.u64_fast], returns: FFIType.cstring },
  retree_update_node: { args: [FFIType.ptr, FFIType.cstring], returns: FFIType.cstring },
  retree_delete_node: { args: [FFIType.ptr, FFIType.u64_fast, FFIType.i32], returns: FFIType.cstring },

  // Graph traversal
  retree_get_children:    { args: [FFIType.ptr, FFIType.u64_fast], returns: FFIType.cstring },
  retree_get_parents:     { args: [FFIType.ptr, FFIType.u64_fast], returns: FFIType.cstring },
  retree_get_ancestors:   { args: [FFIType.ptr, FFIType.u64_fast], returns: FFIType.cstring },
  retree_get_descendants: { args: [FFIType.ptr, FFIType.u64_fast], returns: FFIType.cstring },
  retree_get_roots:       { args: [FFIType.ptr], returns: FFIType.cstring },

  // Queries
  retree_query_nodes: { args: [FFIType.ptr, FFIType.cstring], returns: FFIType.cstring },
  retree_get_status:  { args: [FFIType.ptr, FFIType.cstring], returns: FFIType.cstring },

  // Tags
  retree_add_tags:    { args: [FFIType.ptr, FFIType.u64_fast, FFIType.cstring], returns: FFIType.cstring },
  retree_remove_tags: { args: [FFIType.ptr, FFIType.u64_fast, FFIType.cstring], returns: FFIType.cstring },
  retree_add_parents: { args: [FFIType.ptr, FFIType.u64_fast, FFIType.cstring], returns: FFIType.cstring },
  retree_remove_parents: { args: [FFIType.ptr, FFIType.u64_fast, FFIType.cstring], returns: FFIType.cstring },

  // Artifacts
  retree_add_artifact: { args: [FFIType.ptr, FFIType.u64_fast, FFIType.cstring], returns: FFIType.cstring },
  retree_remove_artifact: { args: [FFIType.ptr, FFIType.u64_fast, FFIType.cstring], returns: FFIType.cstring },

  // Claims
  retree_invalidate_claim: { args: [FFIType.ptr, FFIType.u64_fast, FFIType.u64_fast, FFIType.cstring], returns: FFIType.cstring },
  retree_list_warnings:    { args: [FFIType.ptr, FFIType.cstring, FFIType.i32], returns: FFIType.cstring },
  retree_ack_warning:      { args: [FFIType.ptr, FFIType.cstring], returns: FFIType.cstring },

  // Recovery
  retree_list_snapshots:    { args: [FFIType.ptr], returns: FFIType.cstring },
  retree_restore_snapshot:  { args: [FFIType.ptr, FFIType.cstring], returns: FFIType.cstring },

  // History
  retree_get_node_history: { args: [FFIType.ptr, FFIType.u64_fast], returns: FFIType.cstring },

  // Migration
  retree_migrate_storage: { args: [FFIType.ptr, FFIType.cstring], returns: FFIType.cstring },
});

type NativeHandle = Exclude<ReturnType<typeof lib.symbols.retree_init>, null | undefined | 0>;

// ── TypeScript types ─────────────────────────────────────────

export interface NodeSummary {
  id: number;
  title: string;
  status: "active" | "done" | "paused";
  outcome?: "unset" | "success" | "failure" | "inconclusive";
  claim_status: "provisional" | "validated" | "invalidated" | "superseded";
  milestone_class?: "golden";
  milestone_kind?: "champion" | "breakthrough" | "pivot";
  milestone_reason?: string;
  agent: string;
  tags?: string[];
  revision: number;
  parents?: number[];
  children?: number[];
}

export interface StatusResult {
  total: number;
  active: NodeSummary[];
  done: NodeSummary[];
  paused: NodeSummary[];
  warnings: BranchWarning[];
  agent: string;
}

export interface BranchWarning {
  ID: string;
  Agent: string;
  RootCauseNode: number;
  ImpactedNode: number;
  Severity: string;
  Message: string;
  CreatedAt: string;
  AckedAt?: string | null;
}

export interface SnapshotMeta {
  id: string;
  created_at: string;
  operation: string;
  hash: string;
}

export interface NodeCreateInput {
  title: string;
  status?: string;
  claim_status?: string;
  milestone_class?: "golden";
  milestone_kind?: "champion" | "breakthrough" | "pivot";
  milestone_reason?: string;
  parents?: number[];
  agent?: string;
  tags?: string[];
  body?: string;
}

export interface NodeFilter {
  status?: string;
  claim_status?: string;
  outcome?: string;
  milestone_class?: "golden";
  milestone_kind?: "champion" | "breakthrough" | "pivot";
  tag?: string;
  tags_all?: string[];
  tags_any?: string[];
  agent?: string;
  title_contains?: string;
  body_contains?: string;
  has_artifact?: boolean;
  sort_by?: "id" | "created" | "modified" | "title";
  order?: "asc" | "desc";
  offset?: number;
  limit?: number;
}

// ── Wrapper ──────────────────────────────────────────────────

function call<R>(fn: () => string | null): R {
  const raw = fn();
  if (!raw) throw new Error("retree bridge returned null");
  const parsed = JSON.parse(raw);
  // Note: bun:ffi copies the C string into a JS string and the original
  // malloc'd pointer is lost. Memory allocated by C.CString in Go is
  // intentionally not freed here — the per-call leak (a few KB) is
  // acceptable for research-tree's call frequency.
  if (parsed.error) throw new Error(parsed.error);
  return parsed as R;
}

export class RetreeClient {
  private handle: NativeHandle | null;

  private constructor(handle: NativeHandle) {
    this.handle = handle;
  }

  static init(rootPath: string, format: "json" | "bin" = "json"): RetreeClient {
    const h = lib.symbols.retree_init(
      ptr(Buffer.from(rootPath + "\0")),
      ptr(Buffer.from(format + "\0")),
    );
    if (!h) throw new Error("retree_init returned null");
    return new RetreeClient(h as NativeHandle);
  }

  static open(rootPath: string): RetreeClient {
    const h = lib.symbols.retree_open(ptr(Buffer.from(rootPath + "\0")));
    if (!h) throw new Error(`retree_open failed for ${rootPath}`);
    return new RetreeClient(h as NativeHandle);
  }

  destroy(): void {
    if (!this.handle) return;
    lib.symbols.retree_destroy(this.handle);
    this.handle = null;
  }

  // Node CRUD
  createNode(input: NodeCreateInput): any {
    return call(() => lib.symbols.retree_create_node(this.handle!, ptr(Buffer.from(JSON.stringify(input) + "\0"))));
  }

  getNode(id: number): any {
    return call(() => lib.symbols.retree_get_node(this.handle!, id));
  }

  updateNode(id: number, fields: Record<string, any>): any {
    const payload = { ...fields, id };
    return call(() => lib.symbols.retree_update_node(this.handle!, ptr(Buffer.from(JSON.stringify(payload) + "\0"))));
  }

  deleteNode(id: number, force: boolean = false): any {
    return call(() => lib.symbols.retree_delete_node(this.handle!, id, force ? 1 : 0));
  }

  // Graph traversal
  getChildren(id: number): number[] {
    return call(() => lib.symbols.retree_get_children(this.handle!, id));
  }

  getParents(id: number): number[] {
    return call(() => lib.symbols.retree_get_parents(this.handle!, id));
  }

  getAncestors(id: number): number[] {
    return call(() => lib.symbols.retree_get_ancestors(this.handle!, id));
  }

  getDescendants(id: number): number[] {
    return call(() => lib.symbols.retree_get_descendants(this.handle!, id));
  }

  getRoots(): number[] {
    return call(() => lib.symbols.retree_get_roots(this.handle!));
  }

  // Queries
  queryNodes(filter: NodeFilter = {}): NodeSummary[] {
    return call(() => lib.symbols.retree_query_nodes(this.handle!, ptr(Buffer.from(JSON.stringify(filter) + "\0"))));
  }

  getStatus(agent: string = ""): StatusResult {
    return call(() => lib.symbols.retree_get_status(this.handle!, ptr(Buffer.from(agent + "\0"))));
  }

  // Tags
  addTags(id: number, tags: string[]): any {
    return call(() => lib.symbols.retree_add_tags(this.handle!, id, ptr(Buffer.from(tags.join(",") + "\0"))));
  }

  removeTags(id: number, tags: string[]): any {
    return call(() => lib.symbols.retree_remove_tags(this.handle!, id, ptr(Buffer.from(tags.join(",") + "\0"))));
  }

  addParents(id: number, parents: number[]): any {
    return call(() => lib.symbols.retree_add_parents(this.handle!, id, ptr(Buffer.from(parents.join(",") + "\0"))));
  }

  removeParents(id: number, parents: number[]): any {
    return call(() => lib.symbols.retree_remove_parents(this.handle!, id, ptr(Buffer.from(parents.join(",") + "\0"))));
  }

  // Artifacts
  addArtifact(id: number, artifact: Record<string, any>): any {
    return call(() => lib.symbols.retree_add_artifact(this.handle!, id, ptr(Buffer.from(JSON.stringify(artifact) + "\0"))));
  }

  removeArtifact(id: number, artifact: Record<string, any>): any {
    return call(() => lib.symbols.retree_remove_artifact(this.handle!, id, ptr(Buffer.from(JSON.stringify(artifact) + "\0"))));
  }

  // Claims
  invalidateClaim(target: number, refuter: number, reason: string): any {
    return call(() => lib.symbols.retree_invalidate_claim(this.handle!, target, refuter, ptr(Buffer.from(reason + "\0"))));
  }

  listWarnings(agent: string = "", onlyUnacked: boolean = true): BranchWarning[] {
    return call(() => lib.symbols.retree_list_warnings(this.handle!, ptr(Buffer.from(agent + "\0")), onlyUnacked ? 1 : 0));
  }

  ackWarning(warningID: string): any {
    return call(() => lib.symbols.retree_ack_warning(this.handle!, ptr(Buffer.from(warningID + "\0"))));
  }

  // Recovery
  listSnapshots(): SnapshotMeta[] {
    return call(() => lib.symbols.retree_list_snapshots(this.handle!));
  }

  restoreSnapshot(snapshotID: string): any {
    return call(() => lib.symbols.retree_restore_snapshot(this.handle!, ptr(Buffer.from(snapshotID + "\0"))));
  }

  // History
  getNodeHistory(id: number): any[] {
    return call(() => lib.symbols.retree_get_node_history(this.handle!, id));
  }

  // Migration
  migrateStorage(target: "json" | "bin"): any {
    return call(() => lib.symbols.retree_migrate_storage(this.handle!, ptr(Buffer.from(target + "\0"))));
  }
}
