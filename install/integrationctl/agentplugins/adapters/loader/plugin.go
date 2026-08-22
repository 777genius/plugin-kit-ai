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
	for key, raw := range rawFields {
		if _, known := pluginManifestFields[key]; known {
			if err := rejectDuplicateJSONKeys(raw); err != nil {
				return domain.PluginManifest{}, nil, "", domain.FatalLoad("plugin_manifest_malformed", "plugin.json", fmt.Sprintf("parse plugin.json field %q", key), err)
			}
		}
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
			return domain.PluginManifest{}, diagnostics, "", domain.FatalLoad("plugin_schema_invalid", "plugin.json", "plugin.json extensions must be an object", err)
		}
		for namespace, extensionRaw := range extensionFields {
			var extensionValue any
			if err := decodeJSON(extensionRaw, &extensionValue); err != nil {
				return domain.PluginManifest{}, diagnostics, "", domain.FatalLoad("plugin_schema_invalid", "plugin.json", fmt.Sprintf("plugin extension %q is malformed", namespace), err)
			}
			if _, object := extensionValue.(map[string]any); !object {
				return domain.PluginManifest{}, diagnostics, "", domain.FatalLoad("plugin_schema_invalid", "plugin.json", fmt.Sprintf("plugin extension %q must be an object", namespace), nil)
			}
			extensions[namespace] = append(json.RawMessage(nil), extensionRaw...)
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
	if err := rejectDuplicateTopLevelObjectKeys(body); err != nil {
		return nil, nil, err
	}
	var raw map[string]json.RawMessage
	if err := decodeJSONUnchecked(body, &raw); err != nil {
		return nil, nil, err
	}
	if raw == nil {
		return nil, nil, fmt.Errorf("document must be a JSON object")
	}
	var decoded map[string]any
	if err := decodeJSONUnchecked(body, &decoded); err != nil {
		return nil, nil, err
	}
	return raw, decoded, nil
}

func decodeJSON(body []byte, target any) error {
	if err := rejectDuplicateJSONKeys(body); err != nil {
		return err
	}
	return decodeJSONUnchecked(body, target)
}

func decodeRawJSONObject(body []byte, target any) error {
	if err := rejectDuplicateTopLevelObjectKeys(body); err != nil {
		return err
	}
	return decodeJSONUnchecked(body, target)
}

func decodeJSONUnchecked(body []byte, target any) error {
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

func rejectDuplicateTopLevelObjectKeys(body []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	if token != json.Delim('{') {
		return fmt.Errorf("document must be a JSON object")
	}
	seen := map[string]struct{}{}
	for decoder.More() {
		keyToken, err := decoder.Token()
		if err != nil {
			return err
		}
		key, ok := keyToken.(string)
		if !ok {
			return fmt.Errorf("JSON object key must be a string")
		}
		if _, exists := seen[key]; exists {
			return fmt.Errorf("JSON object contains duplicate field %q", key)
		}
		seen[key] = struct{}{}
		var value json.RawMessage
		if err := decoder.Decode(&value); err != nil {
			return err
		}
	}
	closing, err := decoder.Token()
	if err != nil {
		return err
	}
	if closing != json.Delim('}') {
		return fmt.Errorf("JSON object is not terminated")
	}
	if _, err := decoder.Token(); err != io.EOF {
		if err == nil {
			return fmt.Errorf("document contains multiple JSON values")
		}
		return err
	}
	return nil
}

func rejectDuplicateJSONKeys(body []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	if err := consumeUniqueJSONValue(decoder, token); err != nil {
		return err
	}
	if _, err := decoder.Token(); err != io.EOF {
		if err == nil {
			return fmt.Errorf("document contains multiple JSON values")
		}
		return err
	}
	return nil
}

func consumeUniqueJSONValue(decoder *json.Decoder, token json.Token) error {
	delimiter, compound := token.(json.Delim)
	if !compound {
		return nil
	}
	switch delimiter {
	case '{':
		seen := map[string]struct{}{}
		for decoder.More() {
			keyToken, err := decoder.Token()
			if err != nil {
				return err
			}
			key, ok := keyToken.(string)
			if !ok {
				return fmt.Errorf("JSON object key must be a string")
			}
			if _, exists := seen[key]; exists {
				return fmt.Errorf("JSON object contains duplicate field %q", key)
			}
			seen[key] = struct{}{}
			valueToken, err := decoder.Token()
			if err != nil {
				return err
			}
			if err := consumeUniqueJSONValue(decoder, valueToken); err != nil {
				return err
			}
		}
		closing, err := decoder.Token()
		if err != nil {
			return err
		}
		if closing != json.Delim('}') {
			return fmt.Errorf("JSON object is not terminated")
		}
	case '[':
		for decoder.More() {
			valueToken, err := decoder.Token()
			if err != nil {
				return err
			}
			if err := consumeUniqueJSONValue(decoder, valueToken); err != nil {
				return err
			}
		}
		closing, err := decoder.Token()
		if err != nil {
			return err
		}
		if closing != json.Delim(']') {
			return fmt.Errorf("JSON array is not terminated")
		}
	default:
		return fmt.Errorf("unexpected JSON delimiter %q", delimiter)
	}
	return nil
}
