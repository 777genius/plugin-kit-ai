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

func TestOpenCodeLoaderSmoke(t *testing.T) {
	if strings.TrimSpace(os.Getenv("PLUGIN_KIT_AI_ENABLE_OPENCODE_SMOKE")) != "1" {
		t.Skip("set PLUGIN_KIT_AI_ENABLE_OPENCODE_SMOKE=1 to run real OpenCode loader smoke")
	}
	opencodeBin, err := exec.LookPath("opencode")
	if err != nil {
		t.Skip("opencode not in PATH")
	}

	root := RepoRoot(t)
	pluginKitAIBin := buildPluginKitAI(t)
	workDir := filepath.Join(t.TempDir(), "opencode-basic")
	copyTree(t, filepath.Join(root, "examples", "plugins", "opencode-basic"), workDir)
	for _, rel := range []string{
		filepath.Join("targets", "opencode", "plugins", "custom-tool.js"),
		filepath.Join(".opencode", "plugins", "custom-tool.js"),
	} {
		if err := os.Remove(filepath.Join(workDir, rel)); err != nil && !os.IsNotExist(err) {
			t.Fatalf("remove %s from smoke workspace: %v", rel, err)
		}
	}

	renderCmd := exec.Command(pluginKitAIBin, "render", workDir)
	renderCmd.Dir = root
	renderCmd.Env = append(os.Environ(), "GOWORK=off")
	if out, err := renderCmd.CombinedOutput(); err != nil {
		t.Fatalf("render temp opencode workspace: %v\n%s", err, out)
	}

	markerPath := filepath.Join(t.TempDir(), "opencode-plugin-marker.json")
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, opencodeBin, "serve")
	cmd.Dir = workDir
	cmd.Env = append(os.Environ(),
		"PLUGIN_KIT_AI_OPENCODE_SMOKE_MARKER="+markerPath,
		"OPENCODE_SERVER_PASSWORD=plugin-kit-ai-smoke",
	)
	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output
	errCh := make(chan error, 1)
	if err := cmd.Start(); err != nil {
		t.Fatalf("start opencode serve: %v", err)
	}
	go func() {
		errCh <- cmd.Wait()
	}()

	deadline := time.Now().Add(8 * time.Second)
	for {
		if body, err := os.ReadFile(markerPath); err == nil {
			cancel()
			<-errCh
			if !strings.Contains(string(body), "directory") {
				t.Fatalf("unexpected OpenCode smoke marker:\n%s", body)
			}
			return
		}
		if time.Now().After(deadline) {
			cancel()
			err := <-errCh
			t.Fatalf("OpenCode loader smoke did not observe plugin marker before timeout; err=%v\n%s", err, output.Bytes())
		}
		time.Sleep(150 * time.Millisecond)
	}
}

func TestOpenCodeStandaloneToolsSmoke(t *testing.T) {
	if strings.TrimSpace(os.Getenv("PLUGIN_KIT_AI_ENABLE_OPENCODE_SMOKE")) != "1" {
		t.Skip("set PLUGIN_KIT_AI_ENABLE_OPENCODE_SMOKE=1 to run real OpenCode standalone-tools smoke")
	}
	opencodeBin, err := exec.LookPath("opencode")
	if err != nil {
		t.Skip("opencode not in PATH")
	}

	root := RepoRoot(t)
	pluginKitAIBin := buildPluginKitAI(t)
	workDir := filepath.Join(t.TempDir(), "opencode-basic")
	copyTree(t, filepath.Join(root, "examples", "plugins", "opencode-basic"), workDir)
	for _, rel := range []string{
		filepath.Join("targets", "opencode", "plugins", "example.js"),
		filepath.Join("targets", "opencode", "plugins", "custom-tool.js"),
		filepath.Join(".opencode", "plugins", "example.js"),
		filepath.Join(".opencode", "plugins", "custom-tool.js"),
	} {
		if err := os.Remove(filepath.Join(workDir, rel)); err != nil && !os.IsNotExist(err) {
			t.Fatalf("remove %s from standalone-tools smoke workspace: %v", rel, err)
		}
	}

	renderCmd := exec.Command(pluginKitAIBin, "render", workDir)
	renderCmd.Dir = root
	renderCmd.Env = append(os.Environ(), "GOWORK=off")
	if out, err := renderCmd.CombinedOutput(); err != nil {
		t.Fatalf("render temp opencode workspace for standalone-tools smoke: %v\n%s", err, out)
	}

	markerPath := filepath.Join(t.TempDir(), "opencode-standalone-tool-marker.json")
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, opencodeBin, "serve")
	cmd.Dir = workDir
	cmd.Env = append(os.Environ(),
		"PLUGIN_KIT_AI_OPENCODE_TOOL_SMOKE_MARKER="+markerPath,
		"OPENCODE_SERVER_PASSWORD=plugin-kit-ai-smoke",
	)
	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output
	errCh := make(chan error, 1)
	if err := cmd.Start(); err != nil {
		t.Fatalf("start opencode serve for standalone-tools smoke: %v", err)
	}
	go func() {
		errCh <- cmd.Wait()
	}()

	deadline := time.Now().Add(8 * time.Second)
	for {
		if body, err := os.ReadFile(markerPath); err == nil {
			cancel()
			<-errCh
			if !strings.Contains(string(body), "standalone-tool") || !strings.Contains(string(body), "echo.ts") {
				t.Fatalf("unexpected OpenCode standalone-tools smoke marker:\n%s", body)
			}
			return
		}
		if time.Now().After(deadline) {
			cancel()
			err := <-errCh
			t.Fatalf("OpenCode standalone-tools smoke did not observe tool marker before timeout; err=%v\n%s", err, output.Bytes())
		}
		time.Sleep(150 * time.Millisecond)
	}
}
