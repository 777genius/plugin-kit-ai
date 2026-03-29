package platformmeta

import "strings"

type SupportStatus string
type TransportMode string
type PlatformFamily string
type LauncherRequirement string
type NativeDocRole string
type NativeDocFormat string
type ManagedArtifactKind string
type ContextStrategy string
type SurfaceTier string

const (
	StatusRuntimeSupported SupportStatus = "runtime_supported"
	StatusScaffoldOnly     SupportStatus = "scaffold_only"
	StatusDeferred         SupportStatus = "deferred"
)

const (
	TransportProcess TransportMode = "process"
	TransportHybrid  TransportMode = "hybrid"
	TransportDaemon  TransportMode = "daemon"
)

const (
	FamilyPackagedRuntime  PlatformFamily = "packaged_runtime"
	FamilyExtensionPackage PlatformFamily = "extension_package"
	FamilyCodePlugin       PlatformFamily = "code_plugin"
	FamilyIDEPlugin        PlatformFamily = "ide_plugin"
)

const (
	LauncherRequired LauncherRequirement = "required"
	LauncherOptional LauncherRequirement = "optional"
	LauncherIgnored  LauncherRequirement = "ignored"
)

const (
	NativeDocRoleStructured NativeDocRole = "structured"
	NativeDocRoleExtra      NativeDocRole = "extra"
)

const (
	NativeDocYAML     NativeDocFormat = "yaml"
	NativeDocJSON     NativeDocFormat = "json"
	NativeDocTOML     NativeDocFormat = "toml"
	NativeDocMarkdown NativeDocFormat = "md"
)

const (
	ManagedArtifactStatic          ManagedArtifactKind = "static"
	ManagedArtifactMirror          ManagedArtifactKind = "mirror"
	ManagedArtifactPortableMCP     ManagedArtifactKind = "portable_mcp"
	ManagedArtifactPortableSkills  ManagedArtifactKind = "portable_skills"
	ManagedArtifactSelectedContext ManagedArtifactKind = "selected_context"
)

const (
	ContextStrategyNone              ContextStrategy = ""
	ContextStrategyGeminiPrimaryRoot ContextStrategy = "gemini_primary_root"
)

const (
	SurfaceTierStable          SurfaceTier = "stable"
	SurfaceTierBeta            SurfaceTier = "beta"
	SurfaceTierPreview         SurfaceTier = "preview"
	SurfaceTierPassthroughOnly SurfaceTier = "passthrough_only"
	SurfaceTierUnsupported     SurfaceTier = "unsupported"
)

type TemplateFile struct {
	Path     string
	Template string
	Extra    bool
}

type ScaffoldMeta struct {
	RequiredFiles  []string
	OptionalFiles  []string
	ForbiddenFiles []string
	TemplateFiles  []TemplateFile
}

type ValidateMeta struct {
	RequiredFiles  []string
	ForbiddenFiles []string
	BuildTargets   []string
}

type TargetContractMeta struct {
	PlatformFamily         PlatformFamily
	TargetClass            string
	TargetNoun             string
	ProductionClass        string
	RuntimeContract        string
	InstallModel           string
	DevModel               string
	ActivationModel        string
	NativeRoot             string
	ImportSupport          bool
	RenderSupport          bool
	ValidateSupport        bool
	PortableComponentKinds []string
	TargetComponentKinds   []string
	Summary                string
}

type SDKMeta struct {
	PublicPackage   string
	InternalPackage string
	InternalImport  string
	Status          SupportStatus
	TransportModes  []TransportMode
	LiveTestProfile string
}

type LauncherMeta struct {
	Requirement LauncherRequirement
}

type NativeDocSpec struct {
	Kind        string
	Path        string
	Format      NativeDocFormat
	Role        NativeDocRole
	ManagedKeys []string
}

type ManagedArtifactSpec struct {
	Kind          ManagedArtifactKind
	Path          string
	ComponentKind string
	SourceRoot    string
	OutputRoot    string
	ContextMode   ContextStrategy
}

