package cmds

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/frudas24/research-tree/pkg/retree"
	"github.com/spf13/cobra"
)

// newResourceCmd constructs the "resource" subcommand group.
func newResourceCmd(opts *RootOptions) *cobra.Command {
	cmd := &cobra.Command{Use: "resource", Short: "Resource inventory and occupancy"}
	cmd.AddCommand(newResourceAddCmd(opts))
	cmd.AddCommand(newResourceEditCmd(opts))
	cmd.AddCommand(newResourceDeleteCmd(opts))
	cmd.AddCommand(newResourceListCmd(opts))
	cmd.AddCommand(newResourceShowCmd(opts))
	cmd.AddCommand(newResourceClaimCmd(opts))
	cmd.AddCommand(newResourceReleaseCmd(opts))
	cmd.AddCommand(newResourceReportCmd(opts))
	cmd.AddCommand(newResourceHistoryCmd(opts))
	return cmd
}

// newResourceAddCmd constructs the "resource add" subcommand.
func newResourceAddCmd(opts *RootOptions) *cobra.Command {
	var id, label, endpoint, endpointKind, kind, tagsCSV, osName, cpu, gpu, storageHint string
	var ramGB, vramGB, capacity int
	var enabled, maintenance bool
	cmd := &cobra.Command{
		Use:   "add",
		Short: "Add resource",
		RunE: func(cmd *cobra.Command, args []string) error {
			store, err := openStore(opts)
			if err != nil {
				return err
			}
			resource := retree.Resource{
				ID:           id,
				Label:        label,
				Endpoint:     endpoint,
				EndpointKind: retree.EndpointKind(strings.TrimSpace(endpointKind)),
				Kind:         retree.ResourceKind(strings.TrimSpace(kind)),
				Tags:         parseCSVStrings(tagsCSV),
				Enabled:      enabled,
				Maintenance:  maintenance,
				Capacity:     capacity,
				Spec: retree.ResourceSpec{
					OS:          osName,
					CPU:         cpu,
					RAMGB:       ramGB,
					GPU:         gpu,
					VRAMGB:      vramGB,
					StorageHint: storageHint,
				},
			}
			if err := store.CreateResource(resource); err != nil {
				return err
			}
			return printMaybeJSON(cmd, opts.OutputJSON, resource, fmt.Sprintf("created resource %s", resource.ID))
		},
	}
	bindResourceFlags(cmd, &id, &label, &endpoint, &endpointKind, &kind, &tagsCSV, &osName, &cpu, &gpu, &storageHint, &ramGB, &vramGB, &capacity, &enabled, &maintenance)
	_ = cmd.MarkFlagRequired("id")
	_ = cmd.MarkFlagRequired("label")
	return cmd
}

// newResourceEditCmd constructs the "resource edit" subcommand.
func newResourceEditCmd(opts *RootOptions) *cobra.Command {
	var label, endpoint, endpointKind, kind, tagsCSV, osName, cpu, gpu, storageHint string
	var ramGB, vramGB, capacity int
	var enabled, maintenance bool
	cmd := &cobra.Command{
		Use:   "edit <id>",
		Short: "Edit resource",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			store, err := openStore(opts)
			if err != nil {
				return err
			}
			resource, err := store.GetResource(args[0])
			if err != nil {
				return err
			}
			if cmd.Flags().Changed("label") {
				resource.Label = label
			}
			if cmd.Flags().Changed("endpoint") {
				resource.Endpoint = endpoint
			}
			if cmd.Flags().Changed("endpoint-kind") {
				resource.EndpointKind = retree.EndpointKind(strings.TrimSpace(endpointKind))
			}
			if cmd.Flags().Changed("kind") {
				resource.Kind = retree.ResourceKind(strings.TrimSpace(kind))
			}
			if cmd.Flags().Changed("tags") {
				resource.Tags = parseCSVStrings(tagsCSV)
			}
			if cmd.Flags().Changed("os") {
				resource.Spec.OS = osName
			}
			if cmd.Flags().Changed("cpu") {
				resource.Spec.CPU = cpu
			}
			if cmd.Flags().Changed("gpu") {
				resource.Spec.GPU = gpu
			}
			if cmd.Flags().Changed("storage-hint") {
				resource.Spec.StorageHint = storageHint
			}
			if cmd.Flags().Changed("ram-gb") {
				resource.Spec.RAMGB = ramGB
			}
			if cmd.Flags().Changed("vram-gb") {
				resource.Spec.VRAMGB = vramGB
			}
			if cmd.Flags().Changed("capacity") {
				resource.Capacity = capacity
			}
			if cmd.Flags().Changed("enabled") {
				resource.Enabled = enabled
			}
			if cmd.Flags().Changed("maintenance") {
				resource.Maintenance = maintenance
			}
			if err := store.UpdateResource(*resource); err != nil {
				return err
			}
			return printMaybeJSON(cmd, opts.OutputJSON, resource, fmt.Sprintf("updated resource %s", resource.ID))
		},
	}
	var id string
	bindResourceFlags(cmd, &id, &label, &endpoint, &endpointKind, &kind, &tagsCSV, &osName, &cpu, &gpu, &storageHint, &ramGB, &vramGB, &capacity, &enabled, &maintenance)
	return cmd
}

