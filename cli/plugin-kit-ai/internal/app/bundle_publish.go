package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	gh "github.com/plugin-kit-ai/plugin-kit-ai/plugininstall/adapters/github"
	"github.com/plugin-kit-ai/plugin-kit-ai/plugininstall/domain"
)

type PluginBundlePublishOptions struct {
	Root          string
	Platform      string
	Repo          string
	Tag           string
	Draft         bool
	GitHubToken   string
	GitHubAPIBase string
	Force         bool
}

type PluginBundlePublishResult struct {
	Lines []string
}

type bundleGitHubPublisher interface {
	FindReleaseByTag(ctx context.Context, owner, repo, tag string) (*domain.Release, error)
	CreateRelease(ctx context.Context, owner, repo, tag string, draft bool) (*domain.Release, error)
	UpdateReleaseDraftState(ctx context.Context, owner, repo string, releaseID int64, draft bool) (*domain.Release, error)
	UploadReleaseAsset(ctx context.Context, uploadURL, name string, body []byte, contentType string) (*domain.Asset, error)
	DeleteReleaseAsset(ctx context.Context, owner, repo string, assetID int64) error
}

type bundlePublishDeps struct {
	GitHub bundleGitHubPublisher
	Export func(PluginExportOptions) (PluginExportResult, error)
}

func (PluginService) BundlePublish(ctx context.Context, opts PluginBundlePublishOptions) (PluginBundlePublishResult, error) {
	deps := defaultBundlePublishDeps(opts)
	return bundlePublish(ctx, opts, deps)
}

func defaultBundlePublishDeps(opts PluginBundlePublishOptions) bundlePublishDeps {
	client := gh.NewClient(strings.TrimSpace(opts.GitHubToken))
	if base := strings.TrimSpace(opts.GitHubAPIBase); base != "" {
		client.BaseURL = base
	}
	return bundlePublishDeps{
		GitHub: client,
		Export: func(exportOpts PluginExportOptions) (PluginExportResult, error) {
			return PluginService{}.Export(exportOpts)
		},
	}
}

