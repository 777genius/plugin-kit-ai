package pluginkitairepo_test

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestGeminiCLIExtensionLink(t *testing.T) {
	geminiBin := geminiBinaryOrSkip(t)
	root := RepoRoot(t)
	pluginKitAIBin := buildPluginKitAI(t)

	workRoot := t.TempDir()
	extensionDir := filepath.Join(workRoot, "gemini-extension-package")
	copyTree(t, filepath.Join(root, "examples", "plugins", "gemini-extension-package"), extensionDir)

	runCmd(t, root, exec.Command(pluginKitAIBin, "render", extensionDir, "--check"))
	runCmd(t, root, exec.Command(pluginKitAIBin, "validate", extensionDir, "--platform", "gemini", "--strict"))

	homeDir := filepath.Join(t.TempDir(), "home")
	if err := os.MkdirAll(homeDir, 0o755); err != nil {
		t.Fatal(err)
	}
	seedGeminiHome(t, homeDir)
	output := runGeminiLink(t, geminiBin, homeDir, extensionDir)
	if !strings.Contains(output, `Extension "gemini-extension-package" linked successfully and enabled.`) {
		t.Fatalf("gemini link output missing success marker:\n%s", output)
	}
	installMetadataPath := filepath.Join(homeDir, ".gemini", "extensions", "gemini-extension-package", ".gemini-extension-install.json")
	installMetadataBody, err := os.ReadFile(installMetadataPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(installMetadataBody), extensionDir) || !strings.Contains(string(installMetadataBody), `"type": "link"`) {
		t.Fatalf("unexpected install metadata:\n%s", installMetadataBody)
	}
	envPath := filepath.Join(homeDir, ".gemini", "extensions", "gemini-extension-package", ".env")
	assertFileContains(t, envPath, "RELEASE_PROFILE=stable")

	configOutput := runGeminiConfig(t, geminiBin, homeDir, extensionDir, "gemini-extension-package", "release-profile", "canary\n")
	if !strings.Contains(configOutput, `Setting "release-profile" updated.`) {
		t.Fatalf("gemini config output missing success marker:\n%s", configOutput)
	}
	assertFileContains(t, envPath, "RELEASE_PROFILE=canary")

	disableOutput := runGeminiCommand(t, geminiBin, homeDir, extensionDir, "extensions", "disable", "gemini-extension-package", "--scope", "user")
	if !strings.Contains(disableOutput, `Extension "gemini-extension-package" successfully disabled for scope "user".`) {
		t.Fatalf("gemini disable output missing success marker:\n%s", disableOutput)
	}
	assertEnablementRule(t, filepath.Join(homeDir, ".gemini", "extensions", "extension-enablement.json"), "gemini-extension-package", "!"+homeDir+"/*")

	enableOutput := runGeminiCommand(t, geminiBin, homeDir, extensionDir, "extensions", "enable", "gemini-extension-package", "--scope", "user")
	if !strings.Contains(enableOutput, `Extension "gemini-extension-package" successfully enabled for scope "user".`) {
		t.Fatalf("gemini enable output missing success marker:\n%s", enableOutput)
	}
	assertEnablementRule(t, filepath.Join(homeDir, ".gemini", "extensions", "extension-enablement.json"), "gemini-extension-package", homeDir+"/*")

	registryBody, err := os.ReadFile(filepath.Join(homeDir, ".gemini", "projects.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(registryBody), "hookplex") && (strings.TrimSpace(string(registryBody)) == "{}" || strings.TrimSpace(string(registryBody)) == "") {
		t.Fatalf("gemini project registry was not updated:\n%s", registryBody)
	}
}

func geminiBinaryOrSkip(t *testing.T) string {
	t.Helper()
	if value := strings.TrimSpace(os.Getenv("PLUGIN_KIT_AI_GEMINI_BIN")); value != "" {
		return value
	}
	if value := strings.TrimSpace(os.Getenv("GEMINI_BIN")); value != "" {
		return value
	}
	geminiBin, err := exec.LookPath("gemini")
	if err != nil {
		t.Skip("set PLUGIN_KIT_AI_GEMINI_BIN or GEMINI_BIN, or install gemini in PATH, to run local Gemini CLI extension e2e")
	}
	return geminiBin
}

func runGeminiLink(t *testing.T, geminiBin, homeDir, extensionDir string) string {
	return runGeminiCommandWithInput(t, geminiBin, homeDir, extensionDir, "stable\n", "extensions", "link", extensionDir, "--consent")
}

func runGeminiConfig(t *testing.T, geminiBin, homeDir, extensionDir, name, setting, input string) string {
	return runGeminiCommandWithInput(t, geminiBin, homeDir, extensionDir, input, "extensions", "config", name, setting, "--scope", "user")
}

func runGeminiCommand(t *testing.T, geminiBin, homeDir, extensionDir string, args ...string) string {
	return runGeminiCommandWithInput(t, geminiBin, homeDir, extensionDir, "", args...)
}

func runGeminiCommandWithInput(t *testing.T, geminiBin, homeDir, extensionDir, input string, args ...string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, geminiBin, args...)
	cmd.Dir = extensionDir
	cmd.Env = geminiCLIEnv(homeDir)
	cmd.Stdin = strings.NewReader(input)
	out, err := cmd.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		t.Fatalf("gemini command %q timed out:\n%s", strings.Join(args, " "), out)
	}
	if err != nil {
		if bytes.Contains(out, []byte("Please set an Auth method")) {
			t.Skipf("gemini auth is not usable for isolated live e2e:\n%s", out)
		}
		t.Fatalf("gemini command %q: %v\n%s", strings.Join(args, " "), err, out)
	}
	text := string(out)
	t.Logf("gemini %s output: %s", strings.Join(args, " "), truncateRunes(text, 4000))
	return text
}

