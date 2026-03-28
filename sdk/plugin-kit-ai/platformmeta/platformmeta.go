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
	ManagedArtifactSelectedContext ManagedArtifactKind = "selected_context"
)

const (
	ContextStrategyNone              ContextStrategy = ""
	ContextStrategyGeminiPrimaryRoot ContextStrategy = "gemini_primary_root"
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

type PlatformProfile struct {
	ID               string
	Contract         TargetContractMeta
	SDK              SDKMeta
	Launcher         LauncherMeta
	NativeDocs       []NativeDocSpec
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
				PortableComponentKinds: []string{"skills", "mcp_servers", "agents", "contexts"},
				TargetComponentKinds:   []string{"package_metadata", "hooks", "commands", "contexts"},
				Summary:                "Claude plugin packages compile portable skills and MCP plus target-native hook bindings.",
			},
			SDK: SDKMeta{
				PublicPackage:   "claude",
				InternalPackage: "claude",
				InternalImport:  "github.com/plugin-kit-ai/plugin-kit-ai/sdk/internal/platforms/claude",
				Status:          StatusRuntimeSupported,
				TransportModes:  []TransportMode{TransportProcess},
				LiveTestProfile: "claude_cli",
			},
			Launcher: LauncherMeta{Requirement: LauncherRequired},
			NativeDocs: []NativeDocSpec{
				{Kind: "package_metadata", Path: "targets/claude/package.yaml", Format: NativeDocYAML, Role: NativeDocRoleStructured},
				{Kind: "hooks", Path: "targets/claude/hooks/hooks.json", Format: NativeDocJSON, Role: NativeDocRoleStructured},
			},
			ManagedArtifacts: []ManagedArtifactSpec{
				{Kind: ManagedArtifactStatic, Path: ".claude-plugin/plugin.json"},
				{Kind: ManagedArtifactMirror, ComponentKind: "hooks", SourceRoot: "targets/claude/hooks", OutputRoot: "hooks"},
				{Kind: ManagedArtifactMirror, ComponentKind: "commands", SourceRoot: "targets/claude/commands", OutputRoot: "commands"},
				{Kind: ManagedArtifactMirror, ComponentKind: "contexts", SourceRoot: "targets/claude/contexts", OutputRoot: "contexts"},
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
			ID: "codex",
			Contract: TargetContractMeta{
				PlatformFamily:         FamilyPackagedRuntime,
				TargetClass:            "mixed_package_runtime",
				TargetNoun:             "plugin",
				ProductionClass:        "production-ready",
				RuntimeContract:        "public-stable notify runtime",
				InstallModel:           "plugin directory or marketplace cache",
				DevModel:               "local plugin workspace",
				ActivationModel:        "config reload or restart",
				NativeRoot:             "~/.codex/plugins/...",
				ImportSupport:          true,
				RenderSupport:          true,
				ValidateSupport:        true,
				PortableComponentKinds: []string{"skills", "mcp_servers", "contexts"},
				TargetComponentKinds:   []string{"package_metadata", "commands", "contexts", "manifest_extra", "config_extra"},
				Summary:                "Codex packages compile portable skills and MCP plus target metadata such as model hints and native extra-doc passthrough.",
			},
			SDK: SDKMeta{
				PublicPackage:   "codex",
				InternalPackage: "codex",
				InternalImport:  "github.com/plugin-kit-ai/plugin-kit-ai/sdk/internal/platforms/codex",
				Status:          StatusRuntimeSupported,
				TransportModes:  []TransportMode{TransportProcess},
				LiveTestProfile: "codex_notify",
			},
			Launcher: LauncherMeta{Requirement: LauncherRequired},
			NativeDocs: []NativeDocSpec{
				{Kind: "package_metadata", Path: "targets/codex/package.yaml", Format: NativeDocYAML, Role: NativeDocRoleStructured},
				{Kind: "manifest_extra", Path: "targets/codex/manifest.extra.json", Format: NativeDocJSON, Role: NativeDocRoleExtra, ManagedKeys: []string{"name", "version", "description", "skills", "mcpServers"}},
				{Kind: "config_extra", Path: "targets/codex/config.extra.toml", Format: NativeDocTOML, Role: NativeDocRoleExtra, ManagedKeys: []string{"model", "notify"}},
			},
			ManagedArtifacts: []ManagedArtifactSpec{
				{Kind: ManagedArtifactStatic, Path: ".codex-plugin/plugin.json"},
				{Kind: ManagedArtifactStatic, Path: ".codex/config.toml"},
				{Kind: ManagedArtifactMirror, ComponentKind: "commands", SourceRoot: "targets/codex/commands", OutputRoot: "commands"},
				{Kind: ManagedArtifactMirror, ComponentKind: "contexts", SourceRoot: "targets/codex/contexts", OutputRoot: "contexts"},
				{Kind: ManagedArtifactPortableMCP, Path: ".mcp.json"},
			},
			Scaffold: ScaffoldMeta{
				RequiredFiles: []string{
					"go.mod",
					"README.md",
					"plugin.yaml",
					"launcher.yaml",
					"AGENTS.md",
					"targets/codex/package.yaml",
				},
				OptionalFiles: []string{
					"Makefile",
					".goreleaser.yml",
					"skills/{{.ProjectName}}/SKILL.md",
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
					{Path: "targets/codex/package.yaml", Template: "targets.codex.package.yaml.tmpl"},
					{Path: "AGENTS.md", Template: "codex.AGENTS.md.tmpl"},
					{Path: "README.md", Template: "codex.README.md.tmpl"},
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
					"AGENTS.md",
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
				PortableComponentKinds: []string{"skills", "mcp_servers", "agents", "contexts"},
				TargetComponentKinds:   []string{"package_metadata", "hooks", "commands", "policies", "themes", "settings", "contexts", "manifest_extra"},
				Summary:                "Gemini compiles as an official-style extension package with MCP, a primary root context, and target-native extension assets.",
			},
			SDK: SDKMeta{
				PublicPackage:   "gemini",
				InternalPackage: "gemini",
				InternalImport:  "github.com/plugin-kit-ai/plugin-kit-ai/sdk/internal/platforms/gemini",
				Status:          StatusScaffoldOnly,
				TransportModes:  []TransportMode{TransportProcess},
				LiveTestProfile: "gemini_extension",
			},
			Launcher: LauncherMeta{Requirement: LauncherIgnored},
			NativeDocs: []NativeDocSpec{
				{Kind: "package_metadata", Path: "targets/gemini/package.yaml", Format: NativeDocYAML, Role: NativeDocRoleStructured},
				{Kind: "manifest_extra", Path: "targets/gemini/manifest.extra.json", Format: NativeDocJSON, Role: NativeDocRoleExtra, ManagedKeys: []string{"name", "version", "description", "mcpServers", "contextFileName", "excludeTools", "migratedTo", "settings", "themes", "plan.directory"}},
			},
			ManagedArtifacts: []ManagedArtifactSpec{
				{Kind: ManagedArtifactStatic, Path: "gemini-extension.json"},
				{Kind: ManagedArtifactMirror, ComponentKind: "hooks", SourceRoot: "targets/gemini/hooks", OutputRoot: "hooks"},
				{Kind: ManagedArtifactMirror, ComponentKind: "commands", SourceRoot: "targets/gemini/commands", OutputRoot: "commands"},
				{Kind: ManagedArtifactMirror, ComponentKind: "policies", SourceRoot: "targets/gemini/policies", OutputRoot: "policies"},
				{Kind: ManagedArtifactMirror, ComponentKind: "themes", SourceRoot: "targets/gemini/themes", OutputRoot: "themes"},
				{Kind: ManagedArtifactMirror, ComponentKind: "settings", SourceRoot: "targets/gemini/settings", OutputRoot: "settings"},
				{Kind: ManagedArtifactMirror, ComponentKind: "contexts", SourceRoot: "targets/gemini/contexts", OutputRoot: "contexts"},
				{Kind: ManagedArtifactSelectedContext, ComponentKind: "contexts", OutputRoot: "", ContextMode: ContextStrategyGeminiPrimaryRoot},
			},
			Scaffold: ScaffoldMeta{
				RequiredFiles: []string{
					"plugin.yaml",
					"targets/gemini/package.yaml",
					"contexts/GEMINI.md",
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
					{Path: "contexts/GEMINI.md", Template: "gemini.GEMINI.md.tmpl"},
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
