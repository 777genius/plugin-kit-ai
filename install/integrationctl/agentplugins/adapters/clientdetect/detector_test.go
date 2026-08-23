package clientdetect

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/777genius/plugin-kit-ai/install/integrationctl/agentplugins/domain"
)

func TestMain(m *testing.M) {
	if len(os.Args) == 2 && os.Args[1] == "--version" {
		name := strings.TrimSuffix(filepath.Base(os.Args[0]), filepath.Ext(os.Args[0]))
		if strings.Contains(name, "slow-version-probe") {
			time.Sleep(5 * time.Second)
			os.Exit(0)
		}
		stdin, _ := io.ReadAll(os.Stdin)
		cwd, _ := os.Getwd()
		_ = json.NewEncoder(os.Stdout).Encode(struct {
			CWD   string   `json:"cwd"`
			Env   []string `json:"env"`
			Stdin string   `json:"stdin"`
		}{CWD: cwd, Env: os.Environ(), Stdin: string(stdin)})
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func TestDetectorReturnsAllSupportedClientsWithoutAmbientDiscovery(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	detector := testDetector(home, nil)
	clients, err := detector.Detect(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(clients) != 6 {
		t.Fatalf("clients = %d, want 6", len(clients))
	}
	for _, client := range clients {
		if client.Status != domain.DetectionNotDetected {
			t.Fatalf("client %s unexpectedly detected", client.ClientID)
		}
	}
}

func TestChatGPTDesktopNeverDetectsCodexOrInheritsItsBinary(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	applications := filepath.Join(home, "Applications")
	if err := os.MkdirAll(filepath.Join(applications, "ChatGPT.app"), 0o755); err != nil {
		t.Fatal(err)
	}
	detector := testDetector(home, map[string]string{"codex": filepath.Join(home, "bin", "codex")})
	detector.GOOS = "darwin"
	detector.SystemApplicationsDir = applications
	clients, err := detector.Detect(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	chatgpt := clientOf(clients, domain.ClientChatGPT)
	if chatgpt.Status != domain.DetectionDetected || chatgpt.ExecutablePath != "" || !surfaceDetected(chatgpt.Surfaces, "chatgpt_desktop") {
		t.Fatalf("ChatGPT detection = %+v", chatgpt)
	}
	codex := clientOf(clients, domain.ClientCodex)
	if !surfaceDetected(codex.Surfaces, "codex_cli") || surfaceDetected(codex.Surfaces, "chatgpt_desktop") {
		t.Fatalf("Codex detection leaked ChatGPT surface: %+v", codex)
	}
}

func TestCodexDesktopNeverDetectsChatGPT(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	applications := filepath.Join(home, "Applications")
	if err := os.MkdirAll(filepath.Join(applications, "Codex.app"), 0o755); err != nil {
		t.Fatal(err)
	}
	detector := testDetector(home, nil)
	detector.GOOS = "darwin"
	detector.SystemApplicationsDir = applications
	clients, err := detector.Detect(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if statusOf(clients, domain.ClientCodex) != domain.DetectionDetected || statusOf(clients, domain.ClientChatGPT) != domain.DetectionNotDetected {
		t.Fatalf("split detection = %+v", clients)
	}
}

func TestNewOSDoesNotProbeRelativeLocalApplicationsWithoutHome(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", "")
	detector := NewOS("")
	for _, directory := range detector.LinuxApplicationDirs {
		if directory == path.Join(".local", "share", "applications") || !path.IsAbs(directory) {
			t.Fatalf("NewOS configured unsafe relative application probe %q", directory)
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

func TestDetectorReadOnlyDetectionNeverProbesClientVersion(t *testing.T) {
	home := t.TempDir()
	detector := testDetector(home, map[string]string{"cursor": filepath.Join(home, "bin", "cursor")})
	detector.ProbeVersion = func(context.Context, string) (string, error) {
		t.Fatal("read-only Detect executed a client version probe")
		return "", nil
	}
	clients, err := detector.Detect(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if client := clientOf(clients, domain.ClientCursor); client.Status != domain.DetectionDetected || client.Version != "" {
		t.Fatalf("read-only Cursor detection = %+v", client)
	}
}

func TestExplicitDetectorProbeNormalizesClientVersionWithoutMakingDetectionFatal(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	cursorPath := filepath.Join(home, "bin", "cursor")
	detector := testDetector(home, map[string]string{"cursor": cursorPath, "code": filepath.Join(home, "bin", "code")})
	detector.ProbeVersion = func(_ context.Context, executable string) (string, error) {
		if executable == cursorPath {
			return "Cursor 0.50.7\nsynthetic-build", nil
		}
		return "", context.DeadlineExceeded
	}
	clients, err := detector.DetectWithVersionProbe(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if version := clientOf(clients, domain.ClientCursor).Version; version != "0.50.7" {
		t.Fatalf("Cursor version = %q, want 0.50.7", version)
	}
	if version := clientOf(clients, domain.ClientVSCode).Version; version != "" {
		t.Fatalf("unavailable VS Code version = %q, want empty", version)
	}
}

func TestExplicitDetectorProbeNormalizesOfficialCopilotVersionOutput(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	copilotPath := filepath.Join(home, "bin", "copilot")
	detector := testDetector(home, map[string]string{"copilot": copilotPath})
	detector.ProbeVersion = func(_ context.Context, executable string) (string, error) {
		if executable != copilotPath {
			t.Fatalf("version probe executable = %q", executable)
		}
		return "GitHub Copilot CLI 1.0.80.\nA newer version is available.", nil
	}
	clients, err := detector.DetectWithVersionProbe(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if version := clientOf(clients, domain.ClientCopilot).Version; version != "1.0.80" {
		t.Fatalf("Copilot version = %q, want 1.0.80", version)
	}
}

func TestNormalizeVersionRejectsMalformedTrailingPunctuation(t *testing.T) {
	t.Parallel()
	tests := []struct {
		value string
		want  string
	}{
		{value: "GitHub Copilot CLI 1.0.80.", want: "1.0.80"},
		{value: "GitHub Copilot CLI v1.0.80.", want: "1.0.80"},
		{value: "GitHub Copilot CLI 1.0.80.."},
		{value: "GitHub Copilot CLI .1.0.80."},
		{value: "GitHub Copilot CLI 1..80."},
		{value: "GitHub Copilot CLI 1.0.x."},
		{value: "GitHub Copilot CLI 1.0.80beta."},
	}
	for _, test := range tests {
		if got := normalizeVersion(test.value); got != test.want {
			t.Errorf("normalizeVersion(%q) = %q, want %q", test.value, got, test.want)
		}
	}
}

func TestTargetedVersionProbeExecutesOnlySelectedClient(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	copilotPath := filepath.Join(home, "bin", "copilot")
	codePath := filepath.Join(home, "bin", "code")
	detector := testDetector(home, map[string]string{"copilot": copilotPath, "code": codePath})
	var probed []string
	detector.ProbeVersion = func(_ context.Context, executable string) (string, error) {
		probed = append(probed, executable)
		return "1.0.80.", nil
	}
	clients, err := detector.DetectTargetsWithVersionProbe(context.Background(), []domain.ClientID{domain.ClientCopilot})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(probed, []string{copilotPath}) {
		t.Fatalf("probed executables = %#v", probed)
	}
	if version := clientOf(clients, domain.ClientCopilot).Version; version != "1.0.80" {
		t.Fatalf("Copilot version = %q", version)
	}
	if version := clientOf(clients, domain.ClientVSCode).Version; version != "" {
		t.Fatalf("VS Code version was unexpectedly probed: %q", version)
	}
}

func TestTargetedVersionProbeAllowsSlowAuthoritativeClientWithinExplicitBound(t *testing.T) {
	home := t.TempDir()
	copilotPath := filepath.Join(home, "bin", "copilot")
	detector := testDetector(home, map[string]string{"copilot": copilotPath})
	detector.VersionTimeout = 2 * time.Second
	detector.TargetedVersionTimeout = 3 * time.Second
	detector.ProbeVersion = func(ctx context.Context, executable string) (string, error) {
		if executable != copilotPath {
			t.Fatalf("version probe executable = %q", executable)
		}
		select {
		case <-time.After(2100 * time.Millisecond):
			return "GitHub Copilot CLI 1.0.80.", nil
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}
	clients, err := detector.DetectTargetsWithVersionProbe(context.Background(), []domain.ClientID{domain.ClientCopilot})
	if err != nil {
		t.Fatal(err)
	}
	if version := clientOf(clients, domain.ClientCopilot).Version; version != "1.0.80" {
		t.Fatalf("Copilot version = %q, want 1.0.80", version)
	}
}

func TestTargetedVersionProbeStillBoundsAuthoritativeClientTimeout(t *testing.T) {
	home := t.TempDir()
	detector := testDetector(home, map[string]string{"copilot": filepath.Join(home, "bin", "copilot")})
	detector.TargetedVersionTimeout = 10 * time.Millisecond
	detector.ProbeVersion = func(ctx context.Context, _ string) (string, error) {
		<-ctx.Done()
		return "", ctx.Err()
	}
	started := time.Now()
	clients, err := detector.DetectTargetsWithVersionProbe(context.Background(), []domain.ClientID{domain.ClientCopilot})
	if err != nil {
		t.Fatal(err)
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("bounded targeted version probe took %s", elapsed)
	}
	if version := clientOf(clients, domain.ClientCopilot).Version; version != "" {
		t.Fatalf("timed-out Copilot version = %q, want empty", version)
	}
}

func TestExplicitDetectorBoundsInjectedVersionProbe(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	detector := testDetector(home, map[string]string{"cursor": filepath.Join(home, "bin", "cursor")})
	detector.VersionTimeout = 10 * time.Millisecond
	detector.ProbeVersion = func(ctx context.Context, _ string) (string, error) {
		<-ctx.Done()
		return "", ctx.Err()
	}
	started := time.Now()
	clients, err := detector.DetectWithVersionProbe(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("bounded version probe took %s", elapsed)
	}
	if clientOf(clients, domain.ClientCursor).Version != "" {
		t.Fatal("timed-out version probe populated a version")
	}
}

func TestExecutableVersionProbeIsSanitizedAndIsolated(t *testing.T) {
	t.Setenv("AGENTPLUGINS_TEST_CREDENTIAL", "must-not-leak")
	executable := copyTestExecutable(t, "inspect-version-probe")
	userCWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	output, err := probeExecutableVersion(context.Background(), executable)
	if err != nil {
		t.Fatal(err)
	}
	var observed struct {
		CWD   string   `json:"cwd"`
		Env   []string `json:"env"`
		Stdin string   `json:"stdin"`
	}
	if err := json.Unmarshal([]byte(output), &observed); err != nil {
		t.Fatalf("decode probe observation %q: %v", output, err)
	}
	if observed.CWD == "" || filepath.Clean(observed.CWD) == filepath.Clean(userCWD) || !strings.Contains(filepath.Base(observed.CWD), "agentplugins-version-probe-") {
		t.Fatalf("probe cwd = %q, user cwd = %q", observed.CWD, userCWD)
	}
	for _, value := range observed.Env {
		name, _, _ := strings.Cut(value, "=")
		if !strings.EqualFold(name, "SYSTEMROOT") && !strings.EqualFold(name, "PATH") {
			t.Fatalf("probe inherited unexpected environment value %q", value)
		}
	}
	if observed.Stdin != "" {
		t.Fatalf("probe stdin = %q, want EOF", observed.Stdin)
	}
}

func TestExecutableVersionProbeSupportsEnvRuntimeShebangWithoutSecrets(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("env shebang behavior is Unix-specific")
	}
	bin := t.TempDir()
	node := filepath.Join(bin, "node")
	copilot := filepath.Join(bin, "copilot")
	nodeScript := `#!/bin/sh
if [ -n "${HOME+x}" ] || [ -n "${GITHUB_TOKEN+x}" ] || [ -n "${AGENTPLUGINS_TEST_CREDENTIAL+x}" ]; then
  exit 91
fi
printf 'GitHub Copilot CLI 1.0.80.\n'
`
	if err := os.WriteFile(node, []byte(nodeScript), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(copilot, []byte("#!/usr/bin/env node\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
	t.Setenv("HOME", "/must-not-leak")
	t.Setenv("GITHUB_TOKEN", "must-not-leak")
	t.Setenv("AGENTPLUGINS_TEST_CREDENTIAL", "must-not-leak")
	output, err := probeExecutableVersion(context.Background(), copilot)
	if err != nil {
		t.Fatal(err)
	}
	if version := normalizeVersion(output); version != "1.0.80" {
		t.Fatalf("shebang Copilot version = %q from %q", version, output)
	}
}

func TestExecutableVersionProbeHonorsContextBound(t *testing.T) {
	t.Parallel()
	executable := copyTestExecutable(t, "slow-version-probe")
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	started := time.Now()
	if _, err := probeExecutableVersion(ctx, executable); err == nil {
		t.Fatal("timed-out executable probe succeeded")
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("executable probe exceeded bound: %s", elapsed)
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

func copyTestExecutable(t *testing.T, prefix string) string {
	t.Helper()
	source, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	contents, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(t.TempDir(), prefix+filepath.Ext(source))
	if err := os.WriteFile(destination, contents, 0o755); err != nil {
		t.Fatal(err)
	}
	return destination
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
