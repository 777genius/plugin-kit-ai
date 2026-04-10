package source

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/777genius/plugin-kit-ai/install/integrationctl/adapters/process"
	"github.com/777genius/plugin-kit-ai/install/integrationctl/ports"
)

func (r Resolver) runner() ports.ProcessRunner {
	if r.Runner != nil {
		return r.Runner
	}
	return process.OS{}
}

func (r Resolver) cloneGitHub(ctx context.Context, ownerRepo, subdir, gitRef string) (string, string, error) {
	return r.clone(ctx, "https://github.com/"+ownerRepo+".git", subdir, gitRef)
}

func (r Resolver) cloneURL(ctx context.Context, raw string, gitRef string) (string, string, error) {
	return r.clone(ctx, raw, "", gitRef)
}

func (r Resolver) clone(ctx context.Context, repoURL, subdir, gitRef string) (string, string, error) {
	tmp, err := os.MkdirTemp("", "integrationctl-source-*")
	if err != nil {
		return "", "", err
	}
	cloneResult, err := r.runner().Run(ctx, ports.Command{
		Argv: []string{"git", "clone", "--depth", "1", repoURL, tmp},
	})
	if err != nil {
		_ = os.RemoveAll(tmp)
		if isCommandNotFound(err) {
			return "", "", fmt.Errorf("git not found")
		}
		return "", "", err
	}
	if cloneResult.ExitCode != 0 {
		_ = os.RemoveAll(tmp)
		return "", "", fmt.Errorf("git clone failed: %s", commandOutput(cloneResult))
	}
	if strings.TrimSpace(gitRef) != "" {
		if err := r.checkoutRef(ctx, tmp, gitRef); err != nil {
			_ = os.RemoveAll(tmp)
			return "", "", err
		}
	}
	revResult, err := r.runner().Run(ctx, ports.Command{
		Argv: []string{"git", "-C", tmp, "rev-parse", "HEAD"},
	})
	if err != nil {
		_ = os.RemoveAll(tmp)
		if isCommandNotFound(err) {
			return "", "", fmt.Errorf("git not found")
		}
		return "", "", err
	}
	if revResult.ExitCode != 0 {
		_ = os.RemoveAll(tmp)
		return "", "", fmt.Errorf("git rev-parse failed: %s", commandOutput(revResult))
	}
	root := tmp
	if subdir != "" {
		root = filepath.Join(tmp, subdir)
	}
	if info, err := os.Stat(root); err != nil || !info.IsDir() {
		_ = os.RemoveAll(tmp)
		return "", "", fmt.Errorf("source subdir not found: %s", subdir)
	}
	return root, strings.TrimSpace(string(revResult.Stdout)), nil
}

func (r Resolver) checkoutRef(ctx context.Context, repoRoot, gitRef string) error {
	fetchResult, err := r.runner().Run(ctx, ports.Command{
		Argv: []string{"git", "-C", repoRoot, "fetch", "--depth", "1", "origin", gitRef},
	})
	if err != nil {
		if isCommandNotFound(err) {
			return fmt.Errorf("git not found")
		}
		return err
	}
	if fetchResult.ExitCode != 0 {
		return fmt.Errorf("git fetch %q failed: %s", gitRef, commandOutput(fetchResult))
	}
	checkoutResult, err := r.runner().Run(ctx, ports.Command{
		Argv: []string{"git", "-C", repoRoot, "checkout", "FETCH_HEAD"},
	})
	if err != nil {
		if isCommandNotFound(err) {
			return fmt.Errorf("git not found")
		}
		return err
	}
	if checkoutResult.ExitCode != 0 {
		return fmt.Errorf("git checkout %q failed: %s", gitRef, commandOutput(checkoutResult))
	}
	return nil
}
