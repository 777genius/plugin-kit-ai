package app

import (
	"context"
	"fmt"
	"strings"
)

func resolveBundleRemoteSource(ctx context.Context, opts PluginBundleFetchOptions, deps bundleFetchDeps) (bundleRemoteSource, error) {
	if strings.TrimSpace(opts.URL) != "" {
		return resolveBundleURLSource(ctx, opts, deps.URLDownloader)
	}
	return resolveBundleGitHubSource(ctx, opts, deps.GitHub)
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
