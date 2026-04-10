package source

import (
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

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
