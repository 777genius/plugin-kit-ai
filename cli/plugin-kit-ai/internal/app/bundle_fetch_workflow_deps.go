package app

import (
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
