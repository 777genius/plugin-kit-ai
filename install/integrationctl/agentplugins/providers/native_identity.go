package providers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/777genius/plugin-kit-ai/install/integrationctl/agentplugins/domain"
	"github.com/777genius/plugin-kit-ai/install/integrationctl/agentplugins/ports"
	legacyports "github.com/777genius/plugin-kit-ai/install/integrationctl/ports"
	"github.com/pelletier/go-toml/v2"
)

type packageVerifier interface {
	Verify(context.Context, string, string) error
}

// NativeIdentityObserver combines package ownership with the client's native
// registry. A manager-owned path is not proof that the logical identity is
// free: another prepared marketplace, local plugin, installed CLI entry, or
// native config entry can already claim the same manifest identity.
type NativeIdentityObserver struct {
	Stager packageVerifier
	Runner CommandRunner
}

type registryFinding uint8

const (
	registryClear registryFinding = iota
	registryExpected
	registryCollision
	registryIndeterminate
)

func (observer NativeIdentityObserver) ObserveNativeIdentity(ctx context.Context, client domain.DetectedClient, plan domain.DeliveryPlan, managed *domain.ClientBinding) (domain.NativeIdentityObservation, error) {
	if err := ctx.Err(); err != nil {
		return domain.NativeIdentityObservation{}, err
	}
	name := strings.TrimSpace(plan.DeclaredName)
	if name == "" {
		return domain.NativeIdentityObservation{State: domain.NativeIdentityIndeterminate}, nil
	}

	prepared, err := inspectPreparedRegistry(plan, name, managed != nil)
	if err != nil {
		return domain.NativeIdentityObservation{State: domain.NativeIdentityIndeterminate}, err
	}
	native, err := observer.inspectNativeRegistry(ctx, client, plan, managed)
	if err != nil {
		return domain.NativeIdentityObservation{State: domain.NativeIdentityIndeterminate, NativeDiscoveryState: domain.NativeIdentityIndeterminate}, err
	}
	discoveryState := domain.NativeIdentityAbsent
	discoveryReconciled := false
	switch native {
	case registryExpected:
		discoveryState = domain.NativeIdentityManaged
		discoveryReconciled = managed != nil
	case registryCollision:
		discoveryState = domain.NativeIdentityUnmanaged
	case registryIndeterminate:
		discoveryState = domain.NativeIdentityIndeterminate
	}
	for _, finding := range []registryFinding{prepared, native} {
		switch finding {
		case registryCollision:
			return domain.NativeIdentityObservation{State: domain.NativeIdentityUnmanaged, NativeDiscoveryState: discoveryState}, nil
		case registryIndeterminate:
			return domain.NativeIdentityObservation{State: domain.NativeIdentityIndeterminate, NativeDiscoveryState: domain.NativeIdentityIndeterminate}, nil
		case registryExpected:
			if managed == nil {
				return domain.NativeIdentityObservation{State: domain.NativeIdentityUnmanaged, NativeDiscoveryState: domain.NativeIdentityUnmanaged}, nil
			}
		}
	}

	_, statErr := os.Lstat(plan.ActivePath)
	if os.IsNotExist(statErr) {
		return domain.NativeIdentityObservation{State: domain.NativeIdentityAbsent, NativeDiscoveryReconciled: discoveryReconciled, NativeDiscoveryState: discoveryState}, nil
	}
	if statErr != nil {
		return domain.NativeIdentityObservation{State: domain.NativeIdentityIndeterminate, NativeDiscoveryReconciled: discoveryReconciled, NativeDiscoveryState: discoveryState}, statErr
	}
	if managed == nil {
		return domain.NativeIdentityObservation{State: domain.NativeIdentityUnmanaged, NativeDiscoveryReconciled: discoveryReconciled, NativeDiscoveryState: discoveryState}, nil
	}
	expected := managedPackageDigest(*managed)
	if expected == "" || observer.Stager == nil {
		return domain.NativeIdentityObservation{State: domain.NativeIdentityIndeterminate, NativeDiscoveryReconciled: discoveryReconciled, NativeDiscoveryState: discoveryState}, nil
	}
	if err := observer.Stager.Verify(ctx, plan.ActivePath, expected); err != nil {
		var verification *ports.VerificationError
		if errors.As(err, &verification) && verification.Kind == ports.VerificationDigestMismatch {
			return domain.NativeIdentityObservation{State: domain.NativeIdentityIndeterminate, Digest: verification.ActualDigest,
				NativeDiscoveryReconciled: discoveryReconciled, NativeDiscoveryState: discoveryState}, nil
		}
		return domain.NativeIdentityObservation{State: domain.NativeIdentityIndeterminate,
			NativeDiscoveryReconciled: discoveryReconciled, NativeDiscoveryState: discoveryState}, err
	}
	return domain.NativeIdentityObservation{State: domain.NativeIdentityManaged, Digest: expected,
		ReceiptReconciled: true, NativeDiscoveryReconciled: discoveryReconciled,
		NativeDiscoveryState: discoveryState}, nil
}