// newResourceDeleteCmd constructs the "resource delete" subcommand.
func newResourceDeleteCmd(opts *RootOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "delete <id>",
		Short: "Delete resource",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			store, err := openStore(opts)
			if err != nil {
				return err
			}
			if err := store.DeleteResource(args[0]); err != nil {
				return err
			}
			return printMaybeJSON(cmd, opts.OutputJSON, map[string]any{"deleted": args[0]}, fmt.Sprintf("deleted resource %s", args[0]))
		},
	}
	return cmd
}

// newResourceListCmd constructs the "resource list" subcommand.
func newResourceListCmd(opts *RootOptions) *cobra.Command {
	var freeOnly, usedOnly bool
	var tag, kind string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List resources",
		RunE: func(cmd *cobra.Command, args []string) error {
			store, err := openStore(opts)
			if err != nil {
				return err
			}
			resources, err := store.ListResources()
			if err != nil {
				return err
			}
			leases, err := store.ListResourceLeases()
			if err != nil {
				return err
			}
			leaseCount := map[string]int{}
			for _, lease := range leases {
				leaseCount[lease.ResourceID]++
			}
			filtered := make([]retree.Resource, 0, len(resources))
			for _, resource := range resources {
				if kind != "" && string(resource.Kind) != kind {
					continue
				}
				if tag != "" && !containsResourceTag(resource.Tags, tag) {
					continue
				}
				used := leaseCount[resource.ID] > 0
				if freeOnly && used {
					continue
				}
				if usedOnly && !used {
					continue
				}
				filtered = append(filtered, resource)
			}
			if opts.OutputJSON {
				return printMaybeJSON(cmd, true, map[string]any{"resources": filtered, "leases": leases}, "")
			}
			var b strings.Builder
			for _, resource := range filtered {
				state := "free"
				if resource.Maintenance {
					state = "maintenance"
				} else if !resource.Enabled {
					state = "disabled"
				} else if leaseCount[resource.ID] > 0 {
					state = "used"
				}
				b.WriteString(fmt.Sprintf("%s [%s] %s (%s)\n", resource.ID, resource.Kind, resource.Label, state))
			}
			return printMaybeJSON(cmd, false, nil, b.String())
		},
	}
	cmd.Flags().BoolVar(&freeOnly, "free", false, "Only list free resources")
	cmd.Flags().BoolVar(&usedOnly, "used", false, "Only list used resources")
	cmd.Flags().StringVar(&tag, "tag", "", "Filter by tag")
	cmd.Flags().StringVar(&kind, "kind", "", "Filter by kind")
	return cmd
}

// newResourceShowCmd constructs the "resource show" subcommand.
func newResourceShowCmd(opts *RootOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "show <id>",
		Short: "Show resource details",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			store, err := openStore(opts)
			if err != nil {
				return err
			}
			resource, err := store.GetResource(args[0])
			if err != nil {
				return err
			}
			leases, err := store.ListResourceLeases()
			if err != nil {
				return err
			}
			active := make([]retree.ResourceLease, 0)
			for _, lease := range leases {
				if lease.ResourceID == resource.ID {
					active = append(active, lease)
				}
			}
			events, err := store.GetResourceEvents(resource.ID)
			if err != nil {
				return err
			}
			if opts.OutputJSON {
				return printMaybeJSON(cmd, true, map[string]any{"resource": resource, "leases": active, "events": events}, "")
			}
			var b strings.Builder
			b.WriteString(fmt.Sprintf("%s [%s] %s\n", resource.ID, resource.Kind, resource.Label))
			if resource.Endpoint != "" {
				b.WriteString(fmt.Sprintf("  endpoint: %s (%s)\n", resource.Endpoint, resource.EndpointKind))
			}
			if len(resource.Tags) > 0 {
				b.WriteString(fmt.Sprintf("  tags: %s\n", strings.Join(resource.Tags, ", ")))
			}
			b.WriteString(fmt.Sprintf("  enabled: %t  maintenance: %t  capacity: %d\n", resource.Enabled, resource.Maintenance, resource.Capacity))
			if len(active) > 0 {
				b.WriteString("  active leases:\n")
				for _, lease := range active {
					b.WriteString(fmt.Sprintf("    - node %04d [%s] by %s\n", lease.NodeID, lease.Mode, lease.ClaimedBy))
				}
			}
			if len(events) > 0 {
				b.WriteString("  recent events:\n")
				start := 0
				if len(events) > 5 {
					start = len(events) - 5
				}
				for _, event := range events[start:] {
					b.WriteString(fmt.Sprintf("    - %s node %04d %s at %s\n", event.Action, event.NodeID, event.Mode, event.Timestamp.Format(time.RFC3339)))
				}
			}
			return printMaybeJSON(cmd, false, nil, b.String())
		},
	}
	return cmd
}

