// Command rt-bridge produces a C shared library (libretree.so) that
// exposes the research-tree ABI for FFI consumption from TypeScript
// (bun:ffi) and other languages.
//
// All complex types cross the boundary as JSON strings. The caller
// must free returned strings with retree_free_string.
//
// Stores are referenced by opaque uintptr handles.
//
// Build: CGO_ENABLED=1 go build -buildmode=c-shared -o libretree.so .
package main

/*
#include <stdlib.h>
*/
import "C"
import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"unsafe"

	"github.com/frudas24/research-tree/pkg/retree"
)

var (
	stores     = map[uintptr]*retree.Store{}
	storesMu   sync.Mutex
	nextHandle uintptr = 1
)

func register(s *retree.Store) uintptr {
	storesMu.Lock()
	defer storesMu.Unlock()
	h := nextHandle
	nextHandle++
	stores[h] = s
	return h
}

func getHandle(h uintptr) (*retree.Store, error) {
	storesMu.Lock()
	defer storesMu.Unlock()
	s, ok := stores[h]
	if !ok || s == nil {
		return nil, fmt.Errorf("invalid handle %d", h)
	}
	return s, nil
}

func unregister(h uintptr) {
	storesMu.Lock()
	defer storesMu.Unlock()
	delete(stores, h)
}

func jsonResult(v any) *C.char {
	b, err := json.Marshal(v)
	if err != nil {
		return C.CString(fmt.Sprintf(`{"error":%q}`, err.Error()))
	}
	return C.CString(string(b))
}

func jsonError(err error) *C.char {
	if err == nil {
		return nil
	}
	return C.CString(fmt.Sprintf(`{"error":%q}`, err.Error()))
}

// ── Lifecycle ────────────────────────────────────────────────────

//export retree_init
func retree_init(rootPath *C.char, format *C.char) uintptr {
	s, err := retree.Init(C.GoString(rootPath), retree.StorageFormat(C.GoString(format)))
	if err != nil {
		return 0
	}
	return register(s)
}

//export retree_open
func retree_open(rootPath *C.char) uintptr {
	s, err := retree.Open(C.GoString(rootPath))
	if err != nil {
		return 0
	}
	return register(s)
}

//export retree_destroy
func retree_destroy(handle uintptr) {
	unregister(handle)
}

//export retree_free_string
func retree_free_string(s *C.char) {
	C.free(unsafe.Pointer(s))
}

// ── Node CRUD ────────────────────────────────────────────────────

//export retree_create_node
func retree_create_node(handle uintptr, nodeJSON *C.char) *C.char {
	s, err := getHandle(handle)
	if err != nil {
		return jsonError(err)
	}
	var n retree.Node
	if err := json.Unmarshal([]byte(C.GoString(nodeJSON)), &n); err != nil {
		return jsonError(err)
	}
	if err := s.CreateNode(&n); err != nil {
		return jsonError(err)
	}
	return jsonResult(n)
}

//export retree_get_node
func retree_get_node(handle uintptr, id uint64) *C.char {
	s, err := getHandle(handle)
	if err != nil {
		return jsonError(err)
	}
	n, err := s.GetNode(retree.NodeID(id))
	if err != nil {
		return jsonError(err)
	}
	return jsonResult(n)
}

//export retree_update_node
func retree_update_node(handle uintptr, nodeJSON *C.char) *C.char {
	s, err := getHandle(handle)
	if err != nil {
		return jsonError(err)
	}

	// Parse the partial input to extract the node ID
	var partial map[string]any
	if err := json.Unmarshal([]byte(C.GoString(nodeJSON)), &partial); err != nil {
		return jsonError(err)
	}
	idFloat, ok := partial["id"].(float64)
	if !ok {
		return jsonError(fmt.Errorf("id required in update payload"))
	}
	id := retree.NodeID(idFloat)

	// Get existing node and merge the partial fields on top
	existing, err := s.GetNode(id)
	if err != nil {
		return jsonError(err)
	}
	existingBytes, _ := json.Marshal(existing)
	var merged map[string]any
	json.Unmarshal(existingBytes, &merged)
	for k, v := range partial {
		merged[k] = v
	}
	mergedBytes, _ := json.Marshal(merged)

	var n retree.Node
	if err := json.Unmarshal(mergedBytes, &n); err != nil {
		return jsonError(err)
	}
	if err := s.UpdateNode(&n); err != nil {
		return jsonError(err)
	}
	updated, err := s.GetNode(id)
	if err != nil {
		return jsonError(err)
	}
	return jsonResult(updated)
}

