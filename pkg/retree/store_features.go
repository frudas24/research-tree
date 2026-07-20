package retree

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
)

var slugRegexp = regexp.MustCompile(`[^a-z0-9]+`)

// Slugify normalizes a name into a URL-friendly slug.
func Slugify(name string) string {
	s := strings.ToLower(strings.TrimSpace(name))
	s = slugRegexp.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	return s
}

// nextFeatureID reads and bumps the next_id counter in features.json.
func (s *Store) nextFeatureID() (string, error) {
	payload, err := s.loadFeaturePayload()
	if err != nil {
		return "", err
	}
	id := payload.NextID
	payload.NextID++
	if err := s.saveFeaturePayload(payload); err != nil {
		return "", err
	}
	return fmt.Sprintf("f%04d", id), nil
}

// featurePayload is the top-level structure of features.json.
type featurePayload struct {
	NextID   int        `json:"next_id"`
	Features []*Feature `json:"features"`
}

// loadFeaturePayload reads features.json.
func (s *Store) loadFeaturePayload() (*featurePayload, error) {
	b, err := os.ReadFile(s.featuresPath())
	if err != nil {
		return nil, err
	}
	var p featurePayload
	if err := json.Unmarshal(b, &p); err != nil {
		return nil, err
	}
	if p.Features == nil {
		p.Features = []*Feature{}
	}
	return &p, nil
}

// saveFeaturePayload writes features.json atomically.
func (s *Store) saveFeaturePayload(p *featurePayload) error {
	b, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	tmp := s.featuresPath() + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, s.featuresPath())
}

// resolveFeatureID resolves a feature spec (id, slug, or name) to a canonical id.
func (s *Store) resolveFeatureID(spec string) (string, error) {
	spec = strings.TrimSpace(spec)
	if strings.HasPrefix(spec, "f") {
		return spec, nil
	}
	payload, err := s.loadFeaturePayload()
	if err != nil {
		return "", err
	}
	norm := Slugify(spec)
	for _, f := range payload.Features {
		if f.Slug == norm || Slugify(f.Name) == norm {
			return f.ID, nil
		}
	}
	return "", fmt.Errorf("feature %q not found", spec)
}

// CreateFeature creates a new feature and returns it.
func (s *Store) CreateFeature(name string, createdFrom NodeID) (*Feature, error) {
	return s.createFeature(name, createdFrom)
}

func (s *Store) createFeature(name string, createdFrom NodeID) (*Feature, error) {
	var created *Feature
	err := s.withLock("create_feature", func() error {
		_ = s.ensureFeaturesLayout()
		slug := Slugify(name)
		if slug == "" {
			return &FeatureError{msg: "slug is empty after normalization"}
		}
		payload, lerr := s.loadFeaturePayload()
		if lerr != nil {
			return lerr
		}
		for _, f := range payload.Features {
			if f.Slug == slug {
				return &FeatureError{msg: fmt.Sprintf("feature slug %q already exists", slug)}
			}
		}
		id, ierr := s.nextFeatureID()
		if ierr != nil {
			return ierr
		}
		f := &Feature{
			ID:          id,
			Name:        strings.TrimSpace(name),
			Slug:        slug,
			Status:      FeatureActive,
			CreatedFrom: createdFrom,
		}
		payload.Features = append(payload.Features, f)
		if serr := s.saveFeaturePayload(payload); serr != nil {
			return serr
		}
		created = f
		return nil
	})
	if err != nil {
		return nil, err
	}
	return created, nil
}

// GetFeature retrieves a feature by id, slug, or name.
func (s *Store) GetFeature(spec string) (*Feature, error) {
	id, err := s.resolveFeatureID(spec)
	if err != nil {
		return nil, err
	}
	payload, err := s.loadFeaturePayload()
	if err != nil {
		return nil, err
	}
	for _, f := range payload.Features {
		if f.ID == id {
			return f, nil
		}
	}
	return nil, ErrNotFound
}

// ListFeatures returns all features.
func (s *Store) ListFeatures() ([]*Feature, error) {
	payload, err := s.loadFeaturePayload()
	if err != nil {
		return nil, err
	}
	out := make([]*Feature, len(payload.Features))
	copy(out, payload.Features)
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

// LinkNodeToFeature links a node to a feature with the given role.
// Idempotent: if the node is already linked, the role is updated.
func (s *Store) LinkNodeToFeature(featureSpec string, nodeID NodeID, role FeatureNodeRole) error {
	fid, err := s.resolveFeatureID(featureSpec)
	if err != nil {
		return err
	}
	return s.withLock("link_node_feature", func() error {
		payload, err := s.loadFeaturePayload()
		if err != nil {
			return err
		}
		var target *Feature
		for _, f := range payload.Features {
			if f.ID == fid {
				target = f
				break
			}
		}
		if target == nil {
			return fmt.Errorf("feature %s: %w", fid, ErrNotFound)
		}
		// Check existing link — update role if found
		for i, n := range target.Nodes {
			if n.NodeID == nodeID {
				if n.Role == role {
					return nil // already linked with same role
				}
				target.Nodes[i].Role = role
				target.CurrentNodeMode = "derived"
				target.resolveCurrentNode()
				return s.saveFeaturePayload(payload)
			}
		}
		// New link
		target.Nodes = append(target.Nodes, FeatureLinkedNode{NodeID: nodeID, Role: role})
		target.CurrentNodeMode = "derived"
		target.resolveCurrentNode()
		return s.saveFeaturePayload(payload)
	})
}

// resolveCurrentNode derives current_node from the latest linked node
// with role implementation, fix, or decision.
func (f *Feature) resolveCurrentNode() {
	var latest NodeID
	for _, n := range f.Nodes {
		if currentNodeRoles[n.Role] && n.NodeID > latest {
			latest = n.NodeID
		}
	}
	f.CurrentNode = latest
}

// SetFeatureStatus updates the feature status.
func (s *Store) SetFeatureStatus(featureSpec string, status FeatureStatus) error {
	fid, err := s.resolveFeatureID(featureSpec)
	if err != nil {
		return err
	}
	return s.withLock("set_feature_status", func() error {
		payload, err := s.loadFeaturePayload()
		if err != nil {
			return err
		}
		for _, f := range payload.Features {
			if f.ID == fid {
				f.Status = status
				return s.saveFeaturePayload(payload)
			}
		}
		return ErrNotFound
	})
}

// SetFeatureCurrentNode sets the current_node explicitly.
func (s *Store) SetFeatureCurrentNode(featureSpec string, nodeID NodeID) error {
	fid, err := s.resolveFeatureID(featureSpec)
	if err != nil {
		return err
	}
	return s.withLock("set_feature_current", func() error {
		payload, err := s.loadFeaturePayload()
		if err != nil {
			return err
		}
		for _, f := range payload.Features {
			if f.ID == fid {
				f.CurrentNode = nodeID
				f.CurrentNodeMode = "explicit"
				return s.saveFeaturePayload(payload)
			}
		}
		return ErrNotFound
	})
}

// FeatureExists reports whether a feature spec resolves to an existing feature.
func (s *Store) FeatureExists(spec string) bool {
	_, err := s.resolveFeatureID(spec)
	return err == nil
}
