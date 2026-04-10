package platformmeta

func codexPackageProfile() PlatformProfile {
	return PlatformProfile{
		ID: "codex-package",
		Contract: TargetContractMeta{
			PlatformFamily:         FamilyPackagedRuntime,
			TargetClass:            "plugin_package",
			TargetNoun:             "plugin",
			ProductionClass:        "production-ready package lane",
			RuntimeContract:        "official Codex plugin package only",
			InstallModel:           "plugin directory or marketplace cache",
			DevModel:               "package authoring workspace",
			ActivationModel:        "plugin reload or restart",
			NativeRoot:             "~/.codex/plugins/...",
			ImportSupport:          true,
			GenerateSupport:        true,
			ValidateSupport:        true,
			PortableComponentKinds: []string{"skills", "mcp_servers"},
			TargetComponentKinds:   []string{"package_metadata", "interface", "manifest_extra", "app_manifest"},
			Summary:                "Codex package lane compiles the official plugin bundle from canonical src-authored inputs: plugin.json plus shared package metadata, optional interface/app assets, and optional MCP wiring.",
		},
		SDK: SDKMeta{
			PublicPackage:   "codex",
			InternalPackage: "codex",
			InternalImport:  "github.com/777genius/plugin-kit-ai/sdk/internal/platforms/codex",
			Status:          StatusRuntimeSupported,
			TransportModes:  []TransportMode{TransportProcess},
			LiveTestProfile: "codex_package",
		},
		Launcher: LauncherMeta{Requirement: LauncherIgnored},
		NativeDocs: []NativeDocSpec{
			{Kind: "package_metadata", Path: "src/targets/codex-package/package.yaml", Format: NativeDocYAML, Role: NativeDocRoleStructured},
			{Kind: "interface", Path: "src/targets/codex-package/interface.json", Format: NativeDocJSON, Role: NativeDocRoleStructured},
			{Kind: "manifest_extra", Path: "src/targets/codex-package/manifest.extra.json", Format: NativeDocJSON, Role: NativeDocRoleExtra, ManagedKeys: []string{"name", "version", "description", "author", "homepage", "repository", "license", "keywords", "skills", "mcpServers", "apps", "interface"}},
			{Kind: "app_manifest", Path: "src/targets/codex-package/app.json", Format: NativeDocJSON, Role: NativeDocRoleStructured},
		},
		SurfaceTiers: []SurfaceSupport{
			{Kind: "interface", Tier: SurfaceTierStable},
			{Kind: "manifest_extra", Tier: SurfaceTierStable},
			{Kind: "app_manifest", Tier: SurfaceTierBeta},
			{Kind: "agents", Tier: SurfaceTierUnsupported},
			{Kind: "contexts", Tier: SurfaceTierUnsupported},
			{Kind: "commands", Tier: SurfaceTierUnsupported},
		},
		ManagedArtifacts: []ManagedArtifactSpec{
			{Kind: ManagedArtifactStatic, Path: ".codex-plugin/plugin.json"},
			{Kind: ManagedArtifactStatic, Path: ".app.json"},
			{Kind: ManagedArtifactPortableSkills, SourceRoot: "src/skills", OutputRoot: "skills"},
			{Kind: ManagedArtifactPortableMCP, Path: ".mcp.json"},
		},
		Scaffold: ScaffoldMeta{
			RequiredFiles: []string{
				"src/plugin.yaml",
				"src/README.md",
				"CLAUDE.md",
				"AGENTS.md",
			},
			OptionalFiles: []string{
				"src/mcp/servers.yaml",
				"src/targets/codex-package/package.yaml",
				"src/targets/codex-package/interface.json",
				"src/targets/codex-package/manifest.extra.json",
				"src/targets/codex-package/app.json",
				"src/skills/{{.ProjectName}}/SKILL.md",
			},
			ForbiddenFiles: []string{
				"launcher.yaml",
				".codex/config.toml",
			},
			TemplateFiles: []TemplateFile{
				{Path: "src/plugin.yaml", Template: "plugin.yaml.tmpl"},
				{Path: "src/README.md", Template: "codex-package.README.md.tmpl"},
				{Path: "CLAUDE.md", Template: "ROOT.CLAUDE.md.tmpl"},
				{Path: "AGENTS.md", Template: "ROOT.AGENTS.md.tmpl"},
				{Path: "src/targets/codex-package/package.yaml", Template: "targets.codex-package.package.yaml.tmpl", Extra: true},
				{Path: "src/mcp/servers.yaml", Template: "mcp.servers.yaml.tmpl", Extra: true},
				{Path: "src/targets/codex-package/interface.json", Template: "codex-package.interface.json.tmpl", Extra: true},
				{Path: "src/targets/codex-package/manifest.extra.json", Template: "empty.json.tmpl", Extra: true},
				{Path: "src/targets/codex-package/app.json", Template: "empty.json.tmpl", Extra: true},
				{Path: "src/skills/{{.ProjectName}}/SKILL.md", Template: "SKILL.md.tmpl", Extra: true},
			},
		},
		Validate: ValidateMeta{
			RequiredFiles: []string{
				"README.md",
				".codex-plugin/plugin.json",
			},
			ForbiddenFiles: []string{
				"launcher.yaml",
				".codex/config.toml",
			},
		},
	}
}

