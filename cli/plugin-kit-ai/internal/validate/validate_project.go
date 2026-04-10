package validate

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/777genius/plugin-kit-ai/cli/internal/platformexec"
	"github.com/777genius/plugin-kit-ai/cli/internal/pluginmanifest"
	"github.com/777genius/plugin-kit-ai/cli/internal/pluginmodel"
	"github.com/777genius/plugin-kit-ai/cli/internal/targetcontracts"
	"github.com/777genius/plugin-kit-ai/sdk/platformmeta"
)

type validationContext struct {
	root                 string
	rawRequestedPlatform string
	requestedPlatform    string
	manifest             pluginmanifest.Manifest
	graph                pluginmanifest.PackageGraph
	report               Report
}

func newValidationContext(root, platform string, manifest pluginmanifest.Manifest) validationContext {
	return validationContext{
		root:                 root,
		rawRequestedPlatform: platform,
		requestedPlatform:    strings.TrimSpace(platform),
		manifest:             manifest,
		report: Report{
			Platform: strings.Join(manifest.EnabledTargets(), ","),
			Checks:   []string{"plugin_manifest", "package_graph", "publication", "generated_artifacts", "runtime"},
		},
	}
}

func validatePluginProject(root, platform string) (Report, error) {
	manifest, warnings, err := pluginmanifest.LoadWithWarnings(root)
	if err != nil {
		msg := err.Error()
		return Report{}, invalidProjectReport(FailureManifestInvalid, extractFailurePath(msg), msg)
	}

	ctx := newValidationContext(root, platform, manifest)
	ctx.addManifestWarnings(warnings)
	ctx.validateRequestedPlatform()

	graph, _, err := pluginmanifest.Discover(root)
	if err != nil {
		msg := err.Error()
		ctx.addFailure(Failure{
			Kind:    FailureManifestInvalid,
			Path:    extractFailurePath(msg),
			Message: msg,
		})
		return normalizeReport(ctx.report), nil
	}

	ctx.graph = graph
	ctx.validateSourceFiles()
	ctx.validateTargets()
	ctx.validateDrift()
	ctx.validateRuntime()
	return normalizeReport(ctx.report), nil
}

func (ctx *validationContext) addWarning(warning Warning) {
	ctx.report.Warnings = append(ctx.report.Warnings, warning)
}

func (ctx *validationContext) addFailure(failure Failure) {
	ctx.report.Failures = append(ctx.report.Failures, failure)
}

func (ctx *validationContext) addManifestWarnings(warnings []pluginmanifest.Warning) {
	for _, warning := range warnings {
		ctx.addWarning(Warning{
			Kind:    mapManifestWarningKind(warning.Kind),
			Path:    warning.Path,
			Message: warning.Message,
		})
	}
}

func (ctx *validationContext) validateRequestedPlatform() {
	if ctx.requestedPlatform == "" || slices.Contains(ctx.manifest.EnabledTargets(), ctx.requestedPlatform) {
		return
	}
	ctx.addFailure(Failure{
		Kind:    FailureManifestInvalid,
		Path:    filepath.Join(pluginmodel.SourceDirName, pluginmanifest.FileName),
		Message: fmt.Sprintf("plugin.yaml does not enable target %q", ctx.rawRequestedPlatform),
	})
}

func (ctx *validationContext) validateSourceFiles() {
	for _, rel := range ctx.graph.SourceFiles {
		if _, err := os.Stat(filepath.Join(ctx.root, rel)); err != nil {
			ctx.addFailure(Failure{
				Kind:    FailureSourceFileMissing,
				Path:    rel,
				Message: "referenced source file missing: " + rel,
			})
		}
	}
}

func (ctx *validationContext) validateTargets() {
	for _, targetName := range ctx.manifest.EnabledTargets() {
		ctx.validateTarget(targetName)
	}
}

func (ctx *validationContext) validateTarget(targetName string) {
	if rule, ok := LookupRule(targetName); ok {
		for _, rel := range rule.ForbiddenFiles {
			if !fileExists(filepath.Join(ctx.root, rel)) {
				continue
			}
			ctx.addFailure(Failure{
				Kind:    FailureForbiddenFilePresent,
				Path:    rel,
				Target:  targetName,
				Message: fmt.Sprintf("target %s forbids %s", targetName, rel),
			})
		}
	}

	entry, ok := targetcontracts.Lookup(targetName)
	if !ok {
		return
	}
	profile, _ := platformmeta.Lookup(targetName)
	tc := ctx.graph.Targets[targetName]
	supportedPortable := setOf(entry.PortableComponentKinds)
	if len(ctx.graph.Portable.Paths("skills")) > 0 && !supportedPortable["skills"] {
		ctx.addFailure(Failure{
			Kind:    FailureUnsupportedTargetKind,
			Path:    unsupportedPortablePath(ctx.graph.Portable, "skills"),
			Target:  targetName,
			Message: fmt.Sprintf("target %s does not support portable component kind skills", targetName),
		})
	}
	if ctx.graph.Portable.MCP != nil && !supportedPortable["mcp_servers"] {
		ctx.addFailure(Failure{
			Kind:    FailureUnsupportedTargetKind,
			Path:    unsupportedPortablePath(ctx.graph.Portable, "mcp_servers"),
			Target:  targetName,
			Message: fmt.Sprintf("target %s does not support portable component kind mcp_servers", targetName),
		})
	}
	ctx.report.Failures = append(ctx.report.Failures, validatePortableContractCoverage(targetName, profile, ctx.graph)...)

	supportedNative := setOf(entry.TargetComponentKinds)
	for _, kind := range pluginmanifest.DiscoveredTargetKinds(tc) {
		if supportedNative[kind] {
			continue
		}
		ctx.addFailure(Failure{
			Kind:    FailureUnsupportedTargetKind,
			Path:    unsupportedTargetKindPath(targetName, tc, kind),
			Target:  targetName,
			Message: fmt.Sprintf("target %s does not support target-native component kind %s", targetName, kind),
		})
	}

	validateTargetExtraDocs(ctx.root, targetName, tc, &ctx.report)
	validateUnsupportedTargetSurfaces(ctx.root, targetName, &ctx.report)
	if adapter, ok := platformexec.Lookup(targetName); ok {
		diagnostics, err := adapter.Validate(ctx.root, ctx.graph, tc)
		if err != nil {
			msg := err.Error()
			ctx.addFailure(Failure{
				Kind:    FailureManifestInvalid,
				Path:    extractFailurePath(msg),
				Target:  targetName,
				Message: msg,
			})
			return
		}
		applyAdapterDiagnostics(&ctx.report, diagnostics)
	}
}

func (ctx *validationContext) validateDrift() {
	drift, err := pluginmanifest.Drift(ctx.root, targetOrAll(ctx.requestedPlatform))
	if err != nil {
		msg := err.Error()
		ctx.addFailure(Failure{
			Kind:    FailureGeneratedContractInvalid,
			Path:    extractFailurePath(msg),
			Message: msg,
		})
		return
	}
	for _, rel := range drift {
		ctx.addFailure(Failure{
			Kind:    FailureGeneratedContractInvalid,
			Path:    rel,
			Message: "generated artifact drift: " + rel,
		})
	}
}

func (ctx *validationContext) validateRuntime() {
	validatePluginRuntimeFiles(ctx.root, ctx.manifest, ctx.graph.Launcher, &ctx.report)
}
