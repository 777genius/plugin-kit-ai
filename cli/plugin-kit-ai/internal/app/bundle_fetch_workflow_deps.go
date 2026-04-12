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
	client := newBundleFetchGitHubClient(opts)
	attachBundleFetchHTTPClient(client, customHTTPClient)
	return bundleFetchDeps{
		URLDownloader: downloader,
		GitHub:        client,
	}, nil
}
