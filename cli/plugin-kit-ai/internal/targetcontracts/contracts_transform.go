package targetcontracts

import (
	"path/filepath"
	"strings"

	"github.com/777genius/plugin-kit-ai/cli/internal/pluginmodel"
	"github.com/777genius/plugin-kit-ai/sdk/platformmeta"
)

func fromProfile(profile platformmeta.PlatformProfile) Entry {
	rules := managedArtifactRules(profile)
	return Entry{
		Target:                 profile.ID,
		PlatformFamily:         string(profile.Contract.PlatformFamily),
		TargetClass:            profile.Contract.TargetClass,
		LauncherRequirement:    string(profile.Launcher.Requirement),
		TargetNoun:             profile.Contract.TargetNoun,
		ProductionClass:        profile.Contract.ProductionClass,
		RuntimeContract:        profile.Contract.RuntimeContract,
		InstallModel:           profile.Contract.InstallModel,
		DevModel:               profile.Contract.DevModel,
		ActivationModel:        profile.Contract.ActivationModel,
		NativeRoot:             profile.Contract.NativeRoot,
		ImportSupport:          profile.Contract.ImportSupport,
		GenerateSupport:        profile.Contract.GenerateSupport,
		ValidateSupport:        profile.Contract.ValidateSupport,
		PortableComponentKinds: cloneStrings(profile.Contract.PortableComponentKinds),
		TargetComponentKinds:   cloneStrings(profile.Contract.TargetComponentKinds),
		NativeDocs:             nativeDocs(profile.NativeDocs),
		NativeDocPaths:         nativeDocPaths(profile.NativeDocs),
		NativeSurfaces:         fromSurfaceSupport(profile.SurfaceTiers),
		NativeSurfaceTiers:     nativeSurfaceTiers(profile.SurfaceTiers),
		ManagedArtifactRules:   rules,
		ManagedArtifacts:       managedArtifactStrings(rules),
		Summary:                profile.Contract.Summary,
	}
}

func cloneStrings(items []string) []string {
	if len(items) == 0 {
		return []string{}
	}
	return append([]string{}, items...)
}

func nativeDocs(items []platformmeta.NativeDocSpec) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		if strings.TrimSpace(item.Path) == "" {
			continue
		}
		out = append(out, item.Kind+"="+authoringDocPath(item.Path))
	}
	return out
}

func nativeDocPaths(items []platformmeta.NativeDocSpec) map[string]string {
	if len(items) == 0 {
		return nil
	}
	out := make(map[string]string, len(items))
	for _, item := range items {
		if strings.TrimSpace(item.Kind) == "" || strings.TrimSpace(item.Path) == "" {
			continue
		}
		out[item.Kind] = authoringDocPath(item.Path)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func authoringDocPath(path string) string {
	path = filepath.ToSlash(strings.TrimSpace(path))
	if path == "" {
		return ""
	}
	if path == pluginmodel.SourceDirName || strings.HasPrefix(path, pluginmodel.SourceDirName+"/") {
		return path
	}
	return filepath.ToSlash(filepath.Join(pluginmodel.SourceDirName, path))
}

func fromSurfaceSupport(items []platformmeta.SurfaceSupport) []Surface {
	out := make([]Surface, 0, len(items))
	for _, item := range items {
		out = append(out, Surface{
			Kind: item.Kind,
			Tier: string(item.Tier),
		})
	}
	return out
}

func nativeSurfaceTiers(items []platformmeta.SurfaceSupport) map[string]string {
	if len(items) == 0 {
		return nil
	}
	out := make(map[string]string, len(items))
	for _, item := range items {
		if strings.TrimSpace(item.Kind) == "" || strings.TrimSpace(string(item.Tier)) == "" {
			continue
		}
		out[item.Kind] = string(item.Tier)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func managedArtifactRules(profile platformmeta.PlatformProfile) []ManagedArtifact {
	out := make([]ManagedArtifact, 0, len(profile.ManagedArtifacts))
	for _, item := range profile.ManagedArtifacts {
		switch item.Kind {
		case platformmeta.ManagedArtifactStatic:
			out = append(out, ManagedArtifact{
				Path:      item.Path,
				Condition: managedArtifactCondition(item),
			})
		case platformmeta.ManagedArtifactPortableMCP:
			out = append(out, ManagedArtifact{
				Path:      item.Path,
				Condition: "when portable MCP is authored",
			})
		case platformmeta.ManagedArtifactPortableSkills:
			out = append(out, ManagedArtifact{
				Path:      item.OutputRoot + "/**",
				Condition: "when portable skills are authored",
			})
		case platformmeta.ManagedArtifactMirror:
			path := managedMirrorArtifactPath(profile, item)
			if strings.TrimSpace(path) == "" {
				continue
			}
			out = append(out, ManagedArtifact{
				Path:      path,
				Condition: managedArtifactCondition(item),
			})
		case platformmeta.ManagedArtifactSelectedContext:
			out = append(out, ManagedArtifact{
				Path:      "GEMINI.md or selected root context",
				Condition: "when contexts are authored",
			})
		}
	}
	return out
}

func managedMirrorArtifactPath(profile platformmeta.PlatformProfile, item platformmeta.ManagedArtifactSpec) string {
	if item.OutputRoot != "" {
		return item.OutputRoot + "/**"
	}
	for _, doc := range profile.NativeDocs {
		if doc.Kind == item.ComponentKind {
			return filepath.Base(doc.Path)
		}
	}
	return ""
}

func managedArtifactStrings(items []ManagedArtifact) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		label := item.Path
		if strings.TrimSpace(item.Condition) != "" {
			label += " (" + item.Condition + ")"
		}
		out = append(out, label)
	}
	return out
}

func managedArtifactCondition(item platformmeta.ManagedArtifactSpec) string {
	switch {
	case item.Path == ".app.json":
		return "when app_manifest is enabled"
	case item.ComponentKind != "":
		return authoredCondition(item.ComponentKind)
	case item.OutputRoot != "":
		return authoredCondition(strings.Trim(filepath.Base(item.OutputRoot), "/"))
	default:
		return ""
	}
}

func authoredCondition(kind string) string {
	kind = strings.TrimSpace(kind)
	if kind == "" {
		return ""
	}
	verb := "is"
	if strings.HasSuffix(kind, "s") {
		verb = "are"
	}
	return "when " + kind + " " + verb + " authored"
}
