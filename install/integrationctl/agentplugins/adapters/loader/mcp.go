package loader

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/777genius/plugin-kit-ai/install/integrationctl/agentplugins/domain"
)

func (loader Loader) loadMCP(path string) (domain.MCPComponent, []domain.Diagnostic) {
	body, exists, err := readRegularFile(path)
	if !exists {
		return domain.MCPComponent{}, nil
	}
	component := domain.MCPComponent{
		Present:       true,
		Raw:           append(json.RawMessage(nil), body...),
		Servers:       map[string]domain.MCPServer{},
		InvalidServer: map[string]domain.Diagnostic{},
	}
	if err != nil {
		return component, []domain.Diagnostic{mcpDiagnostic("mcp_read_failed", "read root mcp.json", err)}
	}
	rawFields, decoded, err := decodeJSONObject(body)
	if err != nil {
		return component, []domain.Diagnostic{mcpDiagnostic("mcp_malformed", "parse root mcp.json", err)}
	}
	schemaURI, ok := decoded["$schema"].(string)
	if !ok || strings.TrimSpace(schemaURI) == "" {
		return component, []domain.Diagnostic{mcpDiagnostic("mcp_schema_missing", "mcp.json requires a string $schema", nil)}
	}
	component.SchemaURI = schemaURI
	if schemaURI != domain.MCPSchemaV1 || !loader.Registry.Supports(schemaURI) {
		return component, []domain.Diagnostic{mcpDiagnostic("mcp_schema_unsupported", fmt.Sprintf("unsupported Agent Plugins MCP schema %q", schemaURI), nil)}
	}
	serversRaw, ok := rawFields["mcpServers"]
	if !ok {
		return component, []domain.Diagnostic{mcpDiagnostic("mcp_servers_missing", "mcp.json requires mcpServers", nil)}
	}
	var serverDocuments map[string]json.RawMessage
	if err := decodeJSON(serversRaw, &serverDocuments); err != nil || serverDocuments == nil {
		return component, []domain.Diagnostic{mcpDiagnostic("mcp_servers_invalid", "mcpServers must be a JSON object", err)}
	}
	topLevel := make(map[string]any, len(decoded))
	for key, value := range decoded {
		topLevel[key] = value
	}
	topLevel["mcpServers"] = map[string]any{}
	if err := loader.Registry.Validate(schemaURI, topLevel); err != nil {
		return component, []domain.Diagnostic{mcpDiagnostic("mcp_schema_invalid", "mcp.json top-level document does not conform to Agent Plugins 1.0", err)}
	}

	component.Enabled = true
	names := make([]string, 0, len(serverDocuments))
	for name := range serverDocuments {
		names = append(names, name)
	}
	sort.Strings(names)
	var diagnostics []domain.Diagnostic
	for _, name := range names {
		raw := serverDocuments[name]
		var decodedServer map[string]any
		decodeErr := decodeJSON(raw, &decodedServer)
		if decodeErr == nil && decodedServer == nil {
			decodeErr = fmt.Errorf("server config must be a JSON object")
		}
		if decodeErr == nil {
			document := map[string]any{
				"$schema":    schemaURI,
				"mcpServers": map[string]any{name: decodedServer},
			}
			decodeErr = loader.Registry.Validate(schemaURI, document)
		}
		if decodeErr != nil {
			diagnostic := domain.Diagnostic{
				Severity: domain.SeverityError,
				Boundary: domain.BoundaryMCPServer,
				Code:     "mcp_server_invalid",
				Path:     "mcp.json",
				Item:     name,
				Message:  fmt.Sprintf("MCP server %q was skipped because its configuration is invalid: %v", name, decodeErr),
			}
			component.InvalidServer[name] = diagnostic
			diagnostics = append(diagnostics, diagnostic)
			continue
		}
		typeName, _ := decodedServer["type"].(string)
		component.Servers[name] = domain.MCPServer{
			Name:    name,
			Type:    typeName,
			Raw:     append(json.RawMessage(nil), raw...),
			Decoded: decodedServer,
		}
	}
	return component, diagnostics
}

func mcpDiagnostic(code, message string, cause error) domain.Diagnostic {
	if cause != nil {
		message += ": " + cause.Error()
	}
	return domain.Diagnostic{
		Severity: domain.SeverityError,
		Boundary: domain.BoundaryMCP,
		Code:     code,
		Path:     "mcp.json",
		Message:  message,
	}
}
