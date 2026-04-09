package pluginmanifest

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

func loadManifest(root string) (Manifest, error) {
	manifest, _, err := loadManifestWithWarnings(root)
	return manifest, err
}

func loadLauncher(root string) (Launcher, error) {
	launcher, _, err := loadLauncherWithWarnings(root)
	return launcher, err
}

func loadManifestWithWarnings(root string) (Manifest, []Warning, error) {
	layout, err := detectAuthoredLayout(root)
	if err != nil {
		return Manifest{}, nil, err
	}
	body, err := os.ReadFile(filepath.Join(root, layout.Path(FileName)))
	if err != nil {
		return Manifest{}, nil, err
	}
	return analyzeManifest(body)
}

func loadLauncherWithWarnings(root string) (Launcher, []Warning, error) {
	layout, err := detectAuthoredLayout(root)
	if err != nil {
		return Launcher{}, nil, err
	}
	body, err := os.ReadFile(filepath.Join(root, layout.Path(LauncherFileName)))
	if err != nil {
		return Launcher{}, nil, err
	}
	return analyzeLauncher(body)
}

func parseManifest(body []byte) (Manifest, error) {
	manifest, _, err := analyzeManifest(body)
	return manifest, err
}

func parseLauncher(body []byte) (Launcher, error) {
	launcher, _, err := analyzeLauncher(body)
	return launcher, err
}

func analyzeManifest(body []byte) (Manifest, []Warning, error) {
	var raw map[string]any
	if err := yaml.Unmarshal(body, &raw); err != nil {
		return Manifest{}, nil, fmt.Errorf("parse plugin.yaml: %w", err)
	}
	if _, ok := raw["schema_version"]; ok {
		return Manifest{}, nil, fmt.Errorf("unsupported plugin.yaml format: schema_version-based manifests are not supported; use package-standard plugin.yaml with targets")
	}
	if _, ok := raw["components"]; ok {
		return Manifest{}, nil, fmt.Errorf("unsupported plugin.yaml format: flat components inventory is not supported; use package-standard plugin.yaml plus conventions")
	}
	if rawTargets, ok := raw["targets"]; ok {
		if _, oldShape := rawTargets.(map[string]any); oldShape {
			return Manifest{}, nil, fmt.Errorf("unsupported plugin.yaml format: targets must be a YAML sequence")
		}
	}
	if _, ok := raw["runtime"]; ok {
		return Manifest{}, nil, fmt.Errorf("unsupported plugin.yaml format: runtime moved to %s", LauncherFileName)
	}
	if _, ok := raw["entrypoint"]; ok {
		return Manifest{}, nil, fmt.Errorf("unsupported plugin.yaml format: entrypoint moved to %s", LauncherFileName)
	}
	if rawFormat, hasFormat := raw["format"]; hasFormat && strings.TrimSpace(fmt.Sprint(rawFormat)) != "" {
		return Manifest{}, nil, fmt.Errorf("unsupported plugin.yaml field: format")
	}
	if apiVersion, hasAPIVersion := raw["api_version"]; hasAPIVersion {
		if strings.TrimSpace(fmt.Sprint(apiVersion)) != APIVersionV1 {
			return Manifest{}, nil, fmt.Errorf("unsupported plugin.yaml api_version %q: expected %q", strings.TrimSpace(fmt.Sprint(apiVersion)), APIVersionV1)
		}
	}
	if err := validateSchema(body, FileName, manifestSchema(), true); err != nil {
		return Manifest{}, nil, err
	}
	warnings, err := collectWarnings(body)
	if err != nil {
		return Manifest{}, nil, err
	}
	var out Manifest
	if err := yaml.Unmarshal(body, &out); err != nil {
		return Manifest{}, nil, fmt.Errorf("parse plugin.yaml: %w", err)
	}
	normalizeManifest(&out)
	if err := out.Validate(); err != nil {
		return Manifest{}, warnings, err
	}
	return out, warnings, nil
}

func analyzeLauncher(body []byte) (Launcher, []Warning, error) {
	if err := validateSchema(body, LauncherFileName, launcherSchema(), false); err != nil {
		return Launcher{}, nil, err
	}
	var out Launcher
	if err := yaml.Unmarshal(body, &out); err != nil {
		return Launcher{}, nil, fmt.Errorf("parse %s: %w", LauncherFileName, err)
	}
	normalizeLauncher(&out)
	if err := out.Validate(); err != nil {
		return Launcher{}, nil, err
	}
	return out, nil, nil
}

func defaultManifest(projectName, platform, runtime, description string) Manifest {
	platform = normalizeTarget(platform)
	if strings.TrimSpace(description) == "" {
		description = "plugin-kit-ai plugin"
	}
	return Manifest{
		APIVersion:  APIVersionV1,
		Name:        projectName,
		Version:     "0.1.0",
		Description: description,
		Targets:     []string{platform},
	}
}

