//go:build !windows

package services

import "os"

func defaultShell() []string {
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "bash"
	}
	return []string{shell}
}
