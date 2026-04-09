package pluginmanifest

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	sourceresolver "github.com/777genius/plugin-kit-ai/install/integrationctl/adapters/source"
	"github.com/777genius/plugin-kit-ai/install/integrationctl/domain"
	"github.com/777genius/plugin-kit-ai/install/integrationctl/ports"
	"github.com/777genius/plugin-kit-ai/sdk/platformmeta"
)

type CompatibilityStatus string

const (
	CompatibilityFull        CompatibilityStatus = "full"
	CompatibilityPartial     CompatibilityStatus = "partial"
	CompatibilityUnsupported CompatibilityStatus = "unsupported"
)

type SourceCompatibility struct {
	Target           string              `json:"target"`
	Status           CompatibilityStatus `json:"status"`
	SupportedKinds   []string            `json:"supported_kinds"`
	UnsupportedKinds []string            `json:"unsupported_kinds"`
	Notes            []string            `json:"notes,omitempty"`
}

type SourceInspection struct {
	RequestedSource     string                `json:"requested_source"`
	ResolvedSource      string                `json:"resolved_source"`
	SourceKind          string                `json:"source_kind"`
	SourceDigest        string                `json:"source_digest"`
	CanonicalPackage    bool                  `json:"canonical_package"`
	ImportSource        string                `json:"import_source,omitempty"`
	DetectedImportKinds []string              `json:"detected_import_kinds,omitempty"`
	DroppedKinds        []string              `json:"dropped_kinds,omitempty"`
	OriginTargets       []string              `json:"origin_targets"`
	Inspection          Inspection            `json:"inspection"`
	Compatibility       []SourceCompatibility `json:"compatibility"`
}

func InspectSource(sourceRef string, from string, target string, includeUserScope bool) (SourceInspection, []Warning, error) {
	resolved, cleanup, err := resolveSourceRef(sourceRef)
	if err != nil {
		return SourceInspection{}, nil, err
	}
	defer cleanup()

	if isPackageStandardSource(resolved.LocalPath) {
		return inspectCanonicalSource(sourceRef, resolved, target)
	}
	return inspectImportedSource(sourceRef, resolved, from, target, includeUserScope)
}

func ImportSource(root string, sourceRef string, from string, force bool, includeUserScope bool) (Manifest, []Warning, error) {
	resolved, cleanup, err := resolveSourceRef(sourceRef)
	if err != nil {
		return Manifest{}, nil, err
	}
	defer cleanup()
	if isPackageStandardSource(resolved.LocalPath) {
		return Manifest{}, nil, fmt.Errorf("canonical package sources are already in package-standard layout; clone or sync the repo directly instead of import --source")
	}
	prepared, err := prepareImportFromRoot(resolved.LocalPath, from, includeUserScope)
	if err != nil {
		return Manifest{}, prepared.Warnings, err
	}
	if err := writePreparedImport(root, prepared, force); err != nil {
		return prepared.Manifest, prepared.Warnings, err
	}
	return prepared.Manifest, prepared.Warnings, nil
}

func inspectCanonicalSource(sourceRef string, resolved ports.ResolvedSource, target string) (SourceInspection, []Warning, error) {
	state, err := discoverPackageState(resolved.LocalPath)
	if err != nil {
		return SourceInspection{}, nil, err
	}
	inspection, err := inspectPackageContext(packageContext{
		root:            resolved.LocalPath,
		layout:          state.layout,
		graph:           state.graph,
		publication:     state.publication,
		selectedTargets: state.graph.Manifest.EnabledTargets(),
	})
	if err != nil {
		return SourceInspection{}, state.warnings, err
	}
	compatibility, err := buildSourceCompatibility(state.graph, state.graph.Manifest.EnabledTargets(), target, nil)
	if err != nil {
		return SourceInspection{}, state.warnings, err
	}
	return SourceInspection{
		RequestedSource:  sourceRef,
		ResolvedSource:   resolved.Resolved.Value,
		SourceKind:       resolved.Kind,
		SourceDigest:     resolved.SourceDigest,
		CanonicalPackage: true,
		OriginTargets:    state.graph.Manifest.EnabledTargets(),
		Inspection:       inspection,
		Compatibility:    compatibility,
	}, state.warnings, nil
}

