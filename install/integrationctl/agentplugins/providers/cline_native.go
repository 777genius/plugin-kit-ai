package providers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/777genius/plugin-kit-ai/install/integrationctl/adapters/filetree"
	"github.com/777genius/plugin-kit-ai/install/integrationctl/adapters/pathpolicy"
	"github.com/777genius/plugin-kit-ai/install/integrationctl/agentplugins/domain"
	"github.com/777genius/plugin-kit-ai/install/integrationctl/agentplugins/providers/nativeconfig"
)

const (
	clineSkillObjectKind = "cline_global_skill_directory"
	clineMCPObjectKind   = "cline_global_mcp_server"
	clineProjectionFile  = ".agentplugins-cline-native.json"
)

type clineProjection struct {
	Servers map[string]nativeconfig.Server `json:"servers"`
}

func projectClineNative(root string, envelope domain.PackageEnvelope, plan domain.DeliveryPlan, dataPath string) error {
	servers := make(map[string]nativeconfig.Server)
	for _, name := range supportedMCPNames(plan) {
		portable := envelope.MCP.Servers[name]
		if portable.Type == "stdio" {
			portable.Decoded = cloneObject(portable.Decoded)
			if err := applyStdioDataContract(portable.Decoded, plan.ActivePath, dataPath); err != nil {
				return fmt.Errorf("project Cline MCP server %s: %w", name, err)
			}
		}
		server, err := clineNeutralServer(portable)
		if err != nil {
			return fmt.Errorf("project Cline MCP server %s: %w", name, err)
		}
		// This first pass validates the codec without touching client state. The
		// exact configured path is bound later when ownership objects are built.
		settingsPath := filepath.Join(root, ".cline-settings-validation.json")
		if _, err := nativeconfig.DesiredReceipt(settingsPath, nativeconfig.CodecCline, name, server, nativeconfig.Placeholders{PackageRoot: plan.ActivePath, DataRoot: dataPath}); err != nil {
			return fmt.Errorf("project Cline MCP server %s: %w", name, err)
		}
		servers[name] = server
	}
	if len(servers) == 0 {
		return nil
	}
	return writeJSON(filepath.Join(root, clineProjectionFile), clineProjection{Servers: servers})
}

func buildClineNativeObjects(stagingRoot string, envelope domain.PackageEnvelope, plan domain.DeliveryPlan) ([]domain.NativeObjectOwnership, error) {
	root := strings.TrimSpace(plan.NativeRegistryRoot)
	if root == "" || !filepath.IsAbs(root) {
		return nil, fmt.Errorf("Cline config root is unavailable")
	}
	objects := make([]domain.NativeObjectOwnership, 0, len(envelope.Skills)+len(envelope.MCP.Servers))
	for _, component := range plan.Components {
		if component.Support == domain.SupportUnsupported {
			continue
		}
		if err := pathpolicy.ValidateLeafID(component.Name); err != nil {
			return nil, fmt.Errorf("invalid Cline component name %q: %w", component.Name, err)
		}
		switch component.Kind {
		case domain.ComponentSkill:
			skill, ok := envelope.Skills[component.Name]
			if !ok {
				return nil, fmt.Errorf("planned Cline skill %q is missing", component.Name)
			}
			relative := filepath.FromSlash(strings.TrimSpace(skill.RelativePath))
			if relative == "" {
				relative = filepath.Join("skills", component.Name, "SKILL.md")
			}
			sourceRoot := filepath.Join(stagingRoot, filepath.Dir(relative))
			if err := pathpolicy.RequireContainedChild(stagingRoot, sourceRoot); err != nil {
				return nil, err
			}
			digest, err := digestKiroSkillDirectory(sourceRoot)
			if err != nil {
				return nil, err
			}
			objects = append(objects, domain.NativeObjectOwnership{
				ObjectID: "cline-skill:" + component.Name, Kind: clineSkillObjectKind,
				LogicalName: component.Name, Path: filepath.Join(root, "skills", component.Name),
				SourceRelative: filepath.ToSlash(filepath.Dir(relative)), ManagedDigest: digest,
				ProtectionClass: "managed",
			})
		case domain.ComponentMCPServer:
			projection, err := readClineProjection(stagingRoot)
			if err != nil {
				return nil, err
			}
			server, ok := projection.Servers[component.Name]
			if !ok {
				return nil, fmt.Errorf("projected Cline MCP server %q is missing", component.Name)
			}
			settingsPath := clineMCPSettingsPath(root)
			if !filepath.IsAbs(settingsPath) {
				return nil, fmt.Errorf("Cline MCP settings path must be absolute")
			}
			receipt, err := nativeconfig.DesiredReceipt(settingsPath, nativeconfig.CodecCline, component.Name, server, nativeconfig.Placeholders{})
			if err != nil {
				return nil, err
			}
			objects = append(objects, domain.NativeObjectOwnership{
				ObjectID: "cline-mcp:" + component.Name, Kind: clineMCPObjectKind,
				LogicalName: component.Name, Path: settingsPath, ManagedDigest: receipt.Digest,
				SourceRelative: clineProjectionFile, ProtectionClass: "managed",
			})
		}
	}
	sort.Slice(objects, func(i, j int) bool { return objects[i].ObjectID < objects[j].ObjectID })
	return objects, nil
}

