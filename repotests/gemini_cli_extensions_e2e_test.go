package pluginkitairepo_test

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

const geminiRuntimeLiveEnvVar = "PLUGIN_KIT_AI_RUN_GEMINI_RUNTIME_LIVE"
const geminiExtensionLiveEnvVar = "PLUGIN_KIT_AI_RUN_GEMINI_CLI"

func TestGeminiCLIExtensionLink(t *testing.T) {
	if strings.TrimSpace(os.Getenv(geminiExtensionLiveEnvVar)) != "1" {
		t.Skipf("set %s=1 to run real Gemini extension lifecycle smoke", geminiExtensionLiveEnvVar)
	}
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
	seedGeminiHome(t, homeDir, extensionDir)
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

func TestGeminiCLIRuntimeSessionStart(t *testing.T) {
	if strings.TrimSpace(os.Getenv(geminiRuntimeLiveEnvVar)) != "1" {
		t.Skipf("set %s=1 to run real Gemini runtime hook smoke", geminiRuntimeLiveEnvVar)
	}
	geminiBin := geminiBinaryOrSkip(t)
	root := RepoRoot(t)
	pluginKitAIBin := buildPluginKitAI(t)
	hookBin := buildPluginKitAIE2E(t)
	env := newGoModuleEnv(t)

	workRoot := t.TempDir()
	extensionDir := filepath.Join(workRoot, "gemini-runtime-live")
	run := exec.Command(pluginKitAIBin, "init", "gemini-runtime-live", "--platform", "gemini", "--runtime", "go", "-o", extensionDir)
	run.Dir = root
	if out, err := run.CombinedOutput(); err != nil {
		t.Fatalf("plugin-kit-ai init: %v\n%s", err, out)
	}

	wireGeneratedGoModuleToLocalSDK(t, extensionDir, env)
	tidy := exec.Command("go", "mod", "tidy")
	tidy.Dir = extensionDir
	tidy.Env = env
	if out, err := tidy.CombinedOutput(); err != nil {
		t.Fatalf("go mod tidy: %v\n%s", err, out)
	}

	tracePath := filepath.Join(workRoot, "trace.ndjson")
	binName := "gemini-runtime-live"
	if runtime.GOOS == "windows" {
		binName += ".exe"
	}
	build := exec.Command("go", "build", "-o", filepath.Join("bin", binName), "./cmd/gemini-runtime-live")
	build.Dir = extensionDir
	build.Env = env
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build generated entrypoint: %v\n%s", err, out)
	}

	runCmd(t, root, exec.Command(pluginKitAIBin, "render", extensionDir))
	runCmd(t, root, exec.Command(pluginKitAIBin, "render", extensionDir, "--check"))
	runCmd(t, root, exec.Command(pluginKitAIBin, "validate", extensionDir, "--platform", "gemini", "--strict"))

	absHook, err := filepath.Abs(hookBin)
	if err != nil {
		t.Fatal(err)
	}
	hooksPath := filepath.Join(extensionDir, "hooks", "hooks.json")
	hooksBody, err := os.ReadFile(hooksPath)
	if err != nil {
		t.Fatal(err)
	}
	updatedHooks := strings.ReplaceAll(string(hooksBody), "${extensionPath}${/}bin${/}gemini-runtime-live", absHook)
	if err := os.WriteFile(hooksPath, []byte(updatedHooks), 0o644); err != nil {
		t.Fatal(err)
	}

	homeDir := filepath.Join(t.TempDir(), "home")
	if err := os.MkdirAll(homeDir, 0o755); err != nil {
		t.Fatal(err)
	}
	seedGeminiHome(t, homeDir, extensionDir)
	linkOutput := runGeminiLink(t, geminiBin, homeDir, extensionDir)
	if !strings.Contains(linkOutput, `linked successfully and enabled`) {
		t.Fatalf("gemini runtime live link did not report success:\n%s", linkOutput)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, geminiBin, "-p", "Reply with exactly OK.", "--output-format", "json")
	cmd.Dir = extensionDir
	cmd.Env = append(geminiCLIEnv(homeDir), "PLUGIN_KIT_AI_E2E_TRACE="+tracePath)
	out, err := cmd.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		t.Fatalf("gemini runtime smoke timed out; %s rerun make test-gemini-runtime-live.\ntrace=%s\noutput:\n%s", geminiAuthRecoveryHint(string(out)), tracePath, truncateRunes(string(out), 4000))
	}
	if err != nil {
		if geminiEnvironmentIssue(string(out)) {
			t.Skipf("gemini environment is not ready for isolated runtime live e2e; %s\n%s", geminiAuthRecoveryHint(string(out)), truncateRunes(string(out), 4000))
		}
		t.Fatalf("gemini runtime smoke: %v\ntrace=%s\nhint=confirm gemini extensions link . succeeded, then inspect hooks/hooks.json command wiring and rerun the live smoke.\noutput:\n%s", err, tracePath, truncateRunes(string(out), 4000))
	}

	lines := waitForTraceLines(t, tracePath, 3*time.Second)
	if !traceHas(t, lines, "SessionStart", "allow") {
		t.Fatalf("expected SessionStart allow in trace; hint=confirm the linked extension still points at the generated runtime repo, then inspect hooks/hooks.json and rerun gemini -p.\ntrace=%s\noutput:\n%s\ntrace_lines:\n%s", tracePath, truncateRunes(string(out), 4000), strings.Join(lines, "\n"))
	}
}

