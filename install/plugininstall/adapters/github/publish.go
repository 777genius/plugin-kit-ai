package github

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	neturl "net/url"
	"strings"

	"github.com/plugin-kit-ai/plugin-kit-ai/plugininstall/domain"
	"github.com/plugin-kit-ai/plugin-kit-ai/plugininstall/internal/httpconfig"
)

// FindReleaseByTag returns a release for publishing and allows draft releases.
func (c *Client) FindReleaseByTag(ctx context.Context, owner, repo, tag string) (*domain.Release, error) {
	path := fmt.Sprintf("repos/%s/%s/releases/tags/%s", owner, repo, tag)
	return c.fetchRelease(ctx, path, fmt.Sprintf("release tag %q not found", tag), true)
}

// CreateRelease creates a new release for the provided tag.
func (c *Client) CreateRelease(ctx context.Context, owner, repo, tag string, draft bool) (*domain.Release, error) {
	if c.APIClient == nil {
		c.APIClient = httpconfig.APIClient()
	}
	u := fmt.Sprintf("%s/repos/%s/%s/releases", strings.TrimSuffix(c.BaseURL, "/"), owner, repo)
	payload := map[string]any{
		"tag_name": tag,
		"name":     tag,
		"draft":    draft,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, domain.NewError(domain.ExitNetwork, "github api: marshal release payload: "+err.Error())
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(body))
	if err != nil {
		return nil, domain.NewError(domain.ExitNetwork, "github api request: "+err.Error())
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Content-Type", "application/json")
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
	respBody, err := io.ReadAll(io.LimitReader(resp.Body, maxJSON+1))
	if err != nil {
		return nil, domain.NewError(domain.ExitNetwork, "github api: read body: "+err.Error())
	}
	if int64(len(respBody)) > maxJSON {
		return nil, domain.NewError(domain.ExitNetwork, fmt.Sprintf("github api: release response exceeds %d bytes", maxJSON))
	}
	if resp.StatusCode == http.StatusForbidden {
		return nil, domain.NewError(domain.ExitNetwork, "github api forbidden (set GITHUB_TOKEN or --github-token for release publishing)")
	}
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return nil, domain.NewError(domain.ExitNetwork, fmt.Sprintf("github api: status %s: %s", resp.Status, truncate(string(respBody), 200)))
	}
	var dto releaseDTO
	if err := json.Unmarshal(respBody, &dto); err != nil {
		return nil, domain.NewError(domain.ExitNetwork, "github api: invalid json: "+err.Error())
	}
	return releaseFromDTO(dto), nil
}

// UpdateReleaseDraftState updates the draft visibility of an existing release.
func (c *Client) UpdateReleaseDraftState(ctx context.Context, owner, repo string, releaseID int64, draft bool) (*domain.Release, error) {
	if c.APIClient == nil {
		c.APIClient = httpconfig.APIClient()
	}
	u := fmt.Sprintf("%s/repos/%s/%s/releases/%d", strings.TrimSuffix(c.BaseURL, "/"), owner, repo, releaseID)
	payload := map[string]any{"draft": draft}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, domain.NewError(domain.ExitNetwork, "github api: marshal release patch payload: "+err.Error())
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPatch, u, bytes.NewReader(body))
	if err != nil {
		return nil, domain.NewError(domain.ExitNetwork, "github api request: "+err.Error())
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Content-Type", "application/json")
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
	respBody, err := io.ReadAll(io.LimitReader(resp.Body, maxJSON+1))
	if err != nil {
		return nil, domain.NewError(domain.ExitNetwork, "github api: read body: "+err.Error())
	}
	if int64(len(respBody)) > maxJSON {
		return nil, domain.NewError(domain.ExitNetwork, fmt.Sprintf("github api: release response exceeds %d bytes", maxJSON))
	}
	if resp.StatusCode == http.StatusForbidden {
		return nil, domain.NewError(domain.ExitNetwork, "github api forbidden (set GITHUB_TOKEN or --github-token for release publishing)")
	}
	if resp.StatusCode != http.StatusOK {
		return nil, domain.NewError(domain.ExitNetwork, fmt.Sprintf("github api: status %s: %s", resp.Status, truncate(string(respBody), 200)))
	}
	var dto releaseDTO
	if err := json.Unmarshal(respBody, &dto); err != nil {
		return nil, domain.NewError(domain.ExitNetwork, "github api: invalid json: "+err.Error())
	}
	return releaseFromDTO(dto), nil
}

