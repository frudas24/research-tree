package retree

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type snapshotManifest struct {
	Latest    string         `json:"latest"`
	Snapshots []SnapshotMeta `json:"snapshots"`
}

// createSnapshot creates a tar.gz snapshot and enforces retention policy.
func (s *Store) createSnapshot(operation string) error {
	if err := os.MkdirAll(s.snapshotsDir(), 0o755); err != nil {
		return err
	}
	id := fmt.Sprintf("snapshot_%s", time.Now().UTC().Format("20060102_150405.000000000"))
	path := s.snapshotPath(id)
	if err := s.packSnapshot(path); err != nil {
		return err
	}
	h, err := fileSHA256(path)
	if err != nil {
		return err
	}
	manifest, _ := s.readManifest()
	manifest.Latest = id
	manifest.Snapshots = append(manifest.Snapshots, SnapshotMeta{
		ID:        id,
		CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
		Operation: operation,
		Hash:      h,
	})
	if len(manifest.Snapshots) > 3 {
		toDelete := manifest.Snapshots[:len(manifest.Snapshots)-3]
		for _, d := range toDelete {
			_ = os.Remove(s.snapshotPath(d.ID))
		}
		manifest.Snapshots = manifest.Snapshots[len(manifest.Snapshots)-3:]
	}
	return s.writeManifest(manifest)
}

// packSnapshot walks the root and creates a tar.gz archive.
func (s *Store) packSnapshot(dst string) error {
	f, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer f.Close()
	gz := gzip.NewWriter(f)
	defer gz.Close()
	tw := tar.NewWriter(gz)
	defer tw.Close()

	return filepath.Walk(s.rootPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(s.rootPath, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		if strings.HasPrefix(rel, "snapshots") {
			return nil
		}
		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		header.Name = filepath.ToSlash(rel)
		if err := tw.WriteHeader(header); err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		file, err := os.Open(path)
		if err != nil {
			return err
		}
		defer file.Close()
		_, err = io.Copy(tw, file)
		return err
	})
}

// fileSHA256 computes the SHA-256 hash of a file.
func fileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// readManifest reads the snapshot manifest.
func (s *Store) readManifest() (snapshotManifest, error) {
	b, err := os.ReadFile(s.manifestPath())
	if err != nil {
		if os.IsNotExist(err) {
			return snapshotManifest{}, nil
		}
		return snapshotManifest{}, err
	}
	var m snapshotManifest
	if err := json.Unmarshal(b, &m); err != nil {
		return snapshotManifest{}, err
	}
	return m, nil
}

// writeManifest atomically writes the snapshot manifest.
func (s *Store) writeManifest(m snapshotManifest) error {
	sort.Slice(m.Snapshots, func(i, j int) bool { return m.Snapshots[i].CreatedAt < m.Snapshots[j].CreatedAt })
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.manifestPath() + ".tmp"
	if err := os.WriteFile(tmp, append(b, '\n'), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, s.manifestPath())
}

// listSnapshots returns available snapshots sorted newest-first.
func (s *Store) listSnapshots() ([]SnapshotMeta, error) {
	m, err := s.readManifest()
	if err != nil {
		return nil, err
	}
	sort.Slice(m.Snapshots, func(i, j int) bool { return m.Snapshots[i].CreatedAt > m.Snapshots[j].CreatedAt })
	return m.Snapshots, nil
}

// restoreSnapshot restores a snapshot by ID, preserving history.
func (s *Store) restoreSnapshot(snapshotID string) error {
	return s.withLock("restore_snapshot", func() error {
		if err := s.createSnapshot("pre_restore"); err != nil {
			return err
		}
		snap := s.snapshotPath(snapshotID)
		if _, err := os.Stat(snap); err != nil {
			if os.IsNotExist(err) {
				return ErrNotFound
			}
			return err
		}
		tmpDir, err := os.MkdirTemp("", "retree-restore-")
		if err != nil {
			return err
		}
		defer os.RemoveAll(tmpDir)
		if err := untarGz(snap, tmpDir); err != nil {
			return err
		}
		entries, err := os.ReadDir(s.rootPath)
		if err != nil {
			return err
		}
		for _, e := range entries {
			if e.Name() == "history" || e.Name() == "snapshots" {
				continue
			}
			if err := os.RemoveAll(filepath.Join(s.rootPath, e.Name())); err != nil {
				return err
			}
		}
		return copyDir(tmpDir, s.rootPath)
	})
}

// untarGz extracts a tar.gz archive to the given directory.
func untarGz(src, dst string) error {
	f, err := os.Open(src)
	if err != nil {
		return err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		target := filepath.Join(dst, filepath.FromSlash(h.Name))
		if h.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		out, err := os.Create(target)
		if err != nil {
			return err
		}
		if _, err := io.Copy(out, tr); err != nil {
			_ = out.Close()
			return err
		}
		if err := out.Close(); err != nil {
			return err
		}
	}
	return nil
}

// copyDir recursively copies a directory tree.
func copyDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		return copyFile(path, target)
	})
}