func geminiBinaryOrSkip(t *testing.T) string {
	t.Helper()
	if strings.TrimSpace(os.Getenv("PLUGIN_KIT_AI_SKIP_GEMINI_CLI")) == "1" {
		t.Skip("PLUGIN_KIT_AI_SKIP_GEMINI_CLI=1")
	}
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
		t.Fatalf("gemini command %q timed out; %s\n%s", strings.Join(args, " "), geminiAuthRecoveryHint(string(out)), truncateRunes(string(out), 4000))
	}
	if err != nil {
		if geminiEnvironmentIssue(string(out)) {
			t.Skipf("gemini environment is not ready for isolated live e2e; %s\n%s", geminiAuthRecoveryHint(string(out)), truncateRunes(string(out), 4000))
		}
		t.Fatalf("gemini command %q: %v\nhint=%s\n%s", strings.Join(args, " "), err, geminiCommandRecoveryHint(args), truncateRunes(string(out), 4000))
	}
	text := string(out)
	t.Logf("gemini %s output: %s", strings.Join(args, " "), truncateRunes(text, 4000))
	return text
}

func geminiEnvironmentIssue(output string) bool {
	lower := strings.ToLower(output)
	markers := []string{
		"please set an auth method",
		"not authenticated",
		"authentication required",
		"login required",
		"unauthorized",
		"forbidden",
		"failed to sign in",
		"current account is not eligible",
		"not currently available in your location",
		"please contact your administrator to request an entitlement",
		"unable_to_get_issuer_cert_locally",
		"unable to get local issuer certificate",
		"safe mode",
		"untrusted workspace",
		"extension management is restricted",
		"workspace settings are ignored",
		"mcp servers do not connect",
	}
	for _, marker := range markers {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

func geminiAuthRecoveryHint(output string) string {
	lower := strings.ToLower(output)
	switch {
	case strings.Contains(lower, "unable_to_get_issuer_cert_locally"),
		strings.Contains(lower, "unable to get local issuer certificate"):
		return "per Gemini CLI troubleshooting, corporate TLS interception may require NODE_USE_SYSTEM_CA=1 or NODE_EXTRA_CA_CERTS; then retry."
	case strings.Contains(lower, "safe mode"),
		strings.Contains(lower, "untrusted workspace"),
		strings.Contains(lower, "extension management is restricted"),
		strings.Contains(lower, "workspace settings are ignored"),
		strings.Contains(lower, "mcp servers do not connect"):
		return "per Gemini CLI trusted-folders docs, trust this workspace or parent folder in ~/.gemini/trustedFolders.json or via /permissions, then rerun the Gemini live smoke."
	case strings.Contains(lower, "google_cloud_project"),
		strings.Contains(lower, "google_cloud_project_id"),
		strings.Contains(lower, "gemini code assist"),
		strings.Contains(lower, "current account is not eligible"),
		strings.Contains(lower, "request contains an invalid argument"),
		strings.Contains(lower, "administrator to request an entitlement"):
		return "per Gemini CLI auth docs, headless mode needs cached auth or env-based auth, and Workspace/Code Assist accounts often also need GOOGLE_CLOUD_PROJECT."
	default:
		return "per Gemini CLI auth docs, headless mode needs cached auth or env-based auth (GEMINI_API_KEY or Vertex AI); verify auth and retry."
	}
}

func geminiCommandRecoveryHint(args []string) string {
	if len(args) >= 2 && args[0] == "extensions" {
		switch args[1] {
		case "validate":
			return "verify the workspace is trusted, rerender gemini-extension.json if needed, then rerun gemini extensions validate <path>."
		case "link":
			return "verify the extension repo renders cleanly; after a successful link, restart Gemini CLI before checking session/runtime behavior."
		case "config", "enable", "disable":
			return "verify the extension repo renders cleanly; after changing extension settings or enablement, restart Gemini CLI before checking the new behavior."
		case "list":
			return "after link or config changes, restart Gemini CLI before relying on extensions list or session-visible extension state."
		}
	}
	return "verify the extension repo renders cleanly, then rerun the Gemini extension command."
}

func TestGeminiEnvironmentIssue(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		output string
		want   bool
	}{
		{name: "auth method missing", output: "Please set an auth method before continuing.", want: true},
		{name: "workspace entitlement missing", output: "Please contact your administrator to request an entitlement.", want: true},
		{name: "tls interception", output: "UNABLE_TO_GET_ISSUER_CERT_LOCALLY", want: true},
		{name: "safe mode", output: "The CLI is running in safe mode because this is an untrusted workspace.", want: true},
		{name: "plain runtime failure", output: "hook command exited with status 1", want: false},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := geminiEnvironmentIssue(tc.output); got != tc.want {
				t.Fatalf("geminiEnvironmentIssue(%q) = %v, want %v", tc.output, got, tc.want)
			}
		})
	}
}

