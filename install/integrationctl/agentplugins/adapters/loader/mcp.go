package loader

import (
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/777genius/plugin-kit-ai/install/integrationctl/adapters/pathpolicy"
	"github.com/777genius/plugin-kit-ai/install/integrationctl/agentplugins/domain"
)

func (loader Loader) loadMCP(filename, pluginSchema string) (domain.MCPComponent, []domain.Diagnostic) {
	body, exists, err := readRegularFile(filename)
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
	if schemaVersion(schemaURI) != schemaVersion(pluginSchema) {
		return component, []domain.Diagnostic{mcpDiagnostic("mcp_schema_mismatch", fmt.Sprintf("mcp.json schema %q does not match plugin.json schema version %q", schemaURI, pluginSchema), nil)}
	}
	if schemaURI != domain.MCPSchemaV1 || !loader.Registry.Supports(schemaURI) {
		return component, []domain.Diagnostic{mcpDiagnostic("mcp_schema_unsupported", fmt.Sprintf("unsupported Agent Plugins MCP schema %q", schemaURI), nil)}
	}
	serversRaw, ok := rawFields["mcpServers"]
	if !ok {
		return component, []domain.Diagnostic{mcpDiagnostic("mcp_servers_missing", "mcp.json requires mcpServers", nil)}
	}
	var serverDocuments map[string]json.RawMessage
	if err := decodeRawJSONObject(serversRaw, &serverDocuments); err != nil || serverDocuments == nil {
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
		var requirement *domain.StdioRequirement
		if decodeErr == nil {
			typeName, _ := decodedServer["type"].(string)
			switch typeName {
			case "stdio":
				requirement, decodeErr = validateStdioServer(filepath.Dir(filename), decodedServer)
			case "streamable-http", "sse":
				decodeErr = validateRemoteServer(decodedServer)
			}
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
			Name:             name,
			Type:             typeName,
			Raw:              append(json.RawMessage(nil), raw...),
			Decoded:          decodedServer,
			StdioRequirement: requirement,
		}
	}
	return component, diagnostics
}

func schemaVersion(uri string) string {
	const marker = "/schemas/"
	index := strings.Index(uri, marker)
	if index < 0 {
		return ""
	}
	remainder := uri[index+len(marker):]
	if slash := strings.IndexByte(remainder, '/'); slash >= 0 {
		return remainder[:slash]
	}
	return ""
}

func validateStdioServer(root string, config map[string]any) (*domain.StdioRequirement, error) {
	command, _ := config["command"].(string)
	if command == "" {
		return nil, fmt.Errorf("stdio command must be a non-empty executable token")
	}
	requirement := &domain.StdioRequirement{Command: command, Kind: domain.ExecutableBare}
	if strings.ContainsAny(command, `/\\`) {
		relative, err := bundledCommandPath(command)
		if err != nil {
			return nil, err
		}
		candidate := filepath.Join(root, filepath.FromSlash(relative))
		if err := validateExistingCommandAncestor(root, candidate, command); err != nil {
			return nil, err
		}
		requirement.Kind, requirement.BundledRelativePath = domain.ExecutableBundled, relative
	}
	values := []string{}
	if args, ok := config["args"].([]any); ok {
		for _, value := range args {
			if text, ok := value.(string); ok {
				values = append(values, text)
			}
		}
	}
	if env, ok := config["env"].(map[string]any); ok {
		if _, reserved := env["PLUGIN_ROOT"]; reserved {
			return nil, fmt.Errorf("stdio server must not define reserved environment variable PLUGIN_ROOT")
		}
		if _, reserved := env["PLUGIN_DATA"]; reserved {
			return nil, fmt.Errorf("stdio server must not define reserved environment variable PLUGIN_DATA")
		}
		for _, value := range env {
			if text, ok := value.(string); ok {
				values = append(values, text)
			}
		}
	}
	if cwd, ok := config["cwd"].(string); ok {
		if err := validateCWD(cwd); err != nil {
			return nil, err
		}
		values = append(values, cwd)
	}
	for _, value := range values {
		if strings.Contains(value, "${PLUGIN_ROOT}") {
			requirement.UsesPluginRoot = true
		}
		if strings.Contains(value, "${PLUGIN_DATA}") {
			requirement.UsesPluginData = true
		}
	}
	return requirement, nil
}