func defaultLauncher(projectName, runtime string) Launcher {
	runtime = normalizeRuntime(runtime)
	if runtime == "" {
		runtime = "go"
	}
	return Launcher{
		Runtime:    runtime,
		Entrypoint: "./bin/" + projectName,
	}
}

func saveManifest(root string, manifest Manifest, force bool) error {
	layout, err := detectAuthoredLayout(root)
	if err != nil {
		return err
	}
	normalizeManifest(&manifest)
	if err := manifest.Validate(); err != nil {
		return err
	}
	full := filepath.Join(root, layout.Path(FileName))
	if _, err := os.Stat(full); err == nil && !force {
		return fmt.Errorf("refusing to overwrite existing file %s (use --force)", FileName)
	}
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return err
	}
	body, err := yaml.Marshal(manifest)
	if err != nil {
		return fmt.Errorf("marshal plugin.yaml: %w", err)
	}
	return os.WriteFile(full, body, 0o644)
}

func saveLauncher(root string, launcher Launcher, force bool) error {
	layout, err := detectAuthoredLayout(root)
	if err != nil {
		return err
	}
	normalizeLauncher(&launcher)
	if err := launcher.Validate(); err != nil {
		return err
	}
	full := filepath.Join(root, layout.Path(LauncherFileName))
	if _, err := os.Stat(full); err == nil && !force {
		return fmt.Errorf("refusing to overwrite existing file %s (use --force)", LauncherFileName)
	}
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return err
	}
	body, err := yaml.Marshal(launcher)
	if err != nil {
		return fmt.Errorf("marshal %s: %w", LauncherFileName, err)
	}
	return os.WriteFile(full, body, 0o644)
}

func normalizePackage(root string, force bool) ([]Warning, error) {
	manifest, warnings, err := loadManifestWithWarnings(root)
	if err != nil {
		return nil, err
	}
	if err := saveManifest(root, manifest, force); err != nil {
		return warnings, err
	}
	if launcher, err := loadLauncher(root); err == nil {
		if err := saveLauncher(root, launcher, force); err != nil {
			return warnings, err
		}
	}
	return warnings, nil
}

type schemaSpec struct {
	Kind   yaml.Kind
	Scalar scalarKind
	Fields map[string]schemaSpec
	Seq    *schemaSpec
}

type scalarKind int

const (
	scalarAny scalarKind = iota
	scalarString
)

func manifestSchema() schemaSpec {
	return schemaSpec{Kind: yaml.MappingNode, Fields: map[string]schemaSpec{
		"api_version": {Kind: yaml.ScalarNode, Scalar: scalarString},
		"format":      {Kind: yaml.ScalarNode, Scalar: scalarString},
		"name":        {Kind: yaml.ScalarNode, Scalar: scalarString},
		"version":     {Kind: yaml.ScalarNode, Scalar: scalarString},
		"description": {Kind: yaml.ScalarNode, Scalar: scalarString},
		"author": {Kind: yaml.MappingNode, Fields: map[string]schemaSpec{
			"name":  {Kind: yaml.ScalarNode, Scalar: scalarString},
			"email": {Kind: yaml.ScalarNode, Scalar: scalarString},
			"url":   {Kind: yaml.ScalarNode, Scalar: scalarString},
		}},
		"homepage":   {Kind: yaml.ScalarNode, Scalar: scalarString},
		"repository": {Kind: yaml.ScalarNode, Scalar: scalarString},
		"license":    {Kind: yaml.ScalarNode, Scalar: scalarString},
		"keywords":   {Kind: yaml.SequenceNode, Seq: &schemaSpec{Kind: yaml.ScalarNode, Scalar: scalarString}},
		"targets":    {Kind: yaml.SequenceNode, Seq: &schemaSpec{Kind: yaml.ScalarNode, Scalar: scalarString}},
	}}
}

func launcherSchema() schemaSpec {
	return schemaSpec{Kind: yaml.MappingNode, Fields: map[string]schemaSpec{
		"runtime":    {Kind: yaml.ScalarNode, Scalar: scalarString},
		"entrypoint": {Kind: yaml.ScalarNode, Scalar: scalarString},
	}}
}

func walkNode(node *yaml.Node, path string, spec schemaSpec, seen map[string]struct{}, warnings *[]Warning) {
	if node == nil {
		return
	}
	if len(spec.Fields) > 0 && node.Kind == yaml.MappingNode {
		for i := 0; i < len(node.Content); i += 2 {
			keyNode := node.Content[i]
			valNode := node.Content[i+1]
			key := strings.TrimSpace(keyNode.Value)
			keyPath := joinPath(path, key)
			child, ok := spec.Fields[key]
			if !ok {
				appendWarning(seen, warnings, Warning{
					Kind:    WarningUnknownField,
					Path:    keyPath,
					Message: "unknown plugin.yaml field: " + keyPath,
				})
				continue
			}
			walkNode(valNode, keyPath, child, seen, warnings)
		}
		return
	}
	if spec.Seq != nil && node.Kind == yaml.SequenceNode {
		for idx, item := range node.Content {
			walkNode(item, fmt.Sprintf("%s[%d]", path, idx), *spec.Seq, seen, warnings)
		}
	}
}