func (observer NativeIdentityObserver) inspectNativeRegistry(ctx context.Context, client domain.DetectedClient, plan domain.DeliveryPlan, managed *domain.ClientBinding) (registryFinding, error) {
	switch client.ClientID {
	case domain.ClientCodex:
		if strings.TrimSpace(plan.NativeRegistryExecutable) != "" {
			return observer.inspectCodexCLI(ctx, plan, managed)
		}
		return inspectCodexFiles(plan, managed)
	case domain.ClientChatGPT:
		// ChatGPT's installed-plugin registry is remote and this adapter has no
		// authenticated read-only API. A signed explicit app binding authorizes
		// only preparation of a new local package, while an existing ownership
		// receipt authorizes replacement of that owned local package. Neither is
		// treated as proof of remote activation or registry availability.
		if plan.LocalPreparationAuthorized || managed != nil {
			return registryClear, nil
		}
		return registryIndeterminate, nil
	case domain.ClientCursor:
		// TargetRoot is Cursor's authoritative plugins/local registry and was
		// inspected as the prepared/native boundary above.
		return registryClear, nil
	case domain.ClientCopilot, domain.ClientVSCode:
		if strings.TrimSpace(plan.NativeRegistryExecutable) != "" {
			return observer.inspectCopilotCLI(ctx, plan, managed)
		}
		root := strings.TrimSpace(plan.NativeRegistryRoot)
		if root == "" {
			return registryIndeterminate, nil
		}
		if _, err := os.Lstat(root); os.IsNotExist(err) {
			return registryClear, nil
		} else if err != nil {
			return registryIndeterminate, err
		}
		// Copilot and VS Code share a backend, but its on-disk registry contract
		// is not implemented by these adapters. An existing config root without
		// the CLI therefore cannot prove absence.
		return registryIndeterminate, nil
	case domain.ClientKiro:
		return inspectKiroRegistry(plan, managed)
	default:
		return registryIndeterminate, nil
	}
}

func (observer NativeIdentityObserver) inspectCodexCLI(ctx context.Context, plan domain.DeliveryPlan, managed *domain.ClientBinding) (registryFinding, error) {
	if observer.Runner == nil {
		return registryIndeterminate, nil
	}
	result, err := observer.Runner.Run(ctx, legacyports.Command{Argv: []string{plan.NativeRegistryExecutable, "plugin", "list", "--json"}})
	if err != nil {
		return registryIndeterminate, err
	}
	if result.ExitCode != 0 {
		return registryIndeterminate, fmt.Errorf("Codex plugin registry command failed with exit code %d", result.ExitCode)
	}
	return codexRegistryFinding(result.Stdout, plan.DeclaredName, managedMarketplaceName(plan.PhysicalArtifactID), managed != nil), nil
}

