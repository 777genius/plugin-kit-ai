package publicationmodel

import (
	"path/filepath"
	"testing"

	"github.com/777genius/plugin-kit-ai/cli/internal/pluginmodel"
	"github.com/777genius/plugin-kit-ai/cli/internal/publishschema"
)

func TestBuild_IncludesPublicationCapableTargetsOnly(t *testing.T) {
	graph := pluginmodel.PackageGraph{
		Manifest: pluginmodel.Manifest{
			APIVersion:  "v1",
			Name:        "demo",
			Version:     "0.1.0",
			Description: "demo plugin",
			Targets:     []string{"codex-package", "codex-runtime", "gemini"},
		},
		Portable: pluginmodel.PortableComponents{
			Items: map[string][]string{
				"skills": {filepath.ToSlash(filepath.Join("skills", "demo", "SKILL.md"))},
			},
			MCP: &pluginmodel.PortableMCP{Path: filepath.ToSlash(filepath.Join("mcp", "servers.yaml"))},
		},
		Targets: map[string]pluginmodel.TargetState{
			"codex-package": {
				Target: "codex-package",
				Docs: map[string]string{
					"package_metadata": filepath.ToSlash(filepath.Join("targets", "codex-package", "package.yaml")),
					"interface":        filepath.ToSlash(filepath.Join("targets", "codex-package", "interface.json")),
				},
			},
			"codex-runtime": {
				Target: "codex-runtime",
				Docs: map[string]string{
					"package_metadata": filepath.ToSlash(filepath.Join("targets", "codex-runtime", "package.yaml")),
				},
			},
			"gemini": {
				Target: "gemini",
				Docs: map[string]string{
					"package_metadata": filepath.ToSlash(filepath.Join("targets", "gemini", "package.yaml")),
				},
				Components: map[string][]string{
					"contexts": {filepath.ToSlash(filepath.Join("targets", "gemini", "contexts", "GEMINI.md"))},
				},
			},
		},
	}

	model := Build(graph, publishschema.State{
		Codex:  &publishschema.CodexMarketplace{Path: publishschema.CodexMarketplaceRel},
		Gemini: &publishschema.GeminiGallery{Path: publishschema.GeminiGalleryRel},
	}, []string{"codex-package", "codex-runtime", "gemini"})
	if model.Core.APIVersion != "v1" || model.Core.Name != "demo" {
		t.Fatalf("core = %+v", model.Core)
	}
	if len(model.Packages) != 2 {
		t.Fatalf("packages = %+v", model.Packages)
	}

	codex := model.Packages[0]
	if codex.Target != "codex-package" || codex.PackageFamily != "codex-plugin" {
		t.Fatalf("codex package = %+v", codex)
	}
	if len(codex.ChannelFamilies) != 1 || codex.ChannelFamilies[0] != "codex-marketplace" {
		t.Fatalf("codex channel families = %+v", codex.ChannelFamilies)
	}
	for _, want := range []string{
		"plugin.yaml",
		filepath.ToSlash(filepath.Join("skills", "demo", "SKILL.md")),
		filepath.ToSlash(filepath.Join("mcp", "servers.yaml")),
		filepath.ToSlash(filepath.Join("targets", "codex-package", "package.yaml")),
		filepath.ToSlash(filepath.Join("targets", "codex-package", "interface.json")),
	} {
		if !contains(model.Packages[0].AuthoredInputs, want) {
			t.Fatalf("codex authored_inputs missing %q: %+v", want, model.Packages[0].AuthoredInputs)
		}
	}

	gemini := model.Packages[1]
	if gemini.Target != "gemini" || gemini.PackageFamily != "gemini-extension" {
		t.Fatalf("gemini package = %+v", gemini)
	}
	if len(gemini.ChannelFamilies) != 1 || gemini.ChannelFamilies[0] != "gemini-gallery" {
		t.Fatalf("gemini channel families = %+v", gemini.ChannelFamilies)
	}
	if !contains(gemini.AuthoredInputs, filepath.ToSlash(filepath.Join("targets", "gemini", "contexts", "GEMINI.md"))) {
		t.Fatalf("gemini authored_inputs = %+v", gemini.AuthoredInputs)
	}
	if len(model.Channels) != 2 {
		t.Fatalf("channels = %+v", model.Channels)
	}
	if model.Channels[0].Family != "codex-marketplace" || !contains(model.Channels[0].PackageTargets, "codex-package") {
		t.Fatalf("codex channel = %+v", model.Channels[0])
	}
	if model.Channels[1].Family != "gemini-gallery" || !contains(model.Channels[1].PackageTargets, "gemini") {
		t.Fatalf("gemini channel = %+v", model.Channels[1])
	}
}

func contains(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}