func validateExistingCommandAncestor(root, candidate, command string) error {
	current := candidate
	for {
		if _, err := os.Lstat(current); err == nil {
			resolved, resolveErr := filepath.EvalSymlinks(current)
			if resolveErr != nil {
				return fmt.Errorf("resolve bundled stdio command %q: %w", command, resolveErr)
			}
			contained, relativeErr := filepath.Rel(root, resolved)
			if relativeErr != nil || contained == ".." || filepath.IsAbs(contained) || strings.HasPrefix(contained, ".."+string(filepath.Separator)) {
				return fmt.Errorf("bundled stdio command %q resolves outside the plugin root", command)
			}
			return nil
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("inspect bundled stdio command %q: %w", command, err)
		}

		parent := filepath.Dir(current)
		if parent == current {
			return fmt.Errorf("bundled stdio command %q has no resolvable plugin-root ancestor", command)
		}
		current = parent
	}
}

func validateRemoteServer(config map[string]any) error {
	rawURL, _ := config["url"].(string)
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" || parsed.Opaque != "" {
		return fmt.Errorf("remote MCP URL must be an absolute HTTP or HTTPS URL")
	}
	scheme := strings.ToLower(parsed.Scheme)
	if scheme != "http" && scheme != "https" {
		return fmt.Errorf("remote MCP URL must use HTTP or HTTPS")
	}
	if parsed.User != nil || strings.Contains(rawURL, "#") {
		return fmt.Errorf("remote MCP URL must not contain user information or a fragment")
	}
	host := parsed.Hostname()
	if host == "" {
		return fmt.Errorf("remote MCP URL requires a host")
	}
	if scheme == "http" && !strings.EqualFold(host, "localhost") {
		address := net.ParseIP(host)
		if address == nil || !address.IsLoopback() {
			return fmt.Errorf("non-loopback remote MCP URLs must use HTTPS")
		}
	}
	headers, _ := config["headers"].(map[string]any)
	seen := make(map[string]struct{}, len(headers))
	for name, rawValue := range headers {
		value, _ := rawValue.(string)
		if !validHTTPHeaderName(name) || !validHTTPHeaderValue(value) {
			return fmt.Errorf("remote MCP header %q is not a valid HTTP field", name)
		}
		folded := strings.ToLower(name)
		if _, duplicate := seen[folded]; duplicate {
			return fmt.Errorf("remote MCP header %q is duplicated with different casing", name)
		}
		seen[folded] = struct{}{}
	}
	return nil
}

func validHTTPHeaderName(value string) bool {
	if value == "" {
		return false
	}
	for index := 0; index < len(value); index++ {
		character := value[index]
		if !((character >= 'a' && character <= 'z') ||
			(character >= 'A' && character <= 'Z') ||
			(character >= '0' && character <= '9') ||
			strings.ContainsRune("!#$%&'*+-.^_`|~", rune(character))) {
			return false
		}
	}
	return true
}

func validHTTPHeaderValue(value string) bool {
	for index := 0; index < len(value); index++ {
		character := value[index]
		if (character < 0x20 && character != '\t') || character == 0x7f {
			return false
		}
	}
	return true
}

func bundledCommandPath(command string) (string, error) {
	if !strings.HasPrefix(command, "./") || strings.Contains(command, `\\`) {
		return "", fmt.Errorf("bundled stdio command %q must be a ./-prefixed plugin-relative path", command)
	}
	relative := strings.TrimPrefix(command, "./")
	if relative == "" || path.Clean(relative) != relative || strings.HasPrefix(relative, "../") {
		return "", fmt.Errorf("bundled stdio command %q escapes the plugin root", command)
	}
	for _, segment := range strings.Split(relative, "/") {
		if err := pathpolicy.ValidatePortablePathSegment(segment); err != nil {
			return "", fmt.Errorf("bundled stdio command %q contains a non-portable path segment: %w", command, err)
		}
	}
	return relative, nil
}

func validateCWD(value string) error {
	var suffix string
	switch {
	case strings.HasPrefix(value, "./"):
		suffix = strings.TrimPrefix(value, "./")
	case value == "${PLUGIN_ROOT}" || value == "${PLUGIN_DATA}":
		return nil
	case strings.HasPrefix(value, "${PLUGIN_ROOT}/"):
		suffix = strings.TrimPrefix(value, "${PLUGIN_ROOT}/")
	case strings.HasPrefix(value, "${PLUGIN_DATA}/"):
		suffix = strings.TrimPrefix(value, "${PLUGIN_DATA}/")
	default:
		return fmt.Errorf("stdio cwd must be ./-, PLUGIN_ROOT-, or PLUGIN_DATA-rooted")
	}
	if suffix == "" || path.IsAbs(suffix) || strings.Contains(suffix, `\\`) || path.Clean(suffix) != suffix || strings.HasPrefix(suffix, "../") {
		return fmt.Errorf("stdio cwd escapes its declared root")
	}
	for _, segment := range strings.Split(suffix, "/") {
		if err := pathpolicy.ValidatePortablePathSegment(segment); err != nil {
			return fmt.Errorf("stdio cwd contains a non-portable path segment: %w", err)
		}
	}
	return nil
}