func codexRegistryFinding(body []byte, name, expectedMarketplace string, owned bool) registryFinding {
	decoder := json.NewDecoder(strings.NewReader(string(body)))
	decoder.UseNumber()
	value, err := decodeUniqueJSONValue(decoder)
	if err != nil {
		return registryIndeterminate
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		return registryIndeterminate
	}
	document, ok := value.(map[string]any)
	if !ok {
		return registryIndeterminate
	}
	entries, ok := document["installed"].([]any)
	if !ok {
		return registryIndeterminate
	}
	finding := registryClear
	seen := map[string]bool{}
	for _, raw := range entries {
		entry, ok := raw.(map[string]any)
		if !ok {
			return registryIndeterminate
		}
		entryName, nameOK := entry["name"].(string)
		marketplace, marketplaceOK := entry["marketplaceName"].(string)
		pluginID, idOK := entry["pluginId"].(string)
		_, installedOK := entry["installed"].(bool)
		_, enabledOK := entry["enabled"].(bool)
		if !nameOK || !marketplaceOK || !idOK || !installedOK || !enabledOK || entryName == "" || marketplace == "" || pluginID != entryName+"@"+marketplace || seen[pluginID] {
			return registryIndeterminate
		}
		seen[pluginID] = true
		if entryName != name {
			continue
		}
		if marketplace == expectedMarketplace {
			if !owned {
				return registryCollision
			}
			finding = registryExpected
		}
		// A different non-empty marketplace is positive namespace evidence and
		// can coexist with the managed marketplace.
	}
	return finding
}

func (observer NativeIdentityObserver) inspectCopilotCLI(ctx context.Context, plan domain.DeliveryPlan, managed *domain.ClientBinding) (registryFinding, error) {
	if observer.Runner == nil {
		return registryIndeterminate, nil
	}
	result, err := observer.Runner.Run(ctx, legacyports.Command{Argv: []string{plan.NativeRegistryExecutable, "plugin", "list"}})
	if err != nil {
		return registryIndeterminate, err
	}
	if result.ExitCode != 0 {
		return registryIndeterminate, fmt.Errorf("Copilot plugin registry command failed with exit code %d", result.ExitCode)
	}
	return copilotRegistryFinding(result.Stdout, plan.DeclaredName, managedMarketplaceName(plan.PhysicalArtifactID), managed != nil), nil
}

func copilotRegistryFinding(stdout []byte, name, expectedMarketplace string, owned bool) registryFinding {
	normalized := strings.ReplaceAll(string(stdout), "\r\n", "\n")
	if strings.TrimSpace(normalized) == "No plugins installed.\n\nUse 'copilot plugin install <source>' to install a plugin." {
		return registryClear
	}
	if strings.TrimSpace(normalized) == "No plugins installed." {
		return registryClear
	}
	inInstalled := false
	recognized := false
	finding := registryClear
	seen := map[string]bool{}
	for _, line := range strings.Split(normalized, "\n") {
		if strings.TrimSpace(line) == "Installed plugins:" {
			if inInstalled || recognized {
				return registryIndeterminate
			}
			inInstalled, recognized = true, true
			continue
		}
		if !inInstalled {
			continue
		}
		if line != "" && line[0] != ' ' && line[0] != '\t' {
			inInstalled = false
			continue
		}
		match := copilotInstalledEntry.FindStringSubmatch(line)
		if len(match) == 2 {
			identity := match[1]
			if seen[identity] {
				return registryIndeterminate
			}
			seen[identity] = true
			parts := strings.Split(identity, "@")
			if len(parts) != 2 {
				return registryIndeterminate
			}
			if parts[0] == name && parts[1] == expectedMarketplace {
				if !owned {
					return registryCollision
				}
				finding = registryExpected
			}
			continue
		}
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || trimmed == "No plugins installed." || trimmed == "No plugins installed" {
			continue
		}
		return registryIndeterminate
	}
	if !recognized {
		return registryIndeterminate
	}
	return finding
}

