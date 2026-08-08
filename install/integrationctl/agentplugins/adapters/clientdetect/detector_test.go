package clientdetect

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/777genius/plugin-kit-ai/install/integrationctl/agentplugins/domain"
)

func TestDetectorReturnsAllSupportedClientsWithoutAmbientDiscovery(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	detector := testDetector(home, nil)
	clients, err := detector.Detect(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(clients) != 5 {
		t.Fatalf("clients = %d, want 5", len(clients))
	}
	for _, client := range clients {
		if client.Status != domain.DetectionNotDetected {
			t.Fatalf("client %s unexpectedly detected", client.ClientID)
		}
	}
}

func TestDetectorFindsOneAndMultipleClientsFromInjectedHomeAndPath(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".cursor"), 0o755); err != nil {
		t.Fatal(err)
	}
	detector := testDetector(home, map[string]string{"copilot": filepath.Join(home, "bin", "copilot")})
	clients, err := detector.Detect(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if statusOf(clients, domain.ClientCursor) != domain.DetectionDetected {
		t.Fatal("Cursor config was not detected")
	}
	if statusOf(clients, domain.ClientCopilot) != domain.DetectionDetected {
		t.Fatal("Copilot executable was not detected")
	}
	if statusOf(clients, domain.ClientCodex) != domain.DetectionNotDetected {
		t.Fatal("Codex was unexpectedly detected")
	}
}

func TestDetectorDoesNotTreatSymlinkedConfigAsInstalledClient(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	target := filepath.Join(t.TempDir(), "cursor")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(home, ".cursor")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	detector := testDetector(home, nil)
	clients, err := detector.Detect(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if statusOf(clients, domain.ClientCursor) != domain.DetectionNotDetected {
		t.Fatal("symlinked Cursor config was trusted")
	}
}

func TestDetectorUsesInjectedWindowsConfigRoot(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	appData := filepath.Join(home, "AppData", "Roaming")
	if err := os.MkdirAll(filepath.Join(appData, "Code", "User"), 0o755); err != nil {
		t.Fatal(err)
	}
	detector := testDetector(home, nil)
	detector.GOOS = "windows"
	detector.Environment = map[string]string{"APPDATA": appData}
	clients, err := detector.Detect(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if statusOf(clients, domain.ClientVSCode) != domain.DetectionDetected {
		t.Fatal("VS Code Windows config was not detected")
	}
}

func testDetector(home string, binaries map[string]string) Detector {
	applications := filepath.Join(home, "system-applications")
	return Detector{
		HomeDir:               home,
		GOOS:                  "linux",
		Environment:           map[string]string{},
		SystemApplicationsDir: applications,
		LookPath: func(name string) (string, error) {
			if path := binaries[name]; path != "" {
				return path, nil
			}
			return "", exec.ErrNotFound
		},
		Lstat: os.Lstat,
	}
}

func statusOf(clients []domain.DetectedClient, id domain.ClientID) domain.DetectionStatus {
	for _, client := range clients {
		if client.ClientID == id {
			return client.Status
		}
	}
	return ""
}
