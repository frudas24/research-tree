package retree

import (
	"io"
)

// readBinaryFile reads a replaceable binary-store file through the
// platform-specific sharing mode used by lock-free readers.
func readBinaryFile(path string) ([]byte, error) {
	f, err := openBinaryRead(path)
	if err != nil {
		return nil, err
	}
	b, readErr := io.ReadAll(f)
	closeErr := f.Close()
	if readErr != nil {
		return nil, readErr
	}
	if closeErr != nil {
		return nil, closeErr
	}
	return b, nil
}

// binaryFileSize returns the size of a replaceable binary-store file without
// holding a Windows handle that blocks writer replacement.
func binaryFileSize(path string) (int64, error) {
	f, err := openBinaryRead(path)
	if err != nil {
		return 0, err
	}
	info, statErr := f.Stat()
	closeErr := f.Close()
	if statErr != nil {
		return 0, statErr
	}
	if closeErr != nil {
		return 0, closeErr
	}
	return info.Size(), nil
}
