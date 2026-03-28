package github

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/plugin-kit-ai/plugin-kit-ai/plugininstall/domain"
	"github.com/plugin-kit-ai/plugin-kit-ai/plugininstall/internal/httpconfig"
)

type releaseDTO struct {
	ID         int64      `json:"id"`
	TagName    string     `json:"tag_name"`
	Draft      bool       `json:"draft"`
	Prerelease bool       `json:"prerelease"`
	UploadURL  string     `json:"upload_url"`
	Assets     []assetDTO `json:"assets"`
}

type assetDTO struct {
	ID                 int64  `json:"id"`
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
	Size               int64  `json:"size"`
}

// GetReleaseByTag implements ports.ReleaseSource.
func (c *Client) GetReleaseByTag(ctx context.Context, owner, repo, tag string) (*domain.Release, error) {
	path := fmt.Sprintf("repos/%s/%s/releases/tags/%s", owner, repo, tag)
	return c.fetchRelease(ctx, path, fmt.Sprintf("release tag %q not found", tag), false)
}

// GetLatestRelease implements ports.ReleaseSource (GitHub non-prerelease latest).
func (c *Client) GetLatestRelease(ctx context.Context, owner, repo string) (*domain.Release, error) {
	path := fmt.Sprintf("repos/%s/%s/releases/latest", owner, repo)
	return c.fetchRelease(ctx, path, "no latest release found (GitHub has no published non-prerelease release)", false)
}

func (c *Client) fetchRelease(ctx context.Context, apiPath, notFoundDetail string, allowDraft bool) (*domain.Release, error) {
	if c.APIClient == nil {
		c.APIClient = httpconfig.APIClient()
	}
	u := fmt.Sprintf("%s/%s", strings.TrimSuffix(c.BaseURL, "/"), apiPath)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, domain.NewError(domain.ExitNetwork, "github api request: "+err.Error())
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	if c.APIVersion != "" {
		req.Header.Set("X-GitHub-Api-Version", c.APIVersion)
	}
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}

	resp, err := c.APIClient.Do(req)
	if err != nil {
		return nil, domain.NewError(domain.ExitNetwork, "github api: "+err.Error())
	}
	defer resp.Body.Close()
	maxJSON := c.releaseJSONMaxBytes()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxJSON+1))
	if err != nil {
		return nil, domain.NewError(domain.ExitNetwork, "github api: read body: "+err.Error())
	}
	if int64(len(body)) > maxJSON {
		return nil, domain.NewError(domain.ExitNetwork, fmt.Sprintf("github api: release response exceeds %d bytes", maxJSON))
	}
	if resp.StatusCode == http.StatusNotFound {
		return nil, domain.NewError(domain.ExitRelease, notFoundDetail)
	}
	if resp.StatusCode == http.StatusForbidden {
		return nil, domain.NewError(domain.ExitNetwork, "github api forbidden (set GITHUB_TOKEN or --github-token for private repos and higher rate limits)")
	}
	if resp.StatusCode != http.StatusOK {
		return nil, domain.NewError(domain.ExitNetwork, fmt.Sprintf("github api: status %s: %s", resp.Status, truncate(string(body), 200)))
	}
	var dto releaseDTO
	if err := json.Unmarshal(body, &dto); err != nil {
		return nil, domain.NewError(domain.ExitNetwork, "github api: invalid json: "+err.Error())
	}
	if dto.Draft && !allowDraft {
		return nil, domain.NewError(domain.ExitRelease, "release is draft (refused)")
	}
	return releaseFromDTO(dto), nil
}

func (c *Client) releaseJSONMaxBytes() int64 {
	if c.ReleaseJSONMaxBytes > 0 {
		return c.ReleaseJSONMaxBytes
	}
	return defaultReleaseJSONMaxBytes
}
