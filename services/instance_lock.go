package services

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
)

// InstanceLock prevents multiple gugacode instances from running simultaneously
// and competing for the same settings.json (G-QUAL-05).
//
// Stale locks (crash / Task Manager kill / GUI exit without Release) are
// detected by checking whether the PID written in the lock file is still alive.
type InstanceLock struct {
	mu       sync.Mutex
	released bool
	lockPath string
	file     *os.File
}

// NewInstanceLock creates a lock at the given path (typically in the user config dir).
func NewInstanceLock(configDir string) *InstanceLock {
	return &InstanceLock{
		lockPath: filepath.Join(configDir, "gugacode.lock"),
	}
}

// LockPath returns the absolute lock file path (for error messages / UI).
func (l *InstanceLock) LockPath() string {
	return l.lockPath
}

// Acquire tries to acquire the single-instance lock. Returns an error if
// another *live* instance is already running. Removes stale lock files when
// the recorded PID is not running (common after GUI crash / Force-quit).
func (l *InstanceLock) Acquire() error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if err := l.tryCreateExclusive(); err == nil {
		return nil
	} else if !os.IsExist(err) {
		return fmt.Errorf("create lock file: %w", err)
	}

	// Lock exists — check for stale owner.
	pid, readErr := readLockPID(l.lockPath)
	if readErr == nil && pid > 0 && !processAlive(pid) {
		// Stale: previous process died without Release().
		_ = os.Remove(l.lockPath)
		if err := l.tryCreateExclusive(); err != nil {
			if os.IsExist(err) {
				return fmt.Errorf("another gugacode instance is already running (lock file: %s)", l.lockPath)
			}
			return fmt.Errorf("create lock file after stale cleanup: %w", err)
		}
		return nil
	}

	if pid > 0 && processAlive(pid) {
		return fmt.Errorf("another gugacode instance is already running (pid %d, lock: %s)", pid, l.lockPath)
	}
	// Unreadable / empty lock — still treat as conflict unless we can clear it.
	// Last resort: remove empty/corrupt lock and retry once.
	data, _ := os.ReadFile(l.lockPath)
	if len(strings.TrimSpace(string(data))) == 0 || readErr != nil {
		_ = os.Remove(l.lockPath)
		if err := l.tryCreateExclusive(); err == nil {
			return nil
		}
	}
	return fmt.Errorf("another gugacode instance is already running (lock file: %s)", l.lockPath)
}

func (l *InstanceLock) tryCreateExclusive() error {
	f, err := os.OpenFile(l.lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	fmt.Fprintf(f, "%d\n", os.Getpid())
	l.file = f
	l.released = false
	return nil
}

func readLockPID(path string) (int, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	line := strings.TrimSpace(string(b))
	if line == "" {
		return 0, fmt.Errorf("empty lock")
	}
	// first token only
	fields := strings.Fields(line)
	return strconv.Atoi(fields[0])
}

// Release releases the single-instance lock.
func (l *InstanceLock) Release() error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.released {
		return nil
	}
	l.released = true

	var firstErr error
	if l.file != nil {
		if err := l.file.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
		l.file = nil
	}
	if err := os.Remove(l.lockPath); err != nil && !os.IsNotExist(err) && firstErr == nil {
		firstErr = err
	}
	return firstErr
}