// newResourceClaimCmd constructs the "resource claim" subcommand.
func newResourceClaimCmd(opts *RootOptions) *cobra.Command {
	var mode, claimedBy, note string
	cmd := &cobra.Command{
		Use:   "claim <node-id> <resource-id>",
		Short: "Claim resource for a node",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			store, err := openStore(opts)
			if err != nil {
				return err
			}
			nodeID, err := parseNodeID(args[0])
			if err != nil {
				return err
			}
			lease := retree.ResourceLease{
				ResourceID: args[1],
				NodeID:     nodeID,
				Mode:       retree.LeaseMode(strings.TrimSpace(mode)),
				ClaimedBy:  claimedBy,
				Note:       note,
				ClaimedAt:  time.Now().UTC(),
			}
			if err := store.ClaimResource(lease); err != nil {
				return err
			}
			return printMaybeJSON(cmd, opts.OutputJSON, lease, fmt.Sprintf("claimed %s for node %04d", lease.ResourceID, lease.NodeID))
		},
	}
	cmd.Flags().StringVar(&mode, "mode", string(retree.LeaseExclusive), "Lease mode: exclusive|shared")
	cmd.Flags().StringVar(&claimedBy, "by", "", "Logical holder name (agent or human)")
	cmd.Flags().StringVar(&note, "note", "", "Optional note")
	return cmd
}

// newResourceReleaseCmd constructs the "resource release" subcommand.
func newResourceReleaseCmd(opts *RootOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "release <node-id> <resource-id>",
		Short: "Release resource lease",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			store, err := openStore(opts)
			if err != nil {
				return err
			}
			nodeID, err := parseNodeID(args[0])
			if err != nil {
				return err
			}
			if err := store.ReleaseResource(nodeID, args[1]); err != nil {
				return err
			}
			return printMaybeJSON(cmd, opts.OutputJSON, map[string]any{"node_id": nodeID, "resource_id": args[1]}, fmt.Sprintf("released %s from node %04d", args[1], nodeID))
		},
	}
	return cmd
}

// newResourceReportCmd constructs the "resource report" subcommand.
func newResourceReportCmd(opts *RootOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "report",
		Short: "Global resource occupancy report",
		RunE: func(cmd *cobra.Command, args []string) error {
			store, err := openStore(opts)
			if err != nil {
				return err
			}
			resources, err := store.ListResources()
			if err != nil {
				return err
			}
			leases, err := store.ListResourceLeases()
			if err != nil {
				return err
			}
			grouped := map[string][]retree.ResourceLease{}
			for _, lease := range leases {
				grouped[lease.ResourceID] = append(grouped[lease.ResourceID], lease)
			}
			if opts.OutputJSON {
				return printMaybeJSON(cmd, true, map[string]any{"resources": resources, "leases": leases}, "")
			}
			var free, used, maintenance, disabled int
			var b strings.Builder
			b.WriteString("Resources\n=========\n")
			for _, resource := range resources {
				switch {
				case !resource.Enabled:
					disabled++
				case resource.Maintenance:
					maintenance++
				case len(grouped[resource.ID]) > 0:
					used++
				default:
					free++
				}
			}
			b.WriteString(fmt.Sprintf("free: %d | used: %d | maintenance: %d | disabled: %d\n\n", free, used, maintenance, disabled))
			renderResourceSection(&b, "USED", resources, grouped, func(r retree.Resource) bool { return len(grouped[r.ID]) > 0 && r.Enabled && !r.Maintenance })
			renderResourceSection(&b, "FREE", resources, grouped, func(r retree.Resource) bool { return len(grouped[r.ID]) == 0 && r.Enabled && !r.Maintenance })
			renderResourceSection(&b, "MAINTENANCE", resources, grouped, func(r retree.Resource) bool { return r.Maintenance })
			renderResourceSection(&b, "DISABLED", resources, grouped, func(r retree.Resource) bool { return !r.Enabled })
			return printMaybeJSON(cmd, false, nil, b.String())
		},
	}
	return cmd
}

