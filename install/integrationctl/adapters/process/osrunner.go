package process

import (
	"bytes"
	"context"
	"os/exec"
	"time"

	"github.com/777genius/plugin-kit-ai/install/integrationctl/ports"
)

type OS struct{}

func (OS) Run(ctx context.Context, cmd ports.Command) (ports.CommandResult, error) {
	if len(cmd.Argv) == 0 {
		return ports.CommandResult{}, exec.ErrNotFound
	}
	c := exec.CommandContext(ctx, cmd.Argv[0], cmd.Argv[1:]...)
	c.Env = cmd.Env
	c.Dir = cmd.Dir
	c.WaitDelay = 100 * time.Millisecond
	tree, err := newCommandTree(c)
	if err != nil {
		return ports.CommandResult{}, err
	}
	defer tree.close()
	var stdout, stderr bytes.Buffer
	c.Stdout = &stdout
	c.Stderr = &stderr
	if err := c.Start(); err != nil {
		return ports.CommandResult{}, err
	}
	if err := tree.attach(c); err != nil {
		_ = tree.terminate()
		_ = c.Wait()
		return ports.CommandResult{}, err
	}
	err = c.Wait()
	if err == nil {
		return ports.CommandResult{ExitCode: 0, Stdout: stdout.Bytes()}, nil
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		return ports.CommandResult{}, ctxErr
	}
	if exitErr, ok := err.(*exec.ExitError); ok {
		return ports.CommandResult{
			ExitCode: exitErr.ExitCode(),
			Stdout:   stdout.Bytes(),
			Stderr:   stderr.Bytes(),
		}, nil
	}
	return ports.CommandResult{}, err
}