func inspectImportedSource(sourceRef string, resolved ports.ResolvedSource, from string, target string, includeUserScope bool) (SourceInspection, []Warning, error) {
	prepared, err := prepareImportFromRoot(resolved.LocalPath, from, includeUserScope)
	if err != nil {
		return SourceInspection{}, prepared.Warnings, err
	}
	tmpRoot, cleanup, err := materializePreparedImport(prepared)
	if err != nil {
		return SourceInspection{}, prepared.Warnings, err
	}
	defer cleanup()
	state, err := discoverPackageState(tmpRoot)
	if err != nil {
		return SourceInspection{}, prepared.Warnings, err
	}
	inspection, err := inspectPackageContext(packageContext{
		root:            tmpRoot,
		layout:          state.layout,
		graph:           state.graph,
		publication:     state.publication,
		selectedTargets: state.graph.Manifest.EnabledTargets(),
	})
	if err != nil {
		return SourceInspection{}, append(prepared.Warnings, state.warnings...), err
	}
	compatibility, err := buildSourceCompatibility(state.graph, []string{prepared.ImportSource}, target, prepared.DroppedKinds)
	if err != nil {
		return SourceInspection{}, append(prepared.Warnings, state.warnings...), err
	}
	warnings := append([]Warning{}, prepared.Warnings...)
	warnings = append(warnings, state.warnings...)
	return SourceInspection{
		RequestedSource:     sourceRef,
		ResolvedSource:      resolved.Resolved.Value,
		SourceKind:          resolved.Kind,
		SourceDigest:        resolved.SourceDigest,
		CanonicalPackage:    false,
		ImportSource:        prepared.ImportSource,
		DetectedImportKinds: prepared.DetectedKinds,
		DroppedKinds:        prepared.DroppedKinds,
		OriginTargets:       []string{prepared.ImportSource},
		Inspection:          inspection,
		Compatibility:       compatibility,
	}, warnings, nil
}

func buildSourceCompatibility(graph PackageGraph, originTargets []string, target string, droppedKinds []string) ([]SourceCompatibility, error) {
	selectedTargets, err := compatibilityTargets(target)
	if err != nil {
		return nil, err
	}
	portableKinds := sourcePortableKinds(graph)
	droppedKinds = uniqueSortedKinds(droppedKinds)
	allOriginNativeKinds := uniqueSortedKinds(append(sourceNativeKinds(graph, originTargets), droppedKinds...))
	originSet := setOf(originTargets)
	out := make([]SourceCompatibility, 0, len(selectedTargets))
	for _, candidate := range selectedTargets {
		profile, ok := platformmeta.Lookup(candidate)
		if !ok {
			return nil, fmt.Errorf("unsupported target %q", candidate)
		}
		supported := intersectKinds(portableKinds, profile.Contract.PortableComponentKinds)
		unsupported := differenceKinds(portableKinds, profile.Contract.PortableComponentKinds)
		if originSet[candidate] {
			supported = append(supported, sourceNativeKinds(graph, []string{candidate})...)
			unsupported = append(unsupported, droppedKinds...)
		} else {
			unsupported = append(unsupported, allOriginNativeKinds...)
		}
		supported = uniqueSortedKinds(supported)
		unsupported = uniqueSortedKinds(unsupported)
		status := CompatibilityFull
		switch {
		case len(unsupported) == 0:
			status = CompatibilityFull
		case len(supported) > 0:
			status = CompatibilityPartial
		case len(portableKinds) == 0 && len(allOriginNativeKinds) == 0:
			status = CompatibilityFull
		default:
			status = CompatibilityUnsupported
		}
		notes := compatibilityNotes(originTargets, candidate, status, supported, unsupported, droppedKinds)
		out = append(out, SourceCompatibility{
			Target:           candidate,
			Status:           status,
			SupportedKinds:   supported,
			UnsupportedKinds: unsupported,
			Notes:            notes,
		})
	}
	return out, nil
}

