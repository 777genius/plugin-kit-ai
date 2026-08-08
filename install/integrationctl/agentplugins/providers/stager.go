package providers

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/777genius/plugin-kit-ai/install/integrationctl/adapters/atomicfile"
	"github.com/777genius/plugin-kit-ai/install/integrationctl/adapters/filetree"
	"github.com/777genius/plugin-kit-ai/install/integrationctl/adapters/pathpolicy"
	"github.com/777genius/plugin-kit-ai/install/integrationctl/agentplugins/domain"
	"github.com/777genius/plugin-kit-ai/install/integrationctl/packagesnapshot"
)

type Stager struct {
	SnapshotBuilder packagesnapshot.Builder
}

func (stager Stager) Discard(ctx context.Context, delivery domain.StagedDelivery) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if filepath.Dir(filepath.Clean(delivery.StagingPath)) != filepath.Clean(delivery.OwnedBase) ||
		!strings.HasPrefix(filepath.Base(delivery.StagingPath), ".agentplugins-staging-") {
		return fmt.Errorf("refuse unsafe staged delivery cleanup")
	}
	return removeStaging(delivery.OwnedBase, delivery.StagingPath)
}

func (stager Stager) Verify(ctx context.Context, root, expectedDigest string) error {
	if strings.TrimSpace(expectedDigest) == "" {
		return fmt.Errorf("expected artifact digest is required")
	}
	if err := rejectExcludedOwnershipMarkers(root); err != nil {
		return err
	}
	artifact, err := stager.SnapshotBuilder.Build(ctx, root)
	if err != nil {
		return err
	}
	digest := artifact.Digest
	if closeErr := artifact.Close(); closeErr != nil {
		return closeErr
	}
	if digest != expectedDigest {
		return fmt.Errorf("artifact digest mismatch")
	}
	return nil
}

func rejectExcludedOwnershipMarkers(root string) error {
	return filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if filepath.Clean(path) == filepath.Clean(root) {
			return nil
		}
		if entry.Name() == ".git" || entry.Name() == ".plugin-kit-ai.lock" {
			return fmt.Errorf("managed artifact contains excluded ownership marker %q", entry.Name())
		}
		return nil
	})
}

func (stager Stager) Stage(
	ctx context.Context,
	envelope domain.PackageEnvelope,
	plan domain.DeliveryPlan,
	operationID string,
	hints domain.CompatibilityHints,
) (delivery domain.StagedDelivery, err error) {
	if err := ctx.Err(); err != nil {
		return domain.StagedDelivery{}, err
	}
	if plan.Status == domain.PlanUnsupported {
		return domain.StagedDelivery{}, fmt.Errorf("cannot stage an unsupported delivery plan")
	}
	if strings.TrimSpace(envelope.SnapshotRoot) == "" {
		return domain.StagedDelivery{}, fmt.Errorf("package snapshot root is required")
	}
	if err := pathpolicy.ValidateLeafID(operationID); err != nil {
		return domain.StagedDelivery{}, fmt.Errorf("invalid staging operation id: %w", err)
	}
	if err := validatePlanPaths(plan); err != nil {
		return domain.StagedDelivery{}, err
	}
	if err := os.MkdirAll(plan.TargetRoot, 0o700); err != nil {
		return domain.StagedDelivery{}, fmt.Errorf("create client target root: %w", err)
	}
	if err := validatePlanPaths(plan); err != nil {
		return domain.StagedDelivery{}, err
	}
	suffix := sha256.Sum256([]byte(operationID))
	stagingPath := filepath.Join(plan.TargetRoot, ".agentplugins-staging-"+hex.EncodeToString(suffix[:8]))
	if err := pathpolicy.RequireContainedChild(plan.TargetRoot, stagingPath); err != nil {
		return domain.StagedDelivery{}, fmt.Errorf("unsafe staging path: %w", err)
	}
	if _, statErr := os.Lstat(stagingPath); statErr == nil {
		return domain.StagedDelivery{}, fmt.Errorf("staging path already exists")
	} else if !os.IsNotExist(statErr) {
		return domain.StagedDelivery{}, fmt.Errorf("inspect staging path: %w", statErr)
	}
	defer func() {
		if err != nil {
			_ = removeStaging(plan.TargetRoot, stagingPath)
		}
	}()
	if err := filetree.CopyDir(envelope.SnapshotRoot, stagingPath); err != nil {
		return domain.StagedDelivery{}, fmt.Errorf("copy package snapshot to staging: %w", err)
	}
	if err := sanitizePackage(stagingPath, envelope, plan); err != nil {
		return domain.StagedDelivery{}, err
	}
	if plan.PackageMode == domain.PackageProjection && plan.ClientID == domain.ClientCodex {
		if err := projectOpenAI(stagingPath, envelope, plan, hints); err != nil {
			return domain.StagedDelivery{}, err
		}
		if err := projectCodexMarketplace(stagingPath, envelope, plan); err != nil {
			return domain.StagedDelivery{}, err
		}
	}
	if plan.ClientID == domain.ClientCopilot || plan.ClientID == domain.ClientVSCode {
		if err := projectCopilotMarketplace(stagingPath, envelope, plan); err != nil {
			return domain.StagedDelivery{}, err
		}
	}
	artifact, err := stager.SnapshotBuilder.Build(ctx, stagingPath)
	if err != nil {
		return domain.StagedDelivery{}, fmt.Errorf("verify staged artifact: %w", err)
	}
	artifactDigest := artifact.Digest
	if closeErr := artifact.Close(); closeErr != nil {
		return domain.StagedDelivery{}, fmt.Errorf("clean staged verification snapshot: %w", closeErr)
	}
	objects := []domain.NativeObjectOwnership{
		{
			ObjectID:        "package:" + string(plan.ClientID) + ":" + plan.PhysicalArtifactID,
			Kind:            "managed_package_directory",
			LogicalName:     envelope.Manifest.Name,
			Path:            plan.ActivePath,
			ManagedDigest:   artifactDigest,
			ProtectionClass: "managed",
		},
	}
	return domain.StagedDelivery{
		ClientID:       plan.ClientID,
		OwnedBase:      plan.TargetRoot,
		ActivePath:     plan.ActivePath,
		StagingPath:    stagingPath,
		ArtifactDigest: artifactDigest,
		NativeObjects:  objects,
	}, nil
}

