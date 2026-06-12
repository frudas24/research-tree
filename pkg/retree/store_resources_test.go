package retree

import (
	"errors"
	"os"
	"strings"
	"testing"
)

// TestResourceClaimAutoRelease verifies leases free automatically on done and paused transitions.
func TestResourceClaimAutoRelease(t *testing.T) {
	s := mustInit(t, StorageJSON)
	n := &Node{Frontmatter: Frontmatter{Title: "runner"}}
	mustNoErr(t, s.CreateNode(n))
	mustNoErr(t, s.CreateResource(Resource{
		ID:           "gpu-node-0",
		Label:        "gpu-node-0 gpu0",
		Kind:         ResourceGPU,
		Endpoint:     "10.0.0.14",
		EndpointKind: EndpointIP,
		Enabled:      true,
		Capacity:     1,
	}))
	mustNoErr(t, s.ClaimResource(ResourceLease{ResourceID: "gpu-node-0", NodeID: n.ID, Mode: LeaseExclusive, ClaimedBy: "codex"}))
	leases, err := s.GetNodeResourceLeases(n.ID)
	mustNoErr(t, err)
	if len(leases) != 1 {
		t.Fatalf("expected one active lease, got %d", len(leases))
	}
	node, err := s.GetNode(n.ID)
	mustNoErr(t, err)
	node.Status = StatusDone
	node.Outcome = OutcomeSuccess
	mustNoErr(t, s.UpdateNode(node))
	leases, err = s.GetNodeResourceLeases(n.ID)
	mustNoErr(t, err)
	if len(leases) != 0 {
		t.Fatalf("expected done node to auto-release leases, got %d", len(leases))
	}

	node, err = s.GetNode(n.ID)
	mustNoErr(t, err)
	node.Status = StatusActive
	node.Outcome = OutcomeUnset
	mustNoErr(t, s.UpdateNode(node))
	mustNoErr(t, s.ClaimResource(ResourceLease{ResourceID: "gpu-node-0", NodeID: n.ID, Mode: LeaseExclusive, ClaimedBy: "codex"}))
	node, err = s.GetNode(n.ID)
	mustNoErr(t, err)
	node.Status = StatusPaused
	mustNoErr(t, s.UpdateNode(node))
	leases, err = s.GetNodeResourceLeases(n.ID)
	mustNoErr(t, err)
	if len(leases) != 0 {
		t.Fatalf("expected paused node to auto-release leases, got %d", len(leases))
	}
}

// TestResourceClaimCapacity verifies exclusive/shared capacity enforcement.
func TestResourceClaimCapacity(t *testing.T) {
	s := mustInit(t, StorageJSON)
	for _, title := range []string{"a", "b", "c"} {
		mustNoErr(t, s.CreateNode(&Node{Frontmatter: Frontmatter{Title: title}}))
	}
	mustNoErr(t, s.CreateResource(Resource{
		ID:           "shared-gpu",
		Label:        "shared gpu",
		Kind:         ResourceGPU,
		Endpoint:     "gpu03.int.lab",
		EndpointKind: EndpointDNS,
		Enabled:      true,
		Capacity:     2,
	}))
	mustNoErr(t, s.ClaimResource(ResourceLease{ResourceID: "shared-gpu", NodeID: 1, Mode: LeaseShared}))
	mustNoErr(t, s.ClaimResource(ResourceLease{ResourceID: "shared-gpu", NodeID: 2, Mode: LeaseShared}))
	if err := s.ClaimResource(ResourceLease{ResourceID: "shared-gpu", NodeID: 3, Mode: LeaseShared}); !errors.Is(err, ErrResourceBusy) {
		t.Fatalf("expected shared capacity exhaustion, got %v", err)
	} else if !strings.Contains(err.Error(), "node 0001") || !strings.Contains(err.Error(), "node 0002") {
		t.Fatalf("expected busy error to include blockers, got %v", err)
	}
	mustNoErr(t, s.ReleaseResource(2, "shared-gpu"))
	if err := s.ClaimResource(ResourceLease{ResourceID: "shared-gpu", NodeID: 3, Mode: LeaseExclusive}); !errors.Is(err, ErrResourceBusy) {
		t.Fatalf("expected exclusive claim to fail while shared lease remains, got %v", err)
	}
}

// TestResourceDisabledAndMaintenance verifies disabled and maintenance are distinct failures.
func TestResourceDisabledAndMaintenance(t *testing.T) {
	s := mustInit(t, StorageJSON)
	mustNoErr(t, s.CreateNode(&Node{Frontmatter: Frontmatter{Title: "runner"}}))
	mustNoErr(t, s.CreateResource(Resource{
		ID:       "off-gpu",
		Label:    "off gpu",
		Kind:     ResourceGPU,
		Enabled:  false,
		Capacity: 1,
	}))
	mustNoErr(t, s.CreateResource(Resource{
		ID:          "maint-gpu",
		Label:       "maint gpu",
		Kind:        ResourceGPU,
		Enabled:     true,
		Maintenance: true,
		Capacity:    1,
	}))
	if err := s.ClaimResource(ResourceLease{ResourceID: "off-gpu", NodeID: 1, Mode: LeaseExclusive}); !errors.Is(err, ErrResourceDisabled) {
		t.Fatalf("expected disabled error, got %v", err)
	}
	if err := s.ClaimResource(ResourceLease{ResourceID: "maint-gpu", NodeID: 1, Mode: LeaseExclusive}); !errors.Is(err, ErrResourceMaintenance) {
		t.Fatalf("expected maintenance error, got %v", err)
	}
}

