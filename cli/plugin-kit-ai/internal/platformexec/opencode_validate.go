package platformexec

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/777genius/plugin-kit-ai/cli/internal/pluginmodel"
	skillfs "github.com/777genius/plugin-kit-ai/cli/internal/skills/adapters/filesystem"
	skillsapp "github.com/777genius/plugin-kit-ai/cli/internal/skills/app"
	"github.com/tailscale/hujson"
)

func (opencodeAdapter) Validate(root string, graph pluginmodel.PackageGraph, state pluginmodel.TargetState) ([]Diagnostic, error) {
	var diagnostics []Diagnostic
	meta, _, err := readYAMLDoc[opencodePackageMeta](root, state.DocPath("package_metadata"))
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", state.DocPath("package_metadata"), err)
	}
	if err := validateOpenCodePluginRefs(meta.Plugins); err != nil {
		diagnostics = append(diagnostics, Diagnostic{
			Severity: SeverityFailure,
			Code:     CodeManifestInvalid,
			Path:     state.DocPath("package_metadata"),
			Target:   "opencode",
			Message:  "OpenCode package metadata " + err.Error(),
		})
	}
	configPath, warnings, ok, err := resolveOpenCodeConfigPath(root)
	if err != nil {
		return nil, err
	}
	if !ok {
		return []Diagnostic{{
			Severity: SeverityFailure,
			Code:     CodeGeneratedContractInvalid,
			Path:     "opencode.json",
			Target:   "opencode",
			Message:  "OpenCode config opencode.json or opencode.jsonc is required",
		}}, nil
	}
	for _, warning := range warnings {
		diagnostics = append(diagnostics, Diagnostic{
			Severity: SeverityWarning,
			Code:     CodeManifestInvalid,
			Path:     warning.Path,
			Target:   "opencode",
			Message:  warning.Message,
		})
	}
	configReadPath := filepath.Join(root, configPath)
	body, err := os.ReadFile(configReadPath)
	if err != nil {
		return []Diagnostic{{
			Severity: SeverityFailure,
			Code:     CodeGeneratedContractInvalid,
			Path:     configPath,
			Target:   "opencode",
			Message:  fmt.Sprintf("OpenCode config %s is not readable: %v", configPath, err),
		}}, nil
	}
	body, err = hujson.Standardize(body)
	if err != nil {
		return []Diagnostic{{
			Severity: SeverityFailure,
			Code:     CodeManifestInvalid,
			Path:     configPath,
			Target:   "opencode",
			Message:  fmt.Sprintf("OpenCode config %s is invalid JSON/JSONC: %v", configPath, err),
		}}, nil
	}
	var doc map[string]any
	if err := json.Unmarshal(body, &doc); err != nil {
		return []Diagnostic{{
			Severity: SeverityFailure,
			Code:     CodeManifestInvalid,
			Path:     configPath,
			Target:   "opencode",
			Message:  fmt.Sprintf("OpenCode config %s is invalid JSON/JSONC: %v", configPath, err),
		}}, nil
	}
	if schema, _ := doc["$schema"].(string); strings.TrimSpace(schema) != "https://opencode.ai/config.json" {
		diagnostics = append(diagnostics, Diagnostic{
			Severity: SeverityFailure,
			Code:     CodeGeneratedContractInvalid,
			Path:     configPath,
			Target:   "opencode",
			Message:  fmt.Sprintf("OpenCode config %s must declare $schema %q", configPath, "https://opencode.ai/config.json"),
		})
	}
	if raw, ok := doc["plugin"]; ok {
		values, ok := raw.([]any)
		if !ok {
			diagnostics = append(diagnostics, Diagnostic{
				Severity: SeverityFailure,
				Code:     CodeManifestInvalid,
				Path:     configPath,
				Target:   "opencode",
				Message:  `OpenCode config field "plugin" must be an array of strings or [name, options] tuples`,
			})
		} else {
			for i, value := range values {
				if _, err := normalizeImportedOpenCodePluginRef(value); err != nil {
					diagnostics = append(diagnostics, Diagnostic{
						Severity: SeverityFailure,
						Code:     CodeManifestInvalid,
						Path:     configPath,
						Target:   "opencode",
						Message:  fmt.Sprintf(`OpenCode config field "plugin" has invalid entry at index %d: %v`, i, err),
					})
				}
			}
		}
	}
	if raw, ok := doc["mcp"]; ok {
		if _, ok := raw.(map[string]any); !ok {
			diagnostics = append(diagnostics, Diagnostic{
				Severity: SeverityFailure,
				Code:     CodeManifestInvalid,
				Path:     configPath,
				Target:   "opencode",
				Message:  `OpenCode config field "mcp" must be a JSON object`,
			})
		}
	}
	if raw, ok := doc["default_agent"]; ok {
		text, ok := raw.(string)
		if !ok || strings.TrimSpace(text) == "" {
			diagnostics = append(diagnostics, Diagnostic{
				Severity: SeverityFailure,
				Code:     CodeManifestInvalid,
				Path:     configPath,
				Target:   "opencode",
				Message:  `OpenCode config field "default_agent" must be a non-empty string`,
			})
		}
	}
	if raw, ok := doc["instructions"]; ok {
		values, ok := raw.([]any)
		if !ok {
			diagnostics = append(diagnostics, Diagnostic{
				Severity: SeverityFailure,
				Code:     CodeManifestInvalid,
				Path:     configPath,
				Target:   "opencode",
				Message:  `OpenCode config field "instructions" must be an array of strings`,
			})
		} else {
			for i, value := range values {
				text, ok := value.(string)
				if !ok || strings.TrimSpace(text) == "" {
					diagnostics = append(diagnostics, Diagnostic{
						Severity: SeverityFailure,
						Code:     CodeManifestInvalid,
						Path:     configPath,
						Target:   "opencode",
						Message:  fmt.Sprintf(`OpenCode config field "instructions" must contain non-empty strings (invalid entry at index %d)`, i),
					})
				}
			}
		}
	}
	if raw, ok := doc["permission"]; ok && !isOpenCodePermissionValue(raw) {
		diagnostics = append(diagnostics, Diagnostic{
			Severity: SeverityFailure,
			Code:     CodeManifestInvalid,
			Path:     configPath,
			Target:   "opencode",
			Message:  `OpenCode config field "permission" must be a string or JSON object`,
		})
	}
	if len(graph.Portable.Paths("skills")) > 0 {
		authoredRoot := filepath.Join(root, pluginmodel.SourceDirName)
		report, err := (skillsapp.Service{Repo: skillfs.Repository{}}).Validate(skillsapp.ValidateOptions{Root: authoredRoot})
		if err != nil {
			return nil, err
		}
		for _, failure := range report.Failures {
			diagnostics = append(diagnostics, Diagnostic{
				Severity: SeverityFailure,
				Code:     CodeManifestInvalid,
				Path:     filepath.ToSlash(filepath.Join(pluginmodel.SourceDirName, failure.Path)),
				Target:   "opencode",
				Message:  "OpenCode mirrored skill is incompatible with the shared SKILL.md contract: " + failure.Message,
			})
		}
	}
	diagnostics = append(diagnostics, validateOpenCodeCommandFiles(root, state.ComponentPaths("commands"))...)
	diagnostics = append(diagnostics, validateOpenCodeAgentFiles(root, state.ComponentPaths("agents"))...)
	diagnostics = append(diagnostics, validateOpenCodeDefaultAgent(root, state.DocPath("default_agent"))...)
	diagnostics = append(diagnostics, validateOpenCodeInstructions(root, state.DocPath("instructions_config"))...)
	diagnostics = append(diagnostics, validateOpenCodePermission(root, state.DocPath("permission_config"))...)
	diagnostics = append(diagnostics, validateOpenCodeThemeFiles(root, state.ComponentPaths("themes"))...)
	packageDoc, packageDiagnostics := validateOpenCodePluginPackageJSON(root, state.DocPath("local_plugin_dependencies"))
	diagnostics = append(diagnostics, packageDiagnostics...)
	diagnostics = append(diagnostics, validateOpenCodeToolFiles(root, state.ComponentPaths("tools"), packageDoc)...)
	diagnostics = append(diagnostics, validateOpenCodePluginFiles(root, state.ComponentPaths("local_plugin_code"), packageDoc)...)
	return diagnostics, nil
}

