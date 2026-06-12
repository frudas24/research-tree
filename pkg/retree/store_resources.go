package retree

import (
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"sort"
	"strings"
	"time"
)

// createResource persists a new resource inventory record.
func (s *Store) createResource(r Resource) error {
	return s.withLock("create_resource", func() error {
		ApplyResourceDefaults(&r, nowUTC())
		if err := ValidateResource(r); err != nil {
			return err
		}
		resources, err := s.readResources()
		if err != nil {
			return err
		}
		for _, existing := range resources {
			if strings.EqualFold(existing.ID, r.ID) {
				return fmt.Errorf("%w: duplicate resource id %q", ErrInvalidResource, r.ID)
			}
		}
		resources = append(resources, r)
		if err := s.writeResources(resources); err != nil {
			return err
		}
		return s.createSnapshot("create_resource")
	})
}

// updateResource persists modifications to an existing resource.
func (s *Store) updateResource(r Resource) error {
	return s.withLock("update_resource", func() error {
		resources, err := s.readResources()
		if err != nil {
			return err
		}
		found := false
		for i := range resources {
			if resources[i].ID != r.ID {
				continue
			}
			r.Created = resources[i].Created
			r.Modified = nowUTC()
			ApplyResourceDefaults(&r, r.Created)
			if err := ValidateResource(r); err != nil {
				return err
			}
			resources[i] = r
			found = true
			break
		}
		if !found {
			return ErrNotFound
		}
		if err := s.writeResources(resources); err != nil {
			return err
		}
		return s.createSnapshot("update_resource")
	})
}

// deleteResource removes a resource if it is not actively leased.
func (s *Store) deleteResource(id string) error {
	return s.withLock("delete_resource", func() error {
		leases, err := s.readLeases()
		if err != nil {
			return err
		}
		blockers := make([]ResourceLease, 0)
		for _, lease := range leases {
			if lease.ResourceID == id {
				blockers = append(blockers, lease)
			}
		}
		if len(blockers) > 0 {
			return fmt.Errorf("%w: resource %s is still leased by %s", ErrResourceBusy, id, s.describeLeases(blockers))
		}
		resources, err := s.readResources()
		if err != nil {
			return err
		}
		filtered := resources[:0]
		removed := false
		for _, resource := range resources {
			if resource.ID == id {
				removed = true
				continue
			}
			filtered = append(filtered, resource)
		}
		if !removed {
			return ErrNotFound
		}
		if err := s.writeResources(filtered); err != nil {
			return err
		}
		return s.createSnapshot("delete_resource")
	})
}

// getResource retrieves one resource by ID.
func (s *Store) getResource(id string) (*Resource, error) {
	resources, err := s.readResources()
	if err != nil {
		return nil, err
	}
	for _, resource := range resources {
		if resource.ID == id {
			copy := resource
			copy.Tags = append([]string(nil), resource.Tags...)
			return &copy, nil
		}
	}
	return nil, ErrNotFound
}

// listResources returns all resource records sorted by ID.
func (s *Store) listResources() ([]Resource, error) {
	resources, err := s.readResources()
	if err != nil {
		return nil, err
	}
	sort.Slice(resources, func(i, j int) bool { return resources[i].ID < resources[j].ID })
	return resources, nil
}

// claimResource creates or keeps an active lease when capacity allows it.
func (s *Store) claimResource(lease ResourceLease) error {
	return s.withLock("claim_resource", func() error {
		lease.ClaimedAt = nowUTC()
		if lease.Mode == "" {
			lease.Mode = LeaseExclusive
		}
		if err := ValidateLease(lease); err != nil {
			return err
		}
		resource, err := s.getResource(lease.ResourceID)
		if err != nil {
			return err
		}
		if !resource.Enabled {
			return fmt.Errorf("%w: %s", ErrResourceDisabled, lease.ResourceID)
		}
		if resource.Maintenance {
			return fmt.Errorf("%w: %s", ErrResourceMaintenance, lease.ResourceID)
		}
		node, err := s.GetNode(lease.NodeID)
		if err != nil {
			return err
		}
		if node.Status != StatusActive {
			return fmt.Errorf("%w: only active nodes can claim resources", ErrInvalidNode)
		}
		leases, err := s.readLeases()
		if err != nil {
			return err
		}
		for i := range leases {
			if leases[i].NodeID == lease.NodeID && leases[i].ResourceID == lease.ResourceID {
				if lease.ClaimedBy != "" {
					leases[i].ClaimedBy = lease.ClaimedBy
				}
				if lease.Note != "" {
					leases[i].Note = lease.Note
				}
				if lease.Mode != "" {
					leases[i].Mode = lease.Mode
				}
				if err := s.enforceLeaseCapacity(*resource, leases, leases[i]); err != nil {
					return err
				}
				if err := s.writeLeases(leases); err != nil {
					return err
				}
				if err := s.appendResourceEvent(ResourceEvent{
					ResourceID: lease.ResourceID,
					NodeID:     lease.NodeID,
					Action:     ResourceEventClaim,
					Mode:       leases[i].Mode,
					ClaimedBy:  leases[i].ClaimedBy,
					Note:       leases[i].Note,
					Timestamp:  nowUTC(),
				}); err != nil {
					return err
				}
				return s.createSnapshot("claim_resource")
			}
		}
		if err := s.enforceLeaseCapacity(*resource, leases, lease); err != nil {
			return err
		}
		leases = append(leases, lease)
		if err := s.writeLeases(leases); err != nil {
			return err
		}
		if err := s.appendResourceEvent(ResourceEvent{
			ResourceID: lease.ResourceID,
			NodeID:     lease.NodeID,
			Action:     ResourceEventClaim,
			Mode:       lease.Mode,
			ClaimedBy:  lease.ClaimedBy,
			Note:       lease.Note,
			Timestamp:  lease.ClaimedAt,
		}); err != nil {
			return err
		}
		return s.createSnapshot("claim_resource")
	})
}

