package app

import (
	"context"
	"fmt"
	"os"
	"strings"

	gh "github.com/777genius/plugin-kit-ai/plugininstall/adapters/github"
)

func defaultBundleFetchDeps(opts PluginBundleFetchOptions) (bundleFetchDeps, error) {
	downloader, customHTTPClient, err := newDefaultBundleFetchHTTPClient()
	if err != nil {
		return bundleFetchDeps{}, err
	}
	client := gh.NewClient(strings.TrimSpace(opts.GitHubToken))
	if base := strings.TrimSpace(opts.GitHubAPIBase); base != "" {
		client.BaseURL = base
	}
	if customHTTPClient != nil {
		client.APIClient = customHTTPClient
		client.DLClient = customHTTPClient
	}
	return bundleFetchDeps{
		URLDownloader: downloader,
		GitHub:        client,
	}, nil
}

func bundleFetch(ctx context.Context, opts PluginBundleFetchOptions, deps bundleFetchDeps) (PluginBundleFetchResult, error) {
	if strings.TrimSpace(opts.Dest) == "" {
		return PluginBundleFetchResult{}, fmt.Errorf("bundle fetch requires --dest")
	}
	if err := validateBundleFetchMode(opts); err != nil {
		return PluginBundleFetchResult{}, err
	}
	source, err := resolveBundleRemoteSource(ctx, opts, deps)
	if err != nil {
		return PluginBundleFetchResult{}, err
	}
	archivePath, cleanup, err := writeTempBundleArchive(source.ArchiveBytes)
	if err != nil {
		return PluginBundleFetchResult{}, err
	}
	defer cleanup()

	metadata, err := inspectBundleArchive(archivePath)
	if err != nil {
		return PluginBundleFetchResult{}, err
	}
	if err := validateBundleMetadata(metadata); err != nil {
		return PluginBundleFetchResult{}, err
	}
	if err := validateFetchedBundleMatchesRequest(metadata, opts); err != nil {
		return PluginBundleFetchResult{}, err
	}

	installedPath, err := installBundleArchive(archivePath, opts.Dest, opts.Force)
	if err != nil {
		return PluginBundleFetchResult{}, err
	}

	lines := []string{
		fmt.Sprintf("Bundle: plugin=%s platform=%s runtime=%s manager=%s", metadata.PluginName, metadata.Platform, metadata.Runtime, displayBundleManager(metadata.Manager)),
		"Bundle source: " + source.BundleSource,
		"Checksum source: " + source.ChecksumSource,
		"Installed path: " + installedPath,
	}
	if strings.TrimSpace(metadata.RuntimeRequirement) != "" {
		lines = append(lines, "Runtime requirement: "+metadata.RuntimeRequirement)
	}
	if strings.TrimSpace(metadata.RuntimeInstallHint) != "" {
		lines = append(lines, "Runtime install hint: "+metadata.RuntimeInstallHint)
	}
	lines = append(lines, "Next:")
	for _, step := range resolvedBundleNext(metadata, installedPath) {
		lines = append(lines, "  "+step)
	}
	return PluginBundleFetchResult{Lines: lines}, nil
}

func validateBundleFetchMode(opts PluginBundleFetchOptions) error {
	if strings.TrimSpace(opts.URL) != "" {
		if strings.TrimSpace(opts.Ref) != "" {
			return fmt.Errorf("bundle fetch accepts either --url or owner/repo, not both")
		}
		if strings.TrimSpace(opts.Tag) != "" || opts.Latest {
			return fmt.Errorf("bundle fetch URL mode does not accept --tag or --latest")
		}
		if strings.TrimSpace(opts.AssetName) != "" || strings.TrimSpace(opts.Platform) != "" || strings.TrimSpace(opts.Runtime) != "" {
			return fmt.Errorf("bundle fetch URL mode does not use --asset-name, --platform, or --runtime")
		}
		return nil
	}
	if strings.TrimSpace(opts.Ref) == "" {
		return fmt.Errorf("bundle fetch requires --url or owner/repo")
	}
	if opts.Latest && strings.TrimSpace(opts.Tag) != "" {
		return fmt.Errorf("bundle fetch does not use --tag together with --latest")
	}
	if !opts.Latest && strings.TrimSpace(opts.Tag) == "" {
		return fmt.Errorf("bundle fetch GitHub mode requires --tag or --latest")
	}
	if (strings.TrimSpace(opts.Platform) == "") != (strings.TrimSpace(opts.Runtime) == "") {
		return fmt.Errorf("bundle fetch GitHub mode requires --platform and --runtime together")
	}
	return nil
}

func writeTempBundleArchive(body []byte) (string, func(), error) {
	f, err := os.CreateTemp("", ".plugin-kit-ai-bundle-fetch-*.tar.gz")
	if err != nil {
		return "", nil, err
	}
	if _, err := f.Write(body); err != nil {
		_ = f.Close()
		_ = os.Remove(f.Name())
		return "", nil, err
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(f.Name())
		return "", nil, err
	}
	cleanup := func() { _ = os.Remove(f.Name()) }
	return f.Name(), cleanup, nil
}