func compatibilityNotes(originTargets []string, candidate string, status CompatibilityStatus, supported, unsupported, droppedKinds []string) []string {
	var notes []string
	originTargets = uniqueSortedKinds(originTargets)
	if !slices.Contains(originTargets, candidate) && len(unsupported) > 0 {
		notes = append(notes, fmt.Sprintf("target-native surfaces from %s do not project to %s without an overlay", strings.Join(originTargets, ","), candidate))
	}
	if len(droppedKinds) > 0 {
		notes = append(notes, fmt.Sprintf("source normalization dropped unsupported canonical surfaces: %s", strings.Join(droppedKinds, ",")))
	}
	switch status {
	case CompatibilityPartial:
		notes = append(notes, "installable with degraded surface coverage")
	case CompatibilityUnsupported:
		if len(supported) == 0 && len(unsupported) > 0 {
			notes = append(notes, "no functional source surfaces map to this target")
		}
	}
	return notes
}

func compatibilityTargets(target string) ([]string, error) {
	target = normalizeTarget(target)
	switch target {
	case "", "all":
		return platformmeta.IDs(), nil
	default:
		if _, ok := platformmeta.Lookup(target); !ok {
			return nil, fmt.Errorf("unsupported target %q", target)
		}
		return []string{target}, nil
	}
}

func sourcePortableKinds(graph PackageGraph) []string {
	var kinds []string
	if len(graph.Portable.Paths("skills")) > 0 {
		kinds = append(kinds, "skills")
	}
	if graph.Portable.MCP != nil {
		kinds = append(kinds, "mcp_servers")
	}
	return uniqueSortedKinds(kinds)
}

func sourceNativeKinds(graph PackageGraph, targets []string) []string {
	var kinds []string
	for _, target := range targets {
		state, ok := graph.Targets[target]
		if !ok {
			continue
		}
		kinds = append(kinds, discoveredTargetKinds(state)...)
	}
	return uniqueSortedKinds(kinds)
}

func intersectKinds(actual, allowed []string) []string {
	allowedSet := setOf(allowed)
	var out []string
	for _, item := range actual {
		if allowedSet[item] {
			out = append(out, item)
		}
	}
	return uniqueSortedKinds(out)
}

func differenceKinds(actual, allowed []string) []string {
	allowedSet := setOf(allowed)
	var out []string
	for _, item := range actual {
		if !allowedSet[item] {
			out = append(out, item)
		}
	}
	return uniqueSortedKinds(out)
}

func uniqueSortedKinds(items []string) []string {
	if len(items) == 0 {
		return nil
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		out = append(out, item)
	}
	if len(out) == 0 {
		return nil
	}
	slices.Sort(out)
	return slices.Compact(out)
}

func materializePreparedImport(prepared preparedImport) (string, func(), error) {
	root, err := os.MkdirTemp("", "plugin-kit-ai-source-*")
	if err != nil {
		return "", nil, err
	}
	if err := writePreparedImport(root, prepared, true); err != nil {
		_ = os.RemoveAll(root)
		return "", nil, err
	}
	return root, func() { _ = os.RemoveAll(root) }, nil
}

func resolveSourceRef(sourceRef string) (ports.ResolvedSource, func(), error) {
	sourceRef = strings.TrimSpace(sourceRef)
	if sourceRef == "" {
		return ports.ResolvedSource{}, nil, fmt.Errorf("source is required")
	}
	resolved, err := sourceresolver.Resolver{}.Resolve(context.Background(), domain.IntegrationRef{Raw: sourceRef})
	if err != nil {
		return ports.ResolvedSource{}, nil, err
	}
	return resolved, func() {
		if strings.TrimSpace(resolved.CleanupPath) != "" {
			_ = os.RemoveAll(resolved.CleanupPath)
		}
	}, nil
}

func isPackageStandardSource(root string) bool {
	layout, err := detectAuthoredLayout(root)
	if err != nil {
		return false
	}
	info, err := os.Stat(filepath.Join(root, layout.Path(FileName)))
	return err == nil && !info.IsDir()
}
