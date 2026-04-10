package platformexec

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

type opencodePackageMeta struct {
	Plugins []opencodePluginRef `yaml:"plugins,omitempty"`
}

type opencodePluginRef struct {
	Name    string
	Options map[string]any
}

func (r *opencodePluginRef) UnmarshalYAML(node *yaml.Node) error {
	if node == nil {
		*r = opencodePluginRef{}
		return nil
	}
	switch node.Kind {
	case yaml.ScalarNode:
		var name string
		if err := node.Decode(&name); err != nil {
			return err
		}
		r.Name = strings.TrimSpace(name)
		r.Options = nil
		return nil
	case yaml.MappingNode:
		var raw map[string]any
		if err := node.Decode(&raw); err != nil {
			return err
		}
		for key := range raw {
			switch key {
			case "name", "options":
			default:
				return fmt.Errorf("unsupported OpenCode plugin metadata field %q", key)
			}
		}
		name, _ := raw["name"].(string)
		r.Name = strings.TrimSpace(name)
		if options, ok := raw["options"]; ok {
			typed, ok := options.(map[string]any)
			if !ok {
				return fmt.Errorf("OpenCode plugin metadata options must be a mapping")
			}
			r.Options = typed
		} else {
			r.Options = nil
		}
		return nil
	default:
		return fmt.Errorf("OpenCode plugin metadata entries must be strings or mappings")
	}
}

func (r opencodePluginRef) MarshalYAML() (any, error) {
	if len(r.Options) == 0 {
		return strings.TrimSpace(r.Name), nil
	}
	return map[string]any{
		"name":    strings.TrimSpace(r.Name),
		"options": r.Options,
	}, nil
}

func (r opencodePluginRef) jsonValue() any {
	name := strings.TrimSpace(r.Name)
	if len(r.Options) == 0 {
		return name
	}
	return []any{name, r.Options}
}

func normalizeImportedOpenCodePluginRef(value any) (opencodePluginRef, error) {
	switch typed := value.(type) {
	case string:
		text := strings.TrimSpace(typed)
		if text == "" {
			return opencodePluginRef{}, fmt.Errorf("plugin ref must be a non-empty string")
		}
		return opencodePluginRef{Name: text}, nil
	case []any:
		if len(typed) != 2 {
			return opencodePluginRef{}, fmt.Errorf("tuple plugin ref must have exactly 2 items")
		}
		name, ok := typed[0].(string)
		if !ok || strings.TrimSpace(name) == "" {
			return opencodePluginRef{}, fmt.Errorf("tuple plugin ref name must be a non-empty string")
		}
		options, ok := typed[1].(map[string]any)
		if !ok {
			return opencodePluginRef{}, fmt.Errorf("tuple plugin ref options must be an object")
		}
		return opencodePluginRef{Name: strings.TrimSpace(name), Options: options}, nil
	default:
		return opencodePluginRef{}, fmt.Errorf("plugin ref must be a string or [name, options] tuple")
	}
}

func validateOpenCodePluginRefs(refs []opencodePluginRef) error {
	for i, ref := range refs {
		if strings.TrimSpace(ref.Name) == "" {
			return fmt.Errorf("plugin entry %d must define a non-empty name", i)
		}
		if ref.Options == nil {
			continue
		}
		for key := range ref.Options {
			if strings.TrimSpace(key) == "" {
				return fmt.Errorf("plugin entry %d options may not contain empty keys", i)
			}
		}
	}
	return nil
}

func jsonValuesForOpenCodePlugins(refs []opencodePluginRef) []any {
	out := make([]any, 0, len(refs))
	for _, ref := range refs {
		out = append(out, ref.jsonValue())
	}
	return out
}

func isOpenCodePermissionValue(value any) bool {
	if _, ok := value.(string); ok {
		return true
	}
	_, ok := value.(map[string]any)
	return ok
}
