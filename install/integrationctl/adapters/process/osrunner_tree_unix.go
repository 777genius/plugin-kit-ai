//go:build aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package process

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
)

type commandTree struct {
	cmd *exec.Cmd
}

func newCommandTree(cmd *exec.Cmd) (*commandTree, error) {
	tree := &commandTree{cmd: cmd}
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = tree.terminate
	return tree, nil
}

func (*commandTree) attach(*exec.Cmd) error { return nil }

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

func (*commandTree) close() {}
