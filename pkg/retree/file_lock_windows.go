//go:build windows

package retree

import (
	"os"
	"syscall"
	"unsafe"
)

var (
	kernel32DLL      = syscall.NewLazyDLL("kernel32.dll")
	lockFileExProc   = kernel32DLL.NewProc("LockFileEx")
	unlockFileExProc = kernel32DLL.NewProc("UnlockFileEx")
)

const lockFileExclusiveMode = 0x00000002

// lockFileExclusive takes an exclusive OS-level lock on the guard file.
func lockFileExclusive(file *os.File) error {
	var overlapped syscall.Overlapped
	r1, _, err := lockFileExProc.Call(
		file.Fd(),
		uintptr(lockFileExclusiveMode),
		0,
		1,
		0,
		uintptr(unsafe.Pointer(&overlapped)),
	)
	if r1 == 0 {
		if err != syscall.Errno(0) {
			return err
		}
		return syscall.EINVAL
	}
	return nil
}

// unlockFile releases the OS-level lock previously taken on the guard file.
func unlockFile(file *os.File) error {
	var overlapped syscall.Overlapped
	r1, _, err := unlockFileExProc.Call(
		file.Fd(),
		0,
		1,
		0,
		uintptr(unsafe.Pointer(&overlapped)),
	)
	if r1 == 0 {
		if err != syscall.Errno(0) {
			return err
		}
		return syscall.EINVAL
	}
	return nil
}
