package domain

import "encoding/json"

const (
	LoaderKindAgentPlugins = "agent_plugins"
	FormatIDAgentPluginsV1 = "agent-plugins/1.0.0"
	LoaderKindLegacy       = "legacy"
	FormatIDLegacyV1       = "plugin-kit-ai/v1"

	PluginSchemaV1 = "https://agent-plugins.org/schemas/1.0.0/plugin.schema.json"
	MCPSchemaV1    = "https://agent-plugins.org/schemas/1.0.0/mcp.schema.json"
)

type Severity string

const (
	SeverityInfo    Severity = "info"
	SeverityWarning Severity = "warning"
	SeverityError   Severity = "error"
)

type FailureBoundary string

const (
	BoundaryPlugin    FailureBoundary = "plugin"
	BoundaryMCP       FailureBoundary = "mcp"
	BoundaryMCPServer FailureBoundary = "mcp_server"
	BoundarySkill     FailureBoundary = "skill"
	BoundaryExtension FailureBoundary = "extension"
)

type Diagnostic struct {
	Severity Severity        `json:"severity"`
	Boundary FailureBoundary `json:"boundary"`
	Code     string          `json:"code"`
	Path     string          `json:"path,omitempty"`
	Item     string          `json:"item,omitempty"`
	Message  string          `json:"message"`
}

type SourceIdentity struct {
	RequestedSource   string `json:"requested_source,omitempty"`
	CanonicalSource   string `json:"canonical_source,omitempty"`
	Repository        string `json:"repository,omitempty"`
	PackageSubpath    string `json:"package_subpath,omitempty"`
	ResolvedRevision  string `json:"resolved_revision,omitempty"`
	SourceBindingHint string `json:"source_binding_hint,omitempty"`
}

type Author struct {
	Name  string `json:"name,omitempty"`
	Email string `json:"email,omitempty"`
	URL   string `json:"url,omitempty"`
}

type PluginManifest struct {
	SchemaURI     string                     `json:"schema_uri"`
	Name          string                     `json:"name"`
	Version       string                     `json:"version,omitempty"`
	Description   string                     `json:"description,omitempty"`
	Author        *Author                    `json:"author,omitempty"`
	Homepage      string                     `json:"homepage,omitempty"`
	Repository    string                     `json:"repository,omitempty"`
	License       string                     `json:"license,omitempty"`
	Keywords      []string                   `json:"keywords,omitempty"`
	Extensions    map[string]json.RawMessage `json:"extensions,omitempty"`
	RawExtensions json.RawMessage            `json:"-"`
	Unknown       map[string]json.RawMessage `json:"unknown,omitempty"`
	Raw           json.RawMessage            `json:"-"`
}

// SchemaIdentity makes a document's schema URI and interpreted version
// explicit without replacing its lossless Raw representation.
type SchemaIdentity struct {
	URI     string `json:"uri"`
	Version string `json:"version"`
}

// VersionedDocument preserves author-controlled JSON independently of any
// catalog or legacy metadata that describes the same package.
type VersionedDocument struct {
	Schema  SchemaIdentity             `json:"schema"`
	Raw     json.RawMessage            `json:"raw"`
	Unknown map[string]json.RawMessage `json:"unknown,omitempty"`
}

type MCPServer struct {
	Name    string          `json:"name"`
	Type    string          `json:"type"`
	Raw     json.RawMessage `json:"-"`
	Decoded map[string]any  `json:"config"`
}

type MCPComponent struct {
	Present       bool                  `json:"present"`
	Enabled       bool                  `json:"enabled"`
	SchemaURI     string                `json:"schema_uri,omitempty"`
	Raw           json.RawMessage       `json:"-"`
	Servers       map[string]MCPServer  `json:"servers,omitempty"`
	InvalidServer map[string]Diagnostic `json:"invalid_servers,omitempty"`
}

type Skill struct {
	Name          string         `json:"name"`
	Description   string         `json:"description"`
	License       string         `json:"license,omitempty"`
	Compatibility string         `json:"compatibility,omitempty"`
	Metadata      map[string]any `json:"metadata,omitempty"`
	AllowedTools  string         `json:"allowed_tools,omitempty"`
	RelativePath  string         `json:"relative_path"`
	Raw           []byte         `json:"-"`
}

type ComponentInventory struct {
	MCPPresent        bool     `json:"mcp_present"`
	MCPEnabled        bool     `json:"mcp_enabled"`
	MCPServers        []string `json:"mcp_servers,omitempty"`
	InvalidMCPServer  []string `json:"invalid_mcp_servers,omitempty"`
	Skills            []string `json:"skills,omitempty"`
	InvalidSkills     []string `json:"invalid_skills,omitempty"`
	InvalidSkillsRoot bool     `json:"invalid_skills_root,omitempty"`
	Extensions        []string `json:"extensions,omitempty"`
}

type PackageEnvelope struct {
	LoaderKind      string             `json:"loader_kind"`
	FormatID        string             `json:"format_id"`
	SchemaURI       string             `json:"schema_uri"`
	SchemaVersion   string             `json:"schema_version"`
	ManifestSchema  SchemaIdentity     `json:"manifest_schema"`
	Manifest        PluginManifest     `json:"manifest"`
	MCP             MCPComponent       `json:"mcp"`
	Skills          map[string]Skill   `json:"skills,omitempty"`
	Inventory       ComponentInventory `json:"inventory"`
	Diagnostics     []Diagnostic       `json:"diagnostics,omitempty"`
	CatalogEvidence *CatalogEvidence   `json:"catalog_evidence,omitempty"`
	Source          SourceIdentity     `json:"source"`
	TreeDigest      string             `json:"tree_digest"`
	ManifestDigest  string             `json:"manifest_digest"`
	ExecutableFiles []string           `json:"executable_files,omitempty"`
	SnapshotRoot    string             `json:"-"`
}

type LoadInput struct {
	SnapshotRoot    string
	TreeDigest      string
	ExecutableFiles []string
	Source          SourceIdentity
}
