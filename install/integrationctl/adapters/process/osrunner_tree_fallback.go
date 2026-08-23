//go:build !aix && !darwin && !dragonfly && !freebsd && !linux && !netbsd && !openbsd && !solaris && !windows

package process

import (
	"os"
	"os/exec"
)

type commandTree struct {
	cmd *exec.Cmd
}

func newCommandTree(cmd *exec.Cmd) (*commandTree, error) {
	tree := &commandTree{cmd: cmd}
	cmd.Cancel = tree.terminate
	return tree, nil
}

func (*commandTree) attach(*exec.Cmd) error { return nil }

func (tree *commandTree) terminate() error {
	if tree.cmd.Process == nil {
		return os.ErrProcessDone
	}
	return tree.cmd.Process.Kill()
}

func (*commandTree) close() {}
