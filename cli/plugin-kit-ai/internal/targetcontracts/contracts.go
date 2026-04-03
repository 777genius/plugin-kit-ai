package targetcontracts

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"
	"text/tabwriter"

	"github.com/777genius/plugin-kit-ai/sdk/platformmeta"
)

type Entry struct {
	Target                 string            `json:"target"`
	PlatformFamily         string            `json:"platform_family"`
	TargetClass            string            `json:"target_class"`
	LauncherRequirement    string            `json:"launcher_requirement"`
	TargetNoun             string            `json:"target_noun,omitempty"`
	ProductionClass        string            `json:"production_class"`
	RuntimeContract        string            `json:"runtime_contract"`
	InstallModel           string            `json:"install_model,omitempty"`
	DevModel               string            `json:"dev_model,omitempty"`
	ActivationModel        string            `json:"activation_model,omitempty"`
	NativeRoot             string            `json:"native_root,omitempty"`
	ImportSupport          bool              `json:"import_support"`
	RenderSupport          bool              `json:"render_support"`
	ValidateSupport        bool              `json:"validate_support"`
	PortableComponentKinds []string          `json:"portable_component_kinds"`
	TargetComponentKinds   []string          `json:"target_component_kinds"`
	NativeDocs             []string          `json:"native_docs,omitempty"`
	NativeDocPaths         map[string]string `json:"native_doc_paths,omitempty"`
	NativeSurfaces         []Surface         `json:"native_surfaces,omitempty"`
	NativeSurfaceTiers     map[string]string `json:"native_surface_tiers,omitempty"`
	ManagedArtifactRules   []ManagedArtifact `json:"managed_artifact_rules,omitempty"`
	ManagedArtifacts       []string          `json:"managed_artifacts"`
	Summary                string            `json:"summary"`
}

type Surface struct {
	Kind string `json:"kind"`
	Tier string `json:"tier"`
}

type ManagedArtifact struct {
	Path      string `json:"path"`
	Condition string `json:"condition,omitempty"`
}

func All() []Entry {
	profiles := platformmeta.All()
	out := make([]Entry, 0, len(profiles))
	for _, profile := range profiles {
		out = append(out, fromProfile(profile))
	}
	return out
}

func ByTarget(name string) []Entry {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" {
		return All()
	}
	out := make([]Entry, 0, 1)
	for _, entry := range All() {
		if entry.Target == name {
			out = append(out, entry)
		}
	}
	return out
}

func Lookup(name string) (Entry, bool) {
	profile, ok := platformmeta.Lookup(name)
	if !ok {
		return Entry{}, false
	}
	return fromProfile(profile), true
}

func JSON(entries []Entry) ([]byte, error) {
	return json.MarshalIndent(entries, "", "  ")
}

func Table(entries []Entry) []byte {
	var buf bytes.Buffer
	w := tabwriter.NewWriter(&buf, 0, 0, 2, ' ', 0)
	_, _ = w.Write([]byte("TARGET\tFAMILY\tCLASS\tLAUNCHER\tNOUN\tINSTALL\tDEV\tACTIVATION\tNATIVE ROOT\tPRODUCTION\tRUNTIME\tIMPORT\tRENDER\tVALIDATE\tPORTABLE\tTARGET-NATIVE\tNATIVE DOCS\tSURFACE TIERS\tMANAGED\tSUMMARY\n"))
	for _, entry := range entries {
		_, _ = w.Write([]byte(
			entry.Target + "\t" +
				entry.PlatformFamily + "\t" +
				entry.TargetClass + "\t" +
				entry.LauncherRequirement + "\t" +
				entry.TargetNoun + "\t" +
				entry.InstallModel + "\t" +
				entry.DevModel + "\t" +
				entry.ActivationModel + "\t" +
				entry.NativeRoot + "\t" +
				entry.ProductionClass + "\t" +
				entry.RuntimeContract + "\t" +
				yesNo(entry.ImportSupport) + "\t" +
				yesNo(entry.RenderSupport) + "\t" +
				yesNo(entry.ValidateSupport) + "\t" +
				join(entry.PortableComponentKinds) + "\t" +
				join(entry.TargetComponentKinds) + "\t" +
				join(entry.NativeDocs) + "\t" +
				joinSurfaces(entry.NativeSurfaces) + "\t" +
				joinManagedArtifacts(entry.ManagedArtifactRules) + "\t" +
				entry.Summary + "\n"))
	}
	_ = w.Flush()
	return buf.Bytes()
}

