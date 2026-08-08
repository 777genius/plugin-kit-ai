package clientdetect

import (
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

	"github.com/777genius/plugin-kit-ai/install/integrationctl/agentplugins/domain"
)

type Detector struct {
	HomeDir               string
	GOOS                  string
	Environment           map[string]string
	SystemApplicationsDir string
	LookPath              func(string) (string, error)
	Lstat                 func(string) (fs.FileInfo, error)
}

func NewOS(homeDir string) Detector {
	return Detector{
		HomeDir:               homeDir,
		GOOS:                  runtime.GOOS,
		Environment:           environmentSnapshot(),
		SystemApplicationsDir: "/Applications",
		LookPath:              exec.LookPath,
		Lstat:                 os.Lstat,
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
		detector.detectCodex(),
		detector.detectCursor(),
		detector.detectCopilot(),
		detector.detectVSCode(),
		detector.detectKiro(),
	}
	sort.Slice(clients, func(i, j int) bool { return clients[i].ClientID < clients[j].ClientID })
	return clients, nil
}

func (detector Detector) detectCodex() domain.DetectedClient {
	configRoot := filepath.Join(detector.HomeDir, ".codex")
	surfaces := []domain.ClientSurface{
		detector.binarySurface("codex_cli", "codex"),
		detector.directorySurface("codex_config", configRoot),
	}
	if detector.GOOS == "darwin" {
		surfaces = append(surfaces,
			detector.appSurface("codex_desktop", "Codex.app"),
			detector.appSurface("chatgpt_desktop", "ChatGPT.app"),
		)
	}
	return detected(domain.ClientCodex, "OpenAI Codex / ChatGPT", configRoot, detector.lookup("codex"), surfaces)
}

func (detector Detector) detectCursor() domain.DetectedClient {
	configRoot := filepath.Join(detector.HomeDir, ".cursor")
	surfaces := []domain.ClientSurface{
		detector.binarySurface("cursor_cli", "cursor"),
		detector.directorySurface("cursor_config", configRoot),
	}
	if detector.GOOS == "darwin" {
		surfaces = append(surfaces, detector.appSurface("cursor_desktop", "Cursor.app"))
	}
	return detected(domain.ClientCursor, "Cursor", configRoot, detector.lookup("cursor"), surfaces)
}

func (detector Detector) detectCopilot() domain.DetectedClient {
	configRoot := filepath.Join(detector.HomeDir, ".copilot")
	surfaces := []domain.ClientSurface{
		detector.binarySurface("copilot_cli", "copilot"),
		detector.directorySurface("copilot_config", configRoot),
	}
	return detected(domain.ClientCopilot, "GitHub Copilot CLI", configRoot, detector.lookup("copilot"), surfaces)
}

func (detector Detector) detectVSCode() domain.DetectedClient {
	configRoot := detector.vscodeConfigRoot()
	surfaces := []domain.ClientSurface{
		detector.binarySurface("vscode_cli", "code"),
		detector.directorySurface("vscode_config", configRoot),
	}
	if detector.GOOS == "darwin" {
		surfaces = append(surfaces, detector.appSurface("vscode_desktop", "Visual Studio Code.app"))
	}
	return detected(domain.ClientVSCode, "Visual Studio Code", configRoot, detector.lookup("code"), surfaces)
}

func (detector Detector) detectKiro() domain.DetectedClient {
	configRoot := filepath.Join(detector.HomeDir, ".kiro")
	surfaces := []domain.ClientSurface{
		detector.binarySurface("kiro_cli", "kiro"),
		detector.directorySurface("kiro_config", configRoot),
	}
	if detector.GOOS == "darwin" {
		surfaces = append(surfaces, detector.appSurface("kiro_desktop", "Kiro.app"))
	}
	return detected(domain.ClientKiro, "Kiro", configRoot, detector.lookup("kiro"), surfaces)
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

func detected(clientID domain.ClientID, displayName, configRoot, executablePath string, surfaces []domain.ClientSurface) domain.DetectedClient {
	status := domain.DetectionNotDetected
	for _, surface := range surfaces {
		if surface.Detected {
			status = domain.DetectionDetected
			break
		}
	}
	return domain.DetectedClient{
		ClientID:       clientID,
		DisplayName:    displayName,
		Status:         status,
		Surfaces:       surfaces,
		ExecutablePath: executablePath,
		ConfigRoot:     configRoot,
	}
}

func evidence(ok bool, value string) string {
	if ok {
		return value
	}
	return ""
}

func environmentSnapshot() map[string]string {
	values := map[string]string{}
	for _, name := range []string{"APPDATA", "LOCALAPPDATA", "XDG_CONFIG_HOME"} {
		if value, ok := os.LookupEnv(name); ok {
			values[name] = value
		}
	}
	return values
}

func IsNotFound(err error) bool {
	return errors.Is(err, exec.ErrNotFound)
}
