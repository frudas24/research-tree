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
 *
 * This file mirrors the full `//export` surface of cmd/rt-bridge/main.go.
 * When the bridge ABI changes, regenerate this file in the same change.
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
  // ── Lifecycle ──────────────────────────────────────────────
  retree_init:        { args: [FFIType.cstring, FFIType.cstring], returns: FFIType.ptr },
  retree_open:        { args: [FFIType.cstring], returns: FFIType.ptr },
  retree_destroy:     { args: [FFIType.ptr] },
  retree_free_string: { args: [FFIType.ptr] },

  // ── Node CRUD ──────────────────────────────────────────────
  retree_create_node: { args: [FFIType.ptr, FFIType.cstring], returns: FFIType.cstring },
  retree_get_node:    { args: [FFIType.ptr, FFIType.u64_fast], returns: FFIType.cstring },
  retree_update_node: { args: [FFIType.ptr, FFIType.cstring], returns: FFIType.cstring },
  retree_delete_node: { args: [FFIType.ptr, FFIType.u64_fast, FFIType.i32], returns: FFIType.cstring },

  // ── Resources ──────────────────────────────────────────────
  retree_create_resource:          { args: [FFIType.ptr, FFIType.cstring], returns: FFIType.cstring },
  retree_update_resource:          { args: [FFIType.ptr, FFIType.cstring], returns: FFIType.cstring },
  retree_delete_resource:          { args: [FFIType.ptr, FFIType.cstring], returns: FFIType.cstring },
  retree_get_resource:             { args: [FFIType.ptr, FFIType.cstring], returns: FFIType.cstring },
  retree_list_resources:           { args: [FFIType.ptr], returns: FFIType.cstring },
  retree_claim_resource:           { args: [FFIType.ptr, FFIType.cstring], returns: FFIType.cstring },
  retree_release_resource:         { args: [FFIType.ptr, FFIType.u64_fast, FFIType.cstring], returns: FFIType.cstring },
  retree_list_resource_leases:     { args: [FFIType.ptr], returns: FFIType.cstring },
  retree_get_resource_events:      { args: [FFIType.ptr, FFIType.cstring], returns: FFIType.cstring },
  retree_list_resource_events:     { args: [FFIType.ptr], returns: FFIType.cstring },
  retree_get_node_resource_leases: { args: [FFIType.ptr, FFIType.u64_fast], returns: FFIType.cstring },

  // ── Graph traversal ────────────────────────────────────────
  retree_get_children:    { args: [FFIType.ptr, FFIType.u64_fast], returns: FFIType.cstring },
  retree_get_parents:     { args: [FFIType.ptr, FFIType.u64_fast], returns: FFIType.cstring },
  retree_get_ancestors:   { args: [FFIType.ptr, FFIType.u64_fast], returns: FFIType.cstring },
  retree_get_descendants: { args: [FFIType.ptr, FFIType.u64_fast], returns: FFIType.cstring },
  retree_get_roots:       { args: [FFIType.ptr], returns: FFIType.cstring },

  // ── Queries ────────────────────────────────────────────────
  retree_query_nodes: { args: [FFIType.ptr, FFIType.cstring], returns: FFIType.cstring },
  retree_get_status:  { args: [FFIType.ptr, FFIType.cstring], returns: FFIType.cstring },

  // ── Tags / parents ─────────────────────────────────────────
  retree_add_tags:       { args: [FFIType.ptr, FFIType.u64_fast, FFIType.cstring], returns: FFIType.cstring },
  retree_remove_tags:    { args: [FFIType.ptr, FFIType.u64_fast, FFIType.cstring], returns: FFIType.cstring },
  retree_add_parents:    { args: [FFIType.ptr, FFIType.u64_fast, FFIType.cstring], returns: FFIType.cstring },
  retree_remove_parents: { args: [FFIType.ptr, FFIType.u64_fast, FFIType.cstring], returns: FFIType.cstring },

  // ── Artifacts ──────────────────────────────────────────────
  retree_add_artifact:    { args: [FFIType.ptr, FFIType.u64_fast, FFIType.cstring], returns: FFIType.cstring },
  retree_remove_artifact: { args: [FFIType.ptr, FFIType.u64_fast, FFIType.cstring], returns: FFIType.cstring },

  // ── Claims ─────────────────────────────────────────────────
  retree_invalidate_claim: { args: [FFIType.ptr, FFIType.u64_fast, FFIType.u64_fast, FFIType.cstring], returns: FFIType.cstring },
  retree_list_warnings:    { args: [FFIType.ptr, FFIType.cstring, FFIType.i32], returns: FFIType.cstring },
  retree_ack_warning:      { args: [FFIType.ptr, FFIType.cstring], returns: FFIType.cstring },

  // ── Features ───────────────────────────────────────────────
  retree_create_feature:            { args: [FFIType.ptr, FFIType.cstring], returns: FFIType.cstring },
  retree_list_features:             { args: [FFIType.ptr], returns: FFIType.cstring },
  retree_get_feature:               { args: [FFIType.ptr, FFIType.cstring], returns: FFIType.cstring },
  retree_link_node_to_feature:      { args: [FFIType.ptr, FFIType.cstring], returns: FFIType.cstring },
  retree_relate_features:           { args: [FFIType.ptr, FFIType.cstring], returns: FFIType.cstring },
  retree_unrelate_features:         { args: [FFIType.ptr, FFIType.cstring], returns: FFIType.cstring },
  retree_list_feature_edges:        { args: [FFIType.ptr, FFIType.cstring], returns: FFIType.cstring },
  retree_compute_feature_health:    { args: [FFIType.ptr, FFIType.cstring], returns: FFIType.cstring },
  retree_compute_all_feature_health: { args: [FFIType.ptr], returns: FFIType.cstring },
  retree_compute_feature_timeline:  { args: [FFIType.ptr, FFIType.cstring], returns: FFIType.cstring },
  retree_compute_feature_impact:    { args: [FFIType.ptr, FFIType.cstring], returns: FFIType.cstring },
  retree_compute_feature_graph:     { args: [FFIType.ptr, FFIType.cstring], returns: FFIType.cstring },
  retree_set_feature_status:        { args: [FFIType.ptr, FFIType.cstring], returns: FFIType.cstring },
  retree_set_feature_current_node:  { args: [FFIType.ptr, FFIType.cstring], returns: FFIType.cstring },

  // ── Recovery ───────────────────────────────────────────────
  retree_list_snapshots:   { args: [FFIType.ptr], returns: FFIType.cstring },
  retree_restore_snapshot: { args: [FFIType.ptr, FFIType.cstring], returns: FFIType.cstring },

  // ── History ────────────────────────────────────────────────
  retree_get_node_history: { args: [FFIType.ptr, FFIType.u64_fast], returns: FFIType.cstring },

  // ── Migration ──────────────────────────────────────────────
  retree_migrate_storage: { args: [FFIType.ptr, FFIType.cstring], returns: FFIType.cstring },
});

