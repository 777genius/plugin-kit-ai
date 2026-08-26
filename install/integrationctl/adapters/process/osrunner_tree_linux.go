//go:build linux

package process

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

type commandContainment struct {
	cmd              *exec.Cmd
	mu               sync.Mutex
	descendants      map[int]linuxTrackedProcess
	scanErr          error
	stop             chan struct{}
	done             chan struct{}
	trackerStarted   bool
	groupOnly        bool
	cgroupPath       string
	cgroupDir        *os.File
	closeTransferred bool
	observeExit      func() (bool, error)
	observeEmpty     func() (bool, error)
}

// This interval is frequent enough to observe normal Kiro startup forks while
// avoiding high-frequency process inspection for long-lived clients.
const linuxDescendantScanInterval = 25 * time.Millisecond

const linuxStartupDescendantScanInterval = time.Millisecond
const linuxStartupDescendantScanWindow = 250 * time.Millisecond

func explicitContainmentSupervision() bool { return true }

const linuxCgroupRoot = "/sys/fs/cgroup"

func duplexContainmentPreflight() error {
	cmd := exec.Command("/bin/sh", "-c", "while :; do :; done")
	containment, err := newDuplexCommandContainment(cmd)
	if err != nil {
		return fmt.Errorf("safe duplex process containment unavailable; manual activation required: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return errors.Join(fmt.Errorf("prove CLONE_INTO_CGROUP process creation: %w", err), containment.close())
	}
	if err := containment.attach(cmd); err != nil {
		return fmt.Errorf("attach duplex containment primitive probe: %w", cleanupAttachFailure(err, containment.terminate, cmd.Process.Kill, containment.exited, containment.close, cmd.Wait))
	}
	termination := containment.terminate()
	if !termination.leaderStopped {
		termination = stopLeaderAfterContainmentFailure(termination, cmd.Process.Kill, containment.exited)
	}
	return finishDuplexContainmentProbe(termination, containment.close, cmd.Wait, processReapTimeout)
}

func finishDuplexContainmentProbe(termination terminationResult, closeContainment, wait func() error, timeout time.Duration) error {
	// Close probe-owned containment descriptors before starting its sole Wait.
	// If shutdown cannot be proved or Wait itself stalls, the buffered reaper
	// keeps ownership until actual exit while this capability check fails closed
	// within a real deadline.
	waitErr := closeContainmentAndWait(closeContainment, wait, timeout)
	var proofErr error
	if !termination.leaderStopped {
		proofErr = fmt.Errorf("probe leader shutdown was not proved")
	}
	if termination.err != nil || proofErr != nil {
		return fmt.Errorf("safe duplex process containment primitive probe failed: %w", errors.Join(termination.err, proofErr, waitErr))
	}
	if waitErr != nil {
		if _, expectedKill := waitErr.(*exec.ExitError); !expectedKill {
			return fmt.Errorf("reap duplex containment primitive probe: %w", waitErr)
		}
	}
	return nil
}

func naturalExitNeedsContainmentCleanup() bool { return true }

func newCommandContainment(cmd *exec.Cmd) (*commandContainment, error) {
	// Tree-aware non-duplex commands (notably Git acquisition) require the same
	// atomic kernel boundary as duplex ACP. Sampling /proc cannot prove cleanup
	// for a sub-millisecond setsid/double-fork escape.
	return newDuplexCommandContainment(cmd)
}

func newDuplexCommandContainment(cmd *exec.Cmd) (*commandContainment, error) {
	path, directory, err := createDelegatedCgroup()
	if err != nil {
		return nil, fmt.Errorf("create duplex cgroup containment: %w", err)
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true, UseCgroupFD: true, CgroupFD: int(directory.Fd())}
	return &commandContainment{cmd: cmd, cgroupPath: path, cgroupDir: directory}, nil
}

func createDelegatedCgroup() (string, *os.File, error) {
	return createDelegatedCgroupAt(linuxCgroupRoot)
}

func createDelegatedCgroupAt(root string) (string, *os.File, error) {
	value, err := os.ReadFile("/proc/self/cgroup")
	if err != nil {
		return "", nil, err
	}
	var relative string
	for _, line := range strings.Split(string(value), "\n") {
		if strings.HasPrefix(line, "0::") {
			relative = strings.TrimPrefix(line, "0::")
			break
		}
	}
	if relative == "" || !filepath.IsAbs(relative) {
		return "", nil, fmt.Errorf("unified cgroup v2 membership is unavailable")
	}
	base := filepath.Join(root, filepath.Clean(relative))
	var filesystem unix.Statfs_t
	if err := unix.Statfs(base, &filesystem); err != nil {
		return "", nil, fmt.Errorf("inspect cgroup filesystem: %w", err)
	}
	if filesystem.Type != unix.CGROUP2_SUPER_MAGIC {
		return "", nil, fmt.Errorf("unified cgroup v2 filesystem is unavailable")
	}
	path, err := os.MkdirTemp(base, "agentplugins-duplex-")
	if err != nil {
		return "", nil, fmt.Errorf("delegated cgroup v2 subtree is unavailable: %w", err)
	}
	directory, err := os.Open(path)
	if err != nil {
		_ = os.Remove(path)
		return "", nil, err
	}
	for _, control := range []string{"cgroup.events", "cgroup.kill", "cgroup.procs"} {
		if _, err := os.Stat(filepath.Join(path, control)); err != nil {
			_ = directory.Close()
			_ = os.Remove(path)
			return "", nil, fmt.Errorf("required cgroup v2 control %s is unavailable: %w", control, err)
		}
	}
	return path, directory, nil
}

func (containment *commandContainment) attach(cmd *exec.Cmd) error {
	if cmd.Process == nil {
		return fmt.Errorf("process supervision cannot attach before start")
	}
	if containment.cgroupPath != "" {
		return nil // CLONE_INTO_CGROUP attached the child atomically at creation.
	}
	if containment.groupOnly {
		return nil
	}
	identity, err := readLinuxProcessIdentity(cmd.Process.Pid)
	if err != nil {
		return fmt.Errorf("read Linux process leader identity: %w", err)
	}
	pidfd, err := unix.PidfdOpen(identity.pid, 0)
	if err != nil {
		return fmt.Errorf("open stable Linux process handle: %w", err)
	}
	containment.mu.Lock()
	containment.descendants[identity.pid] = linuxTrackedProcess{startTime: identity.startTime, pidfd: pidfd}
	containment.mu.Unlock()
	if err := containment.scanDescendants(); err != nil {
		return err
	}
	containment.trackerStarted = true
	go containment.trackDescendants()
	return nil
}

func (containment *commandContainment) limitToProcessGroup() { containment.groupOnly = true }

func (containment *commandContainment) terminate() terminationResult {
	if containment.cmd.Process == nil {
		return terminationResult{err: os.ErrProcessDone}
	}
	if containment.cgroupPath != "" {
		return containment.terminateCgroup()
	}
	scanErr := containment.scanDescendants()
	forcedMembers := false
	groupMembers, groupScanErr := liveLinuxProcessGroupMembers(containment.cmd.Process.Pid)
	if groupMembers > 0 {
		forcedMembers = true
	}
	scanErr = errors.Join(scanErr, groupScanErr)
	containment.mu.Lock()
	for pid, tracked := range containment.descendants {
		if pid != containment.cmd.Process.Pid && linuxPidfdAlive(tracked.pidfd) {
			forcedMembers = true
		}
	}
	containment.mu.Unlock()
	groupErr := syscall.Kill(-containment.cmd.Process.Pid, syscall.SIGKILL)
	if errors.Is(groupErr, syscall.ESRCH) {
		groupErr = nil
	}
	containment.mu.Lock()
	identities := make(map[int]linuxTrackedProcess, len(containment.descendants))
	for pid, tracked := range containment.descendants {
		identities[pid] = tracked
	}
	trackerErr := containment.scanErr
	containment.mu.Unlock()
	var cleanupErr = errors.Join(scanErr, trackerErr)
	for pid, tracked := range identities {
		if pid == containment.cmd.Process.Pid {
			continue
		}
		identity, err := readLinuxProcessIdentity(pid)
		if errors.Is(err, os.ErrNotExist) || (err == nil && identity.zombie) {
			continue
		}
		if err != nil || identity.startTime != tracked.startTime {
			cleanupErr = errors.Join(cleanupErr, fmt.Errorf("prove tracked descendant %d identity before cleanup: %w", pid, err))
			continue
		}
		if err := unix.PidfdSendSignal(tracked.pidfd, unix.SIGKILL, nil, 0); err != nil && !errors.Is(err, syscall.ESRCH) {
			cleanupErr = errors.Join(cleanupErr, fmt.Errorf("terminate tracked descendant %d through pidfd: %w", pid, err))
		}
	}
	deadline := time.Now().Add(time.Second)
	for {
		leaderStopped, observeErr := containment.exited()
		remaining := 0
		for pid, tracked := range identities {
			if pid == containment.cmd.Process.Pid {
				continue
			}
			identity, err := readLinuxProcessIdentity(pid)
			if err == nil && identity.startTime == tracked.startTime && !identity.zombie && linuxPidfdAlive(tracked.pidfd) {
				remaining++
			} else if err != nil && linuxPidfdAlive(tracked.pidfd) {
				cleanupErr = errors.Join(cleanupErr, fmt.Errorf("prove tracked descendant %d stopped: %w", pid, err))
			}
		}
		if leaderStopped && remaining == 0 {
			return terminationResult{leaderStopped: true, forcedMembers: forcedMembers, err: errors.Join(groupErr, cleanupErr)}
		}
		if observeErr != nil {
			cleanupErr = errors.Join(cleanupErr, observeErr)
		}
		if time.Now().After(deadline) {
			if !leaderStopped {
				cleanupErr = errors.Join(cleanupErr, fmt.Errorf("process leader did not become waitable before cleanup deadline"))
			}
			if remaining != 0 {
				cleanupErr = errors.Join(cleanupErr, fmt.Errorf("%d tracked descendant process(es) survived cleanup deadline", remaining))
			}
			return terminationResult{leaderStopped: leaderStopped, forcedMembers: forcedMembers, err: errors.Join(groupErr, cleanupErr)}
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func (containment *commandContainment) exited() (bool, error) {
	if containment.observeExit != nil {
		return containment.observeExit()
	}
	if containment.cmd.Process == nil {
		return false, nil
	}
	var info unix.Siginfo
	if err := unix.Waitid(unix.P_PID, containment.cmd.Process.Pid, &info, unix.WEXITED|unix.WNOWAIT|unix.WNOHANG, nil); err != nil {
		return false, err
	}
	return info.Signo != 0, nil
}

func (containment *commandContainment) settleNaturalExit(ctx context.Context, timeout time.Duration) (bool, error) {
	if containment.cgroupPath != "" {
		return containment.settleCgroup(ctx, timeout)
	}
	deadline := time.Now().Add(timeout)
	for {
		members, err := liveLinuxProcessGroupMembers(containment.cmd.Process.Pid)
		if err != nil {
			return false, err
		}
		containment.mu.Lock()
		for pid, tracked := range containment.descendants {
			if pid == containment.cmd.Process.Pid {
				continue
			}
			identity, identityErr := readLinuxProcessIdentity(pid)
			if identityErr == nil && !identity.zombie && linuxPidfdAlive(tracked.pidfd) {
				members++
			}
		}
		containment.mu.Unlock()
		if members == 0 {
			return true, nil
		}
		if time.Now().After(deadline) {
			return false, nil
		}
		select {
		case <-ctx.Done():
			return false, ctx.Err()
		case <-time.After(5 * time.Millisecond):
		}
	}
}

func (containment *commandContainment) close() error {
	if containment.closeTransferred {
		return fmt.Errorf("Linux descendant tracker ownership was transferred to its reaper")
	}
	if containment.cgroupPath != "" {
		if containment.cgroupDir != nil {
			_ = containment.cgroupDir.Close()
			containment.cgroupDir = nil
		}
		if err := os.Remove(containment.cgroupPath); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("remove duplex cgroup scope: %w", err)
		}
		containment.cgroupPath = ""
		return nil
	}
	if containment.cmd.Process == nil {
		return nil
	}
	var resultErr error
	if containment.trackerStarted {
		select {
		case <-containment.stop:
		default:
			close(containment.stop)
		}
		select {
		case <-containment.done:
		case <-time.After(500 * time.Millisecond):
			containment.closeTransferred = true
			go containment.closeTrackedHandlesAfterStop()
			return fmt.Errorf("Linux descendant tracker ownership transferred to reaper after stop deadline")
		}
	}
	containment.mu.Lock()
	for pid, tracked := range containment.descendants {
		if err := unix.Close(tracked.pidfd); err != nil {
			resultErr = errors.Join(resultErr, fmt.Errorf("close stable process handle for %d: %w", pid, err))
		}
	}
	containment.descendants = nil
	containment.mu.Unlock()
	return resultErr
}

func (containment *commandContainment) closeTrackedHandlesAfterStop() {
	<-containment.done
	containment.mu.Lock()
	defer containment.mu.Unlock()
	for _, tracked := range containment.descendants {
		_ = unix.Close(tracked.pidfd)
	}
	containment.descendants = nil
}

var readCgroupProcs = os.ReadFile
var writeCgroupKill = os.WriteFile

func (containment *commandContainment) terminateCgroup() terminationResult {
	return containment.terminateCgroupWithin(time.Second)
}

func (containment *commandContainment) terminateCgroupWithin(timeout time.Duration) terminationResult {
	members, inspectErr := readCgroupProcs(filepath.Join(containment.cgroupPath, "cgroup.procs"))
	forced, inspectErr := cgroupForcedMemberCausality(containment.cmd.Process.Pid, members, inspectErr)
	killErr := writeCgroupKill(filepath.Join(containment.cgroupPath, "cgroup.kill"), []byte("1"), 0)
	if errors.Is(killErr, os.ErrNotExist) {
		killErr = nil
	}
	deadline := time.Now().Add(timeout)
	for {
		exited, observeErr := containment.exited()
		empty, emptyErr := containment.cgroupEmpty()
		if exited && empty {
			return terminationResult{leaderStopped: true, forcedMembers: forced, err: errors.Join(inspectErr, killErr, observeErr, emptyErr)}
		}
		if time.Now().After(deadline) {
			return terminationResult{leaderStopped: exited, forcedMembers: forced, err: errors.Join(inspectErr, killErr, observeErr, emptyErr, fmt.Errorf("duplex cgroup did not empty before cleanup deadline"))}
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func cgroupForcedMemberCausality(leaderPID int, members []byte, inspectErr error) (bool, error) {
	if inspectErr != nil {
		// Once cgroup.kill is requested, inability to inventory the scope makes
		// descendant causality uncertain. Fail closed so a natural-looking leader
		// status cannot turn possibly forced cleanup into success.
		return true, fmt.Errorf("inspect cgroup members before forced cleanup: %w", inspectErr)
	}
	for _, raw := range strings.Fields(string(members)) {
		if raw != strconv.Itoa(leaderPID) {
			return true, nil
		}
	}
	return false, nil
}

func (containment *commandContainment) settleCgroup(ctx context.Context, timeout time.Duration) (bool, error) {
	deadline := time.Now().Add(timeout)
	for {
		empty, err := containment.cgroupEmpty()
		if err != nil || empty {
			return empty, err
		}
		if time.Now().After(deadline) {
			return false, nil
		}
		select {
		case <-ctx.Done():
			return false, ctx.Err()
		case <-time.After(5 * time.Millisecond):
		}
	}
}

func (containment *commandContainment) cgroupEmpty() (bool, error) {
	if containment.observeEmpty != nil {
		return containment.observeEmpty()
	}
	value, err := os.ReadFile(filepath.Join(containment.cgroupPath, "cgroup.events"))
	if err != nil {
		return false, err
	}
	for _, line := range strings.Split(string(value), "\n") {
		if strings.TrimSpace(line) == "populated 0" {
			return true, nil
		}
	}
	return false, nil
}

type linuxProcessIdentity struct {
	pid       int
	ppid      int
	pgid      int
	startTime uint64
	zombie    bool
}

func readLinuxProcessIdentity(pid int) (linuxProcessIdentity, error) {
	value, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return linuxProcessIdentity{}, err
	}
	closing := strings.LastIndexByte(string(value), ')')
	if closing < 0 || closing+2 >= len(value) {
		return linuxProcessIdentity{}, fmt.Errorf("malformed /proc stat for pid %d", pid)
	}
	fields := strings.Fields(string(value[closing+2:]))
	if len(fields) < 20 {
		return linuxProcessIdentity{}, fmt.Errorf("short /proc stat for pid %d", pid)
	}
	startTime, err := strconv.ParseUint(fields[19], 10, 64)
	if err != nil {
		return linuxProcessIdentity{}, err
	}
	ppid, err := strconv.Atoi(fields[1])
	if err != nil {
		return linuxProcessIdentity{}, err
	}
	pgid, err := strconv.Atoi(fields[2])
	if err != nil {
		return linuxProcessIdentity{}, err
	}
	return linuxProcessIdentity{pid: pid, ppid: ppid, pgid: pgid, startTime: startTime, zombie: fields[0] == "Z"}, nil
}

func liveLinuxProcessGroupMembers(pgid int) (int, error) {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return 0, fmt.Errorf("inspect Linux process table for group containment: %w", err)
	}
	members := 0
	for _, entry := range entries {
		pid, err := strconv.Atoi(entry.Name())
		if err != nil || pid == pgid {
			continue
		}
		identity, err := readLinuxProcessIdentity(pid)
		if err != nil {
			continue // unrelated processes routinely exit during a table snapshot
		}
		if identity.pgid == pgid && !identity.zombie {
			members++
		}
	}
	return members, nil
}

type linuxTrackedProcess struct {
	startTime uint64
	pidfd     int
}

func linuxPidfdAlive(pidfd int) bool {
	err := unix.PidfdSendSignal(pidfd, 0, nil, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}

func (containment *commandContainment) trackDescendants() {
	defer close(containment.done)
	ticker := time.NewTicker(linuxStartupDescendantScanInterval)
	defer ticker.Stop()
	startup := time.NewTimer(linuxStartupDescendantScanWindow)
	defer startup.Stop()
	for {
		select {
		case <-ticker.C:
			if err := containment.scanDescendants(); err != nil {
				containment.mu.Lock()
				containment.scanErr = errors.Join(containment.scanErr, err)
				containment.mu.Unlock()
			}
		case <-startup.C:
			ticker.Reset(linuxDescendantScanInterval)
		case <-containment.stop:
			return
		}
	}
}

func (containment *commandContainment) scanDescendants() error {
	containment.mu.Lock()
	defer containment.mu.Unlock()
	queue := make([]int, 0, len(containment.descendants)+1)
	queue = append(queue, containment.cmd.Process.Pid)
	for pid, tracked := range containment.descendants {
		if identity, err := readLinuxProcessIdentity(pid); err == nil && identity.startTime == tracked.startTime && !identity.zombie {
			queue = append(queue, pid)
		}
	}
	var scanErr error
	seen := make(map[int]struct{}, len(queue))
	for len(queue) != 0 {
		pid := queue[0]
		queue = queue[1:]
		if _, duplicate := seen[pid]; duplicate {
			continue
		}
		seen[pid] = struct{}{}
		tasks, err := os.ReadDir(fmt.Sprintf("/proc/%d/task", pid))
		if err != nil {
			if tracked, ok := containment.descendants[pid]; ok && linuxPidfdAlive(tracked.pidfd) {
				scanErr = errors.Join(scanErr, fmt.Errorf("inspect tasks for live tracked process %d: %w", pid, err))
			}
			continue
		}
		for _, task := range tasks {
			children, err := os.ReadFile(fmt.Sprintf("/proc/%d/task/%s/children", pid, task.Name()))
			if err != nil {
				// Threads can disappear between ReadDir and opening their children
				// file. The remaining live threads are still inspected in this same
				// snapshot, so this race is not an ancestry blind spot.
				if errors.Is(err, os.ErrNotExist) {
					continue
				}
				if tracked, ok := containment.descendants[pid]; ok && linuxPidfdAlive(tracked.pidfd) {
					scanErr = errors.Join(scanErr, fmt.Errorf("inspect children for live tracked process %d: %w", pid, err))
				}
				continue
			}
			for _, rawChild := range strings.Fields(string(children)) {
				child, err := strconv.Atoi(rawChild)
				if err != nil {
					continue
				}
				identity, err := readLinuxProcessIdentity(child)
				if err != nil {
					scanErr = errors.Join(scanErr, fmt.Errorf("read observed child %d of %d: %w", child, pid, err))
					continue
				}
				pidfd, err := unix.PidfdOpen(child, 0)
				if err != nil {
					scanErr = errors.Join(scanErr, fmt.Errorf("open stable handle for observed child %d: %w", child, err))
					continue
				}
				revalidated, err := readLinuxProcessIdentity(child)
				if err != nil || revalidated.startTime != identity.startTime {
					_ = unix.Close(pidfd)
					if err != nil {
						scanErr = errors.Join(scanErr, fmt.Errorf("revalidate observed child %d identity: %w", child, err))
					} else {
						scanErr = errors.Join(scanErr, fmt.Errorf("revalidate observed child %d identity: process start time changed", child))
					}
					continue
				}
				if revalidated.ppid != pid {
					parent, parentErr := readLinuxProcessIdentity(pid)
					if parentErr == nil && !parent.zombie {
						_ = unix.Close(pidfd)
						scanErr = errors.Join(scanErr, fmt.Errorf("revalidate observed child %d parentage: live parent changed from %d to %d", child, pid, revalidated.ppid))
						continue
					}
				}
				// Re-read parentage after acquiring the pidfd. A changed parent is
				// safe only because the kernel children file already established the
				// relationship and the stable handle proves the same process survived
				// while its parent exited/reparented it. Numeric signaling is never used.
				if previous, exists := containment.descendants[child]; exists {
					_ = unix.Close(pidfd)
					if previous.startTime != revalidated.startTime {
						scanErr = errors.Join(scanErr, fmt.Errorf("tracked child PID %d changed identity", child))
						continue
					}
				} else {
					containment.descendants[child] = linuxTrackedProcess{startTime: revalidated.startTime, pidfd: pidfd}
				}
				queue = append(queue, child)
			}
		}
	}
	return scanErr
}