func geminiCLIEnv(homeDir string) []string {
	env := os.Environ()
	out := make([]string, 0, len(env)+5)
	for _, item := range env {
		switch {
		case strings.HasPrefix(item, "HOME="),
			strings.HasPrefix(item, "USERPROFILE="),
			strings.HasPrefix(item, "XDG_CONFIG_HOME="),
			strings.HasPrefix(item, "XDG_DATA_HOME="),
			strings.HasPrefix(item, "XDG_STATE_HOME="):
			continue
		default:
			out = append(out, item)
		}
	}
	out = append(out,
		"HOME="+homeDir,
		"USERPROFILE="+homeDir,
		"GEMINI_CLI_HOME="+homeDir,
		"XDG_CONFIG_HOME="+filepath.Join(homeDir, ".config"),
		"XDG_DATA_HOME="+filepath.Join(homeDir, ".local", "share"),
		"XDG_STATE_HOME="+filepath.Join(homeDir, ".local", "state"),
	)
	return out
}

func assertSameFile(t *testing.T, wantPath, gotPath string) {
	t.Helper()
	want, err := os.ReadFile(wantPath)
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(gotPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(want, got) {
		t.Fatalf("file mismatch for %s", gotPath)
	}
}

func seedGeminiHome(t *testing.T, homeDir string) {
	t.Helper()
	geminiDir := filepath.Join(homeDir, ".gemini")
	if err := os.MkdirAll(geminiDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, rel := range []string{
		"projects.json",
		"settings.json",
		"oauth_creds.json",
		"google_accounts.json",
		"installation_id",
		"state.json",
	} {
		src := filepath.Join(os.Getenv("HOME"), ".gemini", rel)
		body, err := os.ReadFile(src)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			t.Fatal(err)
		}
		dst := filepath.Join(geminiDir, rel)
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(dst, body, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if path := filepath.Join(geminiDir, "projects.json"); !fileExists(path) {
		if err := os.WriteFile(path, []byte("{}\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

func assertFileContains(t *testing.T, path, want string) {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), want) {
		t.Fatalf("file %s missing %q:\n%s", path, want, body)
	}
}

func assertEnablementRule(t *testing.T, path, extensionName, want string) {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var config map[string]struct {
		Overrides []string `json:"overrides"`
	}
	if err := json.Unmarshal(body, &config); err != nil {
		t.Fatalf("parse extension enablement: %v\n%s", err, body)
	}
	entry, ok := config[extensionName]
	if !ok {
		t.Fatalf("enablement config missing %q:\n%s", extensionName, body)
	}
	for _, override := range entry.Overrides {
		if override == want {
			return
		}
	}
	t.Fatalf("enablement overrides for %q = %#v, want %q", extensionName, entry.Overrides, want)
}
