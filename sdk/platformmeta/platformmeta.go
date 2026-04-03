package platformmeta

import "strings"

// SupportStatus describes the lifecycle status of a platform profile.
type SupportStatus string

// TransportMode describes how a plugin talks to the target platform.
type TransportMode string

// PlatformFamily groups targets by their broad integration model.
type PlatformFamily string

// LauncherRequirement indicates whether a launcher file is required.
type LauncherRequirement string

// NativeDocRole classifies target-native docs as structured or extra.
type NativeDocRole string

// NativeDocFormat identifies the file format of target-native docs.
type NativeDocFormat string

// ManagedArtifactKind describes how plugin-kit-ai manages generated artifacts.
type ManagedArtifactKind string

// ContextStrategy describes how contextual files are selected or projected.
type ContextStrategy string

// SurfaceTier describes the maturity of a target-native surface area.
type SurfaceTier string

const (
	// StatusRuntimeSupported means the platform has a supported runtime contract.
	StatusRuntimeSupported SupportStatus = "runtime_supported"
	// StatusScaffoldOnly means the platform supports scaffold output but not runtime dispatch.
	StatusScaffoldOnly SupportStatus = "scaffold_only"
	// StatusDeferred means the platform is modeled but intentionally deferred.
	StatusDeferred SupportStatus = "deferred"
)

const (
	// TransportProcess uses direct process execution for hook dispatch.
	TransportProcess TransportMode = "process"
	// TransportHybrid combines process execution with target-specific helpers.
	TransportHybrid TransportMode = "hybrid"
	// TransportDaemon uses a long-lived daemon or service boundary.
	TransportDaemon TransportMode = "daemon"
)

const (
	// FamilyPackagedRuntime describes packaged runtime plugins.
	FamilyPackagedRuntime PlatformFamily = "packaged_runtime"
	// FamilyExtensionPackage describes extension or IDE package targets.
	FamilyExtensionPackage PlatformFamily = "extension_package"
	// FamilyCodePlugin describes repo-local code plugins.
	FamilyCodePlugin PlatformFamily = "code_plugin"
	// FamilyIDEPlugin describes IDE plugin targets with dedicated shells.
	FamilyIDEPlugin PlatformFamily = "ide_plugin"
)

const (
	// LauncherRequired means launcher.yaml is required for the target.
	LauncherRequired LauncherRequirement = "required"
	// LauncherOptional means launcher.yaml may be present but is not required.
	LauncherOptional LauncherRequirement = "optional"
	// LauncherIgnored means launcher.yaml is not used for the target.
	LauncherIgnored LauncherRequirement = "ignored"
)

const (
	// NativeDocRoleStructured marks primary structured target config files.
	NativeDocRoleStructured NativeDocRole = "structured"
	// NativeDocRoleExtra marks extra passthrough docs not fully modeled by the SDK.
	NativeDocRoleExtra NativeDocRole = "extra"
)

const (
	// NativeDocYAML identifies YAML native docs.
	NativeDocYAML NativeDocFormat = "yaml"
	// NativeDocJSON identifies JSON native docs.
	NativeDocJSON NativeDocFormat = "json"
	// NativeDocTOML identifies TOML native docs.
	NativeDocTOML NativeDocFormat = "toml"
	// NativeDocMarkdown identifies Markdown native docs.
	NativeDocMarkdown NativeDocFormat = "md"
)

const (
	// ManagedArtifactStatic describes a static generated file.
	ManagedArtifactStatic ManagedArtifactKind = "static"
	// ManagedArtifactMirror describes a mirrored source-to-output tree.
	ManagedArtifactMirror ManagedArtifactKind = "mirror"
	// ManagedArtifactPortableMCP describes a portable MCP manifest artifact.
	ManagedArtifactPortableMCP ManagedArtifactKind = "portable_mcp"
	// ManagedArtifactPortableSkills describes generated portable skill output.
	ManagedArtifactPortableSkills ManagedArtifactKind = "portable_skills"
	// ManagedArtifactSelectedContext describes context files selected from source material.
	ManagedArtifactSelectedContext ManagedArtifactKind = "selected_context"
)