func validateOpenCodeCommandFiles(root string, rels []string) []Diagnostic {
	var diagnostics []Diagnostic
	for _, rel := range rels {
		if filepath.Ext(rel) != ".md" {
			diagnostics = append(diagnostics, Diagnostic{
				Severity: SeverityFailure,
				Code:     CodeManifestInvalid,
				Path:     rel,
				Target:   "opencode",
				Message:  fmt.Sprintf("OpenCode command file %s must use the .md extension", rel),
			})
			continue
		}
		body, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			diagnostics = append(diagnostics, Diagnostic{
				Severity: SeverityFailure,
				Code:     CodeManifestInvalid,
				Path:     rel,
				Target:   "opencode",
				Message:  fmt.Sprintf("OpenCode command file %s is not readable: %v", rel, err),
			})
			continue
		}
		frontmatter, markdown, err := parseMarkdownFrontmatterDocument(body, "OpenCode command file "+rel)
		if err != nil {
			diagnostics = append(diagnostics, Diagnostic{
				Severity: SeverityFailure,
				Code:     CodeManifestInvalid,
				Path:     rel,
				Target:   "opencode",
				Message:  err.Error(),
			})
			continue
		}
		if strings.TrimSpace(markdown) == "" {
			diagnostics = append(diagnostics, Diagnostic{
				Severity: SeverityFailure,
				Code:     CodeManifestInvalid,
				Path:     rel,
				Target:   "opencode",
				Message:  fmt.Sprintf("OpenCode command file %s must define a markdown command template body", rel),
			})
		}
		if description, ok := frontmatter["description"]; ok {
			text, ok := description.(string)
			if !ok || strings.TrimSpace(text) == "" {
				diagnostics = append(diagnostics, Diagnostic{
					Severity: SeverityFailure,
					Code:     CodeManifestInvalid,
					Path:     rel,
					Target:   "opencode",
					Message:  fmt.Sprintf("OpenCode command file %s frontmatter field %q must be a non-empty string", rel, "description"),
				})
			}
		}
	}
	return diagnostics
}

