package source

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/777genius/plugin-kit-ai/install/integrationctl/adapters/process"
	"github.com/777genius/plugin-kit-ai/install/integrationctl/domain"
	"github.com/777genius/plugin-kit-ai/install/integrationctl/ports"
)

type Resolver struct {
	Runner ports.ProcessRunner
}

func (r Resolver) Resolve(ctx context.Context, ref domain.IntegrationRef) (ports.ResolvedSource, error) {
	raw := strings.TrimSpace(ref.Raw)
	if raw == "" {
		return ports.ResolvedSource{}, domain.NewError(domain.ErrUsage, "source is required", nil)
	}
	if p, ok := resolveLocal(raw); ok {
		digest, err := hashLocalTree(p)
		if err != nil {
			return ports.ResolvedSource{}, domain.NewError(domain.ErrSourceResolve, "hash local source", err)
		}
		return ports.ResolvedSource{
			Kind:         "local_path",
			Requested:    domain.RequestedSourceRef{Kind: "local_path", Value: p},
			Resolved:     domain.ResolvedSourceRef{Kind: "local_path", Value: p},
			LocalPath:    p,
			SourceDigest: digest,
			ImportRoots:  []string{p},
		}, nil
	}
	if resolvedAlias, ok := resolveFirstPartySourceAlias(raw); ok {
		raw = resolvedAlias
	}
	if ownerRepo, gitRef, subdir, ok := parseGitHubRef(raw); ok {
		tmp, commit, err := r.cloneGitHub(ctx, ownerRepo, subdir, gitRef)
		if err != nil {
			return ports.ResolvedSource{}, domain.NewError(domain.ErrSourceResolve, "resolve github source", err)
		}
		digest, err := hashLocalTree(tmp)
		if err != nil {
			_ = os.RemoveAll(cleanupRoot(tmp, subdir))
			return ports.ResolvedSource{}, domain.NewError(domain.ErrSourceResolve, "hash github source", err)
		}
		return ports.ResolvedSource{
			Kind:         "github_repo_path",
			Requested:    domain.RequestedSourceRef{Kind: "github_repo_path", Value: raw},
			Resolved:     domain.ResolvedSourceRef{Kind: "git_commit", Value: "https://github.com/" + ownerRepo + "@" + commit},
			LocalPath:    tmp,
			CleanupPath:  cleanupRoot(tmp, subdir),
			SourceDigest: digest,
			ImportRoots:  []string{tmp},
		}, nil
	}
	if repoURL, gitRef, ok := parseGitURLRef(raw); ok {
		tmp, commit, err := r.cloneURL(ctx, repoURL, gitRef)
		if err != nil {
			return ports.ResolvedSource{}, domain.NewError(domain.ErrSourceResolve, "resolve git url", err)
		}
		digest, err := hashLocalTree(tmp)
		if err != nil {
			_ = os.RemoveAll(cleanupRoot(tmp, ""))
			return ports.ResolvedSource{}, domain.NewError(domain.ErrSourceResolve, "hash git url source", err)
		}
		return ports.ResolvedSource{
			Kind:         "git_url",
			Requested:    domain.RequestedSourceRef{Kind: "git_url", Value: raw},
			Resolved:     domain.ResolvedSourceRef{Kind: "git_commit", Value: raw + "@" + commit},
			LocalPath:    tmp,
			CleanupPath:  cleanupRoot(tmp, ""),
			SourceDigest: digest,
			ImportRoots:  []string{tmp},
		}, nil
	}
	return ports.ResolvedSource{}, domain.NewError(domain.ErrSourceResolve, "unsupported source format", nil)
}

func resolveLocal(raw string) (string, bool) {
	path := filepath.Clean(raw)
	if strings.HasPrefix(raw, ".") || strings.HasPrefix(raw, "/") {
		abs, _ := filepath.Abs(path)
		if info, err := os.Stat(abs); err == nil && info.IsDir() {
			return abs, true
		}
	}
	abs, _ := filepath.Abs(path)
	if info, err := os.Stat(abs); err == nil && info.IsDir() {
		return abs, true
	}
	return "", false
}

func parseGitHubRef(raw string) (ownerRepo, gitRef, subdir string, ok bool) {
	value := strings.TrimPrefix(raw, "github:")
	parts := strings.SplitN(value, "//", 2)
	ownerRepo = strings.TrimSpace(parts[0])
	if ownerRepo == "" {
		return "", "", "", false
	}
	if idx := strings.LastIndex(ownerRepo, "@"); idx > 0 {
		gitRef = strings.TrimSpace(ownerRepo[idx+1:])
		ownerRepo = strings.TrimSpace(ownerRepo[:idx])
	}
	if ownerRepo == "" || strings.Count(ownerRepo, "/") != 1 {
		return "", "", "", false
	}
	if len(parts) == 2 {
		subdir = strings.Trim(parts[1], "/")
	}
	return ownerRepo, gitRef, subdir, true
}

func parseGitURLRef(raw string) (repoURL, gitRef string, ok bool) {
	repoURL = strings.TrimSpace(raw)
	if idx := strings.LastIndex(repoURL, "#"); idx >= 0 {
		gitRef = normalizeGitRef(repoURL[idx+1:])
		repoURL = strings.TrimSpace(repoURL[:idx])
	}
	if !isGitURL(repoURL) {
		return "", "", false
	}
	return repoURL, gitRef, true
}

func normalizeGitRef(raw string) string {
	raw = strings.TrimSpace(raw)
	raw = strings.TrimPrefix(raw, "ref=")
	return strings.TrimSpace(raw)
}

func isGitURL(raw string) bool {
	if strings.HasPrefix(raw, "git@") || strings.HasSuffix(raw, ".git") {
		return true
	}
	u, err := url.Parse(raw)
	return err == nil && (u.Scheme == "https" || u.Scheme == "ssh")
}

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

func cleanupRoot(localPath, subdir string) string {
	if strings.TrimSpace(subdir) == "" {
		return localPath
	}
	root := localPath
	for range strings.Split(strings.Trim(subdir, "/"), "/") {
		root = filepath.Dir(root)
	}
	return root
}

func hashLocalTree(root string) (string, error) {
	hasher := sha256.New()
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		hasher.Write([]byte(filepath.ToSlash(rel)))
		hasher.Write([]byte{0})
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		hasher.Write(data)
		hasher.Write([]byte{0})
		return nil
	})
	if err != nil {
		return "", err
	}
	return "sha256:" + hex.EncodeToString(hasher.Sum(nil)), nil
}

func isCommandNotFound(err error) bool {
	return errors.Is(err, exec.ErrNotFound) || errors.Is(err, os.ErrNotExist)
}

func commandOutput(result ports.CommandResult) string {
	if text := strings.TrimSpace(string(result.Stderr)); text != "" {
		return text
	}
	return strings.TrimSpace(string(result.Stdout))
}