func inspectCodexFiles(plan domain.DeliveryPlan, managed *domain.ClientBinding) (registryFinding, error) {
	root := strings.TrimSpace(plan.NativeRegistryRoot)
	if root == "" {
		return registryIndeterminate, nil
	}
	if _, err := os.Lstat(root); os.IsNotExist(err) {
		return registryClear, nil
	} else if err != nil {
		return registryIndeterminate, err
	}
	expectedMarketplace := managedMarketplaceName(plan.PhysicalArtifactID)
	finding := registryClear
	configPath := filepath.Join(root, "config.toml")
	body, err := os.ReadFile(configPath)
	if err == nil {
		var config struct {
			Plugins map[string]map[string]any `toml:"plugins"`
		}
		if err := toml.Unmarshal(body, &config); err != nil {
			return registryIndeterminate, err
		}
		for identity := range config.Plugins {
			parts := strings.Split(identity, "@")
			if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
				return registryIndeterminate, nil
			}
			if parts[0] == plan.DeclaredName && parts[1] == expectedMarketplace {
				if managed == nil {
					return registryCollision, nil
				}
				finding = registryExpected
			}
		}
	} else if !os.IsNotExist(err) {
		return registryIndeterminate, err
	}
	cacheFinding, err := inspectCodexCache(filepath.Join(root, "plugins", "cache"), plan.DeclaredName, expectedMarketplace, managed != nil)
	if err != nil {
		return registryIndeterminate, err
	}
	if cacheFinding == registryCollision || cacheFinding == registryIndeterminate {
		return cacheFinding, nil
	}
	if cacheFinding == registryExpected {
		finding = registryExpected
	}
	return finding, nil
}

func inspectCodexCache(root, name, expectedMarketplace string, owned bool) (registryFinding, error) {
	markets, err := os.ReadDir(root)
	if os.IsNotExist(err) {
		return registryClear, nil
	}
	if err != nil {
		return registryIndeterminate, err
	}
	finding := registryClear
	for _, market := range markets {
		if market.Type()&os.ModeSymlink != 0 || !market.IsDir() {
			return registryIndeterminate, nil
		}
		plugins, err := os.ReadDir(filepath.Join(root, market.Name()))
		if err != nil {
			return registryIndeterminate, err
		}
		for _, plugin := range plugins {
			if plugin.Type()&os.ModeSymlink != 0 || !plugin.IsDir() {
				return registryIndeterminate, nil
			}
			manifest := filepath.Join(root, market.Name(), plugin.Name(), "local", ".codex-plugin", "plugin.json")
			manifestName, err := readJSONManifestName(manifest)
			if os.IsNotExist(err) {
				return registryIndeterminate, nil
			}
			if err != nil {
				return registryIndeterminate, err
			}
			if manifestName != name {
				continue
			}
			if market.Name() == expectedMarketplace {
				if !owned {
					return registryCollision, nil
				}
				finding = registryExpected
			}
		}
	}
	return finding, nil
}

func inspectKiroRegistry(plan domain.DeliveryPlan, managed *domain.ClientBinding) (registryFinding, error) {
	root := strings.TrimSpace(plan.NativeRegistryRoot)
	if root == "" {
		return registryIndeterminate, nil
	}
	// Custom Powers without MCP are activated manually and the adapter has no
	// documented read-only registry contract for their installed identities.
	// Their local prepared-package boundary was inspected above, so an
	// absent/owned local package may be prepared safely. Mixed Powers still have
	// to inspect every supported MCP name below; a manual skill surface must not
	// hide a positive collision in Kiro's authoritative global MCP registry.
	if !hasSupportedMCP(plan.Components) {
		return registryClear, nil
	}
	if _, err := os.Lstat(root); os.IsNotExist(err) {
		return registryClear, nil
	} else if err != nil {
		return registryIndeterminate, err
	}
	finding := registryClear
	mcpPath := filepath.Join(root, "settings", "mcp.json")
	body, err := os.ReadFile(mcpPath)
	if os.IsNotExist(err) {
		return finding, nil
	}
	if err != nil {
		return registryIndeterminate, err
	}
	value, err := decodeStrictJSONObject(body)
	if err != nil {
		return registryIndeterminate, err
	}
	servers, ok := value["mcpServers"].(map[string]any)
	if !ok {
		return registryIndeterminate, nil
	}
	for _, component := range plan.Components {
		if component.Kind != domain.ComponentMCPServer || component.Support == domain.SupportUnsupported {
			continue
		}
		if _, exists := servers[component.Name]; exists {
			if managed == nil {
				return registryCollision, nil
			}
			finding = registryExpected
		}
	}
	return finding, nil
}

func hasSupportedMCP(components []domain.ComponentDecision) bool {
	for _, component := range components {
		if component.Kind == domain.ComponentMCPServer && component.Support != domain.SupportUnsupported {
			return true
		}
	}
	return false
}