func validateOpenCodeAgentFiles(root string, rels []string) []Diagnostic {
	var diagnostics []Diagnostic
	for _, rel := range rels {
		if filepath.Ext(rel) != ".md" {
			diagnostics = append(diagnostics, Diagnostic{
				Severity: SeverityFailure,
				Code:     CodeManifestInvalid,
				Path:     rel,
				Target:   "opencode",
				Message:  fmt.Sprintf("OpenCode agent file %s must use the .md extension", rel),
			})
			continue
		}
		body, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			diagnostics = append(diagnostics, Diagnostic{
				Severity: SeverityFailure,
				Code:     CodeManifestInvalid,
				Path:     rel,
				Target:   "opencode",
				Message:  fmt.Sprintf("OpenCode agent file %s is not readable: %v", rel, err),
			})
			continue
		}
		frontmatter, _, err := parseMarkdownFrontmatterDocument(body, "OpenCode agent file "+rel)
		if err != nil {
			diagnostics = append(diagnostics, Diagnostic{
				Severity: SeverityFailure,
				Code:     CodeManifestInvalid,
				Path:     rel,
				Target:   "opencode",
				Message:  err.Error(),
			})
			continue
		}
		description, ok := frontmatter["description"].(string)
		if !ok || strings.TrimSpace(description) == "" {
			diagnostics = append(diagnostics, Diagnostic{
				Severity: SeverityFailure,
				Code:     CodeManifestInvalid,
				Path:     rel,
				Target:   "opencode",
				Message:  fmt.Sprintf("OpenCode agent file %s must declare a non-empty frontmatter description", rel),
			})
		}
		if mode, ok := frontmatter["mode"]; ok {
			text, ok := mode.(string)
			if !ok || strings.TrimSpace(text) == "" {
				diagnostics = append(diagnostics, Diagnostic{
					Severity: SeverityFailure,
					Code:     CodeManifestInvalid,
					Path:     rel,
					Target:   "opencode",
					Message:  fmt.Sprintf("OpenCode agent file %s frontmatter field %q must be a non-empty string", rel, "mode"),
				})
			}
		}
		if model, ok := frontmatter["model"]; ok {
			text, ok := model.(string)
			if !ok || strings.TrimSpace(text) == "" {
				diagnostics = append(diagnostics, Diagnostic{
					Severity: SeverityFailure,
					Code:     CodeManifestInvalid,
					Path:     rel,
					Target:   "opencode",
					Message:  fmt.Sprintf("OpenCode agent file %s frontmatter field %q must be a non-empty string", rel, "model"),
				})
			}
		}
		if variant, ok := frontmatter["variant"]; ok {
			text, ok := variant.(string)
			if !ok || strings.TrimSpace(text) == "" {
				diagnostics = append(diagnostics, Diagnostic{
					Severity: SeverityFailure,
					Code:     CodeManifestInvalid,
					Path:     rel,
					Target:   "opencode",
					Message:  fmt.Sprintf("OpenCode agent file %s frontmatter field %q must be a non-empty string", rel, "variant"),
				})
			}
		}
		for _, numericField := range []string{"temperature", "top_p"} {
			if raw, ok := frontmatter[numericField]; ok {
				if _, ok := raw.(float64); !ok {
					diagnostics = append(diagnostics, Diagnostic{
						Severity: SeverityFailure,
						Code:     CodeManifestInvalid,
						Path:     rel,
						Target:   "opencode",
						Message:  fmt.Sprintf("OpenCode agent file %s frontmatter field %q must be a number", rel, numericField),
					})
				}
			}
		}
		for _, boolField := range []string{"disable", "hidden"} {
			if raw, ok := frontmatter[boolField]; ok {
				if _, ok := raw.(bool); !ok {
					diagnostics = append(diagnostics, Diagnostic{
						Severity: SeverityFailure,
						Code:     CodeManifestInvalid,
						Path:     rel,
						Target:   "opencode",
						Message:  fmt.Sprintf("OpenCode agent file %s frontmatter field %q must be a boolean", rel, boolField),
					})
				}
			}
		}
		if raw, ok := frontmatter["color"]; ok {
			text, ok := raw.(string)
			if !ok || strings.TrimSpace(text) == "" {
				diagnostics = append(diagnostics, Diagnostic{
					Severity: SeverityFailure,
					Code:     CodeManifestInvalid,
					Path:     rel,
					Target:   "opencode",
					Message:  fmt.Sprintf("OpenCode agent file %s frontmatter field %q must be a non-empty string", rel, "color"),
				})
			}
		}
		if raw, ok := frontmatter["steps"]; ok {
			value, ok := raw.(int)
			if !ok || value <= 0 {
				if floatValue, ok := raw.(float64); !ok || floatValue != float64(int(floatValue)) || int(floatValue) <= 0 {
					diagnostics = append(diagnostics, Diagnostic{
						Severity: SeverityFailure,
						Code:     CodeManifestInvalid,
						Path:     rel,
						Target:   "opencode",
						Message:  fmt.Sprintf("OpenCode agent file %s frontmatter field %q must be a positive integer", rel, "steps"),
					})
				}
			}
		}
		if raw, ok := frontmatter["options"]; ok {
			if _, ok := raw.(map[string]any); !ok {
				diagnostics = append(diagnostics, Diagnostic{
					Severity: SeverityFailure,
					Code:     CodeManifestInvalid,
					Path:     rel,
					Target:   "opencode",
					Message:  fmt.Sprintf("OpenCode agent file %s frontmatter field %q must be an object", rel, "options"),
				})
			}
		}
		if raw, ok := frontmatter["permission"]; ok && !isOpenCodePermissionValue(raw) {
			diagnostics = append(diagnostics, Diagnostic{
				Severity: SeverityFailure,
				Code:     CodeManifestInvalid,
				Path:     rel,
				Target:   "opencode",
				Message:  fmt.Sprintf("OpenCode agent file %s frontmatter field %q must be a string or object", rel, "permission"),
			})
		}
		if _, ok := frontmatter["tools"]; ok {
			diagnostics = append(diagnostics, Diagnostic{
				Severity: SeverityFailure,
				Code:     CodeManifestInvalid,
				Path:     rel,
				Target:   "opencode",
				Message:  fmt.Sprintf("OpenCode agent file %s frontmatter field %q is deprecated; use %q instead", rel, "tools", "permission"),
			})
		}
		if _, ok := frontmatter["maxSteps"]; ok {
			diagnostics = append(diagnostics, Diagnostic{
				Severity: SeverityFailure,
				Code:     CodeManifestInvalid,
				Path:     rel,
				Target:   "opencode",
				Message:  fmt.Sprintf("OpenCode agent file %s frontmatter field %q is deprecated; use %q instead", rel, "maxSteps", "steps"),
			})
		}
	}
	return diagnostics
}