//export retree_delete_node
func retree_delete_node(handle uintptr, id uint64, force int32) *C.char {
	s, err := getHandle(handle)
	if err != nil {
		return jsonError(err)
	}
	if err := s.DeleteNode(retree.NodeID(id), force != 0); err != nil {
		return jsonError(err)
	}
	return jsonResult(map[string]any{"deleted": id, "force": force != 0})
}

// ── Resources ─────────────────────────────────────────────────────

//export retree_create_resource
func retree_create_resource(handle uintptr, resourceJSON *C.char) *C.char {
	s, err := getHandle(handle)
	if err != nil {
		return jsonError(err)
	}
	var r retree.Resource
	if err := json.Unmarshal([]byte(C.GoString(resourceJSON)), &r); err != nil {
		return jsonError(err)
	}
	if err := s.CreateResource(r); err != nil {
		return jsonError(err)
	}
	return jsonResult(r)
}

//export retree_update_resource
func retree_update_resource(handle uintptr, resourceJSON *C.char) *C.char {
	s, err := getHandle(handle)
	if err != nil {
		return jsonError(err)
	}
	var r retree.Resource
	if err := json.Unmarshal([]byte(C.GoString(resourceJSON)), &r); err != nil {
		return jsonError(err)
	}
	if err := s.UpdateResource(r); err != nil {
		return jsonError(err)
	}
	updated, err := s.GetResource(r.ID)
	if err != nil {
		return jsonError(err)
	}
	return jsonResult(updated)
}

//export retree_delete_resource
func retree_delete_resource(handle uintptr, resourceID *C.char) *C.char {
	s, err := getHandle(handle)
	if err != nil {
		return jsonError(err)
	}
	id := C.GoString(resourceID)
	if err := s.DeleteResource(id); err != nil {
		return jsonError(err)
	}
	return jsonResult(map[string]any{"deleted": id})
}

//export retree_get_resource
func retree_get_resource(handle uintptr, resourceID *C.char) *C.char {
	s, err := getHandle(handle)
	if err != nil {
		return jsonError(err)
	}
	r, err := s.GetResource(C.GoString(resourceID))
	if err != nil {
		return jsonError(err)
	}
	return jsonResult(r)
}

//export retree_list_resources
func retree_list_resources(handle uintptr) *C.char {
	s, err := getHandle(handle)
	if err != nil {
		return jsonError(err)
	}
	resources, err := s.ListResources()
	if err != nil {
		return jsonError(err)
	}
	return jsonResult(resources)
}

//export retree_claim_resource
func retree_claim_resource(handle uintptr, leaseJSON *C.char) *C.char {
	s, err := getHandle(handle)
	if err != nil {
		return jsonError(err)
	}
	var lease retree.ResourceLease
	if err := json.Unmarshal([]byte(C.GoString(leaseJSON)), &lease); err != nil {
		return jsonError(err)
	}
	if err := s.ClaimResource(lease); err != nil {
		return jsonError(err)
	}
	return jsonResult(lease)
}

//export retree_release_resource
func retree_release_resource(handle uintptr, nodeID uint64, resourceID *C.char) *C.char {
	s, err := getHandle(handle)
	if err != nil {
		return jsonError(err)
	}
	if err := s.ReleaseResource(retree.NodeID(nodeID), C.GoString(resourceID)); err != nil {
		return jsonError(err)
	}
	return jsonResult(map[string]any{"node_id": nodeID, "resource_id": C.GoString(resourceID)})
}