func validatePlanPaths(plan domain.DeliveryPlan) error {
	if strings.TrimSpace(plan.TargetAnchor) == "" || strings.TrimSpace(plan.TargetRoot) == "" || strings.TrimSpace(plan.ActivePath) == "" {
		return fmt.Errorf("delivery plan target paths are incomplete")
	}
	if err := pathpolicy.RequireContainedChild(plan.TargetAnchor, plan.TargetRoot); err != nil {
		return fmt.Errorf("unsafe delivery target root: %w", err)
	}
	if filepath.Dir(filepath.Clean(plan.ActivePath)) != filepath.Clean(plan.TargetRoot) {
		return fmt.Errorf("delivery active path must be a direct child of target root")
	}
	if err := pathpolicy.RequireContainedChild(plan.TargetRoot, plan.ActivePath); err != nil {
		return fmt.Errorf("unsafe delivery active path: %w", err)
	}
	return nil
}

func sanitizePackage(root string, envelope domain.PackageEnvelope, plan domain.DeliveryPlan) error {
	if err := removeInvalidAndUnsupportedSkills(root, envelope, plan); err != nil {
		return err
	}
	if err := writeSanitizedMCP(root, envelope, plan); err != nil {
		return err
	}
	return writeSanitizedExtensions(root, plan)
}

func removeInvalidAndUnsupportedSkills(root string, envelope domain.PackageEnvelope, plan domain.DeliveryPlan) error {
	names := append([]string(nil), envelope.Inventory.InvalidSkills...)
	for _, component := range plan.Components {
		if component.Kind == domain.ComponentSkill && component.Support == domain.SupportUnsupported {
			names = append(names, component.Name)
		}
	}
	skillsRoot := filepath.Join(root, "skills")
	if envelope.Inventory.InvalidSkillsRoot {
		if err := pathpolicy.RequireExactPath(filepath.Join(root, "skills"), skillsRoot); err != nil {
			return err
		}
		if err := os.RemoveAll(skillsRoot); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove invalid skills root: %w", err)
		}
		return nil
	}
	for _, name := range names {
		candidate := filepath.Join(skillsRoot, name)
		if filepath.Dir(filepath.Clean(candidate)) != filepath.Clean(skillsRoot) {
			return fmt.Errorf("unsafe invalid skill path %q", name)
		}
		if err := pathpolicy.RequireContainedChild(skillsRoot, candidate); err != nil {
			return fmt.Errorf("unsafe invalid skill path %q: %w", name, err)
		}
		if err := os.RemoveAll(candidate); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove skipped skill %q: %w", name, err)
		}
	}
	return nil
}

func writeSanitizedMCP(root string, envelope domain.PackageEnvelope, plan domain.DeliveryPlan) error {
	path := filepath.Join(root, "mcp.json")
	if !envelope.MCP.Present || !envelope.MCP.Enabled {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove disabled mcp.json: %w", err)
		}
		return nil
	}
	supported := supportedMCPNames(plan)
	servers := make(map[string]json.RawMessage, len(supported))
	for _, name := range supported {
		server, ok := envelope.MCP.Servers[name]
		if ok {
			servers[name] = append(json.RawMessage(nil), server.Raw...)
		}
	}
	if len(servers) == 0 {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove unsupported mcp.json: %w", err)
		}
		return nil
	}
	document := struct {
		Schema     string                     `json:"$schema"`
		MCPServers map[string]json.RawMessage `json:"mcpServers"`
	}{Schema: domain.MCPSchemaV1, MCPServers: servers}
	return writeJSON(path, document)
}

