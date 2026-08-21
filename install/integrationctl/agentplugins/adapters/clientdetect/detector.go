package clientdetect

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/777genius/plugin-kit-ai/install/integrationctl/agentplugins/domain"
)

type Detector struct {
	HomeDir               string
	GOOS                  string
	Environment           map[string]string
	SystemApplicationsDir string
	WindowsProgramFiles   []string
	LinuxApplicationDirs  []string
	LookPath              func(string) (string, error)
	Lstat                 func(string) (fs.FileInfo, error)
	ProbeVersion          func(context.Context, string) (string, error)
	VersionTimeout        time.Duration
}

func NewOS(homeDir string) Detector {
	environment := environmentSnapshot()
	xdgApplications := ""
	if root := strings.TrimSpace(environment["XDG_DATA_HOME"]); root != "" {
		xdgApplications = filepath.Join(root, "applications")
	}
	userApplications := ""
	if root := strings.TrimSpace(homeDir); root != "" {
		userApplications = filepath.Join(root, ".local", "share", "applications")
	}
	return Detector{
		HomeDir:               homeDir,
		GOOS:                  runtime.GOOS,
		Environment:           environment,
		SystemApplicationsDir: "/Applications",
		WindowsProgramFiles:   compactPaths(environment["ProgramFiles"], environment["ProgramW6432"], environment["ProgramFiles(x86)"]),
		LinuxApplicationDirs: compactPaths(
			xdgApplications,
			userApplications,
			"/usr/local/share/applications", "/usr/share/applications",
		),
		LookPath:       exec.LookPath,
		Lstat:          os.Lstat,
		ProbeVersion:   probeExecutableVersion,
		VersionTimeout: 2 * time.Second,
	}
}

func (detector Detector) Detect(ctx context.Context) ([]domain.DetectedClient, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if strings.TrimSpace(detector.HomeDir) == "" {
		return nil, fmt.Errorf("client detector home directory is required")
	}
	if detector.LookPath == nil || detector.Lstat == nil {
		return nil, fmt.Errorf("client detector probes are required")
	}
	clients := []domain.DetectedClient{
		detector.detectCodex(ctx),
		detector.detectChatGPT(ctx),
		detector.detectCursor(ctx),
		detector.detectCopilot(ctx),
		detector.detectVSCode(ctx),
		detector.detectKiro(ctx),
	}
	sort.Slice(clients, func(i, j int) bool { return clients[i].ClientID < clients[j].ClientID })
	return clients, nil
}

func (detector Detector) detectCodex(ctx context.Context) domain.DetectedClient {
	configRoot := filepath.Join(detector.HomeDir, ".codex")
	surfaces := []domain.ClientSurface{
		detector.binarySurface("codex_cli", "codex"),
		detector.directorySurface("codex_config", configRoot),
	}
	if detector.GOOS == "darwin" {
		surfaces = append(surfaces, detector.appSurface("codex_desktop", "Codex.app"))
	} else if detector.GOOS == "linux" {
		surfaces = append(surfaces, detector.linuxDesktopSurface("codex_desktop", "codex.desktop"))
	}
	return detector.detectedClient(ctx, domain.ClientCodex, "OpenAI Codex", configRoot, detector.lookup("codex"), surfaces)
}

func (detector Detector) detectChatGPT(ctx context.Context) domain.DetectedClient {
	var surfaces []domain.ClientSurface
	switch detector.GOOS {
	case "darwin":
		surfaces = append(surfaces, detector.appSurface("chatgpt_desktop", "ChatGPT.app"))
	case "windows":
		surfaces = append(surfaces, detector.windowsAppSurface(
			"chatgpt_desktop",
			filepath.Join("Microsoft", "WindowsApps", "ChatGPT.exe"),
			filepath.Join("WindowsApps", "ChatGPT.exe"),
		))
	case "linux":
		surfaces = append(surfaces, detector.linuxDesktopSurface("chatgpt_desktop", "chatgpt.desktop"))
	}
	// ChatGPT is a remote/manual host. It intentionally has no executable and
	// does not inherit the Codex CLI or config directory.
	return detector.detectedClient(ctx, domain.ClientChatGPT, "ChatGPT", "", "", surfaces)
}

func (detector Detector) detectCursor(ctx context.Context) domain.DetectedClient {
	configRoot := filepath.Join(detector.HomeDir, ".cursor")
	surfaces := []domain.ClientSurface{
		detector.binarySurface("cursor_cli", "cursor"),
		detector.directorySurface("cursor_config", configRoot),
	}
	if detector.GOOS == "darwin" {
		surfaces = append(surfaces, detector.appSurface("cursor_desktop", "Cursor.app"))
	} else if detector.GOOS == "windows" {
		surfaces = append(surfaces, detector.windowsAppSurface("cursor_desktop", filepath.Join("Programs", "cursor", "Cursor.exe"), filepath.Join("Cursor", "Cursor.exe")))
	} else if detector.GOOS == "linux" {
		surfaces = append(surfaces, detector.linuxDesktopSurface("cursor_desktop", "cursor.desktop"))
	}
	return detector.detectedClient(ctx, domain.ClientCursor, "Cursor", configRoot, detector.lookup("cursor"), surfaces)
}