func validateOpenCodeDefaultAgent(root string, rel string) []Diagnostic {
	if strings.TrimSpace(rel) == "" {
		return nil
	}
	body, err := os.ReadFile(filepath.Join(root, rel))
	if err != nil {
		return []Diagnostic{{
			Severity: SeverityFailure,
			Code:     CodeManifestInvalid,
			Path:     rel,
			Target:   "opencode",
			Message:  fmt.Sprintf("OpenCode default agent file %s is not readable: %v", rel, err),
		}}
	}
	if strings.TrimSpace(string(body)) == "" {
		return []Diagnostic{{
			Severity: SeverityFailure,
			Code:     CodeManifestInvalid,
			Path:     rel,
			Target:   "opencode",
			Message:  fmt.Sprintf("OpenCode default agent file %s must contain a non-empty agent name", rel),
		}}
	}
	return nil
}

func validateOpenCodeInstructions(root string, rel string) []Diagnostic {
	if strings.TrimSpace(rel) == "" {
		return nil
	}
	values, _, err := readYAMLDoc[[]string](root, rel)
	if err != nil {
		return []Diagnostic{{
			Severity: SeverityFailure,
			Code:     CodeManifestInvalid,
			Path:     rel,
			Target:   "opencode",
			Message:  fmt.Sprintf("parse %s: %v", rel, err),
		}}
	}
	if len(values) == 0 {
		return []Diagnostic{{
			Severity: SeverityFailure,
			Code:     CodeManifestInvalid,
			Path:     rel,
			Target:   "opencode",
			Message:  fmt.Sprintf("OpenCode instructions file %s must contain at least one instruction path", rel),
		}}
	}
	var diagnostics []Diagnostic
	for i, value := range values {
		if strings.TrimSpace(value) == "" {
			diagnostics = append(diagnostics, Diagnostic{
				Severity: SeverityFailure,
				Code:     CodeManifestInvalid,
				Path:     rel,
				Target:   "opencode",
				Message:  fmt.Sprintf("OpenCode instructions file %s entry %d must be a non-empty string", rel, i),
			})
		}
	}
	return diagnostics
}