func TestGeminiAuthRecoveryHint(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name         string
		output       string
		wantContains string
	}{
		{name: "tls", output: "UNABLE_TO_GET_ISSUER_CERT_LOCALLY", wantContains: "NODE_USE_SYSTEM_CA=1"},
		{name: "trusted folder", output: "Extension management is restricted in safe mode for an untrusted workspace", wantContains: "trustedFolders.json"},
		{name: "workspace project", output: "Set GOOGLE_CLOUD_PROJECT before using Gemini Code Assist", wantContains: "GOOGLE_CLOUD_PROJECT"},
		{name: "default", output: "not authenticated", wantContains: "GEMINI_API_KEY"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := geminiAuthRecoveryHint(tc.output); !strings.Contains(got, tc.wantContains) {
				t.Fatalf("geminiAuthRecoveryHint(%q) = %q, want substring %q", tc.output, got, tc.wantContains)
			}
		})
	}
}

func TestGeminiCommandRecoveryHint(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name         string
		args         []string
		wantContains string
	}{
		{name: "validate", args: []string{"extensions", "validate", "/tmp/demo"}, wantContains: "trusted"},
		{name: "link", args: []string{"extensions", "link", "/tmp/demo"}, wantContains: "restart Gemini CLI"},
		{name: "config", args: []string{"extensions", "config", "demo", "release-profile"}, wantContains: "settings or enablement"},
		{name: "list", args: []string{"extensions", "list"}, wantContains: "extensions list"},
		{name: "default", args: []string{"extensions", "unknown"}, wantContains: "renders cleanly"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := geminiCommandRecoveryHint(tc.args); !strings.Contains(got, tc.wantContains) {
				t.Fatalf("geminiCommandRecoveryHint(%v) = %q, want substring %q", tc.args, got, tc.wantContains)
			}
		})
	}
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