func (detector Detector) detectCopilot(ctx context.Context) domain.DetectedClient {
	configRoot := filepath.Join(detector.HomeDir, ".copilot")
	surfaces := []domain.ClientSurface{
		detector.binarySurface("copilot_cli", "copilot"),
		detector.directorySurface("copilot_config", configRoot),
	}
	return detector.detectedClient(ctx, domain.ClientCopilot, "GitHub Copilot CLI", configRoot, detector.lookup("copilot"), surfaces)
}

func (detector Detector) detectVSCode(ctx context.Context) domain.DetectedClient {
	configRoot := detector.vscodeConfigRoot()
	surfaces := []domain.ClientSurface{
		detector.binarySurface("vscode_cli", "code"),
		detector.directorySurface("vscode_config", configRoot),
	}
	if detector.GOOS == "darwin" {
		surfaces = append(surfaces, detector.appSurface("vscode_desktop", "Visual Studio Code.app"))
	} else if detector.GOOS == "windows" {
		surfaces = append(surfaces, detector.windowsAppSurface("vscode_desktop", filepath.Join("Programs", "Microsoft VS Code", "Code.exe"), filepath.Join("Microsoft VS Code", "Code.exe")))
	} else if detector.GOOS == "linux" {
		surfaces = append(surfaces, detector.linuxDesktopSurface("vscode_desktop", "code.desktop", "visual-studio-code.desktop"))
	}
	return detector.detectedClient(ctx, domain.ClientVSCode, "Visual Studio Code", configRoot, detector.lookup("code"), surfaces)
}

func (detector Detector) detectKiro(ctx context.Context) domain.DetectedClient {
	configRoot := filepath.Join(detector.HomeDir, ".kiro")
	kiroCLI := detector.lookup("kiro-cli")
	legacyCLI := detector.lookup("kiro")
	surfaces := []domain.ClientSurface{
		{ID: "kiro_cli", Detected: kiroCLI != "", Evidence: evidence(kiroCLI != "", "executable_on_path")},
		{ID: "kiro_legacy_cli", Detected: legacyCLI != "", Evidence: evidence(legacyCLI != "", "legacy_executable_on_path")},
		detector.directorySurface("kiro_config", configRoot),
	}
	if detector.GOOS == "darwin" {
		surfaces = append(surfaces, detector.appSurface("kiro_desktop", "Kiro.app"))
	} else if detector.GOOS == "windows" {
		surfaces = append(surfaces, detector.windowsAppSurface("kiro_desktop", filepath.Join("Programs", "Kiro", "Kiro.exe"), filepath.Join("Kiro", "Kiro.exe")))
	} else if detector.GOOS == "linux" {
		surfaces = append(surfaces, detector.linuxDesktopSurface("kiro_desktop", "kiro.desktop"))
	}
	return detector.detectedClient(ctx, domain.ClientKiro, "Kiro", configRoot, firstPath(kiroCLI, legacyCLI), surfaces)
}

func (detector Detector) vscodeConfigRoot() string {
	switch detector.GOOS {
	case "darwin":
		return filepath.Join(detector.HomeDir, "Library", "Application Support", "Code", "User")
	case "windows":
		if root := strings.TrimSpace(detector.Environment["APPDATA"]); root != "" {
			return filepath.Join(root, "Code", "User")
		}
	default:
		if root := strings.TrimSpace(detector.Environment["XDG_CONFIG_HOME"]); root != "" {
			return filepath.Join(root, "Code", "User")
		}
		return filepath.Join(detector.HomeDir, ".config", "Code", "User")
	}
	return filepath.Join(detector.HomeDir, ".config", "Code", "User")
}

func (detector Detector) binarySurface(id, binary string) domain.ClientSurface {
	path := detector.lookup(binary)
	return domain.ClientSurface{ID: id, Detected: path != "", Evidence: evidence(path != "", "executable_on_path")}
}

func (detector Detector) directorySurface(id, path string) domain.ClientSurface {
	detected := detector.realDirectory(path)
	return domain.ClientSurface{ID: id, Detected: detected, Evidence: evidence(detected, "configuration_directory")}
}

func (detector Detector) appSurface(id, appName string) domain.ClientSurface {
	paths := []string{filepath.Join(detector.HomeDir, "Applications", appName)}
	if strings.TrimSpace(detector.SystemApplicationsDir) != "" {
		paths = append(paths, filepath.Join(detector.SystemApplicationsDir, appName))
	}
	for _, path := range paths {
		if detector.realDirectory(path) {
			return domain.ClientSurface{ID: id, Detected: true, Evidence: "application_bundle"}
		}
	}
	return domain.ClientSurface{ID: id}
}

