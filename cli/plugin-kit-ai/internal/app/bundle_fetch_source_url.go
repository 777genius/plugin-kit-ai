package app

import (
	"context"
	"fmt"
	neturl "net/url"
	"strings"
)

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

func bundleSidecarURL(rawURL string) (string, error) {
	u, err := neturl.Parse(rawURL)
	if err != nil {
		return "", fmt.Errorf("bundle fetch invalid URL: %w", err)
	}
	u.Path += ".sha256"
	return u.String(), nil
}
