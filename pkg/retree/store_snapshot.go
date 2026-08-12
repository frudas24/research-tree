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
	return s.createSnapshotProtected(operation, nil)
}

// bestEffortSnapshot records a post-mutation snapshot without surfacing a late
// failure as if the mutation itself had failed.
func (s *Store) bestEffortSnapshot(operation string) {
	_ = s.createSnapshot(operation)
}

// ensureSnapshotCatalogHealthy validates the manifest before a mutation commits
// so a present-but-corrupt catalog fails early instead of being silently
// replaced or only surfacing as a late post-commit snapshot error.
func (s *Store) ensureSnapshotCatalogHealthy() error {
	_, err := s.readManifestStrict()
	return err
}

// createSnapshotProtected creates a tar.gz snapshot while preventing retention
// from deleting any explicitly protected snapshot IDs.
func (s *Store) createSnapshotProtected(operation string, protect map[string]struct{}) error {
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
	manifest, err := s.readManifestStrict()
	if err != nil {
		return err
	}
	manifest.Latest = id
	manifest.Snapshots = append(manifest.Snapshots, SnapshotMeta{
		ID:        id,
		CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
		Operation: operation,
		Hash:      h,
	})
	dropped := make([]string, 0)
	for len(manifest.Snapshots) > 3 {
		drop := -1
		for i, snap := range manifest.Snapshots {
			if protect != nil {
				if _, keep := protect[snap.ID]; keep {
					continue
				}
			}
			drop = i
			break
		}
		if drop == -1 {
			break
		}
		dropped = append(dropped, manifest.Snapshots[drop].ID)
		manifest.Snapshots = append(manifest.Snapshots[:drop], manifest.Snapshots[drop+1:]...)
	}
	if err := s.writeManifest(manifest); err != nil {
		return err
	}
	for _, id := range dropped {
		_ = os.Remove(s.snapshotPath(id))
	}
	return nil
}

// packSnapshot walks the root and creates a tar.gz archive.
func (s *Store) packSnapshot(dst string) error {
	f, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	gz := gzip.NewWriter(f)
	defer func() { _ = gz.Close() }()
	tw := tar.NewWriter(gz)
	defer func() { _ = tw.Close() }()

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
		if strings.HasPrefix(rel, "snapshots") || rel == "lock" {
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
		defer func() { _ = file.Close() }()
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
	defer func() { _ = f.Close() }()
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
		meta, err := s.snapshotMeta(snapshotID)
		if err != nil {
			return err
		}
		if err := s.createSnapshotProtected("pre_restore", map[string]struct{}{meta.ID: {}}); err != nil {
			return err
		}
		snap := s.snapshotPath(meta.ID)
		if _, err := os.Stat(snap); err != nil {
			if os.IsNotExist(err) {
				return ErrNotFound
			}
			return err
		}
		hash, err := fileSHA256(snap)
		if err != nil {
			return err
		}
		if hash != meta.Hash {
			return fmt.Errorf("%w: snapshot %s hash mismatch", ErrInvalidNode, meta.ID)
		}
		tmpDir, err := os.MkdirTemp("", "retree-restore-")
		if err != nil {
			return err
		}
		defer func() { _ = os.RemoveAll(tmpDir) }()
		if err := untarGz(snap, tmpDir); err != nil {
			return err
		}
		_ = os.Remove(filepath.Join(tmpDir, "lock"))
		parentDir := filepath.Dir(s.rootPath)
		stagingDir, err := os.MkdirTemp(parentDir, ".retree-restore-new-")
		if err != nil {
			return err
		}
		rollbackDir := filepath.Join(parentDir, "."+filepath.Base(s.rootPath)+".restore-backup-"+time.Now().UTC().Format("20060102_150405.000000000"))
		success := false
		defer func() {
			if !success {
				_ = os.RemoveAll(stagingDir)
				_ = os.RemoveAll(rollbackDir)
			}
		}()
		if err := copyDir(tmpDir, stagingDir); err != nil {
			return err
		}
		if err := copyIfExists(s.historyDir(), filepath.Join(stagingDir, "history")); err != nil {
			return err
		}
		if err := copyIfExists(s.snapshotsDir(), filepath.Join(stagingDir, "snapshots")); err != nil {
			return err
		}
		lock, err := s.readLockInfo()
		if err != nil && !os.IsNotExist(err) {
			return err
		}
		if err == nil {
			if err := writeLockFile(filepath.Join(stagingDir, "lock"), lock); err != nil {
				return err
			}
		}
		staged, err := openStore(stagingDir)
		if err != nil {
			return err
		}
		if _, err := staged.loadGraph(); err != nil {
			return err
		}
		if err := os.Rename(s.rootPath, rollbackDir); err != nil {
			return err
		}
		if err := os.Rename(stagingDir, s.rootPath); err != nil {
			_ = os.Rename(rollbackDir, s.rootPath)
			return err
		}
		success = true
		return os.RemoveAll(rollbackDir)
	})
}

// untarGz extracts a tar.gz archive to the given directory.
// Entries must be relative paths without ".." segments; anything else
// (absolute paths, symlinks, hardlinks, devices) is rejected or skipped so
// a crafted archive cannot write outside dst.
func untarGz(src, dst string) error {
	f, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer func() { _ = gz.Close() }()
	tr := tar.NewReader(gz)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		if !isSafeArchivePath(h.Name) {
			return fmt.Errorf("unsafe archive entry %q: absolute or .. path", h.Name)
		}
		target := filepath.Join(dst, filepath.FromSlash(h.Name))
		switch h.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
		case tar.TypeReg:
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
		default:
			// Symlinks, hardlinks, and special files are never materialized.
			continue
		}
	}
	return nil
}

// snapshotMeta resolves one snapshot ID from the persisted manifest.
func (s *Store) snapshotMeta(snapshotID string) (SnapshotMeta, error) {
	manifest, err := s.readManifestStrict()
	if err != nil {
		return SnapshotMeta{}, err
	}
	for _, meta := range manifest.Snapshots {
		if meta.ID == snapshotID {
			return meta, nil
		}
	}
	return SnapshotMeta{}, ErrNotFound
}

func (s *Store) readManifestStrict() (snapshotManifest, error) {
	if _, err := os.Stat(s.manifestPath()); err == nil {
		return s.readManifest()
	} else if os.IsNotExist(err) {
		return snapshotManifest{}, nil
	} else {
		return snapshotManifest{}, err
	}
}

// isSafeArchivePath rejects absolute paths and any path containing a ".."
// segment, preventing archive entries from escaping the extraction root.
func isSafeArchivePath(name string) bool {
	if filepath.IsAbs(name) {
		return false
	}
	for _, part := range strings.Split(filepath.ToSlash(name), "/") {
		if part == ".." {
			return false
		}
	}
	return true
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

func copyIfExists(src, dst string) error {
	if _, err := os.Stat(src); err == nil {
		return copyDir(src, dst)
	} else if os.IsNotExist(err) {
		return nil
	} else {
		return err
	}
}

func writeLockFile(path string, info lockInfo) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(formatLockInfo(info)), 0o644)
}
