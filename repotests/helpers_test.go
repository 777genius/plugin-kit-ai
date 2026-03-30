package pluginkitairepo_test

import (
	"bufio"
	"encoding/json"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

const rootGoModModuleLine = "module github.com/777genius/plugin-kit-ai"

// RepoRoot returns the plugin-kit-ai monorepo root (directory containing the anchor go.mod).
// Walks up from the caller's file until it finds go.mod with module github.com/777genius/plugin-kit-ai.
// Override with PLUGIN_KIT_AI_REPO_ROOT for debugging.
func RepoRoot(tb testing.TB) string {
	tb.Helper()
	if v := strings.TrimSpace(os.Getenv("PLUGIN_KIT_AI_REPO_ROOT")); v != "" {
		return v
	}
	_, file, _, ok := runtime.Caller(1)
	if !ok {
		tb.Fatal("runtime.Caller")
	}
	dir := filepath.Dir(file)
	for {
		modPath := filepath.Join(dir, "go.mod")
		if fileExists(modPath) && isAnchorGoMod(modPath) {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			tb.Fatalf("plugin-kit-ai repo root not found from %s (expected %s in a parent go.mod)", file, rootGoModModuleLine)
		}
		dir = parent
	}
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

func isAnchorGoMod(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	s := bufio.NewScanner(f)
	if !s.Scan() {
		return false
	}
	return strings.TrimSpace(s.Text()) == rootGoModModuleLine
}

func buildPluginKitAI(t *testing.T) string {
	t.Helper()
	root := RepoRoot(t)
	cliDir := filepath.Join(root, "cli", "plugin-kit-ai")
	binDir := t.TempDir()
	name := "plugin-kit-ai"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	pluginKitAIBin := filepath.Join(binDir, name)
	build := exec.Command("go", "build", "-o", pluginKitAIBin, "./cmd/plugin-kit-ai")
	build.Dir = cliDir
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build plugin-kit-ai: %v\n%s", err, out)
	}
	return pluginKitAIBin
}

func buildPluginKitAIWithVersion(t *testing.T, cliVersion string) string {
	t.Helper()
	root := RepoRoot(t)
	cliDir := filepath.Join(root, "cli", "plugin-kit-ai")
	binDir := t.TempDir()
	name := "plugin-kit-ai"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	pluginKitAIBin := filepath.Join(binDir, name)
	build := exec.Command("go", "build", "-ldflags", "-X main.version="+cliVersion, "-o", pluginKitAIBin, "./cmd/plugin-kit-ai")
	build.Dir = cliDir
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build plugin-kit-ai with version: %v\n%s", err, out)
	}
	return pluginKitAIBin
}

func newGoModuleEnv(t testing.TB) []string {
	t.Helper()
	cacheRoot := filepath.Join(t.TempDir(), "go-env")
	return append(os.Environ(),
		"GOWORK=off",
		"GOCACHE="+filepath.Join(cacheRoot, "gocache"),
		"GOMODCACHE="+filepath.Join(cacheRoot, "gomodcache"),
		"GOPATH="+filepath.Join(cacheRoot, "gopath"),
	)
}

func copyTree(t *testing.T, src, dst string) {
	t.Helper()
	if err := filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, info.Mode())
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		return os.WriteFile(target, body, info.Mode())
	}); err != nil {
		t.Fatal(err)
	}
}

func runInstall(t *testing.T, pluginKitAIBin, workDir, apiBase string, extraArgs ...string) (exitCode int, output []byte) {
	t.Helper()
	args := append([]string{"install", "o/r", "--github-api-base", apiBase}, extraArgs...)
	cmd := exec.Command(pluginKitAIBin, args...)
	if workDir != "" {
		cmd.Dir = workDir
	}
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
	out, err := cmd.CombinedOutput()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			return ee.ExitCode(), out
		}
		t.Fatalf("install: %v\n%s", err, out)
	}
	return 0, out
}

func runPluginKitAIInstall(t *testing.T, pluginKitAIBin, workDir, ownerRepo string, extraArgs ...string) (exitCode int, output []byte) {
	t.Helper()
	args := append([]string{"install", ownerRepo}, extraArgs...)
	cmd := exec.Command(pluginKitAIBin, args...)
	if workDir != "" {
		cmd.Dir = workDir
	}
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
	out, err := cmd.CombinedOutput()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			return ee.ExitCode(), out
		}
		t.Fatalf("plugin-kit-ai install: %v\n%s", err, out)
	}
	return 0, out
}

func bootstrapGeneratedGoPlugin(t *testing.T, root string) {
	t.Helper()
	env := newGoModuleEnv(t)
	wireGeneratedGoModuleToLocalSDK(t, root, env)
	tidyCmd := exec.Command("go", "mod", "tidy")
	tidyCmd.Dir = root
	tidyCmd.Env = env
	if out, err := tidyCmd.CombinedOutput(); err != nil {
		t.Fatalf("go mod tidy in generated plugin: %v\n%s", err, out)
	}
}

