package validate

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/777genius/plugin-kit-ai/cli/internal/platformexec"
	"github.com/777genius/plugin-kit-ai/cli/internal/pluginmanifest"
	"github.com/777genius/plugin-kit-ai/sdk/platformmeta"
)

func validatePortableContractCoverage(target string, profile platformmeta.PlatformProfile, graph pluginmanifest.PackageGraph) []Failure {
	var failures []Failure
	if len(graph.Portable.Paths("skills")) == 0 {
		return failures
	}
	if !slices.Contains(profile.Contract.PortableComponentKinds, "skills") {
		return failures
	}
	if !portableSkillsManaged(profile) {
		failures = append(failures, Failure{
			Kind:    FailureGeneratedContractInvalid,
			Path:    unsupportedPortablePath(graph.Portable, "skills"),
			Target:  target,
			Message: fmt.Sprintf("target %s declares portable skills support but does not declare managed skill artifacts", target),
		})
	}
	return failures
}

func portableSkillsManaged(profile platformmeta.PlatformProfile) bool {
	for _, spec := range profile.ManagedArtifacts {
		if spec.Kind == platformmeta.ManagedArtifactPortableSkills {
			return true
		}
	}
	return false
}

func validateUnsupportedTargetSurfaces(root, target string, report *Report) {
	profile, ok := platformmeta.Lookup(target)
	if !ok {
		return
	}
	for _, surface := range profile.SurfaceTiers {
		if surface.Tier != platformmeta.SurfaceTierUnsupported {
			continue
		}
		for _, path := range unsupportedSurfacePaths(root, target, surface.Kind, profile) {
			report.Failures = append(report.Failures, Failure{
				Kind:    FailureUnsupportedTargetKind,
				Path:    path,
				Target:  target,
				Message: fmt.Sprintf("target %s does not support authored surface %s", target, surface.Kind),
			})
		}
	}
}

func unsupportedPortablePath(portable pluginmanifest.PortableComponents, kind string) string {
	switch kind {
	case "skills":
		if len(portable.Paths("skills")) > 0 {
			return canonicalAuthoredPath("skills")
		}
		return canonicalAuthoredPath("skills")
	case "mcp_servers":
		if portable.MCP != nil && strings.TrimSpace(portable.MCP.Path) != "" {
			return canonicalAuthoredPath(portable.MCP.Path)
		}
		return canonicalAuthoredPath("mcp")
	default:
		return kind
	}
}

func unsupportedTargetKindPath(target string, tc pluginmanifest.TargetComponents, kind string) string {
	if path := strings.TrimSpace(tc.DocPath(kind)); path != "" {
		return canonicalAuthoredPath(path)
	}
	if len(tc.ComponentPaths(kind)) > 0 {
		return canonicalAuthoredPath(filepath.ToSlash(filepath.Join("targets", target, kind)))
	}
	return canonicalAuthoredPath(filepath.ToSlash(filepath.Join("targets", target, kind)))
}

func unsupportedSurfacePaths(root, target, kind string, profile platformmeta.PlatformProfile) []string {
	seen := map[string]struct{}{}
	for _, doc := range profile.NativeDocs {
		if doc.Kind != kind {
			continue
		}
		if fileExists(filepath.Join(root, doc.Path)) {
			seen[doc.Path] = struct{}{}
		}
	}
	dir := canonicalAuthoredPath(filepath.Join("targets", target, kind))
	if entries, err := os.ReadDir(filepath.Join(root, dir)); err == nil && len(entries) > 0 {
		seen[dir] = struct{}{}
	}
	out := make([]string, 0, len(seen))
	for path := range seen {
		out = append(out, path)
	}
	slices.Sort(out)
	return out
}

func validateTargetExtraDocs(root, target string, tc pluginmanifest.TargetComponents, report *Report) {
	profile, ok := platformmeta.Lookup(target)
	if !ok {
		return
	}
	for _, doc := range profile.NativeDocs {
		if doc.Role != platformmeta.NativeDocRoleExtra {
			continue
		}
		format := pluginmanifest.NativeDocFormatJSON
		if doc.Format == platformmeta.NativeDocTOML {
			format = pluginmanifest.NativeDocFormatTOML
		}
		label := target + " " + filepath.Base(doc.Path)
		validateTargetExtraDoc(root, target, tc.DocPath(doc.Kind), format, label, doc.ManagedKeys, report)
	}
}

func validateTargetExtraDoc(root, target, rel string, format pluginmanifest.NativeDocFormat, label string, managedPaths []string, report *Report) {
	if strings.TrimSpace(rel) == "" {
		return
	}
	doc, err := pluginmanifest.LoadNativeExtraDoc(root, rel, format)
	if err != nil {
		formatName := strings.ToUpper(string(format))
		report.Failures = append(report.Failures, Failure{
			Kind:    FailureManifestInvalid,
			Path:    rel,
			Target:  target,
			Message: fmt.Sprintf("%s %s is invalid %s: %v", label, rel, formatName, err),
		})
		return
	}
	if err := pluginmanifest.ValidateNativeExtraDocConflicts(doc, label, managedPaths); err != nil {
		report.Failures = append(report.Failures, Failure{
			Kind:    FailureManifestInvalid,
			Path:    rel,
			Target:  target,
			Message: err.Error(),
		})
	}
}

func applyAdapterDiagnostics(report *Report, diagnostics []platformexec.Diagnostic) {
	for _, diagnostic := range diagnostics {
		switch diagnostic.Severity {
		case platformexec.SeverityWarning:
			report.Warnings = append(report.Warnings, Warning{
				Kind:    mapAdapterWarningKind(diagnostic.Code),
				Path:    diagnostic.Path,
				Message: diagnostic.Message,
			})
		default:
			report.Failures = append(report.Failures, Failure{
				Kind:    mapAdapterFailureKind(diagnostic.Code),
				Path:    diagnostic.Path,
				Target:  diagnostic.Target,
				Message: diagnostic.Message,
			})
		}
	}
}

func mapAdapterFailureKind(code string) FailureKind {
	switch code {
	case platformexec.CodeGeneratedContractInvalid:
		return FailureGeneratedContractInvalid
	case platformexec.CodeEntrypointMismatch:
		return FailureEntrypointMismatch
	default:
		return FailureManifestInvalid
	}
}

func mapAdapterWarningKind(code string) WarningKind {
	switch code {
	case platformexec.CodeGeminiDirNameMismatch:
		return WarningGeminiDirNameMismatch
	case platformexec.CodeGeminiMCPCommandStyle:
		return WarningGeminiMCPCommandStyle
	case platformexec.CodeGeminiPolicyIgnored:
		return WarningGeminiPolicyIgnored
	default:
		return WarningManifestUnknownField
	}
}

func mapManifestWarningKind(kind pluginmanifest.WarningKind) WarningKind {
	switch kind {
	case pluginmanifest.WarningUnknownField:
		return WarningManifestUnknownField
	default:
		return WarningManifestUnknownField
	}
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func setOf(values []string) map[string]bool {
	out := make(map[string]bool, len(values))
	for _, value := range values {
		out[value] = true
	}
	return out
}
