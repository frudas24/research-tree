package retree

import (
	"fmt"
	"os"
	"strings"
	"time"
)

const (
	lockRetryInterval = 100 * time.Millisecond
	lockTimeout       = 10 * time.Second
	lockStaleAfter    = 30 * time.Second
)

type lockInfo struct {
	PID       int
	Host      string
	Timestamp time.Time
	Operation string
	Owner     string
}

// withLock acquires the lockfile, runs fn, and releases the lock.
func (s *Store) withLock(operation string, fn func() error) error {
	release, err := s.acquireLock(operation)
	if err != nil {
		return err
	}
	defer release()
	return fn()
}

// acquireLock acquires the lockfile with retry and stale takeover. Returns a release function.
func (s *Store) acquireLock(operation string) (func(), error) {
	deadline := time.Now().Add(lockTimeout)
	for {
		fd, err := os.OpenFile(s.lockPath(), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
		if err == nil {
			info := lockInfo{PID: os.Getpid(), Host: "local", Timestamp: time.Now().UTC(), Operation: operation, Owner: "local"}
			_, werr := fd.WriteString(fmt.Sprintf("pid: %d\nhost: %q\ntimestamp: %q\noperation: %q\nowner: %q\n", info.PID, info.Host, info.Timestamp.Format(time.RFC3339), info.Operation, info.Owner))
			cerr := fd.Close()
			if werr != nil {
				_ = os.Remove(s.lockPath())
				return nil, werr
			}
			if cerr != nil {
				_ = os.Remove(s.lockPath())
				return nil, cerr
			}
			return func() { _ = os.Remove(s.lockPath()) }, nil
		}
		if !os.IsExist(err) {
			return nil, err
		}
		stale, serr := s.isLockStale()
		if serr == nil && stale {
			_ = os.Remove(s.lockPath())
			continue
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("lock timeout for op=%s", operation)
		}
		time.Sleep(lockRetryInterval)
	}
}

// isLockStale reports whether the current lockfile has exceeded the stale threshold.
func (s *Store) isLockStale() (bool, error) {
	b, err := os.ReadFile(s.lockPath())
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	var ts string
	for _, line := range strings.Split(string(b), "\n") {
		if !strings.HasPrefix(line, "timestamp:") {
			continue
		}
		ts = strings.TrimSpace(strings.TrimPrefix(line, "timestamp:"))
		ts = strings.Trim(ts, "\"")
		break
	}
	if ts == "" {
		st, err := os.Stat(s.lockPath())
		if err != nil {
			return false, err
		}
		return time.Since(st.ModTime()) > lockStaleAfter, nil
	}
	parsed, err := time.Parse(time.RFC3339, ts)
	if err != nil {
		return false, err
	}
	return time.Since(parsed) > lockStaleAfter, nil
}
