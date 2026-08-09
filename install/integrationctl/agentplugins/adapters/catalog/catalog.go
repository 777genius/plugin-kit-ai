package catalog

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"path"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/777genius/plugin-kit-ai/install/integrationctl/agentplugins/domain"
	"golang.org/x/mod/semver"
)

const SchemaVersion = 1

var (
	repositoryPattern = regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`)
	commitPattern     = regexp.MustCompile(`^[0-9a-f]{40}$`)
	digestPattern     = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)
	namePattern       = regexp.MustCompile(`^[a-z0-9][a-z0-9.-]{0,63}$`)
)

var requiredCompatibility = map[string]string{
	string(domain.ClientCodex):   "projected",
	string(domain.ClientCursor):  "native",
	string(domain.ClientCopilot): "native",
	string(domain.ClientVSCode):  "prepared",
	string(domain.ClientKiro):    "native",
}

type Loader struct {
	CurrentCLIVersion string
}

type Loaded struct {
	Catalog domain.CatalogV1
	Digest  string
	byName  map[string]domain.CatalogPlugin
	cli     string
}

func (loader Loader) Load(body []byte, expectedDigest string) (Loaded, error) {
	digest := sha256Digest(body)
	if expected := strings.TrimSpace(expectedDigest); expected != "" && digest != expected {
		return Loaded{}, fmt.Errorf("catalog checksum mismatch")
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	var value domain.CatalogV1
	if err := decoder.Decode(&value); err != nil {
		return Loaded{}, fmt.Errorf("decode catalog v1: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return Loaded{}, fmt.Errorf("catalog contains trailing JSON values")
	}
	if err := validateCatalog(value); err != nil {
		return Loaded{}, err
	}
	byName := make(map[string]domain.CatalogPlugin, len(value.Plugins))
	for _, plugin := range value.Plugins {
		byName[plugin.Name] = plugin
	}
	return Loaded{Catalog: value, Digest: digest, byName: byName, cli: normalizeVersion(loader.CurrentCLIVersion)}, nil
}

func (loaded Loaded) Resolve(name string) (domain.CatalogResolution, error) {
	name = strings.TrimSpace(name)
	entry, ok := loaded.byName[name]
	if !ok {
		return domain.CatalogResolution{}, fmt.Errorf("plugin %q is not present in catalog %s", name, loaded.Catalog.CatalogVersion)
	}
	if loaded.cli != "" && semver.Compare(loaded.cli, normalizeVersion(entry.MinimumCLIVersion)) < 0 {
		return domain.CatalogResolution{}, fmt.Errorf("plugin %q requires agentplugins %s or newer", name, entry.MinimumCLIVersion)
	}
	return domain.CatalogResolution{
		Entry:           entry,
		SourceReference: fmt.Sprintf("%s@%s//%s", loaded.Catalog.Repository, loaded.Catalog.Revision, entry.SourcePath),
		CatalogDigest:   loaded.Digest,
		Hints: domain.CompatibilityHints{
			Compatibility: cloneCompatibility(entry.Compatibility),
			OpenAIMCPAuth: cloneAuthHints(entry.OpenAIMCPAuth),
		},
		Evidence: domain.CatalogEvidence{
			SchemaVersion:      loaded.Catalog.SchemaVersion,
			CatalogVersion:     loaded.Catalog.CatalogVersion,
			Repository:         loaded.Catalog.Repository,
			Revision:           loaded.Catalog.Revision,
			Digest:             loaded.Digest,
			MinimumCLIVersion:  entry.MinimumCLIVersion,
			AgentPluginsSchema: entry.AgentPluginsSchema,
			Compatibility:      cloneCompatibility(entry.Compatibility),
		},
	}, nil
}

func validateCatalog(value domain.CatalogV1) error {
	if value.Schema != domain.CatalogSchemaV1 || value.SchemaVersion != SchemaVersion {
		return fmt.Errorf("unsupported catalog schema")
	}
	if !semver.IsValid(normalizeVersion(value.CatalogVersion)) {
		return fmt.Errorf("catalog_version must be semantic versioning")
	}
	if !repositoryPattern.MatchString(value.Repository) || !commitPattern.MatchString(value.Revision) {
		return fmt.Errorf("catalog repository and revision must be exact GitHub coordinates")
	}
	if _, err := time.Parse(time.RFC3339, value.PublishedAt); err != nil {
		return fmt.Errorf("catalog published_at must be RFC3339: %w", err)
	}
	if len(value.Plugins) == 0 {
		return fmt.Errorf("catalog must contain at least one plugin")
	}
	seen := map[string]domain.CatalogPlugin{}
	for index, plugin := range value.Plugins {
		if err := validatePlugin(plugin); err != nil {
			return fmt.Errorf("plugins[%d]: %w", index, err)
		}
		if previous, exists := seen[plugin.Name]; exists {
			if previous.Version == plugin.Version && previous.TreeDigest != plugin.TreeDigest {
				return fmt.Errorf("supply-chain conflict for %s@%s", plugin.Name, plugin.Version)
			}
			return fmt.Errorf("duplicate catalog plugin %q", plugin.Name)
		}
		seen[plugin.Name] = plugin
	}
	return nil
}

func validatePlugin(plugin domain.CatalogPlugin) error {
	if !namePattern.MatchString(plugin.Name) || strings.Contains(plugin.Name, "..") || strings.HasSuffix(plugin.Name, ".") {
		return fmt.Errorf("invalid plugin name %q", plugin.Name)
	}
	if !semver.IsValid(normalizeVersion(plugin.Version)) || !semver.IsValid(normalizeVersion(plugin.MinimumCLIVersion)) {
		return fmt.Errorf("plugin %q has invalid version metadata", plugin.Name)
	}
	if plugin.AgentPluginsSchema != domain.PluginSchemaV1 {
		return fmt.Errorf("plugin %q uses unsupported Agent Plugins schema", plugin.Name)
	}
	if err := validateSourcePath(plugin.SourcePath); err != nil {
		return fmt.Errorf("plugin %q source path: %w", plugin.Name, err)
	}
	if !digestPattern.MatchString(plugin.TreeDigest) || !digestPattern.MatchString(plugin.ManifestDigest) {
		return fmt.Errorf("plugin %q requires exact SHA-256 digests", plugin.Name)
	}
	components := map[string]struct{}{}
	for _, component := range plugin.Components {
		if component != "skills" && component != "mcp" && component != "extensions" {
			return fmt.Errorf("plugin %q has unknown component %q", plugin.Name, component)
		}
		if _, duplicate := components[component]; duplicate {
			return fmt.Errorf("plugin %q repeats component %q", plugin.Name, component)
		}
		components[component] = struct{}{}
	}
	if len(plugin.Compatibility) != len(requiredCompatibility) {
		return fmt.Errorf("plugin %q compatibility must contain exactly codex, cursor, copilot, vscode, and kiro", plugin.Name)
	}
	var authentication domain.AuthenticationRequirement
	for client, expectedPackage := range requiredCompatibility {
		compatibility, ok := plugin.Compatibility[client]
		if !ok {
			return fmt.Errorf("plugin %q compatibility is missing %q", plugin.Name, client)
		}
		if compatibility.Package != expectedPackage ||
			!validVerificationCompatibility(compatibility.Verification) || !validAuthCompatibility(compatibility.Authentication) {
			return fmt.Errorf("plugin %q has invalid compatibility for %q", plugin.Name, client)
		}
		if authentication == "" {
			authentication = compatibility.Authentication
		} else if compatibility.Authentication != authentication {
			return fmt.Errorf("plugin %q must use one consistent authentication requirement for every client", plugin.Name)
		}
	}
	for server, hint := range plugin.OpenAIMCPAuth {
		if strings.TrimSpace(server) == "" || (hint.OAuthResource == "" && hint.BearerTokenEnvVar == "") {
			return fmt.Errorf("plugin %q has invalid OpenAI auth hint", plugin.Name)
		}
	}
	return nil
}

func validateSourcePath(value string) error {
	if value == "" || !utf8.ValidString(value) || strings.ContainsAny(value, "\\\x00") || strings.HasPrefix(value, "/") {
		return fmt.Errorf("path must be a portable relative path")
	}
	parts := strings.Split(value, "/")
	if len(parts) > 64 {
		return fmt.Errorf("path exceeds maximum depth")
	}
	for _, part := range parts {
		if part == "" || part == "." || part == ".." || strings.HasSuffix(part, ".") {
			return fmt.Errorf("path contains unsafe segment")
		}
	}
	if path.Clean(value) != value {
		return fmt.Errorf("path is not normalized")
	}
	return nil
}

func validVerificationCompatibility(value string) bool {
	return oneOf(value, "tested", "schema_only", "not_tested")
}

func validAuthCompatibility(value domain.AuthenticationRequirement) bool {
	return value == domain.AuthenticationRequirementNotRequired ||
		value == domain.AuthenticationRequirementRequired ||
		value == domain.AuthenticationRequirementUnknown
}

func cloneCompatibility(source map[string]domain.CatalogCompatibility) map[string]domain.CatalogCompatibility {
	if len(source) == 0 {
		return nil
	}
	result := make(map[string]domain.CatalogCompatibility, len(source))
	for key, value := range source {
		result[key] = value
	}
	return result
}

func oneOf(value string, allowed ...string) bool {
	for _, candidate := range allowed {
		if value == candidate {
			return true
		}
	}
	return false
}

func normalizeVersion(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || strings.Contains(value, "development") {
		return ""
	}
	if !strings.HasPrefix(value, "v") {
		value = "v" + value
	}
	return value
}

func sha256Digest(body []byte) string {
	sum := sha256.Sum256(body)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func cloneAuthHints(source map[string]domain.OpenAIMCPAuthHint) map[string]domain.OpenAIMCPAuthHint {
	if len(source) == 0 {
		return nil
	}
	keys := make([]string, 0, len(source))
	for key := range source {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	result := make(map[string]domain.OpenAIMCPAuthHint, len(source))
	for _, key := range keys {
		result[key] = source[key]
	}
	return result
}
