package retree

import (
	"archive/tar"
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// snapshotEntryNames returns the archive member names of a snapshot, failing if
// the archive cannot be read.
func snapshotEntryNames(t *testing.T, snapshotPath string) []string {
	t.Helper()
	f, err := os.Open(snapshotPath)
	mustNoErr(t, err)
	defer func() { _ = f.Close() }()
	gz, err := gzip.NewReader(f)
	mustNoErr(t, err)
	defer func() { _ = gz.Close() }()
	tr := tar.NewReader(gz)
	names := make([]string, 0, 16)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		mustNoErr(t, err)
		names = append(names, h.Name)
	}
	return names
}

// TestSnapshotOmitsInterruptedWriteStaging verifies that write-staging
// temporaries orphaned by an interrupted publication are not archived, while an
// embedded artifact that merely happens to be named *.tmp is preserved.
func TestSnapshotOmitsInterruptedWriteStaging(t *testing.T) {
	s := mustInit(t, StorageBIN)
	n := &Node{Frontmatter: Frontmatter{Title: "staging", Status: StatusActive}}
	mustNoErr(t, s.CreateNode(n))

	// Simulate the leftovers of a publication that crashed between staging and
	// rename, in both store-owned staging locations.
	mustNoErr(t, os.MkdirAll(s.nodesDir(), 0o755))
	staged := []string{
		filepath.Join(s.rootPath, "meta.json.tmp"),
		s.nodesBinPath() + ".tmp",
		s.nodesIdxPath() + ".pair.tmp",
		s.binaryDirtyPath() + ".tmp",
		s.binaryGenerationPath() + ".tmp",
		filepath.Join(s.nodesDir(), "0001.json.tmp"),
	}
	for _, path := range staged {
		mustNoErr(t, os.WriteFile(path, []byte("staged"), 0o644))
	}
	artifactDir := filepath.Join(s.artifactsDir(), "0001")
	mustNoErr(t, os.MkdirAll(artifactDir, 0o755))
	mustNoErr(t, os.WriteFile(filepath.Join(artifactDir, "capture.tmp"), []byte("artifact payload"), 0o644))

	mustNoErr(t, s.createSnapshot("staging_probe"))
	snaps, err := s.ListSnapshots()
	mustNoErr(t, err)
	if len(snaps) == 0 {
		t.Fatal("expected a snapshot")
	}
	names := snapshotEntryNames(t, s.snapshotPath(snaps[0].ID))

	keep := "artifacts/0001/capture.tmp"
	foundKeep := false
	foundPayload := false
	for _, name := range names {
		if name == keep {
			foundKeep = true
			continue
		}
		if strings.HasSuffix(name, ".tmp") {
			t.Fatalf("snapshot archived write-staging temporary %q", name)
		}
		if name == "nodes.bin" {
			foundPayload = true
		}
	}
	if !foundPayload {
		t.Fatalf("snapshot did not archive authoritative nodes.bin: %v", names)
	}
	if !foundKeep {
		t.Fatalf("snapshot dropped the embedded artifact %q: %v", keep, names)
	}
}