// TestResourceDeleteBusyDetails verifies delete errors name the blocking nodes.
func TestResourceDeleteBusyDetails(t *testing.T) {
	s := mustInit(t, StorageJSON)
	node := &Node{Frontmatter: Frontmatter{Title: "gpu run"}}
	mustNoErr(t, s.CreateNode(node))
	mustNoErr(t, s.CreateResource(Resource{
		ID:       "gpu-node-0",
		Label:    "gpu-node-0 gpu0",
		Kind:     ResourceGPU,
		Enabled:  true,
		Capacity: 1,
	}))
	mustNoErr(t, s.ClaimResource(ResourceLease{ResourceID: "gpu-node-0", NodeID: node.ID, Mode: LeaseExclusive, ClaimedBy: "codex"}))
	err := s.DeleteResource("gpu-node-0")
	if !errors.Is(err, ErrResourceBusy) {
		t.Fatalf("expected busy error, got %v", err)
	}
	if !strings.Contains(err.Error(), "gpu run") || !strings.Contains(err.Error(), "codex") {
		t.Fatalf("expected blocker details in error, got %v", err)
	}
}

// TestResourceEventsHistory verifies claim/release and auto-release write audit events.
func TestResourceEventsHistory(t *testing.T) {
	s := mustInit(t, StorageJSON)
	node := &Node{Frontmatter: Frontmatter{Title: "gpu run"}}
	mustNoErr(t, s.CreateNode(node))
	mustNoErr(t, s.CreateResource(Resource{
		ID:       "gpu-node-0",
		Label:    "gpu-node-0 gpu0",
		Kind:     ResourceGPU,
		Enabled:  true,
		Capacity: 1,
	}))
	mustNoErr(t, s.ClaimResource(ResourceLease{ResourceID: "gpu-node-0", NodeID: node.ID, Mode: LeaseExclusive, ClaimedBy: "codex"}))
	mustNoErr(t, s.ReleaseResource(node.ID, "gpu-node-0"))
	mustNoErr(t, s.ClaimResource(ResourceLease{ResourceID: "gpu-node-0", NodeID: node.ID, Mode: LeaseExclusive, ClaimedBy: "codex"}))
	fresh, err := s.GetNode(node.ID)
	mustNoErr(t, err)
	fresh.Status = StatusDone
	fresh.Outcome = OutcomeSuccess
	mustNoErr(t, s.UpdateNode(fresh))
	events, err := s.GetResourceEvents("gpu-node-0")
	mustNoErr(t, err)
	if len(events) != 4 {
		t.Fatalf("expected 4 events, got %d: %+v", len(events), events)
	}
	if events[0].Action != ResourceEventClaim || events[1].Action != ResourceEventRelease || events[3].Action != ResourceEventAutoReleaseDone {
		t.Fatalf("unexpected event sequence: %+v", events)
	}
}

// TestOpenBackfillsMissingResourceFiles verifies legacy stores created before
// resources existed are auto-healed on open.
func TestOpenBackfillsMissingResourceFiles(t *testing.T) {
	s := mustInit(t, StorageJSON)
	mustNoErr(t, os.Remove(s.resourcesPath()))
	mustNoErr(t, os.Remove(s.leasesPath()))
	mustNoErr(t, os.Remove(s.resourceEventsPath()))

	reopened, err := Open(s.rootPath)
	mustNoErr(t, err)

	resources, err := reopened.ListResources()
	mustNoErr(t, err)
	if len(resources) != 0 {
		t.Fatalf("expected empty resource inventory, got %d", len(resources))
	}
	leases, err := reopened.ListResourceLeases()
	mustNoErr(t, err)
	if len(leases) != 0 {
		t.Fatalf("expected empty leases, got %d", len(leases))
	}
	events, err := reopened.ListResourceEvents()
	mustNoErr(t, err)
	if len(events) != 0 {
		t.Fatalf("expected empty resource event log, got %d", len(events))
	}

	node := &Node{Frontmatter: Frontmatter{Title: "legacy run"}}
	mustNoErr(t, reopened.CreateNode(node))
	mustNoErr(t, reopened.CreateResource(Resource{
		ID:           "legacy-gpu0",
		Label:        "legacy gpu0",
		Kind:         ResourceGPU,
		Endpoint:     "10.0.0.88",
		EndpointKind: EndpointIP,
		Enabled:      true,
	}))
	mustNoErr(t, reopened.ClaimResource(ResourceLease{ResourceID: "legacy-gpu0", NodeID: node.ID, Mode: LeaseExclusive}))
	leases, err = reopened.GetNodeResourceLeases(node.ID)
	mustNoErr(t, err)
	if len(leases) != 1 || leases[0].ResourceID != "legacy-gpu0" {
		t.Fatalf("unexpected leases after backfill claim: %+v", leases)
	}
}
