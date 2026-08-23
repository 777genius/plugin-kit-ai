//go:build windows

package process

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

type commandTree struct {
	cmd      *exec.Cmd
	job      windows.Handle
	mu       sync.Mutex
	attached bool
	once     sync.Once
}

func newCommandTree(cmd *exec.Cmd) (*commandTree, error) {
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return nil, fmt.Errorf("create process job: %w", err)
	}
	limits := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	limits.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	if _, err := windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&limits)), uint32(unsafe.Sizeof(limits))); err != nil {
		windows.CloseHandle(job)
		return nil, fmt.Errorf("configure process job: %w", err)
	}
	tree := &commandTree{cmd: cmd, job: job}
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.CREATE_SUSPENDED}
	cmd.Cancel = tree.terminate
	return tree, nil
}

func (tree *commandTree) attach(cmd *exec.Cmd) error {
	process, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, uint32(cmd.Process.Pid))
	if err != nil {
		return fmt.Errorf("open suspended process: %w", err)
	}
	defer windows.CloseHandle(process)
	if err := windows.AssignProcessToJobObject(tree.job, process); err != nil {
		return fmt.Errorf("assign process job: %w", err)
	}
	tree.mu.Lock()
	tree.attached = true
	tree.mu.Unlock()
	thread, err := suspendedProcessThread(uint32(cmd.Process.Pid))
	if err != nil {
		return err
	}
	defer windows.CloseHandle(thread)
	if _, err := windows.ResumeThread(thread); err != nil {
		return fmt.Errorf("resume process thread: %w", err)
	}
	return nil
}

func suspendedProcessThread(pid uint32) (windows.Handle, error) {
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPTHREAD, 0)
	if err != nil {
		return 0, fmt.Errorf("snapshot suspended process threads: %w", err)
	}
	defer windows.CloseHandle(snapshot)
	entry := windows.ThreadEntry32{Size: uint32(unsafe.Sizeof(windows.ThreadEntry32{}))}
	for err = windows.Thread32First(snapshot, &entry); err == nil; err = windows.Thread32Next(snapshot, &entry) {
		if entry.OwnerProcessID != pid {
			continue
		}
		thread, openErr := windows.OpenThread(windows.THREAD_SUSPEND_RESUME, false, entry.ThreadID)
		if openErr != nil {
			return 0, fmt.Errorf("open suspended process thread: %w", openErr)
		}
		return thread, nil
	}
	if errors.Is(err, windows.ERROR_NO_MORE_FILES) {
		return 0, fmt.Errorf("suspended process thread not found")
	}
	return 0, fmt.Errorf("enumerate suspended process threads: %w", err)
}

func (tree *commandTree) terminate() error {
	tree.mu.Lock()
	attached := tree.attached
	tree.mu.Unlock()
	if attached {
		if err := windows.TerminateJobObject(tree.job, 1); err == nil {
			return nil
		}
	}
	if tree.cmd.Process == nil {
		return os.ErrProcessDone
	}
	if err := tree.cmd.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
		return err
	}
	return nil
}

func (tree *commandTree) close() {
	tree.once.Do(func() { _ = windows.CloseHandle(tree.job) })
}