type SurfaceSupport struct {
	Kind string
	Tier SurfaceTier
}

type PlatformProfile struct {
	ID               string
	Contract         TargetContractMeta
	SDK              SDKMeta
	Launcher         LauncherMeta
	NativeDocs       []NativeDocSpec
	SurfaceTiers     []SurfaceSupport
	ManagedArtifacts []ManagedArtifactSpec
	Scaffold         ScaffoldMeta
	Validate         ValidateMeta
}

func All() []PlatformProfile {
	return []PlatformProfile{
		{
			ID: "claude",
			Contract: TargetContractMeta{
				PlatformFamily:         FamilyPackagedRuntime,
				TargetClass:            "hook_runtime",
				TargetNoun:             "plugin",
				ProductionClass:        "production-ready",
				RuntimeContract:        "public-stable stable-subset runtime",
				InstallModel:           "marketplace or local plugin install",
				DevModel:               "reload plugins",
				ActivationModel:        "reload or restart",
				NativeRoot:             "~/.claude/plugins/...",
				ImportSupport:          true,
				RenderSupport:          true,
				ValidateSupport:        true,
				PortableComponentKinds: []string{"skills", "mcp_servers"},
				TargetComponentKinds:   []string{"package_metadata", "hooks", "commands", "agents", "settings", "lsp", "user_config", "manifest_extra"},
				Summary:                "Claude plugin packages compile portable skills and MCP plus target-native hooks, commands, agents, settings, LSP, and user config.",
			},
			SDK: SDKMeta{
				PublicPackage:   "claude",
				InternalPackage: "claude",
				InternalImport:  "github.com/777genius/plugin-kit-ai/sdk/internal/platforms/claude",
				Status:          StatusRuntimeSupported,
				TransportModes:  []TransportMode{TransportProcess},
				LiveTestProfile: "claude_cli",
			},
			Launcher: LauncherMeta{Requirement: LauncherRequired},
			NativeDocs: []NativeDocSpec{
				{Kind: "package_metadata", Path: "targets/claude/package.yaml", Format: NativeDocYAML, Role: NativeDocRoleStructured},
				{Kind: "hooks", Path: "targets/claude/hooks/hooks.json", Format: NativeDocJSON, Role: NativeDocRoleStructured},
				{Kind: "settings", Path: "targets/claude/settings.json", Format: NativeDocJSON, Role: NativeDocRoleStructured},
				{Kind: "lsp", Path: "targets/claude/lsp.json", Format: NativeDocJSON, Role: NativeDocRoleStructured},
				{Kind: "user_config", Path: "targets/claude/user-config.json", Format: NativeDocJSON, Role: NativeDocRoleStructured},
				{Kind: "manifest_extra", Path: "targets/claude/manifest.extra.json", Format: NativeDocJSON, Role: NativeDocRoleExtra, ManagedKeys: []string{"name", "version", "description", "skills", "agents", "commands", "hooks", "mcpServers", "lspServers", "settings", "userConfig"}},
			},
			SurfaceTiers: []SurfaceSupport{
				{Kind: "hooks", Tier: SurfaceTierStable},
				{Kind: "commands", Tier: SurfaceTierStable},
				{Kind: "agents", Tier: SurfaceTierBeta},
				{Kind: "contexts", Tier: SurfaceTierUnsupported},
				{Kind: "settings", Tier: SurfaceTierStable},
				{Kind: "lsp", Tier: SurfaceTierBeta},
				{Kind: "user_config", Tier: SurfaceTierBeta},
				{Kind: "manifest_extra", Tier: SurfaceTierStable},
			},
			ManagedArtifacts: []ManagedArtifactSpec{
				{Kind: ManagedArtifactStatic, Path: ".claude-plugin/plugin.json"},
				{Kind: ManagedArtifactStatic, Path: "settings.json"},
				{Kind: ManagedArtifactStatic, Path: ".lsp.json"},
				{Kind: ManagedArtifactMirror, ComponentKind: "hooks", SourceRoot: "targets/claude/hooks", OutputRoot: "hooks"},
				{Kind: ManagedArtifactMirror, ComponentKind: "commands", SourceRoot: "targets/claude/commands", OutputRoot: "commands"},
				{Kind: ManagedArtifactMirror, ComponentKind: "agents", SourceRoot: "targets/claude/agents", OutputRoot: "agents"},
				{Kind: ManagedArtifactPortableMCP, Path: ".mcp.json"},
			},
			Scaffold: ScaffoldMeta{
				RequiredFiles: []string{
					"go.mod",
					"README.md",
					"plugin.yaml",
					"launcher.yaml",
					"targets/claude/hooks/hooks.json",
				},
				OptionalFiles: []string{
					"Makefile",
					".goreleaser.yml",
					"targets/claude/settings.json",
					"targets/claude/lsp.json",
					"targets/claude/user-config.json",
					"targets/claude/manifest.extra.json",
					"skills/{{.ProjectName}}/SKILL.md",
				},
				ForbiddenFiles: []string{
					"AGENTS.md",
				},
				TemplateFiles: []TemplateFile{
					{Path: "go.mod", Template: "go.mod.tmpl"},
					{Path: "cmd/{{.ProjectName}}/main.go", Template: "main.go.tmpl"},
					{Path: "plugin.yaml", Template: "plugin.yaml.tmpl"},
					{Path: "launcher.yaml", Template: "launcher.yaml.tmpl"},
					{Path: "targets/claude/hooks/hooks.json", Template: "targets.claude.hooks.json.tmpl"},
					{Path: "targets/claude/settings.json", Template: "empty.json.tmpl", Extra: true},
					{Path: "targets/claude/lsp.json", Template: "empty.json.tmpl", Extra: true},
					{Path: "targets/claude/user-config.json", Template: "empty.json.tmpl", Extra: true},
					{Path: "targets/claude/manifest.extra.json", Template: "empty.json.tmpl", Extra: true},
					{Path: "README.md", Template: "README.md.tmpl"},
					{Path: "Makefile", Template: "Makefile.tmpl", Extra: true},
					{Path: ".goreleaser.yml", Template: "goreleaser.yml.tmpl", Extra: true},
					{Path: "skills/{{.ProjectName}}/SKILL.md", Template: "SKILL.md.tmpl", Extra: true},
				},
			},
			Validate: ValidateMeta{
				RequiredFiles: []string{
					"go.mod",
					"README.md",
					"launcher.yaml",
					".claude-plugin/plugin.json",
					"hooks/hooks.json",
				},
				ForbiddenFiles: []string{
					"AGENTS.md",
					".codex/config.toml",
				},
				BuildTargets: []string{"./..."},
			},
		},
		{
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
				RenderSupport:          true,
				ValidateSupport:        true,
				PortableComponentKinds: []string{"skills", "mcp_servers"},
				TargetComponentKinds:   []string{"package_metadata", "manifest_extra", "app_manifest"},
				Summary:                "Codex package lane compiles the official plugin bundle: plugin.json plus optional MCP and app assets.",
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
				{Kind: "package_metadata", Path: "targets/codex-package/package.yaml", Format: NativeDocYAML, Role: NativeDocRoleStructured},
				{Kind: "manifest_extra", Path: "targets/codex-package/manifest.extra.json", Format: NativeDocJSON, Role: NativeDocRoleExtra, ManagedKeys: []string{"name", "version", "description", "skills", "mcpServers", "apps"}},
				{Kind: "app_manifest", Path: "targets/codex-package/app.json", Format: NativeDocJSON, Role: NativeDocRoleStructured},
			},
			SurfaceTiers: []SurfaceSupport{
				{Kind: "manifest_extra", Tier: SurfaceTierStable},
				{Kind: "app_manifest", Tier: SurfaceTierBeta},
				{Kind: "agents", Tier: SurfaceTierUnsupported},
				{Kind: "contexts", Tier: SurfaceTierUnsupported},
				{Kind: "commands", Tier: SurfaceTierUnsupported},
			},
			ManagedArtifacts: []ManagedArtifactSpec{
				{Kind: ManagedArtifactStatic, Path: ".codex-plugin/plugin.json"},
				{Kind: ManagedArtifactStatic, Path: ".app.json"},
				{Kind: ManagedArtifactPortableMCP, Path: ".mcp.json"},
			},
			Scaffold: ScaffoldMeta{
				RequiredFiles: []string{
					"plugin.yaml",
					"README.md",
					"targets/codex-package/package.yaml",
				},
				OptionalFiles: []string{
					"skills/{{.ProjectName}}/SKILL.md",
				},
				ForbiddenFiles: []string{
					"launcher.yaml",
					".codex/config.toml",
					"AGENTS.md",
				},
				TemplateFiles: []TemplateFile{
					{Path: "plugin.yaml", Template: "plugin.yaml.tmpl"},
					{Path: "targets/codex-package/package.yaml", Template: "targets.codex-package.package.yaml.tmpl"},
					{Path: "README.md", Template: "codex-package.README.md.tmpl"},
					{Path: "skills/{{.ProjectName}}/SKILL.md", Template: "SKILL.md.tmpl", Extra: true},
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
					"AGENTS.md",
				},
			},
		},
		{
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
				RenderSupport:          true,
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
				{Kind: "package_metadata", Path: "targets/codex-runtime/package.yaml", Format: NativeDocYAML, Role: NativeDocRoleStructured},
				{Kind: "config_extra", Path: "targets/codex-runtime/config.extra.toml", Format: NativeDocTOML, Role: NativeDocRoleExtra, ManagedKeys: []string{"model", "notify"}},
			},
			SurfaceTiers: []SurfaceSupport{
				{Kind: "config_extra", Tier: SurfaceTierStable},
				{Kind: "commands", Tier: SurfaceTierBeta},
				{Kind: "contexts", Tier: SurfaceTierBeta},
			},
			ManagedArtifacts: []ManagedArtifactSpec{
				{Kind: ManagedArtifactStatic, Path: ".codex/config.toml"},
				{Kind: ManagedArtifactMirror, ComponentKind: "commands", SourceRoot: "targets/codex-runtime/commands", OutputRoot: "commands"},
				{Kind: ManagedArtifactMirror, ComponentKind: "contexts", SourceRoot: "targets/codex-runtime/contexts", OutputRoot: "contexts"},
			},
			Scaffold: ScaffoldMeta{
				RequiredFiles: []string{
					"go.mod",
					"README.md",
					"plugin.yaml",
					"launcher.yaml",
					"targets/codex-runtime/package.yaml",
				},
				OptionalFiles: []string{
					"Makefile",
					".goreleaser.yml",
				},
				ForbiddenFiles: []string{
					".claude-plugin/plugin.json",
					"hooks/hooks.json",
				},
				TemplateFiles: []TemplateFile{
					{Path: "go.mod", Template: "codex.go.mod.tmpl"},
					{Path: "cmd/{{.ProjectName}}/main.go", Template: "codex.main.go.tmpl"},
					{Path: "plugin.yaml", Template: "plugin.yaml.tmpl"},
					{Path: "launcher.yaml", Template: "launcher.yaml.tmpl"},
					{Path: "targets/codex-runtime/package.yaml", Template: "targets.codex-runtime.package.yaml.tmpl"},
					{Path: "README.md", Template: "codex-runtime.README.md.tmpl"},
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
		},
		{
			ID: "gemini",
			Contract: TargetContractMeta{
				PlatformFamily:         FamilyExtensionPackage,
				TargetClass:            "mcp_extension",
				TargetNoun:             "extension",
				ProductionClass:        "packaging-only target",
				RuntimeContract:        "not a production-ready runtime target",
				InstallModel:           "copy install",
				DevModel:               "link",
				ActivationModel:        "restart required",
				NativeRoot:             "~/.gemini/extensions/<name>",
				ImportSupport:          true,
				RenderSupport:          true,
				ValidateSupport:        true,
				PortableComponentKinds: []string{"skills", "mcp_servers"},
				TargetComponentKinds:   []string{"package_metadata", "hooks", "commands", "policies", "themes", "settings", "contexts", "manifest_extra"},
				Summary:                "Gemini compiles as an official-style extension package with MCP, a primary root context, and target-native extension assets.",
			},
			SDK: SDKMeta{
				PublicPackage:   "gemini",
				InternalPackage: "gemini",
				InternalImport:  "github.com/777genius/plugin-kit-ai/sdk/internal/platforms/gemini",
				Status:          StatusScaffoldOnly,
				TransportModes:  []TransportMode{TransportProcess},
				LiveTestProfile: "gemini_extension",
			},
			Launcher: LauncherMeta{Requirement: LauncherIgnored},
			NativeDocs: []NativeDocSpec{
				{Kind: "package_metadata", Path: "targets/gemini/package.yaml", Format: NativeDocYAML, Role: NativeDocRoleStructured},
				{Kind: "manifest_extra", Path: "targets/gemini/manifest.extra.json", Format: NativeDocJSON, Role: NativeDocRoleExtra, ManagedKeys: []string{"name", "version", "description", "mcpServers", "contextFileName", "excludeTools", "migratedTo", "settings", "themes", "plan.directory"}},
			},
			SurfaceTiers: []SurfaceSupport{
				{Kind: "commands", Tier: SurfaceTierStable},
				{Kind: "hooks", Tier: SurfaceTierStable},
				{Kind: "policies", Tier: SurfaceTierStable},
				{Kind: "settings", Tier: SurfaceTierStable},
				{Kind: "themes", Tier: SurfaceTierStable},
				{Kind: "contexts", Tier: SurfaceTierStable},
				{Kind: "manifest_extra", Tier: SurfaceTierStable},
				{Kind: "agents", Tier: SurfaceTierPreview},
			},
			ManagedArtifacts: []ManagedArtifactSpec{
				{Kind: ManagedArtifactStatic, Path: "gemini-extension.json"},
				{Kind: ManagedArtifactMirror, ComponentKind: "hooks", SourceRoot: "targets/gemini/hooks", OutputRoot: "hooks"},
				{Kind: ManagedArtifactMirror, ComponentKind: "commands", SourceRoot: "targets/gemini/commands", OutputRoot: "commands"},
				{Kind: ManagedArtifactMirror, ComponentKind: "policies", SourceRoot: "targets/gemini/policies", OutputRoot: "policies"},
				{Kind: ManagedArtifactMirror, ComponentKind: "contexts", SourceRoot: "targets/gemini/contexts", OutputRoot: "contexts"},
				{Kind: ManagedArtifactSelectedContext, ComponentKind: "contexts", OutputRoot: "", ContextMode: ContextStrategyGeminiPrimaryRoot},
			},
			Scaffold: ScaffoldMeta{
				RequiredFiles: []string{
					"plugin.yaml",
					"targets/gemini/package.yaml",
					"targets/gemini/contexts/GEMINI.md",
					"README.md",
				},
				OptionalFiles: []string{
					"Makefile",
					".goreleaser.yml",
					"skills/{{.ProjectName}}/SKILL.md",
				},
				TemplateFiles: []TemplateFile{
					{Path: "plugin.yaml", Template: "plugin.yaml.tmpl"},
					{Path: "targets/gemini/package.yaml", Template: "targets.gemini.package.yaml.tmpl"},
					{Path: "targets/gemini/contexts/GEMINI.md", Template: "gemini.GEMINI.md.tmpl"},
					{Path: "README.md", Template: "gemini.README.md.tmpl"},
					{Path: "Makefile", Template: "Makefile.tmpl", Extra: true},
					{Path: ".goreleaser.yml", Template: "goreleaser.yml.tmpl", Extra: true},
					{Path: "skills/{{.ProjectName}}/SKILL.md", Template: "SKILL.md.tmpl", Extra: true},
				},
			},
			Validate: ValidateMeta{
				RequiredFiles: []string{
					"plugin.yaml",
					"targets/gemini/package.yaml",
				},
			},
		},
		{
			ID: "opencode",
			Contract: TargetContractMeta{
				PlatformFamily:         FamilyCodePlugin,
				TargetClass:            "workspace_config_lane",
				TargetNoun:             "workspace",
				ProductionClass:        "packaging-only target",
				RuntimeContract:        "workspace-config lane with first-class npm plugin refs, MCP, skills, commands, agents, themes, beta standalone tools with dedicated live evidence, stable official-style local JS/TS plugins plus shared dependency metadata, JSON/JSONC native import, explicit user-scope and env-config import compatibility, permission-first passthrough config semantics, deprecated tools-config compatibility passthrough, and beta custom tools across standalone tools and plugin code",
				InstallModel:           "workspace config file",
				DevModel:               "config authoring workspace",
				ActivationModel:        "config reload or restart",
				NativeRoot:             "opencode.json",
				ImportSupport:          true,
				RenderSupport:          true,
				ValidateSupport:        true,
				PortableComponentKinds: []string{"skills", "mcp_servers"},
				TargetComponentKinds:   []string{"package_metadata", "config_extra", "commands", "agents", "themes", "tools", "local_plugin_code", "local_plugin_dependencies"},
				Summary:                "OpenCode compiles as a workspace-config lane with canonical repo-local authored outputs for npm plugin refs, shared MCP, skills, commands, agents, themes, beta standalone tools with their own live-evidence path, stable official-style local JS/TS plugins plus shared package metadata for tools and plugins, layered JSON/JSONC import compatibility across project, explicit user scope, and env-config sources, beta custom tools across standalone tools and plugin code, and passthrough support for broader permission-first config-only surfaces.",
			},
			SDK: SDKMeta{
				PublicPackage:   "opencode",
				InternalPackage: "opencode",
				InternalImport:  "github.com/777genius/plugin-kit-ai/sdk/internal/platforms/opencode",
				Status:          StatusScaffoldOnly,
				TransportModes:  []TransportMode{TransportProcess},
				LiveTestProfile: "opencode_workspace",
			},
			Launcher: LauncherMeta{Requirement: LauncherIgnored},
			NativeDocs: []NativeDocSpec{
				{Kind: "package_metadata", Path: "targets/opencode/package.yaml", Format: NativeDocYAML, Role: NativeDocRoleStructured},
				{Kind: "config_extra", Path: "targets/opencode/config.extra.json", Format: NativeDocJSON, Role: NativeDocRoleExtra, ManagedKeys: []string{"$schema", "plugin", "mcp"}},
				{Kind: "local_plugin_dependencies", Path: "targets/opencode/package.json", Format: NativeDocJSON, Role: NativeDocRoleStructured},
			},
			SurfaceTiers: []SurfaceSupport{
				{Kind: "plugins", Tier: SurfaceTierStable},
				{Kind: "mcp", Tier: SurfaceTierStable},
				{Kind: "skills", Tier: SurfaceTierStable},
				{Kind: "commands", Tier: SurfaceTierStable},
				{Kind: "agents", Tier: SurfaceTierStable},
				{Kind: "themes", Tier: SurfaceTierStable},
				{Kind: "tools", Tier: SurfaceTierBeta},
				{Kind: "config_extra", Tier: SurfaceTierStable},
				{Kind: "agent_config", Tier: SurfaceTierPassthroughOnly},
				{Kind: "permission_config", Tier: SurfaceTierPassthroughOnly},
				{Kind: "instructions_config", Tier: SurfaceTierPassthroughOnly},
				{Kind: "tools_config", Tier: SurfaceTierPassthroughOnly},
				{Kind: "modes", Tier: SurfaceTierUnsupported},
				{Kind: "local_plugin_code", Tier: SurfaceTierStable},
				{Kind: "custom_tools", Tier: SurfaceTierBeta},
				{Kind: "local_plugin_dependencies", Tier: SurfaceTierStable},
			},
			ManagedArtifacts: []ManagedArtifactSpec{
				{Kind: ManagedArtifactStatic, Path: "opencode.json"},
				{Kind: ManagedArtifactStatic, Path: ".opencode/package.json"},
				{Kind: ManagedArtifactPortableSkills, OutputRoot: ".opencode/skills"},
				{Kind: ManagedArtifactMirror, ComponentKind: "commands", SourceRoot: "targets/opencode/commands", OutputRoot: ".opencode/commands"},
				{Kind: ManagedArtifactMirror, ComponentKind: "agents", SourceRoot: "targets/opencode/agents", OutputRoot: ".opencode/agents"},
				{Kind: ManagedArtifactMirror, ComponentKind: "themes", SourceRoot: "targets/opencode/themes", OutputRoot: ".opencode/themes"},
				{Kind: ManagedArtifactMirror, ComponentKind: "tools", SourceRoot: "targets/opencode/tools", OutputRoot: ".opencode/tools"},
				{Kind: ManagedArtifactMirror, ComponentKind: "local_plugin_code", SourceRoot: "targets/opencode/plugins", OutputRoot: ".opencode/plugins"},
			},
			Scaffold: ScaffoldMeta{
				RequiredFiles: []string{
					"plugin.yaml",
					"targets/opencode/package.yaml",
					"README.md",
				},
				OptionalFiles: []string{
					"targets/opencode/config.extra.json",
					"skills/{{.ProjectName}}/SKILL.md",
					"targets/opencode/commands/{{.ProjectName}}.md",
					"targets/opencode/agents/{{.ProjectName}}.md",
					"targets/opencode/themes/{{.ProjectName}}.json",
					"targets/opencode/tools/{{.ProjectName}}.ts",
					"targets/opencode/plugins/example.js",
					"targets/opencode/package.json",
				},
				ForbiddenFiles: []string{
					"launcher.yaml",
				},
				TemplateFiles: []TemplateFile{
					{Path: "plugin.yaml", Template: "plugin.yaml.tmpl"},
					{Path: "targets/opencode/package.yaml", Template: "targets.opencode.package.yaml.tmpl"},
					{Path: "targets/opencode/config.extra.json", Template: "empty.json.tmpl", Extra: true},
					{Path: "README.md", Template: "opencode.README.md.tmpl"},
					{Path: "skills/{{.ProjectName}}/SKILL.md", Template: "opencode.SKILL.md.tmpl", Extra: true},
					{Path: "targets/opencode/commands/{{.ProjectName}}.md", Template: "opencode.command.md.tmpl", Extra: true},
					{Path: "targets/opencode/agents/{{.ProjectName}}.md", Template: "opencode.agent.md.tmpl", Extra: true},
					{Path: "targets/opencode/themes/{{.ProjectName}}.json", Template: "opencode.theme.json.tmpl", Extra: true},
					{Path: "targets/opencode/tools/{{.ProjectName}}.ts", Template: "opencode.tool.ts.tmpl", Extra: true},
					{Path: "targets/opencode/plugins/example.js", Template: "opencode.plugin.js.tmpl", Extra: true},
					{Path: "targets/opencode/package.json", Template: "opencode.package.json.tmpl", Extra: true},
				},
			},
			Validate: ValidateMeta{
				RequiredFiles: []string{
					"plugin.yaml",
					"targets/opencode/package.yaml",
				},
				ForbiddenFiles: []string{
					"launcher.yaml",
				},
			},
		},
	}
}

func Lookup(name string) (PlatformProfile, bool) {
	name = normalizeName(name)
	for _, profile := range All() {
		if profile.ID == name {
			return profile, true
		}
	}
	return PlatformProfile{}, false
}

func IDs() []string {
	profiles := All()
	out := make([]string, 0, len(profiles))
	for _, profile := range profiles {
		out = append(out, profile.ID)
	}
	return out
}

func normalizeName(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}
