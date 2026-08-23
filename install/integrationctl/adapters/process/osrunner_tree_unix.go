//go:build aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package process

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
	"time"
)

type commandTree struct {
	cmd        *exec.Cmd
	markerRead *os.File
	markerSend *os.File
}

func newCommandTree(cmd *exec.Cmd) (*commandTree, error) {
	markerRead, markerSend, err := os.Pipe()
	if err != nil {
		return nil, err
	}
	tree := &commandTree{cmd: cmd, markerRead: markerRead, markerSend: markerSend}
	cmd.ExtraFiles = append(cmd.ExtraFiles, markerSend)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = tree.terminate
	return tree, nil
}

func (tree *commandTree) attach(*exec.Cmd) error {
	return tree.markerSend.Close()
}

func (tree *commandTree) terminate() error {
	if tree.cmd.Process == nil {
		return os.ErrProcessDone
	}
	if err := syscall.Kill(-tree.cmd.Process.Pid, syscall.SIGKILL); err != nil {
		if errors.Is(err, syscall.ESRCH) {
			return os.ErrProcessDone
		}
		return err
	}
	return nil
}

func (tree *commandTree) close() {
	_ = tree.markerSend.Close()
	_ = tree.markerRead.SetReadDeadline(time.Now().Add(25 * time.Millisecond))
	var marker [1]byte
	_, err := tree.markerRead.Read(marker[:])
	if timeout, ok := err.(interface{ Timeout() bool }); ok && timeout.Timeout() {
		// A command-owned descendant still holds the inherited marker, so the
		// original process group cannot have been released or reused.
		_ = tree.terminate()
	}
	_ = tree.markerRead.Close()
}
