package generator

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/777genius/plugin-kit-ai/sdk/internal/descriptors/defs"
	"github.com/777genius/plugin-kit-ai/sdk/internal/runtime"
	"github.com/777genius/plugin-kit-ai/sdk/platformmeta"
)

func renderSupportMatrix(m model) string {
	var b strings.Builder
	b.WriteString("# Generated Support Matrix\n\n")
	b.WriteString("This generated table is the canonical per-event runtime reference for shipped support claims. Use it as exact contract data, not as front-door positioning. Package, extension, and repo-managed integration lanes remain summarized in SUPPORT.md and the target support matrix.\n\n")
	b.WriteString("| Platform | Event | Status | Maturity | Contract Class | V1 Target | Invocation | Carrier | Transport Modes | Scaffold | Validate | Capabilities | Live Test | Summary |\n")
	b.WriteString("|----------|-------|--------|----------|----------------|-----------|------------|---------|-----------------|----------|----------|--------------|-----------|---------|\n")
	for _, e := range m.events {
		p := m.profiles[e.Platform]
		b.WriteString(fmt.Sprintf("| %s | %s | %s | %s | %s | %t | %s | %s | %s | %t | %t | %s | %s | %s |\n",
			e.Platform,
			e.Event,
			p.Status,
			e.Contract.Maturity,
			contractClass(p, e),
			e.Contract.V1Target,
			e.Invocation.Kind,
			e.Carrier,
			joinTransportModes(p.TransportModes),
			len(p.Scaffold.RequiredFiles) > 0,
			len(p.Validate.RequiredFiles) > 0,
			joinCapabilities(e.Capabilities),
			p.LiveTestProfile,
			e.Docs.Summary,
		))
	}
	return b.String()
}

func renderTargetSupportMatrix(m model) string {
	var b strings.Builder
	b.WriteString("# Target Support Matrix\n\n")
	b.WriteString("| Target | Platform Family | Target Class | Launcher | Target Noun | Install Model | Dev Model | Activation Model | Native Root | Production Class | Runtime Contract | Import | Generate | Validate | Portable Components | Target-native Components | Native Docs | Surface Tiers | Managed Artifacts | Summary |\n")
	b.WriteString("| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |\n")
	for _, profile := range scaffoldTargetProfiles(m) {
		b.WriteString("| " + profile.ID + " | " +
			string(profile.Contract.PlatformFamily) + " | " +
			profile.Contract.TargetClass + " | " +
			string(profile.Launcher.Requirement) + " | " +
			profile.Contract.TargetNoun + " | " +
			profile.Contract.InstallModel + " | " +
			profile.Contract.DevModel + " | " +
			profile.Contract.ActivationModel + " | " +
			profile.Contract.NativeRoot + " | " +
			profile.Contract.ProductionClass + " | " +
			profile.Contract.RuntimeContract + " | " +
			boolString(profile.Contract.ImportSupport) + " | " +
			boolString(profile.Contract.GenerateSupport) + " | " +
			boolString(profile.Contract.ValidateSupport) + " | " +
			joinStrings(profile.Contract.PortableComponentKinds) + " | " +
			joinStrings(profile.Contract.TargetComponentKinds) + " | " +
			joinNativeDocs(profile.NativeDocs) + " | " +
			joinSurfaceSupport(profile.SurfaceTiers) + " | " +
			joinManagedArtifacts(profile) + " | " +
			profile.Contract.Summary + " |\n")
	}
	return b.String()
}

func contractClass(p defs.PlatformProfile, e defs.EventDescriptor) string {
	if p.Status == runtime.StatusRuntimeSupported {
		switch e.Contract.Maturity {
		case runtime.MaturityStable:
			return "production-ready"
		case runtime.MaturityExperimental:
			return "public-experimental"
		default:
			return "runtime-supported but not stable"
		}
	}
	switch e.Contract.Maturity {
	case runtime.MaturityExperimental:
		return "public-experimental"
	default:
		return "public-beta"
	}
}

func boolString(v bool) string {
	if v {
		return "yes"
	}
	return "no"
}

func joinStrings(items []string) string {
	if len(items) == 0 {
		return "-"
	}
	return strings.Join(items, ", ")
}

func joinSurfaceSupport(items []platformmeta.SurfaceSupport) string {
	if len(items) == 0 {
		return "-"
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		out = append(out, item.Kind+"="+string(item.Tier))
	}
	return strings.Join(out, ", ")
}

func joinNativeDocs(items []platformmeta.NativeDocSpec) string {
	if len(items) == 0 {
		return "-"
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		if strings.TrimSpace(item.Path) == "" {
			continue
		}
		out = append(out, item.Kind+"="+authoringDocPath(item.Path))
	}
	if len(out) == 0 {
		return "-"
	}
	return strings.Join(out, ", ")
}

func authoringDocPath(path string) string {
	path = filepath.ToSlash(strings.TrimSpace(path))
	if path == "" {
		return ""
	}
	if path == platformmeta.CanonicalAuthoredRoot || strings.HasPrefix(path, platformmeta.CanonicalAuthoredRoot+"/") {
		return path
	}
	if path == platformmeta.LegacyAuthoredRoot {
		return platformmeta.CanonicalAuthoredRoot
	}
	if strings.HasPrefix(path, platformmeta.LegacyAuthoredRoot+"/") {
		return filepath.ToSlash(filepath.Join(platformmeta.CanonicalAuthoredRoot, strings.TrimPrefix(path, platformmeta.LegacyAuthoredRoot+"/")))
	}
	return filepath.ToSlash(filepath.Join(platformmeta.CanonicalAuthoredRoot, path))
}

func joinManagedArtifacts(profile platformmeta.PlatformProfile) string {
	if len(profile.ManagedArtifacts) == 0 {
		return "-"
	}
	out := make([]string, 0, len(profile.ManagedArtifacts))
	for _, item := range profile.ManagedArtifacts {
		label := ""
		switch item.Kind {
		case platformmeta.ManagedArtifactStatic, platformmeta.ManagedArtifactPortableMCP:
			label = item.Path
		case platformmeta.ManagedArtifactPortableSkills:
			label = item.OutputRoot + "/**"
		case platformmeta.ManagedArtifactMirror:
			if item.OutputRoot != "" {
				label = item.OutputRoot + "/**"
			} else {
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
				label = filepath.Base(docPath)
			}
		case platformmeta.ManagedArtifactSelectedContext:
			label = "GEMINI.md or selected root context"
		}
		if strings.TrimSpace(label) == "" {
			continue
		}
		if condition := managedArtifactCondition(profile, item); strings.TrimSpace(condition) != "" {
			label += " (" + condition + ")"
		}
		out = append(out, label)
	}
	if len(out) == 0 {
		return "-"
	}
	return strings.Join(out, ", ")
}

func managedArtifactCondition(profile platformmeta.PlatformProfile, item platformmeta.ManagedArtifactSpec) string {
	switch {
	case item.Kind == platformmeta.ManagedArtifactPortableMCP:
		return "when portable MCP is authored"
	case item.Kind == platformmeta.ManagedArtifactPortableSkills:
		return "when portable skills are authored"
	case item.Kind == platformmeta.ManagedArtifactSelectedContext:
		return "when contexts are authored"
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