// releaseResource removes one active lease if present.
func (s *Store) releaseResource(nodeID NodeID, resourceID string) error {
	return s.withLock("release_resource", func() error {
		leases, err := s.readLeases()
		if err != nil {
			return err
		}
		filtered := leases[:0]
		removed := false
		var released *ResourceLease
		for _, lease := range leases {
			if lease.NodeID == nodeID && lease.ResourceID == resourceID {
				removed = true
				leaseCopy := lease
				released = &leaseCopy
				continue
			}
			filtered = append(filtered, lease)
		}
		if !removed {
			return nil
		}
		if err := s.writeLeases(filtered); err != nil {
			return err
		}
		if released != nil {
			if err := s.appendResourceEvent(ResourceEvent{
				ResourceID: released.ResourceID,
				NodeID:     released.NodeID,
				Action:     ResourceEventRelease,
				Mode:       released.Mode,
				ClaimedBy:  released.ClaimedBy,
				Note:       released.Note,
				Timestamp:  nowUTC(),
			}); err != nil {
				return err
			}
		}
		return s.createSnapshot("release_resource")
	})
}

// getNodeResourceLeases returns all active leases held by a node.
func (s *Store) getNodeResourceLeases(nodeID NodeID) ([]ResourceLease, error) {
	leases, err := s.readLeases()
	if err != nil {
		return nil, err
	}
	out := make([]ResourceLease, 0)
	for _, lease := range leases {
		if lease.NodeID == nodeID {
			out = append(out, lease)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ResourceID < out[j].ResourceID })
	return out, nil
}

// listResourceLeases returns all active leases sorted by resource then node.
func (s *Store) listResourceLeases() ([]ResourceLease, error) {
	leases, err := s.readLeases()
	if err != nil {
		return nil, err
	}
	sort.Slice(leases, func(i, j int) bool {
		if leases[i].ResourceID == leases[j].ResourceID {
			return leases[i].NodeID < leases[j].NodeID
		}
		return leases[i].ResourceID < leases[j].ResourceID
	})
	return leases, nil
}

// listResourceEvents returns all historical resource occupancy events.
func (s *Store) listResourceEvents() ([]ResourceEvent, error) {
	events, err := readJSONLines[ResourceEvent](s.resourceEventsPath())
	if err != nil {
		return nil, err
	}
	sort.Slice(events, func(i, j int) bool {
		if events[i].Timestamp.Equal(events[j].Timestamp) {
			if events[i].ResourceID == events[j].ResourceID {
				return events[i].NodeID < events[j].NodeID
			}
			return events[i].ResourceID < events[j].ResourceID
		}
		return events[i].Timestamp.Before(events[j].Timestamp)
	})
	return events, nil
}

// getResourceEvents returns historical occupancy events for one resource.
func (s *Store) getResourceEvents(resourceID string) ([]ResourceEvent, error) {
	events, err := s.listResourceEvents()
	if err != nil {
		return nil, err
	}
	out := make([]ResourceEvent, 0)
	for _, event := range events {
		if event.ResourceID == resourceID {
			out = append(out, event)
		}
	}
	return out, nil
}

// releaseNodeResourcesUnlocked removes all active leases for a node.
func (s *Store) releaseNodeResourcesUnlocked(nodeID NodeID, action ResourceEventAction) error {
	leases, err := s.readLeases()
	if err != nil {
		return err
	}
	filtered := leases[:0]
	events := make([]ResourceEvent, 0)
	removed := false
	for _, lease := range leases {
		if lease.NodeID == nodeID {
			removed = true
			events = append(events, ResourceEvent{
				ResourceID: lease.ResourceID,
				NodeID:     lease.NodeID,
				Action:     action,
				Mode:       lease.Mode,
				ClaimedBy:  lease.ClaimedBy,
				Note:       lease.Note,
				Timestamp:  nowUTC(),
			})
			continue
		}
		filtered = append(filtered, lease)
	}
	if !removed {
		return nil
	}
	if err := s.writeLeases(filtered); err != nil {
		return err
	}
	for _, event := range events {
		if err := s.appendResourceEvent(event); err != nil {
			return err
		}
	}
	return nil
}