func activateClineNative(ctx context.Context, request domain.ActivationRequest) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return applyClineNativeMutation(request.Client.ConfigRoot, request.Delivery.ActivePath, request.PreviousNativeObjects, request.Delivery.NativeObjects)
}

func deactivateClineNative(ctx context.Context, request domain.DeactivationRequest) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return applyClineNativeMutation(request.Client.ConfigRoot, "", request.NativeObjects, nil)
}

func verifyClineNativeObjects(configRoot string, objects []domain.NativeObjectOwnership, allowMissing bool) error {
	kernel := nativeconfig.New()
	for _, object := range clineObjects(objects) {
		if err := validateClineObject(configRoot, object); err != nil {
			return err
		}
		switch object.Kind {
		case clineSkillObjectKind:
			digest, err := digestKiroSkillDirectory(object.Path)
			if os.IsNotExist(err) && allowMissing {
				continue
			}
			if err != nil || digest != object.ManagedDigest {
				return fmt.Errorf("managed Cline skill %q changed outside agentplugins", object.LogicalName)
			}
		case clineMCPObjectKind:
			receipt := clineReceipt(object)
			present, owned, err := kernel.Inspect(nativeconfig.Paths{JSON: object.Path}, nativeconfig.CodecCline, object.LogicalName, &receipt)
			if err != nil {
				return fmt.Errorf("inspect managed Cline MCP server %q: %w", object.LogicalName, err)
			}
			if !present && allowMissing {
				continue
			}
			if !present || !owned {
				return fmt.Errorf("managed Cline MCP server %q changed outside agentplugins: %w", object.LogicalName, nativeconfig.ErrNotOwned)
			}
		}
	}
	return nil
}

