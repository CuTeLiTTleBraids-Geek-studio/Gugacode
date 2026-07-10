//go:build windows

package services

import (
	"io"
	"syscall"

	"github.com/UserExistsError/conpty"
)

type windowsPty struct {
	cpty *conpty.ConPty
}

func (w *windowsPty) Read(p []byte) (int, error)  { return w.cpty.Read(p) }
func (w *windowsPty) Write(p []byte) (int, error) { return w.cpty.Write(p) }
func (w *windowsPty) Resize(cols, rows int) error {
	if w.cpty == nil {
		return nil
	}
	return w.cpty.Resize(cols, rows)
}
func (w *windowsPty) Close() error {
	if w.cpty != nil {
		return w.cpty.Close()
	}
	return nil
}

func startPty(shell []string, workingDir string) (io.ReadWriteCloser, error) {
	// M-3/HIGH-02: use syscall.EscapeArg to properly quote each argument.
	// The previous strings.Join(shell, " ") did not escape spaces or special
	// characters, allowing argument injection when a path contained spaces.
	commandLine := buildWindowsCommandLine(shell)
	opts := []conpty.ConPtyOption{}
	if workingDir != "" {
		opts = append(opts, conpty.ConPtyWorkDir(workingDir))
	}
	cpty, err := conpty.Start(commandLine, opts...)
	if err != nil {
		return nil, err
	}
	return &windowsPty{cpty: cpty}, nil
}

// buildWindowsCommandLine builds a properly escaped Windows command-line
// string from a shell argv slice using syscall.EscapeArg. Extracted from
// startPty for testability (HIGH-02): ConPTY tests are skipped in CI
// (skipIfNoConsole), so this function lets us verify the escaping logic
// directly without a real console.
func buildWindowsCommandLine(shell []string) string {
	var commandLine string
	for i, arg := range shell {
		if i > 0 {
			commandLine += " "
		}
		commandLine += syscall.EscapeArg(arg)
	}
	return commandLine
}
