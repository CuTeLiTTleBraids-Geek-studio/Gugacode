package services

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// InstanceLock prevents multiple gugacode instances from running simultaneously
// and competing for the same settings.json (G-QUAL-05).
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

// Acquire tries to acquire the single-instance lock. Returns an error if
// another instance is already running.
func (l *InstanceLock) Acquire() error {
	l.mu.Lock()
	defer l.mu.Unlock()

	// Create the lock file with O_EXCL (fails if it already exists)
	f, err := os.OpenFile(l.lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		if os.IsExist(err) {
			return fmt.Errorf("another gugacode instance is already running (lock file: %s)", l.lockPath)
		}
		return fmt.Errorf("create lock file: %w", err)
	}
	// Write the current PID for debugging
	fmt.Fprintf(f, "%d\n", os.Getpid())
	l.file = f
	// Reset the released flag so a subsequent Release() actually closes
	// the new file handle. Without this, reacquiring after release would
	// leave the file open (Release would no-op on the stale flag).
	l.released = false
	return nil
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