func validateSchema(body []byte, label string, spec schemaSpec, allowUnknown bool) error {
	var doc yaml.Node
	dec := yaml.NewDecoder(bytes.NewReader(body))
	if err := dec.Decode(&doc); err != nil {
		return fmt.Errorf("parse %s: %w", label, err)
	}
	if len(doc.Content) == 0 {
		return nil
	}
	return validateSchemaNode(doc.Content[0], label, spec, allowUnknown)
}

func validateSchemaNode(node *yaml.Node, path string, spec schemaSpec, allowUnknown bool) error {
	if node == nil {
		return nil
	}
	if spec.Kind != 0 && node.Kind != spec.Kind {
		return fmt.Errorf("invalid %s: expected %s", path, describeSchemaKind(spec.Kind))
	}
	switch spec.Kind {
	case yaml.MappingNode:
		seen := map[string]struct{}{}
		for i := 0; i < len(node.Content); i += 2 {
			keyNode := node.Content[i]
			valNode := node.Content[i+1]
			if keyNode.Kind != yaml.ScalarNode || !isStringSchemaScalar(keyNode) {
				return fmt.Errorf("invalid %s: mapping keys must be strings", path)
			}
			key := strings.TrimSpace(keyNode.Value)
			keyPath := joinPath(path, key)
			if _, ok := seen[key]; ok {
				return fmt.Errorf("invalid %s: duplicate field %q", path, key)
			}
			seen[key] = struct{}{}
			child, ok := spec.Fields[key]
			if !ok {
				if allowUnknown {
					continue
				}
				return fmt.Errorf("invalid %s: unknown field %q", path, key)
			}
			if err := validateSchemaNode(valNode, keyPath, child, allowUnknown); err != nil {
				return err
			}
		}
	case yaml.SequenceNode:
		for idx, item := range node.Content {
			if err := validateSchemaNode(item, fmt.Sprintf("%s[%d]", path, idx), *spec.Seq, allowUnknown); err != nil {
				return err
			}
		}
	case yaml.ScalarNode:
		if spec.Scalar == scalarString && !isStringSchemaScalar(node) {
			return fmt.Errorf("invalid %s: expected string", path)
		}
	}
	return nil
}

func isStringSchemaScalar(node *yaml.Node) bool {
	switch node.Tag {
	case "", "!!str", "tag:yaml.org,2002:str", "!!null", "tag:yaml.org,2002:null":
		return true
	default:
		return false
	}
}

func describeSchemaKind(kind yaml.Kind) string {
	switch kind {
	case yaml.MappingNode:
		return "a YAML mapping"
	case yaml.SequenceNode:
		return "a YAML sequence"
	case yaml.ScalarNode:
		return "a YAML scalar"
	default:
		return "a valid YAML value"
	}
}

func collectWarnings(body []byte) ([]Warning, error) {
	var doc yaml.Node
	dec := yaml.NewDecoder(bytes.NewReader(body))
	if err := dec.Decode(&doc); err != nil {
		return nil, fmt.Errorf("parse plugin.yaml: %w", err)
	}
	if len(doc.Content) == 0 {
		return nil, nil
	}
	var warnings []Warning
	seen := map[string]struct{}{}
	walkNode(doc.Content[0], "", manifestSchema(), seen, &warnings)
	return warnings, nil
}

func saveManifestWithLayout(root string, layout authoredLayout, manifest Manifest, force bool) error {
	normalizeManifest(&manifest)
	if err := manifest.Validate(); err != nil {
		return err
	}
	full := filepath.Join(root, layout.Path(FileName))
	if _, err := os.Stat(full); err == nil && !force {
		return fmt.Errorf("refusing to overwrite existing file %s (use --force)", layout.Path(FileName))
	}
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return err
	}
	body, err := yaml.Marshal(manifest)
	if err != nil {
		return fmt.Errorf("marshal %s: %w", layout.Path(FileName), err)
	}
	return os.WriteFile(full, body, 0o644)
}

func saveLauncherWithLayout(root string, layout authoredLayout, launcher Launcher, force bool) error {
	normalizeLauncher(&launcher)
	if err := launcher.Validate(); err != nil {
		return err
	}
	full := filepath.Join(root, layout.Path(LauncherFileName))
	if _, err := os.Stat(full); err == nil && !force {
		return fmt.Errorf("refusing to overwrite existing file %s (use --force)", layout.Path(LauncherFileName))
	}
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return err
	}
	body, err := yaml.Marshal(launcher)
	if err != nil {
		return fmt.Errorf("marshal %s: %w", layout.Path(LauncherFileName), err)
	}
	return os.WriteFile(full, body, 0o644)
}
