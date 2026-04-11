package app

import (
	"context"
	"fmt"
	"strings"
)

func bundleFetch(ctx context.Context, opts PluginBundleFetchOptions, deps bundleFetchDeps) (PluginBundleFetchResult, error) {
	if err := requireBundleFetchDest(opts); err != nil {
		return PluginBundleFetchResult{}, err
	}
	if err := validateBundleFetchMode(opts); err != nil {
		return PluginBundleFetchResult{}, err
	}
	source, err := resolveBundleRemoteSource(ctx, opts, deps)
	if err != nil {
		return PluginBundleFetchResult{}, err
	}
	metadata, installedPath, err := installFetchedBundleSource(source, opts)
	if err != nil {
		return PluginBundleFetchResult{}, err
	}
	return buildBundleFetchResult(metadata, source, installedPath), nil
}

func requireBundleFetchDest(opts PluginBundleFetchOptions) error {
	if strings.TrimSpace(opts.Dest) == "" {
		return fmt.Errorf("bundle fetch requires --dest")
	}
	return nil
}