const (
	// ContextStrategyNone means no special context projection is required.
	ContextStrategyNone ContextStrategy = ""
	// ContextStrategyGeminiPrimaryRoot selects Gemini's primary context root strategy.
	ContextStrategyGeminiPrimaryRoot ContextStrategy = "gemini_primary_root"
)

const (
	// SurfaceTierStable marks a stable public surface.
	SurfaceTierStable SurfaceTier = "stable"
	// SurfaceTierBeta marks a beta public surface.
	SurfaceTierBeta SurfaceTier = "beta"
	// SurfaceTierPreview marks a preview-only surface.
	SurfaceTierPreview SurfaceTier = "preview"
	// SurfaceTierPassthroughOnly marks surfaces that exist only as passthrough artifacts.
	SurfaceTierPassthroughOnly SurfaceTier = "passthrough_only"
	// SurfaceTierUnsupported marks unsupported surfaces that should not be relied on.
	SurfaceTierUnsupported SurfaceTier = "unsupported"
)

// TemplateFile describes a scaffolded output file and its template source.
type TemplateFile struct {
	// Path is the destination path inside the generated project.
	Path string
	// Template is the template file used to render the destination.
	Template string
	// Extra marks optional scaffold material that is not required by default.
	Extra bool
}

// ScaffoldMeta describes the generated file set for `plugin-kit-ai init`.
type ScaffoldMeta struct {
	// RequiredFiles must exist in a scaffolded target.
	RequiredFiles []string
	// OptionalFiles may be added for richer scaffolds.
	OptionalFiles []string
	// ForbiddenFiles must be absent for a valid target layout.
	ForbiddenFiles []string
	// TemplateFiles maps scaffold output files to their render templates.
	TemplateFiles []TemplateFile
}

// ValidateMeta describes the contract enforced by `plugin-kit-ai validate`.
type ValidateMeta struct {
	// RequiredFiles must exist for the target to validate successfully.
	RequiredFiles []string
	// ForbiddenFiles must not exist for the target to validate successfully.
	ForbiddenFiles []string
	// BuildTargets lists buildable artifacts that validation should check.
	BuildTargets []string
}

// TargetContractMeta describes the author-facing contract for a platform target.
type TargetContractMeta struct {
	// PlatformFamily groups the target into a broad integration family.
	PlatformFamily PlatformFamily
	// TargetClass names the internal target class used by docs and scaffolds.
	TargetClass string
	// TargetNoun is the user-facing noun for the produced artifact.
	TargetNoun string
	// ProductionClass summarizes the intended production posture.
	ProductionClass string
	// RuntimeContract describes the public runtime support promise.
	RuntimeContract string
	// InstallModel describes how the target is installed by end users.
	InstallModel string
	// DevModel describes the expected local development loop.
	DevModel string
	// ActivationModel describes how changes become active in the target.
	ActivationModel string
	// NativeRoot points to the target's native install or config root.
	NativeRoot string
	// ImportSupport reports whether `plugin-kit-ai import` is supported.
	ImportSupport bool
	// RenderSupport reports whether `plugin-kit-ai render` is supported.
	RenderSupport bool
	// ValidateSupport reports whether `plugin-kit-ai validate` is supported.
	ValidateSupport bool
	// PortableComponentKinds lists portable authoring components the target consumes.
	PortableComponentKinds []string
	// TargetComponentKinds lists native target components rendered for the target.
	TargetComponentKinds []string
	// Summary provides the high-level target description used in docs.
	Summary string
}

// SDKMeta describes the runtime SDK package associated with a platform target.
type SDKMeta struct {
	// PublicPackage is the public SDK package name for the target.
	PublicPackage string
	// InternalPackage is the internal runtime package name used by generators.
	InternalPackage string
	// InternalImport is the import path for the internal runtime package.
	InternalImport string
	// Status is the support status for the target's runtime lane.
	Status SupportStatus
	// TransportModes lists the supported runtime transport modes.
	TransportModes []TransportMode
	// LiveTestProfile names the live integration test profile for the target.
	LiveTestProfile string
}

// LauncherMeta describes whether a launcher is required for the target.
type LauncherMeta struct {
	Requirement LauncherRequirement
}

