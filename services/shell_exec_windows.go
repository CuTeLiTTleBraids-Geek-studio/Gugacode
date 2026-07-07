//go:build windows

package services

// defaultShellForExec returns the shell binary and args used to execute a
// one-shot command line. On Windows we use `cmd /c` so shell features
// (pipes, redirects, env vars) are available to agent commands.
func defaultShellForExec() []string {
	return []string{"cmd.exe", "/c"}
}
