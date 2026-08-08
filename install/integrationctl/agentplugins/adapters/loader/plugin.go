package loader

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/777genius/plugin-kit-ai/install/integrationctl/adapters/pathpolicy"
	"github.com/777genius/plugin-kit-ai/install/integrationctl/agentplugins/domain"
)

var pluginManifestFields = map[string]struct{}{
	"$schema": {}, "name": {}, "version": {}, "description": {}, "author": {},
	"homepage": {}, "repository": {}, "license": {}, "keywords": {}, "extensions": {},
}

func (loader Loader) loadPluginManifest(path string) (domain.PluginManifest, []domain.Diagnostic, string, error) {
	body, exists, err := readRegularFile(path)
	if err != nil {
		return domain.PluginManifest{}, nil, "", domain.FatalLoad("plugin_manifest_read_failed", "plugin.json", "read root plugin.json", err)
	}
	if !exists {
		return domain.PluginManifest{}, nil, "", domain.FatalLoad("plugin_manifest_missing", "plugin.json", "root plugin.json is required", nil)
	}
	rawFields, decoded, err := decodeJSONObject(body)
	if err != nil {
		return domain.PluginManifest{}, nil, "", domain.FatalLoad("plugin_manifest_malformed", "plugin.json", "parse root plugin.json", err)
	}
	schemaURI, ok := decoded["$schema"].(string)
	if !ok || strings.TrimSpace(schemaURI) == "" {
		return domain.PluginManifest{}, nil, "", domain.FatalLoad("plugin_schema_missing", "plugin.json", "plugin.json requires a string $schema", nil)
	}
	if schemaURI != domain.PluginSchemaV1 || !loader.Registry.Supports(schemaURI) {
		return domain.PluginManifest{}, nil, "", domain.FatalLoad("plugin_schema_unsupported", "plugin.json", fmt.Sprintf("unsupported Agent Plugins schema %q", schemaURI), nil)
	}

	var diagnostics []domain.Diagnostic
	validationDocument := make(map[string]any, len(decoded))
	for key, value := range decoded {
		if _, known := pluginManifestFields[key]; !known {
			diagnostics = append(diagnostics, domain.Diagnostic{
				Severity: domain.SeverityWarning,
				Boundary: domain.BoundaryPlugin,
				Code:     "plugin_unknown_field",
				Path:     "plugin.json",
				Item:     key,
				Message:  fmt.Sprintf("unknown plugin.json field %q was preserved but is not interpreted", key),
			})
			continue
		}
		validationDocument[key] = value
	}

	extensions := map[string]json.RawMessage{}
	var rawExtensions json.RawMessage
	if raw, exists := rawFields["extensions"]; exists {
		rawExtensions = append(json.RawMessage(nil), raw...)
		var extensionFields map[string]json.RawMessage
		if err := json.Unmarshal(raw, &extensionFields); err != nil || extensionFields == nil {
			diagnostics = append(diagnostics, domain.Diagnostic{
				Severity: domain.SeverityWarning,
				Boundary: domain.BoundaryExtension,
				Code:     "extensions_ignored",
				Path:     "plugin.json",
				Message:  "plugin extensions is not an object; raw bytes were preserved but extensions were ignored",
			})
			delete(validationDocument, "extensions")
		} else {
			validForSchema := map[string]any{}
			for namespace, extensionRaw := range extensionFields {
				extensions[namespace] = append(json.RawMessage(nil), extensionRaw...)
				var extensionValue any
				if err := decodeJSON(extensionRaw, &extensionValue); err != nil {
					continue
				}
				if _, object := extensionValue.(map[string]any); !object {
					diagnostics = append(diagnostics, domain.Diagnostic{
						Severity: domain.SeverityWarning,
						Boundary: domain.BoundaryExtension,
						Code:     "extension_value_ignored",
						Path:     "plugin.json",
						Item:     namespace,
						Message:  fmt.Sprintf("extension %q is not an object; raw bytes were preserved but the extension was ignored", namespace),
					})
					continue
				}
				validForSchema[namespace] = extensionValue
			}
			validationDocument["extensions"] = validForSchema
		}
	}
	if err := loader.Registry.Validate(schemaURI, validationDocument); err != nil {
		return domain.PluginManifest{}, diagnostics, "", domain.FatalLoad("plugin_schema_invalid", "plugin.json", "plugin.json does not conform to Agent Plugins 1.0", err)
	}

	var typed struct {
		Schema      string         `json:"$schema"`
		Name        string         `json:"name"`
		Version     string         `json:"version"`
		Description string         `json:"description"`
		Author      *domain.Author `json:"author"`
		Homepage    string         `json:"homepage"`
		Repository  string         `json:"repository"`
		License     string         `json:"license"`
		Keywords    []string       `json:"keywords"`
	}
	if err := json.Unmarshal(body, &typed); err != nil {
		return domain.PluginManifest{}, diagnostics, "", domain.FatalLoad("plugin_manifest_decode_failed", "plugin.json", "decode plugin.json fields", err)
	}
	if err := pathpolicy.ValidateLeafID(typed.Name); err != nil {
		return domain.PluginManifest{}, diagnostics, "", domain.FatalLoad("plugin_name_unsafe", "plugin.json", "plugin name cannot be used as a portable physical identifier", err)
	}
	unknown := map[string]json.RawMessage{}
	for key, raw := range rawFields {
		if _, known := pluginManifestFields[key]; !known {
			unknown[key] = append(json.RawMessage(nil), raw...)
		}
	}
	return domain.PluginManifest{
		SchemaURI:     typed.Schema,
		Name:          typed.Name,
		Version:       typed.Version,
		Description:   typed.Description,
		Author:        typed.Author,
		Homepage:      typed.Homepage,
		Repository:    typed.Repository,
		License:       typed.License,
		Keywords:      append([]string(nil), typed.Keywords...),
		Extensions:    extensions,
		RawExtensions: rawExtensions,
		Unknown:       unknown,
		Raw:           append(json.RawMessage(nil), body...),
	}, diagnostics, sha256Digest(body), nil
}

func decodeJSONObject(body []byte) (map[string]json.RawMessage, map[string]any, error) {
	var raw map[string]json.RawMessage
	if err := decodeJSON(body, &raw); err != nil {
		return nil, nil, err
	}
	if raw == nil {
		return nil, nil, fmt.Errorf("document must be a JSON object")
	}
	var decoded map[string]any
	if err := decodeJSON(body, &decoded); err != nil {
		return nil, nil, err
	}
	return raw, decoded, nil
}

func decodeJSON(body []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			return fmt.Errorf("document contains multiple JSON values")
		}
		return err
	}
	return nil
}
