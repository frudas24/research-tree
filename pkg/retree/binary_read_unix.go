//go:build !windows

package retree

import "os"

// openBinaryRead opens a replaceable binary-store file for lock-free reading.
func openBinaryRead(path string) (*os.File, error) {
	return os.Open(path)
}

// isBinaryMarkerTransitionError reports platform-specific transient marker
// errors. Unix rename and unlink do not require special handling.
func isBinaryMarkerTransitionError(error) bool {
	return false
}
