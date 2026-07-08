//go:build !windows

package services

// defaultShellForExec returns the shell binary and args used to execute a
// one-shot command line. On unix-like systems we use `sh -c` so shell
// features (pipes, redirects, env vars) are available to agent commands.
// We deliberately use `sh` (not the user's $SHELL) for predictability.
func defaultShellForExec() []string {
	return []string{"sh", "-c"}
}
