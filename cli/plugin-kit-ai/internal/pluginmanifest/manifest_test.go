package pluginmanifest

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/777genius/plugin-kit-ai/cli/internal/platformexec"
)

func TestRender_RendersVersionIntoEveryNativeManifest(t *testing.T) {
	root := t.TempDir()
	manifest := Default("demo", "codex-runtime", "go", "demo plugin", true)
	manifest.Version = "1.2.3"
	manifest.Targets = []string{"claude", "codex-package", "codex-runtime", "gemini", "opencode"}
	mustSavePackage(t, root, manifest, "go")
	result, err := Render(root, "all")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Artifacts) != 6 {
		t.Fatalf("artifacts = %d, want 6", len(result.Artifacts))
	}
	if err := WriteArtifacts(root, result.Artifacts); err != nil {
		t.Fatal(err)
	}
	for _, rel := range []string{
		filepath.Join(".claude-plugin", "plugin.json"),
		filepath.Join("hooks", "hooks.json"),
		filepath.Join(".codex", "config.toml"),
		filepath.Join(".codex-plugin", "plugin.json"),
		"gemini-extension.json",
		"opencode.json",
	} {
		full := filepath.Join(root, rel)
		if _, err := os.Stat(full); err != nil {
			t.Fatalf("stat %s: %v", rel, err)
		}
		body, err := os.ReadFile(full)
		if err != nil {
			t.Fatal(err)
		}
		if rel == filepath.Join("hooks", "hooks.json") || rel == filepath.Join(".codex", "config.toml") || rel == "opencode.json" {
			continue
		}
		if !strings.Contains(string(body), `"version": "1.2.3"`) {
			t.Fatalf("%s missing rendered version:\n%s", rel, body)
		}
	}
}

func TestRender_NormalizesGeneratedArtifactPaths(t *testing.T) {
	root := t.TempDir()
	manifest := Default("demo", "codex-runtime", "python", "demo plugin", true)
	mustSavePackage(t, root, manifest, "python")

	result, err := Render(root, "all")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Artifacts) == 0 {
		t.Fatal("expected generated artifacts")
	}
	for _, artifact := range result.Artifacts {
		if strings.ContainsRune(artifact.RelPath, '\\') {
			t.Fatalf("artifact path %q must use slash separators", artifact.RelPath)
		}
	}
}

