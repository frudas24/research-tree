package retree

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type metaInfo struct {
	SchemaVersion SchemaVersion `json:"schema_version"`
	StorageFormat StorageFormat `json:"storage_format"`
	CreatedAt     time.Time     `json:"created_at"`
}

// openStore opens an existing research root at rootPath.
func openStore(rootPath string) (*Store, error) {
	s := &Store{rootPath: rootPath}
	meta, err := s.readMeta()
	if err != nil {
		return nil, err
	}
	if meta.SchemaVersion != CurrentSchemaVersion {
		return nil, fmt.Errorf("%w: got=%d want=%d", ErrUnsupportedSchema, meta.SchemaVersion, CurrentSchemaVersion)
	}
	s.format = meta.StorageFormat
	if s.format != StorageJSON && s.format != StorageBIN {
		return nil, fmt.Errorf("%w: unknown storage format %q", ErrInvalidNode, s.format)
	}
	if err := s.ensureResourceLayout(); err != nil {
		return nil, err
	}
	if err := s.ensureRelationsLayout(); err != nil {
		return nil, err
	}
	return s, nil
}

// initStore creates a new research root at rootPath with the given format.
func initStore(rootPath string, format StorageFormat) (*Store, error) {
	if format != StorageJSON && format != StorageBIN {
		return nil, fmt.Errorf("%w: unsupported format %q", ErrInvalidNode, format)
	}
	if err := os.MkdirAll(rootPath, 0o755); err != nil {
		return nil, err
	}
	s := &Store{rootPath: rootPath, format: format}
	if exists, err := s.isInitialized(); err != nil {
		return nil, err
	} else if exists {
		return nil, fmt.Errorf("%w: root already initialized", ErrInvalidNode)
	}
	if err := s.createLayout(); err != nil {
		return nil, err
	}
	meta := metaInfo{SchemaVersion: CurrentSchemaVersion, StorageFormat: format, CreatedAt: time.Now().UTC()}
	if err := s.writeMeta(meta); err != nil {
		return nil, err
	}
	if err := os.WriteFile(s.nextIDPath(), []byte("1\n"), 0o644); err != nil {
		return nil, err
	}
	if err := os.WriteFile(s.edgesPath(), nil, 0o644); err != nil {
		return nil, err
	}
	if err := os.WriteFile(s.relationsPath(), nil, 0o644); err != nil {
		return nil, err
	}
	if err := os.WriteFile(s.alertsPath(), nil, 0o644); err != nil {
		return nil, err
	}
	if err := os.WriteFile(s.agentsPath(), []byte("{\n  \"agent.local\": {\"name\": \"local\"}\n}\n"), 0o644); err != nil {
		return nil, err
	}
	if err := os.WriteFile(s.resourcesPath(), []byte("[]\n"), 0o644); err != nil {
		return nil, err
	}
	if err := os.WriteFile(s.leasesPath(), []byte("[]\n"), 0o644); err != nil {
		return nil, err
	}
	if err := os.WriteFile(s.resourceEventsPath(), nil, 0o644); err != nil {
		return nil, err
	}
	if format == StorageBIN {
		var headerBuf bytes.Buffer
		WriteBinHeader(&headerBuf)
		if err := os.WriteFile(s.nodesBinPath(), headerBuf.Bytes(), 0o644); err != nil {
			return nil, err
		}
		if err := os.WriteFile(s.nodesIdxPath(), []byte("{}\n"), 0o644); err != nil {
			return nil, err
		}
	}
	if err := s.createSnapshot("init"); err != nil {
		return nil, err
	}
	return s, nil
}

// isInitialized reports whether the root path already looks initialized.
func (s *Store) isInitialized() (bool, error) {
	if _, err := os.Stat(filepath.Join(s.rootPath, "meta.json")); err == nil {
		return true, nil
	} else if !os.IsNotExist(err) {
		return false, err
	}
	entries, err := os.ReadDir(s.rootPath)
	if err != nil {
		return false, err
	}
	for _, e := range entries {
		if !strings.HasPrefix(e.Name(), ".") {
			return true, nil
		}
	}
	return false, nil
}

// createLayout creates the directory structure for the research root.
// In binary mode, nodes/ is skipped — node data lives in nodes.bin.
func (s *Store) createLayout() error {
	dirs := []string{s.artifactsDir(), s.historyDir(), s.snapshotsDir()}
	if s.format == StorageJSON {
		dirs = append(dirs, s.nodesDir())
	}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	return nil
}

// ensureResourceLayout backfills resource inventory files for stores created
// before the resource-coordination feature existed.
func (s *Store) ensureResourceLayout() error {
	for _, path := range []string{s.resourcesPath(), s.leasesPath()} {
		if _, err := os.Stat(path); err == nil {
			continue
		} else if !os.IsNotExist(err) {
			return err
		}
		if err := os.WriteFile(path, []byte("[]\n"), 0o644); err != nil {
			return err
		}
	}
	if _, err := os.Stat(s.resourceEventsPath()); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := os.WriteFile(s.resourceEventsPath(), nil, 0o644); err != nil {
		return err
	}
	return nil
}

// ensureRelationsLayout backfills the relation index file for stores created
// before typed node relations existed.
func (s *Store) ensureRelationsLayout() error {
	if _, err := os.Stat(s.relationsPath()); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
	return os.WriteFile(s.relationsPath(), nil, 0o644)
}

// readMeta reads and parses meta.json.
func (s *Store) readMeta() (metaInfo, error) {
	b, err := os.ReadFile(s.metaPath())
	if err != nil {
		if os.IsNotExist(err) {
			return metaInfo{}, ErrNotFound
		}
		return metaInfo{}, err
	}
	var m metaInfo
	if err := json.Unmarshal(b, &m); err != nil {
		return metaInfo{}, err
	}
	return m, nil
}

// writeMeta atomically writes meta.json.
func (s *Store) writeMeta(m metaInfo) error {
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.metaPath() + ".tmp"
	if err := os.WriteFile(tmp, append(b, '\n'), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, s.metaPath())
}

// readNextID reads the next available node ID.
func (s *Store) readNextID() (NodeID, error) {
	b, err := os.ReadFile(s.nextIDPath())
	if err != nil {
		return 0, err
	}
	var next uint64
	if _, err := fmt.Sscanf(strings.TrimSpace(string(b)), "%d", &next); err != nil {
		return 0, err
	}
	return NodeID(next), nil
}

// writeNextID atomically writes the next ID counter.
func (s *Store) writeNextID(next NodeID) error {
	tmp := s.nextIDPath() + ".tmp"
	if err := os.WriteFile(tmp, []byte(fmt.Sprintf("%d\n", next)), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, s.nextIDPath())
}