type NativeHandle = Exclude<ReturnType<typeof lib.symbols.retree_init>, null | undefined | 0>;

// ── TypeScript types ─────────────────────────────────────────

export type NodeStatus = "active" | "done" | "paused";
export type Outcome = "unset" | "success" | "failure" | "inconclusive";
export type ClaimStatus = "provisional" | "validated" | "invalidated" | "superseded";
export type EvidenceStatus = "" | "clean" | "suspect" | "poisoned" | "revalidated";
export type EvidenceCause =
  | "" | "base_snapshot" | "toolchain" | "exporter" | "dataset"
  | "prompt_surface" | "runtime_env" | "unknown";
export type MilestoneClass = "" | "golden";
export type MilestoneKind = "" | "champion" | "breakthrough" | "pivot";
export type RelationType = "depends_on" | "compares_against" | "inspired_by" | "aggregates";
export type ArtifactMode = "path" | "embedded";
export type ResourceKind = "machine" | "gpu" | "cpu-slot" | "other";
export type EndpointKind = "" | "none" | "ip" | "dns";
export type LeaseMode = "exclusive" | "shared";
export type ResourceEventAction =
  | "claim" | "release" | "auto_release_done" | "auto_release_paused" | "auto_release_delete";
export type FeatureStatus = "active" | "degraded" | "retired";
export type DerivedHealth = "clean" | "warning" | "degraded" | "unmoored";
export type FeatureNodeRole =
  | "proposal" | "implementation" | "experiment" | "benchmark"
  | "regression" | "fix" | "decision" | "documentation";
