package pluginkitairepo_test

import (
	"bytes"
	"context"
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
	registryBody, err := os.ReadFile(filepath.Join(homeDir, ".gemini", "projects.json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(registryBody)) == "{}" || strings.TrimSpace(string(registryBody)) == "" {
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
	t.Skip("set PLUGIN_KIT_AI_GEMINI_BIN or GEMINI_BIN to run local Gemini CLI extension e2e")
	return ""
}

func runGeminiLink(t *testing.T, geminiBin, homeDir, extensionDir string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, geminiBin, "extensions", "link", extensionDir, "--consent")
	cmd.Dir = extensionDir
	cmd.Env = geminiCLIEnv(homeDir)
	cmd.Stdin = strings.NewReader("plugin-kit-ai-gemini-e2e\n")
	out, err := cmd.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		t.Fatalf("gemini extensions link timed out:\n%s", out)
	}
	if err != nil {
		if bytes.Contains(out, []byte("Please set an Auth method")) {
			t.Skipf("gemini auth is not usable for isolated live e2e:\n%s", out)
		}
		t.Fatalf("gemini extensions link: %v\n%s", err, out)
	}
	text := string(out)
	t.Logf("gemini extensions link output: %s", truncateRunes(text, 4000))
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
		"XDG_CONFIG_HOME="+filepath.Join(homeDir, ".config"),
		"XDG_DATA_HOME="+filepath.Join(homeDir, ".local", "share"),
		"XDG_STATE_HOME="+filepath.Join(homeDir, ".local", "state"),
		"RELEASE_API_TOKEN=plugin-kit-ai-gemini-e2e",
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