func (detector Detector) windowsAppSurface(id, userRelativePath, systemRelativePath string) domain.ClientSurface {
	paths := []string{}
	if root := strings.TrimSpace(detector.Environment["LOCALAPPDATA"]); root != "" {
		paths = append(paths, filepath.Join(root, userRelativePath))
	}
	for _, root := range detector.WindowsProgramFiles {
		if strings.TrimSpace(root) != "" {
			paths = append(paths, filepath.Join(root, systemRelativePath))
		}
	}
	for _, path := range paths {
		if detector.realFile(path) {
			return domain.ClientSurface{ID: id, Detected: true, Evidence: "application_installation"}
		}
	}
	return domain.ClientSurface{ID: id}
}

func (detector Detector) linuxDesktopSurface(id string, filenames ...string) domain.ClientSurface {
	for _, root := range detector.LinuxApplicationDirs {
		for _, filename := range filenames {
			if detector.realFile(filepath.Join(root, filename)) {
				return domain.ClientSurface{ID: id, Detected: true, Evidence: "desktop_entry"}
			}
		}
	}
	return domain.ClientSurface{ID: id}
}

func (detector Detector) lookup(binary string) string {
	path, err := detector.LookPath(binary)
	if err != nil || strings.TrimSpace(path) == "" {
		return ""
	}
	return path
}

func (detector Detector) realDirectory(path string) bool {
	if strings.TrimSpace(path) == "" {
		return false
	}
	info, err := detector.Lstat(path)
	return err == nil && info.IsDir() && info.Mode()&os.ModeSymlink == 0
}

func (detector Detector) realFile(path string) bool {
	if strings.TrimSpace(path) == "" {
		return false
	}
	info, err := detector.Lstat(path)
	return err == nil && info.Mode().IsRegular() && info.Mode()&os.ModeSymlink == 0
}

func firstPath(paths ...string) string {
	for _, path := range paths {
		if strings.TrimSpace(path) != "" {
			return path
		}
	}
	return ""
}

func (detector Detector) detectedClient(ctx context.Context, clientID domain.ClientID, displayName, configRoot, executablePath string, surfaces []domain.ClientSurface) domain.DetectedClient {
	status := domain.DetectionNotDetected
	for _, surface := range surfaces {
		if surface.Detected {
			status = domain.DetectionDetected
			break
		}
	}
	client := domain.DetectedClient{
		ClientID:       clientID,
		DisplayName:    displayName,
		Status:         status,
		Surfaces:       surfaces,
		ExecutablePath: executablePath,
		ConfigRoot:     configRoot,
	}
	if status == domain.DetectionDetected && executablePath != "" && detector.ProbeVersion != nil {
		timeout := detector.VersionTimeout
		if timeout <= 0 {
			timeout = 2 * time.Second
		}
		ctx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		if version, err := detector.ProbeVersion(ctx, executablePath); err == nil {
			client.Version = normalizeVersion(version)
		}
	}
	return client
}

const maximumVersionOutput = 4096

type cappedBuffer struct {
	bytes.Buffer
	remaining int
}

func (buffer *cappedBuffer) Write(value []byte) (int, error) {
	if len(value) > buffer.remaining {
		return 0, fmt.Errorf("version output exceeds %d bytes", maximumVersionOutput)
	}
	buffer.remaining -= len(value)
	return buffer.Buffer.Write(value)
}

func probeExecutableVersion(ctx context.Context, executable string) (string, error) {
	output := &cappedBuffer{remaining: maximumVersionOutput}
	command := exec.CommandContext(ctx, executable, "--version")
	command.Stdout, command.Stderr = output, output
	if err := command.Run(); err != nil {
		return "", err
	}
	return output.String(), nil
}

func normalizeVersion(value string) string {
	for _, field := range strings.Fields(value) {
		candidate := strings.Trim(field, "vV,;()[]{}")
		parts := strings.SplitN(strings.SplitN(candidate, "+", 2)[0], "-", 2)
		core := strings.Split(parts[0], ".")
		if len(core) < 2 {
			continue
		}
		valid := true
		for _, segment := range core {
			if segment == "" || strings.Trim(segment, "0123456789") != "" {
				valid = false
				break
			}
		}
		if valid {
			return candidate
		}
	}
	return ""
}

func evidence(ok bool, value string) string {
	if ok {
		return value
	}
	return ""
}

func environmentSnapshot() map[string]string {
	values := map[string]string{}
	for _, name := range []string{"APPDATA", "LOCALAPPDATA", "ProgramFiles", "ProgramW6432", "ProgramFiles(x86)", "XDG_CONFIG_HOME", "XDG_DATA_HOME"} {
		if value, ok := os.LookupEnv(name); ok {
			values[name] = value
		}
	}
	return values
}

func compactPaths(paths ...string) []string {
	result := make([]string, 0, len(paths))
	seen := map[string]struct{}{}
	for _, path := range paths {
		path = strings.TrimSpace(path)
		if path == "" {
			continue
		}
		clean := filepath.Clean(path)
		if _, ok := seen[clean]; ok {
			continue
		}
		seen[clean] = struct{}{}
		result = append(result, clean)
	}
	return result
}

func IsNotFound(err error) bool {
	return errors.Is(err, exec.ErrNotFound)
}