func (loader Loader) loadOpenAIMCP(path string, declared bool) (domain.MCPComponent, []domain.Diagnostic) {
	body, exists, err := readRegularFile(path)
	component := domain.MCPComponent{
		Present: exists, Raw: append(json.RawMessage(nil), body...),
		Servers: map[string]domain.MCPServer{}, InvalidServer: map[string]domain.Diagnostic{},
	}
	if !exists {
		if declared {
			return component, []domain.Diagnostic{openAIMCPDiagnostic("mcp_manifest_missing", "official manifest declares .mcp.json but the file is missing", nil)}
		}
		return component, nil
	}
	if err != nil {
		return component, []domain.Diagnostic{openAIMCPDiagnostic("mcp_read_failed", "read root .mcp.json", err)}
	}
	if !declared {
		return component, []domain.Diagnostic{{
			Severity: domain.SeverityWarning, Boundary: domain.BoundaryMCP,
			Code: "undeclared_mcp_manifest_ignored", Path: ".mcp.json",
			Message: "root .mcp.json is ignored because .codex-plugin/plugin.json does not declare mcpServers",
		}}
	}
	rawFields, _, err := decodeJSONObject(body)
	if err != nil {
		return component, []domain.Diagnostic{openAIMCPDiagnostic("mcp_malformed", "parse root .mcp.json", err)}
	}
	serverDocuments := rawFields
	for _, wrapper := range []string{"mcp_servers", "mcpServers"} {
		if wrapped, ok := rawFields[wrapper]; ok {
			if len(rawFields) != 1 {
				return component, []domain.Diagnostic{openAIMCPDiagnostic("mcp_servers_invalid", "wrapped .mcp.json cannot contain sibling fields", nil)}
			}
			if err := decodeRawJSONObject(wrapped, &serverDocuments); err != nil || serverDocuments == nil {
				return component, []domain.Diagnostic{openAIMCPDiagnostic("mcp_servers_invalid", wrapper+" must be an object", err)}
			}
			break
		}
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
		var config map[string]any
		if err := decodeJSON(raw, &config); err != nil || config == nil {
			diagnostic := invalidOfficialMCP(name, "server config must be an object")
			component.InvalidServer[name] = diagnostic
			diagnostics = append(diagnostics, diagnostic)
			continue
		}
		typeName, valid := officialMCPType(config)
		if !valid {
			diagnostic := invalidOfficialMCP(name, "server requires a command for stdio or a URL for http/sse")
			component.InvalidServer[name] = diagnostic
			diagnostics = append(diagnostics, diagnostic)
			continue
		}
		component.Servers[name] = domain.MCPServer{Name: name, Type: typeName, Raw: append(json.RawMessage(nil), raw...), Decoded: config}
	}
	return component, diagnostics
}

func openAIMCPDiagnostic(code, message string, cause error) domain.Diagnostic {
	if cause != nil {
		message += ": " + cause.Error()
	}
	return domain.Diagnostic{Severity: domain.SeverityError, Boundary: domain.BoundaryMCP, Code: code, Path: ".mcp.json", Message: message}
}

func officialMCPType(config map[string]any) (string, bool) {
	typeName, _ := config["type"].(string)
	switch strings.ToLower(strings.TrimSpace(typeName)) {
	case "stdio":
		_, ok := config["command"].(string)
		return "stdio", ok
	case "http", "streamable-http":
		_, ok := config["url"].(string)
		return "streamable-http", ok
	case "sse":
		_, ok := config["url"].(string)
		return "sse", ok
	case "":
		if _, ok := config["command"].(string); ok {
			return "stdio", true
		}
		if _, ok := config["url"].(string); ok {
			return "streamable-http", true
		}
	}
	return "", false
}

func invalidOfficialMCP(name, message string) domain.Diagnostic {
	return domain.Diagnostic{
		Severity: domain.SeverityError, Boundary: domain.BoundaryMCPServer,
		Code: "mcp_server_invalid", Path: ".mcp.json", Item: name,
		Message: fmt.Sprintf("MCP server %q was skipped because %s", name, message),
	}
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
