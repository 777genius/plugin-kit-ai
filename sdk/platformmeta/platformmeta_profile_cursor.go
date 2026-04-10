package platformmeta

func cursorProfile() PlatformProfile {
	return PlatformProfile{
		ID: "cursor",
		Contract: TargetContractMeta{
			PlatformFamily:         FamilyIDEPlugin,
			TargetClass:            "plugin_package",
			TargetNoun:             "plugin",
			ProductionClass:        "packaging-only target",
			RuntimeContract:        "Cursor marketplace plugin bundle with portable skills and optional portable MCP",
			InstallModel:           "marketplace or /add-plugin install",
			DevModel:               "package authoring workspace",
			ActivationModel:        "plugin reload or restart",
			NativeRoot:             ".cursor-plugin/plugin.json",
			ImportSupport:          true,
			GenerateSupport:        true,
			ValidateSupport:        true,
			PortableComponentKinds: []string{"skills", "mcp_servers"},
			TargetComponentKinds:   []string{},
			Summary:                "Cursor plugin packages compile portable skills and optional shared MCP into the current observed .cursor-plugin bundle shape without inventing unsupported target-native authoring surfaces.",
		},
		SDK: SDKMeta{
			PublicPackage:   "cursor",
			InternalPackage: "cursor",
			InternalImport:  "github.com/777genius/plugin-kit-ai/sdk/internal/platforms/cursor",
			Status:          StatusScaffoldOnly,
			TransportModes:  []TransportMode{TransportProcess},
			LiveTestProfile: "cursor_plugin",
		},
		Launcher: LauncherMeta{Requirement: LauncherIgnored},
		SurfaceTiers: []SurfaceSupport{
			{Kind: "rules", Tier: SurfaceTierUnsupported},
			{Kind: "agents_markdown", Tier: SurfaceTierUnsupported},
			{Kind: "hooks", Tier: SurfaceTierUnsupported},
			{Kind: "commands", Tier: SurfaceTierUnsupported},
			{Kind: "subagents", Tier: SurfaceTierUnsupported},
		},
		ManagedArtifacts: []ManagedArtifactSpec{
			{Kind: ManagedArtifactStatic, Path: ".cursor-plugin/plugin.json"},
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
				"src/skills/{{.ProjectName}}/SKILL.md",
			},
			ForbiddenFiles: []string{
				"launcher.yaml",
			},
			TemplateFiles: []TemplateFile{
				{Path: "src/plugin.yaml", Template: "plugin.yaml.tmpl"},
				{Path: "src/README.md", Template: "cursor.README.md.tmpl"},
				{Path: "src/mcp/servers.yaml", Template: "mcp.servers.yaml.tmpl", Extra: true},
				{Path: "src/skills/{{.ProjectName}}/SKILL.md", Template: "SKILL.md.tmpl", Extra: true},
				{Path: "CLAUDE.md", Template: "ROOT.CLAUDE.md.tmpl"},
				{Path: "AGENTS.md", Template: "ROOT.AGENTS.md.tmpl"},
			},
		},
		Validate: ValidateMeta{
			RequiredFiles: []string{
				"README.md",
				".cursor-plugin/plugin.json",
			},
			ForbiddenFiles: []string{
				"launcher.yaml",
			},
		},
	}
}

func cursorWorkspaceProfile() PlatformProfile {
	return PlatformProfile{
		ID: "cursor-workspace",
		Contract: TargetContractMeta{
			PlatformFamily:         FamilyCodePlugin,
			TargetClass:            "workspace_config_lane",
			TargetNoun:             "workspace",
			ProductionClass:        "packaging-only target",
			RuntimeContract:        "workspace-config lane with first-class MCP config and project rules",
			InstallModel:           "workspace config files",
			DevModel:               "config authoring workspace",
			ActivationModel:        "config reload or restart",
			NativeRoot:             ".cursor/mcp.json",
			ImportSupport:          true,
			GenerateSupport:        true,
			ValidateSupport:        true,
			PortableComponentKinds: []string{"mcp_servers"},
			TargetComponentKinds:   []string{"rules", "agents_markdown"},
			Summary:                "Cursor workspace compiles as a repo-local config lane with generated .cursor MCP config, project rules, and authored Cursor AGENTS content merged into the generated root AGENTS.md boundary file.",
		},
		SDK: SDKMeta{
			PublicPackage:   "cursor",
			InternalPackage: "cursor",
			InternalImport:  "github.com/777genius/plugin-kit-ai/sdk/internal/platforms/cursor",
			Status:          StatusScaffoldOnly,
			TransportModes:  []TransportMode{TransportProcess},
			LiveTestProfile: "cursor_workspace",
		},
		Launcher: LauncherMeta{Requirement: LauncherIgnored},
		NativeDocs: []NativeDocSpec{
			{Kind: "agents_markdown", Path: "src/targets/cursor-workspace/AGENTS.md", Format: NativeDocMarkdown, Role: NativeDocRoleStructured},
		},
		SurfaceTiers: []SurfaceSupport{
			{Kind: "mcp", Tier: SurfaceTierStable},
			{Kind: "rules", Tier: SurfaceTierStable},
			{Kind: "agents_markdown", Tier: SurfaceTierStable},
		},
		ManagedArtifacts: []ManagedArtifactSpec{
			{Kind: ManagedArtifactPortableMCP, Path: ".cursor/mcp.json"},
			{Kind: ManagedArtifactMirror, ComponentKind: "rules", SourceRoot: "src/targets/cursor-workspace/rules", OutputRoot: ".cursor/rules"},
		},
		Scaffold: ScaffoldMeta{
			RequiredFiles: []string{
				"src/plugin.yaml",
				"src/README.md",
				"src/targets/cursor-workspace/rules/project.mdc",
				"CLAUDE.md",
				"AGENTS.md",
			},
			OptionalFiles: []string{
				"src/targets/cursor-workspace/AGENTS.md",
			},
			ForbiddenFiles: []string{
				"launcher.yaml",
			},
			TemplateFiles: []TemplateFile{
				{Path: "src/plugin.yaml", Template: "plugin.yaml.tmpl"},
				{Path: "src/README.md", Template: "cursor-workspace.README.md.tmpl"},
				{Path: "src/targets/cursor-workspace/rules/project.mdc", Template: "cursor.rule.mdc.tmpl"},
				{Path: "src/targets/cursor-workspace/AGENTS.md", Template: "cursor.AGENTS.md.tmpl", Extra: true},
				{Path: "CLAUDE.md", Template: "ROOT.CLAUDE.md.tmpl"},
				{Path: "AGENTS.md", Template: "ROOT.AGENTS.md.tmpl"},
			},
		},
		Validate: ValidateMeta{
			RequiredFiles: []string{
				"README.md",
			},
			ForbiddenFiles: []string{
				"launcher.yaml",
			},
		},
	}
}