//export retree_list_resource_leases
func retree_list_resource_leases(handle uintptr) *C.char {
	s, err := getHandle(handle)
	if err != nil {
		return jsonError(err)
	}
	leases, err := s.ListResourceLeases()
	if err != nil {
		return jsonError(err)
	}
	return jsonResult(leases)
}

//export retree_get_resource_events
func retree_get_resource_events(handle uintptr, resourceID *C.char) *C.char {
	s, err := getHandle(handle)
	if err != nil {
		return jsonError(err)
	}
	events, err := s.GetResourceEvents(C.GoString(resourceID))
	if err != nil {
		return jsonError(err)
	}
	return jsonResult(events)
}

//export retree_list_resource_events
func retree_list_resource_events(handle uintptr) *C.char {
	s, err := getHandle(handle)
	if err != nil {
		return jsonError(err)
	}
	events, err := s.ListResourceEvents()
	if err != nil {
		return jsonError(err)
	}
	return jsonResult(events)
}

//export retree_get_node_resource_leases
func retree_get_node_resource_leases(handle uintptr, id uint64) *C.char {
	s, err := getHandle(handle)
	if err != nil {
		return jsonError(err)
	}
	leases, err := s.GetNodeResourceLeases(retree.NodeID(id))
	if err != nil {
		return jsonError(err)
	}
	return jsonResult(leases)
}

// ── Graph traversal ──────────────────────────────────────────────

//export retree_get_children
func retree_get_children(handle uintptr, id uint64) *C.char {
	s, err := getHandle(handle)
	if err != nil {
		return jsonError(err)
	}
	ids, err := s.GetChildren(retree.NodeID(id))
	if err != nil {
		return jsonError(err)
	}
	if ids == nil {
		ids = []retree.NodeID{}
	}
	return jsonResult(ids)
}

//export retree_get_parents
func retree_get_parents(handle uintptr, id uint64) *C.char {
	s, err := getHandle(handle)
	if err != nil {
		return jsonError(err)
	}
	ids, err := s.GetParents(retree.NodeID(id))
	if err != nil {
		return jsonError(err)
	}
	if ids == nil {
		ids = []retree.NodeID{}
	}
	return jsonResult(ids)
}

//export retree_get_ancestors
func retree_get_ancestors(handle uintptr, id uint64) *C.char {
	s, err := getHandle(handle)
	if err != nil {
		return jsonError(err)
	}
	ids, err := s.GetAncestors(retree.NodeID(id))
	if err != nil {
		return jsonError(err)
	}
	if ids == nil {
		ids = []retree.NodeID{}
	}
	return jsonResult(ids)
}

//export retree_get_descendants
func retree_get_descendants(handle uintptr, id uint64) *C.char {
	s, err := getHandle(handle)
	if err != nil {
		return jsonError(err)
	}
	ids, err := s.GetDescendants(retree.NodeID(id))
	if err != nil {
		return jsonError(err)
	}
	if ids == nil {
		ids = []retree.NodeID{}
	}
	return jsonResult(ids)
}

//export retree_get_roots
func retree_get_roots(handle uintptr) *C.char {
	s, err := getHandle(handle)
	if err != nil {
		return jsonError(err)
	}
	ids, err := s.GetRoots()
	if err != nil {
		return jsonError(err)
	}
	if ids == nil {
		ids = []retree.NodeID{}
	}
	return jsonResult(ids)
}

// ── Queries ──────────────────────────────────────────────────────

type bridgeSummary struct {
	ID          retree.NodeID      `json:"id"`
	Title       string             `json:"title"`
	Status      retree.NodeStatus  `json:"status"`
	Outcome     retree.Outcome     `json:"outcome,omitempty"`
	ClaimStatus retree.ClaimStatus `json:"claim_status"`
	Agent       string             `json:"agent"`
	Tags        []string           `json:"tags,omitempty"`
	Revision    uint64             `json:"revision"`
	Parents     []retree.NodeID    `json:"parents,omitempty"`
	Children    []retree.NodeID    `json:"children,omitempty"`
}

