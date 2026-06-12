package retree

import (
	"fmt"
	"net"
	"regexp"
	"slices"
	"strings"
	"time"
)

var validNodeStatuses = []NodeStatus{
	StatusActive,
	StatusDone,
	StatusPaused,
}

var validOutcomes = []Outcome{
	OutcomeUnset,
	OutcomeSuccess,
	OutcomeFailure,
	OutcomeInconclusive,
}

var validClaimStatuses = []ClaimStatus{
	ClaimProvisional,
	ClaimValidated,
	ClaimInvalidated,
	ClaimSuperseded,
}

var validMilestoneClasses = []MilestoneClass{
	MilestoneNone,
	MilestoneGolden,
}

var validMilestoneKinds = []MilestoneKind{
	MilestoneKindNone,
	MilestoneKindChampion,
	MilestoneKindBreakthrough,
	MilestoneKindPivot,
}

var (
	validResourceKinds = []ResourceKind{ResourceMachine, ResourceGPU, ResourceCPUSlot, ResourceOther}
	validEndpointKinds = []EndpointKind{EndpointNone, EndpointIP, EndpointDNS}
	validLeaseModes    = []LeaseMode{LeaseExclusive, LeaseShared}
	dnsLabelPattern    = regexp.MustCompile(`^[A-Za-z0-9](?:[A-Za-z0-9-]{0,61}[A-Za-z0-9])?$`)
)

// ApplyNodeDefaults mutates n by filling optional fields with deterministic defaults.
func ApplyNodeDefaults(n *Node, now time.Time) {
	if n == nil {
		return
	}
	if n.SchemaVersion == 0 {
		n.SchemaVersion = CurrentSchemaVersion
	}
	if n.Status == "" {
		n.Status = StatusActive
	}
	if n.ClaimStatus == "" {
		n.ClaimStatus = ClaimProvisional
	}
	if n.Outcome == "" {
		n.Outcome = OutcomeUnset
	}
	if n.Created.IsZero() {
		n.Created = now.UTC()
	}
	if n.Modified.IsZero() {
		n.Modified = n.Created
	}
	if n.Revision == 0 {
		n.Revision = 1
	}
}

// ValidateNode validates a normalized node payload.
func ValidateNode(n *Node) error {
	if n == nil {
		return fmt.Errorf("%w: nil", ErrInvalidNode)
	}
	if n.Title == "" {
		return fmt.Errorf("%w: title required", ErrInvalidNode)
	}
	if n.SchemaVersion != CurrentSchemaVersion {
		return fmt.Errorf("%w: got=%d want=%d", ErrUnsupportedSchema, n.SchemaVersion, CurrentSchemaVersion)
	}
	if !slices.Contains(validNodeStatuses, n.Status) {
		return fmt.Errorf("%w: %q", ErrInvalidStatus, n.Status)
	}
	if !slices.Contains(validOutcomes, n.Outcome) {
		return fmt.Errorf("%w: outcome=%q", ErrInvalidNode, n.Outcome)
	}
	if !slices.Contains(validClaimStatuses, n.ClaimStatus) {
		return fmt.Errorf("%w: %q", ErrInvalidClaimStatus, n.ClaimStatus)
	}
	if !slices.Contains(validMilestoneClasses, n.MilestoneClass) {
		return fmt.Errorf("%w: milestone_class=%q", ErrInvalidNode, n.MilestoneClass)
	}
	if !slices.Contains(validMilestoneKinds, n.MilestoneKind) {
		return fmt.Errorf("%w: milestone_kind=%q", ErrInvalidNode, n.MilestoneKind)
	}
	if n.MilestoneClass == MilestoneNone {
		if n.MilestoneKind != MilestoneKindNone || strings.TrimSpace(n.MilestoneReason) != "" {
			return fmt.Errorf("%w: milestone_kind/reason require milestone_class", ErrInvalidNode)
		}
	}
	if n.MilestoneClass == MilestoneGolden && strings.TrimSpace(n.MilestoneReason) == "" {
		return fmt.Errorf("%w: golden milestone requires milestone_reason", ErrInvalidNode)
	}
	if n.ClaimStatus == ClaimInvalidated && len(n.InvalidatedBy) == 0 {
		return fmt.Errorf("%w: invalidated requires invalidated_by", ErrInvalidNode)
	}
	for _, pid := range n.Parents {
		if pid == 0 {
			return fmt.Errorf("%w: parent id cannot be 0", ErrInvalidNode)
		}
	}
	for _, id := range n.ContinuedBy {
		if id == 0 {
			return fmt.Errorf("%w: continued_by id cannot be 0", ErrInvalidNode)
		}
	}
	for _, id := range n.SupersededBy {
		if id == 0 {
			return fmt.Errorf("%w: superseded_by id cannot be 0", ErrInvalidNode)
		}
	}
	for _, a := range n.Artifacts {
		if err := ValidateArtifact(a); err != nil {
			return err
		}
	}
	for _, r := range n.Runs {
		if err := ValidateRunRecord(r); err != nil {
			return err
		}
	}
	return nil
}

