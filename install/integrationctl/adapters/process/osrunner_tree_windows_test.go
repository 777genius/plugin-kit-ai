//go:build windows

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
	"testing"
	"time"

	"github.com/777genius/plugin-kit-ai/install/integrationctl/ports"
	"golang.org/x/sys/windows"
)

func TestOSRunnerTerminatesWindowsGrandchildTree(t *testing.T) {
	powershell, err := exec.LookPath("powershell.exe")
	if err != nil {
		t.Skip("powershell.exe is unavailable")
	}
	root := t.TempDir()
	childScript := filepath.Join(root, "child.ps1")
	parentScript := filepath.Join(root, "parent.ps1")
	pidPath := filepath.Join(root, "tree.pid")
	if err := os.WriteFile(childScript, []byte("Start-Sleep -Seconds 60\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	quote := func(value string) string { return strings.ReplaceAll(value, "'", "''") }
	parent := fmt.Sprintf("$child = Start-Process -FilePath '%s' -ArgumentList '-NoProfile','-File','%s' -PassThru\n\"$PID $($child.Id)\" | Set-Content -NoNewline -Path '%s'\nWait-Process -Id $child.Id\n",
		quote(powershell), quote(childScript), quote(pidPath))
	if err := os.WriteFile(parentScript, []byte(parent), 0o600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	started := time.Now()
	runResult := make(chan error, 1)
	go func() {
		_, runErr := (OS{}).Run(ctx, ports.Command{Argv: []string{powershell, "-NoProfile", "-File", parentScript}, Env: os.Environ()})
		runResult <- runErr
	}()
	var body []byte
	readyDeadline := time.Now().Add(10 * time.Second)
	for len(strings.Fields(string(body))) != 2 && time.Now().Before(readyDeadline) {
		body, _ = os.ReadFile(pidPath)
		time.Sleep(20 * time.Millisecond)
	}
	cancel()
	var runErr error
	select {
	case runErr = <-runResult:
	case <-time.After(3 * time.Second):
		t.Fatal("process tree termination exceeded bound")
	}
	if !errors.Is(runErr, context.Canceled) {
		t.Fatalf("run error = %v, want context canceled", runErr)
	}
	if elapsed := time.Since(started); elapsed > 13*time.Second {
		t.Fatalf("process tree test took %s", elapsed)
	}
	if len(strings.Fields(string(body))) != 2 {
		t.Fatalf("process tree pids = %q", body)
	}
	for _, field := range strings.Fields(string(body)) {
		pid, err := strconv.Atoi(field)
		if err != nil {
			t.Fatalf("parse process pid %q: %v", field, err)
		}
		if windowsProcessAlive(uint32(pid)) {
			t.Fatalf("process %d remained alive after job termination", pid)
		}
	}
}

func windowsProcessAlive(pid uint32) bool {
	const stillActive = 259
	process, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION|windows.PROCESS_TERMINATE, false, pid)
	if err != nil {
		return false
	}
	defer windows.CloseHandle(process)
	var exitCode uint32
	if err := windows.GetExitCodeProcess(process, &exitCode); err != nil {
		return false
	}
	if exitCode == stillActive {
		_ = windows.TerminateProcess(process, 1)
		return true
	}
	return false
}
