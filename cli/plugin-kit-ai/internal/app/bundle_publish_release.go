package app

import (
	"context"
	"fmt"

	"github.com/777genius/plugin-kit-ai/plugininstall/domain"
)

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
