package app

import (
	"context"
	"fmt"
	neturl "net/url"
	"strings"

	"github.com/777genius/plugin-kit-ai/plugininstall/domain"
)

func resolveBundleRemoteSource(ctx context.Context, opts PluginBundleFetchOptions, deps bundleFetchDeps) (bundleRemoteSource, error) {
	if strings.TrimSpace(opts.URL) != "" {
		return resolveBundleURLSource(ctx, opts, deps.URLDownloader)
	}
	return resolveBundleGitHubSource(ctx, opts, deps.GitHub)
}

func resolveBundleURLSource(ctx context.Context, opts PluginBundleFetchOptions, downloader bundleHTTPDownloader) (bundleRemoteSource, error) {
	rawURL := strings.TrimSpace(opts.URL)
	parsed, err := neturl.Parse(rawURL)
	if err != nil {
		return bundleRemoteSource{}, fmt.Errorf("bundle fetch invalid URL: %w", err)
	}
	if parsed.Scheme != "https" {
		return bundleRemoteSource{}, fmt.Errorf("bundle fetch supports only https:// bundle URLs")
	}
	if !strings.HasSuffix(strings.ToLower(parsed.Path), ".tar.gz") {
		return bundleRemoteSource{}, fmt.Errorf("bundle fetch URL must point to a .tar.gz bundle")
	}
	if downloader == nil {
		return bundleRemoteSource{}, fmt.Errorf("bundle fetch downloader is required")
	}

	body, _, err := downloader.Download(ctx, rawURL)
	if err != nil {
		return bundleRemoteSource{}, err
	}
	sum, checksumSource, err := resolveURLBundleChecksum(ctx, downloader, rawURL, strings.TrimSpace(opts.SHA256))
	if err != nil {
		return bundleRemoteSource{}, err
	}
	if err := verifyBundleChecksum(body, sum); err != nil {
		return bundleRemoteSource{}, fmt.Errorf("bundle fetch checksum verification failed: %w", err)
	}
	return bundleRemoteSource{
		ArchiveBytes:   body,
		BundleSource:   rawURL,
		ChecksumSource: checksumSource,
	}, nil
}

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

func selectBundleReleaseAsset(rel *domain.Release, assetName, platform, runtime string) (*domain.Asset, error) {
	assetName = strings.TrimSpace(assetName)
	if assetName != "" {
		asset := findReleaseAsset(rel.Assets, assetName)
		if asset == nil {
			return nil, fmt.Errorf("bundle fetch release has no asset named %q", assetName)
		}
		return asset, nil
	}

	platform = strings.TrimSpace(platform)
	runtime = strings.TrimSpace(runtime)
	candidates := bundleReleaseCandidates(rel.Assets)
	if platform != "" && runtime != "" {
		suffix := fmt.Sprintf("_%s_%s_bundle.tar.gz", platform, runtime)
		matches := make([]domain.Asset, 0, len(candidates))
		for _, asset := range candidates {
			if strings.HasSuffix(asset.Name, suffix) {
				matches = append(matches, asset)
			}
		}
		if len(matches) == 1 {
			return &matches[0], nil
		}
		if len(matches) == 0 {
			return nil, fmt.Errorf("bundle fetch release has no bundle asset matching %s", suffix)
		}
		return nil, fmt.Errorf("bundle fetch release has multiple bundle assets matching %s: %s", suffix, joinAssetNames(matches))
	}

	if len(candidates) == 1 {
		return &candidates[0], nil
	}
	if len(candidates) == 0 {
		return nil, fmt.Errorf("bundle fetch release has no *_bundle.tar.gz assets")
	}
	return nil, fmt.Errorf("bundle fetch release bundle assets are ambiguous; use --asset-name or --platform with --runtime: %s", joinAssetNames(candidates))
}

func bundleReleaseCandidates(assets []domain.Asset) []domain.Asset {
	out := make([]domain.Asset, 0, len(assets))
	for _, asset := range assets {
		name := strings.ToLower(asset.Name)
		if strings.HasSuffix(name, "_bundle.tar.gz") {
			out = append(out, asset)
		}
	}
	return out
}

func joinAssetNames(assets []domain.Asset) string {
	names := make([]string, 0, len(assets))
	for _, asset := range assets {
		names = append(names, asset.Name)
	}
	return strings.Join(names, ", ")
}

func findReleaseAsset(assets []domain.Asset, name string) *domain.Asset {
	name = strings.TrimSpace(name)
	for i := range assets {
		if assets[i].Name == name {
			return &assets[i]
		}
	}
	return nil
}

func splitOwnerRepo(ref string) (string, string, error) {
	parts := strings.SplitN(strings.TrimSpace(ref), "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("bundle fetch GitHub mode requires owner/repo")
	}
	return parts[0], parts[1], nil
}

func bundleSidecarURL(rawURL string) (string, error) {
	u, err := neturl.Parse(rawURL)
	if err != nil {
		return "", fmt.Errorf("bundle fetch invalid URL: %w", err)
	}
	u.Path += ".sha256"
	return u.String(), nil
}

func validateFetchedBundleMatchesRequest(metadata exportMetadata, opts PluginBundleFetchOptions) error {
	if platform := strings.TrimSpace(opts.Platform); platform != "" && metadata.Platform != platform {
		return fmt.Errorf("bundle fetch selected asset does not match requested platform %q", platform)
	}
	if runtime := strings.TrimSpace(opts.Runtime); runtime != "" && metadata.Runtime != runtime {
		return fmt.Errorf("bundle fetch selected asset does not match requested runtime %q", runtime)
	}
	return nil
}
