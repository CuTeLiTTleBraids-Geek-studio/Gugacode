//go:build !windows

package services

import (
	"os"
	"syscall"
)

// processAlive reports whether a process with the given PID is running.
func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	p, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	// Signal 0 checks existence without killing (POSIX).
	err = p.Signal(syscall.Signal(0))
	return err == nil
}
