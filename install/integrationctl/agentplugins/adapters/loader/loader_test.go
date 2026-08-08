package loader

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/777genius/plugin-kit-ai/install/integrationctl/agentplugins/adapters/specregistry"
	"github.com/777genius/plugin-kit-ai/install/integrationctl/agentplugins/domain"
)

func TestLoadMinimalPlugin(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	manifest := `{"$schema":"https://agent-plugins.org/schemas/1.0.0/plugin.schema.json","name":"minimal"}`
	writeLoaderFile(t, filepath.Join(root, "plugin.json"), manifest)

	envelope, err := testLoader(t).Load(context.Background(), domain.LoadInput{SnapshotRoot: root, TreeDigest: "sha256:tree"})
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if envelope.LoaderKind != domain.LoaderKindAgentPlugins || envelope.FormatID != domain.FormatIDAgentPluginsV1 {
		t.Fatalf("format identity = %s %s", envelope.LoaderKind, envelope.FormatID)
	}
	if envelope.Manifest.Name != "minimal" || envelope.Manifest.Version != "" {
		t.Fatalf("manifest = %+v", envelope.Manifest)
	}
	if string(envelope.Manifest.Raw) != manifest {
		t.Fatalf("raw manifest changed: %q", envelope.Manifest.Raw)
	}
	if envelope.MCP.Present || len(envelope.Skills) != 0 {
		t.Fatalf("unexpected components: %+v %+v", envelope.MCP, envelope.Skills)
	}
}

func TestLoadFullPluginPreservesExtensionsAndUnknownFields(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeLoaderFile(t, filepath.Join(root, "plugin.json"), `{
  "$schema":"https://agent-plugins.org/schemas/1.0.0/plugin.schema.json",
  "name":"full-plugin",
  "version":"release-candidate",
  "description":"full",
  "author":{"name":"Example"},
  "keywords":["one"],
  "extensions":{"com.example.good":{"enabled":true},"com.example.future":"opaque"},
  "futureField":{"kept":true}
}`)

	envelope, err := testLoader(t).Load(context.Background(), domain.LoadInput{SnapshotRoot: root})
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if envelope.Manifest.Version != "release-candidate" {
		t.Fatalf("opaque version changed: %q", envelope.Manifest.Version)
	}
	if _, ok := envelope.Manifest.Unknown["futureField"]; !ok {
		t.Fatal("unknown field was not preserved")
	}
	if len(envelope.Manifest.Extensions) != 2 || len(envelope.Manifest.RawExtensions) == 0 {
		t.Fatalf("extensions were not preserved: %+v", envelope.Manifest.Extensions)
	}
	if !hasDiagnostic(envelope.Diagnostics, "plugin_unknown_field") || !hasDiagnostic(envelope.Diagnostics, "extension_value_ignored") {
		t.Fatalf("diagnostics = %+v", envelope.Diagnostics)
	}
}

func TestLoadRejectsInvalidAndUnsupportedPluginManifests(t *testing.T) {
	t.Parallel()
	tests := map[string]string{
		"invalid":     `{"$schema":"https://agent-plugins.org/schemas/1.0.0/plugin.schema.json","name":"Invalid Name"}`,
		"unsupported": `{"$schema":"https://agent-plugins.org/schemas/2.0.0/plugin.schema.json","name":"future"}`,
		"malformed":   `{`,
	}
	for name, manifest := range tests {
		name, manifest := name, manifest
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			writeLoaderFile(t, filepath.Join(root, "plugin.json"), manifest)
			_, err := testLoader(t).Load(context.Background(), domain.LoadInput{SnapshotRoot: root})
			var loadErr *domain.LoadError
			if !errors.As(err, &loadErr) || loadErr.Diagnostic.Boundary != domain.BoundaryPlugin {
				t.Fatalf("error = %v, want plugin LoadError", err)
			}
		})
	}
}

func TestLoadMalformedMCPDisablesOnlyMCP(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeMinimalPlugin(t, root, "mcp-malformed")
	writeLoaderFile(t, filepath.Join(root, "mcp.json"), `{`)

	envelope, err := testLoader(t).Load(context.Background(), domain.LoadInput{SnapshotRoot: root})
	if err != nil {
		t.Fatalf("load plugin: %v", err)
	}
	if !envelope.MCP.Present || envelope.MCP.Enabled || !hasDiagnostic(envelope.Diagnostics, "mcp_malformed") {
		t.Fatalf("MCP boundary = %+v diagnostics=%+v", envelope.MCP, envelope.Diagnostics)
	}
}

