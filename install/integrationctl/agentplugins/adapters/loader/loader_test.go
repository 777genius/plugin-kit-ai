package loader

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
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

func TestLoadPortablePackageDoesNotInferNativeAppBinding(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeMinimalPlugin(t, root, "portable-app")
	app := `{"apps":{"docs":{"id":"asdk_app_docs_123","required":true,"future":{"kept":true}}}}`
	writeLoaderFile(t, filepath.Join(root, ".app.json"), app)

	envelope, err := testLoader(t).Load(context.Background(), domain.LoadInput{SnapshotRoot: root})
	if err != nil {
		t.Fatal(err)
	}
	if envelope.App.Present || envelope.App.Enabled || envelope.Inventory.AppPresent || len(envelope.Inventory.AppBindings) != 0 {
		t.Fatalf("portable package inferred a native app binding: app=%+v inventory=%+v", envelope.App, envelope.Inventory)
	}
}

func TestLoadOfficialOpenAIPackageWithAppAndMCP(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	manifest := `{
  "name":"official-demo",
  "version":"1.2.3",
  "skills":"./skills/",
  "mcpServers":"./.mcp.json",
  "apps":"./.app.json",
  "interface":{"displayName":"Official Demo"},
  "future":{"preserved":true}
}`
	writeLoaderFile(t, filepath.Join(root, ".codex-plugin", "plugin.json"), manifest)
	writeLoaderFile(t, filepath.Join(root, ".mcp.json"), `{"mcp_servers":{"docs":{"url":"https://example.test/mcp"}}}`)
	writeLoaderFile(t, filepath.Join(root, ".app.json"), `{"apps":{"docs":{"id":"plugin_asdk_app_docs_123"}}}`)
	writeLoaderFile(t, filepath.Join(root, "skills", "docs", "SKILL.md"), "---\nname: docs\ndescription: Docs workflow\n---\nUse docs.\n")

	envelope, err := testOpenAILoader(t).Load(context.Background(), domain.LoadInput{SnapshotRoot: root, TreeDigest: "sha256:tree"})
	if err != nil {
		t.Fatal(err)
	}
	if envelope.FormatID != domain.FormatIDOpenAIPlugin || envelope.SchemaURI != "" || envelope.Manifest.Name != "official-demo" {
		t.Fatalf("official identity = %+v", envelope)
	}
	if string(envelope.Manifest.Raw) != manifest || envelope.Manifest.Unknown["future"] == nil {
		t.Fatal("official manifest fields were not preserved")
	}
	if envelope.MCP.Servers["docs"].Type != "streamable-http" || !envelope.App.Enabled || len(envelope.Skills) != 1 {
		t.Fatalf("official components = mcp=%+v app=%+v skills=%+v", envelope.MCP, envelope.App, envelope.Skills)
	}
}

func TestLoadOfficialPackageReportsMissingDeclaredApp(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeLoaderFile(t, filepath.Join(root, ".codex-plugin", "plugin.json"), `{"name":"missing-app","apps":"./.app.json"}`)
	envelope, err := testOpenAILoader(t).Load(context.Background(), domain.LoadInput{SnapshotRoot: root})
	if err != nil {
		t.Fatal(err)
	}
	if !envelope.App.Declared || envelope.App.Enabled || !hasDiagnostic(envelope.Diagnostics, "app_manifest_missing") {
		t.Fatalf("app boundary = %+v diagnostics=%+v", envelope.App, envelope.Diagnostics)
	}
}

func TestPortableManifestTakesPrecedenceOverOfficialManifest(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeMinimalPlugin(t, root, "portable-wins")
	writeLoaderFile(t, filepath.Join(root, ".codex-plugin", "plugin.json"), `{"name":"official-loses","apps":"./.app.json"}`)

	envelope, err := testLoader(t).Load(context.Background(), domain.LoadInput{SnapshotRoot: root})
	if err != nil {
		t.Fatal(err)
	}
	if envelope.FormatID != domain.FormatIDAgentPluginsV1 || envelope.Manifest.Name != "portable-wins" || envelope.App.Declared {
		t.Fatalf("portable precedence = %+v", envelope)
	}
}