func validateOpenCodePermission(root string, rel string) []Diagnostic {
	if strings.TrimSpace(rel) == "" {
		return nil
	}
	body, err := os.ReadFile(filepath.Join(root, rel))
	if err != nil {
		return []Diagnostic{{
			Severity: SeverityFailure,
			Code:     CodeManifestInvalid,
			Path:     rel,
			Target:   "opencode",
			Message:  fmt.Sprintf("OpenCode permission file %s is not readable: %v", rel, err),
		}}
	}
	var permission any
	if err := json.Unmarshal(body, &permission); err != nil {
		return []Diagnostic{{
			Severity: SeverityFailure,
			Code:     CodeManifestInvalid,
			Path:     rel,
			Target:   "opencode",
			Message:  fmt.Sprintf("parse %s: %v", rel, err),
		}}
	}
	if !isOpenCodePermissionValue(permission) {
		return []Diagnostic{{
			Severity: SeverityFailure,
			Code:     CodeManifestInvalid,
			Path:     rel,
			Target:   "opencode",
			Message:  fmt.Sprintf("OpenCode permission file %s must be a JSON string or object", rel),
		}}
	}
	return nil
}

func validateOpenCodeThemeFiles(root string, rels []string) []Diagnostic {
	var diagnostics []Diagnostic
	for _, rel := range rels {
		if filepath.Ext(rel) != ".json" {
			diagnostics = append(diagnostics, Diagnostic{
				Severity: SeverityFailure,
				Code:     CodeManifestInvalid,
				Path:     rel,
				Target:   "opencode",
				Message:  fmt.Sprintf("OpenCode theme file %s must use the .json extension", rel),
			})
			continue
		}
		body, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			diagnostics = append(diagnostics, Diagnostic{
				Severity: SeverityFailure,
				Code:     CodeManifestInvalid,
				Path:     rel,
				Target:   "opencode",
				Message:  fmt.Sprintf("OpenCode theme file %s is not readable: %v", rel, err),
			})
			continue
		}
		doc, err := decodeJSONObject(body, "OpenCode theme file "+rel)
		if err != nil {
			diagnostics = append(diagnostics, Diagnostic{
				Severity: SeverityFailure,
				Code:     CodeManifestInvalid,
				Path:     rel,
				Target:   "opencode",
				Message:  err.Error(),
			})
			continue
		}
		if _, ok := doc["theme"]; !ok {
			diagnostics = append(diagnostics, Diagnostic{
				Severity: SeverityFailure,
				Code:     CodeManifestInvalid,
				Path:     rel,
				Target:   "opencode",
				Message:  fmt.Sprintf("OpenCode theme file %s must define a top-level theme object", rel),
			})
		}
	}
	return diagnostics
}

