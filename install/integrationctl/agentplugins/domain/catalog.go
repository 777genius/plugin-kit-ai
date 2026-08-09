package domain

const CatalogSchemaV1 = "https://github.com/777genius/universal-agent-plugins/schemas/catalog-v1.schema.json"

type CatalogCompatibility struct {
	Package        string                    `json:"package"`
	Verification   string                    `json:"verification"`
	Authentication AuthenticationRequirement `json:"authentication"`
}

type AuthenticationRequirement string

const (
	AuthenticationRequirementNotRequired AuthenticationRequirement = "not_required"
	AuthenticationRequirementRequired    AuthenticationRequirement = "required"
	AuthenticationRequirementUnknown     AuthenticationRequirement = "unknown"
)

type CatalogPlugin struct {
	Name               string                          `json:"name"`
	Version            string                          `json:"version"`
	AgentPluginsSchema string                          `json:"agent_plugins_schema"`
	MinimumCLIVersion  string                          `json:"minimum_cli_version"`
	SourcePath         string                          `json:"source_path"`
	TreeDigest         string                          `json:"tree_digest"`
	ManifestDigest     string                          `json:"manifest_digest"`
	Components         []string                        `json:"components"`
	Compatibility      map[string]CatalogCompatibility `json:"compatibility"`
	OpenAIMCPAuth      map[string]OpenAIMCPAuthHint    `json:"openai_mcp_auth,omitempty"`
}

type CatalogV1 struct {
	Schema         string          `json:"$schema"`
	SchemaVersion  int             `json:"schema_version"`
	CatalogVersion string          `json:"catalog_version"`
	Repository     string          `json:"repository"`
	Revision       string          `json:"revision"`
	PublishedAt    string          `json:"published_at"`
	Plugins        []CatalogPlugin `json:"plugins"`
}

type CatalogResolution struct {
	Entry           CatalogPlugin      `json:"entry"`
	SourceReference string             `json:"source_reference"`
	CatalogDigest   string             `json:"catalog_digest"`
	Hints           CompatibilityHints `json:"hints,omitempty"`
	Evidence        CatalogEvidence    `json:"evidence"`
}

// CatalogEvidence is an immutable, versioned snapshot of the catalog facts
// used to resolve a package. It is kept separate from author-controlled
// plugin.json metadata so neither source can overwrite the other as schemas
// evolve.
type CatalogEvidence struct {
	SchemaVersion      int                             `json:"schema_version"`
	CatalogVersion     string                          `json:"catalog_version"`
	Repository         string                          `json:"repository"`
	Revision           string                          `json:"revision"`
	Digest             string                          `json:"digest"`
	MinimumCLIVersion  string                          `json:"minimum_cli_version"`
	AgentPluginsSchema string                          `json:"agent_plugins_schema"`
	Compatibility      map[string]CatalogCompatibility `json:"compatibility,omitempty"`
}