// ApplyResourceDefaults fills default resource fields deterministically.
func ApplyResourceDefaults(r *Resource, now time.Time) {
	if r == nil {
		return
	}
	if r.Kind == "" {
		r.Kind = ResourceOther
	}
	if r.EndpointKind == "" {
		r.EndpointKind = EndpointNone
	}
	if r.Capacity == 0 {
		r.Capacity = 1
	}
	if r.Created.IsZero() {
		r.Created = now.UTC()
	}
	if r.Modified.IsZero() {
		r.Modified = r.Created
	}
	r.Tags = uniqueStrings(r.Tags)
}

// readResources loads the persisted resource inventory.
func (s *Store) readResources() ([]Resource, error) {
	data, err := os.ReadFile(s.resourcesPath())
	if err != nil {
		return nil, err
	}
	if len(strings.TrimSpace(string(data))) == 0 {
		return []Resource{}, nil
	}
	var resources []Resource
	if err := json.Unmarshal(data, &resources); err != nil {
		return nil, err
	}
	for i := range resources {
		ApplyResourceDefaults(&resources[i], resources[i].Created)
	}
	return resources, nil
}

// writeResources atomically writes the resource inventory file.
func (s *Store) writeResources(resources []Resource) error {
	sort.Slice(resources, func(i, j int) bool { return resources[i].ID < resources[j].ID })
	data, err := json.MarshalIndent(resources, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.resourcesPath() + ".tmp"
	if err := os.WriteFile(tmp, append(data, '\n'), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, s.resourcesPath())
}

// readLeases loads the persisted active lease set.
func (s *Store) readLeases() ([]ResourceLease, error) {
	data, err := os.ReadFile(s.leasesPath())
	if err != nil {
		return nil, err
	}
	if len(strings.TrimSpace(string(data))) == 0 {
		return []ResourceLease{}, nil
	}
	var leases []ResourceLease
	if err := json.Unmarshal(data, &leases); err != nil {
		return nil, err
	}
	return leases, nil
}

// writeLeases atomically writes the lease file.
func (s *Store) writeLeases(leases []ResourceLease) error {
	sort.Slice(leases, func(i, j int) bool {
		if leases[i].ResourceID == leases[j].ResourceID {
			return leases[i].NodeID < leases[j].NodeID
		}
		return leases[i].ResourceID < leases[j].ResourceID
	})
	data, err := json.MarshalIndent(leases, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.leasesPath() + ".tmp"
	if err := os.WriteFile(tmp, append(data, '\n'), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, s.leasesPath())
}

// appendResourceEvent appends one lease lifecycle event to the audit log.
func (s *Store) appendResourceEvent(event ResourceEvent) error {
	if event.Timestamp.IsZero() {
		event.Timestamp = nowUTC()
	}
	return appendJSONLine(s.resourceEventsPath(), event)
}

// enforceLeaseCapacity rejects claims that exceed a resource's current occupancy rules.
func (s *Store) enforceLeaseCapacity(resource Resource, leases []ResourceLease, candidate ResourceLease) error {
	if candidate.Mode == "" {
		candidate.Mode = LeaseExclusive
	}
	active := make([]ResourceLease, 0)
	for _, lease := range leases {
		if lease.ResourceID == resource.ID && !(lease.NodeID == candidate.NodeID && lease.ResourceID == candidate.ResourceID) {
			active = append(active, lease)
		}
	}
	if candidate.Mode == LeaseExclusive {
		if len(active) > 0 {
			return fmt.Errorf("%w: resource %s already leased by %s", ErrResourceBusy, resource.ID, s.describeLeases(active))
		}
		return nil
	}
	if slices.ContainsFunc(active, func(existing ResourceLease) bool { return existing.Mode == LeaseExclusive }) {
		return fmt.Errorf("%w: resource %s held exclusively by %s", ErrResourceBusy, resource.ID, s.describeLeases(active))
	}
	if len(active) >= resource.Capacity {
		return fmt.Errorf("%w: resource %s capacity exhausted by %s", ErrResourceBusy, resource.ID, s.describeLeases(active))
	}
	return nil
}

// describeLeases renders leases with node context for human-facing errors.
func (s *Store) describeLeases(leases []ResourceLease) string {
	parts := make([]string, 0, len(leases))
	for _, lease := range leases {
		label := fmt.Sprintf("node %04d", lease.NodeID)
		if node, err := s.GetNode(lease.NodeID); err == nil {
			label = fmt.Sprintf("node %04d %q [%s]", lease.NodeID, node.Title, node.Status)
		}
		holder := ""
		if strings.TrimSpace(lease.ClaimedBy) != "" {
			holder = fmt.Sprintf(" by %s", lease.ClaimedBy)
		}
		since := ""
		if !lease.ClaimedAt.IsZero() {
			since = fmt.Sprintf(" since %s", lease.ClaimedAt.Format(time.RFC3339))
		}
		parts = append(parts, fmt.Sprintf("%s (%s%s%s)", label, lease.Mode, holder, since))
	}
	return strings.Join(parts, ", ")
}