func codexRuntimeProfile() PlatformProfile {
	return PlatformProfile{
		ID: "codex-runtime",
		Contract: TargetContractMeta{
			PlatformFamily:         FamilyPackagedRuntime,
			TargetClass:            "local_runtime_integration",
			TargetNoun:             "plugin",
			ProductionClass:        "production-ready runtime lane",
			RuntimeContract:        "public-stable notify runtime",
			InstallModel:           "repo-local config wiring",
			DevModel:               "local plugin workspace",
			ActivationModel:        "config reload or restart",
			NativeRoot:             ".codex/config.toml",
			ImportSupport:          true,
			GenerateSupport:        true,
			ValidateSupport:        true,
			PortableComponentKinds: []string{},
			TargetComponentKinds:   []string{"package_metadata", "commands", "contexts", "config_extra"},
			Summary:                "Codex runtime lane owns repo-local notify integration and managed config.toml, separate from the official package bundle.",
		},
		SDK: SDKMeta{
			PublicPackage:   "codex",
			InternalPackage: "codex",
			InternalImport:  "github.com/777genius/plugin-kit-ai/sdk/internal/platforms/codex",
			Status:          StatusRuntimeSupported,
			TransportModes:  []TransportMode{TransportProcess},
			LiveTestProfile: "codex_notify",
		},
		Launcher: LauncherMeta{Requirement: LauncherRequired},
		NativeDocs: []NativeDocSpec{
			{Kind: "package_metadata", Path: "src/targets/codex-runtime/package.yaml", Format: NativeDocYAML, Role: NativeDocRoleStructured},
			{Kind: "config_extra", Path: "src/targets/codex-runtime/config.extra.toml", Format: NativeDocTOML, Role: NativeDocRoleExtra, ManagedKeys: []string{"model", "notify"}},
		},
		SurfaceTiers: []SurfaceSupport{
			{Kind: "config_extra", Tier: SurfaceTierStable},
			{Kind: "commands", Tier: SurfaceTierBeta},
			{Kind: "contexts", Tier: SurfaceTierBeta},
		},
		ManagedArtifacts: []ManagedArtifactSpec{
			{Kind: ManagedArtifactStatic, Path: ".codex/config.toml"},
			{Kind: ManagedArtifactMirror, ComponentKind: "commands", SourceRoot: "src/targets/codex-runtime/commands", OutputRoot: "commands"},
			{Kind: ManagedArtifactMirror, ComponentKind: "contexts", SourceRoot: "src/targets/codex-runtime/contexts", OutputRoot: "contexts"},
		},
		Scaffold: ScaffoldMeta{
			RequiredFiles: []string{
				"go.mod",
				"src/README.md",
				"src/plugin.yaml",
				"src/launcher.yaml",
				"src/targets/codex-runtime/package.yaml",
				"CLAUDE.md",
				"AGENTS.md",
			},
			OptionalFiles: []string{
				"Makefile",
				".goreleaser.yml",
				"src/targets/codex-runtime/config.extra.toml",
			},
			ForbiddenFiles: []string{
				".claude-plugin/plugin.json",
				"hooks/hooks.json",
			},
			TemplateFiles: []TemplateFile{
				{Path: "go.mod", Template: "codex.go.mod.tmpl"},
				{Path: "cmd/{{.ProjectName}}/main.go", Template: "codex.main.go.tmpl"},
				{Path: "src/plugin.yaml", Template: "plugin.yaml.tmpl"},
				{Path: "src/launcher.yaml", Template: "launcher.yaml.tmpl"},
				{Path: "src/targets/codex-runtime/package.yaml", Template: "targets.codex-runtime.package.yaml.tmpl"},
				{Path: "src/README.md", Template: "codex-runtime.README.md.tmpl"},
				{Path: "CLAUDE.md", Template: "ROOT.CLAUDE.md.tmpl"},
				{Path: "AGENTS.md", Template: "ROOT.AGENTS.md.tmpl"},
				{Path: "src/targets/codex-runtime/config.extra.toml", Template: "empty.toml.tmpl", Extra: true},
				{Path: "Makefile", Template: "Makefile.tmpl", Extra: true},
				{Path: ".goreleaser.yml", Template: "goreleaser.yml.tmpl", Extra: true},
			},
		},
		Validate: ValidateMeta{
			RequiredFiles: []string{
				"go.mod",
				"README.md",
				"launcher.yaml",
				".codex/config.toml",
			},
			ForbiddenFiles: []string{
				".claude-plugin/plugin.json",
				"hooks/hooks.json",
			},
			BuildTargets: []string{"./..."},
		},
	}
}
