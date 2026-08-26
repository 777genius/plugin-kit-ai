package process

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/777genius/plugin-kit-ai/install/integrationctl/ports"
)

type OS struct{}

type terminationResult struct {
	leaderStopped bool
	forcedMembers bool
	err           error
}

type onceWriteCloser struct {
	io.WriteCloser
	once sync.Once
	err  error
}

func (writer *onceWriteCloser) Close() error {
	writer.once.Do(func() { writer.err = writer.WriteCloser.Close() })
	return writer.err
}

// Race-instrumented Go clients can take slightly over a second to complete
// runtime shutdown after stdin EOF; retain a bounded but realistic exit grace.
const duplexExitGrace = 2 * time.Second
const processReapTimeout = time.Second

type duplexPipeFactory struct {
	output      func() (*os.File, *os.File, error)
	input       func(*exec.Cmd) (io.WriteCloser, error)
	containment func(*exec.Cmd) (duplexCommandContainment, error)
}

type duplexCommandContainment interface {
	attach(*exec.Cmd) error
	terminate() terminationResult
	exited() (bool, error)
	close() error
}

// latchedExitObservation turns an edge-triggered process-exit notification
// into persistent state. Platform observers may be polled by supervision,
// settlement, and termination in succession; only the first poll must consume
// the underlying kernel event.
type latchedExitObservation struct {
	mu       sync.Mutex
	observed bool
}

func (observation *latchedExitObservation) latch() {
	observation.mu.Lock()
	observation.observed = true
	observation.mu.Unlock()
}

func (observation *latchedExitObservation) observe(poll func() (bool, error)) (bool, error) {
	observation.mu.Lock()
	defer observation.mu.Unlock()
	if observation.observed {
		return true, nil
	}
	exited, err := poll()
	if exited {
		observation.observed = true
	}
	return exited, err
}

var defaultDuplexPipeFactory = duplexPipeFactory{
	output: os.Pipe,
	input:  func(cmd *exec.Cmd) (io.WriteCloser, error) { return cmd.StdinPipe() },
	containment: func(cmd *exec.Cmd) (duplexCommandContainment, error) {
		return newDuplexCommandContainment(cmd)
	},
}

// RunDuplex runs one bounded-lifetime interactive exchange over a child's
// standard input and output. A successful exchange closes stdin and gives the
// child a bounded opportunity to report its own exit status. Platform cleanup
// is explicit and is always performed before the single Wait reaps the leader.
// It supervises the trusted, pinned client and its observable descendants; it
// is not an arbitrary hostile-process sandbox.
func (OS) RunDuplex(ctx context.Context, command ports.Command, exchange func(io.Writer, io.Reader) error) error {
	if err := duplexContainmentPreflight(); err != nil {
		return err
	}
	return runDuplex(ctx, command, exchange, defaultDuplexPipeFactory)
}

// RunDuplexWithPlannedShutdown runs an exchange whose nil return explicitly
// requests termination of a deliberately long-lived process tree. It is for
// protocols such as Kiro ACP where completing the verified exchange, rather
// than natural process exit, is the success boundary. Requested termination is
// successful only when containment proves the leader and every descendant are
// gone before the leader's sole reap.
func (OS) RunDuplexWithPlannedShutdown(ctx context.Context, command ports.Command, exchange func(io.Writer, io.Reader) error) error {
	if err := duplexContainmentPreflight(); err != nil {
		return err
	}
	return runDuplexMode(ctx, command, exchange, defaultDuplexPipeFactory, true)
}

// DuplexCapability proves that both duplex lifecycle modes can place a child
// and every process it creates inside a kernel-enforced containment boundary.
// Callers use this before materializing package state or invoking a native
// client mutation.
func (OS) DuplexCapability() error { return duplexContainmentPreflight() }

func runDuplex(ctx context.Context, command ports.Command, exchange func(io.Writer, io.Reader) error, pipes duplexPipeFactory) error {
	return runDuplexMode(ctx, command, exchange, pipes, false)
}

