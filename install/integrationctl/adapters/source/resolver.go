package source

import (
	"context"
	"os"
	"strings"

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
