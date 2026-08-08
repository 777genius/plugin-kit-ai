package loader

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/777genius/plugin-kit-ai/install/integrationctl/agentplugins/domain"
	"gopkg.in/yaml.v3"
)

var skillNamePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*[a-z0-9]$|^[a-z0-9]$`)

var skillFields = map[string]struct{}{
	"name": {}, "description": {}, "license": {}, "compatibility": {}, "metadata": {}, "allowed-tools": {},
}

func loadSkills(root string) (map[string]domain.Skill, []string, bool, []domain.Diagnostic) {
	skills := map[string]domain.Skill{}
	info, err := os.Lstat(root)
	if os.IsNotExist(err) {
		return skills, nil, false, nil
	}
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		diagnostic := skillDiagnostic("skills_root_invalid", "", "skills must be a real directory", err)
		return skills, nil, true, []domain.Diagnostic{diagnostic}
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		diagnostic := skillDiagnostic("skills_read_failed", "", "read skills directory", err)
		return skills, nil, true, []domain.Diagnostic{diagnostic}
	}
	var invalid []string
	var diagnostics []domain.Diagnostic
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		skillPath := filepath.Join(root, name, "SKILL.md")
		body, exists, readErr := readRegularFile(skillPath)
		if readErr != nil || !exists {
			invalid = append(invalid, name)
			diagnostics = append(diagnostics, skillDiagnostic("skill_missing", name, "skill directory requires an immediate SKILL.md", readErr))
			continue
		}
		skill, parseErr := parseSkill(name, body)
		if parseErr != nil {
			invalid = append(invalid, name)
			diagnostics = append(diagnostics, skillDiagnostic("skill_invalid", name, "skill was skipped because SKILL.md is invalid", parseErr))
			continue
		}
		skills[name] = skill
	}
	sort.Strings(invalid)
	return skills, invalid, false, diagnostics
}

func parseSkill(directoryName string, body []byte) (domain.Skill, error) {
	if len(directoryName) > 64 || !skillNamePattern.MatchString(directoryName) || strings.Contains(directoryName, "--") {
		return domain.Skill{}, fmt.Errorf("directory name %q is not a portable Agent Skill name", directoryName)
	}
	frontmatter, err := skillFrontmatter(body)
	if err != nil {
		return domain.Skill{}, err
	}
	for key := range frontmatter {
		if _, ok := skillFields[key]; !ok {
			return domain.Skill{}, fmt.Errorf("unsupported frontmatter field %q", key)
		}
	}
	name, err := requiredString(frontmatter, "name")
	if err != nil {
		return domain.Skill{}, err
	}
	if name != directoryName {
		return domain.Skill{}, fmt.Errorf("frontmatter name %q does not match directory %q", name, directoryName)
	}
	description, err := requiredString(frontmatter, "description")
	if err != nil {
		return domain.Skill{}, err
	}
	if count := utf8.RuneCountInString(description); count < 1 || count > 1024 {
		return domain.Skill{}, fmt.Errorf("description length must be between 1 and 1024 characters")
	}
	license, err := optionalString(frontmatter, "license")
	if err != nil {
		return domain.Skill{}, err
	}
	compatibility, err := optionalString(frontmatter, "compatibility")
	if err != nil {
		return domain.Skill{}, err
	}
	if utf8.RuneCountInString(compatibility) > 500 {
		return domain.Skill{}, fmt.Errorf("compatibility exceeds 500 characters")
	}
	allowedTools, err := optionalString(frontmatter, "allowed-tools")
	if err != nil {
		return domain.Skill{}, err
	}
	metadata := map[string]any(nil)
	if value, ok := frontmatter["metadata"]; ok {
		var object bool
		metadata, object = value.(map[string]any)
		if !object {
			return domain.Skill{}, fmt.Errorf("metadata must be an object")
		}
	}
	return domain.Skill{
		Name:          name,
		Description:   description,
		License:       license,
		Compatibility: compatibility,
		Metadata:      metadata,
		AllowedTools:  allowedTools,
		RelativePath:  filepath.ToSlash(filepath.Join("skills", directoryName, "SKILL.md")),
		Raw:           append([]byte(nil), body...),
	}, nil
}

func skillFrontmatter(body []byte) (map[string]any, error) {
	normalized := bytes.ReplaceAll(body, []byte("\r\n"), []byte("\n"))
	if !bytes.HasPrefix(normalized, []byte("---\n")) {
		return nil, fmt.Errorf("SKILL.md must begin with YAML frontmatter")
	}
	remainder := normalized[len("---\n"):]
	end := bytes.Index(remainder, []byte("\n---\n"))
	if end < 0 {
		if bytes.HasSuffix(remainder, []byte("\n---")) {
			end = len(remainder) - len("\n---")
		} else {
			return nil, fmt.Errorf("SKILL.md frontmatter is not terminated")
		}
	}
	frontmatterBytes := remainder[:end]
	var frontmatter map[string]any
	decoder := yaml.NewDecoder(bytes.NewReader(frontmatterBytes))
	if err := decoder.Decode(&frontmatter); err != nil {
		return nil, fmt.Errorf("parse skill frontmatter: %w", err)
	}
	if frontmatter == nil {
		return nil, fmt.Errorf("skill frontmatter must be an object")
	}
	return frontmatter, nil
}

func requiredString(values map[string]any, key string) (string, error) {
	value, ok := values[key]
	if !ok {
		return "", fmt.Errorf("frontmatter requires %s", key)
	}
	text, ok := value.(string)
	if !ok || strings.TrimSpace(text) == "" {
		return "", fmt.Errorf("frontmatter %s must be a non-empty string", key)
	}
	return text, nil
}

func optionalString(values map[string]any, key string) (string, error) {
	value, ok := values[key]
	if !ok {
		return "", nil
	}
	text, ok := value.(string)
	if !ok {
		return "", fmt.Errorf("frontmatter %s must be a string", key)
	}
	return text, nil
}

func skillDiagnostic(code, name, message string, cause error) domain.Diagnostic {
	if cause != nil {
		message += ": " + cause.Error()
	}
	path := "skills"
	if name != "" {
		path = filepath.ToSlash(filepath.Join("skills", name, "SKILL.md"))
	}
	return domain.Diagnostic{
		Severity: domain.SeverityError,
		Boundary: domain.BoundarySkill,
		Code:     code,
		Path:     path,
		Item:     name,
		Message:  message,
	}
}
