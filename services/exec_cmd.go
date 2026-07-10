package services

import (
	"context"
	"os/exec"
)

// command is like exec.Command but hides the child console window on Windows
// so GUI launches do not flash black CMD windows for version probes, git, etc.
func command(name string, arg ...string) *exec.Cmd {
	cmd := exec.Command(name, arg...)
	hideConsoleWindow(cmd)
	return cmd
}

// commandContext is like exec.CommandContext with Windows console suppression.
func commandContext(ctx context.Context, name string, arg ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, name, arg...)
	hideConsoleWindow(cmd)
	return cmd
}
