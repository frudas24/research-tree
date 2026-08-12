package retree

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"
)

var processLocks sync.Map

const (
	lockRetryInterval = 100 * time.Millisecond
	lockTimeout       = 60 * time.Second
	lockStaleAfter    = 30 * time.Second
	lockRefreshEvery  = 5 * time.Second
)

type lockInfo struct {
	PID       int
	Host      string
	Timestamp time.Time
	Operation string
	Owner     string
	Token     string
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
	local := localProcessLock(s.rootPath)
	local.Lock()
	deadline := time.Now().Add(lockTimeout)
	for {
		fd, err := os.OpenFile(s.lockPath(), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
		if err == nil {
			token, terr := newLockToken()
			if terr != nil {
				_ = fd.Close()
				_ = os.Remove(s.lockPath())
				local.Unlock()
				return nil, terr
			}
			info := lockInfo{PID: os.Getpid(), Host: "local", Timestamp: time.Now().UTC(), Operation: operation, Owner: "local", Token: token}
			_, werr := fd.WriteString(formatLockInfo(info))
			cerr := fd.Close()
			if werr != nil {
				_ = os.Remove(s.lockPath())
				local.Unlock()
				return nil, werr
			}
			if cerr != nil {
				_ = os.Remove(s.lockPath())
				local.Unlock()
				return nil, cerr
			}
			stop := make(chan struct{})
			done := make(chan struct{})
			var once sync.Once
			go s.refreshLock(info, stop, done)
			return func() {
				once.Do(func() {
					close(stop)
					<-done
					_ = s.markLockReleasedIfOwned(info.Token)
					local.Unlock()
				})
			}, nil
		}
		if !os.IsExist(err) {
			local.Unlock()
			return nil, err
		}
		stale, serr := s.isLockStale()
		if serr == nil && stale {
			_ = os.Remove(s.lockPath())
			continue
		}
		if time.Now().After(deadline) {
			local.Unlock()
			return nil, fmt.Errorf("lock timeout for op=%s", operation)
		}
		time.Sleep(lockRetryInterval)
	}
}

// refreshLock keeps the lockfile timestamp fresh while the owner still holds it.
func (s *Store) refreshLock(info lockInfo, stop <-chan struct{}, done chan<- struct{}) {
	ticker := time.NewTicker(lockRefreshEvery)
	defer ticker.Stop()
	defer close(done)
	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			updated := info
			updated.Timestamp = time.Now().UTC()
			owned, err := s.writeLockIfOwned(updated)
			if err != nil {
				continue
			}
			if !owned {
				return
			}
		}
	}
}

// isLockStale reports whether the current lockfile has exceeded the stale threshold.
func (s *Store) isLockStale() (bool, error) {
	info, err := s.readLockInfo()
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	if info.Timestamp.IsZero() {
		st, err := os.Stat(s.lockPath())
		if err != nil {
			return false, err
		}
		return time.Since(st.ModTime()) > lockStaleAfter, nil
	}
	return time.Since(info.Timestamp) > lockStaleAfter, nil
}

// readLockInfo parses the current lockfile payload.
func (s *Store) readLockInfo() (lockInfo, error) {
	b, err := os.ReadFile(s.lockPath())
	if err != nil {
		return lockInfo{}, err
	}
	return parseLockInfo(string(b))
}

// writeLockIfOwned refreshes the lockfile only if the token still matches the current owner.
func (s *Store) writeLockIfOwned(info lockInfo) (bool, error) {
	current, err := s.readLockInfo()
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	if current.Token == "" || current.Token != info.Token {
		return false, nil
	}
	path := s.lockPath()
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(formatLockInfo(info)), 0o644); err != nil {
		return false, err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return false, err
	}
	return true, nil
}

// markLockReleasedIfOwned marks the lock stale immediately, but only if the
// token still belongs to this owner. The next waiter can reclaim it safely.
func (s *Store) markLockReleasedIfOwned(token string) error {
	current, err := s.readLockInfo()
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if current.Token == "" || current.Token != token {
		return nil
	}
	current.Timestamp = time.Unix(0, 0).UTC()
	return s.writeLockInfo(current)
}

// formatLockInfo serializes lock metadata to the on-disk lockfile format.
func formatLockInfo(info lockInfo) string {
	return fmt.Sprintf(
		"pid: %d\nhost: %q\ntimestamp: %q\noperation: %q\nowner: %q\ntoken: %q\n",
		info.PID,
		info.Host,
		info.Timestamp.Format(time.RFC3339),
		info.Operation,
		info.Owner,
		info.Token,
	)
}

// writeLockInfo atomically writes one lockfile payload.
func (s *Store) writeLockInfo(info lockInfo) error {
	path := s.lockPath()
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(formatLockInfo(info)), 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

// parseLockInfo decodes the textual lockfile payload.
func parseLockInfo(raw string) (lockInfo, error) {
	var info lockInfo
	var ts string
	for _, line := range strings.Split(raw, "\n") {
		switch {
		case strings.HasPrefix(line, "pid:"):
			_, _ = fmt.Sscanf(strings.TrimSpace(strings.TrimPrefix(line, "pid:")), "%d", &info.PID)
		case strings.HasPrefix(line, "host:"):
			info.Host = trimLockField(line, "host:")
		case strings.HasPrefix(line, "timestamp:"):
			ts = trimLockField(line, "timestamp:")
		case strings.HasPrefix(line, "operation:"):
			info.Operation = trimLockField(line, "operation:")
		case strings.HasPrefix(line, "owner:"):
			info.Owner = trimLockField(line, "owner:")
		case strings.HasPrefix(line, "token:"):
			info.Token = trimLockField(line, "token:")
		}
	}
	if ts != "" {
		parsed, err := time.Parse(time.RFC3339, ts)
		if err != nil {
			return lockInfo{}, err
		}
		info.Timestamp = parsed
	}
	return info, nil
}

// trimLockField extracts and unquotes a lockfile field value after its prefix.
func trimLockField(line, prefix string) string {
	value := strings.TrimSpace(strings.TrimPrefix(line, prefix))
	return strings.Trim(value, "\"")
}

// newLockToken returns a random owner token for one lock acquisition.
func newLockToken() (string, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw[:]), nil
}

// localProcessLock returns the process-local mutex for one research root path.
func localProcessLock(rootPath string) *sync.Mutex {
	if v, ok := processLocks.Load(rootPath); ok {
		return v.(*sync.Mutex)
	}
	mu := &sync.Mutex{}
	actual, _ := processLocks.LoadOrStore(rootPath, mu)
	return actual.(*sync.Mutex)
}
