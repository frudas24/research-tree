package retree

import "errors"

var (
	// ErrNotFound is returned when a node or resource does not exist.
	ErrNotFound = errors.New("not found")
	// ErrUnsupportedSchema is returned when schema_version is not supported.
	ErrUnsupportedSchema = errors.New("unsupported schema")
	// ErrInvalidNode is returned when a node payload is invalid.
	ErrInvalidNode = errors.New("invalid node")
	// ErrInvalidStatus is returned when status is unknown.
	ErrInvalidStatus = errors.New("invalid status")
	// ErrInvalidClaimStatus is returned when claim status is unknown.
	ErrInvalidClaimStatus = errors.New("invalid claim status")
	// ErrInvalidArtifact is returned when artifact metadata is invalid.
	ErrInvalidArtifact = errors.New("invalid artifact")
	// ErrDuplicateID is returned when adding an existing node ID.
	ErrDuplicateID = errors.New("duplicate node id")
	// ErrCycleDetected is returned when an edge would introduce a cycle.
	ErrCycleDetected = errors.New("cycle detected")
	// ErrHasChildren is returned when deleting a node that still has children.
	ErrHasChildren = errors.New("node has children")
	// ErrInvalidResource is returned when a resource payload is invalid.
	ErrInvalidResource = errors.New("invalid resource")
	// ErrResourceBusy is returned when a resource cannot accept more leases.
	ErrResourceBusy = errors.New("resource busy")
	// ErrResourceDisabled is returned when a resource is disabled.
	ErrResourceDisabled = errors.New("resource disabled")
	// ErrResourceMaintenance is returned when a resource is under maintenance.
	ErrResourceMaintenance = errors.New("resource in maintenance")
)