func validateOpenCodeToolFiles(root string, rels []string, packageDoc map[string]any) []Diagnostic {
	if len(rels) == 0 {
		return nil
	}
	var (
		diagnostics      []Diagnostic
		hasDefinition    bool
		usesPluginHelper bool
		seenCaseFolded   = map[string]string{}
	)
	for _, rel := range rels {
		clean := filepath.ToSlash(filepath.Clean(rel))
		if clean != rel || strings.Contains(clean, "..") {
			diagnostics = append(diagnostics, Diagnostic{
				Severity: SeverityFailure,
				Code:     CodeManifestInvalid,
				Path:     rel,
				Target:   "opencode",
				Message:  fmt.Sprintf("OpenCode tool file %s must stay within targets/opencode/tools without path traversal", rel),
			})
			continue
		}
		lower := strings.ToLower(clean)
		if prior, ok := seenCaseFolded[lower]; ok && prior != clean {
			diagnostics = append(diagnostics, Diagnostic{
				Severity: SeverityFailure,
				Code:     CodeManifestInvalid,
				Path:     rel,
				Target:   "opencode",
				Message:  fmt.Sprintf("OpenCode tool files %s and %s collide on case-insensitive filesystems", prior, rel),
			})
		} else {
			seenCaseFolded[lower] = clean
		}
		fullPath := filepath.Join(root, rel)
		info, err := os.Lstat(fullPath)
		if err != nil {
			diagnostics = append(diagnostics, Diagnostic{
				Severity: SeverityFailure,
				Code:     CodeManifestInvalid,
				Path:     rel,
				Target:   "opencode",
				Message:  fmt.Sprintf("OpenCode tool file %s is not readable: %v", rel, err),
			})
			continue
		}
		if info.Mode()&os.ModeSymlink != 0 {
			diagnostics = append(diagnostics, Diagnostic{
				Severity: SeverityFailure,
				Code:     CodeManifestInvalid,
				Path:     rel,
				Target:   "opencode",
				Message:  fmt.Sprintf("OpenCode tool file %s must not be a symlink", rel),
			})
			continue
		}
		if info.IsDir() {
			continue
		}
		body, err := os.ReadFile(fullPath)
		if err != nil {
			diagnostics = append(diagnostics, Diagnostic{
				Severity: SeverityFailure,
				Code:     CodeManifestInvalid,
				Path:     rel,
				Target:   "opencode",
				Message:  fmt.Sprintf("OpenCode tool file %s is not readable: %v", rel, err),
			})
			continue
		}
		if isOpenCodePluginEntryFile(rel) {
			hasDefinition = true
		}
		if strings.Contains(string(body), `@opencode-ai/plugin`) {
			usesPluginHelper = true
		}
	}
	if !hasDefinition {
		diagnostics = append(diagnostics, Diagnostic{
			Severity: SeverityFailure,
			Code:     CodeManifestInvalid,
			Path:     filepath.ToSlash(filepath.Join("src", "targets", "opencode", "tools")),
			Target:   "opencode",
			Message:  "OpenCode standalone tools require at least one JS/TS tool definition file under src/targets/opencode/tools",
		})
	}
	if usesPluginHelper && !openCodePackageDeclaresDependency(packageDoc, "@opencode-ai/plugin") {
		diagnostics = append(diagnostics, Diagnostic{
			Severity: SeverityFailure,
			Code:     CodeManifestInvalid,
			Path:     filepath.ToSlash(filepath.Join("src", "targets", "opencode", "package.json")),
			Target:   "opencode",
			Message:  `OpenCode standalone tool files that import "@opencode-ai/plugin" must declare that dependency in src/targets/opencode/package.json`,
		})
	}
	return diagnostics
}

