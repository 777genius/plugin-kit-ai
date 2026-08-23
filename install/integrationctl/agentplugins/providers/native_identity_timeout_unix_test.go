//go:build !windows

package providers

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	processadapter "github.com/777genius/plugin-kit-ai/install/integrationctl/adapters/process"
	"github.com/777genius/plugin-kit-ai/install/integrationctl/agentplugins/domain"
)

func TestNativeIdentityTimeoutReapsAuthoritativeDiscoveryChild(t *testing.T) {
	root := t.TempDir()
	pidPath := filepath.Join(root, "child.pid")
	executable := filepath.Join(root, "copilot")
	script := "#!/bin/sh\nprintf '%s' \"$$\" > \"$AGENTPLUGINS_TEST_PID\"\nexec sleep 60\n"
	if err := os.WriteFile(executable, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AGENTPLUGINS_TEST_PID", pidPath)
	plan := identityPlan(filepath.Join(root, "prepared"))
	plan.NativeRegistryExecutable = executable
	observation, err := (NativeIdentityObserver{
		Runner: processadapter.OS{}, DiscoveryTimeout: 500 * time.Millisecond,
	}).ObserveNativeIdentity(context.Background(), domain.DetectedClient{ClientID: domain.ClientCopilot}, plan, nil)
	if !errors.Is(err, context.DeadlineExceeded) || observation.State != domain.NativeIdentityIndeterminate {
		t.Fatalf("observation = %+v, err = %v", observation, err)
	}
	body, readErr := os.ReadFile(pidPath)
	if readErr != nil {
		t.Fatalf("read child pid: %v", readErr)
	}
	pid, parseErr := strconv.Atoi(strings.TrimSpace(string(body)))
	if parseErr != nil {
		t.Fatalf("parse child pid: %v", parseErr)
	}
	if signalErr := syscall.Kill(pid, 0); signalErr == nil {
		_ = syscall.Kill(pid, syscall.SIGKILL)
		t.Fatal("authoritative discovery child remained alive after timeout")
	} else if !errors.Is(signalErr, syscall.ESRCH) {
		t.Fatalf("inspect child process: %v", signalErr)
	}
}
