package retree

import (
	"errors"
	"os"
	"strings"
	"testing"
)

// TestStaleStoreReadsRejectMigration checks public and legacy read boundaries.
func TestStaleStoreReadsRejectMigration(t *testing.T) {
	for _, format := range []StorageFormat{StorageBIN, StorageJSON} {
		t.Run(string(format), func(t *testing.T) {
			s := mustInit(t, format)
			mustNoErr(t, s.CreateNode(&Node{Frontmatter: Frontmatter{Title: "present"}}))
			stale, err := Open(s.rootPath)
			mustNoErr(t, err)
			target := StorageBIN
			if format == StorageBIN {
				target = StorageJSON
			}
			mustNoErr(t, s.MigrateStorageFormat(target))
			reads := map[string]func() error{
				"query":  func() error { _, err := stale.QueryNodes(Filter{}); return err },
				"get":    func() error { _, err := stale.GetNode(1); return err },
				"roots":  func() error { _, err := stale.GetRoots(); return err },
				"legacy": func() error { _, err := stale.ScanLegacyDoneUnsetOutcomes(); return err },
			}
			for name, read := range reads {
				if err := read(); !errors.Is(err, ErrStaleStore) || errors.Is(err, ErrNotFound) {
					t.Errorf("%s: expected stale store error, got %v", name, err)
				}
			}
			fresh, err := Open(s.rootPath)
			mustNoErr(t, err)
			nodes, err := fresh.QueryNodes(Filter{})
			mustNoErr(t, err)
			if len(nodes) != 1 {
				t.Fatalf("fresh reader lost nodes: %d", len(nodes))
			}
			if _, err := fresh.GetNode(99); !errors.Is(err, ErrNotFound) {
				t.Fatalf("genuine absence: %v", err)
			}
		})
	}
}

// TestStorageReadRejectsMigrationDuringRead verifies that the trailing check
// discards both successful results and not-found errors from an obsolete read.
func TestStorageReadRejectsMigrationDuringRead(t *testing.T) {
	for _, format := range []StorageFormat{StorageBIN, StorageJSON} {
		for _, readErr := range []error{nil, ErrNotFound} {
			s := mustInit(t, format)
			mustNoErr(t, s.CreateNode(&Node{Frontmatter: Frontmatter{Title: "present"}}))
			reader, err := Open(s.rootPath)
			mustNoErr(t, err)
			target := StorageBIN
			if format == StorageBIN {
				target = StorageJSON
			}
			value, err := withCurrentStorageRead(reader, func() (int, error) {
				mustNoErr(t, s.MigrateStorageFormat(target))
				return 42, readErr
			})
			if value != 0 || !errors.Is(err, ErrStaleStore) {
				t.Fatalf("accepted obsolete read: value=%d err=%v", value, err)
			}
		}
	}
}

// TestStaleStoreRejectsWritesAfterMigration covers both format transitions.
func TestStaleStoreRejectsWritesAfterMigration(t *testing.T) {
	for _, format := range []StorageFormat{StorageBIN, StorageJSON} {
		t.Run(string(format), func(t *testing.T) {
			s := mustInit(t, format)
			mustNoErr(t, s.CreateNode(&Node{Frontmatter: Frontmatter{Title: "original"}}))
			stale, err := Open(s.rootPath)
			mustNoErr(t, err)
			target := StorageBIN
			if format == StorageBIN {
				target = StorageJSON
			}
			mustNoErr(t, s.MigrateStorageFormat(target))
			before, err := os.ReadFile(s.nextIDPath())
			mustNoErr(t, err)
			if err := stale.CreateNode(&Node{Frontmatter: Frontmatter{Title: "invisible"}}); err == nil || !strings.Contains(err.Error(), "reopen the store") {
				t.Fatalf("expected explicit stale handle rejection, got %v", err)
			}
			if err := stale.MigrateStorageFormat(format); err == nil {
				t.Fatal("stale migration reported success without checking metadata")
			}
			after, err := os.ReadFile(s.nextIDPath())
			mustNoErr(t, err)
			if string(before) != string(after) {
				t.Fatal("stale write advanced next_id")
			}
			fresh, err := Open(s.rootPath)
			mustNoErr(t, err)
			mustNoErr(t, fresh.CreateNode(&Node{Frontmatter: Frontmatter{Title: "visible"}}))
			nodes, err := fresh.QueryNodes(Filter{})
			mustNoErr(t, err)
			if len(nodes) != 2 {
				t.Fatalf("expected two visible nodes, got %d", len(nodes))
			}
		})
	}
}
