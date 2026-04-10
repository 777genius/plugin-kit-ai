package app

import (
	"context"
	"fmt"
	"strings"

	"github.com/777genius/plugin-kit-ai/plugininstall/domain"
)

func resolveBundleGitHubSource(ctx context.Context, opts PluginBundleFetchOptions, source bundleGitHubSource) (bundleRemoteSource, error) {
	if source == nil {
		return bundleRemoteSource{}, fmt.Errorf("bundle fetch GitHub source is required")
	}
	owner, repo, err := splitOwnerRepo(opts.Ref)
	if err != nil {
		return bundleRemoteSource{}, err
	}
	var rel *domain.Release
	if opts.Latest {
		rel, err = source.GetLatestRelease(ctx, owner, repo)
	} else {
		rel, err = source.FindReleaseByTag(ctx, owner, repo, strings.TrimSpace(opts.Tag))
	}
	if err != nil {
		return bundleRemoteSource{}, err
	}
	asset, err := selectBundleReleaseAsset(rel, opts.AssetName, opts.Platform, opts.Runtime)
	if err != nil {
		return bundleRemoteSource{}, err
	}
	body, _, err := source.DownloadAsset(ctx, asset.BrowserDownloadURL)
	if err != nil {
		return bundleRemoteSource{}, err
	}
	sum, checksumSource, err := resolveGitHubBundleChecksum(ctx, source, rel, *asset)
	if err != nil {
		return bundleRemoteSource{}, err
	}
	if err := verifyBundleChecksum(body, sum); err != nil {
		return bundleRemoteSource{}, fmt.Errorf("bundle fetch checksum verification failed: %w", err)
	}

	releaseRef := strings.TrimSpace(rel.TagName)
	if releaseRef == "" {
		releaseRef = strings.TrimSpace(opts.Tag)
	}
	refLabel := owner + "/" + repo
	if releaseRef != "" {
		refLabel += "@" + releaseRef
	}
	if opts.Latest {
		refLabel += " (latest)"
	} else {
		refLabel += " (tag)"
	}
	return bundleRemoteSource{
		ArchiveBytes:   body,
		BundleSource:   fmt.Sprintf("github release %s asset=%s", refLabel, asset.Name),
		ChecksumSource: checksumSource,
	}, nil
}

func splitOwnerRepo(ref string) (string, string, error) {
	parts := strings.SplitN(strings.TrimSpace(ref), "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("bundle fetch GitHub mode requires owner/repo")
	}
	return parts[0], parts[1], nil
}
