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

func TestDetectorRecognizesKiroCLIAndPreservesLegacyEvidence(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	modern := filepath.Join(home, "bin", "kiro-cli")
	legacy := filepath.Join(home, "bin", "kiro")
	detector := testDetector(home, map[string]string{"kiro-cli": modern, "kiro": legacy})
	clients, err := detector.Detect(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	kiro := clientOf(clients, domain.ClientKiro)
	if kiro.ExecutablePath != modern || !surfaceDetected(kiro.Surfaces, "kiro_cli") || !surfaceDetected(kiro.Surfaces, "kiro_legacy_cli") {
		t.Fatalf("Kiro detection = %+v", kiro)
	}
}

func TestDetectorFindsFixedWindowsDesktopInstallationWithoutPATH(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	local := filepath.Join(home, "AppData", "Local")
	executable := filepath.Join(local, "Programs", "cursor", "Cursor.exe")
	if err := os.MkdirAll(filepath.Dir(executable), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(executable, []byte("synthetic"), 0o755); err != nil {
		t.Fatal(err)
	}
	detector := testDetector(home, nil)
	detector.GOOS = "windows"
	detector.Environment = map[string]string{"LOCALAPPDATA": local}
	clients, err := detector.Detect(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	cursor := clientOf(clients, domain.ClientCursor)
	if cursor.Status != domain.DetectionDetected || !surfaceDetected(cursor.Surfaces, "cursor_desktop") {
		t.Fatalf("Cursor Windows detection = %+v", cursor)
	}
}

func TestDetectorFindsWindowsSystemInstallationWithoutUserInstall(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	programFiles := filepath.Join(home, "Program Files")
	executable := filepath.Join(programFiles, "Microsoft VS Code", "Code.exe")
	if err := os.MkdirAll(filepath.Dir(executable), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(executable, []byte("synthetic"), 0o755); err != nil {
		t.Fatal(err)
	}
	detector := testDetector(home, nil)
	detector.GOOS = "windows"
	detector.WindowsProgramFiles = []string{programFiles}
	clients, err := detector.Detect(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	vscode := clientOf(clients, domain.ClientVSCode)
	if vscode.Status != domain.DetectionDetected || !surfaceDetected(vscode.Surfaces, "vscode_desktop") {
		t.Fatalf("VS Code system detection = %+v", vscode)
	}
}

func TestDetectorFindsLinuxGUIOnlyFromExactDesktopEntry(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	applications := filepath.Join(home, "usr-share-applications")
	if err := os.MkdirAll(applications, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(applications, "kiro.desktop"), []byte("[Desktop Entry]"), 0o644); err != nil {
		t.Fatal(err)
	}
	detector := testDetector(home, nil)
	detector.LinuxApplicationDirs = []string{applications}
	clients, err := detector.Detect(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	kiro := clientOf(clients, domain.ClientKiro)
	if kiro.Status != domain.DetectionDetected || kiro.ExecutablePath != "" || !surfaceDetected(kiro.Surfaces, "kiro_desktop") {
		t.Fatalf("Kiro Linux GUI-only detection = %+v", kiro)
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

func clientOf(clients []domain.DetectedClient, id domain.ClientID) domain.DetectedClient {
	for _, client := range clients {
		if client.ClientID == id {
			return client
		}
	}
	return domain.DetectedClient{}
}

func surfaceDetected(surfaces []domain.ClientSurface, id string) bool {
	for _, surface := range surfaces {
		if surface.ID == id {
			return surface.Detected
		}
	}
	return false
}