func bundlePublish(ctx context.Context, opts PluginBundlePublishOptions, deps bundlePublishDeps) (PluginBundlePublishResult, error) {
	root := strings.TrimSpace(opts.Root)
	if root == "" {
		root = "."
	}
	platform := strings.TrimSpace(opts.Platform)
	if platform == "" {
		return PluginBundlePublishResult{}, fmt.Errorf("bundle publish requires --platform")
	}
	ref := strings.TrimSpace(opts.Repo)
	if ref == "" {
		return PluginBundlePublishResult{}, fmt.Errorf("bundle publish requires --repo owner/repo")
	}
	tag := strings.TrimSpace(opts.Tag)
	if tag == "" {
		return PluginBundlePublishResult{}, fmt.Errorf("bundle publish requires --tag")
	}
	if deps.GitHub == nil {
		return PluginBundlePublishResult{}, fmt.Errorf("bundle publish GitHub client is required")
	}
	if deps.Export == nil {
		return PluginBundlePublishResult{}, fmt.Errorf("bundle publish export dependency is required")
	}
	owner, repo, err := splitOwnerRepo(ref)
	if err != nil {
		return PluginBundlePublishResult{}, fmt.Errorf("bundle publish %w", err)
	}

	exportPath := filepath.Join(os.TempDir(), fmt.Sprintf(".plugin-kit-ai-publish-%d.tar.gz", os.Getpid()))
	tmpFile, err := os.CreateTemp("", ".plugin-kit-ai-publish-*.tar.gz")
	if err != nil {
		return PluginBundlePublishResult{}, err
	}
	exportPath = tmpFile.Name()
	_ = tmpFile.Close()
	defer os.Remove(exportPath)

	exportResult, err := deps.Export(PluginExportOptions{
		Root:     root,
		Platform: platform,
		Output:   exportPath,
	})
	if err != nil {
		return PluginBundlePublishResult{}, err
	}
	_ = exportResult

	metadata, err := inspectBundleArchive(exportPath)
	if err != nil {
		return PluginBundlePublishResult{}, err
	}
	if err := validateBundleMetadata(metadata); err != nil {
		return PluginBundlePublishResult{}, err
	}

	body, err := os.ReadFile(exportPath)
	if err != nil {
		return PluginBundlePublishResult{}, err
	}
	bundleName := fmt.Sprintf("%s_%s_%s_bundle.tar.gz", metadata.PluginName, metadata.Platform, metadata.Runtime)
	sum := sha256.Sum256(body)
	sumBody := []byte(hex.EncodeToString(sum[:]) + "  " + bundleName + "\n")
	sidecarName := bundleName + ".sha256"

	release, state, err := ensurePublishRelease(ctx, deps.GitHub, owner, repo, tag, opts.Draft)
	if err != nil {
		return PluginBundlePublishResult{}, err
	}

	if err := maybeDeleteReleaseAsset(ctx, deps.GitHub, owner, repo, release, bundleName, opts.Force); err != nil {
		return PluginBundlePublishResult{}, err
	}
	if err := maybeDeleteReleaseAsset(ctx, deps.GitHub, owner, repo, release, sidecarName, opts.Force); err != nil {
		return PluginBundlePublishResult{}, err
	}

	if _, err := deps.GitHub.UploadReleaseAsset(ctx, release.UploadURL, bundleName, body, "application/gzip"); err != nil {
		return PluginBundlePublishResult{}, err
	}
	if _, err := deps.GitHub.UploadReleaseAsset(ctx, release.UploadURL, sidecarName, sumBody, "text/plain; charset=utf-8"); err != nil {
		return PluginBundlePublishResult{}, err
	}

	releaseLabel := owner + "/" + repo + "@" + tag
	lines := []string{
		fmt.Sprintf("Bundle: plugin=%s platform=%s runtime=%s manager=%s", metadata.PluginName, metadata.Platform, metadata.Runtime, displayBundleManager(metadata.Manager)),
		"Release: " + releaseLabel,
		"Release state: " + state,
		"Uploaded assets:",
		"  " + bundleName,
		"  " + sidecarName,
		"Next:",
		fmt.Sprintf("  plugin-kit-ai bundle fetch %s --tag %s --platform %s --runtime %s --dest <path>", ref, tag, metadata.Platform, metadata.Runtime),
		fmt.Sprintf("  plugin-kit-ai bundle install ./%s --dest <path>", bundleName),
	}
	return PluginBundlePublishResult{Lines: lines}, nil
}

func ensurePublishRelease(ctx context.Context, client bundleGitHubPublisher, owner, repo, tag string, wantDraft bool) (*domain.Release, string, error) {
	release, err := client.FindReleaseByTag(ctx, owner, repo, tag)
	if err == nil {
		if !wantDraft && release.Draft {
			if release.ID == 0 {
				return nil, "", fmt.Errorf("bundle publish cannot promote draft release %q because GitHub release id is missing", tag)
			}
			release, err = client.UpdateReleaseDraftState(ctx, owner, repo, release.ID, false)
			if err != nil {
				return nil, "", err
			}
			return release, "promoted draft release to published", nil
		}
		if release.Draft {
			return release, "reused existing draft release", nil
		}
		return release, "reused existing published release", nil
	}
	if de, ok := err.(*domain.Error); ok && de.Code == domain.ExitRelease {
		release, err = client.CreateRelease(ctx, owner, repo, tag, wantDraft)
		if err != nil {
			return nil, "", err
		}
		if wantDraft {
			return release, "created draft release", nil
		}
		return release, "created published release", nil
	}
	return nil, "", err
}

func maybeDeleteReleaseAsset(ctx context.Context, client bundleGitHubPublisher, owner, repo string, release *domain.Release, name string, force bool) error {
	asset := findReleaseAsset(release.Assets, name)
	if asset == nil {
		return nil
	}
	if !force {
		return fmt.Errorf("bundle publish release already has asset %q; use --force to replace", name)
	}
	if asset.ID == 0 {
		return fmt.Errorf("bundle publish cannot replace existing asset %q because GitHub asset id is missing", name)
	}
	return client.DeleteReleaseAsset(ctx, owner, repo, asset.ID)
}