func runDuplexMode(ctx context.Context, command ports.Command, exchange func(io.Writer, io.Reader) error, pipes duplexPipeFactory, plannedShutdownEnabled bool) (resultErr error) {
	if len(command.Argv) == 0 {
		return exec.ErrNotFound
	}
	if exchange == nil {
		return fmt.Errorf("duplex process exchange is required")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	cmd := exec.Command(command.Argv[0], command.Argv[1:]...)
	cmd.Env = command.Env
	cmd.Dir = command.Dir
	// Allocate the independently-owned output pipe first. If it fails, no
	// command-owned stdin pipe exists to leak before Start can arrange cleanup.
	stdout, childStdout, err := pipes.output()
	if err != nil {
		if stdout != nil {
			_ = stdout.Close()
		}
		if childStdout != nil {
			_ = childStdout.Close()
		}
		return err
	}
	containmentFactory := pipes.containment
	if containmentFactory == nil {
		containmentFactory = func(cmd *exec.Cmd) (duplexCommandContainment, error) {
			return newCommandContainment(cmd)
		}
	}
	containment, err := containmentFactory(cmd)
	if err != nil {
		_ = stdout.Close()
		_ = childStdout.Close()
		return err
	}
	// StdinPipe retains its child-side descriptor inside exec.Cmd until Start
	// transfers ownership. Establish containment first so a pre-Start
	// containment failure cannot strand that descriptor.
	stdin, err := pipes.input(cmd)
	if err != nil {
		if stdin != nil {
			_ = stdin.Close()
		}
		_ = stdout.Close()
		_ = childStdout.Close()
		return errors.Join(err, containment.close())
	}
	stdin = &onceWriteCloser{WriteCloser: stdin}
	containmentClosed := false
	var containmentCloseMu sync.Mutex
	closeContainment := func() error {
		containmentCloseMu.Lock()
		defer containmentCloseMu.Unlock()
		if containmentClosed {
			return nil
		}
		err := containment.close()
		containmentClosed = err == nil
		return err
	}
	defer func() { resultErr = errors.Join(resultErr, closeContainment()) }()
	defer stdout.Close()
	cmd.Stdout = childStdout
	stderr := newBoundedDiagnosticBuffer(32 * 1024)
	cmd.Stderr = stderr
	cmd.WaitDelay = 100 * time.Millisecond
	if err := cmd.Start(); err != nil {
		_ = stdin.Close()
		_ = childStdout.Close()
		return err
	}
	_ = childStdout.Close()
	if err := containment.attach(cmd); err != nil {
		_ = stdin.Close()
		return cleanupAttachFailure(err, containment.terminate, cmd.Process.Kill, containment.exited, closeContainment, cmd.Wait)
	}
	exchanged := make(chan error, 1)
	go func() { exchanged <- exchange(stdin, stdout) }()

	poll := time.NewTicker(5 * time.Millisecond)
	defer poll.Stop()
	exited := false
	exchangeDone := false
	var exchangeErr error
	var supervisionErr error
	for !exited && !exchangeDone && supervisionErr == nil {
		select {
		case exchangeErr = <-exchanged:
			exchangeDone = true
		case <-poll.C:
			exited, err = containment.exited()
			if err != nil {
				supervisionErr = fmt.Errorf("observe duplex process exit: %w", err)
			}
		case <-ctx.Done():
			supervisionErr = ctx.Err()
		}
	}
	// Cancellation and exchange completion can become ready in the same
	// scheduling turn. Preserve the context cause even when the buffered
	// exchange result wins the select.
	if ctxErr := ctx.Err(); ctxErr != nil {
		supervisionErr = errors.Join(supervisionErr, ctxErr)
	}
	if exchangeDone && supervisionErr == nil && !exited {
		// Close the observation gap between the exchange result winning the
		// select and a planned teardown request. A long-lived protocol process
		// that is already waitable did not reach the planned-shutdown boundary.
		exited, err = containment.exited()
		if err != nil {
			supervisionErr = fmt.Errorf("observe duplex process before planned shutdown: %w", err)
		}
	}

	earlyExit := exited && (!exchangeDone || plannedShutdownEnabled)
	plannedShutdown := plannedShutdownEnabled && exchangeDone && exchangeErr == nil && supervisionErr == nil && !exited
	_ = stdin.Close()
	forced := false
	if plannedShutdown {
		// The successful exchange explicitly established the protocol's success
		// boundary. Terminate now, while the unreaped leader still anchors a
		// stable process-tree identity; do not wait for a natural exit that the
		// protocol intentionally never performs.
	} else if !exited && exchangeDone && exchangeErr == nil {
		grace := time.NewTimer(duplexExitGrace)
		defer grace.Stop()
		exited, exchangeErr = awaitDuplexExit(ctx, poll.C, grace.C, containment.exited)
		forced = !exited || exchangeErr != nil
	} else if !exited {
		forced = true
	}
	termination := terminationResult{leaderStopped: exited}
	if plannedShutdown || forced || (exited && naturalExitNeedsContainmentCleanup()) {
		// The leader is still an unreaped, stable identity here, including after
		// exited reported a waitable zombie. Thus group/job cleanup cannot target
		// a recycled numeric process identity. Natural exit cleanup is part of
		// the bounded-process contract and retains the leader's exact status.
		termination = containment.terminate()
		if errors.Is(termination.err, os.ErrProcessDone) {
			// ESRCH/ErrProcessDone means the pre-reap group/job identity no longer has
			// any signalable members, so cleanup is complete rather than uncertain.
			termination.err = nil
		}
		if !termination.leaderStopped && !exited {
			termination = stopLeaderAfterContainmentFailure(termination, cmd.Process.Kill, containment.exited)
		}
	}
	_ = stdout.Close()
	// Every successfully started leader has exactly one owned Wait. Even when
	// termination could not prove shutdown, the buffered sole-reaper goroutine
	// retains ownership until a later exit while caller cleanup stays bounded.
	waitErr := closeContainmentAndWait(closeContainment, cmd.Wait, processReapTimeout)
	if !exchangeDone {
		observedExchangeErr := <-exchanged
		exchangeDone = true
		exchangeErr = observedExchangeErr
	}
	exchangeErr = errors.Join(exchangeErr, supervisionErr)
	// Cancellation can arrive while containment closes or the sole Wait reaps.
	// Recheck after cleanup before accepting protocol success.
	exchangeErr = errors.Join(exchangeErr, ctx.Err())
	if !exited && !termination.leaderStopped && exchangeErr == nil {
		// This can only happen when a platform cannot observe exit without reaping.
		forced = true
	}
	if plannedShutdown {
		if exchangeErr != nil {
			return withDuplexDiagnostic(errors.Join(exchangeErr, termination.err, waitErr), stderr.Bytes())
		}
		if !termination.leaderStopped {
			return withDuplexDiagnostic(errors.Join(fmt.Errorf("planned duplex process-tree shutdown did not prove the leader stopped"), termination.err, waitErr), stderr.Bytes())
		}
		if termination.err != nil {
			return withDuplexDiagnostic(fmt.Errorf("planned duplex process-tree shutdown failed: %w", errors.Join(termination.err, waitErr)), stderr.Bytes())
		}
		if waitErr == nil {
			return nil
		}
		if _, expectedTerminationStatus := waitErr.(*exec.ExitError); expectedTerminationStatus {
			// The runner caused this status only after the exchange explicitly
			// requested planned shutdown and containment proved the tree empty.
			return nil
		}
		return withDuplexDiagnostic(fmt.Errorf("reap planned duplex process-tree shutdown: %w", waitErr), stderr.Bytes())
	}
	if forced {
		forcedErr := fmt.Errorf("duplex process required forced supervised cleanup after its exit grace")
		return withDuplexDiagnostic(errors.Join(exchangeErr, forcedErr, termination.err, waitErr), stderr.Bytes())
	}
	if termination.forcedMembers && !plannedShutdown {
		return withDuplexDiagnostic(errors.Join(exchangeErr, fmt.Errorf("duplex process left live descendants that required forced cleanup"), termination.err, waitErr), stderr.Bytes())
	}
	if earlyExit {
		earlyMessage := "duplex process exited before the exchange completed"
		if plannedShutdownEnabled && exchangeDone {
			earlyMessage = "duplex process exited before planned process-tree shutdown could begin"
		}
		earlyErr := errors.New(earlyMessage)
		return withDuplexDiagnostic(errors.Join(exchangeErr, earlyErr, termination.err, waitErr), stderr.Bytes())
	}
	if exchangeErr != nil {
		return withDuplexDiagnostic(errors.Join(exchangeErr, termination.err, waitErr), stderr.Bytes())
	}
	if termination.err != nil {
		return withDuplexDiagnostic(joinCleanupAndWaitError("terminate duplex supervised process", termination.err, waitErr), stderr.Bytes())
	}
	if waitErr != nil {
		return withDuplexDiagnostic(waitErr, stderr.Bytes())
	}
	return nil
}

func awaitDuplexExit(ctx context.Context, poll, grace <-chan time.Time, exited func() (bool, error)) (bool, error) {
	for {
		select {
		case <-poll:
			done, err := exited()
			if err != nil {
				return false, fmt.Errorf("observe duplex process exit: %w", err)
			}
			if !done {
				continue
			}
			// Cancellation can become observable while exit polling wins the
			// select. Recheck it before accepting a natural successful exit.
			return true, ctx.Err()
		case <-grace:
			return false, ctx.Err()
		case <-ctx.Done():
			return false, ctx.Err()
		}
	}
}

func cleanupAttachFailure(attachErr error, terminate func() terminationResult, killLeader func() error, exited func() (bool, error), closeContainment, wait func() error) error {
	return cleanupAttachFailureWithin(attachErr, terminate, killLeader, exited, closeContainment, wait, processReapTimeout)
}

func cleanupAttachFailureWithin(attachErr error, terminate func() terminationResult, killLeader func() error, exited func() (bool, error), closeContainment, wait func() error, timeout time.Duration) error {
	termination := terminate()
	if errors.Is(termination.err, os.ErrProcessDone) {
		termination.err = nil
	}
	if !termination.leaderStopped {
		termination = stopLeaderAfterContainmentFailure(termination, killLeader, exited)
	}
	return errors.Join(attachErr, termination.err, closeContainmentAndWait(closeContainment, wait, timeout))
}

func closeContainmentAndWait(closeContainment, wait func() error, timeout time.Duration) error {
	closeErr := closeContainment()
	waitErr := boundedProcessWait(func() error {
		waitErr := wait()
		// A close that could not finish while the process was alive (for
		// example, removal of a populated cgroup) remains owned by the sole
		// reaper. Retry after Wait so eventual exit also releases containment
		// resources even when the bounded caller has already returned.
		if closeErr != nil {
			return errors.Join(waitErr, closeContainment())
		}
		return waitErr
	}, timeout)
	if closeErr == nil {
		// Preserve *exec.ExitError so callers can retain natural exit status.
		return waitErr
	}
	return errors.Join(closeErr, waitErr)
}

func boundedProcessWait(wait func() error, timeout time.Duration) error {
	// All runner-owned containment handles and pipes are already closed. The
	// buffered result gives the sole reaper clear ownership until process exit,
	// including when shutdown could not be proved, while keeping the caller's
	// cleanup deadline real and preventing a future zombie.
	result := make(chan error, 1)
	go func() { result <- wait() }()
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case err := <-result:
		return err
	case <-timer.C:
		return fmt.Errorf("process wait/reap exceeded cleanup deadline of %s; sole reaper retains ownership until process exit", timeout)
	}
}

