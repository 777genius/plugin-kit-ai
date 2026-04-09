package process

import (
	"context"
	"os/exec"

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
	stdout, err := c.Output()
	if err == nil {
		return ports.CommandResult{ExitCode: 0, Stdout: stdout}, nil
	}
	if exitErr, ok := err.(*exec.ExitError); ok {
		return ports.CommandResult{
			ExitCode: exitErr.ExitCode(),
			Stdout:   stdout,
			Stderr:   exitErr.Stderr,
		}, nil
	}
	return ports.CommandResult{}, err
}
