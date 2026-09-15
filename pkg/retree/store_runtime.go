package retree

import (
	"fmt"
	"os"
	"path/filepath"
)

// Runtime sidecars are not research content. Keep the generation and recovery
// markers together: separating them could publish an inconsistent binary pair.
var runtimeSidecars = []string{".nodes.generation", ".nodes.dirty", ".derived.dirty"}

// runtimeDirectory reports which directory holds harness runtime state for root.
// It returns the .state subdirectory once the ready marker commits the
// conversion, and the legacy root otherwise.
func runtimeDirectory(root string) string {
	if _, err := os.Stat(filepath.Join(root, ".state", "ready")); err == nil {
		return filepath.Join(root, ".state")
	}
	return root
}

// SeparateRuntime opts a root into the harness-managed runtime layout. Legacy
// content is copied, never removed. Publishing ready is the commit point; an
// interrupted copy is repeated under the legacy writer lock on the next call.
// Older binaries must not write this root after conversion.
func SeparateRuntime(root string) error {
	if runtimeDirectory(root) != root {
		return nil
	}
	if err := os.MkdirAll(root, 0755); err != nil {
		return err
	}
	state := filepath.Join(root, ".state")
	migrate := func() error {
		if runtimeDirectory(root) != root {
			return nil
		}
		if err := os.MkdirAll(state, 0700); err != nil {
			return err
		}
		for _, name := range runtimeSidecars {
			data, err := os.ReadFile(filepath.Join(root, name))
			if os.IsNotExist(err) {
				if e := os.Remove(filepath.Join(state, name)); e != nil && !os.IsNotExist(e) {
					return e
				}
				continue
			}
			if err != nil {
				return err
			}
			if err = os.WriteFile(filepath.Join(state, name), data, 0600); err != nil {
				return err
			}
		}
		marker := filepath.Join(state, "ready")
		if err := os.WriteFile(marker+".tmp", []byte("runtime-layout-v1\n"), 0600); err != nil {
			return err
		}
		return os.Rename(marker+".tmp", marker)
	}
	if _, err := os.Stat(filepath.Join(root, "meta.json")); os.IsNotExist(err) {
		// Initialization is serialized by the harness's resource claim.
		return migrate()
	} else if err != nil {
		return err
	}
	s, err := readStoreMetadata(root)
	if err != nil {
		return err
	}
	return s.withLock("separate_runtime", migrate)
}

// runtimeFile resolves name against the store's runtime directory, falling back
// to the legacy root for stores opened before runtime separation.
func (s *Store) runtimeFile(name string) string {
	if s.runtimePath == "" {
		return filepath.Join(s.rootPath, name)
	}
	return filepath.Join(s.runtimePath, name)
}

// validateRuntimeLayout rejects a handle whose runtime directory no longer
// matches its store root, so callers reopen instead of writing to a stale
// location.
func (s *Store) validateRuntimeLayout() error {
	path := s.runtimePath
	if path == "" {
		path = s.rootPath
	}
	if runtimeDirectory(s.rootPath) != path {
		return fmt.Errorf("%w: runtime layout changed; reopen the store", ErrStaleStore)
	}
	return nil
}
