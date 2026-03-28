package pluginkitairepo_test

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestPluginKitAIBundleFetchURLPythonRequirementsFlow(t *testing.T) {
	if !pythonRuntimeAvailable() {
		t.Skip("python runtime not available for bundle fetch flow")
	}

	pluginKitAIBin := buildPluginKitAI(t)
	plugRoot := runtimeProjectRoot(t)
	run := exec.Command(pluginKitAIBin, "init", "genplug", "--platform", "codex-runtime", "--runtime", "python", "-o", plugRoot, "--extras")
	if out, err := run.CombinedOutput(); err != nil {
		t.Fatalf("plugin-kit-ai init: %v\n%s", err, out)
	}
	writeRuntimeFile(t, plugRoot, "requirements.txt", "requests==2.32.0\n")

	bootstrap := exec.Command(pluginKitAIBin, "bootstrap", plugRoot)
	if out, err := bootstrap.CombinedOutput(); err != nil {
		t.Fatalf("plugin-kit-ai bootstrap before export: %v\n%s", err, out)
	}
	export := exec.Command(pluginKitAIBin, "export", plugRoot, "--platform", "codex-runtime")
	if out, err := export.CombinedOutput(); err != nil {
		t.Fatalf("plugin-kit-ai export before fetch: %v\n%s", err, out)
	}

	bundlePath := filepath.Join(plugRoot, "genplug_codex-runtime_python_bundle.tar.gz")
	bundleBody, err := os.ReadFile(bundlePath)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(bundleBody)
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/bundle.tar.gz":
			_, _ = w.Write(bundleBody)
		case "/bundle.tar.gz.sha256":
			_, _ = w.Write([]byte(hex.EncodeToString(sum[:]) + "\n"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	dest := filepath.Join(t.TempDir(), "installed")
	fetch := exec.Command(pluginKitAIBin, "bundle", "fetch", "--url", server.URL+"/bundle.tar.gz", "--dest", dest, "--insecure-skip-tls-verify")
	out, err := fetch.CombinedOutput()
	if err != nil {
		t.Fatalf("plugin-kit-ai bundle fetch python requirements: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "Checksum source: "+server.URL+"/bundle.tar.gz.sha256") {
		t.Fatalf("fetch output missing checksum source:\n%s", out)
	}

	doctor := exec.Command(pluginKitAIBin, "doctor", dest)
	out, err = doctor.CombinedOutput()
	if err == nil {
		t.Fatalf("expected doctor to require bootstrap after bundle fetch:\n%s", out)
	}
	if !strings.Contains(string(out), "Status: needs_bootstrap") {
		t.Fatalf("doctor output missing needs_bootstrap:\n%s", out)
	}

	bootstrap = exec.Command(pluginKitAIBin, "bootstrap", dest)
	if out, err := bootstrap.CombinedOutput(); err != nil {
		t.Fatalf("plugin-kit-ai bootstrap after bundle fetch: %v\n%s", err, out)
	}
	validate := exec.Command(pluginKitAIBin, "validate", dest, "--platform", "codex-runtime", "--strict")
	if out, err := validate.CombinedOutput(); err != nil {
		t.Fatalf("plugin-kit-ai validate after bundle fetch: %v\n%s", err, out)
	}
}

func TestPluginKitAIBundleFetchURLClaudeNodeTypeScriptFlow(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node not in PATH")
	}
	if _, err := exec.LookPath("npm"); err != nil {
		t.Skip("npm not in PATH")
	}

	pluginKitAIBin := buildPluginKitAI(t)
	plugRoot := runtimeProjectRoot(t)
	run := exec.Command(pluginKitAIBin, "init", "genplug", "--platform", "claude", "--runtime", "node", "--typescript", "-o", plugRoot, "--extras")
	if out, err := run.CombinedOutput(); err != nil {
		t.Fatalf("plugin-kit-ai init claude node typescript: %v\n%s", err, out)
	}

	bootstrap := exec.Command(pluginKitAIBin, "bootstrap", plugRoot)
	if out, err := bootstrap.CombinedOutput(); err != nil {
		t.Fatalf("plugin-kit-ai bootstrap before claude export: %v\n%s", err, out)
	}
	export := exec.Command(pluginKitAIBin, "export", plugRoot, "--platform", "claude")
	if out, err := export.CombinedOutput(); err != nil {
		t.Fatalf("plugin-kit-ai export before fetch: %v\n%s", err, out)
	}

	bundlePath := filepath.Join(plugRoot, "genplug_claude_node_bundle.tar.gz")
	bundleBody, err := os.ReadFile(bundlePath)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(bundleBody)
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/bundle.tar.gz":
			_, _ = w.Write(bundleBody)
		case "/bundle.tar.gz.sha256":
			_, _ = w.Write([]byte(hex.EncodeToString(sum[:]) + "  bundle.tar.gz\n"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	dest := filepath.Join(t.TempDir(), "installed")
	fetch := exec.Command(pluginKitAIBin, "bundle", "fetch", "--url", server.URL+"/bundle.tar.gz", "--dest", dest, "--insecure-skip-tls-verify")
	out, err := fetch.CombinedOutput()
	if err != nil {
		t.Fatalf("plugin-kit-ai bundle fetch claude node typescript: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "platform=claude runtime=node") {
		t.Fatalf("fetch output missing claude runtime summary:\n%s", out)
	}

	doctor := exec.Command(pluginKitAIBin, "doctor", dest)
	out, err = doctor.CombinedOutput()
	if err == nil {
		t.Fatalf("expected doctor to require bootstrap after bundle fetch:\n%s", out)
	}
	if !strings.Contains(string(out), "Status: needs_bootstrap") {
		t.Fatalf("doctor output missing needs_bootstrap:\n%s", out)
	}

	bootstrap = exec.Command(pluginKitAIBin, "bootstrap", dest)
	if out, err := bootstrap.CombinedOutput(); err != nil {
		t.Fatalf("plugin-kit-ai bootstrap after bundle fetch: %v\n%s", err, out)
	}
	validate := exec.Command(pluginKitAIBin, "validate", dest, "--platform", "claude", "--strict")
	if out, err := validate.CombinedOutput(); err != nil {
		t.Fatalf("plugin-kit-ai validate after bundle fetch: %v\n%s", err, out)
	}
}
