package retree

import (
	"fmt"
	"path/filepath"
)

// metaPath returns the path to meta.json.
func (s *Store) metaPath() string { return filepath.Join(s.rootPath, "meta.json") }

// nodesDir returns the path to the nodes directory.
func (s *Store) nodesDir() string { return filepath.Join(s.rootPath, "nodes") }

// nodesBinPath returns the path to nodes.bin.
func (s *Store) nodesBinPath() string { return filepath.Join(s.rootPath, "nodes.bin") }

// nodesIdxPath returns the path to nodes.idx.
func (s *Store) nodesIdxPath() string { return filepath.Join(s.rootPath, "nodes.idx") }

// edgesPath returns the path to edges.jsonl.
func (s *Store) edgesPath() string { return filepath.Join(s.rootPath, "edges.jsonl") }

// nextIDPath returns the path to the next_id counter.
func (s *Store) nextIDPath() string { return filepath.Join(s.rootPath, "next_id") }

// lockPath returns the path to the lockfile.
func (s *Store) lockPath() string { return filepath.Join(s.rootPath, "lock") }

// alertsPath returns the path to alerts.jsonl.
func (s *Store) alertsPath() string { return filepath.Join(s.rootPath, "alerts.jsonl") }

// agentsPath returns the path to agents.json.
func (s *Store) agentsPath() string { return filepath.Join(s.rootPath, "agents.json") }

// resourcesPath returns the path to resources.json.
func (s *Store) resourcesPath() string { return filepath.Join(s.rootPath, "resources.json") }

// leasesPath returns the path to leases.json.
func (s *Store) leasesPath() string { return filepath.Join(s.rootPath, "leases.json") }

// relationsPath returns the path to relations.jsonl.
func (s *Store) relationsPath() string { return filepath.Join(s.rootPath, "relations.jsonl") }

// featuresPath returns the path to features.json.
func (s *Store) featuresPath() string { return filepath.Join(s.rootPath, "features.json") }

// featureEdgesPath returns the path to feature_edges.jsonl.
func (s *Store) featureEdgesPath() string { return filepath.Join(s.rootPath, "feature_edges.jsonl") }

// resourceEventsPath returns the path to resource_events.jsonl.
func (s *Store) resourceEventsPath() string {
	return filepath.Join(s.rootPath, "resource_events.jsonl")
}

// artifactsDir returns the path to the artifacts directory.
func (s *Store) artifactsDir() string { return filepath.Join(s.rootPath, "artifacts") }

// historyDir returns the path to the per-node edit history directory.
func (s *Store) historyDir() string { return filepath.Join(s.rootPath, "history") }

// snapshotsDir returns the path to the snapshot backup directory.
func (s *Store) snapshotsDir() string { return filepath.Join(s.rootPath, "snapshots") }

// manifestPath returns the path to the snapshot manifest.
func (s *Store) manifestPath() string { return filepath.Join(s.snapshotsDir(), "manifest.json") }

// nodeHistoryDir returns the path to the per-node history directory.
func (s *Store) nodeHistoryDir() string { return filepath.Join(s.historyDir(), "nodes") }

// snapshotPath returns the path for a snapshot archive by ID.
func (s *Store) snapshotPath(id string) string {
	return filepath.Join(s.snapshotsDir(), fmt.Sprintf("%s.tar.gz", id))
}
