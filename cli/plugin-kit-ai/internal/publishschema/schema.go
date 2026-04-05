package publishschema

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/777genius/plugin-kit-ai/cli/internal/scaffold"
	"gopkg.in/yaml.v3"
)

const (
	APIVersionV1         = "v1"
	CodexMarketplaceRel  = "publish/codex/marketplace.yaml"
	ClaudeMarketplaceRel = "publish/claude/marketplace.yaml"
	GeminiGalleryRel     = "publish/gemini/gallery.yaml"
)

var authPolicyRe = regexp.MustCompile(`^[A-Z][A-Z0-9_]*$`)

type CodexMarketplace struct {
	APIVersion           string `yaml:"api_version" json:"api_version"`
	MarketplaceName      string `yaml:"marketplace_name" json:"marketplace_name"`
	DisplayName          string `yaml:"display_name,omitempty" json:"display_name,omitempty"`
	SourceRoot           string `yaml:"source_root,omitempty" json:"source_root,omitempty"`
	Category             string `yaml:"category" json:"category"`
	InstallationPolicy   string `yaml:"installation_policy,omitempty" json:"installation_policy,omitempty"`
	AuthenticationPolicy string `yaml:"authentication_policy,omitempty" json:"authentication_policy,omitempty"`
	Path                 string `yaml:"-" json:"path"`
}

type ClaudeMarketplace struct {
	APIVersion      string `yaml:"api_version" json:"api_version"`
	MarketplaceName string `yaml:"marketplace_name" json:"marketplace_name"`
	OwnerName       string `yaml:"owner_name" json:"owner_name"`
	SourceRoot      string `yaml:"source_root,omitempty" json:"source_root,omitempty"`
	Path            string `yaml:"-" json:"path"`
}

type GeminiGallery struct {
	APIVersion           string `yaml:"api_version" json:"api_version"`
	Distribution         string `yaml:"distribution,omitempty" json:"distribution,omitempty"`
	RepositoryVisibility string `yaml:"repository_visibility,omitempty" json:"repository_visibility,omitempty"`
	GitHubTopic          string `yaml:"github_topic,omitempty" json:"github_topic,omitempty"`
	ManifestRoot         string `yaml:"manifest_root,omitempty" json:"manifest_root,omitempty"`
	Path                 string `yaml:"-" json:"path"`
}

type State struct {
	Codex  *CodexMarketplace  `json:"codex,omitempty"`
	Claude *ClaudeMarketplace `json:"claude,omitempty"`
	Gemini *GeminiGallery     `json:"gemini,omitempty"`
}

func Discover(root string) (State, error) {
	return DiscoverInLayout(root, "")
}

func DiscoverInLayout(root, authoredRoot string) (State, error) {
	var out State
	if doc, ok, err := loadCodexMarketplace(root, authoredRoot); err != nil {
		return State{}, err
	} else if ok {
		out.Codex = doc
	}
	if doc, ok, err := loadClaudeMarketplace(root, authoredRoot); err != nil {
		return State{}, err
	} else if ok {
		out.Claude = doc
	}
	if doc, ok, err := loadGeminiGallery(root, authoredRoot); err != nil {
		return State{}, err
	} else if ok {
		out.Gemini = doc
	}
	return out, nil
}

func (s State) Paths() []string {
	var out []string
	if s.Codex != nil {
		out = append(out, s.Codex.Path)
	}
	if s.Claude != nil {
		out = append(out, s.Claude.Path)
	}
	if s.Gemini != nil {
		out = append(out, s.Gemini.Path)
	}
	slices.Sort(out)
	return out
}

func (s State) ValidateTargets(targets []string) error {
	enabled := setOf(targets)
	if s.Codex != nil && !enabled["codex-package"] {
		return fmt.Errorf("%s requires target %q in plugin.yaml", s.Codex.Path, "codex-package")
	}
	if s.Claude != nil && !enabled["claude"] {
		return fmt.Errorf("%s requires target %q in plugin.yaml", s.Claude.Path, "claude")
	}
	if s.Gemini != nil && !enabled["gemini"] {
		return fmt.Errorf("%s requires target %q in plugin.yaml", s.Gemini.Path, "gemini")
	}
	return nil
}

