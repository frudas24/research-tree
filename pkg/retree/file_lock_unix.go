//go:build unix

package retree

import (
	"os"
	"syscall"
)

// lockFileExclusive takes an exclusive OS-level lock on the guard file.
func lockFileExclusive(file *os.File) error {
	return syscall.Flock(int(file.Fd()), syscall.LOCK_EX)
}

// unlockFile releases the OS-level lock previously taken on the guard file.
func unlockFile(file *os.File) error {
	return syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
}
