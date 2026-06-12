package retree

import (
	"errors"
	"os"
	"sort"
	"time"
)

// listBranchWarnings returns warnings filtered by agent and ack status.
func (s *Store) listBranchWarnings(agent string, onlyUnacked bool) ([]BranchWarning, error) {
	warnings, err := readJSONLines[BranchWarning](s.alertsPath())
	if err != nil {
		return nil, err
	}
	out := make([]BranchWarning, 0, len(warnings))
	for _, w := range warnings {
		if agent != "" && w.Agent != agent {
			continue
		}
		if onlyUnacked && w.AckedAt != nil {
			continue
		}
		out = append(out, w)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out, nil
}

// ackBranchWarning marks a warning as acknowledged.
func (s *Store) ackBranchWarning(warningID string) error {
	return s.withLock("ack_warning", func() error {
		warnings, err := readJSONLines[BranchWarning](s.alertsPath())
		if err != nil {
			return err
		}
		found := false
		now := time.Now().UTC()
		for i := range warnings {
			if warnings[i].ID != warningID {
				continue
			}
			found = true
			if warnings[i].AckedAt == nil {
				warnings[i].AckedAt = &now
			}
		}
		if !found {
			return ErrNotFound
		}
		return rewriteAlerts(s.alertsPath(), warnings)
	})
}

// rewriteAlerts atomically rewrites the alerts file.
func rewriteAlerts(path string, warnings []BranchWarning) error {
	tmp := path + ".tmp"
	if err := os.RemoveAll(tmp); err != nil {
		return err
	}
	for _, w := range warnings {
		if err := appendJSONLine(tmp, w); err != nil {
			if !errors.Is(err, os.ErrNotExist) {
				return err
			}
		}
	}
	if _, err := os.Stat(tmp); errors.Is(err, os.ErrNotExist) {
		if err := os.WriteFile(tmp, nil, 0o644); err != nil {
			return err
		}
	}
	return os.Rename(tmp, path)
}