func seedGeminiHome(t *testing.T, homeDir string, trustedDirs ...string) {
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
	if len(trustedDirs) > 0 {
		trustedFolders := map[string]string{}
		trustedFoldersPath := filepath.Join(geminiDir, "trustedFolders.json")
		if body, err := os.ReadFile(filepath.Join(os.Getenv("HOME"), ".gemini", "trustedFolders.json")); err == nil {
			if err := json.Unmarshal(body, &trustedFolders); err != nil {
				t.Fatalf("parse source trustedFolders.json: %v\n%s", err, body)
			}
		} else if !os.IsNotExist(err) {
			t.Fatal(err)
		}
		for _, dir := range trustedDirs {
			dir = strings.TrimSpace(dir)
			if dir == "" {
				continue
			}
			absDir, err := filepath.Abs(dir)
			if err != nil {
				t.Fatal(err)
			}
			trustedFolders[filepath.Clean(absDir)] = "TRUST_FOLDER"
		}
		body, err := json.MarshalIndent(trustedFolders, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		body = append(body, '\n')
		if err := os.WriteFile(trustedFoldersPath, body, 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

func TestSeedGeminiHomeAddsTrustedFolders(t *testing.T) {
	t.Parallel()
	sourceHome := t.TempDir()
	t.Setenv("HOME", sourceHome)
	if err := os.MkdirAll(filepath.Join(sourceHome, ".gemini"), 0o755); err != nil {
		t.Fatal(err)
	}
	destHome := t.TempDir()
	trustedDir := filepath.Join(t.TempDir(), "workspace")
	if err := os.MkdirAll(trustedDir, 0o755); err != nil {
		t.Fatal(err)
	}

	seedGeminiHome(t, destHome, trustedDir)

	body, err := os.ReadFile(filepath.Join(destHome, ".gemini", "trustedFolders.json"))
	if err != nil {
		t.Fatal(err)
	}
	var trusted map[string]string
	if err := json.Unmarshal(body, &trusted); err != nil {
		t.Fatalf("parse trustedFolders.json: %v\n%s", err, body)
	}
	absTrustedDir, err := filepath.Abs(trustedDir)
	if err != nil {
		t.Fatal(err)
	}
	if got := trusted[filepath.Clean(absTrustedDir)]; got != "TRUST_FOLDER" {
		t.Fatalf("trustedFolders[%q] = %q, want %q", filepath.Clean(absTrustedDir), got, "TRUST_FOLDER")
	}
}

func TestSeedGeminiHomeMergesSourceTrustedFolders(t *testing.T) {
	t.Parallel()
	sourceHome := t.TempDir()
	t.Setenv("HOME", sourceHome)
	sourceGeminiDir := filepath.Join(sourceHome, ".gemini")
	if err := os.MkdirAll(sourceGeminiDir, 0o755); err != nil {
		t.Fatal(err)
	}
	existingTrusted := map[string]string{
		"/source/already-trusted": "TRUST_PARENT",
	}
	body, err := json.MarshalIndent(existingTrusted, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	body = append(body, '\n')
	if err := os.WriteFile(filepath.Join(sourceGeminiDir, "trustedFolders.json"), body, 0o600); err != nil {
		t.Fatal(err)
	}

	destHome := t.TempDir()
	newTrustedDir := filepath.Join(t.TempDir(), "workspace")
	if err := os.MkdirAll(newTrustedDir, 0o755); err != nil {
		t.Fatal(err)
	}

	seedGeminiHome(t, destHome, newTrustedDir)

	mergedBody, err := os.ReadFile(filepath.Join(destHome, ".gemini", "trustedFolders.json"))
	if err != nil {
		t.Fatal(err)
	}
	var merged map[string]string
	if err := json.Unmarshal(mergedBody, &merged); err != nil {
		t.Fatalf("parse merged trustedFolders.json: %v\n%s", err, mergedBody)
	}
	if got := merged["/source/already-trusted"]; got != "TRUST_PARENT" {
		t.Fatalf("merged trustedFolders lost source entry: got %q", got)
	}
	absNewTrustedDir, err := filepath.Abs(newTrustedDir)
	if err != nil {
		t.Fatal(err)
	}
	if got := merged[filepath.Clean(absNewTrustedDir)]; got != "TRUST_FOLDER" {
		t.Fatalf("merged trustedFolders[%q] = %q, want %q", filepath.Clean(absNewTrustedDir), got, "TRUST_FOLDER")
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