func TestLoadOfficialPackageRejectsEscapingPreservedPaths(t *testing.T) {
	t.Parallel()
	for name, manifest := range map[string]string{
		"hooks":       `{"name":"unsafe-hooks","hooks":"./../outside.json"}`,
		"asset":       `{"name":"unsafe-asset","interface":{"logo":"./assets/../../outside.png"}}`,
		"dark-asset":  `{"name":"unsafe-dark-asset","interface":{"logoDark":"./assets/../../outside.png"}}`,
		"screenshots": `{"name":"unsafe-screenshots","interface":{"screenshots":["https://example.test/image.png"]}}`,
	} {
		name, manifest := name, manifest
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			writeLoaderFile(t, filepath.Join(root, ".codex-plugin", "plugin.json"), manifest)
			if _, err := testOpenAILoader(t).Load(context.Background(), domain.LoadInput{SnapshotRoot: root}); err == nil {
				t.Fatal("unsafe official manifest path accepted")
			}
		})
	}
}

func TestLoadOfficialPackageRejectsLifecycleHooksUntilModeled(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeLoaderFile(t, filepath.Join(root, ".codex-plugin", "plugin.json"), `{"name":"hooked","hooks":"./hooks/session.json"}`)
	writeLoaderFile(t, filepath.Join(root, "hooks", "session.json"), `{}`)

	_, err := testOpenAILoader(t).Load(context.Background(), domain.LoadInput{SnapshotRoot: root})
	var loadErr *domain.LoadError
	if !errors.As(err, &loadErr) || loadErr.Diagnostic.Code != "official_hooks_unsupported" {
		t.Fatalf("hook package error = %v", err)
	}
}

func TestLoadRejectsImplicitLifecycleHooksForPortableAndOfficialPackages(t *testing.T) {
	t.Parallel()
	for _, format := range []string{"portable", "official"} {
		format := format
		t.Run(format, func(t *testing.T) {
			root := t.TempDir()
			if format == "portable" {
				writeLoaderFile(t, filepath.Join(root, "plugin.json"), `{"$schema":"https://agent-plugins.org/schemas/1.0.0/plugin.schema.json","name":"implicit-hooks","version":"1.0.0","description":"Implicit hooks"}`)
			} else {
				writeLoaderFile(t, filepath.Join(root, ".codex-plugin", "plugin.json"), `{"name":"implicit-hooks"}`)
			}
			writeLoaderFile(t, filepath.Join(root, "hooks", "hooks.json"), `{}`)

			var err error
			if format == "portable" {
				_, err = testLoader(t).Load(context.Background(), domain.LoadInput{SnapshotRoot: root})
			} else {
				_, err = testOpenAILoader(t).Load(context.Background(), domain.LoadInput{SnapshotRoot: root})
			}
			var loadErr *domain.LoadError
			if !errors.As(err, &loadErr) || loadErr.Diagnostic.Code != "official_hooks_unsupported" || !strings.Contains(err.Error(), "remove the hooks directory") {
				t.Fatalf("implicit %s hooks error = %v", format, err)
			}
		})
	}
}

