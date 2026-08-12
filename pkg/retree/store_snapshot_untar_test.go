package retree

import (
	"archive/tar"
	"compress/gzip"
	"os"
	"path/filepath"
	"testing"
)

// writeTarGz writes entries to a tar.gz at path.
func writeTarGz(t *testing.T, path string, entries []struct {
	name   string
	body   string
	typeF  byte
	linkTo string
}) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	defer func() { _ = f.Close() }()
	gz := gzip.NewWriter(f)
	defer func() { _ = gz.Close() }()
	tw := tar.NewWriter(gz)
	defer func() { _ = tw.Close() }()
	for _, e := range entries {
		hdr := &tar.Header{Name: e.name, Mode: 0o644, Typeflag: e.typeF, Size: int64(len(e.body))}
		if e.typeF == tar.TypeSymlink {
			hdr.Linkname = e.linkTo
		}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatalf("header %q: %v", e.name, err)
		}
		if e.typeF == tar.TypeReg {
			if _, err := tw.Write([]byte(e.body)); err != nil {
				t.Fatalf("write %q: %v", e.name, err)
			}
		}
	}
}

// TestUntarRejectsPathTraversalAdversarial verifies a crafted snapshot with a
// ".." entry cannot write outside the extraction directory.
func TestUntarRejectsPathTraversalAdversarial(t *testing.T) {
	dst := t.TempDir()
	escape := filepath.Join(t.TempDir(), "pwned.txt")
	src := filepath.Join(t.TempDir(), "evil.tar.gz")
	writeTarGz(t, src, []struct {
		name   string
		body   string
		typeF  byte
		linkTo string
	}{
		{"../../pwned.txt", "boom", tar.TypeReg, ""},
	})
	if err := untarGz(src, dst); err == nil {
		t.Fatalf("path traversal must be rejected")
	}
	if _, err := os.Stat(escape); !os.IsNotExist(err) {
		t.Fatalf("file escaped the extraction root: %v", err)
	}
}

// TestUntarRejectsAbsolutePathAdversarial verifies absolute entries are rejected.
func TestUntarRejectsAbsolutePathAdversarial(t *testing.T) {
	dst := t.TempDir()
	src := filepath.Join(t.TempDir(), "abs.tar.gz")
	writeTarGz(t, src, []struct {
		name   string
		body   string
		typeF  byte
		linkTo string
	}{
		{"/etc/evil", "boom", tar.TypeReg, ""},
	})
	if err := untarGz(src, dst); err == nil {
		t.Fatalf("absolute archive path must be rejected")
	}
}

// TestUntarSkipsSymlinks verifies symlink entries are never materialized.
func TestUntarSkipsSymlinks(t *testing.T) {
	dst := t.TempDir()
	src := filepath.Join(t.TempDir(), "link.tar.gz")
	writeTarGz(t, src, []struct {
		name   string
		body   string
		typeF  byte
		linkTo string
	}{
		{"real.txt", "payload", tar.TypeReg, ""},
		{"evil-link", "", tar.TypeSymlink, "real.txt"},
	})
	if err := untarGz(src, dst); err != nil {
		t.Fatalf("untar: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dst, "evil-link")); !os.IsNotExist(err) {
		t.Fatalf("symlink must not be materialized: %v", err)
	}
	if b, err := os.ReadFile(filepath.Join(dst, "real.txt")); err != nil || string(b) != "payload" {
		t.Fatalf("regular file damaged: %q err=%v", b, err)
	}
}

// TestUntarNestedSafeEntries verifies legitimate nested paths extract fine.
func TestUntarNestedSafeEntries(t *testing.T) {
	dst := t.TempDir()
	src := filepath.Join(t.TempDir(), "ok.tar.gz")
	writeTarGz(t, src, []struct {
		name   string
		body   string
		typeF  byte
		linkTo string
	}{
		{"a/b/c.txt", "deep", tar.TypeReg, ""},
	})
	if err := untarGz(src, dst); err != nil {
		t.Fatalf("untar: %v", err)
	}
	if b, err := os.ReadFile(filepath.Join(dst, "a", "b", "c.txt")); err != nil || string(b) != "deep" {
		t.Fatalf("nested file damaged: %q err=%v", b, err)
	}
}