// NativeDocSpec describes one target-native config file or manifest surface.
type NativeDocSpec struct {
	// Kind is the normalized component kind represented by the native file.
	Kind string
	// Path is the target-relative path to the native file.
	Path string
	// Format is the native file format.
	Format NativeDocFormat
	// Role describes whether the file is structured or extra.
	Role NativeDocRole
	// ManagedKeys lists keys that plugin-kit-ai manages in extra docs.
	ManagedKeys []string
}

// ManagedArtifactSpec describes an artifact managed or mirrored by the toolchain.
type ManagedArtifactSpec struct {
	// Kind describes how the artifact is managed.
	Kind ManagedArtifactKind
	// Path is the file path for single-file artifacts.
	Path string
	// ComponentKind identifies the component family for mirrored artifacts.
	ComponentKind string
	// SourceRoot is the source directory for mirrored artifacts.
	SourceRoot string
	// OutputRoot is the destination directory for mirrored artifacts.
	OutputRoot string
	// ContextMode controls how contextual sources are selected.
	ContextMode ContextStrategy
}

// SurfaceSupport describes the maturity tier for one surface kind.
type SurfaceSupport struct {
	// Kind is the normalized surface kind name.
	Kind string
	// Tier is the maturity tier for that surface kind.
	Tier SurfaceTier
}

// PlatformProfile collects the public metadata for one supported target platform.
type PlatformProfile struct {
	// ID is the normalized platform identifier.
	ID string
	// Contract describes the author-facing target contract.
	Contract TargetContractMeta
	// SDK describes the runtime SDK metadata for the target.
	SDK SDKMeta
	// Launcher describes launcher requirements for the target.
	Launcher LauncherMeta
	// NativeDocs enumerates target-native config files and manifests.
	NativeDocs []NativeDocSpec
	// SurfaceTiers enumerates maturity tiers for target-native surfaces.
	SurfaceTiers []SurfaceSupport
	// ManagedArtifacts enumerates generated or mirrored artifacts.
	ManagedArtifacts []ManagedArtifactSpec
	// Scaffold describes `init` output for the target.
	Scaffold ScaffoldMeta
	// Validate describes `validate` rules for the target.
	Validate ValidateMeta
}

