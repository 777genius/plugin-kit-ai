package process

import (
	"bytes"
	"context"
	"fmt"
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
	var stdout bytes.Buffer
	stderr := newBoundedDiagnosticBuffer(32 * 1024)
	c.Stdout = &stdout
	c.Stderr = stderr
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

type boundedDiagnosticBuffer struct {
	prefix []byte
	suffix []byte
	total  int
	limit  int
}

func newBoundedDiagnosticBuffer(limit int) *boundedDiagnosticBuffer {
	return &boundedDiagnosticBuffer{limit: limit}
}

func (buffer *boundedDiagnosticBuffer) Write(value []byte) (int, error) {
	written := len(value)
	buffer.total += written
	prefixLimit := buffer.limit / 2
	if remaining := prefixLimit - len(buffer.prefix); remaining > 0 {
		count := min(remaining, len(value))
		buffer.prefix = append(buffer.prefix, value[:count]...)
		value = value[count:]
	}
	suffixLimit := buffer.limit - prefixLimit
	if len(value) >= suffixLimit {
		buffer.suffix = append(buffer.suffix[:0], value[len(value)-suffixLimit:]...)
	} else if len(value) > 0 {
		buffer.suffix = append(buffer.suffix, value...)
		if overflow := len(buffer.suffix) - suffixLimit; overflow > 0 {
			copy(buffer.suffix, buffer.suffix[overflow:])
			buffer.suffix = buffer.suffix[:suffixLimit]
		}
	}
	return written, nil
}

func (buffer *boundedDiagnosticBuffer) Bytes() []byte {
	omitted := buffer.total - len(buffer.prefix) - len(buffer.suffix)
	if omitted <= 0 {
		return append(append([]byte(nil), buffer.prefix...), buffer.suffix...)
	}
	marker := []byte(fmt.Sprintf("\n... %d stderr bytes omitted ...\n", omitted))
	result := make([]byte, 0, len(buffer.prefix)+len(marker)+len(buffer.suffix))
	result = append(result, buffer.prefix...)
	result = append(result, marker...)
	result = append(result, buffer.suffix...)
	return result
}