func TestRender_OpenCodeRendersWorkspaceConfigAndSkills(t *testing.T) {
	root := t.TempDir()
	manifest := Default("demo", "opencode", "", "demo plugin", false)
	mustSavePackage(t, root, manifest, "")
	mustWritePluginFile(t, root, filepath.Join("targets", "opencode", "package.yaml"), "plugins:\n  - \"@acme/demo-opencode\"\n")
	mustWritePluginFile(t, root, filepath.Join("targets", "opencode", "config.extra.json"), `{"theme":"midnight"}`)
	mustWritePortableMCPFile(t, root, `format: plugin-kit-ai/mcp
version: 1

servers:
  context7:
    type: stdio
    stdio:
      command: npx
      args:
        - -y
        - "@upstash/context7-mcp"
`)
	mustWritePluginFile(t, root, filepath.Join("skills", "demo", "SKILL.md"), "---\nname: demo\ndescription: demo skill\nexecution_mode: docs_only\nsupported_agents:\n  - opencode\n---\n\n# Demo\n")
	mustWritePluginFile(t, root, filepath.Join("targets", "opencode", "commands", "ship.md"), "---\ndescription: ship command\n---\n\nShip it.\n")
	mustWritePluginFile(t, root, filepath.Join("targets", "opencode", "agents", "reviewer.md"), "---\ndescription: reviewer\nmode: subagent\n---\n\nReview carefully.\n")
	mustWritePluginFile(t, root, filepath.Join("targets", "opencode", "themes", "midnight.json"), `{"$schema":"https://opencode.ai/theme.json","theme":{"primary":"#111827","text":"#f9fafb","background":"#020617"}}`)
	mustWritePluginFile(t, root, filepath.Join("targets", "opencode", "tools", "echo.ts"), "import { tool } from \"@opencode-ai/plugin\"\nexport default tool({ description: \"echo\", args: { value: tool.schema.string() }, async execute(args) { return args.value } })\n")
	mustWritePluginFile(t, root, filepath.Join("targets", "opencode", "plugins", "example.js"), "export const ExamplePlugin = async () => ({})\n")
	mustWritePluginFile(t, root, filepath.Join("targets", "opencode", "package.json"), `{"name":"demo-opencode-local","private":true,"type":"module","dependencies":{"@opencode-ai/plugin":"latest"}}`)

	result, err := Render(root, "opencode")
	if err != nil {
		t.Fatal(err)
	}
	if err := WriteArtifacts(root, result.Artifacts); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(filepath.Join(root, "opencode.json"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(body)
	for _, want := range []string{
		`"$schema": "https://opencode.ai/config.json"`,
		`"plugin": [`,
		`"@acme/demo-opencode"`,
		`"mcp": {`,
		`"theme": "midnight"`,
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("opencode.json missing %q:\n%s", want, text)
		}
	}
	if _, err := os.Stat(filepath.Join(root, ".opencode", "skills", "demo", "SKILL.md")); err != nil {
		t.Fatalf("stat mirrored opencode skill: %v", err)
	}
	for _, rel := range []string{
		filepath.Join(".opencode", "commands", "ship.md"),
		filepath.Join(".opencode", "agents", "reviewer.md"),
		filepath.Join(".opencode", "themes", "midnight.json"),
		filepath.Join(".opencode", "tools", "echo.ts"),
		filepath.Join(".opencode", "plugins", "example.js"),
		filepath.Join(".opencode", "package.json"),
	} {
		if _, err := os.Stat(filepath.Join(root, rel)); err != nil {
			t.Fatalf("stat %s: %v", rel, err)
		}
	}
}

func TestDrift_IgnoresCRLFDifferencesForTextArtifacts(t *testing.T) {
	root := t.TempDir()
	manifest := Default("demo", "codex-runtime", "go", "demo plugin", false)
	mustSavePackage(t, root, manifest, "go")

	result, err := Render(root, "all")
	if err != nil {
		t.Fatal(err)
	}
	if err := WriteArtifacts(root, result.Artifacts); err != nil {
		t.Fatal(err)
	}

	configPath := filepath.Join(root, ".codex", "config.toml")
	body, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	body = bytes.ReplaceAll(body, []byte("\n"), []byte("\r\n"))
	if err := os.WriteFile(configPath, body, 0o644); err != nil {
		t.Fatal(err)
	}

	drift, err := Drift(root, "all")
	if err != nil {
		t.Fatal(err)
	}
	if len(drift) != 0 {
		t.Fatalf("drift = %v, want none for newline-only changes", drift)
	}
}

func TestRender_OpenCodeRejectsManagedOverridesInConfigExtra(t *testing.T) {
	root := t.TempDir()
	manifest := Default("demo", "opencode", "", "demo plugin", false)
	mustSavePackage(t, root, manifest, "")
	mustWritePluginFile(t, root, filepath.Join("targets", "opencode", "config.extra.json"), `{"plugin":["override"]}`)
	if _, err := Render(root, "opencode"); err == nil || !strings.Contains(err.Error(), `opencode config.extra.json may not override canonical field "plugin"`) {
		t.Fatalf("Render error = %v", err)
	}
}

func TestRender_PortableMCPStandardProjectsAcrossExistingTargets(t *testing.T) {
	root := t.TempDir()
	manifest := Default("demo", "claude", "go", "demo plugin", true)
	manifest.Targets = []string{"claude", "codex-package", "gemini", "opencode", "cursor"}
	mustSavePackage(t, root, manifest, "go")
	mustWritePluginFile(t, root, filepath.Join("mcp", "servers.yaml"), `format: plugin-kit-ai/mcp
version: 1

servers:
  docs:
    description: Remote docs
    type: remote
    remote:
      protocol: streamable_http
      url: "https://example.com/mcp"
      headers:
        Authorization: "Bearer ${env.DOCS_TOKEN}"
    targets:
      - claude
      - codex-package
      - gemini
      - opencode
      - cursor
    overrides:
      gemini:
        excludeTools:
          - delete_docs

  release-checks:
    type: stdio
    stdio:
      command: node
      args:
        - "${package.root}/bin/release-checks.mjs"
      env:
        LOG_LEVEL: info
`)

	result, err := Render(root, "all")
	if err != nil {
		t.Fatal(err)
	}
	if err := WriteArtifacts(root, result.Artifacts); err != nil {
		t.Fatal(err)
	}

	for _, rel := range []string{
		filepath.Join(".claude-plugin", "plugin.json"),
		".mcp.json",
		filepath.Join(".codex-plugin", "plugin.json"),
		"gemini-extension.json",
		"opencode.json",
		filepath.Join(".cursor", "mcp.json"),
	} {
		if _, err := os.Stat(filepath.Join(root, rel)); err != nil {
			t.Fatalf("stat %s: %v", rel, err)
		}
	}

	sharedMCP, err := os.ReadFile(filepath.Join(root, ".mcp.json"))
	if err != nil {
		t.Fatal(err)
	}
	sharedText := string(sharedMCP)
	for _, want := range []string{`"release-checks"`, `"command": "node"`, `"docs"`, `"type": "http"`} {
		if !strings.Contains(sharedText, want) {
			t.Fatalf(".mcp.json missing %q:\n%s", want, sharedText)
		}
	}

	geminiBody, err := os.ReadFile(filepath.Join(root, "gemini-extension.json"))
	if err != nil {
		t.Fatal(err)
	}
	geminiText := string(geminiBody)
	for _, want := range []string{`"mcpServers"`, `"httpUrl": "https://example.com/mcp"`, `"excludeTools": [`, `"${extensionPath}/bin/release-checks.mjs"`} {
		if !strings.Contains(geminiText, want) {
			t.Fatalf("gemini-extension.json missing %q:\n%s", want, geminiText)
		}
	}

	opencodeBody, err := os.ReadFile(filepath.Join(root, "opencode.json"))
	if err != nil {
		t.Fatal(err)
	}
	opencodeText := string(opencodeBody)
	for _, want := range []string{`"mcp": {`, `"type": "local"`, `"command": [`, `"environment": {`, `"type": "remote"`} {
		if !strings.Contains(opencodeText, want) {
			t.Fatalf("opencode.json missing %q:\n%s", want, opencodeText)
		}
	}

	cursorBody, err := os.ReadFile(filepath.Join(root, ".cursor", "mcp.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(cursorBody), `"type": "http"`) {
		t.Fatalf(".cursor/mcp.json missing remote http projection:\n%s", cursorBody)
	}
}

func TestImport_OpenCodeNativeLayout(t *testing.T) {
	root := t.TempDir()
	mustWritePluginFile(t, root, "opencode.json", `{
  "$schema": "https://opencode.ai/config.json",
  "plugin": ["@acme/demo-opencode"],
  "mcp": {
    "context7": {
      "type": "local",
      "command": ["npx", "-y", "@upstash/context7-mcp"]
    }
  },
  "theme": "midnight"
}`)
	mustWritePluginFile(t, root, filepath.Join(".opencode", "skills", "demo", "SKILL.md"), "# Demo\n")
	mustWritePluginFile(t, root, filepath.Join(".opencode", "commands", "ship.md"), "---\ndescription: ship command\n---\n\nShip it.\n")
	mustWritePluginFile(t, root, filepath.Join(".opencode", "agents", "reviewer.md"), "---\ndescription: reviewer\nmode: subagent\n---\n\nReview carefully.\n")
	mustWritePluginFile(t, root, filepath.Join(".opencode", "themes", "midnight.json"), `{"$schema":"https://opencode.ai/theme.json","theme":{"primary":"#111827","text":"#f9fafb","background":"#020617"}}`)
	mustWritePluginFile(t, root, filepath.Join(".opencode", "tools", "echo.ts"), "import { tool } from \"@opencode-ai/plugin\"\nexport default tool({ description: \"echo\", args: { value: tool.schema.string() }, async execute(args) { return args.value } })\n")
	mustWritePluginFile(t, root, filepath.Join(".opencode", "plugins", "demo.ts"), "export const DemoPlugin = async () => ({})\n")
	mustWritePluginFile(t, root, filepath.Join(".opencode", "package.json"), `{"name":"demo-opencode","dependencies":{"@opencode-ai/plugin":"latest"}}`)

	imported, warnings, err := Import(root, "", false, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(imported.Targets) != 1 || imported.Targets[0] != "opencode" {
		t.Fatalf("targets = %v", imported.Targets)
	}
	for _, rel := range []string{
		filepath.Join("targets", "opencode", "package.yaml"),
		filepath.Join("targets", "opencode", "config.extra.json"),
		filepath.Join("targets", "opencode", "package.json"),
		filepath.Join("mcp", "servers.yaml"),
		filepath.Join("skills", "demo", "SKILL.md"),
		filepath.Join("targets", "opencode", "commands", "ship.md"),
		filepath.Join("targets", "opencode", "agents", "reviewer.md"),
		filepath.Join("targets", "opencode", "themes", "midnight.json"),
		filepath.Join("targets", "opencode", "tools", "echo.ts"),
		filepath.Join("targets", "opencode", "plugins", "demo.ts"),
	} {
		if _, err := os.Stat(filepath.Join(root, rel)); err != nil {
			t.Fatalf("stat %s: %v", rel, err)
		}
	}
	if len(warnings) != 0 {
		t.Fatalf("warnings = %v, want none", warnings)
	}
}

func TestImport_OpenCodeNormalizesInlineCommandsAndAgents(t *testing.T) {
	root := t.TempDir()
	mustWritePluginFile(t, root, "opencode.json", `{
  "$schema": "https://opencode.ai/config.json",
  "plugin": ["@acme/demo-opencode"],
  "command": {
    "ship": {
      "description": "ship command",
      "template": "Ship the release."
    },
    "kept": {
      "description": "kept command",
      "template": "Keep it.",
      "unknown": true
    }
  },
  "agent": {
    "reviewer": {
      "description": "reviewer",
      "mode": "subagent",
      "prompt": "Review carefully."
    },
    "kept": {
      "description": "kept agent",
      "prompt": "{file:./prompts/kept.md}"
    }
  }
}`)

	_, warnings, err := Import(root, "opencode", false, false)
	if err != nil {
		t.Fatal(err)
	}
	for _, rel := range []string{
		filepath.Join("targets", "opencode", "commands", "ship.md"),
		filepath.Join("targets", "opencode", "agents", "reviewer.md"),
	} {
		if _, err := os.Stat(filepath.Join(root, rel)); err != nil {
			t.Fatalf("stat %s: %v", rel, err)
		}
	}
	body, err := os.ReadFile(filepath.Join(root, "targets", "opencode", "config.extra.json"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(body)
	for _, want := range []string{`"kept"`, `"command"`, `"agent"`} {
		if !strings.Contains(text, want) {
			t.Fatalf("config.extra.json missing %q:\n%s", want, text)
		}
	}
	if len(warnings) < 2 {
		t.Fatalf("warnings = %v, want fidelity warnings for preserved inline command and agent", warnings)
	}
}

func TestImport_OpenCodeIncludeUserScope(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	root := t.TempDir()

	mustWritePluginFile(t, home, filepath.Join(".config", "opencode", "opencode.jsonc"), `{
  "$schema": "https://opencode.ai/config.json",
  "plugin": ["@acme/global-opencode"],
  "theme": "midnight"
}`)
	mustWritePluginFile(t, home, filepath.Join(".config", "opencode", "commands", "ship.md"), "---\ndescription: ship command\n---\n\nShip it.\n")
	mustWritePluginFile(t, home, filepath.Join(".config", "opencode", "agents", "reviewer.md"), "---\ndescription: reviewer\nmode: subagent\n---\n\nReview carefully.\n")
	mustWritePluginFile(t, home, filepath.Join(".config", "opencode", "themes", "midnight.json"), `{"$schema":"https://opencode.ai/theme.json","theme":{"primary":"#111827","text":"#f9fafb","background":"#020617"}}`)
	mustWritePluginFile(t, home, filepath.Join(".config", "opencode", "tools", "echo.ts"), "import { tool } from \"@opencode-ai/plugin\"\nexport default tool({ description: \"echo\", args: { value: tool.schema.string() }, async execute(args) { return args.value } })\n")
	mustWritePluginFile(t, home, filepath.Join(".config", "opencode", "skills", "demo", "SKILL.md"), "---\nname: demo\ndescription: global skill\nexecution_mode: docs_only\nsupported_agents:\n  - opencode\n---\n\n# Demo\n")
	mustWritePluginFile(t, home, filepath.Join(".config", "opencode", "plugins", "global.js"), "export default {};\n")
	mustWritePluginFile(t, home, filepath.Join(".config", "opencode", "package.json"), `{"name":"global-opencode-local","private":true,"dependencies":{"@opencode-ai/plugin":"latest"}}`)

	imported, warnings, err := Import(root, "opencode", false, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(imported.Targets) != 1 || imported.Targets[0] != "opencode" {
		t.Fatalf("targets = %v", imported.Targets)
	}
	for _, rel := range []string{
		filepath.Join("targets", "opencode", "package.yaml"),
		filepath.Join("targets", "opencode", "config.extra.json"),
		filepath.Join("targets", "opencode", "package.json"),
		filepath.Join("targets", "opencode", "commands", "ship.md"),
		filepath.Join("targets", "opencode", "agents", "reviewer.md"),
		filepath.Join("targets", "opencode", "themes", "midnight.json"),
		filepath.Join("targets", "opencode", "tools", "echo.ts"),
		filepath.Join("targets", "opencode", "plugins", "global.js"),
		filepath.Join("skills", "demo", "SKILL.md"),
	} {
		if _, err := os.Stat(filepath.Join(root, rel)); err != nil {
			t.Fatalf("stat %s: %v", rel, err)
		}
	}
	if len(warnings) != 0 {
		t.Fatalf("warnings = %v, want none", warnings)
	}
}

func TestImport_OpenCodeEnvSourcesLayerDeterministically(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	root := t.TempDir()
	envDir := t.TempDir()
	envFile := filepath.Join(t.TempDir(), "custom-opencode.jsonc")
	t.Setenv("OPENCODE_CONFIG_DIR", envDir)
	t.Setenv("OPENCODE_CONFIG", envFile)

	mustWritePluginFile(t, home, filepath.Join(".config", "opencode", "opencode.json"), `{"$schema":"https://opencode.ai/config.json","plugin":["@acme/global"],"theme":"global"}`)
	mustWritePluginFile(t, home, filepath.Join(".config", "opencode", "commands", "ship.md"), "---\ndescription: global\n---\n\nglobal\n")
	mustWritePluginFile(t, root, "opencode.json", `{"$schema":"https://opencode.ai/config.json","plugin":["@acme/project"],"theme":"project","mcp":{"project":{"type":"local","command":["echo","project"]}}}`)
	mustWritePluginFile(t, root, filepath.Join(".opencode", "commands", "ship.md"), "---\ndescription: project\n---\n\nproject\n")
	mustWritePluginFile(t, envDir, "opencode.json", `{"$schema":"https://opencode.ai/config.json","theme":"env-dir","mcp":{"envdir":{"type":"local","command":["echo","envdir"]}}}`)
	mustWritePluginFile(t, envDir, filepath.Join("commands", "ship.md"), "---\ndescription: envdir\n---\n\nenvdir\n")
	mustWritePluginFile(t, envDir, filepath.Join("plugins", "shared.js"), "export const EnvDirPlugin = async () => ({})\n")
	mustWritePluginFile(t, envDir, "package.json", `{"name":"env-dir-opencode-local"}`)
	mustWritePluginFile(t, filepath.Dir(envFile), filepath.Base(envFile), `{
  "$schema": "https://opencode.ai/config.json",
  "plugin": ["@acme/env-file"],
  "theme": "env-file",
  "permission": {"edit":"ask"}
}`)

	imported, warnings, err := Import(root, "opencode", false, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(imported.Targets) != 1 || imported.Targets[0] != "opencode" {
		t.Fatalf("targets = %v", imported.Targets)
	}
	if len(warnings) != 0 {
		t.Fatalf("warnings = %v, want none", warnings)
	}
	body, err := os.ReadFile(filepath.Join(root, "targets", "opencode", "package.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "@acme/env-file") || strings.Contains(string(body), "@acme/project") {
		t.Fatalf("package.yaml = %s", body)
	}
	configBody, err := os.ReadFile(filepath.Join(root, "targets", "opencode", "config.extra.json"))
	if err != nil {
		t.Fatal(err)
	}
	configText := string(configBody)
	for _, want := range []string{`"theme": "env-file"`, `"permission"`, `"edit": "ask"`} {
		if !strings.Contains(configText, want) {
			t.Fatalf("config.extra.json missing %q:\n%s", want, configText)
		}
	}
	mcpBody, err := os.ReadFile(filepath.Join(root, "mcp", "servers.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(mcpBody), "envdir:") || !strings.Contains(string(mcpBody), "project:") {
		t.Fatalf("servers.yaml = %s", mcpBody)
	}
	commandBody, err := os.ReadFile(filepath.Join(root, "targets", "opencode", "commands", "ship.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(commandBody), "envdir") {
		t.Fatalf("ship.md = %s", commandBody)
	}
	pluginBody, err := os.ReadFile(filepath.Join(root, "targets", "opencode", "plugins", "shared.js"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(pluginBody), "EnvDirPlugin") {
		t.Fatalf("shared.js = %s", pluginBody)
	}
	packageJSONBody, err := os.ReadFile(filepath.Join(root, "targets", "opencode", "package.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(packageJSONBody), "env-dir-opencode-local") {
		t.Fatalf("package.json = %s", packageJSONBody)
	}
}

func TestImport_OpenCodeNativeJSONCLayout(t *testing.T) {
	root := t.TempDir()
	mustWritePluginFile(t, root, "opencode.jsonc", `{
  // comment
  "$schema": "https://opencode.ai/config.json",
  "plugin": ["@acme/demo-opencode",],
  "mcp": {
    "context7": {
      "type": "local",
      "command": ["npx", "-y", "@upstash/context7-mcp",],
    },
  },
  "theme": "midnight",
}`)

	imported, warnings, err := Import(root, "opencode", false, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(imported.Targets) != 1 || imported.Targets[0] != "opencode" {
		t.Fatalf("targets = %v", imported.Targets)
	}
	for _, rel := range []string{
		filepath.Join("targets", "opencode", "package.yaml"),
		filepath.Join("targets", "opencode", "config.extra.json"),
		filepath.Join("mcp", "servers.yaml"),
	} {
		if _, err := os.Stat(filepath.Join(root, rel)); err != nil {
			t.Fatalf("stat %s: %v", rel, err)
		}
	}
	if len(warnings) != 0 {
		t.Fatalf("warnings = %v, want none", warnings)
	}
}

func TestValidate_OpenCodeAcceptsOPENCODECONFIGFallback(t *testing.T) {
	root := t.TempDir()
	envFile := filepath.Join(t.TempDir(), "custom-opencode.jsonc")
	t.Setenv("OPENCODE_CONFIG", envFile)
	mustWritePluginFile(t, filepath.Dir(envFile), filepath.Base(envFile), `{"$schema":"https://opencode.ai/config.json","plugin":["@acme/env-file"]}`)

	manifest := Default("demo", "opencode", "", "demo plugin", false)
	mustSavePackage(t, root, manifest, "")
	mustWritePluginFile(t, root, filepath.Join("targets", "opencode", "package.yaml"), "plugins:\n  - \"@acme/demo-opencode\"\n")

	graph, _, err := Discover(root)
	if err != nil {
		t.Fatal(err)
	}
	adapter, ok := platformexec.Lookup("opencode")
	if !ok {
		t.Fatal("missing opencode adapter")
	}
	diagnostics, err := adapter.Validate(root, graph, graph.Targets["opencode"])
	if err != nil {
		t.Fatal(err)
	}
	for _, failure := range diagnostics {
		if strings.Contains(failure.Message, "OpenCode config opencode.json") {
			t.Fatalf("diagnostics = %+v", diagnostics)
		}
	}
}

func TestValidate_OpenCodeAcceptsOPENCODECONFIGDIRFallback(t *testing.T) {
	root := t.TempDir()
	envDir := t.TempDir()
	t.Setenv("OPENCODE_CONFIG_DIR", envDir)
	mustWritePluginFile(t, envDir, "opencode.json", `{"$schema":"https://opencode.ai/config.json","plugin":["@acme/env-dir"]}`)

	manifest := Default("demo", "opencode", "", "demo plugin", false)
	mustSavePackage(t, root, manifest, "")
	mustWritePluginFile(t, root, filepath.Join("targets", "opencode", "package.yaml"), "plugins:\n  - \"@acme/demo-opencode\"\n")

	graph, _, err := Discover(root)
	if err != nil {
		t.Fatal(err)
	}
	adapter, ok := platformexec.Lookup("opencode")
	if !ok {
		t.Fatal("missing opencode adapter")
	}
	diagnostics, err := adapter.Validate(root, graph, graph.Targets["opencode"])
	if err != nil {
		t.Fatal(err)
	}
	for _, failure := range diagnostics {
		if strings.Contains(failure.Message, "OpenCode config opencode.json") {
			t.Fatalf("diagnostics = %+v", diagnostics)
		}
	}
}

func TestImport_OpenCodePrefersJSONOverJSONC(t *testing.T) {
	root := t.TempDir()
	mustWritePluginFile(t, root, "opencode.json", `{"$schema":"https://opencode.ai/config.json","plugin":["@acme/json"]}`)
	mustWritePluginFile(t, root, "opencode.jsonc", `{"$schema":"https://opencode.ai/config.json","plugin":["@acme/jsonc"]}`)

	_, warnings, err := Import(root, "opencode", false, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(warnings) == 0 {
		t.Fatalf("warnings = %v, want precedence warning", warnings)
	}
	if got := warnings[0].Message; !strings.Contains(got, "opencode.json takes precedence") {
		t.Fatalf("warning = %q", got)
	}
	body, err := os.ReadFile(filepath.Join(root, filepath.Join("targets", "opencode", "package.yaml")))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "@acme/json") || strings.Contains(string(body), "@acme/jsonc") {
		t.Fatalf("package.yaml = %s", body)
	}
}

func TestImport_OpenCodeRejectsCompatibilitySkillRoots(t *testing.T) {
	root := t.TempDir()
	mustWritePluginFile(t, root, filepath.Join(".claude", "skills", "demo", "SKILL.md"), "---\nname: demo\ndescription: claude\n---\n")
	mustWritePluginFile(t, root, filepath.Join(".agents", "skills", "demo", "SKILL.md"), "---\nname: demo\ndescription: agents\n---\n")

	if _, _, err := Import(root, "opencode", false, false); err == nil || !strings.Contains(err.Error(), "unsupported OpenCode native skill path .agents/skills: use skills/**") {
		t.Fatalf("Import error = %v", err)
	}
}

func TestImport_OpenCodeUserScopeProjectOverridesPluginFiles(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	root := t.TempDir()

	mustWritePluginFile(t, home, filepath.Join(".config", "opencode", "plugins", "shared.js"), "export default {name:\"global\"};\n")
	mustWritePluginFile(t, home, filepath.Join(".config", "opencode", "package.json"), `{"name":"global-opencode-local"}`)
	mustWritePluginFile(t, root, "opencode.json", `{"$schema":"https://opencode.ai/config.json","plugin":["@acme/project"]}`)
	mustWritePluginFile(t, root, filepath.Join(".opencode", "plugins", "shared.js"), "export default {name:\"project\"};\n")
	mustWritePluginFile(t, root, filepath.Join(".opencode", "package.json"), `{"name":"project-opencode-local"}`)

	_, warnings, err := Import(root, "opencode", false, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(warnings) != 0 {
		t.Fatalf("warnings = %v, want none", warnings)
	}
	body, err := os.ReadFile(filepath.Join(root, "targets", "opencode", "plugins", "shared.js"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), `name:"project"`) {
		t.Fatalf("shared.js = %s", body)
	}
	pkgBody, err := os.ReadFile(filepath.Join(root, "targets", "opencode", "package.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(pkgBody), "project-opencode-local") {
		t.Fatalf("package.json = %s", pkgBody)
	}
}

func TestValidate_OpenCodeRejectsInvalidPluginPackageJSON(t *testing.T) {
	root := t.TempDir()
	manifest := Default("demo", "opencode", "", "demo plugin", false)
	mustSavePackage(t, root, manifest, "")
	mustWritePluginFile(t, root, filepath.Join("targets", "opencode", "package.yaml"), "plugins:\n  - \"@acme/demo-opencode\"\n")
	mustWritePluginFile(t, root, filepath.Join("targets", "opencode", "package.json"), `[]`)
	mustWritePluginFile(t, root, "opencode.json", `{"$schema":"https://opencode.ai/config.json","plugin":["@acme/demo-opencode"]}`)

	graph, _, err := Discover(root)
	if err != nil {
		t.Fatal(err)
	}
	adapter, ok := platformexec.Lookup("opencode")
	if !ok {
		t.Fatal("missing opencode adapter")
	}
	diagnostics, err := adapter.Validate(root, graph, graph.Targets["opencode"])
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, failure := range diagnostics {
		if failure.Path == filepath.ToSlash(filepath.Join("targets", "opencode", "package.json")) &&
			strings.Contains(failure.Message, "invalid JSON") {
			found = true
		}
	}
	if !found {
		t.Fatalf("diagnostics = %+v", diagnostics)
	}
}

func TestValidate_OpenCodeRejectsPluginTreeWithoutEntryFile(t *testing.T) {
	root := t.TempDir()
	manifest := Default("demo", "opencode", "", "demo plugin", false)
	mustSavePackage(t, root, manifest, "")
	mustWritePluginFile(t, root, filepath.Join("targets", "opencode", "package.yaml"), "plugins:\n  - \"@acme/demo-opencode\"\n")
	mustWritePluginFile(t, root, filepath.Join("targets", "opencode", "plugins", "README.md"), "# demo\n")
	mustWritePluginFile(t, root, "opencode.json", `{"$schema":"https://opencode.ai/config.json","plugin":["@acme/demo-opencode"]}`)

	graph, _, err := Discover(root)
	if err != nil {
		t.Fatal(err)
	}
	adapter, ok := platformexec.Lookup("opencode")
	if !ok {
		t.Fatal("missing opencode adapter")
	}
	diagnostics, err := adapter.Validate(root, graph, graph.Targets["opencode"])
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, failure := range diagnostics {
		if failure.Path == filepath.ToSlash(filepath.Join("targets", "opencode", "plugins")) &&
			strings.Contains(failure.Message, "requires at least one JS/TS plugin entry file") {
			found = true
		}
	}
	if !found {
		t.Fatalf("diagnostics = %+v", diagnostics)
	}
}

func TestValidate_OpenCodeRejectsOldScaffoldPluginShape(t *testing.T) {
	root := t.TempDir()
	manifest := Default("demo", "opencode", "", "demo plugin", false)
	mustSavePackage(t, root, manifest, "")
	mustWritePluginFile(t, root, filepath.Join("targets", "opencode", "package.yaml"), "plugins:\n  - \"@acme/demo-opencode\"\n")
	mustWritePluginFile(t, root, filepath.Join("targets", "opencode", "plugins", "example.js"), "export default { setup() { return {} } }\n")
	mustWritePluginFile(t, root, "opencode.json", `{"$schema":"https://opencode.ai/config.json","plugin":["@acme/demo-opencode"]}`)

	graph, _, err := Discover(root)
	if err != nil {
		t.Fatal(err)
	}
	adapter, ok := platformexec.Lookup("opencode")
	if !ok {
		t.Fatal("missing opencode adapter")
	}
	diagnostics, err := adapter.Validate(root, graph, graph.Targets["opencode"])
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, failure := range diagnostics {
		if failure.Path == filepath.ToSlash(filepath.Join("targets", "opencode", "plugins", "example.js")) &&
			strings.Contains(failure.Message, "old scaffold shape") {
			found = true
		}
	}
	if !found {
		t.Fatalf("diagnostics = %+v", diagnostics)
	}
}

func TestValidate_OpenCodeRejectsHelperImportWithoutDependency(t *testing.T) {
	root := t.TempDir()
	manifest := Default("demo", "opencode", "", "demo plugin", false)
	mustSavePackage(t, root, manifest, "")
	mustWritePluginFile(t, root, filepath.Join("targets", "opencode", "package.yaml"), "plugins:\n  - \"@acme/demo-opencode\"\n")
	mustWritePluginFile(t, root, filepath.Join("targets", "opencode", "plugins", "custom-tool.js"), "import { tool } from \"@opencode-ai/plugin\"\nexport const CustomToolPlugin = async () => ({ tool: { demo: tool({ description: \"demo\", args: {}, async execute() { return \"ok\" } }) } })\n")
	mustWritePluginFile(t, root, filepath.Join("targets", "opencode", "package.json"), `{"name":"demo-opencode-local","private":true,"type":"module"}`)
	mustWritePluginFile(t, root, "opencode.json", `{"$schema":"https://opencode.ai/config.json","plugin":["@acme/demo-opencode"]}`)

	graph, _, err := Discover(root)
	if err != nil {
		t.Fatal(err)
	}
	adapter, ok := platformexec.Lookup("opencode")
	if !ok {
		t.Fatal("missing opencode adapter")
	}
	diagnostics, err := adapter.Validate(root, graph, graph.Targets["opencode"])
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, failure := range diagnostics {
		if failure.Path == filepath.ToSlash(filepath.Join("targets", "opencode", "package.json")) &&
			strings.Contains(failure.Message, `@opencode-ai/plugin`) {
			found = true
		}
	}
	if !found {
		t.Fatalf("diagnostics = %+v", diagnostics)
	}
}

func TestValidate_OpenCodeAcceptsHelperImportWithDependency(t *testing.T) {
	root := t.TempDir()
	manifest := Default("demo", "opencode", "", "demo plugin", false)
	mustSavePackage(t, root, manifest, "")
	mustWritePluginFile(t, root, filepath.Join("targets", "opencode", "package.yaml"), "plugins:\n  - \"@acme/demo-opencode\"\n")
	mustWritePluginFile(t, root, filepath.Join("targets", "opencode", "plugins", "custom-tool.js"), "import { tool } from \"@opencode-ai/plugin\"\nexport const CustomToolPlugin = async () => ({ tool: { demo: tool({ description: \"demo\", args: {}, async execute() { return \"ok\" } }) } })\n")
	mustWritePluginFile(t, root, filepath.Join("targets", "opencode", "package.json"), `{"name":"demo-opencode-local","private":true,"type":"module","dependencies":{"@opencode-ai/plugin":"latest"}}`)
	mustWritePluginFile(t, root, "opencode.json", `{"$schema":"https://opencode.ai/config.json","plugin":["@acme/demo-opencode"]}`)

	graph, _, err := Discover(root)
	if err != nil {
		t.Fatal(err)
	}
	adapter, ok := platformexec.Lookup("opencode")
	if !ok {
		t.Fatal("missing opencode adapter")
	}
	diagnostics, err := adapter.Validate(root, graph, graph.Targets["opencode"])
	if err != nil {
		t.Fatal(err)
	}
	for _, failure := range diagnostics {
		if failure.Path == filepath.ToSlash(filepath.Join("targets", "opencode", "package.json")) &&
			strings.Contains(failure.Message, `@opencode-ai/plugin`) {
			t.Fatalf("unexpected helper dependency failure: %+v", diagnostics)
		}
	}
}

func TestValidate_OpenCodeRejectsToolHelperImportWithoutDependency(t *testing.T) {
	root := t.TempDir()
	manifest := Default("demo", "opencode", "", "demo plugin", false)
	mustSavePackage(t, root, manifest, "")
	mustWritePluginFile(t, root, filepath.Join("targets", "opencode", "package.yaml"), "plugins:\n  - \"@acme/demo-opencode\"\n")
	mustWritePluginFile(t, root, filepath.Join("targets", "opencode", "tools", "echo.ts"), "import { tool } from \"@opencode-ai/plugin\"\nexport default tool({ description: \"echo\", args: { value: tool.schema.string() }, async execute(args) { return args.value } })\n")
	mustWritePluginFile(t, root, filepath.Join("targets", "opencode", "package.json"), `{"name":"demo-opencode-local","private":true,"type":"module"}`)
	mustWritePluginFile(t, root, "opencode.json", `{"$schema":"https://opencode.ai/config.json","plugin":["@acme/demo-opencode"]}`)

	graph, _, err := Discover(root)
	if err != nil {
		t.Fatal(err)
	}
	adapter, ok := platformexec.Lookup("opencode")
	if !ok {
		t.Fatal("missing opencode adapter")
	}
	diagnostics, err := adapter.Validate(root, graph, graph.Targets["opencode"])
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, failure := range diagnostics {
		if failure.Path == filepath.ToSlash(filepath.Join("targets", "opencode", "package.json")) &&
			strings.Contains(failure.Message, "standalone tool files") {
			found = true
		}
	}
	if !found {
		t.Fatalf("diagnostics = %+v", diagnostics)
	}
}

func TestImport_OpenCodeRejectsLegacyToolDirectory(t *testing.T) {
	root := t.TempDir()
	mustWritePluginFile(t, root, "opencode.json", `{"$schema":"https://opencode.ai/config.json","plugin":["@acme/demo-opencode"]}`)
	mustWritePluginFile(t, root, filepath.Join(".opencode", "tool", "echo.ts"), "export default { description: \"echo\", args: {}, async execute() { return \"ok\" } }\n")

	if _, _, err := Import(root, "opencode", false, false); err == nil || !strings.Contains(err.Error(), "unsupported OpenCode native path .opencode/tool: use .opencode/tools") {
		t.Fatalf("Import error = %v", err)
	}
}

func TestImport_CursorRejectsLegacyCursorRules(t *testing.T) {
	root := t.TempDir()
	mustWritePluginFile(t, root, ".cursorrules", "Always review generated code.\n")

	if _, _, err := Import(root, "cursor", false, false); err == nil || !strings.Contains(err.Error(), "unsupported Cursor native path .cursorrules: use .cursor/rules/*.mdc and optional root AGENTS.md") {
		t.Fatalf("Import error = %v", err)
	}
}

func TestRender_CursorRendersWorkspaceConfig(t *testing.T) {
	root := t.TempDir()
	manifest := Default("cursor-demo", "cursor", "", "cursor demo", false)
	mustSavePackage(t, root, manifest, "")
	mustWritePortableMCPFile(t, root, `format: plugin-kit-ai/mcp
version: 1

servers:
  context7:
    type: stdio
    stdio:
      command: npx
      args:
        - -y
        - "@upstash/context7-mcp"
`)
	mustWritePluginFile(t, root, filepath.Join("targets", "cursor", "rules", "project.mdc"), "---\ndescription: project rule\nglobs:\nalwaysApply: true\n---\n\n- Keep Cursor config generated.\n")
	mustWritePluginFile(t, root, filepath.Join("targets", "cursor", "AGENTS.md"), "# Cursor agents\n")

	result, err := Render(root, "cursor")
	if err != nil {
		t.Fatal(err)
	}
	if err := WriteArtifacts(root, result.Artifacts); err != nil {
		t.Fatal(err)
	}
	for _, rel := range []string{
		filepath.Join(".cursor", "mcp.json"),
		filepath.Join(".cursor", "rules", "project.mdc"),
		"AGENTS.md",
	} {
		if _, err := os.Stat(filepath.Join(root, rel)); err != nil {
			t.Fatalf("stat %s: %v", rel, err)
		}
	}
	body, err := os.ReadFile(filepath.Join(root, ".cursor", "mcp.json"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(body)
	for _, want := range []string{`"context7"`, `@upstash/context7-mcp`} {
		if !strings.Contains(text, want) {
			t.Fatalf(".cursor/mcp.json missing %q:\n%s", want, text)
		}
	}
}

func TestRender_CursorTracksRootAgentsAsManagedArtifact(t *testing.T) {
	root := t.TempDir()
	manifest := Default("cursor-demo", "cursor", "", "cursor demo", false)
	mustSavePackage(t, root, manifest, "")
	mustWritePluginFile(t, root, filepath.Join("targets", "cursor", "rules", "project.mdc"), "---\ndescription: project rule\nglobs:\nalwaysApply: true\n---\n\n- Keep Cursor config generated.\n")
	mustWritePluginFile(t, root, filepath.Join("targets", "cursor", "AGENTS.md"), "# Cursor agents\n")

	first, err := Render(root, "cursor")
	if err != nil {
		t.Fatal(err)
	}
	if err := WriteArtifacts(root, first.Artifacts); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(root, "targets", "cursor", "AGENTS.md")); err != nil {
		t.Fatal(err)
	}

	second, err := Render(root, "cursor")
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, rel := range second.StalePaths {
		if rel == "AGENTS.md" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("stale paths = %v, want AGENTS.md", second.StalePaths)
	}
}

func TestImport_CursorNativeLayout(t *testing.T) {
	root := t.TempDir()
	mustWritePluginFile(t, root, filepath.Join(".cursor", "mcp.json"), `{"context7":{"command":"npx","args":["-y","@upstash/context7-mcp"]}}`)
	mustWritePluginFile(t, root, filepath.Join(".cursor", "rules", "project.mdc"), "---\ndescription: project rule\nglobs:\nalwaysApply: true\n---\n\n- Keep Cursor config generated.\n")
	mustWritePluginFile(t, root, "AGENTS.md", "# Cursor agents\n")

	imported, warnings, err := Import(root, "", false, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(imported.Targets) != 1 || imported.Targets[0] != "cursor" {
		t.Fatalf("targets = %v", imported.Targets)
	}
	for _, rel := range []string{
		filepath.Join("mcp", "servers.yaml"),
		filepath.Join("targets", "cursor", "rules", "project.mdc"),
		filepath.Join("targets", "cursor", "AGENTS.md"),
	} {
		if _, err := os.Stat(filepath.Join(root, rel)); err != nil {
			t.Fatalf("stat %s: %v", rel, err)
		}
	}
	if len(warnings) != 0 {
		t.Fatalf("warnings = %v, want none", warnings)
	}
}

func TestImport_CursorExplicitAllowsRootAgentsOnly(t *testing.T) {
	root := t.TempDir()
	mustWritePluginFile(t, root, "AGENTS.md", "# Shared agents\n")

	imported, warnings, err := Import(root, "cursor", false, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(imported.Targets) != 1 || imported.Targets[0] != "cursor" {
		t.Fatalf("targets = %v", imported.Targets)
	}
	if _, err := os.Stat(filepath.Join(root, "targets", "cursor", "AGENTS.md")); err != nil {
		t.Fatalf("stat targets/cursor/AGENTS.md: %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("warnings = %v, want none", warnings)
	}
}

func TestInspect_CursorExposesWorkspaceSurfaceTiers(t *testing.T) {
	root := t.TempDir()
	manifest := Default("demo", "cursor", "", "demo plugin", false)
	mustSavePackage(t, root, manifest, "")
	mustWritePluginFile(t, root, filepath.Join("targets", "cursor", "rules", "project.mdc"), "---\ndescription: project rule\nglobs:\nalwaysApply: true\n---\n\n- Keep Cursor config generated.\n")

	inspection, _, err := Inspect(root, "cursor")
	if err != nil {
		t.Fatal(err)
	}
	if len(inspection.Targets) != 1 {
		t.Fatalf("targets = %+v", inspection.Targets)
	}
	target := inspection.Targets[0]
	var foundMCP, foundRules, foundAgents bool
	for _, surface := range target.NativeSurfaces {
		switch {
		case surface.Kind == "mcp" && surface.Tier == "stable":
			foundMCP = true
		case surface.Kind == "rules" && surface.Tier == "stable":
			foundRules = true
		case surface.Kind == "agents_md" && surface.Tier == "stable":
			foundAgents = true
		}
	}
	if !foundMCP || !foundRules || !foundAgents {
		t.Fatalf("native_surfaces = %+v", target.NativeSurfaces)
	}
}

func TestRender_ClaudeDefaultHooksStayStableSubset(t *testing.T) {
	root := t.TempDir()
	manifest := Default("demo", "claude", "go", "demo plugin", false)
	mustSavePackage(t, root, manifest, "go")
	result, err := Render(root, "claude")
	if err != nil {
		t.Fatal(err)
	}
	if err := WriteArtifacts(root, result.Artifacts); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(filepath.Join(root, "hooks", "hooks.json"))
	if err != nil {
		t.Fatal(err)
	}
	got := string(body)
	for _, want := range []string{`"Stop"`, `"PreToolUse"`, `"UserPromptSubmit"`} {
		if !strings.Contains(got, want) {
			t.Fatalf("default Claude hooks missing %s:\n%s", want, got)
		}
	}
	for _, unwanted := range []string{`"SessionStart"`, `"WorktreeRemove"`} {
		if strings.Contains(got, unwanted) {
			t.Fatalf("default Claude hooks unexpectedly contain %s:\n%s", unwanted, got)
		}
	}
}

func TestRender_ClaudeRendersSettingsLSPAndUserConfig(t *testing.T) {
	root := t.TempDir()
	manifest := Default("demo", "claude", "go", "demo plugin", false)
	mustSavePackage(t, root, manifest, "go")
	mustWritePluginFile(t, root, filepath.Join("targets", "claude", "settings.json"), `{"agent":"reviewer"}`)
	mustWritePluginFile(t, root, filepath.Join("targets", "claude", "lsp.json"), `{"servers":{"demo":{"command":["demo-lsp"]}}}`)
	mustWritePluginFile(t, root, filepath.Join("targets", "claude", "user-config.json"), `{"api_token":{"description":"API token","secret":true}}`)
	mustWritePluginFile(t, root, filepath.Join("targets", "claude", "agents", "reviewer.md"), "---\nname: reviewer\ndescription: review\n---\nReview.\n")

	result, err := Render(root, "claude")
	if err != nil {
		t.Fatal(err)
	}
	if err := WriteArtifacts(root, result.Artifacts); err != nil {
		t.Fatal(err)
	}

	settingsBody, err := os.ReadFile(filepath.Join(root, "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(settingsBody), `"agent":"reviewer"`) {
		t.Fatalf("settings.json = %s", settingsBody)
	}
	lspBody, err := os.ReadFile(filepath.Join(root, ".lsp.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(lspBody), `"servers"`) {
		t.Fatalf(".lsp.json = %s", lspBody)
	}
	pluginBody, err := os.ReadFile(filepath.Join(root, ".claude-plugin", "plugin.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(pluginBody), `"userConfig"`) || !strings.Contains(string(pluginBody), `"api_token"`) {
		t.Fatalf("plugin.json = %s", pluginBody)
	}
}

func TestRender_ClaudeRejectsManagedOverridesInManifestExtra(t *testing.T) {
	root := t.TempDir()
	manifest := Default("demo", "claude", "go", "demo plugin", false)
	mustSavePackage(t, root, manifest, "go")
	mustWritePluginFile(t, root, filepath.Join("targets", "claude", "manifest.extra.json"), `{"settings":{"agent":"override"}}`)

	if _, err := Render(root, "claude"); err == nil || !strings.Contains(err.Error(), `claude manifest.extra.json may not override canonical field "settings"`) {
		t.Fatalf("Render error = %v", err)
	}
}

func TestImport_CurrentNativeCodexShellProject(t *testing.T) {
	root := t.TempDir()
	mustWritePluginFile(t, root, filepath.Join("scripts", "main.sh"), "#!/usr/bin/env bash\n")
	mustWritePluginFile(t, root, filepath.Join(".codex", "config.toml"), "notify = [\"./bin/demo\", \"notify\"]\nmodel = \"gpt-5.4-mini\"\n")
	mustWritePluginFile(t, root, filepath.Join(".codex-plugin", "plugin.json"), `{"name":"demo","version":"0.1.0","description":"demo"}`)

	_, _, err := Import(root, "codex-native", false, false)
	if err != nil {
		t.Fatal(err)
	}
	launcher, err := LoadLauncher(root)
	if err != nil {
		t.Fatal(err)
	}
	if launcher.Runtime != "shell" {
		t.Fatalf("runtime = %q, want shell", launcher.Runtime)
	}
	if launcher.Entrypoint != "./bin/demo" {
		t.Fatalf("entrypoint = %q", launcher.Entrypoint)
	}
	body, err := os.ReadFile(filepath.Join(root, "targets", "codex-runtime", "package.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "model_hint: gpt-5.4-mini") {
		t.Fatalf("package metadata = %q", string(body))
	}
}

func TestRender_CodexMergesManifestAndConfigExtra(t *testing.T) {
	root := t.TempDir()
	manifest := Default("demo", "codex-runtime", "go", "demo plugin", false)
	manifest.Targets = []string{"codex-package", "codex-runtime"}
	mustSavePackage(t, root, manifest, "go")
	mustWritePluginFile(t, root, filepath.Join("targets", "codex-runtime", "package.yaml"), "model_hint: gpt-5.4-mini\n")
	mustWritePluginFile(t, root, filepath.Join("targets", "codex-package", "package.yaml"), "homepage: https://example.com/demo\nauthor:\n  name: Example Maintainer\nrepository: https://github.com/example/demo\nlicense: MIT\nkeywords:\n  - codex\n  - demo\n")
	mustWritePluginFile(t, root, filepath.Join("targets", "codex-package", "interface.json"), `{"defaultPrompt":["Run the demo"]}`)
	mustWritePluginFile(t, root, filepath.Join("targets", "codex-package", "manifest.extra.json"), `{"x-example":{"enabled":true}}`)
	mustWritePluginFile(t, root, filepath.Join("targets", "codex-runtime", "config.extra.toml"), "approval_policy = \"never\"\n[ui]\nverbose = true\n")
	mustWritePluginFile(t, root, filepath.Join("targets", "codex-package", "app.json"), `{"name":"demo-app"}`)

	result, err := Render(root, "all")
	if err != nil {
		t.Fatal(err)
	}
	if err := WriteArtifacts(root, result.Artifacts); err != nil {
		t.Fatal(err)
	}

	pluginBody, err := os.ReadFile(filepath.Join(root, ".codex-plugin", "plugin.json"))
	if err != nil {
		t.Fatal(err)
	}
	var plugin map[string]any
	if err := json.Unmarshal(pluginBody, &plugin); err != nil {
		t.Fatal(err)
	}
	if plugin["name"] != "demo" || plugin["version"] != "0.1.0" || plugin["description"] != "demo plugin" {
		t.Fatalf("plugin manifest = %+v", plugin)
	}
	if plugin["homepage"] != "https://example.com/demo" {
		t.Fatalf("plugin manifest missing homepage: %+v", plugin)
	}
	if plugin["repository"] != "https://github.com/example/demo" || plugin["license"] != "MIT" {
		t.Fatalf("plugin manifest missing package metadata: %+v", plugin)
	}
	if _, ok := plugin["author"].(map[string]any); !ok {
		t.Fatalf("plugin manifest missing author object: %+v", plugin)
	}
	if _, ok := plugin["interface"].(map[string]any); !ok {
		t.Fatalf("plugin manifest missing interface object: %+v", plugin)
	}
	if plugin["apps"] != "./.app.json" {
		t.Fatalf("plugin manifest missing apps: %+v", plugin)
	}
	if _, ok := plugin["x-example"].(map[string]any); !ok {
		t.Fatalf("plugin manifest missing passthrough extra: %+v", plugin)
	}

	configBody, err := os.ReadFile(filepath.Join(root, ".codex", "config.toml"))
	if err != nil {
		t.Fatal(err)
	}
	got := string(configBody)
	for _, want := range []string{
		`model = "gpt-5.4-mini"`,
		`notify = ["./bin/demo", "notify"]`,
		`approval_policy = "never"`,
		`[ui]`,
		`verbose = true`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf(".codex/config.toml missing %q:\n%s", want, got)
		}
	}
}

func TestRender_CodexRejectsManagedOverridesInExtraDocs(t *testing.T) {
	root := t.TempDir()
	manifest := Default("demo", "codex-runtime", "go", "demo plugin", false)
	manifest.Targets = []string{"codex-package", "codex-runtime"}
	mustSavePackage(t, root, manifest, "go")
	mustWritePluginFile(t, root, filepath.Join("targets", "codex-package", "manifest.extra.json"), `{"homepage":"https://override.example.com"}`)
	if _, err := Render(root, "codex-package"); err == nil || !strings.Contains(err.Error(), `codex-package manifest.extra.json may not override canonical field "homepage"`) {
		t.Fatalf("Render error = %v", err)
	}

	mustWritePluginFile(t, root, filepath.Join("targets", "codex-package", "manifest.extra.json"), `{"interface":{"defaultPrompt":["override"]}}`)
	if _, err := Render(root, "codex-package"); err == nil || !strings.Contains(err.Error(), `codex-package manifest.extra.json may not override canonical field "interface"`) {
		t.Fatalf("Render error = %v", err)
	}

	mustWritePluginFile(t, root, filepath.Join("targets", "codex-package", "manifest.extra.json"), `{}`)
	mustWritePluginFile(t, root, filepath.Join("targets", "codex-runtime", "config.extra.toml"), "model = \"gpt-4.1\"\n")
	if _, err := Render(root, "codex-runtime"); err == nil || !strings.Contains(err.Error(), `codex-runtime config.extra.toml may not override canonical field "model"`) {
		t.Fatalf("Render error = %v", err)
	}
}

func TestRender_CodexRejectsInvalidStructuredDocs(t *testing.T) {
	root := t.TempDir()
	manifest := Default("demo", "codex-package", "", "demo plugin", false)
	manifest.Targets = []string{"codex-package"}
	mustSavePackage(t, root, manifest, "")
	mustWritePluginFile(t, root, filepath.Join("targets", "codex-package", "interface.json"), `{"defaultPrompt":"Run the demo"}`)
	if _, err := Render(root, "codex-package"); err == nil || !strings.Contains(err.Error(), "interface.defaultPrompt must be an array of strings") {
		t.Fatalf("Render error = %v", err)
	}

	mustWritePluginFile(t, root, filepath.Join("targets", "codex-package", "interface.json"), `{"defaultPrompt":["Run the demo"]}`)
	mustWritePluginFile(t, root, filepath.Join("targets", "codex-package", "app.json"), `["demo-app"]`)
	if _, err := Render(root, "codex-package"); err == nil || !strings.Contains(err.Error(), "Codex app manifest must be a JSON object") {
		t.Fatalf("Render error = %v", err)
	}
}

func TestRender_CodexSkipsEmptyAppPlaceholder(t *testing.T) {
	root := t.TempDir()
	manifest := Default("demo", "codex-package", "", "demo plugin", false)
	manifest.Targets = []string{"codex-package"}
	mustSavePackage(t, root, manifest, "")
	mustWritePluginFile(t, root, filepath.Join("targets", "codex-package", "interface.json"), `{"defaultPrompt":["Run the demo"]}`)
	mustWritePluginFile(t, root, filepath.Join("targets", "codex-package", "app.json"), `{}`)

	result, err := Render(root, "codex-package")
	if err != nil {
		t.Fatal(err)
	}
	if err := WriteArtifacts(root, result.Artifacts); err != nil {
		t.Fatal(err)
	}
	pluginBody, err := os.ReadFile(filepath.Join(root, ".codex-plugin", "plugin.json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(pluginBody), `"apps": "./.app.json"`) {
		t.Fatalf("plugin manifest unexpectedly enables apps placeholder:\n%s", pluginBody)
	}
	if _, err := os.Stat(filepath.Join(root, ".app.json")); !os.IsNotExist(err) {
		t.Fatalf("unexpected .app.json artifact")
	}
}

func TestImport_CurrentNativeCodexPreservesExtraDocs(t *testing.T) {
	root := t.TempDir()
	mustWritePluginFile(t, root, filepath.Join("scripts", "main.sh"), "#!/usr/bin/env bash\n")
	mustWritePluginFile(t, root, filepath.Join(".codex", "config.toml"), "model = \"gpt-5.4-mini\"\nnotify = [\"./bin/demo\", \"notify\", \"extra\"]\napproval_policy = \"never\"\n[ui]\nverbose = true\n")
	mustWritePluginFile(t, root, filepath.Join(".codex-plugin", "plugin.json"), `{"name":"demo","version":"0.1.0","description":"demo","author":{"name":"Example Maintainer","email":"maintainer@example.com"},"homepage":"https://example.com/demo","repository":"https://github.com/example/demo","license":"MIT","keywords":["codex","demo"],"interface":{"defaultPrompt":["Run the demo"]},"apps":"./.app.json","x-extra":{"enabled":true}}`)
	mustWritePluginFile(t, root, ".app.json", `{"name":"demo-app"}`)

	_, warnings, err := Import(root, "codex-native", false, false)
	if err != nil {
		t.Fatal(err)
	}
	launcher, err := LoadLauncher(root)
	if err != nil {
		t.Fatal(err)
	}
	if launcher.Entrypoint != "./bin/demo" {
		t.Fatalf("entrypoint = %q", launcher.Entrypoint)
	}
	if len(warnings) == 0 {
		t.Fatal("expected fidelity warnings")
	}

	packageBody, err := os.ReadFile(filepath.Join(root, "targets", "codex-package", "package.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"homepage: https://example.com/demo",
		"repository: https://github.com/example/demo",
		"license: MIT",
		"- codex",
		"email: maintainer@example.com",
	} {
		if !strings.Contains(string(packageBody), want) {
			t.Fatalf("package.yaml missing %q:\n%s", want, packageBody)
		}
	}
	interfaceBody, err := os.ReadFile(filepath.Join(root, "targets", "codex-package", "interface.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(interfaceBody), `"defaultPrompt": [`) || !strings.Contains(string(interfaceBody), `"Run the demo"`) {
		t.Fatalf("interface.json = %s", interfaceBody)
	}
	appBody, err := os.ReadFile(filepath.Join(root, "targets", "codex-package", "app.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(appBody), `"name":"demo-app"`) {
		t.Fatalf("app.json = %s", appBody)
	}
	manifestExtra, err := os.ReadFile(filepath.Join(root, "targets", "codex-package", "manifest.extra.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(manifestExtra), `"x-extra": {`) {
		t.Fatalf("manifest.extra.json = %s", manifestExtra)
	}
	configExtra, err := os.ReadFile(filepath.Join(root, "targets", "codex-runtime", "config.extra.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(configExtra), `approval_policy =`) || !strings.Contains(string(configExtra), `never`) || !strings.Contains(string(configExtra), `[ui]`) {
		t.Fatalf("config.extra.toml = %s", configExtra)
	}
}

func TestImport_CurrentNativeCodexRejectsLegacyPluginShapes(t *testing.T) {
	root := t.TempDir()
	mustWritePluginFile(t, root, filepath.Join(".codex-plugin", "plugin.json"), `{"name":"demo","version":"0.1.0","description":"demo","author":"Example Maintainer","apps":["./.app.json"]}`)
	mustWritePluginFile(t, root, ".app.json", `{"name":"demo-app"}`)

	if _, _, err := Import(root, "codex-package", false, false); err == nil || !strings.Contains(err.Error(), "Codex plugin author must be a JSON object") {
		t.Fatalf("Import error = %v", err)
	}

	mustWritePluginFile(t, root, filepath.Join(".codex-plugin", "plugin.json"), `{"name":"demo","version":"0.1.0","description":"demo","author":{"name":"Example Maintainer"},"apps":["./.app.json"]}`)
	if _, _, err := Import(root, "codex-package", false, false); err == nil || !strings.Contains(err.Error(), "Codex plugin apps must be a string") {
		t.Fatalf("Import error = %v", err)
	}
}

func TestImport_CurrentNativeCodexRejectsMalformedStructuredDocs(t *testing.T) {
	root := t.TempDir()
	mustWritePluginFile(t, root, filepath.Join(".codex-plugin", "plugin.json"), `{"name":"demo","version":"0.1.0","description":"demo","interface":{"defaultPrompt":"Run the demo"},"apps":"./.app.json"}`)
	mustWritePluginFile(t, root, ".app.json", `["demo-app"]`)

	if _, _, err := Import(root, "codex-package", false, false); err == nil || !strings.Contains(err.Error(), "interface.defaultPrompt must be an array of strings") {
		t.Fatalf("Import error = %v", err)
	}

	mustWritePluginFile(t, root, filepath.Join(".codex-plugin", "plugin.json"), `{"name":"demo","version":"0.1.0","description":"demo","interface":{"defaultPrompt":["Run the demo"]},"apps":"./.app.json"}`)
	if _, _, err := Import(root, "codex-package", false, false); err == nil || !strings.Contains(err.Error(), "Codex app manifest must be a JSON object") {
		t.Fatalf("Import error = %v", err)
	}
}

func TestImport_CurrentNativeCodexRejectsMalformedConfigShapes(t *testing.T) {
	root := t.TempDir()
	mustWritePluginFile(t, root, filepath.Join(".codex", "config.toml"), "model = true\nnotify = [\"./bin/demo\", \"notify\"]\n")

	if _, _, err := Import(root, "codex-runtime", false, false); err == nil || !strings.Contains(err.Error(), `Codex config field "model" must be a string`) {
		t.Fatalf("Import error = %v", err)
	}

	mustWritePluginFile(t, root, filepath.Join(".codex", "config.toml"), "model = \"gpt-5.4-mini\"\nnotify = \"./bin/demo notify\"\n")
	if _, _, err := Import(root, "codex-runtime", false, false); err == nil || !strings.Contains(err.Error(), `Codex config field "notify" must be an array of non-empty strings`) {
		t.Fatalf("Import error = %v", err)
	}
}

func TestImport_ClaudeHooksJSONParsingHandlesNonFirstCommand(t *testing.T) {
	root := t.TempDir()
	mustWritePluginFile(t, root, filepath.Join(".claude-plugin", "plugin.json"), `{"name":"demo","version":"0.1.0","description":"demo"}`)
	mustWritePluginFile(t, root, filepath.Join("hooks", "hooks.json"), `{
  "hooks": {
    "Stop": [{
      "hooks": [
        {"type": "prompt", "command": "ignored"},
        {"type": "command", "command": "./bin/demo Stop"}
      ]
    }]
  }
}`)

	_, _, err := Import(root, "claude", false, false)
	if err != nil {
		t.Fatal(err)
	}
	launcher, err := LoadLauncher(root)
	if err != nil {
		t.Fatal(err)
	}
	if launcher.Entrypoint != "./bin/demo" {
		t.Fatalf("entrypoint = %q", launcher.Entrypoint)
	}
}

func TestImport_ClaudeManifestlessLayoutPreservesCanonicalDocs(t *testing.T) {
	root := t.TempDir()
	mustWritePluginFile(t, root, filepath.Join("hooks", "hooks.json"), `{"hooks":{"Stop":[{"hooks":[{"type":"command","command":"./bin/demo Stop"}]}]}}`)
	mustWritePluginFile(t, root, "settings.json", `{"agent":"reviewer"}`)
	mustWritePluginFile(t, root, ".lsp.json", `{"servers":{"demo":{"command":["demo-lsp"]}}}`)
	mustWritePluginFile(t, root, filepath.Join("commands", "ship.md"), "# ship\n")
	mustWritePluginFile(t, root, filepath.Join("agents", "reviewer.md"), "---\nname: reviewer\ndescription: review\n---\nReview.\n")

	imported, warnings, err := Import(root, "claude", false, false)
	if err != nil {
		t.Fatal(err)
	}
	if imported.Name != "plugin" {
		t.Fatalf("imported manifest name = %q, want %q", imported.Name, "plugin")
	}
	for _, rel := range []string{
		filepath.Join("targets", "claude", "settings.json"),
		filepath.Join("targets", "claude", "lsp.json"),
		filepath.Join("targets", "claude", "agents", "reviewer.md"),
		filepath.Join("targets", "claude", "commands", "ship.md"),
	} {
		if _, err := os.Stat(filepath.Join(root, rel)); err != nil {
			t.Fatalf("stat %s: %v", rel, err)
		}
	}
	var found bool
	for _, warning := range warnings {
		if strings.Contains(warning.Message, "native Claude plugin imported without manifest") {
			found = true
		}
	}
	if !found {
		t.Fatalf("warnings = %+v", warnings)
	}
}

func TestImport_ClaudeNormalizesCustomPathsAndUserConfig(t *testing.T) {
	root := t.TempDir()
	mustWritePluginFile(t, root, filepath.Join(".claude-plugin", "plugin.json"), `{
  "name":"demo",
  "version":"0.1.0",
  "description":"demo",
  "commands":"./custom-commands",
  "agents":["./custom-agents"],
  "hooks":"./custom-hooks.json",
  "lspServers":"./custom-lsp.json",
  "settings":{"agent":"reviewer"},
  "userConfig":{"api_token":{"description":"token","secret":true}}
}`)
	mustWritePluginFile(t, root, filepath.Join("custom-commands", "ship.md"), "# ship\n")
	mustWritePluginFile(t, root, filepath.Join("custom-agents", "reviewer.md"), "---\nname: reviewer\ndescription: review\n---\nReview.\n")
	mustWritePluginFile(t, root, "custom-hooks.json", `{"hooks":{"Stop":[{"hooks":[{"type":"command","command":"./bin/demo Stop"}]}]}}`)
	mustWritePluginFile(t, root, "custom-lsp.json", `{"servers":{"demo":{"command":["demo-lsp"]}}}`)

	_, warnings, err := Import(root, "claude", false, false)
	if err != nil {
		t.Fatal(err)
	}
	for _, rel := range []string{
		filepath.Join("targets", "claude", "commands", "ship.md"),
		filepath.Join("targets", "claude", "agents", "reviewer.md"),
		filepath.Join("targets", "claude", "hooks", "hooks.json"),
		filepath.Join("targets", "claude", "lsp.json"),
		filepath.Join("targets", "claude", "settings.json"),
		filepath.Join("targets", "claude", "user-config.json"),
	} {
		if _, err := os.Stat(filepath.Join(root, rel)); err != nil {
			t.Fatalf("stat %s: %v", rel, err)
		}
	}
	var foundCommands, foundHooks bool
	for _, warning := range warnings {
		if strings.Contains(warning.Message, "custom Claude commands paths were normalized") {
			foundCommands = true
		}
		if strings.Contains(warning.Message, "custom Claude hooks path was normalized") {
			foundHooks = true
		}
	}
	if !foundCommands || !foundHooks {
		t.Fatalf("warnings = %+v", warnings)
	}
}

func TestImport_ClaudeNormalizesMultiPathArrays(t *testing.T) {
	root := t.TempDir()
	mustWritePluginFile(t, root, filepath.Join(".claude-plugin", "plugin.json"), `{
  "name":"demo",
  "version":"0.1.0",
  "description":"demo",
  "hooks":["./custom-hooks-a.json","./custom-hooks-b.json"],
  "lspServers":["./custom-lsp-a.json","./custom-lsp-b.json"],
  "mcpServers":["./custom-mcp-a.json","./custom-mcp-b.json"]
}`)
	mustWritePluginFile(t, root, "custom-hooks-a.json", `{"hooks":{"Stop":[{"hooks":[{"type":"command","command":"./bin/demo Stop"}]}]}}`)
	mustWritePluginFile(t, root, "custom-hooks-b.json", `{"hooks":{"Stop":[{"hooks":[{"type":"command","command":"./bin/demo-again Stop"}]}],"UserPromptSubmit":[{"hooks":[{"type":"command","command":"./bin/demo UserPromptSubmit"}]}]}}`)
	mustWritePluginFile(t, root, "custom-lsp-a.json", `{"demo":{"command":["demo-lsp"]}}`)
	mustWritePluginFile(t, root, "custom-lsp-b.json", `{"demo":{"command":["demo-lsp"]},"review":{"command":["review-lsp"]}}`)
	mustWritePluginFile(t, root, "custom-mcp-a.json", `{"demo":{"command":"demo","args":["serve"]}}`)
	mustWritePluginFile(t, root, "custom-mcp-b.json", `{"demo":{"command":"demo","args":["serve"]},"review":{"command":"review","args":["serve"]}}`)

	_, warnings, err := Import(root, "claude", false, false)
	if err != nil {
		t.Fatal(err)
	}

	hooksBody, err := os.ReadFile(filepath.Join(root, "targets", "claude", "hooks", "hooks.json"))
	if err != nil {
		t.Fatal(err)
	}
	var hooksDoc map[string]any
	if err := json.Unmarshal(hooksBody, &hooksDoc); err != nil {
		t.Fatal(err)
	}
	hooksMap, ok := hooksDoc["hooks"].(map[string]any)
	if !ok {
		t.Fatalf("hooks document = %#v", hooksDoc)
	}
	stopEntries, ok := hooksMap["Stop"].([]any)
	if !ok || len(stopEntries) != 2 {
		t.Fatalf("Stop hooks = %#v", hooksMap["Stop"])
	}
	if _, err := os.Stat(filepath.Join(root, "targets", "claude", "lsp.json")); err != nil {
		t.Fatalf("stat lsp: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "mcp", "servers.yaml")); err != nil {
		t.Fatalf("stat mcp: %v", err)
	}

	var foundHooks, foundLSP, foundMCP bool
	for _, warning := range warnings {
		if strings.Contains(warning.Message, "custom Claude hooks path array was normalized") {
			foundHooks = true
		}
		if strings.Contains(warning.Message, "custom Claude lspServers path array was normalized") {
			foundLSP = true
		}
		if strings.Contains(warning.Message, "custom Claude mcpServers path array was normalized") {
			foundMCP = true
		}
	}
	if !foundHooks || !foundLSP || !foundMCP {
		t.Fatalf("warnings = %+v", warnings)
	}
}

func TestImport_ClaudeRejectsConflictingMultiPathArrays(t *testing.T) {
	root := t.TempDir()
	mustWritePluginFile(t, root, filepath.Join(".claude-plugin", "plugin.json"), `{
  "name":"demo",
  "version":"0.1.0",
  "description":"demo",
  "lspServers":["./custom-lsp-a.json","./custom-lsp-b.json"]
}`)
	mustWritePluginFile(t, root, "custom-lsp-a.json", `{"demo":{"command":["demo-lsp"]}}`)
	mustWritePluginFile(t, root, "custom-lsp-b.json", `{"demo":{"command":["other-lsp"]}}`)

	_, _, err := Import(root, "claude", false, false)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), `duplicate key "demo" conflicts`) {
		t.Fatalf("error = %v", err)
	}
}

func TestImport_ClaudeRejectsInvalidHookMultiPathMerge(t *testing.T) {
	root := t.TempDir()
	mustWritePluginFile(t, root, filepath.Join(".claude-plugin", "plugin.json"), `{
  "name":"demo",
  "version":"0.1.0",
  "description":"demo",
  "hooks":["./custom-hooks-a.json","./custom-hooks-b.json"]
}`)
	mustWritePluginFile(t, root, "custom-hooks-a.json", `{"hooks":{"Stop":[{"hooks":[{"type":"command","command":"./bin/demo Stop"}]}]}}`)
	mustWritePluginFile(t, root, "custom-hooks-b.json", `{"hooks":{"Stop":{"bad":true}}}`)

	_, _, err := Import(root, "claude", false, false)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "mixes object and non-object shapes") && !strings.Contains(err.Error(), "mixes array and non-array shapes") {
		t.Fatalf("error = %v", err)
	}
}

func TestImport_ClaudeManifestlessDetectsCommandsOnly(t *testing.T) {
	root := t.TempDir()
	mustWritePluginFile(t, root, filepath.Join("commands", "ship.md"), "# ship\n")

	imported, warnings, err := Import(root, "", false, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(imported.Targets) != 1 || imported.Targets[0] != "claude" {
		t.Fatalf("targets = %+v", imported.Targets)
	}
	if _, err := os.Stat(filepath.Join(root, "targets", "claude", "commands", "ship.md")); err != nil {
		t.Fatalf("stat command: %v", err)
	}
	if !containsWarning(warnings, "native Claude plugin imported without manifest") {
		t.Fatalf("warnings = %+v", warnings)
	}
}

func TestImport_ClaudeManifestlessDetectsAgentsOnly(t *testing.T) {
	root := t.TempDir()
	mustWritePluginFile(t, root, filepath.Join("agents", "reviewer.md"), "---\nname: reviewer\ndescription: review\n---\nReview.\n")

	imported, warnings, err := Import(root, "", false, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(imported.Targets) != 1 || imported.Targets[0] != "claude" {
		t.Fatalf("targets = %+v", imported.Targets)
	}
	if _, err := os.Stat(filepath.Join(root, "targets", "claude", "agents", "reviewer.md")); err != nil {
		t.Fatalf("stat agent: %v", err)
	}
	if !containsWarning(warnings, "native Claude plugin imported without manifest") {
		t.Fatalf("warnings = %+v", warnings)
	}
}

func TestImport_RefusesOverwriteBeforeWritingImportedLayout(t *testing.T) {
	root := t.TempDir()
	mustWritePluginFile(t, root, FileName, `format: plugin-kit-ai/package
name: "existing"
version: "0.1.0"
description: "existing"
targets: ["codex"]
`)
	mustWritePluginFile(t, root, LauncherFileName, "runtime: go\nentrypoint: ./bin/existing\n")
	mustWritePluginFile(t, root, filepath.Join(".codex", "config.toml"), "model = \"gpt-5.4-mini\"\nnotify = [\"./bin/demo\", \"notify\"]\n")
	mustWritePluginFile(t, root, filepath.Join(".codex-plugin", "plugin.json"), `{"name":"demo","version":"0.1.0","description":"demo"}`)

	_, _, err := Import(root, "codex-native", false, false)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "refusing to overwrite existing file plugin.yaml") {
		t.Fatalf("error = %v", err)
	}
	for _, rel := range []string{
		filepath.Join("targets", "codex-package", "package.yaml"),
		filepath.Join("targets", "codex-runtime", "package.yaml"),
		filepath.Join("mcp", "servers.yaml"),
	} {
		if _, statErr := os.Stat(filepath.Join(root, rel)); !os.IsNotExist(statErr) {
			t.Fatalf("expected %s to stay absent, err=%v", rel, statErr)
		}
	}
}

func TestImport_CurrentNativeGeminiLayout(t *testing.T) {
	root := t.TempDir()
	mustWritePluginFile(t, root, "gemini-extension.json", `{
	  "name":"demo",
	  "version":"0.2.0",
	  "description":"gemini demo",
	  "contextFileName":"TEAM.md",
	  "excludeTools":["run_shell_command(rm -rf)"],
	  "migratedTo":"https://github.com/example/gemini-demo-v2",
	  "plan":{"directory":".gemini/plans","retentionDays":7},
	  "settings":[{"name":"release-profile","description":"profile","envVar":"RELEASE_PROFILE","sensitive":false}],
	  "themes":[{"name":"release-dawn","background":{"primary":"#fff9f2"},"text":{"primary":"#2e1f14"}}],
	  "mcpServers":{"demo":{"command":"demo","args":["serve"]}},
	  "x_galleryTopic":"gemini-cli-extension"
	}`)
	mustWritePluginFile(t, root, "TEAM.md", "# Team\n")
	mustWritePluginFile(t, root, filepath.Join("contexts", "RELEASE.md"), "# Release\n")
	mustWritePluginFile(t, root, filepath.Join("commands", "release", "deploy.toml"), "description = \"deploy\"\n")
	mustWritePluginFile(t, root, filepath.Join("policies", "review.toml"), "name = \"review\"\n")
	mustWritePluginFile(t, root, filepath.Join("hooks", "hooks.json"), "{\"hooks\":{}}\n")

	manifest, _, err := Import(root, "gemini", false, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(manifest.Targets) != 1 || manifest.Targets[0] != "gemini" {
		t.Fatalf("targets = %+v", manifest.Targets)
	}
	if manifest.Version != "0.2.0" {
		t.Fatalf("version = %q", manifest.Version)
	}
	if _, err := os.Stat(filepath.Join(root, "targets", "gemini", "package.yaml")); err != nil {
		t.Fatalf("stat gemini package metadata: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "mcp", "servers.yaml")); err != nil {
		t.Fatalf("stat mcp servers: %v", err)
	}
	for _, rel := range []string{
		filepath.Join("targets", "gemini", "settings", "release-profile.yaml"),
		filepath.Join("targets", "gemini", "themes", "release-dawn.yaml"),
		filepath.Join("targets", "gemini", "manifest.extra.json"),
		filepath.Join("targets", "gemini", "commands", "release", "deploy.toml"),
		filepath.Join("targets", "gemini", "policies", "review.toml"),
		filepath.Join("targets", "gemini", "hooks", "hooks.json"),
	} {
		if _, err := os.Stat(filepath.Join(root, rel)); err != nil {
			t.Fatalf("stat %s: %v", rel, err)
		}
	}
	body, err := os.ReadFile(filepath.Join(root, "targets", "gemini", "package.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"context_file_name: TEAM.md", "exclude_tools:", "migrated_to: https://github.com/example/gemini-demo-v2", "plan_directory: .gemini/plans"} {
		if !strings.Contains(string(body), want) {
			t.Fatalf("package metadata missing %q:\n%s", want, body)
		}
	}
	if _, err := os.Stat(filepath.Join(root, "targets", "gemini", "contexts", "TEAM.md")); err != nil {
		t.Fatalf("stat imported custom primary context: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "targets", "gemini", "contexts", "RELEASE.md")); err != nil {
		t.Fatalf("stat imported extra Gemini context: %v", err)
	}
	extra, err := os.ReadFile(filepath.Join(root, "targets", "gemini", "manifest.extra.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(extra), `"x_galleryTopic": "gemini-cli-extension"`) || !strings.Contains(string(extra), `"retentionDays": 7`) {
		t.Fatalf("manifest extra = %s", extra)
	}
}

func TestImport_CurrentNativeGeminiRuntimeLayoutCreatesLauncher(t *testing.T) {
	root := t.TempDir()
	mustWritePluginFile(t, root, "gemini-extension.json", `{
	  "name":"demo-runtime",
	  "version":"0.2.0",
	  "description":"gemini runtime demo"
	}`)
	mustWritePluginFile(t, root, filepath.Join("hooks", "hooks.json"), `{
	  "hooks": {
	    "SessionStart": [{"matcher":"*","hooks":[{"type":"command","command":"${extensionPath}${/}bin${/}demo-runtime GeminiSessionStart"}]}],
	    "SessionEnd": [{"matcher":"*","hooks":[{"type":"command","command":"${extensionPath}${/}bin${/}demo-runtime GeminiSessionEnd"}]}],
	    "BeforeTool": [{"matcher":"*","hooks":[{"type":"command","command":"${extensionPath}${/}bin${/}demo-runtime GeminiBeforeTool"}]}],
	    "AfterTool": [{"matcher":"*","hooks":[{"type":"command","command":"${extensionPath}${/}bin${/}demo-runtime GeminiAfterTool"}]}]
	  }
	}`)

	manifest, _, err := Import(root, "gemini", false, false)
	if err != nil {
		t.Fatal(err)
	}
	if manifest.Name != "demo-runtime" {
		t.Fatalf("name = %q", manifest.Name)
	}
	body, err := os.ReadFile(filepath.Join(root, LauncherFileName))
	if err != nil {
		t.Fatalf("read launcher: %v", err)
	}
	if !strings.Contains(string(body), "runtime: go") {
		t.Fatalf("launcher missing runtime:\n%s", body)
	}
	if !strings.Contains(string(body), "entrypoint: ./bin/demo-runtime") {
		t.Fatalf("launcher missing entrypoint:\n%s", body)
	}
}

func TestImport_CurrentNativeGeminiRejectsMalformedManifestShapes(t *testing.T) {
	root := t.TempDir()
	mustWritePluginFile(t, root, "gemini-extension.json", `{
	  "name":"demo",
	  "version":"0.2.0",
	  "description":"gemini demo",
	  "excludeTools":"run_shell_command(rm -rf)"
	}`)

	if _, _, err := Import(root, "gemini", false, false); err == nil || !strings.Contains(err.Error(), `Gemini extension field "excludeTools" must be an array of strings`) {
		t.Fatalf("Import error = %v", err)
	}
}

func TestImport_CurrentNativeGeminiRejectsMalformedHooksFile(t *testing.T) {
	root := t.TempDir()
	mustWritePluginFile(t, root, "gemini-extension.json", `{
	  "name":"demo-runtime",
	  "version":"0.2.0",
	  "description":"gemini runtime demo"
	}`)
	mustWritePluginFile(t, root, filepath.Join("hooks", "hooks.json"), `{"hooks":[]}`)

	if _, _, err := Import(root, "gemini", false, false); err == nil || !strings.Contains(err.Error(), "Gemini hooks file must define a top-level hooks object") {
		t.Fatalf("Import error = %v", err)
	}
}

func TestImport_AmbiguousNativeLayoutsRequireExplicitFrom(t *testing.T) {
	root := t.TempDir()
	mustWritePluginFile(t, root, filepath.Join(".claude-plugin", "plugin.json"), `{"name":"demo","version":"0.1.0","description":"demo"}`)
	mustWritePluginFile(t, root, filepath.Join(".codex", "config.toml"), "model = \"gpt-5.4-mini\"\nnotify = [\"./bin/demo\", \"notify\"]\n")

	_, _, err := Import(root, "", false, false)
	if err == nil || !strings.Contains(err.Error(), "ambiguous import source") {
		t.Fatalf("Import error = %v", err)
	}
}

func TestRender_GeminiRejectsManifestExtraCanonicalOverride(t *testing.T) {
	root := t.TempDir()
	manifest := Default("demo-gemini", "gemini", "", "gemini demo", true)
	mustSavePackage(t, root, manifest, "")
	mustWritePluginFile(t, root, filepath.Join("targets", "gemini", "contexts", "GEMINI.md"), "# Gemini\n")
	mustWritePluginFile(t, root, filepath.Join("targets", "gemini", "manifest.extra.json"), `{"plan":{"directory":".gemini/other"}}`)
	if _, err := Render(root, "gemini"); err == nil || !strings.Contains(err.Error(), `gemini manifest.extra.json may not override canonical field "plan.directory"`) {
		t.Fatalf("Render error = %v", err)
	}
}

func TestRender_GeminiManifestParity(t *testing.T) {
	root := t.TempDir()
	manifest := Default("demo-gemini", "gemini", "", "gemini demo", true)
	mustSavePackage(t, root, manifest, "")
	mustWritePluginFile(t, root, filepath.Join("targets", "gemini", "contexts", "GEMINI.md"), "# Gemini\n")
	mustWritePortableMCPFile(t, root, `format: plugin-kit-ai/mcp
version: 1

servers:
  demo:
    type: stdio
    stdio:
      command: node
      args:
        - "${package.root}/server.mjs"
`)
	mustWritePluginFile(t, root, filepath.Join("targets", "gemini", "package.yaml"), "context_file_name: GEMINI.md\nexclude_tools:\n  - run_shell_command(rm -rf)\nmigrated_to: https://github.com/example/demo-gemini-v2\nplan_directory: .gemini/plans\n")
	mustWritePluginFile(t, root, filepath.Join("targets", "gemini", "settings", "release-profile.yaml"), "name: release-profile\ndescription: profile\nenv_var: RELEASE_PROFILE\nsensitive: false\n")
	mustWritePluginFile(t, root, filepath.Join("targets", "gemini", "themes", "release-dawn.yaml"), "name: release-dawn\nbackground:\n  primary: \"#fff9f2\"\ntext:\n  primary: \"#2e1f14\"\n")
	mustWritePluginFile(t, root, filepath.Join("targets", "gemini", "manifest.extra.json"), `{"x_galleryTopic":"gemini-cli-extension","plan":{"retentionDays":7}}`)
	mustWritePluginFile(t, root, filepath.Join("targets", "gemini", "commands", "deploy.toml"), "description = \"deploy\"\n")
	mustWritePluginFile(t, root, filepath.Join("targets", "gemini", "hooks", "hooks.json"), "{\"hooks\":{}}\n")
	mustWritePluginFile(t, root, filepath.Join("targets", "gemini", "contexts", "RELEASE.md"), "# Release\n")
	result, err := Render(root, "gemini")
	if err != nil {
		t.Fatal(err)
	}
	if err := WriteArtifacts(root, result.Artifacts); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(filepath.Join(root, "gemini-extension.json"))
	if err != nil {
		t.Fatal(err)
	}
	var rendered map[string]any
	if err := json.Unmarshal(body, &rendered); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"mcpServers", "excludeTools", "migratedTo", "plan", "settings", "themes", "contextFileName", "x_galleryTopic"} {
		if _, ok := rendered[key]; !ok {
			t.Fatalf("rendered manifest missing %q: %s", key, body)
		}
	}
	plan := rendered["plan"].(map[string]any)
	if plan["directory"] != ".gemini/plans" || plan["retentionDays"] != float64(7) {
		t.Fatalf("plan = %#v", plan)
	}
	if rendered["migratedTo"] != "https://github.com/example/demo-gemini-v2" {
		t.Fatalf("migratedTo = %#v", rendered["migratedTo"])
	}
	if _, err := os.Stat(filepath.Join(root, "commands", "deploy.toml")); err != nil {
		t.Fatalf("stat generated command: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "GEMINI.md")); err != nil {
		t.Fatalf("stat generated primary context: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "contexts", "RELEASE.md")); err != nil {
		t.Fatalf("stat generated extra context: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "hooks", "hooks.json")); err != nil {
		t.Fatalf("stat generated hooks: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "settings", "release-profile.yaml")); !os.IsNotExist(err) {
		t.Fatalf("settings should be rendered into manifest, err=%v", err)
	}
}

func TestRender_GeminiRuntimeGeneratesDefaultHooksFromLauncher(t *testing.T) {
	root := t.TempDir()
	manifest := Default("demo-gemini", "gemini", "go", "gemini demo", true)
	mustSavePackage(t, root, manifest, "go")
	mustWritePluginFile(t, root, LauncherFileName, "runtime: go\nentrypoint: ./bin/demo-gemini\n")
	mustWritePluginFile(t, root, filepath.Join("targets", "gemini", "contexts", "GEMINI.md"), "# Gemini\n")

	result, err := Render(root, "gemini")
	if err != nil {
		t.Fatal(err)
	}
	if err := WriteArtifacts(root, result.Artifacts); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(filepath.Join(root, "hooks", "hooks.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"${extensionPath}${/}bin${/}demo-gemini GeminiSessionStart",
		"${extensionPath}${/}bin${/}demo-gemini GeminiSessionEnd",
		"${extensionPath}${/}bin${/}demo-gemini GeminiBeforeTool",
		"${extensionPath}${/}bin${/}demo-gemini GeminiAfterTool",
	} {
		if !strings.Contains(string(body), want) {
			t.Fatalf("generated hooks missing %q:\n%s", want, body)
		}
	}
}

func TestRender_GeminiRejectsHookEntrypointMismatch(t *testing.T) {
	root := t.TempDir()
	manifest := Default("demo-gemini", "gemini", "go", "gemini demo", true)
	mustSavePackage(t, root, manifest, "go")
	mustWritePluginFile(t, root, LauncherFileName, "runtime: go\nentrypoint: ./bin/demo-gemini\n")
	mustWritePluginFile(t, root, filepath.Join("targets", "gemini", "contexts", "GEMINI.md"), "# Gemini\n")
	mustWritePluginFile(t, root, filepath.Join("targets", "gemini", "hooks", "hooks.json"), `{
  "hooks": {
    "SessionStart": [{"matcher":"*","hooks":[{"type":"command","command":"${extensionPath}${/}bin${/}other GeminiSessionStart"}]}],
    "SessionEnd": [{"matcher":"*","hooks":[{"type":"command","command":"${extensionPath}${/}bin${/}demo-gemini GeminiSessionEnd"}]}],
    "BeforeTool": [{"matcher":"*","hooks":[{"type":"command","command":"${extensionPath}${/}bin${/}demo-gemini GeminiBeforeTool"}]}],
    "AfterTool": [{"matcher":"*","hooks":[{"type":"command","command":"${extensionPath}${/}bin${/}demo-gemini GeminiAfterTool"}]}]
  }
}`)

	if _, err := Render(root, "gemini"); err == nil || !strings.Contains(err.Error(), `entrypoint mismatch: Gemini hook "SessionStart"`) {
		t.Fatalf("Render error = %v", err)
	}
}

func TestRender_GeminiRejectsMalformedStructuredSettingsAndThemes(t *testing.T) {
	root := t.TempDir()
	manifest := Default("demo-gemini", "gemini", "", "gemini demo", true)
	mustSavePackage(t, root, manifest, "")
	mustWritePluginFile(t, root, filepath.Join("targets", "gemini", "contexts", "GEMINI.md"), "# Gemini\n")
	mustWritePluginFile(t, root, filepath.Join("targets", "gemini", "settings", "broken.yaml"), "name: broken\n")

	if _, err := Render(root, "gemini"); err == nil || !strings.Contains(err.Error(), "Gemini settings require") {
		t.Fatalf("Render error = %v", err)
	}

	mustWritePluginFile(t, root, filepath.Join("targets", "gemini", "settings", "broken.yaml"), "name: fixed\ndescription: desc\nenv_var: FIXED\nsensitive: false\n")
	mustWritePluginFile(t, root, filepath.Join("targets", "gemini", "themes", "broken.yaml"), "name: broken\n")
	if _, err := Render(root, "gemini"); err == nil || !strings.Contains(err.Error(), "Gemini themes require at least one theme token besides name") {
		t.Fatalf("Render error = %v", err)
	}
}

func TestRender_GeminiRejectsInvalidThemeObjectShapeAndDuplicateSettings(t *testing.T) {
	root := t.TempDir()
	manifest := Default("demo-gemini", "gemini", "", "gemini demo", true)
	mustSavePackage(t, root, manifest, "")
	mustWritePluginFile(t, root, filepath.Join("targets", "gemini", "contexts", "GEMINI.md"), "# Gemini\n")
	mustWritePluginFile(t, root, filepath.Join("targets", "gemini", "themes", "broken.yaml"), "name: release-dawn\nbackground: \"#fff9f2\"\n")
	if _, err := Render(root, "gemini"); err == nil || !strings.Contains(err.Error(), `Gemini theme key "background" must be a YAML object`) {
		t.Fatalf("Render error = %v", err)
	}

	mustWritePluginFile(t, root, filepath.Join("targets", "gemini", "themes", "broken.yaml"), "name: release-dawn\nbackground:\n  primary: \"#fff9f2\"\n")
	mustWritePluginFile(t, root, filepath.Join("targets", "gemini", "settings", "first.yaml"), "name: release-profile\ndescription: one\nenv_var: RELEASE_PROFILE\nsensitive: false\n")
	mustWritePluginFile(t, root, filepath.Join("targets", "gemini", "settings", "second.yaml"), "name: duplicate\ndescription: two\nenv_var: RELEASE_PROFILE\nsensitive: false\n")
	if _, err := Render(root, "gemini"); err == nil || !strings.Contains(err.Error(), `duplicates`) {
		t.Fatalf("Render error = %v", err)
	}
}

func TestRender_GeminiRejectsInvalidMCPTransportAndExcludeTools(t *testing.T) {
	root := t.TempDir()
	manifest := Default("demo-gemini", "gemini", "", "gemini demo", true)
	mustSavePackage(t, root, manifest, "")
	mustWritePluginFile(t, root, filepath.Join("targets", "gemini", "contexts", "GEMINI.md"), "# Gemini\n")
	mustWritePortableMCPFile(t, root, `format: plugin-kit-ai/mcp
version: 1

servers:
  demo:
    type: stdio
    stdio:
      command: node
    remote:
      protocol: sse
      url: "https://example.com/sse"
`)
	if _, err := Render(root, "gemini"); err == nil || !strings.Contains(err.Error(), "type stdio may not define remote config") {
		t.Fatalf("Render error = %v", err)
	}

	mustWritePortableMCPFile(t, root, `format: plugin-kit-ai/mcp
version: 1

servers:
  demo:
    type: stdio
    stdio:
      command: node
      args:
        - server.mjs
`)
	mustWritePluginFile(t, root, filepath.Join("targets", "gemini", "package.yaml"), "exclude_tools:\n  - \"\"\n")
	if _, err := Render(root, "gemini"); err == nil || !strings.Contains(err.Error(), "exclude_tools entries must be non-empty strings") {
		t.Fatalf("Render error = %v", err)
	}
}

func TestDiscover_RejectsLegacyPortableMCPAuthoredPaths(t *testing.T) {
	root := t.TempDir()
	manifest := Default("demo", "cursor", "", "demo plugin", false)
	mustSavePackage(t, root, manifest, "")
	mustWritePluginFile(t, root, filepath.Join("mcp", "servers.json"), `{"demo":{"command":"node","args":["server.mjs"]}}`)

	_, _, err := Discover(root)
	if err == nil || !strings.Contains(err.Error(), "unsupported portable MCP authored path mcp/servers.json") {
		t.Fatalf("Discover error = %v", err)
	}
}

func TestImport_RejectsLegacyInternalProjectManifest(t *testing.T) {
	root := t.TempDir()
	mustWritePluginFile(t, root, filepath.Join(".plugin-kit-ai", "project.toml"), "schema_version = 1\n")
	_, _, err := Import(root, "", false, false)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), ".plugin-kit-ai/project.toml is not supported") {
		t.Fatalf("error = %q", err)
	}
}

func TestAnalyze_RejectsLegacySchemaVersion(t *testing.T) {
	body := []byte(`
schema_version: 1
name: "demo"
version: "0.1.0"
description: "demo"
runtime: "go"
entrypoint: "./bin/demo"
targets:
  enabled: ["codex"]
`)
	_, _, err := Analyze(body)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "unsupported plugin.yaml format: schema_version-based manifests are not supported") {
		t.Fatalf("error = %q", err)
	}
}

func TestAnalyze_RejectsInvalidGeminiExtensionName(t *testing.T) {
	body := []byte(`
format: plugin-kit-ai/package
name: "Demo_Extension"
version: "0.1.0"
description: "demo"
targets: ["gemini"]
`)
	_, _, err := Analyze(body)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "invalid Gemini extension name") {
		t.Fatalf("error = %q", err)
	}
}

func TestAnalyze_WarnsOnUnknownFields(t *testing.T) {
	body := []byte(`
format: plugin-kit-ai/package
name: "demo"
version: "0.1.0"
description: "demo"
targets: ["claude"]
nonsense: true
`)
	_, warnings, err := Analyze(body)
	if err != nil {
		t.Fatal(err)
	}
	if len(warnings) != 1 {
		t.Fatalf("warnings = %+v", warnings)
	}
	var foundUnknown bool
	for _, warning := range warnings {
		if warning.Path == "nonsense" {
			foundUnknown = warning.Kind == WarningUnknownField
		}
	}
	if !foundUnknown {
		t.Fatalf("warnings = %+v", warnings)
	}
}

func TestAnalyze_RejectsLegacyComponentsInventory(t *testing.T) {
	body := []byte(`
format: plugin-kit-ai/package
name: "demo"
version: "0.1.0"
description: "demo"
targets: ["claude"]
components:
  hooks: []
`)
	_, _, err := Analyze(body)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "unsupported plugin.yaml format: flat components inventory is not supported") {
		t.Fatalf("error = %q", err)
	}
}

func TestInspect_ReturnsTargetCoverage(t *testing.T) {
	root := t.TempDir()
	manifest := Default("demo", "claude", "go", "demo plugin", true)
	manifest.Targets = []string{"claude", "gemini"}
	mustSavePackage(t, root, manifest, "go")
	if err := os.MkdirAll(filepath.Join(root, "targets", "claude", "hooks"), 0o755); err != nil {
		t.Fatal(err)
	}
	mustWritePluginFile(t, root, filepath.Join("targets", "claude", "hooks", "hooks.json"), "{}\n")
	inspection, _, err := Inspect(root, "all")
	if err == nil {
		if len(inspection.Targets) != 2 {
			t.Fatalf("targets = %+v", inspection.Targets)
		}
		return
	}
	t.Fatal(err)
}

func TestInspect_IncludesTargetLifecycleMetadata(t *testing.T) {
	root := t.TempDir()
	manifest := Default("gemini-inspect", "gemini", "", "gemini inspect", true)
	mustSavePackage(t, root, manifest, "")
	mustWritePluginFile(t, root, filepath.Join("targets", "gemini", "contexts", "GEMINI.md"), "# Gemini\n")

	inspection, _, err := Inspect(root, "gemini")
	if err != nil {
		t.Fatal(err)
	}
	if len(inspection.Targets) != 1 {
		t.Fatalf("targets = %+v", inspection.Targets)
	}
	target := inspection.Targets[0]
	if target.TargetNoun != "extension" || target.InstallModel != "copy install" || target.DevModel != "link" || target.ActivationModel != "restart required" {
		t.Fatalf("target lifecycle metadata = %+v", target)
	}
	if target.NativeRoot != "~/.gemini/extensions/<name>" {
		t.Fatalf("native root = %q", target.NativeRoot)
	}
}

func TestInspect_CodexIncludesExtraDocKinds(t *testing.T) {
	root := t.TempDir()
	manifest := Default("codex-inspect", "codex-runtime", "go", "codex inspect", true)
	manifest.Targets = []string{"codex-package", "codex-runtime"}
	mustSavePackage(t, root, manifest, "go")
	mustWritePluginFile(t, root, filepath.Join("targets", "codex-package", "manifest.extra.json"), `{"homepage":"https://example.com"}`)
	mustWritePluginFile(t, root, filepath.Join("targets", "codex-package", "interface.json"), `{"defaultPrompt":["Inspect"]}`)
	mustWritePluginFile(t, root, filepath.Join("targets", "codex-runtime", "config.extra.toml"), "approval_policy = \"never\"\n")

	inspection, _, err := Inspect(root, "all")
	if err != nil {
		t.Fatal(err)
	}
	if len(inspection.Targets) != 2 {
		t.Fatalf("targets = %+v", inspection.Targets)
	}
	for _, target := range inspection.Targets {
		switch target.Target {
		case "codex-package":
			if !slices.Contains(target.TargetNativeKinds, "manifest_extra") {
				t.Fatalf("codex-package target_native_kinds = %v", target.TargetNativeKinds)
			}
			if !slices.Contains(target.TargetNativeKinds, "interface") {
				t.Fatalf("codex-package target_native_kinds = %v", target.TargetNativeKinds)
			}
			if got := target.NativeDocPaths["interface"]; got != filepath.Join("targets", "codex-package", "interface.json") {
				t.Fatalf("codex-package native_doc_paths[interface] = %q", got)
			}
			if got := target.NativeDocPaths["manifest_extra"]; got != filepath.Join("targets", "codex-package", "manifest.extra.json") {
				t.Fatalf("codex-package native_doc_paths[manifest_extra] = %q", got)
			}
			if got := target.NativeSurfaceTiers["interface"]; got != "stable" {
				t.Fatalf("codex-package native_surface_tiers[interface] = %q", got)
			}
			if got := target.NativeSurfaceTiers["app_manifest"]; got != "beta" {
				t.Fatalf("codex-package native_surface_tiers[app_manifest] = %q", got)
			}
		case "codex-runtime":
			if !slices.Contains(target.TargetNativeKinds, "config_extra") {
				t.Fatalf("codex-runtime target_native_kinds = %v", target.TargetNativeKinds)
			}
			if got := target.NativeDocPaths["config_extra"]; got != filepath.Join("targets", "codex-runtime", "config.extra.toml") {
				t.Fatalf("codex-runtime native_doc_paths[config_extra] = %q", got)
			}
			if got := target.NativeSurfaceTiers["config_extra"]; got != "stable" {
				t.Fatalf("codex-runtime native_surface_tiers[config_extra] = %q", got)
			}
			if got := target.NativeSurfaceTiers["commands"]; got != "beta" {
				t.Fatalf("codex-runtime native_surface_tiers[commands] = %q", got)
			}
		default:
			t.Fatalf("unexpected target %+v", target)
		}
	}
}

func TestDiscover_RejectsRemovedPortableContexts(t *testing.T) {
	root := t.TempDir()
	manifest := Default("demo-gemini", "gemini", "", "gemini demo", true)
	mustSavePackage(t, root, manifest, "")
	mustWritePluginFile(t, root, filepath.Join("contexts", "GEMINI.md"), "# Gemini\n")

	_, _, err := Discover(root)
	if err == nil || !strings.Contains(err.Error(), "portable contexts were removed") {
		t.Fatalf("Discover error = %v", err)
	}
}

func TestInspect_ExposesSurfaceTiers(t *testing.T) {
	root := t.TempDir()
	manifest := Default("demo", "claude", "go", "demo plugin", true)
	mustSavePackage(t, root, manifest, "go")
	mustWritePluginFile(t, root, filepath.Join("targets", "claude", "hooks", "hooks.json"), "{\"hooks\":{}}\n")

	inspection, _, err := Inspect(root, "claude")
	if err != nil {
		t.Fatal(err)
	}
	if len(inspection.Targets) != 1 {
		t.Fatalf("targets = %+v", inspection.Targets)
	}
	target := inspection.Targets[0]
	if got := target.NativeSurfaceTiers["agents"]; got != "beta" {
		t.Fatalf("native_surface_tiers[agents] = %q", got)
	}
	if got := target.NativeSurfaceTiers["contexts"]; got != "unsupported" {
		t.Fatalf("native_surface_tiers[contexts] = %q", got)
	}
	var foundAgents, foundContexts bool
	for _, surface := range target.NativeSurfaces {
		switch {
		case surface.Kind == "agents" && surface.Tier == "beta":
			foundAgents = true
		case surface.Kind == "contexts" && surface.Tier == "unsupported":
			foundContexts = true
		}
	}
	if !foundAgents || !foundContexts {
		t.Fatalf("native_surfaces = %+v", target.NativeSurfaces)
	}
}

func TestInspect_OpenCodeExposesWorkspaceSurfaceTiers(t *testing.T) {
	root := t.TempDir()
	manifest := Default("demo", "opencode", "", "demo plugin", false)
	mustSavePackage(t, root, manifest, "")
	mustWritePluginFile(t, root, filepath.Join("targets", "opencode", "package.yaml"), "plugins:\n  - \"@acme/demo-opencode\"\n")

	inspection, _, err := Inspect(root, "opencode")
	if err != nil {
		t.Fatal(err)
	}
	if len(inspection.Targets) != 1 {
		t.Fatalf("targets = %+v", inspection.Targets)
	}
	target := inspection.Targets[0]
	if got := target.NativeSurfaceTiers["commands"]; got != "stable" {
		t.Fatalf("native_surface_tiers[commands] = %q", got)
	}
	if got := target.NativeSurfaceTiers["tools"]; got != "beta" {
		t.Fatalf("native_surface_tiers[tools] = %q", got)
	}
	var foundAgentConfig, foundPermissionConfig, foundInstructionsConfig, foundCommands, foundAgents, foundThemes, foundTools, foundModes bool
	var foundLocalPluginCode, foundCustomTools, foundLocalPluginDependencies bool
	for _, surface := range target.NativeSurfaces {
		switch {
		case surface.Kind == "agent_config" && surface.Tier == "passthrough_only":
			foundAgentConfig = true
		case surface.Kind == "permission_config" && surface.Tier == "passthrough_only":
			foundPermissionConfig = true
		case surface.Kind == "instructions_config" && surface.Tier == "passthrough_only":
			foundInstructionsConfig = true
		case surface.Kind == "commands" && surface.Tier == "stable":
			foundCommands = true
		case surface.Kind == "agents" && surface.Tier == "stable":
			foundAgents = true
		case surface.Kind == "themes" && surface.Tier == "stable":
			foundThemes = true
		case surface.Kind == "tools" && surface.Tier == "beta":
			foundTools = true
		case surface.Kind == "modes" && surface.Tier == "unsupported":
			foundModes = true
		case surface.Kind == "local_plugin_code" && surface.Tier == "stable":
			foundLocalPluginCode = true
		case surface.Kind == "custom_tools" && surface.Tier == "beta":
			foundCustomTools = true
		case surface.Kind == "local_plugin_dependencies" && surface.Tier == "stable":
			foundLocalPluginDependencies = true
		}
	}
	if !foundAgentConfig || !foundPermissionConfig || !foundInstructionsConfig || !foundCommands || !foundAgents || !foundThemes || !foundTools || !foundModes || !foundLocalPluginCode || !foundCustomTools || !foundLocalPluginDependencies {
		t.Fatalf("native_surfaces = %+v", target.NativeSurfaces)
	}
}

func TestNormalize_RewritesManifestIntoPackageStandardShape(t *testing.T) {
	root := t.TempDir()
	mustWritePluginFile(t, root, FileName, `format: plugin-kit-ai/package
name: "demo"
version: "0.1.0"
description: "demo"
targets: ["codex-runtime"]
extra_field: true
`)
	mustWritePluginFile(t, root, LauncherFileName, "runtime: go\nentrypoint: ./bin/demo\n")
	warnings, err := Normalize(root, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(warnings) != 1 {
		t.Fatalf("warnings = %+v", warnings)
	}
	body, err := os.ReadFile(filepath.Join(root, FileName))
	if err != nil {
		t.Fatal(err)
	}
	text := string(body)
	for _, unwanted := range []string{"extra_field"} {
		if strings.Contains(text, unwanted) {
			t.Fatalf("normalized manifest still contains %q:\n%s", unwanted, text)
		}
	}
}

func TestImport_WarnsOnIgnoredAssets(t *testing.T) {
	root := t.TempDir()
	mustWritePluginFile(t, root, filepath.Join(".codex", "config.toml"), "model = \"gpt-5.4-mini\"\nnotify = [\"./bin/demo\", \"notify\"]\n")
	mustWritePluginFile(t, root, filepath.Join(".codex-plugin", "plugin.json"), `{"name":"demo","version":"0.1.0","description":"demo"}`)
	mustWritePluginFile(t, root, filepath.Join("scripts", "main.sh"), "#!/usr/bin/env bash\n")
	mustWritePluginFile(t, root, ".mcp.json", `{"demo":{"command":"node","args":["server.mjs"]}}`)
	mustWritePluginFile(t, root, filepath.Join("agents", "reviewer.md"), "reviewer\n")

	_, warnings, err := Import(root, "codex-native", false, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(warnings) < 2 {
		t.Fatalf("warnings = %+v", warnings)
	}
	var foundMCP, foundAgents bool
	for _, warning := range warnings {
		if warning.Path == ".mcp.json" {
			foundMCP = true
		}
		if warning.Path == "agents" {
			foundAgents = true
		}
	}
	if !foundMCP || !foundAgents {
		t.Fatalf("warnings = %+v", warnings)
	}
}

func TestImport_RejectsLegacyCodexSource(t *testing.T) {
	root := t.TempDir()
	mustWritePluginFile(t, root, filepath.Join(".codex", "config.toml"), "model = \"gpt-5.4-mini\"\nnotify = [\"./bin/demo\", \"notify\"]\n")
	mustWritePluginFile(t, root, filepath.Join(".codex-plugin", "plugin.json"), `{"name":"demo","version":"0.1.0","description":"demo"}`)

	_, _, err := Import(root, "codex", false, false)
	if err == nil || !strings.Contains(err.Error(), `unsupported import source "codex"`) {
		t.Fatalf("Import error = %v", err)
	}
}

func containsWarning(warnings []Warning, needle string) bool {
	for _, warning := range warnings {
		if strings.Contains(warning.Message, needle) {
			return true
		}
	}
	return false
}

func mustWritePluginFile(t *testing.T, root, rel, body string) {
	t.Helper()
	full := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func mustWritePortableMCPFile(t *testing.T, root, body string) {
	t.Helper()
	mustWritePluginFile(t, root, filepath.Join("mcp", "servers.yaml"), body)
}

func mustSavePackage(t *testing.T, root string, manifest Manifest, runtime string) {
	t.Helper()
	if err := Save(root, manifest, false); err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(runtime) != "" {
		if err := SaveLauncher(root, DefaultLauncher(manifest.Name, runtime), false); err != nil {
			t.Fatal(err)
		}
	}
}
