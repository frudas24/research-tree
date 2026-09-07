package retree

import (
	"os"
	"path/filepath"
	"testing"
)

// TestOpenPreservesEmbeddedStagingLikeName protects published user filenames.
func TestOpenPreservesEmbeddedStagingLikeName(t *testing.T) {
	for _, format := range []StorageFormat{StorageBIN, StorageJSON} {
		t.Run(string(format), func(t *testing.T) {
			s := mustInit(t, format)
			n := &Node{Frontmatter: Frontmatter{Title: "evidence"}}
			mustNoErr(t, s.CreateNode(n))
			source := filepath.Join(t.TempDir(), ".embed-evidence.tmp")
			mustNoErr(t, os.WriteFile(source, []byte("evidence"), 0o600))
			mustNoErr(t, s.EmbedArtifact(n.ID, source, "published"))
			n, err := s.GetNode(n.ID)
			mustNoErr(t, err)
			_, err = Open(s.rootPath)
			mustNoErr(t, err)
			data, err := os.ReadFile(filepath.Join(s.rootPath, n.Artifacts[0].Path))
			mustNoErr(t, err)
			if string(data) != "evidence" {
				t.Fatal("artifact changed")
			}
		})
	}
}
