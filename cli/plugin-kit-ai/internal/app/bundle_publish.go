package app

import (
	"context"
	"fmt"
	"strings"

	gh "github.com/777genius/plugin-kit-ai/plugininstall/adapters/github"
	"github.com/777genius/plugin-kit-ai/plugininstall/domain"
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

	artifact, err := prepareBundlePublishArtifact(root, platform, deps)
	if err != nil {
		return PluginBundlePublishResult{}, err
	}

	release, state, err := ensurePublishRelease(ctx, deps.GitHub, owner, repo, tag, opts.Draft)
	if err != nil {
		return PluginBundlePublishResult{}, err
	}

	if err := maybeDeleteReleaseAsset(ctx, deps.GitHub, owner, repo, release, artifact.BundleName, opts.Force); err != nil {
		return PluginBundlePublishResult{}, err
	}
	if err := maybeDeleteReleaseAsset(ctx, deps.GitHub, owner, repo, release, artifact.SidecarName, opts.Force); err != nil {
		return PluginBundlePublishResult{}, err
	}

	if _, err := deps.GitHub.UploadReleaseAsset(ctx, release.UploadURL, artifact.BundleName, artifact.Body, "application/gzip"); err != nil {
		return PluginBundlePublishResult{}, err
	}
	if _, err := deps.GitHub.UploadReleaseAsset(ctx, release.UploadURL, artifact.SidecarName, artifact.SidecarBody, "text/plain; charset=utf-8"); err != nil {
		return PluginBundlePublishResult{}, err
	}

	releaseLabel := owner + "/" + repo + "@" + tag
	lines := []string{
		fmt.Sprintf("Bundle: plugin=%s platform=%s runtime=%s manager=%s", artifact.Metadata.PluginName, artifact.Metadata.Platform, artifact.Metadata.Runtime, displayBundleManager(artifact.Metadata.Manager)),
		"Release: " + releaseLabel,
		"Release state: " + state,
		"Uploaded assets:",
		"  " + artifact.BundleName,
		"  " + artifact.SidecarName,
		"Next:",
		fmt.Sprintf("  plugin-kit-ai bundle fetch %s --tag %s --platform %s --runtime %s --dest <path>", ref, tag, artifact.Metadata.Platform, artifact.Metadata.Runtime),
		fmt.Sprintf("  plugin-kit-ai bundle install ./%s --dest <path>", artifact.BundleName),
	}
	return PluginBundlePublishResult{Lines: lines}, nil
}
