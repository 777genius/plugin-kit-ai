package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPluginServicePublicationMaterializeCodexMarketplaceRoot(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	dest := t.TempDir()
	mustWritePublicationSourceFile(t, root, "plugin.yaml", "api_version: v1\nname: \"demo\"\nversion: \"0.1.0\"\ndescription: \"demo\"\ntargets: [\"codex-package\"]\n")
	mustWritePublicationSourceFile(t, root, filepath.Join("targets", "codex-package", "package.yaml"), "homepage: https://example.com/demo\n")
	mustWritePublicationSourceFile(t, root, filepath.Join("targets", "codex-package", "interface.json"), `{"defaultPrompt":["Inspect"]}`)
	mustWritePublicationSourceFile(t, root, filepath.Join("publish", "codex", "marketplace.yaml"), "api_version: v1\nmarketplace_name: local-repo\ndisplay_name: Local Repo\nsource_root: ./\ncategory: Productivity\n")
	mustWritePublicationSourceFile(t, root, filepath.Join("skills", "demo", "SKILL.md"), "# Demo\n")

	result, err := PluginService{}.PublicationMaterialize(PluginPublicationMaterializeOptions{
		Root:   root,
		Target: "codex-package",
		Dest:   dest,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Lines) == 0 {
		t.Fatal("expected output lines")
	}
	mustContainFileText(t, filepath.Join(dest, "plugins", "demo", ".codex-plugin", "plugin.json"), `"name": "demo"`)
	mustContainFileText(t, filepath.Join(dest, "plugins", "demo", "skills", "demo", "SKILL.md"), "# Demo")
	mustContainFileText(t, filepath.Join(dest, ".agents", "plugins", "marketplace.json"), `"path": "./plugins/demo"`)
}

func TestPluginServicePublicationMaterializeClaudeMarketplaceMergesExistingCatalog(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	dest := t.TempDir()
	mustWritePublicationSourceFile(t, root, "plugin.yaml", "api_version: v1\nname: \"demo\"\nversion: \"0.1.0\"\ndescription: \"demo\"\ntargets: [\"claude\"]\n")
	mustWritePublicationSourceFile(t, root, filepath.Join("launcher.yaml"), "runtime: go\nentrypoint: ./bin/demo\n")
	mustWritePublicationSourceFile(t, root, filepath.Join("targets", "claude", "package.yaml"), "homepage: https://example.com/demo\n")
	mustWritePublicationSourceFile(t, root, filepath.Join("publish", "claude", "marketplace.yaml"), "api_version: v1\nmarketplace_name: team-tools\nowner_name: Team\nsource_root: ./\n")
	mustWritePublicationSourceFile(t, dest, filepath.Join(".claude-plugin", "marketplace.json"), `{"name":"team-tools","owner":{"name":"Team"},"plugins":[{"name":"alpha","source":"./plugins/alpha","description":"alpha","version":"1.0.0"}]}`)

	_, err := PluginService{}.PublicationMaterialize(PluginPublicationMaterializeOptions{
		Root:   root,
		Target: "claude",
		Dest:   dest,
	})
	if err != nil {
		t.Fatal(err)
	}
	mustContainFileText(t, filepath.Join(dest, ".claude-plugin", "marketplace.json"), `"name": "demo"`)
}

func TestPluginServicePublicationMaterializeRemovesOldPackageRoot(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	dest := t.TempDir()
	mustWritePublicationSourceFile(t, root, "plugin.yaml", "api_version: v1\nname: \"demo\"\nversion: \"0.1.0\"\ndescription: \"demo\"\ntargets: [\"codex-package\"]\n")
	mustWritePublicationSourceFile(t, root, filepath.Join("targets", "codex-package", "package.yaml"), "homepage: https://example.com/demo\n")
	mustWritePublicationSourceFile(t, root, filepath.Join("targets", "codex-package", "interface.json"), `{"defaultPrompt":["Inspect"]}`)
	mustWritePublicationSourceFile(t, root, filepath.Join("publish", "codex", "marketplace.yaml"), "api_version: v1\nmarketplace_name: local-repo\nsource_root: ./\ncategory: Productivity\n")
	mustWritePublicationSourceFile(t, dest, filepath.Join("plugins", "demo", "stale.txt"), "old\n")

	_, err := PluginService{}.PublicationMaterialize(PluginPublicationMaterializeOptions{
		Root:   root,
		Target: "codex-package",
		Dest:   dest,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dest, "plugins", "demo", "stale.txt")); !os.IsNotExist(err) {
		t.Fatalf("stale file still present: %v", err)
	}
}

func TestPluginServicePublicationRemoveCodexMarketplaceRoot(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	dest := t.TempDir()
	mustWritePublicationSourceFile(t, root, "plugin.yaml", "api_version: v1\nname: \"demo\"\nversion: \"0.1.0\"\ndescription: \"demo\"\ntargets: [\"codex-package\"]\n")
	mustWritePublicationSourceFile(t, root, filepath.Join("targets", "codex-package", "package.yaml"), "homepage: https://example.com/demo\n")
	mustWritePublicationSourceFile(t, root, filepath.Join("targets", "codex-package", "interface.json"), `{"defaultPrompt":["Inspect"]}`)
	mustWritePublicationSourceFile(t, root, filepath.Join("publish", "codex", "marketplace.yaml"), "api_version: v1\nmarketplace_name: local-repo\nsource_root: ./\ncategory: Productivity\n")
	mustWritePublicationSourceFile(t, dest, filepath.Join("plugins", "demo", ".codex-plugin", "plugin.json"), "{}\n")
	mustWritePublicationSourceFile(t, dest, filepath.Join(".agents", "plugins", "marketplace.json"), `{"name":"local-repo","plugins":[{"name":"demo","source":{"source":"local","path":"./plugins/demo"},"policy":{"installation":"AVAILABLE","authentication":"ON_INSTALL"},"category":"Productivity"},{"name":"alpha","source":{"source":"local","path":"./plugins/alpha"},"policy":{"installation":"AVAILABLE","authentication":"ON_INSTALL"},"category":"Productivity"}]}`)

	result, err := PluginService{}.PublicationRemove(PluginPublicationRemoveOptions{
		Root:   root,
		Target: "codex-package",
		Dest:   dest,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Lines) == 0 {
		t.Fatal("expected output lines")
	}
	if _, err := os.Stat(filepath.Join(dest, "plugins", "demo")); !os.IsNotExist(err) {
		t.Fatalf("package root still present: %v", err)
	}
	body, err := os.ReadFile(filepath.Join(dest, ".agents", "plugins", "marketplace.json"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(body)
	if strings.Contains(text, `"name": "demo"`) || strings.Contains(text, `"name":"demo"`) {
		t.Fatalf("demo entry still present:\n%s", text)
	}
	if !strings.Contains(text, `"name":"alpha"`) && !strings.Contains(text, `"name": "alpha"`) {
		t.Fatalf("alpha entry missing:\n%s", text)
	}
}

func TestPluginServicePublicationRemoveIsIdempotentWhenEntryMissing(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	dest := t.TempDir()
	mustWritePublicationSourceFile(t, root, "plugin.yaml", "api_version: v1\nname: \"demo\"\nversion: \"0.1.0\"\ndescription: \"demo\"\ntargets: [\"claude\"]\n")
	mustWritePublicationSourceFile(t, root, filepath.Join("launcher.yaml"), "runtime: go\nentrypoint: ./bin/demo\n")
	mustWritePublicationSourceFile(t, root, filepath.Join("targets", "claude", "package.yaml"), "homepage: https://example.com/demo\n")
	mustWritePublicationSourceFile(t, root, filepath.Join("publish", "claude", "marketplace.yaml"), "api_version: v1\nmarketplace_name: team-tools\nowner_name: Team\nsource_root: ./\n")
	mustWritePublicationSourceFile(t, dest, filepath.Join(".claude-plugin", "marketplace.json"), `{"name":"team-tools","owner":{"name":"Team"},"plugins":[{"name":"alpha","source":"./plugins/alpha","description":"alpha","version":"1.0.0"}]}`)

	result, err := PluginService{}.PublicationRemove(PluginPublicationRemoveOptions{
		Root:   root,
		Target: "claude",
		Dest:   dest,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Lines) == 0 {
		t.Fatal("expected output lines")
	}
	body, err := os.ReadFile(filepath.Join(dest, ".claude-plugin", "marketplace.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "alpha") {
		t.Fatalf("existing alpha entry should remain:\n%s", body)
	}
}

func TestPluginServicePublicationVerifyRootReady(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	dest := t.TempDir()
	mustWritePublicationSourceFile(t, root, "plugin.yaml", "api_version: v1\nname: \"demo\"\nversion: \"0.1.0\"\ndescription: \"demo\"\ntargets: [\"codex-package\"]\n")
	mustWritePublicationSourceFile(t, root, filepath.Join("targets", "codex-package", "package.yaml"), "homepage: https://example.com/demo\n")
	mustWritePublicationSourceFile(t, root, filepath.Join("targets", "codex-package", "interface.json"), `{"defaultPrompt":["Inspect"]}`)
	mustWritePublicationSourceFile(t, root, filepath.Join("publish", "codex", "marketplace.yaml"), "api_version: v1\nmarketplace_name: local-repo\nsource_root: ./\ncategory: Productivity\n")
	mustWritePublicationSourceFile(t, root, filepath.Join("skills", "demo", "SKILL.md"), "# Demo\n")
	if _, err := (PluginService{}).PublicationMaterialize(PluginPublicationMaterializeOptions{
		Root:   root,
		Target: "codex-package",
		Dest:   dest,
	}); err != nil {
		t.Fatal(err)
	}

	result, err := PluginService{}.PublicationVerifyRoot(PluginPublicationVerifyRootOptions{
		Root:   root,
		Target: "codex-package",
		Dest:   dest,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Ready || result.Status != "ready" || result.IssueCount != 0 {
		t.Fatalf("verify result = %+v", result)
	}
}

func TestPluginServicePublicationVerifyRootReportsDriftedCatalogEntry(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	dest := t.TempDir()
	mustWritePublicationSourceFile(t, root, "plugin.yaml", "api_version: v1\nname: \"demo\"\nversion: \"0.1.0\"\ndescription: \"demo\"\ntargets: [\"claude\"]\n")
	mustWritePublicationSourceFile(t, root, filepath.Join("launcher.yaml"), "runtime: go\nentrypoint: ./bin/demo\n")
	mustWritePublicationSourceFile(t, root, filepath.Join("targets", "claude", "package.yaml"), "homepage: https://example.com/demo\n")
	mustWritePublicationSourceFile(t, root, filepath.Join("publish", "claude", "marketplace.yaml"), "api_version: v1\nmarketplace_name: team-tools\nowner_name: Team\nsource_root: ./\n")
	if _, err := (PluginService{}).PublicationMaterialize(PluginPublicationMaterializeOptions{
		Root:   root,
		Target: "claude",
		Dest:   dest,
	}); err != nil {
		t.Fatal(err)
	}
	mustWritePublicationSourceFile(t, dest, filepath.Join(".claude-plugin", "marketplace.json"), `{"name":"team-tools","owner":{"name":"Team"},"plugins":[{"name":"demo","source":"./plugins/demo-drift","description":"demo","version":"0.1.0"}]}`)

	result, err := PluginService{}.PublicationVerifyRoot(PluginPublicationVerifyRootOptions{
		Root:   root,
		Target: "claude",
		Dest:   dest,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Ready || result.Status != "needs_sync" {
		t.Fatalf("verify result = %+v", result)
	}
	if result.IssueCount != 1 || result.Issues[0].Code != "drifted_materialized_catalog_entry" {
		t.Fatalf("verify issues = %+v", result.Issues)
	}
}

func mustWritePublicationSourceFile(t *testing.T, root, rel, body string) {
	t.Helper()
	full := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func mustContainFileText(t *testing.T, path, want string) {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), want) {
		t.Fatalf("%s missing %q:\n%s", path, want, body)
	}
}