func writeSanitizedExtensions(root string, plan domain.DeliveryPlan) error {
	var unsupported []string
	for _, component := range plan.Components {
		if component.Kind == domain.ComponentExtension && component.Support == domain.SupportUnsupported {
			unsupported = append(unsupported, component.Name)
		}
	}
	if len(unsupported) == 0 {
		return nil
	}
	path := filepath.Join(root, "plugin.json")
	body, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read staged plugin.json: %w", err)
	}
	var document map[string]json.RawMessage
	if err := json.Unmarshal(body, &document); err != nil {
		return fmt.Errorf("decode staged plugin.json: %w", err)
	}
	var extensions map[string]json.RawMessage
	if raw := document["extensions"]; len(raw) > 0 {
		if err := json.Unmarshal(raw, &extensions); err != nil {
			return fmt.Errorf("decode staged plugin extensions: %w", err)
		}
	}
	for _, name := range unsupported {
		delete(extensions, name)
	}
	if len(extensions) == 0 {
		delete(document, "extensions")
	} else {
		raw, err := json.Marshal(extensions)
		if err != nil {
			return err
		}
		document["extensions"] = raw
	}
	return writeJSON(path, document)
}

func projectOpenAI(root string, envelope domain.PackageEnvelope, plan domain.DeliveryPlan, hints domain.CompatibilityHints) error {
	manifest := map[string]any{"name": envelope.Manifest.Name}
	copyString := func(key, value string) {
		if strings.TrimSpace(value) != "" {
			manifest[key] = value
		}
	}
	copyString("version", envelope.Manifest.Version)
	copyString("description", envelope.Manifest.Description)
	copyString("homepage", envelope.Manifest.Homepage)
	copyString("repository", envelope.Manifest.Repository)
	copyString("license", envelope.Manifest.License)
	if envelope.Manifest.Author != nil {
		manifest["author"] = envelope.Manifest.Author
	}
	if len(envelope.Manifest.Keywords) > 0 {
		manifest["keywords"] = envelope.Manifest.Keywords
	}
	if hasSupported(plan, domain.ComponentSkill) {
		manifest["skills"] = "./skills/"
	}
	serverNames := supportedMCPNames(plan)
	if len(serverNames) > 0 {
		manifest["mcpServers"] = "./.mcp.json"
	}
	manifestPath := filepath.Join(root, ".codex-plugin", "plugin.json")
	if err := writeJSON(manifestPath, manifest); err != nil {
		return fmt.Errorf("write OpenAI compatibility manifest: %w", err)
	}
	if len(serverNames) == 0 {
		return nil
	}
	servers := map[string]map[string]any{}
	for _, name := range serverNames {
		server := envelope.MCP.Servers[name]
		config := cloneObject(server.Decoded)
		switch server.Type {
		case "stdio":
			delete(config, "type")
		case "streamable-http":
			config["type"] = "http"
		case "sse":
			config["type"] = "sse"
		default:
			continue
		}
		if hint, ok := hints.OpenAIMCPAuth[name]; ok {
			if hint.OAuthResource != "" {
				config["oauth_resource"] = hint.OAuthResource
			}
			if hint.BearerTokenEnvVar != "" {
				config["bearer_token_env_var"] = hint.BearerTokenEnvVar
			}
		}
		servers[name] = config
	}
	return writeJSON(filepath.Join(root, ".mcp.json"), map[string]any{"mcpServers": servers})
}

func supportedMCPNames(plan domain.DeliveryPlan) []string {
	var names []string
	for _, component := range plan.Components {
		if component.Kind == domain.ComponentMCPServer && component.Support != domain.SupportUnsupported {
			names = append(names, component.Name)
		}
	}
	sort.Strings(names)
	return names
}

func hasSupported(plan domain.DeliveryPlan, kind domain.ComponentKind) bool {
	for _, component := range plan.Components {
		if component.Kind == kind && component.Support != domain.SupportUnsupported {
			return true
		}
	}
	return false
}

func cloneObject(source map[string]any) map[string]any {
	result := make(map[string]any, len(source))
	for key, value := range source {
		result[key] = value
	}
	return result
}

func writeJSON(path string, value any) error {
	body, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	body = append(body, '\n')
	return atomicfile.Write(path, body, 0o644)
}

func removeStaging(base, path string) error {
	if err := pathpolicy.RequireContainedChild(base, path); err != nil {
		return err
	}
	return os.RemoveAll(path)
}
