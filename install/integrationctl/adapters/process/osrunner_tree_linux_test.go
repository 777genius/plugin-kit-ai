//go:build linux

package process

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/777genius/plugin-kit-ai/install/integrationctl/ports"
)

func TestOSRunnerPlannedDuplexShutdownContainsImmediateSetsidDescendant(t *testing.T) {
	requireDuplexCapability(t)
	if os.Getenv("AGENTPLUGINS_DUPLEX_CGROUP_ESCAPE_CHILD") == "1" {
		_, _ = syscall.Setsid()
		for {
			time.Sleep(time.Hour)
		}
	}
	if pidPath := os.Getenv("AGENTPLUGINS_DUPLEX_CGROUP_ESCAPE_PID"); pidPath != "" {
		child := exec.Command(os.Args[0], "-test.run=TestOSRunnerPlannedDuplexShutdownContainsImmediateSetsidDescendant")
		child.Env = append(os.Environ(), "AGENTPLUGINS_DUPLEX_CGROUP_ESCAPE_CHILD=1")
		if err := child.Start(); err != nil {
			os.Exit(81)
		}
		if err := os.WriteFile(pidPath, []byte(strconv.Itoa(child.Process.Pid)), 0o600); err != nil {
			os.Exit(82)
		}
		fmt.Fprintln(os.Stdout, "ready")
		for {
			time.Sleep(time.Hour)
		}
	}
	pidPath := t.TempDir() + "/escape.pid"
	environment := append(os.Environ(), "AGENTPLUGINS_DUPLEX_CGROUP_ESCAPE_PID="+pidPath)
	err := (OS{}).RunDuplexWithPlannedShutdown(context.Background(), ports.Command{
		Argv: []string{os.Args[0], "-test.run=TestOSRunnerPlannedDuplexShutdownContainsImmediateSetsidDescendant"}, Env: environment,
	}, func(_ io.Writer, stdout io.Reader) error {
		line, err := bufio.NewReader(stdout).ReadString('\n')
		if err != nil || line != "ready\n" {
			return fmt.Errorf("escape helper readiness = %q: %w", line, err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(pidPath)
	if err != nil {
		t.Fatal(err)
	}
	pid, err := strconv.Atoi(string(body))
	if err != nil {
		t.Fatal(err)
	}
	if err := syscall.Kill(pid, 0); err == nil || !errors.Is(err, syscall.ESRCH) {
		t.Fatalf("immediate setsid descendant %d survived cgroup cleanup: %v", pid, err)
	}
}

func TestOSRunnerCancellationContainsImmediateSetsidDescendant(t *testing.T) {
	requireDuplexCapability(t)
	if os.Getenv("AGENTPLUGINS_RUN_CANCEL_ESCAPE_CHILD") == "1" {
		_, _ = syscall.Setsid()
		for {
			time.Sleep(time.Hour)
		}
	}
	if pidPath := os.Getenv("AGENTPLUGINS_RUN_CANCEL_ESCAPE_PID"); pidPath != "" {
		child := exec.Command(os.Args[0], "-test.run=TestOSRunnerCancellationContainsImmediateSetsidDescendant")
		child.Env = append(os.Environ(), "AGENTPLUGINS_RUN_CANCEL_ESCAPE_CHILD=1")
		if err := child.Start(); err != nil {
			os.Exit(91)
		}
		if err := os.WriteFile(pidPath, []byte(strconv.Itoa(child.Process.Pid)), 0o600); err != nil {
			os.Exit(92)
		}
		for {
			time.Sleep(time.Hour)
		}
	}

	pidPath := t.TempDir() + "/escape.pid"
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := (OS{}).Run(ctx, ports.Command{
			Argv: []string{os.Args[0], "-test.run=TestOSRunnerCancellationContainsImmediateSetsidDescendant"},
			Env:  append(os.Environ(), "AGENTPLUGINS_RUN_CANCEL_ESCAPE_PID="+pidPath),
		})
		done <- err
	}()
	deadline := time.Now().Add(time.Second)
	for {
		if _, err := os.Stat(pidPath); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("escape descendant was not started")
		}
		time.Sleep(time.Millisecond)
	}
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("Run cancellation error = %v, want context cancellation", err)
	}
	body, err := os.ReadFile(pidPath)
	if err != nil {
		t.Fatal(err)
	}
	pid, err := strconv.Atoi(string(body))
	if err != nil {
		t.Fatal(err)
	}
	if processStillRunning(pid) {
		t.Fatalf("setsid descendant %d survived Run cancellation", pid)
	}
}

func TestDuplexCapabilityRejectsNonCgroupDelegationBeforeStart(t *testing.T) {
	_, _, err := createDelegatedCgroupAt(t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "cgroup") {
		t.Fatalf("capability primitive error = %v, want honest pre-start unsupported result", err)
	}
}

func TestDuplexCapabilityProbeUnprovedShutdownWaitIsHardBoundAndRetainsSoleReaper(t *testing.T) {
	waitRelease := make(chan struct{})
	waitStarted := make(chan struct{})
	waitReturned := make(chan struct{})
	defer func() {
		close(waitRelease)
		select {
		case <-waitReturned:
		case <-time.After(time.Second):
			t.Fatal("probe's owned Wait did not finish after simulated exit")
		}
	}()
	started := time.Now()
	terminationErr := errors.New("containment and fallback shutdown failed")
	err := finishDuplexContainmentProbe(terminationResult{err: terminationErr}, func() error { return nil }, func() error {
		close(waitStarted)
		<-waitRelease
		close(waitReturned)
		return nil
	}, time.Millisecond)
	<-waitStarted
	if err == nil || !errors.Is(err, terminationErr) || !strings.Contains(err.Error(), "shutdown was not proved") || !strings.Contains(err.Error(), "exceeded cleanup deadline") || !strings.Contains(err.Error(), "sole reaper retains ownership until process exit") {
		t.Fatalf("unproved stuck probe Wait error = %v, want bounded fail-closed owned-reaper failure", err)
	}
	if elapsed := time.Since(started); elapsed > 100*time.Millisecond {
		t.Fatalf("stuck probe Wait returned too slowly after %s", elapsed)
	}
}

func TestDuplexCapabilityProbeCannotSucceedWithoutProvenLeaderStop(t *testing.T) {
	waitCalls := 0
	err := finishDuplexContainmentProbe(terminationResult{}, func() error { return nil }, func() error {
		waitCalls++
		return nil
	}, time.Second)
	if err == nil || !strings.Contains(err.Error(), "shutdown was not proved") || waitCalls != 1 {
		t.Fatalf("unproved probe cleanup error = %v wait calls = %d, want fail-closed sole reap", err, waitCalls)
	}
}

func TestLinuxTrackerTimeoutTransfersHandleOwnershipUntilJoin(t *testing.T) {
	done := make(chan struct{})
	containment := &commandContainment{
		cmd: &exec.Cmd{Process: &os.Process{Pid: os.Getpid()}}, descendants: make(map[int]linuxTrackedProcess),
		stop: make(chan struct{}), done: done, trackerStarted: true,
	}
	err := containment.close()
	if err == nil || !strings.Contains(err.Error(), "ownership transferred") {
		t.Fatalf("stalled tracker close error = %v", err)
	}
	containment.mu.Lock()
	owned := containment.descendants != nil
	containment.mu.Unlock()
	if !owned {
		t.Fatal("tracker-owned map was released before the tracker joined")
	}
	close(done)
	deadline := time.Now().Add(time.Second)
	for {
		containment.mu.Lock()
		released := containment.descendants == nil
		containment.mu.Unlock()
		if released {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("owned tracker reaper did not release handles after join")
		}
		time.Sleep(time.Millisecond)
	}
}

func TestOSRunnerCleansSameProcessGroupMemberAfterLeaderNaturalExit(t *testing.T) {
	requireDuplexCapability(t)
	if os.Getenv("AGENTPLUGINS_PROCESS_RUN_DESCENDANT") == "1" {
		for {
			time.Sleep(time.Hour)
		}
	}
	if pidPath := os.Getenv("AGENTPLUGINS_PROCESS_RUN_LEADER_PID_PATH"); pidPath != "" {
		child := exec.Command(os.Args[0], "-test.run=TestOSRunnerCleansSameProcessGroupMemberAfterLeaderNaturalExit")
		child.Env = append(os.Environ(), "AGENTPLUGINS_PROCESS_RUN_DESCENDANT=1")
		devNull, err := os.OpenFile(os.DevNull, os.O_RDWR, 0)
		if err != nil {
			os.Exit(61)
		}
		child.Stdin = devNull
		child.Stdout = devNull
		child.Stderr = devNull
		if err := child.Start(); err != nil {
			os.Exit(62)
		}
		if err := os.WriteFile(pidPath, []byte(strconv.Itoa(child.Process.Pid)), 0o600); err != nil {
			os.Exit(63)
		}
		os.Exit(0)
	}

	pidPath := t.TempDir() + "/descendant.pid"
	environment := append(os.Environ(), "AGENTPLUGINS_PROCESS_RUN_LEADER_PID_PATH="+pidPath)
	_, err := (OS{}).RunWithTreeExitGrace(context.Background(), ports.Command{
		Argv: []string{os.Args[0], "-test.run=TestOSRunnerCleansSameProcessGroupMemberAfterLeaderNaturalExit"},
		Env:  environment,
	}, time.Millisecond)
	if err == nil || !strings.Contains(err.Error(), "live descendants that required forced cleanup") {
		t.Fatalf("clean leader exit error = %v, want forced-descendant causality", err)
	}
	body, readErr := os.ReadFile(pidPath)
	if readErr != nil {
		t.Fatal(readErr)
	}
	pid, parseErr := strconv.Atoi(string(body))
	if parseErr != nil {
		t.Fatal(parseErr)
	}
	deadline := time.Now().Add(time.Second)
	for processStillRunning(pid) && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if processStillRunning(pid) {
		t.Fatalf("same-process-group member %d survived leader cleanup", pid)
	}
}

func TestOSRunnerPreservesDiagnosticAndStatusWhileCleaningNaturalExitGroup(t *testing.T) {
	requireDuplexCapability(t)
	if os.Getenv("AGENTPLUGINS_PROCESS_DIAGNOSTIC_DESCENDANT") == "1" {
		for {
			time.Sleep(time.Hour)
		}
	}
	if pidPath := os.Getenv("AGENTPLUGINS_PROCESS_DIAGNOSTIC_LEADER_PID_PATH"); pidPath != "" {
		child := exec.Command(os.Args[0], "-test.run=TestOSRunnerPreservesDiagnosticAndStatusWhileCleaningNaturalExitGroup")
		child.Env = append(os.Environ(), "AGENTPLUGINS_PROCESS_DIAGNOSTIC_DESCENDANT=1")
		devNull, err := os.OpenFile(os.DevNull, os.O_RDWR, 0)
		if err != nil {
			os.Exit(71)
		}
		child.Stdin = devNull
		child.Stdout = devNull
		child.Stderr = devNull
		if err := child.Start(); err != nil {
			os.Exit(72)
		}
		if err := os.WriteFile(pidPath, []byte(strconv.Itoa(child.Process.Pid)), 0o600); err != nil {
			os.Exit(73)
		}
		fmt.Fprint(os.Stderr, "fatal bounded-process diagnostic")
		os.Exit(47)
	}

	pidPath := t.TempDir() + "/descendant.pid"
	environment := append(os.Environ(), "AGENTPLUGINS_PROCESS_DIAGNOSTIC_LEADER_PID_PATH="+pidPath)
	_, err := (OS{}).RunWithTreeExitGrace(context.Background(), ports.Command{
		Argv: []string{os.Args[0], "-test.run=TestOSRunnerPreservesDiagnosticAndStatusWhileCleaningNaturalExitGroup"},
		Env:  environment,
	}, time.Millisecond)
	if err == nil || !strings.Contains(err.Error(), "live descendants that required forced cleanup") || !strings.Contains(err.Error(), "fatal bounded-process diagnostic") {
		t.Fatalf("error = %v, want forced-descendant causality and bounded diagnostic", err)
	}
	body, readErr := os.ReadFile(pidPath)
	if readErr != nil {
		t.Fatal(readErr)
	}
	pid, parseErr := strconv.Atoi(string(body))
	if parseErr != nil {
		t.Fatal(parseErr)
	}
	deadline := time.Now().Add(time.Second)
	for processStillRunning(pid) && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if processStillRunning(pid) {
		t.Fatalf("same-process-group member %d survived diagnostic leader cleanup", pid)
	}
}

func TestOSRunnerCleansTrackedDescendantThatCreatesNewSession(t *testing.T) {
	requireDuplexCapability(t)
	if os.Getenv("AGENTPLUGINS_PROCESS_SETSID_DESCENDANT") == "1" {
		for {
			time.Sleep(time.Hour)
		}
	}
	if pidPath := os.Getenv("AGENTPLUGINS_PROCESS_SETSID_LEADER_PID_PATH"); pidPath != "" {
		child := exec.Command(os.Args[0], "-test.run=TestOSRunnerCleansTrackedDescendantThatCreatesNewSession")
		child.Env = append(os.Environ(), "AGENTPLUGINS_PROCESS_SETSID_DESCENDANT=1")
		child.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
		devNull, err := os.OpenFile(os.DevNull, os.O_RDWR, 0)
		if err != nil {
			os.Exit(81)
		}
		child.Stdin, child.Stdout, child.Stderr = devNull, devNull, devNull
		if err := child.Start(); err != nil {
			os.Exit(82)
		}
		if err := os.WriteFile(pidPath, []byte(strconv.Itoa(child.Process.Pid)), 0o600); err != nil {
			os.Exit(83)
		}
		// Exit immediately after the child is observable to model a sub-millisecond
		// setsid escape. Atomic cgroup placement, not sampling, must contain it.
		os.Exit(0)
	}

	pidPath := t.TempDir() + "/setsid-descendant.pid"
	_, err := (OS{}).RunWithTreeExitGrace(context.Background(), ports.Command{
		Argv: []string{os.Args[0], "-test.run=TestOSRunnerCleansTrackedDescendantThatCreatesNewSession"},
		Env:  append(os.Environ(), "AGENTPLUGINS_PROCESS_SETSID_LEADER_PID_PATH="+pidPath),
	}, time.Millisecond)
	if err == nil || !strings.Contains(err.Error(), "live descendants that required forced cleanup") {
		t.Fatalf("clean leader exit with tracked setsid descendant error = %v, want forced-descendant causality", err)
	}
	body, err := os.ReadFile(pidPath)
	if err != nil {
		t.Fatal(err)
	}
	pid, err := strconv.Atoi(string(body))
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
	for processStillRunning(pid) && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if processStillRunning(pid) {
		t.Fatalf("tracked new-session descendant %d survived leader cleanup", pid)
	}
}

func TestCgroupMemberInspectionFailurePreservesForcedCleanupCausality(t *testing.T) {
	want := errors.New("cgroup.procs unreadable")
	forced, err := cgroupForcedMemberCausality(1234, nil, want)
	if !forced || !errors.Is(err, want) {
		t.Fatalf("forced = %v error = %v, want uncertain forced cleanup causality", forced, err)
	}
}

func TestFailedCgroupKillCannotClaimTreeShutdown(t *testing.T) {
	originalRead, originalWrite := readCgroupProcs, writeCgroupKill
	defer func() { readCgroupProcs, writeCgroupKill = originalRead, originalWrite }()
	readCgroupProcs = func(string) ([]byte, error) { return []byte("1234\n5678\n"), nil }
	killFailure := errors.New("cgroup.kill failed")
	writeCgroupKill = func(string, []byte, os.FileMode) error { return killFailure }
	containment := &commandContainment{
		cmd:          &exec.Cmd{Process: &os.Process{Pid: 1234}},
		cgroupPath:   "/deterministic-test-cgroup",
		observeExit:  func() (bool, error) { return true, nil },
		observeEmpty: func() (bool, error) { return false, nil },
	}
	result := containment.terminateCgroupWithin(time.Millisecond)
	if !result.leaderStopped || !result.forcedMembers || !errors.Is(result.err, killFailure) || !strings.Contains(result.err.Error(), "did not empty") {
		t.Fatalf("termination = %+v, want observed leader plus failed tree shutdown", result)
	}
}

func TestSuccessfulLeaderKillRequestRequiresObservedShutdown(t *testing.T) {
	result := stopLeaderAfterContainmentFailure(terminationResult{}, func() error { return nil }, func() (bool, error) { return false, errors.New("observation failed") })
	if result.leaderStopped || result.err == nil || !strings.Contains(result.err.Error(), "confirm process leader stopped") {
		t.Fatalf("termination = %+v, want unconfirmed leader shutdown failure", result)
	}
}

func processStillRunning(pid int) bool {
	stat, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if os.IsNotExist(err) {
		return false
	}
	if err != nil {
		return true
	}
	closing := strings.LastIndexByte(string(stat), ')')
	return closing < 0 || len(stat) <= closing+2 || stat[closing+2] != 'Z'
}
