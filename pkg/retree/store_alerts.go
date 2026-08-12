package retree

import (
	"encoding/json"
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
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	enc := json.NewEncoder(f)
	for _, w := range warnings {
		if err := enc.Encode(w); err != nil {
			_ = f.Close()
			return err
		}
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