func TestLoadSkipsOneInvalidMCPServer(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeMinimalPlugin(t, root, "mixed-mcp")
	writeLoaderFile(t, filepath.Join(root, "mcp.json"), `{
  "$schema":"https://agent-plugins.org/schemas/1.0.0/mcp.schema.json",
  "mcpServers":{
    "valid":{"type":"stdio","command":"npx","args":["server"]},
    "invalid":{"type":"stdio","args":["missing-command"]}
  }
}`)

	envelope, err := testLoader(t).Load(context.Background(), domain.LoadInput{SnapshotRoot: root})
	if err != nil {
		t.Fatal(err)
	}
	if !envelope.MCP.Enabled || len(envelope.MCP.Servers) != 1 || envelope.MCP.Servers["valid"].Type != "stdio" {
		t.Fatalf("valid MCP servers = %+v", envelope.MCP.Servers)
	}
	if _, ok := envelope.MCP.InvalidServer["invalid"]; !ok || !hasDiagnostic(envelope.Diagnostics, "mcp_server_invalid") {
		t.Fatalf("invalid MCP server was not isolated: %+v", envelope.MCP)
	}
}

func TestLoadSkipsInvalidSkillAndKeepsValidSkill(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeMinimalPlugin(t, root, "skills-plugin")
	writeLoaderFile(t, filepath.Join(root, "skills", "valid-skill", "SKILL.md"), "---\nname: valid-skill\ndescription: Useful test skill\nmetadata:\n  owner: example\n---\n\n# Valid\n")
	writeLoaderFile(t, filepath.Join(root, "skills", "invalid-skill", "SKILL.md"), "---\nname: another-name\ndescription: mismatch\n---\n")

	envelope, err := testLoader(t).Load(context.Background(), domain.LoadInput{SnapshotRoot: root})
	if err != nil {
		t.Fatal(err)
	}
	if len(envelope.Skills) != 1 || envelope.Skills["valid-skill"].Metadata["owner"] != "example" {
		t.Fatalf("skills = %+v", envelope.Skills)
	}
	if len(envelope.Inventory.InvalidSkills) != 1 || envelope.Inventory.InvalidSkills[0] != "invalid-skill" || !hasDiagnostic(envelope.Diagnostics, "skill_invalid") {
		t.Fatalf("skill diagnostics = %+v inventory=%+v", envelope.Diagnostics, envelope.Inventory)
	}
}

func TestLoadDistinguishesInvalidSkillsRootFromSkillNamedSkills(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeMinimalPlugin(t, root, "skills-plugin")
	writeLoaderFile(t, filepath.Join(root, "skills", "skills", "SKILL.md"), "invalid\n")
	envelope, err := testLoader(t).Load(context.Background(), domain.LoadInput{SnapshotRoot: root})
	if err != nil {
		t.Fatal(err)
	}
	if envelope.Inventory.InvalidSkillsRoot || len(envelope.Inventory.InvalidSkills) != 1 || envelope.Inventory.InvalidSkills[0] != "skills" {
		t.Fatalf("inventory = %+v", envelope.Inventory)
	}

	rootFile := t.TempDir()
	writeMinimalPlugin(t, rootFile, "skills-root-plugin")
	writeLoaderFile(t, filepath.Join(rootFile, "skills"), "not a directory")
	envelope, err = testLoader(t).Load(context.Background(), domain.LoadInput{SnapshotRoot: rootFile})
	if err != nil {
		t.Fatal(err)
	}
	if !envelope.Inventory.InvalidSkillsRoot || len(envelope.Inventory.InvalidSkills) != 0 {
		t.Fatalf("root inventory = %+v", envelope.Inventory)
	}
}

func TestLoadNeverMergesLegacyPluginYAML(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeMinimalPlugin(t, root, "standard-name")
	writeLoaderFile(t, filepath.Join(root, "plugin", "plugin.yaml"), "api_version: v1\nname: legacy-name\nversion: 9.9.9\ndescription: legacy\ntargets:\n  - cursor\n")

	envelope, err := testLoader(t).Load(context.Background(), domain.LoadInput{SnapshotRoot: root})
	if err != nil {
		t.Fatal(err)
	}
	if envelope.Manifest.Name != "standard-name" || envelope.Manifest.Version != "" {
		t.Fatalf("legacy manifest leaked into envelope: %+v", envelope.Manifest)
	}
	encoded, err := json.Marshal(envelope)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "legacy-name") {
		t.Fatalf("legacy content leaked into envelope JSON: %s", encoded)
	}
}

func testLoader(t *testing.T) Loader {
	t.Helper()
	registry, err := specregistry.New()
	if err != nil {
		t.Fatalf("schema registry: %v", err)
	}
	return Loader{Registry: registry}
}

func writeMinimalPlugin(t *testing.T, root, name string) {
	t.Helper()
	writeLoaderFile(t, filepath.Join(root, "plugin.json"), `{"$schema":"https://agent-plugins.org/schemas/1.0.0/plugin.schema.json","name":"`+name+`"}`)
}

func writeLoaderFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func hasDiagnostic(diagnostics []domain.Diagnostic, code string) bool {
	for _, diagnostic := range diagnostics {
		if diagnostic.Code == code {
			return true
		}
	}
	return false
}