func inspectPreparedRegistry(plan domain.DeliveryPlan, name string, owned bool) (registryFinding, error) {
	return inspectUnqualifiedPluginRoot(plan.TargetRoot, name, plan.ActivePath, owned)
}

func inspectUnqualifiedPluginRoot(root, name, activePath string, owned bool) (registryFinding, error) {
	if strings.TrimSpace(root) == "" {
		return registryIndeterminate, nil
	}
	entries, err := os.ReadDir(root)
	if os.IsNotExist(err) {
		return registryClear, nil
	}
	if err != nil {
		return registryIndeterminate, err
	}
	finding := registryClear
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".agentplugins-staging-") {
			continue
		}
		if entry.Type()&os.ModeSymlink != 0 || !entry.IsDir() {
			return registryIndeterminate, nil
		}
		path := filepath.Join(root, entry.Name())
		manifestName, qualified, namespace, err := nativeManifestIdentity(path)
		if err != nil {
			return registryIndeterminate, err
		}
		if manifestName != name {
			continue
		}
		if qualified && namespace != "" && namespace != managedMarketplaceName(filepath.Base(activePath)) {
			continue
		}
		if activePath != "" && sameCleanPath(path, activePath) && owned {
			finding = registryExpected
			continue
		}
		return registryCollision, nil
	}
	return finding, nil
}

func nativeManifestIdentity(root string) (name string, qualified bool, namespace string, err error) {
	for _, marketplace := range []string{filepath.Join(root, ".agents", "plugins", "marketplace.json"), filepath.Join(root, ".github", "plugin", "marketplace.json")} {
		body, readErr := os.ReadFile(marketplace)
		if readErr == nil {
			value, decodeErr := decodeStrictJSONObject(body)
			if decodeErr != nil {
				return "", false, "", decodeErr
			}
			namespace, _ = value["name"].(string)
			plugins, ok := value["plugins"].([]any)
			if namespace == "" || !ok || len(plugins) != 1 {
				return "", false, "", fmt.Errorf("invalid prepared marketplace identity")
			}
			plugin, ok := plugins[0].(map[string]any)
			if !ok {
				return "", false, "", fmt.Errorf("invalid prepared marketplace plugin identity")
			}
			name, ok = plugin["name"].(string)
			if !ok || name == "" {
				return "", false, "", fmt.Errorf("invalid prepared marketplace plugin name")
			}
			return name, true, namespace, nil
		}
		if !os.IsNotExist(readErr) {
			return "", false, "", readErr
		}
	}
	for _, manifest := range []string{filepath.Join(root, "plugin.json"), filepath.Join(root, ".cursor-plugin", "plugin.json"), filepath.Join(root, ".codex-plugin", "plugin.json")} {
		name, readErr := readJSONManifestName(manifest)
		if readErr == nil {
			return name, false, "", nil
		}
		if !os.IsNotExist(readErr) {
			return "", false, "", readErr
		}
	}
	return "", false, "", fmt.Errorf("native package has no recognized authoritative manifest")
}

func readJSONManifestName(path string) (string, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	value, err := decodeStrictJSONObject(body)
	if err != nil {
		return "", err
	}
	name, ok := value["name"].(string)
	if !ok || strings.TrimSpace(name) == "" {
		return "", fmt.Errorf("manifest %s has no valid name", path)
	}
	return name, nil
}

func decodeStrictJSONObject(body []byte) (map[string]any, error) {
	decoder := json.NewDecoder(strings.NewReader(string(body)))
	decoder.UseNumber()
	value, err := decodeUniqueJSONValue(decoder)
	if err != nil {
		return nil, err
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("JSON document has trailing data")
	}
	object, ok := value.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("JSON document must be an object")
	}
	return object, nil
}

func sameCleanPath(left, right string) bool {
	leftAbsolute, leftErr := filepath.Abs(left)
	rightAbsolute, rightErr := filepath.Abs(right)
	return leftErr == nil && rightErr == nil && filepath.Clean(leftAbsolute) == filepath.Clean(rightAbsolute)
}

func managedPackageDigest(client domain.ClientBinding) string {
	for _, object := range client.NativeObjects {
		if object.Kind == "managed_package_directory" {
			return object.ManagedDigest
		}
	}
	return ""
}
