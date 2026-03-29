package targetcontracts

import (
	"bytes"
	"encoding/json"
	"strings"
	"text/tabwriter"

	"github.com/777genius/plugin-kit-ai/sdk/platformmeta"
)

type Entry struct {
	Target                 string    `json:"target"`
	PlatformFamily         string    `json:"platform_family"`
	TargetClass            string    `json:"target_class"`
	LauncherRequirement    string    `json:"launcher_requirement"`
	TargetNoun             string    `json:"target_noun,omitempty"`
	ProductionClass        string    `json:"production_class"`
	RuntimeContract        string    `json:"runtime_contract"`
	InstallModel           string    `json:"install_model,omitempty"`
	DevModel               string    `json:"dev_model,omitempty"`
	ActivationModel        string    `json:"activation_model,omitempty"`
	NativeRoot             string    `json:"native_root,omitempty"`
	ImportSupport          bool      `json:"import_support"`
	RenderSupport          bool      `json:"render_support"`
	ValidateSupport        bool      `json:"validate_support"`
	PortableComponentKinds []string  `json:"portable_component_kinds"`
	TargetComponentKinds   []string  `json:"target_component_kinds"`
	NativeSurfaces         []Surface `json:"native_surfaces,omitempty"`
	ManagedArtifacts       []string  `json:"managed_artifacts"`
	Summary                string    `json:"summary"`
}

type Surface struct {
	Kind string `json:"kind"`
	Tier string `json:"tier"`
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
	_, _ = w.Write([]byte("TARGET\tFAMILY\tCLASS\tLAUNCHER\tNOUN\tINSTALL\tDEV\tACTIVATION\tNATIVE ROOT\tPRODUCTION\tRUNTIME\tIMPORT\tRENDER\tVALIDATE\tPORTABLE\tTARGET-NATIVE\tSURFACE TIERS\tMANAGED\tSUMMARY\n"))
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
				joinSurfaces(entry.NativeSurfaces) + "\t" +
				join(entry.ManagedArtifacts) + "\t" +
				entry.Summary + "\n"))
	}
	_ = w.Flush()
	return buf.Bytes()
}

func Markdown(entries []Entry) []byte {
	var b bytes.Buffer
	b.WriteString("# Target Support Matrix\n\n")
	b.WriteString("| Target | Platform Family | Target Class | Launcher | Target Noun | Install Model | Dev Model | Activation Model | Native Root | Production Class | Runtime Contract | Import | Render | Validate | Portable Components | Target-native Components | Surface Tiers | Managed Artifacts | Summary |\n")
	b.WriteString("| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |\n")
	for _, entry := range entries {
		b.WriteString("| " + entry.Target + " | " + entry.PlatformFamily + " | " + entry.TargetClass + " | " + entry.LauncherRequirement + " | " + entry.TargetNoun + " | " + entry.InstallModel + " | " + entry.DevModel + " | " + entry.ActivationModel + " | " + entry.NativeRoot + " | " + entry.ProductionClass + " | " + entry.RuntimeContract + " | " + yesNo(entry.ImportSupport) + " | " + yesNo(entry.RenderSupport) + " | " + yesNo(entry.ValidateSupport) + " | " + join(entry.PortableComponentKinds) + " | " + join(entry.TargetComponentKinds) + " | " + joinSurfaces(entry.NativeSurfaces) + " | " + join(entry.ManagedArtifacts) + " | " + entry.Summary + " |\n")
	}
	return b.Bytes()
}

func fromProfile(profile platformmeta.PlatformProfile) Entry {
	managed := make([]string, 0, len(profile.ManagedArtifacts))
	for _, item := range profile.ManagedArtifacts {
		switch item.Kind {
		case platformmeta.ManagedArtifactStatic, platformmeta.ManagedArtifactPortableMCP:
			managed = append(managed, item.Path)
		case platformmeta.ManagedArtifactPortableSkills:
			managed = append(managed, item.OutputRoot+"/**")
		case platformmeta.ManagedArtifactMirror:
			managed = append(managed, item.OutputRoot+"/**")
		case platformmeta.ManagedArtifactSelectedContext:
			managed = append(managed, "GEMINI.md or selected root context")
		}
	}
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
		PortableComponentKinds: append([]string(nil), profile.Contract.PortableComponentKinds...),
		TargetComponentKinds:   append([]string(nil), profile.Contract.TargetComponentKinds...),
		NativeSurfaces:         fromSurfaceSupport(profile.SurfaceTiers),
		ManagedArtifacts:       managed,
		Summary:                profile.Contract.Summary,
	}
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

func yesNo(v bool) string {
	if v {
		return "yes"
	}
	return "no"
}