func Markdown(entries []Entry) []byte {
	var b bytes.Buffer
	b.WriteString("# Target Support Matrix\n\n")
	b.WriteString("| Target | Platform Family | Target Class | Launcher | Target Noun | Install Model | Dev Model | Activation Model | Native Root | Production Class | Runtime Contract | Import | Render | Validate | Portable Components | Target-native Components | Native Docs | Surface Tiers | Managed Artifacts | Summary |\n")
	b.WriteString("| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |\n")
	for _, entry := range entries {
		b.WriteString("| " + entry.Target + " | " + entry.PlatformFamily + " | " + entry.TargetClass + " | " + entry.LauncherRequirement + " | " + entry.TargetNoun + " | " + entry.InstallModel + " | " + entry.DevModel + " | " + entry.ActivationModel + " | " + entry.NativeRoot + " | " + entry.ProductionClass + " | " + entry.RuntimeContract + " | " + yesNo(entry.ImportSupport) + " | " + yesNo(entry.RenderSupport) + " | " + yesNo(entry.ValidateSupport) + " | " + join(entry.PortableComponentKinds) + " | " + join(entry.TargetComponentKinds) + " | " + join(entry.NativeDocs) + " | " + joinSurfaces(entry.NativeSurfaces) + " | " + joinManagedArtifacts(entry.ManagedArtifactRules) + " | " + entry.Summary + " |\n")
	}
	return b.Bytes()
}

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
		RenderSupport:          profile.Contract.RenderSupport,
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
		out = append(out, item.Kind+"="+item.Path)
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
		out[item.Kind] = item.Path
	}
	if len(out) == 0 {
		return nil
	}
	return out
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

func join(items []string) string {
	if len(items) == 0 {
		return "-"
	}
	return strings.Join(items, ", ")
}

func joinSurfaces(items []Surface) string {
	if len(items) == 0 {
		return "-"
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		out = append(out, item.Kind+"="+item.Tier)
	}
	return strings.Join(out, ", ")
}

func joinManagedArtifacts(items []ManagedArtifact) string {
	if len(items) == 0 {
		return "-"
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		label := item.Path
		if strings.TrimSpace(item.Condition) != "" {
			label += " (" + item.Condition + ")"
		}
		out = append(out, label)
	}
	return strings.Join(out, ", ")
}

func yesNo(v bool) string {
	if v {
		return "yes"
	}
	return "no"
}

func managedArtifactRules(profile platformmeta.PlatformProfile) []ManagedArtifact {
	out := make([]ManagedArtifact, 0, len(profile.ManagedArtifacts))
	for _, item := range profile.ManagedArtifacts {
		switch item.Kind {
		case platformmeta.ManagedArtifactStatic:
			out = append(out, ManagedArtifact{
				Path:      item.Path,
				Condition: managedArtifactCondition(profile, item),
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
			path := ""
			if item.OutputRoot == "" {
				docPath := ""
				for _, doc := range profile.NativeDocs {
					if doc.Kind == item.ComponentKind {
						docPath = doc.Path
						break
					}
				}
				if strings.TrimSpace(docPath) == "" {
					continue
				}
				path = filepath.Base(docPath)
			} else {
				path = item.OutputRoot + "/**"
			}
			out = append(out, ManagedArtifact{
				Path:      path,
				Condition: managedArtifactCondition(profile, item),
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

func managedArtifactCondition(profile platformmeta.PlatformProfile, item platformmeta.ManagedArtifactSpec) string {
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