// newResourceHistoryCmd constructs the "resource history" subcommand.
func newResourceHistoryCmd(opts *RootOptions) *cobra.Command {
	var limit int
	cmd := &cobra.Command{
		Use:   "history <id>",
		Short: "Show lease history for a resource",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			store, err := openStore(opts)
			if err != nil {
				return err
			}
			events, err := store.GetResourceEvents(args[0])
			if err != nil {
				return err
			}
			if limit > 0 && len(events) > limit {
				events = events[len(events)-limit:]
			}
			if opts.OutputJSON {
				return printMaybeJSON(cmd, true, map[string]any{"resource_id": args[0], "events": events}, "")
			}
			if len(events) == 0 {
				return printMaybeJSON(cmd, false, nil, fmt.Sprintf("no history for resource %s", args[0]))
			}
			var b strings.Builder
			for _, event := range events {
				label := fmt.Sprintf("node %04d", event.NodeID)
				if node, err := store.GetNode(event.NodeID); err == nil {
					label = fmt.Sprintf("node %04d %q [%s]", event.NodeID, node.Title, node.Status)
				}
				by := ""
				if strings.TrimSpace(event.ClaimedBy) != "" {
					by = fmt.Sprintf(" by %s", event.ClaimedBy)
				}
				reason := ""
				if strings.TrimSpace(event.Reason) != "" {
					reason = fmt.Sprintf(" reason=%q", event.Reason)
				}
				b.WriteString(fmt.Sprintf("%s %s %s (%s)%s%s\n", event.Timestamp.Format(time.RFC3339), event.Action, label, event.Mode, by, reason))
			}
			return printMaybeJSON(cmd, false, nil, b.String())
		},
	}
	cmd.Flags().IntVar(&limit, "limit", 20, "Maximum number of recent events to show")
	return cmd
}

// bindResourceFlags wires the shared resource metadata flags into a command.
func bindResourceFlags(cmd *cobra.Command, id, label, endpoint, endpointKind, kind, tagsCSV, osName, cpu, gpu, storageHint *string, ramGB, vramGB, capacity *int, enabled, maintenance *bool) {
	cmd.Flags().StringVar(id, "id", "", "Stable resource identifier")
	cmd.Flags().StringVar(label, "label", "", "Human-readable label")
	cmd.Flags().StringVar(endpoint, "endpoint", "", "Technical endpoint (IP or DNS)")
	cmd.Flags().StringVar(endpointKind, "endpoint-kind", string(retree.EndpointNone), "Endpoint kind: none|ip|dns")
	cmd.Flags().StringVar(kind, "kind", string(retree.ResourceOther), "Kind: machine|gpu|cpu-slot|other")
	cmd.Flags().StringVar(tagsCSV, "tags", "", "Comma-separated tags")
	cmd.Flags().StringVar(osName, "os", "", "Operating system note")
	cmd.Flags().StringVar(cpu, "cpu", "", "CPU note")
	cmd.Flags().IntVar(ramGB, "ram-gb", 0, "RAM in GiB")
	cmd.Flags().StringVar(gpu, "gpu", "", "GPU note")
	cmd.Flags().IntVar(vramGB, "vram-gb", 0, "VRAM in GiB")
	cmd.Flags().StringVar(storageHint, "storage-hint", "", "Storage hint")
	cmd.Flags().IntVar(capacity, "capacity", 1, "Concurrent shared capacity")
	cmd.Flags().BoolVar(enabled, "enabled", true, "Whether the resource is schedulable")
	cmd.Flags().BoolVar(maintenance, "maintenance", false, "Whether the resource is temporarily unavailable")
}

// renderResourceSection prints one report section for a subset of resources.
func renderResourceSection(b *strings.Builder, title string, resources []retree.Resource, grouped map[string][]retree.ResourceLease, keep func(retree.Resource) bool) {
	items := make([]retree.Resource, 0)
	for _, resource := range resources {
		if keep(resource) {
			items = append(items, resource)
		}
	}
	if len(items) == 0 {
		return
	}
	sort.Slice(items, func(i, j int) bool { return items[i].ID < items[j].ID })
	b.WriteString(title + "\n")
	for _, resource := range items {
		b.WriteString(fmt.Sprintf("- %s [%s] %s\n", resource.ID, resource.Kind, resource.Label))
		for _, lease := range grouped[resource.ID] {
			b.WriteString(fmt.Sprintf("  node: #%04d mode=%s by=%s since=%s\n", lease.NodeID, lease.Mode, lease.ClaimedBy, lease.ClaimedAt.UTC().Format("2006-01-02 15:04 UTC")))
		}
	}
	b.WriteString("\n")
}

// containsResourceTag reports whether a resource tag slice contains a value.
func containsResourceTag(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}