func stopLeaderAfterContainmentFailure(termination terminationResult, killLeader func() error, exited func() (bool, error)) terminationResult {
	if termination.leaderStopped {
		return termination
	}
	killErr := killLeader()
	if killErr != nil && !errors.Is(killErr, os.ErrProcessDone) {
		termination.err = errors.Join(termination.err, fmt.Errorf("kill process leader after supervised cleanup failure: %w", killErr))
		return termination
	}
	deadline := time.Now().Add(processReapTimeout)
	for {
		stopped, observeErr := exited()
		if stopped {
			termination.leaderStopped = true
			return termination
		}
		if observeErr != nil {
			termination.err = errors.Join(termination.err, fmt.Errorf("confirm process leader stopped after fallback kill: %w", observeErr))
			return termination
		}
		if time.Now().After(deadline) {
			termination.err = errors.Join(termination.err, fmt.Errorf("fallback leader kill was requested but shutdown was not observed before cleanup deadline"))
			return termination
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func withDuplexDiagnostic(err error, diagnostic []byte) error {
	if len(diagnostic) == 0 {
		return err
	}
	return fmt.Errorf("%w (client diagnostic: %s)", err, diagnostic)
}

func (OS) Run(ctx context.Context, cmd ports.Command) (ports.CommandResult, error) {
	return runCommand(ctx, cmd, time.Second, false)
}

// RunWithTreeExitGrace is used for trusted commands such as Git that may leave
// a protocol/helper process completing natural shutdown after the leader exits.
// The grace never converts forced cleanup into success.
func (OS) RunWithTreeExitGrace(ctx context.Context, cmd ports.Command, treeExitGrace time.Duration) (ports.CommandResult, error) {
	return runCommand(ctx, cmd, treeExitGrace, true)
}

func runCommand(ctx context.Context, cmd ports.Command, treeExitGrace time.Duration, processGroupOnly bool) (ports.CommandResult, error) {
	if len(cmd.Argv) == 0 {
		return ports.CommandResult{}, exec.ErrNotFound
	}
	if err := ctx.Err(); err != nil {
		return ports.CommandResult{}, err
	}
	// Every production command is explicitly supervised. Even commands that are
	// nominally diagnostic can load client configuration and start helpers, and
	// lifecycle commands can mutate or acquire state. On Linux the containment
	// constructor uses CLONE_INTO_CGROUP, so there is no post-Start sampling
	// window in which a descendant can escape.
	return runExplicit(ctx, cmd, treeExitGrace, processGroupOnly)
}

func runExplicit(ctx context.Context, command ports.Command, treeExitGrace time.Duration, processGroupOnly bool) (result ports.CommandResult, resultErr error) {
	c := exec.Command(command.Argv[0], command.Argv[1:]...)
	c.Env = command.Env
	c.Dir = command.Dir
	c.WaitDelay = 100 * time.Millisecond
	var stdout bytes.Buffer
	stderr := newBoundedDiagnosticBuffer(32 * 1024)
	c.Stdout = &stdout
	c.Stderr = stderr
	containment, err := newCommandContainment(c)
	if err != nil {
		return ports.CommandResult{}, err
	}
	if processGroupOnly {
		containment.limitToProcessGroup()
	}
	containmentClosed := false
	var containmentCloseMu sync.Mutex
	closeContainment := func() error {
		containmentCloseMu.Lock()
		defer containmentCloseMu.Unlock()
		if containmentClosed {
			return nil
		}
		err := containment.close()
		containmentClosed = err == nil
		return err
	}
	defer func() { resultErr = errors.Join(resultErr, closeContainment()) }()
	if err := c.Start(); err != nil {
		return ports.CommandResult{}, err
	}
	if err := containment.attach(c); err != nil {
		return ports.CommandResult{}, cleanupAttachFailure(err, containment.terminate, c.Process.Kill, containment.exited, closeContainment, c.Wait)
	}

	poll := time.NewTicker(5 * time.Millisecond)
	defer poll.Stop()
	supervisionErr := superviseExplicit(ctx, poll.C, containment.exited)

	termination := terminationResult{leaderStopped: supervisionErr == nil}
	if supervisionErr != nil || naturalExitNeedsContainmentCleanup() {
		if supervisionErr == nil {
			// A well-behaved command may have a just-exiting helper still visible
			// when the leader first becomes waitable. Give only observed helpers a
			// bounded natural shutdown grace; anything still live afterward is
			// forced and reported with explicit termination causality.
			_, settleErr := containment.settleNaturalExit(ctx, treeExitGrace)
			supervisionErr = errors.Join(settleErr, ctx.Err())
		}
		// Explicit cleanup happens while the leader is unreaped, so its numeric
		// process/group identity cannot have been recycled.
		termination = containment.terminate()
		if errors.Is(termination.err, os.ErrProcessDone) {
			termination.err = nil
		}
		if !termination.leaderStopped && supervisionErr != nil {
			termination = stopLeaderAfterContainmentFailure(termination, c.Process.Kill, containment.exited)
		}
	}
	// Wait may remain blocked if shutdown could not be proved. Close containment
	// first, then give the sole reaper durable ownership while bounding caller
	// return; no second Wait or Process.Release may abandon a future zombie.
	waitErr := closeContainmentAndWait(closeContainment, c.Wait, processReapTimeout)
	if supervisionErr != nil {
		return ports.CommandResult{}, withDuplexDiagnostic(errors.Join(supervisionErr, termination.err, waitErr), stderr.Bytes())
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		return ports.CommandResult{}, withDuplexDiagnostic(errors.Join(ctxErr, termination.err, waitErr), stderr.Bytes())
	}
	if termination.forcedMembers {
		return ports.CommandResult{}, withDuplexDiagnostic(errors.Join(fmt.Errorf("process left live descendants that required forced cleanup"), termination.err, waitErr), stderr.Bytes())
	}
	if termination.err != nil {
		return ports.CommandResult{}, withDuplexDiagnostic(joinCleanupAndWaitError("terminate supervised process", termination.err, waitErr), stderr.Bytes())
	}
	if waitErr == nil {
		return ports.CommandResult{ExitCode: 0, Stdout: stdout.Bytes()}, nil
	}
	if exitErr, ok := waitErr.(*exec.ExitError); ok {
		return ports.CommandResult{ExitCode: exitErr.ExitCode(), Stdout: stdout.Bytes(), Stderr: stderr.Bytes()}, nil
	}
	return ports.CommandResult{}, waitErr
}

func joinCleanupAndWaitError(operation string, cleanupErr, waitErr error) error {
	return fmt.Errorf("%s: %w", operation, errors.Join(cleanupErr, waitErr))
}

func superviseExplicit(ctx context.Context, poll <-chan time.Time, exited func() (bool, error)) error {
	var supervisionErr error
	for {
		select {
		case <-ctx.Done():
			supervisionErr = ctx.Err()
		case <-poll:
			done, observeErr := exited()
			if observeErr != nil {
				supervisionErr = fmt.Errorf("observe process exit: %w", observeErr)
			} else if !done {
				continue
			}
		}
		break
	}
	// The poll observation and cancellation can become ready in the same
	// scheduling turn. Re-check after observation so a natural exit cannot hide
	// cancellation and incorrectly retain a successful command result.
	return errors.Join(supervisionErr, ctx.Err())
}

type boundedDiagnosticBuffer struct {
	mu     sync.Mutex
	prefix []byte
	suffix []byte
	total  int
	limit  int
}

func newBoundedDiagnosticBuffer(limit int) *boundedDiagnosticBuffer {
	return &boundedDiagnosticBuffer{limit: limit}
}

func (buffer *boundedDiagnosticBuffer) Write(value []byte) (int, error) {
	buffer.mu.Lock()
	defer buffer.mu.Unlock()
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
	buffer.mu.Lock()
	defer buffer.mu.Unlock()
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