func TestLoadAppIDsAreOpaqueSafeTokens(t *testing.T) {
	t.Parallel()
	for _, id := range []string{"connector_68df038e0ba48191908c8434991bbac2", "asdk_app_69c18c28f1188191bf5b8445c4ab0a2e", "plugin_asdk_app_current", "future:v2~opaque.value"} {
		id := id
		t.Run(id, func(t *testing.T) {
			root := t.TempDir()
			writeLoaderFile(t, filepath.Join(root, ".codex-plugin", "plugin.json"), `{"name":"opaque-app","apps":"./.app.json"}`)
			writeLoaderFile(t, filepath.Join(root, ".app.json"), `{"apps":{"opaque":{"id":"`+id+`"}}}`)
			envelope, err := testOpenAILoader(t).Load(context.Background(), domain.LoadInput{SnapshotRoot: root})
			if err != nil || !envelope.App.Enabled {
				t.Fatalf("opaque app id %q rejected: %+v, %v", id, envelope.App, err)
			}
		})
	}
	for _, id := range []string{" leading", "contains space", "../escape", "slash/value"} {
		id := id
		t.Run("reject-"+id, func(t *testing.T) {
			root := t.TempDir()
			writeLoaderFile(t, filepath.Join(root, ".codex-plugin", "plugin.json"), `{"name":"bad-app","apps":"./.app.json"}`)
			writeLoaderFile(t, filepath.Join(root, ".app.json"), `{"apps":{"bad":{"id":"`+id+`"}}}`)
			envelope, err := testOpenAILoader(t).Load(context.Background(), domain.LoadInput{SnapshotRoot: root})
			if err != nil || envelope.App.Enabled || !hasDiagnostic(envelope.Diagnostics, "app_id_invalid") {
				t.Fatalf("unsafe app id %q accepted: %+v, %v", id, envelope.App, err)
			}
		})
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
  "extensions":{"com.example.good":{"enabled":true},"com.example.future":{"opaque":true}},
  "futureField":{"kept":true,"kept":false}
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
	if !hasDiagnostic(envelope.Diagnostics, "plugin_unknown_field") {
		t.Fatalf("diagnostics = %+v", envelope.Diagnostics)
	}
}

func TestLoadRejectsInvalidKnownManifestFieldDespiteUnknownFieldTolerance(t *testing.T) {
	t.Parallel()
	for name, extensions := range map[string]string{
		"non-object extensions":      `"invalid"`,
		"non-object extension value": `{"com.example.invalid":"opaque"}`,
	} {
		name, extensions := name, extensions
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			writeLoaderFile(t, filepath.Join(root, "plugin.json"), `{"$schema":"https://agent-plugins.org/schemas/1.0.0/plugin.schema.json","name":"invalid-extension","futureField":true,"extensions":`+extensions+`}`)
			_, err := testLoader(t).Load(context.Background(), domain.LoadInput{SnapshotRoot: root})
			var loadErr *domain.LoadError
			if !errors.As(err, &loadErr) || loadErr.Diagnostic.Code != "plugin_schema_invalid" {
				t.Fatalf("known-field violation error = %v", err)
			}
		})
	}
}

