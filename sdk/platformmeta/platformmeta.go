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
	// SurfaceTierPassthroughOnly marks config surfaces that are preserved but not modeled as first-class authored APIs.
	SurfaceTierPassthroughOnly SurfaceTier = "passthrough_only"
	// SurfaceTierUnsupported marks unsupported surfaces that should not be relied on.
	SurfaceTierUnsupported SurfaceTier = "unsupported"
)

// TemplateFile describes a scaffolded output file and its template source.
type TemplateFile struct {
	// Path is the destination path inside the generated project.
	Path string
	// Template is the template file used to generate the destination.
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
	// TemplateFiles maps scaffold output files to their generate templates.
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
	// GenerateSupport reports whether `plugin-kit-ai generate` is supported.
	GenerateSupport bool
	// ValidateSupport reports whether `plugin-kit-ai validate` is supported.
	ValidateSupport bool
	// PortableComponentKinds lists portable authoring components the target consumes.
	PortableComponentKinds []string
	// TargetComponentKinds lists native target components generated for the target.
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

func normalizeName(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}