export type FeatureEdgeType = "depends_on" | "collaborates_with" | "supersedes";
export type StorageFormat = "json" | "bin";

export interface GitCommit {
  hash: string;
  message?: string;
}

export interface Artifact {
  mode: ArtifactMode;
  host?: string;
  path: string;
  description?: string;
  size_bytes?: number;
}

export interface RunRecord {
  timestamp?: string;
  resource_id?: string;
  endpoint?: string;
  endpoint_kind?: EndpointKind;
  host?: string;
  command?: string;
  outdir?: string;
  seed?: string;
  eta?: string;
  cost?: string;
  note?: string;
  valid?: boolean | null;
  invalid_reason?: string;
}

export interface Relation {
  type: RelationType;
  target: number;
  note?: string;
}

export interface Node {
  schema_version: number;
  id: number;
  title: string;
  status: NodeStatus;
  claim_status?: ClaimStatus;
  evidence_status?: EvidenceStatus;
  evidence_cause?: EvidenceCause;
  evidence_scope?: string;
  scope?: string;
  exit_criteria?: string;
  parents?: number[];
  continued_by?: number[];
  superseded_by?: number[];
  agent?: string;
  tags?: string[];
  created?: string;
  modified?: string;
  outcome?: Outcome;
  revision: number;
  milestone_class?: MilestoneClass;
  milestone_kind?: MilestoneKind;
  milestone_reason?: string;
  relations?: Relation[];
  primary_parent?: number;
  commits?: GitCommit[];
  runs?: RunRecord[];
  artifacts?: Artifact[];
  invalidated_by?: number[];
  invalidation_reason?: string;
  poisoned_by?: number[];
  revalidated_by?: number[];
  poison_reason?: string;
  body?: string;
}

