package app

import "testing"

func TestNewBundleFetchGitHubClientUsesAPIBaseWhenProvided(t *testing.T) {
	t.Parallel()

	client := newBundleFetchGitHubClient(PluginBundleFetchOptions{
		GitHubAPIBase: " https://example.test/api/ ",
	})
	if got := client.BaseURL; got != "https://example.test/api/" {
		t.Fatalf("base URL = %q", got)
	}
}

func TestAttachBundleFetchHTTPClientNoopsOnNilClient(t *testing.T) {
	t.Parallel()

	client := newBundleFetchGitHubClient(PluginBundleFetchOptions{})
	attachBundleFetchHTTPClient(client, nil)
	if client.APIClient != nil || client.DLClient != nil {
		t.Fatalf("client = %#v", client)
	}
}