func TestLoadRejectsInvalidAndUnsupportedPluginManifests(t *testing.T) {
	t.Parallel()
	tests := map[string]string{
		"invalid":       `{"$schema":"https://agent-plugins.org/schemas/1.0.0/plugin.schema.json","name":"Invalid Name"}`,
		"working-draft": `{"$schema":"https://agent-plugins.org/schemas/1.1.0/plugin.schema.json","name":"draft"}`,
		"unsupported":   `{"$schema":"https://agent-plugins.org/schemas/2.0.0/plugin.schema.json","name":"future"}`,
		"malformed":     `{`,
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

func TestUnsupportedSchemaDiagnosticPrecedesPackageComponentInspection(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeLoaderFile(t, filepath.Join(root, "plugin.json"), `{"$schema":"https://agent-plugins.org/schemas/1.1.0/plugin.schema.json","name":"draft"}`)
	writeLoaderFile(t, filepath.Join(root, "hooks", "hooks.json"), `{}`)
	_, err := testLoader(t).Load(context.Background(), domain.LoadInput{SnapshotRoot: root})
	var loadErr *domain.LoadError
	if !errors.As(err, &loadErr) || loadErr.Diagnostic.Code != "plugin_schema_unsupported" {
		t.Fatalf("unsupported schema selection error = %v", err)
	}
}

func TestStandardLoaderDoesNotImplicitlyImportNativeOrLegacyManifests(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeLoaderFile(t, filepath.Join(root, ".codex-plugin", "plugin.json"), `{"name":"native-only"}`)
	writeLoaderFile(t, filepath.Join(root, "plugin", "plugin.yaml"), "name: legacy-only\n")
	_, err := testLoader(t).Load(context.Background(), domain.LoadInput{SnapshotRoot: root})
	var loadErr *domain.LoadError
	if !errors.As(err, &loadErr) || loadErr.Diagnostic.Code != "plugin_manifest_missing" {
		t.Fatalf("standard loader fallback error = %v", err)
	}
}

func TestMCPVersionMismatchDisablesMCPAndPreservesSkills(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeMinimalPlugin(t, root, "mismatch")
	writeLoaderFile(t, filepath.Join(root, "mcp.json"), `{"$schema":"https://agent-plugins.org/schemas/1.1.0/mcp.schema.json","mcpServers":{}}`)
	writeLoaderFile(t, filepath.Join(root, "skills", "kept", "SKILL.md"), "---\nname: kept\ndescription: Kept skill\n---\n")
	envelope, err := testLoader(t).Load(context.Background(), domain.LoadInput{SnapshotRoot: root})
	if err != nil {
		t.Fatal(err)
	}
	if envelope.MCP.Enabled || len(envelope.Skills) != 1 || !hasDiagnostic(envelope.Diagnostics, "mcp_schema_mismatch") {
		t.Fatalf("mismatch boundary = mcp=%+v skills=%+v diagnostics=%+v", envelope.MCP, envelope.Skills, envelope.Diagnostics)
	}
}

func TestStdioRequirementsAreValidatedWithoutExecution(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeMinimalPlugin(t, root, "stdio")
	writeLoaderFile(t, filepath.Join(root, "bin", "server"), "not executable code")
	writeLoaderFile(t, filepath.Join(root, "mcp.json"), `{
  "$schema":"https://agent-plugins.org/schemas/1.0.0/mcp.schema.json",
  "mcpServers":{
    "bundled":{"type":"stdio","command":"./bin/server","args":["${PLUGIN_ROOT}/config"],"env":{"DATA":"${PLUGIN_DATA}/state"},"cwd":"${PLUGIN_DATA}/work"},
    "bare":{"type":"stdio","command":"npx"},
    "literal-command-placeholder":{"type":"stdio","command":"${PLUGIN_ROOT}"},
    "spaced-bare-command":{"type":"stdio","command":"node helper"},
    "escape":{"type":"stdio","command":"npx","cwd":"${PLUGIN_ROOT}/../outside"}
  }
}`)
	envelope, err := testLoader(t).Load(context.Background(), domain.LoadInput{SnapshotRoot: root, ExecutableFiles: []string{"bin/server"}})
	if err != nil {
		t.Fatal(err)
	}
	requirement := envelope.MCP.Servers["bundled"].StdioRequirement
	if requirement == nil || requirement.Kind != domain.ExecutableBundled || requirement.BundledRelativePath != "bin/server" || !requirement.UsesPluginRoot || !requirement.UsesPluginData {
		t.Fatalf("bundled requirement = %+v", requirement)
	}
	if bare := envelope.MCP.Servers["bare"].StdioRequirement; bare == nil || bare.Kind != domain.ExecutableBare || bare.Command != "npx" {
		t.Fatalf("bare requirement = %+v", bare)
	}
	if literal := envelope.MCP.Servers["literal-command-placeholder"].StdioRequirement; literal == nil || literal.Kind != domain.ExecutableBare || literal.Command != "${PLUGIN_ROOT}" || literal.UsesPluginRoot {
		t.Fatalf("literal command placeholder was expanded or classified as an argument placeholder: %+v", literal)
	}
	if spaced := envelope.MCP.Servers["spaced-bare-command"].StdioRequirement; spaced == nil || spaced.Kind != domain.ExecutableBare || spaced.Command != "node helper" {
		t.Fatalf("bare executable lookup token was split or rejected: %+v", spaced)
	}
	if _, ok := envelope.MCP.InvalidServer["escape"]; !ok {
		t.Fatal("escaping cwd server was accepted")
	}
}

func TestStdioRejectsInvalidReservedContractAndExecutableRequirements(t *testing.T) {
	t.Parallel()
	tests := map[string]struct {
		server      string
		prepare     func(*testing.T, string)
		executables []string
	}{
		"placeholder path in command":  {server: `{"type":"stdio","command":"${PLUGIN_ROOT}/bin/server"}`},
		"unknown reserved placeholder": {server: `{"type":"stdio","command":"node","args":["${PLUGIN_CACHE}"]}`},
		"reserved root environment":    {server: `{"type":"stdio","command":"node","env":{"PLUGIN_ROOT":"override"}}`},
		"reserved data environment":    {server: `{"type":"stdio","command":"node","env":{"PLUGIN_DATA":"override"}}`},
		"missing bundled command":      {server: `{"type":"stdio","command":"./bin/server"}`, executables: []string{"bin/server"}},
		"non-executable bundled command": {
			server:  `{"type":"stdio","command":"./bin/server"}`,
			prepare: func(t *testing.T, root string) { writeLoaderFile(t, filepath.Join(root, "bin", "server"), "fixture") },
		},
	}
	for name, test := range tests {
		name, test := name, test
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			writeMinimalPlugin(t, root, "invalid-stdio")
			if test.prepare != nil {
				test.prepare(t, root)
			}
			writeLoaderFile(t, filepath.Join(root, "mcp.json"), `{"$schema":"https://agent-plugins.org/schemas/1.0.0/mcp.schema.json","mcpServers":{"bad":`+test.server+`}}`)
			envelope, err := testLoader(t).Load(context.Background(), domain.LoadInput{SnapshotRoot: root, ExecutableFiles: test.executables})
			if err != nil {
				t.Fatal(err)
			}
			if _, exists := envelope.MCP.Servers["bad"]; exists || envelope.MCP.InvalidServer["bad"].Code != "mcp_server_invalid" {
				t.Fatalf("invalid stdio server accepted: %+v", envelope.MCP)
			}
		})
	}
}