func wireGeneratedGoModuleToLocalSDK(t testing.TB, root string, env []string) {
	t.Helper()
	repoRoot := RepoRoot(t)
	cmd := exec.Command("go", "mod", "edit", "-replace", "github.com/777genius/plugin-kit-ai/sdk="+filepath.Join(repoRoot, "sdk"))
	cmd.Dir = root
	cmd.Env = env
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go mod edit replace sdk: %v\n%s", err, out)
	}
}

// buildPluginKitAIE2E builds sdk/cmd/plugin-kit-ai-e2e into a temp dir and returns the binary path.
func buildPluginKitAIE2E(t *testing.T) string {
	t.Helper()
	root := RepoRoot(t)
	sdkDir := filepath.Join(root, "sdk")
	binDir := t.TempDir()
	name := "plugin-kit-ai-e2e"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	out := filepath.Join(binDir, name)
	cmd := exec.Command("go", "build", "-o", out, "./cmd/plugin-kit-ai-e2e")
	cmd.Dir = sdkDir
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build plugin-kit-ai-e2e: %v\n%s", err, b)
	}
	return out
}

func requireBindTests(t *testing.T) {
	t.Helper()
	if os.Getenv("PLUGIN_KIT_AI_BIND_TESTS") == "1" {
		return
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("requires loopback bind support or PLUGIN_KIT_AI_BIND_TESTS=1: %v", err)
	}
	_ = ln.Close()
}

type traceRec struct {
	Hook    string `json:"hook"`
	Outcome string `json:"outcome"`
	Client  string `json:"client,omitempty"`
	RawJSON string `json:"raw_json,omitempty"`
}

type repoVersionContractValue struct {
	GoSDKVersion          string
	RuntimePackageVersion string
}

func repoVersionContract(tb testing.TB) repoVersionContractValue {
	tb.Helper()
	root := RepoRoot(tb)
	f, err := os.Open(filepath.Join(root, "scripts", "version-contract.env"))
	if err != nil {
		tb.Fatal(err)
	}
	defer f.Close()

	var out repoVersionContractValue
	s := bufio.NewScanner(f)
	for s.Scan() {
		line := strings.TrimSpace(s.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		switch parts[0] {
		case "GO_SDK_VERSION":
			out.GoSDKVersion = strings.TrimSpace(parts[1])
		case "RUNTIME_PACKAGE_VERSION":
			out.RuntimePackageVersion = strings.TrimSpace(parts[1])
		}
	}
	if err := s.Err(); err != nil {
		tb.Fatal(err)
	}
	if out.GoSDKVersion == "" || out.RuntimePackageVersion == "" {
		tb.Fatalf("incomplete version contract: %+v", out)
	}
	return out
}

func repoGoSDKVersion(tb testing.TB) string {
	tb.Helper()
	return repoVersionContract(tb).GoSDKVersion
}

func repoRuntimePackageVersion(tb testing.TB) string {
	tb.Helper()
	return repoVersionContract(tb).RuntimePackageVersion
}

func readTraceLines(t *testing.T, path string) []string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		t.Fatal(err)
	}
	var lines []string
	s := bufio.NewScanner(strings.NewReader(string(b)))
	for s.Scan() {
		if strings.TrimSpace(s.Text()) != "" {
			lines = append(lines, s.Text())
		}
	}
	return lines
}

func waitForTraceLines(t *testing.T, path string, timeout time.Duration) []string {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		lines := readTraceLines(t, path)
		if len(lines) > 0 || time.Now().After(deadline) {
			return lines
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func traceHas(t *testing.T, lines []string, wantHook, wantOutcome string) bool {
	t.Helper()
	for _, line := range lines {
		var rec traceRec
		if json.Unmarshal([]byte(line), &rec) != nil {
			continue
		}
		if rec.Hook == wantHook && rec.Outcome == wantOutcome {
			return true
		}
	}
	return false
}

func traceFind(t *testing.T, lines []string, wantHook string) (traceRec, bool) {
	t.Helper()
	for _, line := range lines {
		var rec traceRec
		if json.Unmarshal([]byte(line), &rec) != nil {
			continue
		}
		if rec.Hook == wantHook {
			return rec, true
		}
	}
	return traceRec{}, false
}

func assertCodexConfig(t *testing.T, root, wantModel, wantEntrypoint string) {
	t.Helper()
	body, err := os.ReadFile(filepath.Join(root, ".codex", "config.toml"))
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(string(body), "\n")
	var got []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		got = append(got, line)
	}
	want := []string{
		`model = "` + wantModel + `"`,
		`notify = ["` + wantEntrypoint + `", "notify"]`,
	}
	if len(got) < len(want) {
		t.Fatalf(".codex/config.toml lines = %v, want prefix %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf(".codex/config.toml lines = %v, want prefix %v", got, want)
		}
	}
}