//export retree_query_nodes
func retree_query_nodes(handle uintptr, filterJSON *C.char) *C.char {
	s, err := getHandle(handle)
	if err != nil {
		return jsonError(err)
	}
	var f retree.Filter
	if err := json.Unmarshal([]byte(C.GoString(filterJSON)), &f); err != nil {
		return jsonError(err)
	}
	nodes, err := s.QueryNodes(f)
	if err != nil {
		return jsonError(err)
	}
	base := retree.SummarizeNodes(nodes)
	summaries := make([]bridgeSummary, len(base))
	for i, n := range base {
		summaries[i] = bridgeSummary{
			ID:          n.ID,
			Title:       n.Title,
			Status:      n.Status,
			Outcome:     n.Outcome,
			ClaimStatus: n.ClaimStatus,
			Agent:       n.Agent,
			Tags:        n.Tags,
			Revision:    n.Revision,
			Parents:     n.Parents,
			Children:    n.Children,
		}
	}
	return jsonResult(summaries)
}

//export retree_get_status
func retree_get_status(handle uintptr, agentFilter *C.char) *C.char {
	s, err := getHandle(handle)
	if err != nil {
		return jsonError(err)
	}
	agent := C.GoString(agentFilter)
	af := retree.Filter{}
	if agent != "" {
		af.Agent = agent
	}
	all, _ := s.QueryNodes(af)
	warnings, _ := s.ListBranchWarnings(agent, true)
	if warnings == nil {
		warnings = []retree.BranchWarning{}
	}
	summary := retree.BuildStatusSummary(all, warnings, retree.StatusBuildOptions{
		Agent:        agent,
		HotspotLimit: 10,
	})
	return jsonResult(summary)
}

// ── Tags / Artifacts / Claims ────────────────────────────────────

//export retree_add_tags
func retree_add_tags(handle uintptr, id uint64, tagsCSV *C.char) *C.char {
	s, err := getHandle(handle)
	if err != nil {
		return jsonError(err)
	}
	tags := splitCSV(C.GoString(tagsCSV))
	if err := s.AddTags(retree.NodeID(id), tags...); err != nil {
		return jsonError(err)
	}
	return jsonResult(map[string]any{"id": id, "tags": tags})
}

//export retree_remove_tags
func retree_remove_tags(handle uintptr, id uint64, tagsCSV *C.char) *C.char {
	s, err := getHandle(handle)
	if err != nil {
		return jsonError(err)
	}
	tags := splitCSV(C.GoString(tagsCSV))
	if err := s.RemoveTags(retree.NodeID(id), tags...); err != nil {
		return jsonError(err)
	}
	return jsonResult(map[string]any{"id": id, "removed": tags})
}

//export retree_add_parents
func retree_add_parents(handle uintptr, id uint64, parentsCSV *C.char) *C.char {
	s, err := getHandle(handle)
	if err != nil {
		return jsonError(err)
	}
	parents := splitNodeIDs(C.GoString(parentsCSV))
	if err := s.AddParents(retree.NodeID(id), parents...); err != nil {
		return jsonError(err)
	}
	return jsonResult(map[string]any{"id": id, "parents": parents})
}

//export retree_remove_parents
func retree_remove_parents(handle uintptr, id uint64, parentsCSV *C.char) *C.char {
	s, err := getHandle(handle)
	if err != nil {
		return jsonError(err)
	}
	parents := splitNodeIDs(C.GoString(parentsCSV))
	if err := s.RemoveParents(retree.NodeID(id), parents...); err != nil {
		return jsonError(err)
	}
	return jsonResult(map[string]any{"id": id, "removed": parents})
}

//export retree_add_artifact
func retree_add_artifact(handle uintptr, id uint64, artifactJSON *C.char) *C.char {
	s, err := getHandle(handle)
	if err != nil {
		return jsonError(err)
	}
	var a retree.Artifact
	if err := json.Unmarshal([]byte(C.GoString(artifactJSON)), &a); err != nil {
		return jsonError(err)
	}
	if err := s.AddArtifact(retree.NodeID(id), a); err != nil {
		return jsonError(err)
	}
	return jsonResult(map[string]any{"id": id, "artifact": a})
}