func loadCodexMarketplace(root, authoredRoot string) (*CodexMarketplace, bool, error) {
	body, ok, err := readOptional(root, authoredRoot, CodexMarketplaceRel)
	if err != nil || !ok {
		return nil, ok, err
	}
	var out CodexMarketplace
	if err := yaml.Unmarshal(body, &out); err != nil {
		return nil, true, fmt.Errorf("parse %s: %w", CodexMarketplaceRel, err)
	}
	normalizeCodexMarketplace(&out)
	out.Path = prefixedRel(authoredRoot, CodexMarketplaceRel)
	if err := out.Validate(); err != nil {
		return nil, true, fmt.Errorf("parse %s: %w", CodexMarketplaceRel, err)
	}
	return &out, true, nil
}

func loadClaudeMarketplace(root, authoredRoot string) (*ClaudeMarketplace, bool, error) {
	body, ok, err := readOptional(root, authoredRoot, ClaudeMarketplaceRel)
	if err != nil || !ok {
		return nil, ok, err
	}
	var out ClaudeMarketplace
	if err := yaml.Unmarshal(body, &out); err != nil {
		return nil, true, fmt.Errorf("parse %s: %w", ClaudeMarketplaceRel, err)
	}
	normalizeClaudeMarketplace(&out)
	out.Path = prefixedRel(authoredRoot, ClaudeMarketplaceRel)
	if err := out.Validate(); err != nil {
		return nil, true, fmt.Errorf("parse %s: %w", ClaudeMarketplaceRel, err)
	}
	return &out, true, nil
}

func loadGeminiGallery(root, authoredRoot string) (*GeminiGallery, bool, error) {
	body, ok, err := readOptional(root, authoredRoot, GeminiGalleryRel)
	if err != nil || !ok {
		return nil, ok, err
	}
	var out GeminiGallery
	if err := yaml.Unmarshal(body, &out); err != nil {
		return nil, true, fmt.Errorf("parse %s: %w", GeminiGalleryRel, err)
	}
	normalizeGeminiGallery(&out)
	out.Path = prefixedRel(authoredRoot, GeminiGalleryRel)
	if err := out.Validate(); err != nil {
		return nil, true, fmt.Errorf("parse %s: %w", GeminiGalleryRel, err)
	}
	return &out, true, nil
}

func normalizeCodexMarketplace(doc *CodexMarketplace) {
	doc.APIVersion = strings.TrimSpace(doc.APIVersion)
	doc.MarketplaceName = strings.TrimSpace(doc.MarketplaceName)
	doc.DisplayName = strings.TrimSpace(doc.DisplayName)
	doc.SourceRoot = normalizeSourceRoot(doc.SourceRoot)
	doc.Category = strings.TrimSpace(doc.Category)
	doc.InstallationPolicy = normalizePolicy(doc.InstallationPolicy, "AVAILABLE")
	doc.AuthenticationPolicy = normalizePolicy(doc.AuthenticationPolicy, "ON_INSTALL")
}

func normalizeClaudeMarketplace(doc *ClaudeMarketplace) {
	doc.APIVersion = strings.TrimSpace(doc.APIVersion)
	doc.MarketplaceName = strings.TrimSpace(doc.MarketplaceName)
	doc.OwnerName = strings.TrimSpace(doc.OwnerName)
	doc.SourceRoot = normalizeSourceRoot(doc.SourceRoot)
}

func normalizeGeminiGallery(doc *GeminiGallery) {
	doc.APIVersion = strings.TrimSpace(doc.APIVersion)
	doc.Distribution = strings.ToLower(strings.TrimSpace(doc.Distribution))
	if doc.Distribution == "" {
		doc.Distribution = "git_repository"
	}
	doc.RepositoryVisibility = strings.ToLower(strings.TrimSpace(doc.RepositoryVisibility))
	if doc.RepositoryVisibility == "" {
		doc.RepositoryVisibility = "public"
	}
	doc.GitHubTopic = strings.TrimSpace(doc.GitHubTopic)
	if doc.GitHubTopic == "" {
		doc.GitHubTopic = "gemini-cli-extension"
	}
	doc.ManifestRoot = strings.ToLower(strings.TrimSpace(doc.ManifestRoot))
	if doc.ManifestRoot == "" {
		doc.ManifestRoot = "repository_root"
	}
}