func TestStdioBundledCommandAllowsContainedInternalSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink fixture requires an unprivileged symlink platform")
	}
	root := t.TempDir()
	writeMinimalPlugin(t, root, "symlink-stdio")
	writeLoaderFile(t, filepath.Join(root, "bin", "server"), "fixture")
	if err := os.Symlink("server", filepath.Join(root, "bin", "server-link")); err != nil {
		t.Fatal(err)
	}
	writeLoaderFile(t, filepath.Join(root, "mcp.json"), `{"$schema":"https://agent-plugins.org/schemas/1.0.0/mcp.schema.json","mcpServers":{"linked":{"type":"stdio","command":"./bin/server-link"}}}`)
	envelope, err := testLoader(t).Load(context.Background(), domain.LoadInput{SnapshotRoot: root, ExecutableFiles: []string{"bin/server"}})
	if err != nil {
		t.Fatal(err)
	}
	requirement := envelope.MCP.Servers["linked"].StdioRequirement
	if requirement == nil || requirement.Kind != domain.ExecutableBundled || requirement.BundledRelativePath != "bin/server-link" {
		t.Fatalf("internal symlink command requirement = %+v diagnostics=%+v", requirement, envelope.Diagnostics)
	}
}

func TestLoadSnapshotRequiresExactVersionedSHA256Digest(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeMinimalPlugin(t, root, "snapshot-digest")
	valid := "sha256:" + strings.Repeat("a", 64)
	if _, err := testLoader(t).LoadSnapshot(context.Background(), domain.PackageSnapshot{Root: root, DigestAlgorithm: domain.TreeDigestAlgorithm, TreeDigest: valid}); err != nil {
		t.Fatalf("valid snapshot digest rejected: %v", err)
	}
	for _, digest := range []string{"sha256:tree", "sha256:" + strings.Repeat("A", 64), "sha256:" + strings.Repeat("z", 64), strings.Repeat("a", 64)} {
		if _, err := testLoader(t).LoadSnapshot(context.Background(), domain.PackageSnapshot{Root: root, DigestAlgorithm: domain.TreeDigestAlgorithm, TreeDigest: digest}); err == nil {
			t.Fatalf("invalid snapshot digest accepted: %q", digest)
		}
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

func TestDuplicateJSONFieldsRespectComponentFailureBoundaries(t *testing.T) {
	t.Parallel()
	pluginRoot := t.TempDir()
	writeLoaderFile(t, filepath.Join(pluginRoot, "plugin.json"), `{"$schema":"https://agent-plugins.org/schemas/1.0.0/plugin.schema.json","name":"first","name":"second"}`)
	if _, err := testLoader(t).Load(context.Background(), domain.LoadInput{SnapshotRoot: pluginRoot}); err == nil {
		t.Fatal("duplicate plugin manifest field was accepted")
	}

	mcpRoot := t.TempDir()
	writeMinimalPlugin(t, mcpRoot, "duplicate-mcp")
	writeLoaderFile(t, filepath.Join(mcpRoot, "mcp.json"), `{
  "$schema":"https://agent-plugins.org/schemas/1.0.0/mcp.schema.json",
  "mcpServers":{
    "valid":{"type":"stdio","command":"node"},
    "invalid":{"type":"stdio","command":"node","command":"other"}
  }
}`)
	envelope, err := testLoader(t).Load(context.Background(), domain.LoadInput{SnapshotRoot: mcpRoot})
	if err != nil {
		t.Fatal(err)
	}
	if len(envelope.MCP.Servers) != 1 || envelope.MCP.Servers["valid"].Name != "valid" || envelope.MCP.InvalidServer["invalid"].Code != "mcp_server_invalid" {
		t.Fatalf("duplicate server field crossed component boundary: %+v", envelope.MCP)
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

func testOpenAILoader(t *testing.T) OpenAILoader {
	t.Helper()
	return OpenAILoader{Loader: testLoader(t)}
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