// ValidateArtifact validates one artifact metadata record.
func ValidateArtifact(a Artifact) error {
	if a.Mode != ArtifactPath && a.Mode != ArtifactEmbedded {
		return fmt.Errorf("%w: mode=%q", ErrInvalidArtifact, a.Mode)
	}
	if a.Path == "" {
		return fmt.Errorf("%w: path required", ErrInvalidArtifact)
	}
	if a.Mode == ArtifactPath && a.Host == "" {
		return fmt.Errorf("%w: host required for path mode (use host like local, gpu-node-0, gpu-node-0)", ErrInvalidArtifact)
	}
	return nil
}

// ValidateResource validates a resource inventory record.
func ValidateResource(r Resource) error {
	if r.ID == "" {
		return fmt.Errorf("%w: id required", ErrInvalidResource)
	}
	if r.Label == "" {
		return fmt.Errorf("%w: label required", ErrInvalidResource)
	}
	if !slices.Contains(validResourceKinds, r.Kind) {
		return fmt.Errorf("%w: kind=%q", ErrInvalidResource, r.Kind)
	}
	if r.EndpointKind == "" {
		r.EndpointKind = EndpointNone
	}
	if !slices.Contains(validEndpointKinds, r.EndpointKind) {
		return fmt.Errorf("%w: endpoint_kind=%q", ErrInvalidResource, r.EndpointKind)
	}
	switch {
	case r.EndpointKind == EndpointNone:
		if r.Endpoint != "" {
			return fmt.Errorf("%w: endpoint requires endpoint_kind ip|dns", ErrInvalidResource)
		}
	case r.Endpoint == "":
		return fmt.Errorf("%w: endpoint required for endpoint_kind=%s", ErrInvalidResource, r.EndpointKind)
	default:
		switch r.EndpointKind {
		case EndpointIP:
			if net.ParseIP(r.Endpoint) == nil {
				return fmt.Errorf("%w: invalid ip endpoint %q", ErrInvalidResource, r.Endpoint)
			}
		case EndpointDNS:
			if !isValidDNSName(r.Endpoint) {
				return fmt.Errorf("%w: invalid dns endpoint %q", ErrInvalidResource, r.Endpoint)
			}
		}
	}
	if r.Capacity <= 0 {
		return fmt.Errorf("%w: capacity must be >= 1", ErrInvalidResource)
	}
	return nil
}

// ValidateLease validates a resource lease payload.
func ValidateLease(l ResourceLease) error {
	if l.ResourceID == "" {
		return fmt.Errorf("%w: resource_id required", ErrInvalidResource)
	}
	if l.NodeID == 0 {
		return fmt.Errorf("%w: node_id required", ErrInvalidResource)
	}
	if !slices.Contains(validLeaseModes, l.Mode) {
		return fmt.Errorf("%w: mode=%q", ErrInvalidResource, l.Mode)
	}
	return nil
}

// ValidateRunRecord validates a structured run record.
func ValidateRunRecord(r RunRecord) error {
	if r.EndpointKind == "" {
		r.EndpointKind = EndpointNone
	}
	if !slices.Contains(validEndpointKinds, r.EndpointKind) {
		return fmt.Errorf("%w: run endpoint_kind=%q", ErrInvalidNode, r.EndpointKind)
	}
	switch {
	case r.EndpointKind == EndpointNone:
		if r.Endpoint != "" {
			return fmt.Errorf("%w: run endpoint requires endpoint_kind ip|dns", ErrInvalidNode)
		}
	case r.Endpoint == "":
		return fmt.Errorf("%w: run endpoint required when endpoint_kind is set", ErrInvalidNode)
	default:
		switch r.EndpointKind {
		case EndpointIP:
			if net.ParseIP(r.Endpoint) == nil {
				return fmt.Errorf("%w: invalid run ip endpoint %q", ErrInvalidNode, r.Endpoint)
			}
		case EndpointDNS:
			if !isValidDNSName(r.Endpoint) {
				return fmt.Errorf("%w: invalid run dns endpoint %q", ErrInvalidNode, r.Endpoint)
			}
		}
	}
	return nil
}

// isValidDNSName validates a DNS/FQDN-style endpoint without resolving it.
func isValidDNSName(value string) bool {
	if value == "" || len(value) > 253 || !strings.Contains(value, ".") {
		return false
	}
	parts := strings.Split(value, ".")
	for _, part := range parts {
		if part == "" || !dnsLabelPattern.MatchString(part) {
			return false
		}
	}
	return true
}

// CloneNode returns a deep copy of n.
func CloneNode(n *Node) *Node {
	if n == nil {
		return nil
	}
	cpy := *n
	cpy.Parents = append([]NodeID(nil), n.Parents...)
	cpy.ContinuedBy = append([]NodeID(nil), n.ContinuedBy...)
	cpy.SupersededBy = append([]NodeID(nil), n.SupersededBy...)
	cpy.Tags = append([]string(nil), n.Tags...)
	cpy.Commits = append([]GitCommit(nil), n.Commits...)
	cpy.Runs = append([]RunRecord(nil), n.Runs...)
	cpy.Artifacts = append([]Artifact(nil), n.Artifacts...)
	cpy.InvalidatedBy = append([]NodeID(nil), n.InvalidatedBy...)
	return &cpy
}

// IsGolden reports whether the node is marked as a golden milestone.
func (n *Node) IsGolden() bool {
	return n != nil && n.MilestoneClass == MilestoneGolden
}
