package opencode

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/777genius/plugin-kit-ai/install/integrationctl/domain"
	"github.com/777genius/plugin-kit-ai/install/integrationctl/ports"
	"github.com/tailscale/hujson"
)

func (a Adapter) patchConfig(ctx context.Context, path string, mutation configMutation, target *domain.TargetInstallation) (configPatchResult, error) {
	body, err := a.fs().ReadFile(ctx, path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return configPatchResult{}, domain.NewError(domain.ErrMutationApply, "read OpenCode config", err)
	}
	if errors.Is(err, os.ErrNotExist) {
		body = []byte("{}\n")
	}
	ast, err := hujson.Parse(body)
	if err != nil {
		return configPatchResult{}, domain.NewError(domain.ErrMutationApply, "parse OpenCode config", err)
	}
	obj, ok := ast.Value.(*hujson.Object)
	if !ok {
		return configPatchResult{}, domain.NewError(domain.ErrMutationApply, "OpenCode config root must be an object", nil)
	}
	doc, err := decodeConfigMap(body)
	if err != nil {
		return configPatchResult{}, domain.NewError(domain.ErrMutationApply, "decode OpenCode config", err)
	}
	currentPlugins, err := existingPluginRefs(doc["plugin"])
	if err != nil {
		return configPatchResult{}, domain.NewError(domain.ErrMutationApply, "parse OpenCode plugin refs", err)
	}
	currentMCP, err := existingObjectMap(doc["mcp"], "mcp")
	if err != nil {
		return configPatchResult{}, domain.NewError(domain.ErrMutationApply, "parse OpenCode MCP config", err)
	}
	oldPluginRefs := mapFromSlice(ownedPluginRefsOrMetadata(target), func(value string) string { return value })
	oldMCPAliases := mapFromSlice(ownedMCPAliasesOrMetadata(target), func(value string) string { return value })
	for _, ref := range mutation.PluginsSet {
		name := strings.TrimSpace(ref.Name)
		if name == "" {
			continue
		}
		if existing, ok := currentPlugins[name]; ok && !oldPluginRefs[name] && !pluginRefsEqual(existing, ref) {
			return configPatchResult{}, domain.NewError(domain.ErrStateConflict, "OpenCode plugin ref conflict for "+name, nil)
		}
	}
	for alias, desired := range mutation.MCPSet {
		if existing, ok := currentMCP[alias]; ok && !oldMCPAliases[alias] && !jsonValuesEqual(existing, desired) {
			return configPatchResult{}, domain.NewError(domain.ErrStateConflict, "OpenCode MCP alias conflict for "+alias, nil)
		}
	}
	for _, key := range mutation.WholeRemove {
		if strings.TrimSpace(key) == "" || key == "$schema" {
			continue
		}
		removeTopLevelMember(obj, key)
	}
	mergedPlugins := mergePluginRefs(currentPlugins, mutation.PluginsRemove, mutation.PluginsSet)
	if len(mergedPlugins) == 0 {
		removeTopLevelMember(obj, "plugin")
	} else if err := setTopLevelMember(obj, "plugin", pluginRefsToJSON(mergedPlugins)); err != nil {
		return configPatchResult{}, domain.NewError(domain.ErrMutationApply, "patch OpenCode plugin refs", err)
	}
	mergedMCP := mergeNamedObject(currentMCP, mutation.MCPRemove, mutation.MCPSet)
	if len(mergedMCP) == 0 {
		removeTopLevelMember(obj, "mcp")
	} else if err := setTopLevelMember(obj, "mcp", mergedMCP); err != nil {
		return configPatchResult{}, domain.NewError(domain.ErrMutationApply, "patch OpenCode MCP config", err)
	}
	for key, value := range mutation.WholeSet {
		if err := setTopLevelMember(obj, key, value); err != nil {
			return configPatchResult{}, domain.NewError(domain.ErrMutationApply, "patch OpenCode config", err)
		}
	}
	if len(obj.Members) == 0 {
		if err := a.fs().Remove(ctx, path); err != nil {
			return configPatchResult{}, domain.NewError(domain.ErrMutationApply, "remove empty OpenCode config", err)
		}
		return configPatchResult{
			ConfigPath: path,
		}, nil
	}
	rendered := ast.Pack()
	if _, err := a.mutator().MutateFile(ctx, ports.SafeFileMutationInput{
		Path: path,
		Mode: 0o644,
		Build: func(_ []byte, _ bool) ([]byte, error) {
			return rendered, nil
		},
		ValidateBefore: func(next []byte) error {
			_, err := hujson.Parse(next)
			return err
		},
		ValidateAfter: func(_ context.Context, path string, _ []byte) error {
			body, err := a.fs().ReadFile(ctx, path)
			if err != nil {
				return err
			}
			_, err = hujson.Parse(body)
			return err
		},
	}); err != nil {
		return configPatchResult{}, domain.NewError(domain.ErrMutationApply, "write OpenCode config", err)
	}
	return configPatchResult{
		Body:            rendered,
		ConfigPath:      path,
		ManagedKeys:     sortedManagedKeys(mutation.WholeSet),
		OwnedPluginRefs: pluginRefNames(mutation.PluginsSet),
		OwnedMCPAliases: sortedMapKeys(mutation.MCPSet),
	}, nil
}