// All returns the full set of public platform profiles.
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
				ProductionClass:        "runtime-supported beta extension target",
				RuntimeContract:        "Gemini Go runtime beta lane plus full extension packaging lane; not production-ready",
				InstallModel:           "copy install",
				DevModel:               "link",
				ActivationModel:        "restart required",
				NativeRoot:             "~/.gemini/extensions/<name>",
				ImportSupport:          true,
				RenderSupport:          true,
				ValidateSupport:        true,
				PortableComponentKinds: []string{"skills", "mcp_servers"},
				TargetComponentKinds:   []string{"package_metadata", "hooks", "commands", "policies", "themes", "settings", "contexts", "manifest_extra"},
				Summary:                "Gemini compiles as an official-style extension package with MCP, a primary root context, target-native extension assets, and an optional Go hook runtime lane.",
			},
			SDK: SDKMeta{
				PublicPackage:   "gemini",
				InternalPackage: "gemini",
				InternalImport:  "github.com/777genius/plugin-kit-ai/sdk/internal/platforms/gemini",
				Status:          StatusRuntimeSupported,
				TransportModes:  []TransportMode{TransportProcess},
				LiveTestProfile: "gemini_extension",
			},
			Launcher: LauncherMeta{Requirement: LauncherOptional},
			NativeDocs: []NativeDocSpec{
				{Kind: "package_metadata", Path: "targets/gemini/package.yaml", Format: NativeDocYAML, Role: NativeDocRoleStructured},
				{Kind: "manifest_extra", Path: "targets/gemini/manifest.extra.json", Format: NativeDocJSON, Role: NativeDocRoleExtra, ManagedKeys: []string{"name", "version", "description", "mcpServers", "contextFileName", "excludeTools", "settings", "themes", "plan.directory"}},
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
			ID: "cursor",
			Contract: TargetContractMeta{
				PlatformFamily:         FamilyCodePlugin,
				TargetClass:            "workspace_config_lane",
				TargetNoun:             "workspace",
				ProductionClass:        "packaging-only target",
				RuntimeContract:        "workspace-config lane with first-class MCP config, project rules, and optional root AGENTS.md support",
				InstallModel:           "workspace config files",
				DevModel:               "config authoring workspace",
				ActivationModel:        "config reload or restart",
				NativeRoot:             ".cursor/mcp.json",
				ImportSupport:          true,
				RenderSupport:          true,
				ValidateSupport:        true,
				PortableComponentKinds: []string{"mcp_servers"},
				TargetComponentKinds:   []string{"rules", "agents_md"},
				Summary:                "Cursor compiles as a workspace-config lane with repo-local MCP, project rules, optional root AGENTS.md support, and a strict documented-subset contract.",
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
				{Kind: "agents_md", Path: "targets/cursor/AGENTS.md", Format: NativeDocMarkdown, Role: NativeDocRoleStructured},
			},
			SurfaceTiers: []SurfaceSupport{
				{Kind: "mcp", Tier: SurfaceTierStable},
				{Kind: "rules", Tier: SurfaceTierStable},
				{Kind: "agents_md", Tier: SurfaceTierStable},
				{Kind: "claude_md", Tier: SurfaceTierUnsupported},
			},
			ManagedArtifacts: []ManagedArtifactSpec{
				{Kind: ManagedArtifactPortableMCP, Path: ".cursor/mcp.json"},
				{Kind: ManagedArtifactMirror, ComponentKind: "rules", SourceRoot: "targets/cursor/rules", OutputRoot: ".cursor/rules"},
				{Kind: ManagedArtifactMirror, ComponentKind: "agents_md", SourceRoot: "targets/cursor", OutputRoot: ""},
			},
			Scaffold: ScaffoldMeta{
				RequiredFiles: []string{
					"plugin.yaml",
					"README.md",
					"targets/cursor/rules/project.mdc",
				},
				OptionalFiles: []string{
					"targets/cursor/AGENTS.md",
				},
				ForbiddenFiles: []string{
					"launcher.yaml",
				},
				TemplateFiles: []TemplateFile{
					{Path: "plugin.yaml", Template: "plugin.yaml.tmpl"},
					{Path: "README.md", Template: "cursor.README.md.tmpl"},
					{Path: "targets/cursor/rules/project.mdc", Template: "cursor.rule.mdc.tmpl"},
					{Path: "targets/cursor/AGENTS.md", Template: "cursor.AGENTS.md.tmpl", Extra: true},
				},
			},
			Validate: ValidateMeta{
				RequiredFiles: []string{
					"plugin.yaml",
				},
				ForbiddenFiles: []string{
					"launcher.yaml",
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
				RuntimeContract:        "workspace-config lane with first-class npm plugin refs, MCP, skills, commands, agents, themes, beta standalone tools with dedicated live evidence, stable official-style local JS/TS plugins plus shared dependency metadata, JSON/JSONC native import, explicit user-scope import, permission-first passthrough config semantics, deprecated tools-config compatibility passthrough, and beta custom tools across standalone tools and plugin code",
				InstallModel:           "workspace config file",
				DevModel:               "config authoring workspace",
				ActivationModel:        "config reload or restart",
				NativeRoot:             "opencode.json",
				ImportSupport:          true,
				RenderSupport:          true,
				ValidateSupport:        true,
				PortableComponentKinds: []string{"skills", "mcp_servers"},
				TargetComponentKinds:   []string{"package_metadata", "config_extra", "commands", "agents", "themes", "tools", "local_plugin_code", "local_plugin_dependencies"},
				Summary:                "OpenCode compiles as a workspace-config lane with canonical repo-local authored outputs for npm plugin refs, shared MCP, skills, commands, agents, themes, beta standalone tools with their own live-evidence path, stable official-style local JS/TS plugins plus shared package metadata for tools and plugins, layered JSON/JSONC import compatibility across project and explicit user scope, beta custom tools across standalone tools and plugin code, and passthrough support for broader permission-first config-only surfaces.",
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

// Lookup resolves a platform profile by normalized platform name.
func Lookup(name string) (PlatformProfile, bool) {
	name = normalizeName(name)
	for _, profile := range All() {
		if profile.ID == name {
			return profile, true
		}
	}
	return PlatformProfile{}, false
}

// IDs returns the normalized identifiers for every known platform profile.
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
