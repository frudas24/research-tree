//go:build !windows

package retree

import "os"

// replaceBinaryFile atomically replaces one binary-store file on Unix.
func replaceBinaryFile(oldPath, newPath string) error {
	return os.Rename(oldPath, newPath)
}