// UploadReleaseAsset uploads an asset to the release upload URL.
func (c *Client) UploadReleaseAsset(ctx context.Context, uploadURL, name string, body []byte, contentType string) (*domain.Asset, error) {
	if c.APIClient == nil {
		c.APIClient = httpconfig.APIClient()
	}
	if strings.TrimSpace(uploadURL) == "" {
		return nil, domain.NewError(domain.ExitRelease, "release upload_url missing")
	}
	u, err := releaseUploadURL(uploadURL, name)
	if err != nil {
		return nil, domain.NewError(domain.ExitNetwork, "github upload url: "+err.Error())
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(body))
	if err != nil {
		return nil, domain.NewError(domain.ExitNetwork, "github upload request: "+err.Error())
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	if strings.TrimSpace(contentType) != "" {
		req.Header.Set("Content-Type", contentType)
	} else {
		req.Header.Set("Content-Type", "application/octet-stream")
	}
	if c.APIVersion != "" {
		req.Header.Set("X-GitHub-Api-Version", c.APIVersion)
	}
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	resp, err := c.APIClient.Do(req)
	if err != nil {
		return nil, domain.NewError(domain.ExitNetwork, "github upload: "+err.Error())
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, domain.NewError(domain.ExitNetwork, "github upload: read body: "+err.Error())
	}
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return nil, domain.NewError(domain.ExitNetwork, fmt.Sprintf("github upload: status %s: %s", resp.Status, truncate(string(respBody), 200)))
	}
	var dto assetDTO
	if err := json.Unmarshal(respBody, &dto); err != nil {
		return nil, domain.NewError(domain.ExitNetwork, "github upload: invalid json: "+err.Error())
	}
	return &domain.Asset{
		ID:                 dto.ID,
		Name:               dto.Name,
		BrowserDownloadURL: dto.BrowserDownloadURL,
		Size:               dto.Size,
	}, nil
}

// DeleteReleaseAsset removes an existing asset by id.
func (c *Client) DeleteReleaseAsset(ctx context.Context, owner, repo string, assetID int64) error {
	if c.APIClient == nil {
		c.APIClient = httpconfig.APIClient()
	}
	u := fmt.Sprintf("%s/repos/%s/%s/releases/assets/%d", strings.TrimSuffix(c.BaseURL, "/"), owner, repo, assetID)
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, u, nil)
	if err != nil {
		return domain.NewError(domain.ExitNetwork, "github delete request: "+err.Error())
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
		return domain.NewError(domain.ExitNetwork, "github delete: "+err.Error())
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return domain.NewError(domain.ExitNetwork, fmt.Sprintf("github delete: status %s: %s", resp.Status, truncate(string(body), 200)))
	}
	return nil
}

func releaseUploadURL(raw, name string) (string, error) {
	base := raw
	if idx := strings.Index(base, "{"); idx >= 0 {
		base = base[:idx]
	}
	u, err := neturl.Parse(base)
	if err != nil {
		return "", err
	}
	q := u.Query()
	q.Set("name", name)
	u.RawQuery = q.Encode()
	return u.String(), nil
}

func releaseFromDTO(dto releaseDTO) *domain.Release {
	out := &domain.Release{
		ID:         dto.ID,
		TagName:    dto.TagName,
		Draft:      dto.Draft,
		Prerelease: dto.Prerelease,
		UploadURL:  dto.UploadURL,
		Assets:     make([]domain.Asset, 0, len(dto.Assets)),
	}
	for _, a := range dto.Assets {
		out.Assets = append(out.Assets, domain.Asset{
			ID:                 a.ID,
			Name:               a.Name,
			BrowserDownloadURL: a.BrowserDownloadURL,
			Size:               a.Size,
		})
	}
	return out
}