func applyClineNativeMutation(configRoot, activePath string, previous, desired []domain.NativeObjectOwnership) (resultErr error) {
	configRoot = strings.TrimSpace(configRoot)
	if configRoot == "" || !filepath.IsAbs(configRoot) {
		return fmt.Errorf("Cline config root is unavailable")
	}
	previous, desired = clineObjects(previous), clineObjects(desired)
	if err := verifyClineNativeObjects(configRoot, previous, true); err != nil {
		return err
	}
	previousByID, desiredByID := objectMap(previous), objectMap(desired)
	for id, object := range desiredByID {
		if prior, replacing := previousByID[id]; replacing {
			if prior.Kind != object.Kind || prior.LogicalName != object.LogicalName || !sameCleanPath(prior.Path, object.Path) {
				return fmt.Errorf("Cline native object identity changed unexpectedly for %s", id)
			}
			continue
		}
		if object.Kind == clineSkillObjectKind {
			if _, err := os.Lstat(object.Path); err == nil {
				return fmt.Errorf("Cline skill %q already exists without agentplugins ownership", object.LogicalName)
			} else if !os.IsNotExist(err) {
				return err
			}
		}
	}

	skillsRoot := filepath.Join(configRoot, "skills")
	if err := pathpolicy.RequireContainedChild(configRoot, skillsRoot); err != nil {
		return err
	}
	if err := os.MkdirAll(skillsRoot, 0o700); err != nil {
		return fmt.Errorf("create Cline skills root: %w", err)
	}
	transactionRoot, err := os.MkdirTemp(skillsRoot, ".agentplugins-native-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(transactionRoot)

	staged := map[string]string{}
	for id, object := range desiredByID {
		if object.Kind != clineSkillObjectKind {
			continue
		}
		if activePath == "" {
			return fmt.Errorf("active package path is required for Cline skill installation")
		}
		source := filepath.Join(activePath, filepath.FromSlash(object.SourceRelative))
		if err := pathpolicy.RequireContainedChild(activePath, source); err != nil {
			return err
		}
		target := filepath.Join(transactionRoot, "new-"+object.LogicalName)
		if err := filetree.CopyDir(source, target); err != nil {
			return err
		}
		digest, err := digestKiroSkillDirectory(target)
		if err != nil || digest != object.ManagedDigest {
			return fmt.Errorf("staged Cline skill %q does not match its ownership digest", object.LogicalName)
		}
		staged[id] = target
	}

	backups := map[string]string{}
	installed := map[string]domain.NativeObjectOwnership{}
	rollbackSkills := func() error {
		var first error
		for id, object := range installed {
			if digest, err := digestKiroSkillDirectory(object.Path); err == nil && digest == object.ManagedDigest {
				if err := os.RemoveAll(object.Path); err != nil && first == nil {
					first = err
				}
			}
			if backup := backups[id]; backup != "" {
				if err := os.Rename(backup, object.Path); err != nil && first == nil {
					first = err
				}
				delete(backups, id)
			}
		}
		for id, backup := range backups {
			if err := os.Rename(backup, previousByID[id].Path); err != nil && first == nil {
				first = err
			}
		}
		return first
	}
	defer func() {
		if resultErr != nil {
			if rollbackErr := rollbackSkills(); rollbackErr != nil {
				resultErr = errors.Join(resultErr, fmt.Errorf("Cline skill rollback: %w", rollbackErr))
			}
		}
	}()

	for id, object := range previousByID {
		if object.Kind != clineSkillObjectKind {
			continue
		}
		if _, err := os.Lstat(object.Path); os.IsNotExist(err) {
			continue
		} else if err != nil {
			return err
		}
		backup := filepath.Join(transactionRoot, "old-"+object.LogicalName)
		if err := os.Rename(object.Path, backup); err != nil {
			return err
		}
		backups[id] = backup
	}
	for id, source := range staged {
		object := desiredByID[id]
		if err := os.Rename(source, object.Path); err != nil {
			return err
		}
		installed[id] = object
	}

	// Prove the filesystem half before committing the single CAS-protected MCP
	// batch. ApplyBatch is the final operation and either writes every server or
	// none, so a config failure can still roll the skills back safely.
	if err := verifyClineNativeObjects(configRoot, clineSkillObjects(desired), false); err != nil {
		return err
	}
	if err := mutateClineMCP(configRoot, activePath, previousByID, desiredByID); err != nil {
		return err
	}
	return nil
}

func mutateClineMCP(configRoot, activePath string, previous, desired map[string]domain.NativeObjectOwnership) error {
	if !hasClineMCP(previous) && !hasClineMCP(desired) {
		return nil
	}
	path := clineMCPSettingsPath(configRoot)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	projection := clineProjection{Servers: map[string]nativeconfig.Server{}}
	if activePath != "" {
		var err error
		projection, err = readClineProjection(activePath)
		if err != nil {
			return err
		}
	}
	kernel := nativeconfig.New()
	ids := make([]string, 0, len(previous)+len(desired))
	seen := map[string]bool{}
	for id, object := range previous {
		if object.Kind == clineMCPObjectKind {
			ids, seen[id] = append(ids, id), true
		}
	}
	for id, object := range desired {
		if object.Kind == clineMCPObjectKind && !seen[id] {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	requests := make([]nativeconfig.Request, 0, len(ids))
	for _, id := range ids {
		prior, hadPrior := previous[id]
		next, hasNext := desired[id]
		req := nativeconfig.Request{Paths: nativeconfig.Paths{JSON: path}, Codec: nativeconfig.CodecCline}
		switch {
		case hadPrior && hasNext:
			receipt := clineReceipt(prior)
			req.Action, req.Name, req.Owned = nativeconfig.ActionUpdate, next.LogicalName, &receipt
			req.Server = projection.Servers[next.LogicalName]
		case hadPrior:
			receipt := clineReceipt(prior)
			req.Action, req.Name, req.Owned = nativeconfig.ActionRemove, prior.LogicalName, &receipt
		case hasNext:
			req.Action, req.Name, req.Server = nativeconfig.ActionAdd, next.LogicalName, projection.Servers[next.LogicalName]
		}
		if hasNext {
			receipt, err := nativeconfig.DesiredReceipt(path, nativeconfig.CodecCline, next.LogicalName, req.Server, req.Placeholders)
			if err != nil {
				return err
			}
			if receipt.Digest != next.ManagedDigest {
				return fmt.Errorf("Cline MCP desired receipt mismatch for %q", next.LogicalName)
			}
		}
		requests = append(requests, req)
	}
	_, err := kernel.ApplyBatch(requests)
	if err != nil {
		return err
	}
	return nil
}

func readClineProjection(root string) (clineProjection, error) {
	body, err := os.ReadFile(filepath.Join(root, clineProjectionFile))
	if err != nil {
		return clineProjection{}, fmt.Errorf("read projected Cline MCP configuration: %w", err)
	}
	var projection clineProjection
	if err := json.Unmarshal(body, &projection); err != nil || projection.Servers == nil {
		return clineProjection{}, fmt.Errorf("decode projected Cline MCP configuration: %w", err)
	}
	return projection, nil
}

func clineMCPSettingsPath(configRoot string) string {
	if path := strings.TrimSpace(os.Getenv("CLINE_MCP_SETTINGS_PATH")); path != "" {
		return filepath.Clean(path)
	}
	if data := strings.TrimSpace(os.Getenv("CLINE_DATA_DIR")); data != "" {
		return filepath.Join(filepath.Clean(data), "settings", "cline_mcp_settings.json")
	}
	return filepath.Join(configRoot, "data", "settings", "cline_mcp_settings.json")
}

func clineReceipt(object domain.NativeObjectOwnership) nativeconfig.Receipt {
	return nativeconfig.Receipt{Version: "1", Path: object.Path, Codec: nativeconfig.CodecCline, Name: object.LogicalName, Digest: object.ManagedDigest}
}

func clineObjects(objects []domain.NativeObjectOwnership) []domain.NativeObjectOwnership {
	result := make([]domain.NativeObjectOwnership, 0, len(objects))
	for _, object := range objects {
		if object.Kind == clineSkillObjectKind || object.Kind == clineMCPObjectKind {
			result = append(result, object)
		}
	}
	return result
}

func clineSkillObjects(objects []domain.NativeObjectOwnership) []domain.NativeObjectOwnership {
	result := make([]domain.NativeObjectOwnership, 0, len(objects))
	for _, object := range objects {
		if object.Kind == clineSkillObjectKind {
			result = append(result, object)
		}
	}
	return result
}

func validateClineObject(configRoot string, object domain.NativeObjectOwnership) error {
	if object.ProtectionClass != "managed" || object.ObjectID == "" || object.LogicalName == "" || object.ManagedDigest == "" {
		return fmt.Errorf("invalid Cline native ownership object")
	}
	if object.Kind == clineSkillObjectKind {
		if err := pathpolicy.RequireContainedChild(filepath.Join(configRoot, "skills"), object.Path); err != nil {
			return fmt.Errorf("unsafe Cline skill path: %w", err)
		}
	} else if object.Kind == clineMCPObjectKind {
		if !filepath.IsAbs(object.Path) || !sameCleanPath(object.Path, clineMCPSettingsPath(configRoot)) {
			return fmt.Errorf("Cline MCP ownership path changed")
		}
	} else {
		return fmt.Errorf("unsupported Cline native object kind %q", object.Kind)
	}
	return nil
}

func hasClineMCP(objects map[string]domain.NativeObjectOwnership) bool {
	for _, object := range objects {
		if object.Kind == clineMCPObjectKind {
			return true
		}
	}
	return false
}

func clineNeutralServer(server domain.MCPServer) (nativeconfig.Server, error) {
	getString := func(key string) string {
		value, _ := server.Decoded[key].(string)
		return value
	}
	result := nativeconfig.Server{Type: server.Type, Command: getString("command"), URL: getString("url")}
	if server.Type == "streamable-http" || server.Type == "sse" {
		result.Type = "remote"
		result.RemoteTransport = server.Type
	}
	if values, ok := server.Decoded["args"].([]any); ok {
		for _, value := range values {
			text, ok := value.(string)
			if !ok {
				return result, fmt.Errorf("args must be strings")
			}
			result.Args = append(result.Args, text)
		}
	} else if values, ok := server.Decoded["args"].([]string); ok {
		result.Args = append([]string(nil), values...)
	}
	copyMap := func(key string) (map[string]string, error) {
		result := map[string]string{}
		switch values := server.Decoded[key].(type) {
		case nil:
			return nil, nil
		case map[string]any:
			for name, value := range values {
				text, ok := value.(string)
				if !ok {
					return nil, fmt.Errorf("%s values must be strings", key)
				}
				result[name] = text
			}
		case map[string]string:
			for name, value := range values {
				result[name] = value
			}
		default:
			return nil, fmt.Errorf("%s must be an object", key)
		}
		return result, nil
	}
	var err error
	result.Env, err = copyMap("env")
	if err != nil {
		return result, err
	}
	result.Headers, err = copyMap("headers")
	return result, err
}
