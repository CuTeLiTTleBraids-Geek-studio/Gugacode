//go:build windows

package services

import (
	"io"
	"strings"

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
	commandLine := strings.Join(shell, " ")
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