//export retree_remove_artifact
func retree_remove_artifact(handle uintptr, id uint64, artifactJSON *C.char) *C.char {
	s, err := getHandle(handle)
	if err != nil {
		return jsonError(err)
	}
	var a retree.Artifact
	if err := json.Unmarshal([]byte(C.GoString(artifactJSON)), &a); err != nil {
		return jsonError(err)
	}
	if err := s.RemoveArtifact(retree.NodeID(id), a); err != nil {
		return jsonError(err)
	}
	return jsonResult(map[string]any{"id": id, "artifact": a})
}

//export retree_invalidate_claim
func retree_invalidate_claim(handle uintptr, target uint64, refuter uint64, reason *C.char) *C.char {
	s, err := getHandle(handle)
	if err != nil {
		return jsonError(err)
	}
	if err := s.InvalidateClaim(retree.NodeID(target), retree.NodeID(refuter), C.GoString(reason)); err != nil {
		return jsonError(err)
	}
	return jsonResult(map[string]any{"target": target, "refuter": refuter, "reason": C.GoString(reason)})
}

//export retree_list_warnings
func retree_list_warnings(handle uintptr, agentFilter *C.char, onlyUnacked int32) *C.char {
	s, err := getHandle(handle)
	if err != nil {
		return jsonError(err)
	}
	warnings, err := s.ListBranchWarnings(C.GoString(agentFilter), onlyUnacked != 0)
	if err != nil {
		return jsonError(err)
	}
	if warnings == nil {
		warnings = []retree.BranchWarning{}
	}
	return jsonResult(warnings)
}

//export retree_ack_warning
func retree_ack_warning(handle uintptr, warningID *C.char) *C.char {
	s, err := getHandle(handle)
	if err != nil {
		return jsonError(err)
	}
	if err := s.AckBranchWarning(C.GoString(warningID)); err != nil {
		return jsonError(err)
	}
	return jsonResult(map[string]string{"ack": C.GoString(warningID)})
}

// ── Recovery ─────────────────────────────────────────────────────

//export retree_list_snapshots
func retree_list_snapshots(handle uintptr) *C.char {
	s, err := getHandle(handle)
	if err != nil {
		return jsonError(err)
	}
	snaps, err := s.ListSnapshots()
	if err != nil {
		return jsonError(err)
	}
	if snaps == nil {
		snaps = []retree.SnapshotMeta{}
	}
	return jsonResult(snaps)
}

//export retree_restore_snapshot
func retree_restore_snapshot(handle uintptr, snapshotID *C.char) *C.char {
	s, err := getHandle(handle)
	if err != nil {
		return jsonError(err)
	}
	if err := s.RestoreSnapshot(C.GoString(snapshotID)); err != nil {
		return jsonError(err)
	}
	return jsonResult(map[string]string{"restored": C.GoString(snapshotID)})
}

// ── History ──────────────────────────────────────────────────────

//export retree_get_node_history
func retree_get_node_history(handle uintptr, id uint64) *C.char {
	s, err := getHandle(handle)
	if err != nil {
		return jsonError(err)
	}
	history, err := s.GetNodeHistory(retree.NodeID(id))
	if err != nil {
		return jsonError(err)
	}
	if history == nil {
		history = []*retree.Node{}
	}
	return jsonResult(history)
}

// ── Migration ────────────────────────────────────────────────────

//export retree_migrate_storage
func retree_migrate_storage(handle uintptr, target *C.char) *C.char {
	s, err := getHandle(handle)
	if err != nil {
		return jsonError(err)
	}
	if err := s.MigrateStorageFormat(retree.StorageFormat(C.GoString(target))); err != nil {
		return jsonError(err)
	}
	return jsonResult(map[string]string{"format": string(s.StorageFormat())})
}

func splitCSV(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func splitNodeIDs(s string) []retree.NodeID {
	parts := splitCSV(s)
	out := make([]retree.NodeID, 0, len(parts))
	for _, p := range parts {
		id, err := strconv.ParseUint(p, 10, 64)
		if err != nil {
			continue
		}
		out = append(out, retree.NodeID(id))
	}
	return out
}

// main is required for c-shared build mode.
func main() {}
