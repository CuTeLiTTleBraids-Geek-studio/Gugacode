//go:build !windows

package services

import (
	"io"
	"os"
	"os/exec"

	"github.com/creack/pty"
)

type unixPty struct {
	ptmx *os.File
	cmd  *exec.Cmd
}

func (u *unixPty) Read(p []byte) (int, error)  { return u.ptmx.Read(p) }
func (u *unixPty) Write(p []byte) (int, error) { return u.ptmx.Write(p) }
func (u *unixPty) Resize(cols, rows int) error {
	return pty.Setsize(u.ptmx, &pty.Winsize{
		Cols: uint16(cols),
		Rows: uint16(rows),
	})
}
func (u *unixPty) Close() error {
	if u.ptmx != nil {
		u.ptmx.Close()
	}
	if u.cmd != nil && u.cmd.Process != nil {
		u.cmd.Process.Kill()
		u.cmd.Wait()
	}
	return nil
}

func startPty(shell []string, workingDir string) (io.ReadWriteCloser, error) {
	cmd := exec.Command(shell[0], shell[1:]...)
	cmd.Dir = workingDir
	ptmx, err := pty.Start(cmd)
	if err != nil {
		return nil, err
	}
	return &unixPty{ptmx: ptmx, cmd: cmd}, nil
}