export interface NodeSummary {
  id: number;
  title: string;
  status: NodeStatus;
  outcome?: Outcome;
  claim_status: ClaimStatus;
  agent: string;
  tags?: string[];
  revision: number;
  parents?: number[];
  children?: number[];
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

export interface HotspotSummary {
  id: number;
  title: string;
  status: NodeStatus;
  outcome: Outcome;
  claim_status: ClaimStatus;
  milestone_class?: MilestoneClass;
  milestone_kind?: MilestoneKind;
  milestone_reason?: string;
  agent: string;
  pending_children: number;
  age_days: number;
  pending_weight: number;
  inconclusive_bonus: number;
  hotness: number;
}

export interface StatusSummary {
  total: number;
  active: NodeSummary[];
  done: NodeSummary[];
  paused: NodeSummary[];
  warnings: BranchWarning[];
  agent: string;
  status_counts: Record<string, number>;
  claim_status_counts: Record<string, number>;
  outcome_counts: Record<string, number>;
  run_validity_counts: Record<string, number>;
  matrix: Record<string, Record<string, number>>;
  hotspot_formula: string;
  hotspots: HotspotSummary[];
}

export interface ResourceSpec {
  os?: string;
  cpu?: string;
  ram_gb?: number;
  gpu?: string;
  vram_gb?: number;
  storage_hint?: string;
}

export interface Resource {
  id: string;
  label: string;
  endpoint?: string;
  endpoint_kind?: EndpointKind;
  kind: ResourceKind;
  tags?: string[];
  enabled: boolean;
  maintenance?: boolean;
  capacity?: number;
  spec?: ResourceSpec;
  created?: string;
  modified?: string;
}

export interface ResourceLease {
  resource_id: string;
  node_id: number;
  mode: LeaseMode;
  claimed_by?: string;
  note?: string;
  claimed_at: string;
}

export interface ResourceEvent {
  resource_id: string;
  node_id: number;
  action: ResourceEventAction;
  mode?: LeaseMode;
  claimed_by?: string;
  note?: string;
  reason?: string;
  timestamp: string;
}

export interface SnapshotMeta {
  id: string;
  created_at: string;
  operation: string;
  hash: string;
}

export interface FeatureLinkedNode {
  node_id: number;
  role: FeatureNodeRole;
}

export interface Feature {
  id: string;
  name: string;
  slug: string;
  status: FeatureStatus;
  created_from: number;
  current_node?: number;
  current_node_mode?: "explicit" | "derived";
  nodes: FeatureLinkedNode[];
}

export interface FeatureEdge {
  from: string;
  to: string;
  type: FeatureEdgeType;
  created_from: number;
}

export interface TimelineEntry {
  node_id: number;
  role: FeatureNodeRole;
  title: string;
  status: NodeStatus;
}

export interface FeatureHealthReport {
  feature_id: string;
  feature_name: string;
  status: FeatureStatus;
  health: DerivedHealth;
  issues: string[];
  timeline?: TimelineEntry[];
}

export interface FeatureImpact {
  feature_id: string;
  feature_name: string;
  depends_on_us: string[];
  collaborates_with_us: string[];
  we_depend_on: string[];
}

export interface FeatureGraphNode {
  id: string;
  name: string;
  status: FeatureStatus;
}

export interface FeatureGraphEdge {
  from: string;
  to: string;
  type: FeatureEdgeType;
}

export interface FeatureGraph {
  nodes: FeatureGraphNode[];
  edges: FeatureGraphEdge[];
}

// ── Input payloads ───────────────────────────────────────────

export interface NodeCreateInput {
  title: string;
  status?: NodeStatus;
  claim_status?: ClaimStatus;
  evidence_status?: EvidenceStatus;
  evidence_cause?: EvidenceCause;
  evidence_scope?: string;
  scope?: string;
  exit_criteria?: string;
  parents?: number[];
  continued_by?: number[];
  superseded_by?: number[];
  agent?: string;
  tags?: string[];
  outcome?: Outcome;
  milestone_class?: MilestoneClass;
  milestone_kind?: MilestoneKind;
  milestone_reason?: string;
  body?: string;
}

export type NodeUpdateInput = Partial<NodeCreateInput> & { id: number };

export interface NodeFilter {
  status?: NodeStatus;
  claim_status?: ClaimStatus;
  evidence_status?: EvidenceStatus;
  evidence_cause?: EvidenceCause;
  outcome?: Outcome;
  tag?: string;
  tags_all?: string[];
  tags_any?: string[];
  agent?: string;
  title_contains?: string;
  scope_contains?: string;
  body_contains?: string;
  continued_by?: number;
  superseded_by?: number;
  has_artifact?: boolean;
  milestone_class?: MilestoneClass;
  milestone_kind?: MilestoneKind;
  created_after?: string;
  created_before?: string;
  sort_by?: "id" | "created" | "modified" | "title";
  order?: "asc" | "desc";
  offset?: number;
  limit?: number;
}

export type ResourceInput = Omit<Resource, "created" | "modified">;

export interface ResourceLeaseInput {
  resource_id: string;
  node_id: number;
  mode: LeaseMode;
  claimed_by?: string;
  note?: string;
}

export type ArtifactInput = Artifact;

export interface CreateFeaturePayload {
  name: string;
  created_from: number;
}

export interface LinkNodeToFeaturePayload {
  feature: string;
  node_id: number;
  role: FeatureNodeRole;
}

export interface RelateFeaturesPayload {
  from: string;
  to: string;
  type: FeatureEdgeType;
  created_from: number;
}

export interface UnrelateFeaturesPayload {
  from: string;
  to: string;
  type: FeatureEdgeType;
}

export interface SetFeatureStatusPayload {
  feature: string;
  status: FeatureStatus;
}

export interface SetFeatureCurrentNodePayload {
  feature: string;
  node_id: number;
}

// ── Wrapper ──────────────────────────────────────────────────

function toPtr(s: string) {
  return ptr(Buffer.from(s + "\0"));
}

function call<R>(fn: () => unknown): R {
  const raw = fn();
  if (raw === null || raw === undefined) throw new Error("retree bridge returned null");
  // bun:ffi types cstring returns as CString (a String subclass); coerce to a
  // primitive so JSON.parse always receives a string.
  const text = String(raw);
  const parsed = JSON.parse(text);
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

  static init(rootPath: string, format: StorageFormat = "json"): RetreeClient {
    const h = lib.symbols.retree_init(toPtr(rootPath), toPtr(format));
    if (!h) throw new Error("retree_init failed");
    return new RetreeClient(h as NativeHandle);
  }

  static open(rootPath: string): RetreeClient {
    const h = lib.symbols.retree_open(toPtr(rootPath));
    if (!h) throw new Error(`retree_open failed for ${rootPath}`);
    return new RetreeClient(h as NativeHandle);
  }

  destroy(): void {
    if (!this.handle) return;
    lib.symbols.retree_destroy(this.handle);
    this.handle = null;
  }

  // ── Node CRUD ──────────────────────────────────────────────
  createNode(input: NodeCreateInput): Node {
    return call(() => lib.symbols.retree_create_node(this.handle!, toPtr(JSON.stringify(input))));
  }

  getNode(id: number): Node {
    return call(() => lib.symbols.retree_get_node(this.handle!, id));
  }

  updateNode(id: number, fields: Partial<NodeCreateInput>): Node {
    const payload: NodeUpdateInput = { ...fields, id };
    return call(() => lib.symbols.retree_update_node(this.handle!, toPtr(JSON.stringify(payload))));
  }

  deleteNode(id: number, force: boolean = false): { deleted: number; force: boolean } {
    return call(() => lib.symbols.retree_delete_node(this.handle!, id, force ? 1 : 0));
  }

  // ── Resources ──────────────────────────────────────────────
  createResource(input: ResourceInput): Resource {
    return call(() => lib.symbols.retree_create_resource(this.handle!, toPtr(JSON.stringify(input))));
  }

  updateResource(input: ResourceInput): Resource {
    return call(() => lib.symbols.retree_update_resource(this.handle!, toPtr(JSON.stringify(input))));
  }

  deleteResource(resourceID: string): { deleted: string } {
    return call(() => lib.symbols.retree_delete_resource(this.handle!, toPtr(resourceID)));
  }

  getResource(resourceID: string): Resource {
    return call(() => lib.symbols.retree_get_resource(this.handle!, toPtr(resourceID)));
  }

  listResources(): Resource[] {
    return call(() => lib.symbols.retree_list_resources(this.handle!));
  }

  claimResource(lease: ResourceLeaseInput): ResourceLease {
    return call(() => lib.symbols.retree_claim_resource(this.handle!, toPtr(JSON.stringify(lease))));
  }

  releaseResource(nodeID: number, resourceID: string): { node_id: number; resource_id: string } {
    return call(() => lib.symbols.retree_release_resource(this.handle!, nodeID, toPtr(resourceID)));
  }

  listResourceLeases(): ResourceLease[] {
    return call(() => lib.symbols.retree_list_resource_leases(this.handle!));
  }

  getResourceEvents(resourceID: string): ResourceEvent[] {
    return call(() => lib.symbols.retree_get_resource_events(this.handle!, toPtr(resourceID)));
  }

  listResourceEvents(): ResourceEvent[] {
    return call(() => lib.symbols.retree_list_resource_events(this.handle!));
  }

  getNodeResourceLeases(id: number): ResourceLease[] {
    return call(() => lib.symbols.retree_get_node_resource_leases(this.handle!, id));
  }

  // ── Graph traversal ────────────────────────────────────────
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

  // ── Queries ────────────────────────────────────────────────
  queryNodes(filter: NodeFilter = {}): NodeSummary[] {
    return call(() => lib.symbols.retree_query_nodes(this.handle!, toPtr(JSON.stringify(filter))));
  }

  getStatus(agent: string = ""): StatusSummary {
    return call(() => lib.symbols.retree_get_status(this.handle!, toPtr(agent)));
  }

  // ── Tags / parents ─────────────────────────────────────────
  addTags(id: number, tags: string[]): { id: number; tags: string[] } {
    return call(() => lib.symbols.retree_add_tags(this.handle!, id, toPtr(tags.join(","))));
  }

  removeTags(id: number, tags: string[]): { id: number; removed: string[] } {
    return call(() => lib.symbols.retree_remove_tags(this.handle!, id, toPtr(tags.join(","))));
  }

  addParents(id: number, parents: number[]): { id: number; parents: number[] } {
    return call(() => lib.symbols.retree_add_parents(this.handle!, id, toPtr(parents.join(","))));
  }

  removeParents(id: number, parents: number[]): { id: number; removed: number[] } {
    return call(() => lib.symbols.retree_remove_parents(this.handle!, id, toPtr(parents.join(","))));
  }

  // ── Artifacts ──────────────────────────────────────────────
  addArtifact(id: number, artifact: ArtifactInput): { id: number; artifact: Artifact } {
    return call(() => lib.symbols.retree_add_artifact(this.handle!, id, toPtr(JSON.stringify(artifact))));
  }

  removeArtifact(id: number, artifact: ArtifactInput): { id: number; artifact: Artifact } {
    return call(() => lib.symbols.retree_remove_artifact(this.handle!, id, toPtr(JSON.stringify(artifact))));
  }

  // ── Claims ─────────────────────────────────────────────────
  invalidateClaim(target: number, refuter: number, reason: string): { target: number; refuter: number; reason: string } {
    return call(() => lib.symbols.retree_invalidate_claim(this.handle!, target, refuter, toPtr(reason)));
  }

  listWarnings(agent: string = "", onlyUnacked: boolean = true): BranchWarning[] {
    return call(() => lib.symbols.retree_list_warnings(this.handle!, toPtr(agent), onlyUnacked ? 1 : 0));
  }

  ackWarning(warningID: string): { ack: string } {
    return call(() => lib.symbols.retree_ack_warning(this.handle!, toPtr(warningID)));
  }

  // ── Features ───────────────────────────────────────────────
  createFeature(payload: CreateFeaturePayload): Feature {
    return call(() => lib.symbols.retree_create_feature(this.handle!, toPtr(JSON.stringify(payload))));
  }

  listFeatures(): Feature[] {
    return call(() => lib.symbols.retree_list_features(this.handle!));
  }

  getFeature(spec: string): Feature {
    return call(() => lib.symbols.retree_get_feature(this.handle!, toPtr(spec)));
  }

  linkNodeToFeature(payload: LinkNodeToFeaturePayload): Feature {
    return call(() => lib.symbols.retree_link_node_to_feature(this.handle!, toPtr(JSON.stringify(payload))));
  }

  relateFeatures(payload: RelateFeaturesPayload): { from: string; to: string; type: FeatureEdgeType; created_from: number } {
    return call(() => lib.symbols.retree_relate_features(this.handle!, toPtr(JSON.stringify(payload))));
  }

  unrelateFeatures(payload: UnrelateFeaturesPayload): { from: string; to: string; type: FeatureEdgeType } {
    return call(() => lib.symbols.retree_unrelate_features(this.handle!, toPtr(JSON.stringify(payload))));
  }

  listFeatureEdges(spec: string): FeatureEdge[] {
    return call(() => lib.symbols.retree_list_feature_edges(this.handle!, toPtr(spec)));
  }

  computeFeatureHealth(spec: string): FeatureHealthReport {
    return call(() => lib.symbols.retree_compute_feature_health(this.handle!, toPtr(spec)));
  }

  computeAllFeatureHealth(): FeatureHealthReport[] {
    return call(() => lib.symbols.retree_compute_all_feature_health(this.handle!));
  }

  computeFeatureTimeline(spec: string): FeatureHealthReport {
    return call(() => lib.symbols.retree_compute_feature_timeline(this.handle!, toPtr(spec)));
  }

  computeFeatureImpact(spec: string): FeatureImpact {
    return call(() => lib.symbols.retree_compute_feature_impact(this.handle!, toPtr(spec)));
  }

  computeFeatureGraph(spec: string): FeatureGraph {
    return call(() => lib.symbols.retree_compute_feature_graph(this.handle!, toPtr(spec)));
  }

  setFeatureStatus(payload: SetFeatureStatusPayload): Feature {
    return call(() => lib.symbols.retree_set_feature_status(this.handle!, toPtr(JSON.stringify(payload))));
  }

  setFeatureCurrentNode(payload: SetFeatureCurrentNodePayload): Feature {
    return call(() => lib.symbols.retree_set_feature_current_node(this.handle!, toPtr(JSON.stringify(payload))));
  }

  // ── Recovery ───────────────────────────────────────────────
  listSnapshots(): SnapshotMeta[] {
    return call(() => lib.symbols.retree_list_snapshots(this.handle!));
  }

  restoreSnapshot(snapshotID: string): { restored: string } {
    return call(() => lib.symbols.retree_restore_snapshot(this.handle!, toPtr(snapshotID)));
  }

  // ── History ────────────────────────────────────────────────
  getNodeHistory(id: number): Node[] {
    return call(() => lib.symbols.retree_get_node_history(this.handle!, id));
  }

  // ── Migration ──────────────────────────────────────────────
  migrateStorage(target: StorageFormat): { format: StorageFormat } {
    return call(() => lib.symbols.retree_migrate_storage(this.handle!, toPtr(target)));
  }
}
