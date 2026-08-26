package process

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/777genius/plugin-kit-ai/install/integrationctl/ports"
)

type noopDuplexCommandContainment struct{}

func (*noopDuplexCommandContainment) attach(*exec.Cmd) error { return nil }

func (*noopDuplexCommandContainment) terminate() terminationResult {
	return terminationResult{leaderStopped: true}
}

func (*noopDuplexCommandContainment) exited() (bool, error) { return false, nil }

func (*noopDuplexCommandContainment) close() error { return nil }

func successfulNoopDuplexContainment(*exec.Cmd) (duplexCommandContainment, error) {
	return &noopDuplexCommandContainment{}, nil
}

type naturalExitBoundaryContainment struct{}

func (*naturalExitBoundaryContainment) attach(*exec.Cmd) error { return nil }
func (*naturalExitBoundaryContainment) terminate() terminationResult {
	// Model containment finding an already-stopped leader and an empty tree.
	return terminationResult{leaderStopped: true}
}
func (*naturalExitBoundaryContainment) exited() (bool, error) { return false, nil }
func (*naturalExitBoundaryContainment) close() error          { return nil }

func TestOSRunnerPreservesCommandOutputAndExitCode(t *testing.T) {
	requireDuplexCapability(t)
	if os.Getenv("AGENTPLUGINS_PROCESS_OUTPUT_HELPER") == "1" {
		fmt.Fprint(os.Stdout, "expected stdout")
		fmt.Fprint(os.Stderr, "expected stderr")
		os.Exit(7)
	}
	environment := append([]string(nil), os.Environ()...)
	environment = append(environment, "AGENTPLUGINS_PROCESS_OUTPUT_HELPER=1")
	result, err := (OS{}).Run(context.Background(), ports.Command{
		Argv: []string{os.Args[0], "-test.run=TestOSRunnerPreservesCommandOutputAndExitCode"}, Env: environment,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.ExitCode != 7 || strings.TrimSpace(string(result.Stdout)) != "expected stdout" || strings.TrimSpace(string(result.Stderr)) != "expected stderr" {
		t.Fatalf("result = %+v", result)
	}
}

func TestOSRunnerCleanExitWithoutOtherGroupMembersRemainsSuccess(t *testing.T) {
	requireDuplexCapability(t)
	if os.Getenv("AGENTPLUGINS_PROCESS_CLEAN_EXIT_HELPER") == "1" {
		fmt.Fprint(os.Stdout, "clean")
		os.Exit(0)
	}
	environment := append(os.Environ(), "AGENTPLUGINS_PROCESS_CLEAN_EXIT_HELPER=1")
	result, err := (OS{}).Run(context.Background(), ports.Command{
		Argv: []string{os.Args[0], "-test.run=TestOSRunnerCleanExitWithoutOtherGroupMembersRemainsSuccess"}, Env: environment,
	})
	if err != nil || result.ExitCode != 0 || string(result.Stdout) != "clean" {
		t.Fatalf("result = %+v error = %v, want clean natural success", result, err)
	}
}

func TestEdgeTriggeredExitObservationRemainsLatchedThroughTermination(t *testing.T) {
	var observation latchedExitObservation
	polls := 0
	poll := func() (bool, error) {
		polls++
		return polls == 1, nil
	}
	for phase := 0; phase < 3; phase++ {
		exited, err := observation.observe(poll)
		if err != nil || !exited {
			t.Fatalf("exit observation phase %d = %v, %v; want persistent natural exit", phase, exited, err)
		}
	}
	// Darwin termination observes the leader again after process-group cleanup.
	// An EV_CLEAR NOTE_EXIT has already been consumed at this point, so this
	// final observation must use the latch rather than poll the kqueue again.
	exited, err := observation.observe(poll)
	if err != nil || !exited || polls != 1 {
		t.Fatalf("termination observation = %v, %v, polls %d; want one consumed event", exited, err, polls)
	}
}

func TestExitObservationDoesNotLatchFailedPoll(t *testing.T) {
	var observation latchedExitObservation
	want := errors.New("exit observation failed")
	polls := 0
	poll := func() (bool, error) {
		polls++
		if polls == 1 {
			return false, want
		}
		return true, nil
	}
	if exited, err := observation.observe(poll); exited || !errors.Is(err, want) {
		t.Fatalf("failed observation = %v, %v", exited, err)
	}
	if exited, err := observation.observe(poll); err != nil || !exited {
		t.Fatalf("successful observation = %v, %v", exited, err)
	}
	if exited, err := observation.observe(poll); err != nil || !exited || polls != 2 {
		t.Fatalf("latched observation = %v, %v, polls %d", exited, err, polls)
	}
}

func TestExplicitSupervisionPreservesCancellationAtNaturalExitBoundary(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	poll := make(chan time.Time, 1)
	poll <- time.Now()

	err := superviseExplicit(ctx, poll, func() (bool, error) {
		// Deterministically model the select race: the poll case was selected,
		// then cancellation became observable while the natural exit was checked.
		cancel()
		return true, nil
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("supervision error = %v, want context cancellation at natural-exit boundary", err)
	}
}

func TestDuplexGracePreservesCancellationAtNaturalExitBoundary(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	poll := make(chan time.Time, 1)
	grace := make(chan time.Time)
	poll <- time.Now()

	exited, err := awaitDuplexExit(ctx, poll, grace, func() (bool, error) {
		cancel()
		return true, nil
	})
	if !exited || !errors.Is(err, context.Canceled) {
		t.Fatalf("exited = %v error = %v, want observed exit plus context cancellation", exited, err)
	}
}

func TestAttachFailureCleanupWaitsWithoutRedundantLeaderKillAfterContainmentTermination(t *testing.T) {
	attachErr := errors.New("attach failed")
	containmentErr := errors.New("containment members incompletely terminated")
	waitErr := errors.New("wait failed")
	killCalls := 0
	waitCalls := 0
	err := cleanupAttachFailure(attachErr, func() terminationResult {
		return terminationResult{leaderStopped: true, err: containmentErr}
	}, func() error {
		killCalls++
		return errors.New("redundant leader kill failed")
	}, func() (bool, error) { return true, nil }, func() error { return nil }, func() error {
		waitCalls++
		return waitErr
	})
	if killCalls != 0 || waitCalls != 1 || !errors.Is(err, attachErr) || !errors.Is(err, containmentErr) || !errors.Is(err, waitErr) {
		t.Fatalf("kill calls = %d wait calls = %d error = %v, want no redundant kill and one synchronous wait", killCalls, waitCalls, err)
	}
}

func TestAttachFailureCleanupWaitsAfterLeaderFallbackSuccessAndPreservesContainmentError(t *testing.T) {
	attachErr := errors.New("attach failed")
	containmentErr := errors.New("containment cleanup failed")
	waitErr := errors.New("wait failed")
	waitCalls := 0
	err := cleanupAttachFailure(attachErr, func() terminationResult { return terminationResult{err: containmentErr} }, func() error { return os.ErrProcessDone }, func() (bool, error) { return true, nil }, func() error { return nil }, func() error {
		waitCalls++
		return waitErr
	})
	if waitCalls != 1 || !errors.Is(err, attachErr) || !errors.Is(err, containmentErr) || !errors.Is(err, waitErr) {
		t.Fatalf("wait calls = %d error = %v, want one synchronous wait with attach, containment, and wait errors", waitCalls, err)
	}
}

func TestAttachFailureCleanupOwnsSoleWaitWhenLeaderStopCannotBeProven(t *testing.T) {
	attachErr := errors.New("attach failed")
	containmentErr := errors.New("containment cleanup failed")
	killErr := errors.New("leader kill failed")
	waitCalls := 0
	waitRelease := make(chan struct{})
	waitStarted := make(chan struct{})
	ownedCleanupDone := make(chan struct{})
	closeCalls := 0
	closeErr := errors.New("containment close blocked by live process")
	defer func() {
		close(waitRelease)
		select {
		case <-ownedCleanupDone:
		case <-time.After(time.Second):
			t.Fatal("owned sole Wait did not finish containment cleanup after simulated process exit")
		}
	}()
	timeout := 10 * time.Millisecond
	started := time.Now()
	err := cleanupAttachFailureWithin(attachErr, func() terminationResult { return terminationResult{err: containmentErr} }, func() error { return killErr }, func() (bool, error) { return false, nil }, func() error {
		closeCalls++
		if closeCalls == 1 {
			return closeErr
		}
		close(ownedCleanupDone)
		return nil
	}, func() error {
		waitCalls++
		close(waitStarted)
		<-waitRelease
		return nil
	}, timeout)
	<-waitStarted
	if elapsed := time.Since(started); elapsed < timeout || elapsed > time.Second {
		t.Fatalf("failed termination cleanup duration = %s, want genuine %s bound", elapsed, timeout)
	}
	if waitCalls != 1 || closeCalls != 1 || !errors.Is(err, attachErr) || !errors.Is(err, containmentErr) || !errors.Is(err, killErr) || !errors.Is(err, closeErr) || !strings.Contains(err.Error(), "sole reaper retains ownership until process exit") {
		t.Fatalf("wait calls = %d error = %v, want bounded cleanup with one retained Wait and all failure causes", waitCalls, err)
	}
}

func TestBoundedProcessWaitReturnsAtTheRealDeadline(t *testing.T) {
	release := make(chan struct{})
	defer close(release)
	started := time.Now()
	err := boundedProcessWait(func() error {
		<-release
		return nil
	}, time.Millisecond)
	if err == nil || !strings.Contains(err.Error(), "exceeded cleanup deadline") {
		t.Fatalf("error = %v, want real bounded deadline", err)
	}
	if elapsed := time.Since(started); elapsed > 100*time.Millisecond {
		t.Fatalf("bounded wait returned too slowly after %s", elapsed)
	}
}

func TestCleanupFailurePreservesNaturalLeaderExitStatus(t *testing.T) {
	cleanupErr := errors.New("containment cleanup failed")
	waitErr := errors.New("leader exited with status 47")
	err := joinCleanupAndWaitError("terminate supervised process", cleanupErr, waitErr)
	if !errors.Is(err, cleanupErr) || !errors.Is(err, waitErr) {
		t.Fatalf("cleanup error = %v, want both cleanup and natural wait status", err)
	}
}

func TestContainmentCloseFailurePreservesExactExitStatusWithoutTrustingArbitraryJoin(t *testing.T) {
	cleanupErr := errors.New("containment cleanup failed")
	command := exec.Command("/bin/sh", "-c", "exit 47")
	runErr := command.Run()
	exitErr, ok := runErr.(*exec.ExitError)
	if !ok {
		t.Fatalf("helper error = %v, want *exec.ExitError", runErr)
	}
	err := closeContainmentAndWait(func() error { return cleanupErr }, func() error { return exitErr }, time.Second)

	gotExit, gotCleanup, ok := splitCommandWaitError(err)
	if !ok || gotExit != exitErr || !errors.Is(gotCleanup, cleanupErr) {
		t.Fatalf("split result = (%v, %v, %v), want exact exit status and cleanup error", gotExit, gotCleanup, ok)
	}
	if _, _, ok := splitCommandWaitError(errors.Join(cleanupErr, exitErr)); ok {
		t.Fatal("arbitrary joined ExitError was incorrectly trusted as a command wait result")
	}
	result, resultErr, ok := commandResultFromWait(err, []byte("command output"), []byte("command diagnostic"))
	if !ok || result.ExitCode != 47 || string(result.Stdout) != "command output" || string(result.Stderr) != "command diagnostic" {
		t.Fatalf("command result = (%+v, %v), want exact exit code/stdout/stderr", result, ok)
	}
	if !errors.Is(resultErr, cleanupErr) || !strings.Contains(resultErr.Error(), "command diagnostic") {
		t.Fatalf("command error = %v, want cleanup uncertainty and diagnostic", resultErr)
	}
}

func TestExplicitCleanupFailurePreservesNaturalNonzeroResultAndStderr(t *testing.T) {
	cleanupErr := errors.New("supervisor cleanup failed")
	command := exec.Command("/bin/sh", "-c", "exit 47")
	waitErr := command.Run()
	if _, ok := waitErr.(*exec.ExitError); !ok {
		t.Fatalf("helper error = %v, want *exec.ExitError", waitErr)
	}

	result, err := finishExplicitCommand(cleanupErr, waitErr, []byte("command output"), []byte("command diagnostic"))
	if result.ExitCode != 47 || string(result.Stdout) != "command output" || string(result.Stderr) != "command diagnostic" {
		t.Fatalf("command result = %+v, want exact natural exit code/stdout/stderr", result)
	}
	if !errors.Is(err, cleanupErr) || !strings.Contains(err.Error(), "command diagnostic") {
		t.Fatalf("command error = %v, want supervisor cleanup failure with stderr diagnostic", err)
	}
}

func TestAttachFailureClosesContainmentBeforeStartingWait(t *testing.T) {
	closed := false
	err := cleanupAttachFailure(errors.New("attach failed"), func() terminationResult {
		return terminationResult{leaderStopped: true}
	}, func() error {
		t.Fatal("leader kill must not be repeated")
		return nil
	}, func() (bool, error) { return true, nil }, func() error {
		closed = true
		return nil
	}, func() error {
		if !closed {
			t.Fatal("Wait started before containment closed")
		}
		return nil
	})
	if err == nil || !strings.Contains(err.Error(), "attach failed") {
		t.Fatalf("error = %v, want attach failure", err)
	}
}

func TestBoundedDiagnosticBufferSupportsConcurrentWaitTimeoutDiagnostics(t *testing.T) {
	buffer := newBoundedDiagnosticBuffer(1024)
	done := make(chan struct{})
	go func() {
		defer close(done)
		for range 1000 {
			_, _ = buffer.Write([]byte("diagnostic"))
		}
	}()
	for range 1000 {
		_ = buffer.Bytes()
	}
	<-done
}

func TestOSRunnerDuplexReportsForcedShutdownAfterSuccessfulExchange(t *testing.T) {
	requireDuplexCapability(t)
	if os.Getenv("AGENTPLUGINS_PROCESS_DUPLEX_HELPER") == "1" {
		reader := bufio.NewReader(os.Stdin)
		line, err := reader.ReadString('\n')
		if err != nil {
			os.Exit(8)
		}
		fmt.Fprint(os.Stdout, line)
		for {
			time.Sleep(time.Hour)
		}
	}
	environment := append([]string(nil), os.Environ()...)
	environment = append(environment, "AGENTPLUGINS_PROCESS_DUPLEX_HELPER=1")
	err := (OS{}).RunDuplex(context.Background(), ports.Command{
		Argv: []string{os.Args[0], "-test.run=TestOSRunnerDuplexReportsForcedShutdownAfterSuccessfulExchange"}, Env: environment,
	}, func(stdin io.Writer, stdout io.Reader) error {
		if _, err := io.WriteString(stdin, "request\n"); err != nil {
			return err
		}
		line, err := bufio.NewReader(stdout).ReadString('\n')
		if err != nil {
			return err
		}
		if line != "request\n" {
			return fmt.Errorf("duplex response = %q", line)
		}
		return nil
	})
	if err == nil || !strings.Contains(err.Error(), "required forced supervised cleanup") {
		t.Fatalf("forced long-lived shutdown error = %v, want explicit cleanup failure", err)
	}
}

func TestOSRunnerDuplexPlannedShutdownAcceptsOnlyRequestedLongLivedCleanup(t *testing.T) {
	requireDuplexCapability(t)
	if os.Getenv("AGENTPLUGINS_PROCESS_PLANNED_DUPLEX_HELPER") == "1" {
		line, err := bufio.NewReader(os.Stdin).ReadString('\n')
		if err != nil {
			os.Exit(8)
		}
		fmt.Fprint(os.Stdout, line)
		for {
			time.Sleep(time.Hour)
		}
	}
	environment := append(os.Environ(), "AGENTPLUGINS_PROCESS_PLANNED_DUPLEX_HELPER=1")
	err := (OS{}).RunDuplexWithPlannedShutdown(context.Background(), ports.Command{
		Argv: []string{os.Args[0], "-test.run=TestOSRunnerDuplexPlannedShutdownAcceptsOnlyRequestedLongLivedCleanup"}, Env: environment,
	}, func(stdin io.Writer, stdout io.Reader) error {
		if _, err := io.WriteString(stdin, "verified\n"); err != nil {
			return err
		}
		line, err := bufio.NewReader(stdout).ReadString('\n')
		if err != nil {
			return err
		}
		if line != "verified\n" {
			return fmt.Errorf("duplex response = %q", line)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("explicitly planned long-lived shutdown failed: %v", err)
	}
}

func TestOSRunnerDuplexPlannedShutdownRejectsExitBeforeTeardownRequest(t *testing.T) {
	requireDuplexCapability(t)
	if os.Getenv("AGENTPLUGINS_PROCESS_PREMATURE_PLANNED_HELPER") == "1" {
		fmt.Fprintln(os.Stdout, "verified")
		os.Exit(0)
	}
	environment := append(os.Environ(), "AGENTPLUGINS_PROCESS_PREMATURE_PLANNED_HELPER=1")
	err := (OS{}).RunDuplexWithPlannedShutdown(context.Background(), ports.Command{
		Argv: []string{os.Args[0], "-test.run=TestOSRunnerDuplexPlannedShutdownRejectsExitBeforeTeardownRequest"}, Env: environment,
	}, func(_ io.Writer, stdout io.Reader) error {
		body, err := io.ReadAll(stdout)
		if err != nil {
			return err
		}
		if string(body) != "verified\n" {
			return fmt.Errorf("duplex response = %q", body)
		}
		return nil
	})
	if err == nil || !strings.Contains(err.Error(), "exited before planned process-tree shutdown") {
		t.Fatalf("premature planned-process exit error = %v", err)
	}
}

func TestOSRunnerDuplexPlannedShutdownRejectsNaturalNonzeroExitAtTeardownBoundary(t *testing.T) {
	if os.Getenv("AGENTPLUGINS_PROCESS_NATURAL_NONZERO_BOUNDARY_HELPER") == "1" {
		if _, err := bufio.NewReader(os.Stdin).ReadString('\n'); err != nil {
			os.Exit(46)
		}
		os.Exit(47)
	}
	environment := append(os.Environ(), "AGENTPLUGINS_PROCESS_NATURAL_NONZERO_BOUNDARY_HELPER=1")
	err := runDuplexMode(context.Background(), ports.Command{
		Argv: []string{os.Args[0], "-test.run=TestOSRunnerDuplexPlannedShutdownRejectsNaturalNonzeroExitAtTeardownBoundary"}, Env: environment,
	}, func(stdin io.Writer, stdout io.Reader) error {
		if _, err := io.WriteString(stdin, "verified\n"); err != nil {
			return err
		}
		// EOF proves the helper closed its last inherited output descriptor. The
		// scripted observer then models the narrow stale pre-teardown poll.
		_, err := io.ReadAll(stdout)
		return err
	}, duplexPipeFactory{
		output: os.Pipe,
		input:  func(cmd *exec.Cmd) (io.WriteCloser, error) { return cmd.StdinPipe() },
		containment: func(*exec.Cmd) (duplexCommandContainment, error) {
			return &naturalExitBoundaryContainment{}, nil
		},
	}, true)
	if err == nil || !strings.Contains(err.Error(), "exited before planned process-tree shutdown") {
		t.Fatalf("natural nonzero teardown-boundary exit error = %v", err)
	}
}

func TestOSRunnerDuplexPlannedShutdownDoesNotHideCancellationAtProofBoundary(t *testing.T) {
	requireDuplexCapability(t)
	if os.Getenv("AGENTPLUGINS_PROCESS_CANCELED_PLANNED_HELPER") == "1" {
		line, err := bufio.NewReader(os.Stdin).ReadString('\n')
		if err != nil {
			os.Exit(8)
		}
		fmt.Fprint(os.Stdout, line)
		for {
			time.Sleep(time.Hour)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	environment := append(os.Environ(), "AGENTPLUGINS_PROCESS_CANCELED_PLANNED_HELPER=1")
	err := (OS{}).RunDuplexWithPlannedShutdown(ctx, ports.Command{
		Argv: []string{os.Args[0], "-test.run=TestOSRunnerDuplexPlannedShutdownDoesNotHideCancellationAtProofBoundary"}, Env: environment,
	}, func(stdin io.Writer, stdout io.Reader) error {
		if _, err := io.WriteString(stdin, "verified\n"); err != nil {
			return err
		}
		if line, err := bufio.NewReader(stdout).ReadString('\n'); err != nil || line != "verified\n" {
			return fmt.Errorf("duplex response = %q: %w", line, err)
		}
		cancel()
		return nil
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("planned-shutdown cancellation error = %v, want context cancellation", err)
	}
}

func TestOSRunnerDuplexOutputPipeFailureDoesNotAllocateStdinPipe(t *testing.T) {
	want := errors.New("forced stdout pipe allocation failure")
	stdinCalls := 0
	err := runDuplex(context.Background(), ports.Command{Argv: []string{"unused"}}, func(io.Writer, io.Reader) error {
		return nil
	}, duplexPipeFactory{
		output: func() (*os.File, *os.File, error) { return nil, nil, want },
		input: func(*exec.Cmd) (io.WriteCloser, error) {
			stdinCalls++
			return nil, errors.New("stdin must not be allocated")
		},
	})
	if !errors.Is(err, want) || stdinCalls != 0 {
		t.Fatalf("error = %v stdin allocations = %d, want output failure before stdin allocation", err, stdinCalls)
	}
}

func TestOSRunnerDuplexOutputPipeFailureClosesReturnedEndpoints(t *testing.T) {
	readPipe, writePipe, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	want := errors.New("forced partial stdout pipe allocation failure")
	err = runDuplex(context.Background(), ports.Command{Argv: []string{"unused"}}, func(io.Writer, io.Reader) error {
		return nil
	}, duplexPipeFactory{
		output: func() (*os.File, *os.File, error) { return readPipe, writePipe, want },
		input: func(*exec.Cmd) (io.WriteCloser, error) {
			t.Fatal("stdin must not be allocated after output allocation failure")
			return nil, nil
		},
	})
	if !errors.Is(err, want) {
		t.Fatalf("error = %v, want output allocation failure", err)
	}
	if _, readErr := readPipe.Read(make([]byte, 1)); !errors.Is(readErr, os.ErrClosed) {
		t.Fatalf("returned output reader remains open: %v", readErr)
	}
	if _, writeErr := writePipe.Write([]byte("x")); !errors.Is(writeErr, os.ErrClosed) {
		t.Fatalf("returned output writer remains open: %v", writeErr)
	}
}

func TestOSRunnerDuplexRepeatedContainmentFailureDoesNotAllocateOrLeakPipeDescriptors(t *testing.T) {
	want := errors.New("forced containment allocation failure")
	stdinCalls := 0
	const attempts = 128
	for attempt := 0; attempt < attempts; attempt++ {
		var readPipe, writePipe *os.File
		err := runDuplex(context.Background(), ports.Command{Argv: []string{"unused"}}, func(io.Writer, io.Reader) error {
			return nil
		}, duplexPipeFactory{
			output: func() (*os.File, *os.File, error) {
				var err error
				readPipe, writePipe, err = os.Pipe()
				return readPipe, writePipe, err
			},
			input: func(cmd *exec.Cmd) (io.WriteCloser, error) {
				stdinCalls++
				return cmd.StdinPipe()
			},
			containment: func(*exec.Cmd) (duplexCommandContainment, error) { return nil, want },
		})
		if !errors.Is(err, want) {
			t.Fatalf("attempt %d error = %v, want containment failure", attempt, err)
		}
		if _, err := readPipe.Read(make([]byte, 1)); !errors.Is(err, os.ErrClosed) {
			t.Fatalf("attempt %d retained output reader descriptor: %v", attempt, err)
		}
		if _, err := writePipe.Write([]byte("x")); !errors.Is(err, os.ErrClosed) {
			t.Fatalf("attempt %d retained output writer descriptor: %v", attempt, err)
		}
	}
	if stdinCalls != 0 {
		t.Fatalf("stdin pipe allocations = %d, want zero before repeated containment failures", stdinCalls)
	}
}

func TestOSRunnerDuplexStdinPipeFailureClosesOwnedOutputEndpoints(t *testing.T) {
	readPipe, writePipe, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	want := errors.New("forced stdin pipe allocation failure")
	err = runDuplex(context.Background(), ports.Command{Argv: []string{"unused"}}, func(io.Writer, io.Reader) error {
		return nil
	}, duplexPipeFactory{
		output:      func() (*os.File, *os.File, error) { return readPipe, writePipe, nil },
		input:       func(*exec.Cmd) (io.WriteCloser, error) { return nil, want },
		containment: successfulNoopDuplexContainment,
	})
	if !errors.Is(err, want) {
		t.Fatalf("error = %v, want stdin allocation failure", err)
	}
	if _, readErr := readPipe.Read(make([]byte, 1)); !errors.Is(readErr, os.ErrClosed) {
		t.Fatalf("owned output reader remains open: %v", readErr)
	}
	if _, writeErr := writePipe.Write([]byte("x")); !errors.Is(writeErr, os.ErrClosed) {
		t.Fatalf("owned output writer remains open: %v", writeErr)
	}
}

func TestOSRunnerDuplexStdinPipeFailureClosesReturnedStdinEndpoint(t *testing.T) {
	readOutput, writeOutput, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	readInput, writeInput, err := os.Pipe()
	if err != nil {
		_ = readOutput.Close()
		_ = writeOutput.Close()
		t.Fatal(err)
	}
	defer readInput.Close()
	want := errors.New("forced partial stdin pipe allocation failure")
	err = runDuplex(context.Background(), ports.Command{Argv: []string{"unused"}}, func(io.Writer, io.Reader) error {
		return nil
	}, duplexPipeFactory{
		output:      func() (*os.File, *os.File, error) { return readOutput, writeOutput, nil },
		input:       func(*exec.Cmd) (io.WriteCloser, error) { return writeInput, want },
		containment: successfulNoopDuplexContainment,
	})
	if !errors.Is(err, want) {
		t.Fatalf("error = %v, want stdin allocation failure", err)
	}
	if _, writeErr := writeInput.Write([]byte("x")); !errors.Is(writeErr, os.ErrClosed) {
		t.Fatalf("returned stdin writer remains open: %v", writeErr)
	}
	if _, readErr := readOutput.Read(make([]byte, 1)); !errors.Is(readErr, os.ErrClosed) {
		t.Fatalf("owned output reader remains open: %v", readErr)
	}
	if _, writeErr := writeOutput.Write([]byte("x")); !errors.Is(writeErr, os.ErrClosed) {
		t.Fatalf("owned output writer remains open: %v", writeErr)
	}
}

func TestOSRunnerDuplexClosesStdinAndPreservesCleanExit(t *testing.T) {
	requireDuplexCapability(t)
	if os.Getenv("AGENTPLUGINS_PROCESS_DUPLEX_CLEAN_HELPER") == "1" {
		reader := bufio.NewReader(os.Stdin)
		line, err := reader.ReadString('\n')
		if err != nil {
			os.Exit(40)
		}
		fmt.Fprint(os.Stdout, line)
		if _, err := reader.ReadByte(); !errors.Is(err, io.EOF) {
			os.Exit(41)
		}
		os.Exit(0)
	}
	environment := append(os.Environ(), "AGENTPLUGINS_PROCESS_DUPLEX_CLEAN_HELPER=1")
	err := (OS{}).RunDuplex(context.Background(), ports.Command{
		Argv: []string{os.Args[0], "-test.run=TestOSRunnerDuplexClosesStdinAndPreservesCleanExit"}, Env: environment,
	}, func(stdin io.Writer, stdout io.Reader) error {
		if _, err := io.WriteString(stdin, "request\n"); err != nil {
			return err
		}
		line, err := bufio.NewReader(stdout).ReadString('\n')
		if err != nil || line != "request\n" {
			return fmt.Errorf("duplex response = %q: %w", line, err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestOSRunnerDuplexReturnsNaturalChildExitAndDiagnostic(t *testing.T) {
	requireDuplexCapability(t)
	if os.Getenv("AGENTPLUGINS_PROCESS_DUPLEX_EXIT_HELPER") == "1" {
		fmt.Fprintln(os.Stdout, "connected")
		if line, err := bufio.NewReader(os.Stdin).ReadString('\n'); err != nil || line != "exit\n" {
			os.Exit(22)
		}
		fmt.Fprint(os.Stderr, "fatal after connected")
		os.Exit(23)
	}
	environment := append([]string(nil), os.Environ()...)
	environment = append(environment, "AGENTPLUGINS_PROCESS_DUPLEX_EXIT_HELPER=1")
	err := (OS{}).RunDuplex(context.Background(), ports.Command{
		Argv: []string{os.Args[0], "-test.run=TestOSRunnerDuplexReturnsNaturalChildExitAndDiagnostic"}, Env: environment,
	}, func(stdin io.Writer, stdout io.Reader) error {
		reader := bufio.NewReader(stdout)
		line, err := reader.ReadString('\n')
		if err != nil || line != "connected\n" {
			return fmt.Errorf("duplex response = %q: %w", line, err)
		}
		// The stdin/stdout handoff is a deterministic barrier: the child cannot
		// exit until the exchange releases it, and the exchange cannot return
		// until the child's stdout closes. Thus Wait and exchange completion are
		// both ready at the cleanup scheduling boundary.
		if _, err := io.WriteString(stdin, "exit\n"); err != nil {
			return err
		}
		if _, err := reader.ReadByte(); !errors.Is(err, io.EOF) {
			return fmt.Errorf("wait for natural exit barrier: %w", err)
		}
		return nil
	})
	if err == nil || !strings.Contains(err.Error(), "exit status 23") || !strings.Contains(err.Error(), "fatal after connected") {
		t.Fatalf("error = %v, want child exit status and fatal diagnostic", err)
	}
}

func TestOSRunnerDuplexLeaderExitWithInheritedPipeGroupMemberDoesNotHang(t *testing.T) {
	requireDuplexCapability(t)
	if os.Getenv("AGENTPLUGINS_PROCESS_INHERITED_PIPE_GRANDCHILD") == "1" {
		for {
			time.Sleep(time.Hour)
		}
	}
	if os.Getenv("AGENTPLUGINS_PROCESS_INHERITED_PIPE_LEADER") == "1" {
		grandchild := exec.Command(os.Args[0], "-test.run=TestOSRunnerDuplexLeaderExitWithInheritedPipeGroupMemberDoesNotHang")
		grandchild.Env = append(os.Environ(), "AGENTPLUGINS_PROCESS_INHERITED_PIPE_GRANDCHILD=1")
		grandchild.Stdout = os.Stdout
		grandchild.Stderr = os.Stderr
		if err := grandchild.Start(); err != nil {
			os.Exit(31)
		}
		fmt.Fprintln(os.Stdout, "leader-exiting")
		os.Exit(0)
	}

	environment := append(os.Environ(), "AGENTPLUGINS_PROCESS_INHERITED_PIPE_LEADER=1")
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	started := time.Now()
	err := (OS{}).RunDuplex(ctx, ports.Command{
		Argv: []string{os.Args[0], "-test.run=TestOSRunnerDuplexLeaderExitWithInheritedPipeGroupMemberDoesNotHang"}, Env: environment,
	}, func(_ io.Writer, stdout io.Reader) error {
		reader := bufio.NewReader(stdout)
		if line, readErr := reader.ReadString('\n'); readErr != nil || line != "leader-exiting\n" {
			return fmt.Errorf("leader barrier = %q: %w", line, readErr)
		}
		_, readErr := reader.ReadByte()
		return readErr
	})
	if err == nil || errors.Is(err, context.DeadlineExceeded) || !strings.Contains(err.Error(), "live descendants that required forced cleanup") {
		t.Fatalf("error = %v, want prompt natural-exit process-group cleanup failure", err)
	}
	if elapsed := time.Since(started); elapsed >= 1500*time.Millisecond {
		t.Fatalf("inherited output pipe was not unblocked promptly: %s", elapsed)
	}
}

func TestOSRunnerDuplexPreservesExchangeErrorWhenContextIsCanceled(t *testing.T) {
	requireDuplexCapability(t)
	if os.Getenv("AGENTPLUGINS_PROCESS_CANCEL_HELPER") == "1" {
		for {
			time.Sleep(time.Hour)
		}
	}
	environment := append([]string(nil), os.Environ()...)
	environment = append(environment, "AGENTPLUGINS_PROCESS_CANCEL_HELPER=1")
	ctx, cancel := context.WithCancel(context.Background())
	negative := errors.New("recognized negative exchange evidence")
	err := (OS{}).RunDuplex(ctx, ports.Command{
		Argv: []string{os.Args[0], "-test.run=TestOSRunnerDuplexPreservesExchangeErrorWhenContextIsCanceled"}, Env: environment,
	}, func(io.Writer, io.Reader) error {
		cancel()
		<-ctx.Done()
		return negative
	})
	if !errors.Is(err, negative) || !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want both exchange evidence and context cancellation", err)
	}
}

func requireDuplexCapability(t *testing.T) {
	t.Helper()
	for _, helper := range []string{
		"AGENTPLUGINS_PROCESS_DUPLEX_HELPER",
		"AGENTPLUGINS_PROCESS_DUPLEX_CLEAN_HELPER",
		"AGENTPLUGINS_PROCESS_DUPLEX_EXIT_HELPER",
		"AGENTPLUGINS_PROCESS_INHERITED_PIPE_GRANDCHILD",
		"AGENTPLUGINS_PROCESS_INHERITED_PIPE_LEADER",
		"AGENTPLUGINS_PROCESS_CANCEL_HELPER",
		"AGENTPLUGINS_PROCESS_NATURAL_SIGKILL_HELPER",
		"AGENTPLUGINS_DUPLEX_CGROUP_ESCAPE_CHILD",
		"AGENTPLUGINS_DUPLEX_CGROUP_ESCAPE_PID",
	} {
		if os.Getenv(helper) == "1" {
			return
		}
	}
	if err := (OS{}).DuplexCapability(); err != nil {
		t.Skipf("kernel duplex containment is not delegated to this test environment: %v", err)
	}
}

func TestOSRunnerBoundsStderrWhileRetainingDiagnosticEdges(t *testing.T) {
	requireDuplexCapability(t)
	if os.Getenv("AGENTPLUGINS_PROCESS_STDERR_HELPER") == "1" {
		fmt.Fprint(os.Stderr, "diagnostic-start:")
		fmt.Fprint(os.Stderr, strings.Repeat("x", 256*1024))
		fmt.Fprint(os.Stderr, ":diagnostic-end")
		os.Exit(9)
	}
	environment := append([]string(nil), os.Environ()...)
	environment = append(environment, "AGENTPLUGINS_PROCESS_STDERR_HELPER=1")
	result, err := (OS{}).Run(context.Background(), ports.Command{
		Argv: []string{os.Args[0], "-test.run=TestOSRunnerBoundsStderrWhileRetainingDiagnosticEdges"}, Env: environment,
	})
	if err != nil {
		t.Fatal(err)
	}
	diagnostic := string(result.Stderr)
	if result.ExitCode != 9 || len(result.Stderr) > 34*1024 || !strings.Contains(diagnostic, "diagnostic-start:") ||
		!strings.Contains(diagnostic, ":diagnostic-end") || !strings.Contains(diagnostic, "stderr bytes omitted") {
		t.Fatalf("bounded stderr result: exit=%d bytes=%d diagnostic=%q", result.ExitCode, len(result.Stderr), diagnostic)
	}
}