func setTopLevelMember(obj *hujson.Object, key string, value any) error {
	memberValue, err := valueToHuJSONValue(value)
	if err != nil {
		return err
	}
	for i := range obj.Members {
		name := obj.Members[i].Name.Value.(hujson.Literal).String()
		if name == key {
			memberValue.BeforeExtra = obj.Members[i].Value.BeforeExtra
			memberValue.AfterExtra = obj.Members[i].Value.AfterExtra
			obj.Members[i].Value = memberValue
			return nil
		}
	}
	nameValue := hujson.Value{Value: hujson.String(key)}
	memberValue.BeforeExtra = []byte("\n  ")
	memberValue.AfterExtra = []byte{}
	obj.Members = append(obj.Members, hujson.ObjectMember{Name: nameValue, Value: memberValue})
	return nil
}

func valueToHuJSONValue(value any) (hujson.Value, error) {
	body, err := json.Marshal(value)
	if err != nil {
		return hujson.Value{}, err
	}
	parsed, err := hujson.Parse(body)
	if err != nil {
		return hujson.Value{}, err
	}
	return parsed, nil
}

func removeTopLevelMember(obj *hujson.Object, key string) {
	filtered := obj.Members[:0]
	for i := range obj.Members {
		name := obj.Members[i].Name.Value.(hujson.Literal).String()
		if name != key {
			filtered = append(filtered, obj.Members[i])
		}
	}
	obj.Members = filtered
}

func decodeConfigMap(body []byte) (map[string]any, error) {
	body, err := hujson.Standardize(body)
	if err != nil {
		return nil, err
	}
	var doc map[string]any
	if err := json.Unmarshal(body, &doc); err != nil {
		return nil, err
	}
	if doc == nil {
		doc = map[string]any{}
	}
	return doc, nil
}

func existingObjectMap(raw any, field string) (map[string]any, error) {
	if raw == nil {
		return map[string]any{}, nil
	}
	typed, ok := raw.(map[string]any)
	if !ok {
		return nil, errors.New("OpenCode field " + field + " must be an object")
	}
	return typed, nil
}

func existingPluginRefs(raw any) (map[string]pluginRef, error) {
	if raw == nil {
		return map[string]pluginRef{}, nil
	}
	values, ok := raw.([]any)
	if !ok {
		return nil, errors.New("OpenCode field plugin must be an array")
	}
	out := make(map[string]pluginRef, len(values))
	for i, value := range values {
		ref, err := normalizePluginRef(value)
		if err != nil {
			return nil, errors.New("OpenCode plugin ref at index " + strconvI(i) + " is invalid: " + err.Error())
		}
		out[ref.Name] = ref
	}
	return out, nil
}

func normalizePluginRef(value any) (pluginRef, error) {
	switch typed := value.(type) {
	case string:
		name := strings.TrimSpace(typed)
		if name == "" {
			return pluginRef{}, errors.New("plugin ref must be a non-empty string")
		}
		return pluginRef{Name: name}, nil
	case []any:
		if len(typed) != 2 {
			return pluginRef{}, errors.New("tuple plugin ref must have exactly 2 items")
		}
		name, ok := typed[0].(string)
		if !ok || strings.TrimSpace(name) == "" {
			return pluginRef{}, errors.New("tuple plugin ref name must be a non-empty string")
		}
		options, ok := typed[1].(map[string]any)
		if !ok {
			return pluginRef{}, errors.New("tuple plugin ref options must be an object")
		}
		return pluginRef{Name: strings.TrimSpace(name), Options: options}, nil
	default:
		return pluginRef{}, errors.New("plugin ref must be a string or [name, options] tuple")
	}
}

func mergePluginRefs(existing map[string]pluginRef, remove []string, set []pluginRef) []pluginRef {
	removeSet := mapFromSlice(remove, func(value string) string { return value })
	setMap := make(map[string]pluginRef, len(set))
	for _, ref := range set {
		if strings.TrimSpace(ref.Name) == "" {
			continue
		}
		setMap[ref.Name] = ref
	}
	for name := range setMap {
		removeSet[name] = true
	}
	out := make([]pluginRef, 0, len(existing)+len(setMap))
	for name, ref := range existing {
		if !removeSet[name] {
			out = append(out, ref)
		}
	}
	for _, ref := range setMap {
		out = append(out, ref)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func mergeNamedObject(existing map[string]any, remove []string, set map[string]any) map[string]any {
	out := make(map[string]any, len(existing)+len(set))
	removeSet := mapFromSlice(remove, func(value string) string { return value })
	for key, value := range existing {
		if !removeSet[key] {
			out[key] = value
		}
	}
	for key, value := range set {
		out[key] = value
	}
	return out
}

func pluginRefsToJSON(refs []pluginRef) []any {
	out := make([]any, 0, len(refs))
	for _, ref := range refs {
		out = append(out, ref.jsonValue())
	}
	return out
}

func pluginRefNames(refs []pluginRef) []string {
	out := make([]string, 0, len(refs))
	for _, ref := range refs {
		if strings.TrimSpace(ref.Name) != "" {
			out = append(out, ref.Name)
		}
	}
	sort.Strings(out)
	return out
}

func pluginRefsEqual(left, right pluginRef) bool {
	if strings.TrimSpace(left.Name) != strings.TrimSpace(right.Name) {
		return false
	}
	return jsonValuesEqual(left.Options, right.Options)
}

func jsonValuesEqual(left, right any) bool {
	leftBody, err := json.Marshal(left)
	if err != nil {
		return false
	}
	rightBody, err := json.Marshal(right)
	if err != nil {
		return false
	}
	return string(leftBody) == string(rightBody)
}

func strconvI(value int) string {
	return strconv.Itoa(value)
}