func validateOpenCodePluginPackageJSON(root string, rel string) (map[string]any, []Diagnostic) {
	if strings.TrimSpace(rel) == "" {
		return nil, nil
	}
	body, err := os.ReadFile(filepath.Join(root, rel))
	if err != nil {
		return nil, []Diagnostic{{
			Severity: SeverityFailure,
			Code:     CodeManifestInvalid,
			Path:     rel,
			Target:   "opencode",
			Message:  fmt.Sprintf("OpenCode plugin dependency metadata %s is not readable: %v", rel, err),
		}}
	}
	doc, err := decodeJSONObject(body, "OpenCode plugin dependency metadata "+rel)
	if err != nil {
		return nil, []Diagnostic{{
			Severity: SeverityFailure,
			Code:     CodeManifestInvalid,
			Path:     rel,
			Target:   "opencode",
			Message:  err.Error(),
		}}
	}
	return doc, nil
}

func validateOpenCodePluginFiles(root string, rels []string, packageDoc map[string]any) []Diagnostic {
	if len(rels) == 0 {
		return nil
	}
	var (
		diagnostics      []Diagnostic
		hasEntry         bool
		usesPluginHelper bool
	)
	for _, rel := range rels {
		fullPath := filepath.Join(root, rel)
		info, err := os.Stat(fullPath)
		if err != nil {
			diagnostics = append(diagnostics, Diagnostic{
				Severity: SeverityFailure,
				Code:     CodeManifestInvalid,
				Path:     rel,
				Target:   "opencode",
				Message:  fmt.Sprintf("OpenCode local plugin file %s is not readable: %v", rel, err),
			})
			continue
		}
		if info.IsDir() {
			continue
		}
		if isOpenCodePluginEntryFile(rel) {
			hasEntry = true
		}
		body, err := os.ReadFile(fullPath)
		if err != nil {
			diagnostics = append(diagnostics, Diagnostic{
				Severity: SeverityFailure,
				Code:     CodeManifestInvalid,
				Path:     rel,
				Target:   "opencode",
				Message:  fmt.Sprintf("OpenCode local plugin file %s is not readable: %v", rel, err),
			})
			continue
		}
		src := string(body)
		if strings.Contains(src, `export default`) && strings.Contains(src, `setup(`) {
			diagnostics = append(diagnostics, Diagnostic{
				Severity: SeverityFailure,
				Code:     CodeManifestInvalid,
				Path:     rel,
				Target:   "opencode",
				Message:  "OpenCode local plugin file uses the old scaffold shape `export default { setup() { ... } }`; use official named async plugin exports instead",
			})
		}
		if strings.Contains(src, `@opencode-ai/plugin`) {
			usesPluginHelper = true
		}
	}
	if !hasEntry {
		diagnostics = append(diagnostics, Diagnostic{
			Severity: SeverityFailure,
			Code:     CodeManifestInvalid,
			Path:     filepath.ToSlash(filepath.Join("src", "targets", "opencode", "plugins")),
			Target:   "opencode",
			Message:  "OpenCode local plugin code requires at least one JS/TS plugin entry file under src/targets/opencode/plugins (for example .js, .mjs, .cjs, .ts, .mts, or .cts)",
		})
	}
	if usesPluginHelper && !openCodePackageDeclaresDependency(packageDoc, "@opencode-ai/plugin") {
		diagnostics = append(diagnostics, Diagnostic{
			Severity: SeverityFailure,
			Code:     CodeManifestInvalid,
			Path:     filepath.ToSlash(filepath.Join("src", "targets", "opencode", "package.json")),
			Target:   "opencode",
			Message:  `OpenCode plugin files that import "@opencode-ai/plugin" must declare that dependency in src/targets/opencode/package.json`,
		})
	}
	return diagnostics
}

func openCodePackageDeclaresDependency(doc map[string]any, name string) bool {
	if len(doc) == 0 {
		return false
	}
	for _, field := range []string{"dependencies", "devDependencies", "peerDependencies"} {
		raw, ok := doc[field]
		if !ok {
			continue
		}
		deps, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if value, ok := deps[name]; ok {
			if text, ok := value.(string); ok && strings.TrimSpace(text) != "" {
				return true
			}
		}
	}
	return false
}

func isOpenCodePluginEntryFile(rel string) bool {
	switch strings.ToLower(filepath.Ext(rel)) {
	case ".js", ".mjs", ".cjs", ".ts", ".mts", ".cts":
		return true
	default:
		return false
	}
}
