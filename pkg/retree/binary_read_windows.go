//go:build windows

package retree

import (
	"errors"
	"os"
	"syscall"
)

const (
	windowsErrorAccessDenied     = syscall.Errno(5)
	windowsErrorSharingViolation = syscall.Errno(32)
	windowsErrorDeletePending    = syscall.Errno(303)
)

// openBinaryRead opens a binary-store file without preventing another process
// from atomically replacing it while this reader retains its handle.
func openBinaryRead(path string) (*os.File, error) {
	name, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return nil, &os.PathError{Op: "open", Path: path, Err: err}
	}
	handle, err := syscall.CreateFile(
		name,
		syscall.GENERIC_READ,
		syscall.FILE_SHARE_READ|syscall.FILE_SHARE_WRITE|syscall.FILE_SHARE_DELETE,
		nil,
		syscall.OPEN_EXISTING,
		syscall.FILE_ATTRIBUTE_NORMAL,
		0,
	)
	if err != nil {
		return nil, &os.PathError{Op: "open", Path: path, Err: err}
	}
	return os.NewFile(uintptr(handle), path), nil
}

// isBinaryMarkerTransitionError identifies Windows errors observed while the
// writer atomically replaces or removes the dirty marker.
func isBinaryMarkerTransitionError(err error) bool {
	return errors.Is(err, windowsErrorAccessDenied) ||
		errors.Is(err, windowsErrorSharingViolation) ||
		errors.Is(err, windowsErrorDeletePending)
}
