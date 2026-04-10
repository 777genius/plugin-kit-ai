package app

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/777genius/plugin-kit-ai/cli/internal/pluginmanifest"
	"github.com/777genius/plugin-kit-ai/cli/internal/pluginmodel"
	"github.com/777genius/plugin-kit-ai/cli/internal/publicationexec"
	"github.com/777genius/plugin-kit-ai/cli/internal/publicationmodel"
	"github.com/777genius/plugin-kit-ai/cli/internal/publishschema"
)

type publicationContext struct {
	root             string
	target           string
	dest             string
	packageRoot      string
	graph            pluginmanifest.PackageGraph
	inspection       pluginmanifest.Inspection
	publication      publicationmodel.Model
	publicationState publishschema.State
	channel          publicationmodel.Channel
}

func loadPublicationContextForMaterialize(opts PluginPublicationMaterializeOptions) (publicationContext, error) {
	root := strings.TrimSpace(opts.Root)
	if root == "" {
		root = "."
	}
	target := strings.TrimSpace(opts.Target)
	switch target {
	case "codex-package", "claude":
	default:
		return publicationContext{}, fmt.Errorf("publication materialize supports only %q or %q", "codex-package", "claude")
	}
	dest := strings.TrimSpace(opts.Dest)
	if dest == "" {
		return publicationContext{}, fmt.Errorf("publication materialize requires --dest")
	}

	graph, _, err := pluginmanifest.Discover(root)
	if err != nil {
		return publicationContext{}, err
	}
	if _, err := graph.Manifest.SelectedTargets(target); err != nil {
		return publicationContext{}, err
	}
	publicationState, err := publishschema.DiscoverInLayout(root, pluginmodel.SourceDirName)
	if err != nil {
		return publicationContext{}, err
	}
	inspection, _, err := pluginmanifest.Inspect(root, target)
	if err != nil {
		return publicationContext{}, err
	}
	publication := inspection.Publication
	if _, ok := publicationPackageForTarget(publication, target); !ok {
		return publicationContext{}, fmt.Errorf("target %s is not publication-capable", target)
	}
	channel, ok := publicationChannelForTarget(publication, target)
	if !ok {
		return publicationContext{}, fmt.Errorf("target %s requires authored publication channel metadata under src/publish/...", target)
	}
	packageRoot, err := normalizePackageRoot(opts.PackageRoot, graph.Manifest.Name)
	if err != nil {
		return publicationContext{}, err
	}

	return publicationContext{
		root:             root,
		target:           target,
		dest:             dest,
		packageRoot:      packageRoot,
		graph:            graph,
		inspection:       inspection,
		publication:      publication,
		publicationState: publicationState,
		channel:          channel,
	}, nil
}

func loadPublicationContextForRemove(opts PluginPublicationRemoveOptions) (publicationContext, error) {
	root := strings.TrimSpace(opts.Root)
	if root == "" {
		root = "."
	}
	target := strings.TrimSpace(opts.Target)
	switch target {
	case "codex-package", "claude":
	default:
		return publicationContext{}, fmt.Errorf("publication remove supports only %q or %q", "codex-package", "claude")
	}
	dest := strings.TrimSpace(opts.Dest)
	if dest == "" {
		return publicationContext{}, fmt.Errorf("publication remove requires --dest")
	}

	graph, _, err := pluginmanifest.Discover(root)
	if err != nil {
		return publicationContext{}, err
	}
	if _, err := graph.Manifest.SelectedTargets(target); err != nil {
		return publicationContext{}, err
	}
	inspection, _, err := pluginmanifest.Inspect(root, target)
	if err != nil {
		return publicationContext{}, err
	}
	publication := inspection.Publication
	if _, ok := publicationPackageForTarget(publication, target); !ok {
		return publicationContext{}, fmt.Errorf("target %s is not publication-capable", target)
	}
	channel, ok := publicationChannelForTarget(publication, target)
	if !ok {
		return publicationContext{}, fmt.Errorf("target %s requires authored publication channel metadata under publish/...", target)
	}
	packageRoot, err := normalizePackageRoot(opts.PackageRoot, graph.Manifest.Name)
	if err != nil {
		return publicationContext{}, err
	}

	return publicationContext{
		root:        root,
		target:      target,
		dest:        dest,
		packageRoot: packageRoot,
		graph:       graph,
		inspection:  inspection,
		publication: publication,
		channel:     channel,
	}, nil
}

func loadPublicationContextForVerify(opts PluginPublicationVerifyRootOptions) (publicationContext, error) {
	root := strings.TrimSpace(opts.Root)
	if root == "" {
		root = "."
	}
	target := strings.TrimSpace(opts.Target)
	switch target {
	case "codex-package", "claude":
	default:
		return publicationContext{}, fmt.Errorf("publication doctor local-root verification supports only %q or %q", "codex-package", "claude")
	}
	dest := strings.TrimSpace(opts.Dest)
	if dest == "" {
		return publicationContext{}, fmt.Errorf("publication doctor local-root verification requires --dest")
	}

	graph, _, err := pluginmanifest.Discover(root)
	if err != nil {
		return publicationContext{}, err
	}
	if _, err := graph.Manifest.SelectedTargets(target); err != nil {
		return publicationContext{}, err
	}
	publicationState, err := publishschema.DiscoverInLayout(root, pluginmodel.SourceDirName)
	if err != nil {
		return publicationContext{}, err
	}
	inspection, _, err := pluginmanifest.Inspect(root, target)
	if err != nil {
		return publicationContext{}, err
	}
	packageRoot, err := normalizePackageRoot(opts.PackageRoot, graph.Manifest.Name)
	if err != nil {
		return publicationContext{}, err
	}

	return publicationContext{
		root:             root,
		target:           target,
		dest:             dest,
		packageRoot:      packageRoot,
		graph:            graph,
		inspection:       inspection,
		publication:      inspection.Publication,
		publicationState: publicationState,
	}, nil
}

func (ctx publicationContext) expectedMaterializedPackageArtifacts() ([]pluginmanifest.Artifact, pluginmanifest.RenderResult, error) {
	generated, err := pluginmanifest.Generate(ctx.root, ctx.target)
	if err != nil {
		return nil, pluginmanifest.RenderResult{}, err
	}
	managedPaths, err := inspectionManagedPathsForTarget(ctx.inspection, ctx.target)
	if err != nil {
		return nil, pluginmanifest.RenderResult{}, err
	}
	managedPaths = append(managedPaths, ctx.graph.Portable.Paths("skills")...)
	managedPaths = slices.Compact(sortedSlashPaths(managedPaths))
	packageFiles, err := materializedPackageArtifacts(ctx.root, ctx.packageRoot, managedPaths, generated)
	if err != nil {
		return nil, pluginmanifest.RenderResult{}, err
	}
	return packageFiles, generated, nil
}

func (ctx publicationContext) renderLocalCatalogArtifact() (pluginmanifest.Artifact, error) {
	return publicationexec.RenderLocalCatalogArtifact(ctx.graph, ctx.publicationState, ctx.target, "./"+ctx.packageRoot)
}

func (ctx publicationContext) catalogArtifactPath() (string, error) {
	return publicationexec.CatalogArtifactPath(ctx.target)
}

func (ctx publicationContext) destPackageRoot() string {
	return filepath.Join(ctx.dest, filepath.FromSlash(ctx.packageRoot))
}
