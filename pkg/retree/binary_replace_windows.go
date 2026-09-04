//go:build windows

package retree

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"unsafe"
)

const (
	fileRenameInfoEx          = 22
	fileRenameReplaceIfExists = 0x00000001
	fileRenamePOSIXSemantics  = 0x00000002
	deleteAccess              = 0x00010000
	synchronizeAccess         = 0x00100000
)

var setFileInformationByHandleProc = syscall.NewLazyDLL("kernel32.dll").NewProc("SetFileInformationByHandle")

type windowsFileRenameInfoEx struct {
	Flags          uint32
	RootDirectory  syscall.Handle
	FileNameLength uint32
	FileName       [1]uint16
}

// replaceBinaryFile replaces a binary-store path even while lock-free readers
// retain delete-sharing handles to the previous generation.
func replaceBinaryFile(oldPath, newPath string) error {
	if err := replaceBinaryFilePOSIX(oldPath, newPath); err == nil {
		return nil
	} else if fallbackErr := os.Rename(oldPath, newPath); fallbackErr != nil {
		return fmt.Errorf("POSIX replace failed: %v; standard replace failed: %w", err, fallbackErr)
	}
	return nil
}

// replaceBinaryFilePOSIX uses Windows rename-by-handle semantics so replacing
// an open destination behaves like Unix unlink-and-rename.
func replaceBinaryFilePOSIX(oldPath, newPath string) error {
	oldName, err := syscall.UTF16PtrFromString(oldPath)
	if err != nil {
		return &os.PathError{Op: "rename", Path: oldPath, Err: err}
	}
	handle, err := syscall.CreateFile(
		oldName,
		deleteAccess|synchronizeAccess,
		syscall.FILE_SHARE_READ|syscall.FILE_SHARE_WRITE|syscall.FILE_SHARE_DELETE,
		nil,
		syscall.OPEN_EXISTING,
		syscall.FILE_ATTRIBUTE_NORMAL,
		0,
	)
	if err != nil {
		return &os.PathError{Op: "rename", Path: oldPath, Err: err}
	}
	defer func() { _ = syscall.CloseHandle(handle) }()

	absNewPath, err := filepath.Abs(newPath)
	if err != nil {
		return &os.PathError{Op: "rename", Path: newPath, Err: err}
	}
	newName, err := syscall.UTF16FromString(absNewPath)
	if err != nil {
		return &os.PathError{Op: "rename", Path: newPath, Err: err}
	}

	nameOffset := unsafe.Offsetof(windowsFileRenameInfoEx{}.FileName)
	buffer := make([]byte, nameOffset+uintptr(len(newName))*unsafe.Sizeof(newName[0]))
	info := (*windowsFileRenameInfoEx)(unsafe.Pointer(&buffer[0]))
	info.Flags = fileRenameReplaceIfExists | fileRenamePOSIXSemantics
	info.FileNameLength = uint32((len(newName) - 1) * 2)
	copy(unsafe.Slice(&info.FileName[0], len(newName)), newName)

	r1, _, callErr := setFileInformationByHandleProc.Call(
		uintptr(handle),
		uintptr(fileRenameInfoEx),
		uintptr(unsafe.Pointer(&buffer[0])),
		uintptr(len(buffer)),
	)
	if r1 == 0 {
		if callErr == syscall.Errno(0) {
			callErr = syscall.EINVAL
		}
		return &os.LinkError{Op: "rename", Old: oldPath, New: newPath, Err: callErr}
	}
	return nil
}
