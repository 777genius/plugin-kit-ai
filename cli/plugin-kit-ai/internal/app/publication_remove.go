package app

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/777genius/plugin-kit-ai/cli/internal/pluginmanifest"
	"github.com/777genius/plugin-kit-ai/cli/internal/publicationexec"
)

func (PluginService) publicationRemove(opts PluginPublicationRemoveOptions) (PluginPublicationRemoveResult, error) {
	ctx, err := loadPublicationContextForRemove(opts)
	if err != nil {
		return PluginPublicationRemoveResult{}, err
	}

	removedPackage := false
	if _, err := os.Stat(ctx.destPackageRoot()); err == nil {
		if !opts.DryRun {
			if err := os.RemoveAll(ctx.destPackageRoot()); err != nil {
				return PluginPublicationRemoveResult{}, err
			}
		}
		removedPackage = true
	} else if !os.IsNotExist(err) {
		return PluginPublicationRemoveResult{}, err
	}

	catalogRel, err := ctx.catalogArtifactPath()
	if err != nil {
		return PluginPublicationRemoveResult{}, err
	}
	removedCatalogEntry := false
	catalogFull := filepath.Join(ctx.dest, filepath.FromSlash(catalogRel))
	if existing, err := os.ReadFile(catalogFull); err == nil {
		updated, removed, err := publicationexec.RemoveCatalogArtifact(ctx.target, existing, ctx.graph.Manifest.Name)
		if err != nil {
			return PluginPublicationRemoveResult{}, err
		}
		if removed {
			if !opts.DryRun {
				if err := pluginmanifest.WriteArtifacts(ctx.dest, []pluginmanifest.Artifact{{
					RelPath: catalogRel,
					Content: updated,
				}}); err != nil {
					return PluginPublicationRemoveResult{}, err
				}
			}
			removedCatalogEntry = true
		}
	} else if !os.IsNotExist(err) {
		return PluginPublicationRemoveResult{}, err
	}

	lines := []string{
		fmt.Sprintf("Removed publication target: %s", ctx.target),
		fmt.Sprintf("Marketplace family: %s", ctx.channel.Family),
		fmt.Sprintf("Marketplace root: %s", filepath.Clean(ctx.dest)),
		fmt.Sprintf("Package root: %s", ctx.packageRoot),
		fmt.Sprintf("Mode: %s", publicationModeLabel(opts.DryRun)),
	}
	if removedPackage {
		lines = append(lines, "Package root action: remove")
	} else {
		lines = append(lines, "Package root action: no existing package root")
	}
	if removedCatalogEntry {
		lines = append(lines, fmt.Sprintf("Catalog artifact action: prune %s", catalogRel))
	} else {
		lines = append(lines, fmt.Sprintf("Catalog artifact action: no matching %q entry was present", ctx.graph.Manifest.Name))
	}
	lines = append(lines,
		"Next:",
		fmt.Sprintf("  plugin-kit-ai publication doctor %s --target %s --dest %s", ctx.root, ctx.target, ctx.dest),
		fmt.Sprintf("  review %s from the marketplace root if you keep additional plugins there", catalogRel),
	)
	return PluginPublicationRemoveResult{Lines: lines}, nil
}