func (doc CodexMarketplace) Validate() error {
	if doc.APIVersion != APIVersionV1 {
		return fmt.Errorf("api_version must be %q", APIVersionV1)
	}
	if err := scaffold.ValidateProjectName(doc.MarketplaceName); err != nil {
		return fmt.Errorf("invalid marketplace_name: %w", err)
	}
	if err := validateSourceRoot(doc.SourceRoot); err != nil {
		return err
	}
	if strings.TrimSpace(doc.Category) == "" {
		return fmt.Errorf("category required")
	}
	switch doc.InstallationPolicy {
	case "AVAILABLE", "INSTALLED_BY_DEFAULT", "NOT_AVAILABLE":
	default:
		return fmt.Errorf("installation_policy must be one of AVAILABLE, INSTALLED_BY_DEFAULT, NOT_AVAILABLE")
	}
	if !authPolicyRe.MatchString(doc.AuthenticationPolicy) {
		return fmt.Errorf("authentication_policy must use uppercase snake case")
	}
	return nil
}

func (doc ClaudeMarketplace) Validate() error {
	if doc.APIVersion != APIVersionV1 {
		return fmt.Errorf("api_version must be %q", APIVersionV1)
	}
	if err := scaffold.ValidateProjectName(doc.MarketplaceName); err != nil {
		return fmt.Errorf("invalid marketplace_name: %w", err)
	}
	if strings.TrimSpace(doc.OwnerName) == "" {
		return fmt.Errorf("owner_name required")
	}
	if err := validateSourceRoot(doc.SourceRoot); err != nil {
		return err
	}
	return nil
}

func (doc GeminiGallery) Validate() error {
	if doc.APIVersion != APIVersionV1 {
		return fmt.Errorf("api_version must be %q", APIVersionV1)
	}
	switch doc.Distribution {
	case "git_repository", "github_release":
	default:
		return fmt.Errorf("distribution must be one of git_repository, github_release")
	}
	if doc.RepositoryVisibility != "public" {
		return fmt.Errorf("repository_visibility must be %q", "public")
	}
	if doc.GitHubTopic != "gemini-cli-extension" {
		return fmt.Errorf("github_topic must be %q", "gemini-cli-extension")
	}
	switch doc.ManifestRoot {
	case "repository_root", "release_archive_root":
	default:
		return fmt.Errorf("manifest_root must be one of repository_root, release_archive_root")
	}
	if doc.Distribution == "git_repository" && doc.ManifestRoot != "repository_root" {
		return fmt.Errorf("manifest_root must be %q when distribution is %q", "repository_root", "git_repository")
	}
	return nil
}

func normalizeSourceRoot(value string) string {
	value = strings.TrimSpace(value)
	switch value {
	case "", ".":
		return "./"
	default:
		if value == "./" {
			return value
		}
		return filepath.ToSlash(value)
	}
}

func normalizePolicy(value, fallback string) string {
	value = strings.ToUpper(strings.TrimSpace(value))
	if value == "" {
		return fallback
	}
	return value
}

func validateSourceRoot(value string) error {
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("source_root required")
	}
	if strings.HasPrefix(value, "/") {
		return fmt.Errorf("source_root must stay relative to the publication root")
	}
	if !strings.HasPrefix(value, "./") {
		return fmt.Errorf("source_root must start with ./")
	}
	trimmed := strings.TrimPrefix(value, "./")
	if trimmed == ".." || strings.HasPrefix(trimmed, "../") || strings.Contains(trimmed, "/../") {
		return fmt.Errorf("source_root may not escape the publication root")
	}
	return nil
}

func readOptional(root, authoredRoot, rel string) ([]byte, bool, error) {
	body, err := os.ReadFile(filepath.Join(root, prefixedRel(authoredRoot, rel)))
	if err == nil {
		return body, true, nil
	}
	if os.IsNotExist(err) {
		return nil, false, nil
	}
	return nil, false, err
}

func prefixedRel(rootRel, rel string) string {
	rootRel = filepath.ToSlash(strings.TrimSpace(rootRel))
	rel = filepath.ToSlash(strings.TrimSpace(rel))
	if rootRel == "" {
		return rel
	}
	return filepath.ToSlash(filepath.Join(rootRel, rel))
}

func setOf(values []string) map[string]bool {
	out := make(map[string]bool, len(values))
	for _, value := range values {
		out[strings.TrimSpace(value)] = true
	}
	return out
}
