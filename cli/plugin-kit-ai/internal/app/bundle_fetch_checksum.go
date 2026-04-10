package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/777genius/plugin-kit-ai/plugininstall/domain"
)

func resolveURLBundleChecksum(ctx context.Context, downloader bundleHTTPDownloader, rawURL, flagValue string) ([]byte, string, error) {
	if flagValue != "" {
		sum, err := parseBundleChecksum([]byte(flagValue), "")
		if err != nil {
			return nil, "", fmt.Errorf("invalid --sha256: %w", err)
		}
		return sum, "flag --sha256", nil
	}
	sidecarURL, err := bundleSidecarURL(rawURL)
	if err != nil {
		return nil, "", err
	}
	body, _, err := downloader.Download(ctx, sidecarURL)
	if err != nil {
		return nil, "", fmt.Errorf("bundle fetch requires --sha256 or %s: %w", sidecarURL, err)
	}
	sum, err := parseBundleChecksum(body, "")
	if err != nil {
		return nil, "", fmt.Errorf("invalid checksum sidecar %s: %w", sidecarURL, err)
	}
	return sum, sidecarURL, nil
}

func resolveGitHubBundleChecksum(ctx context.Context, source bundleGitHubSource, rel *domain.Release, asset domain.Asset) ([]byte, string, error) {
	if checksums := findReleaseAsset(rel.Assets, "checksums.txt"); checksums != nil {
		body, _, err := source.DownloadAsset(ctx, checksums.BrowserDownloadURL)
		if err != nil {
			return nil, "", err
		}
		sum, err := parseBundleChecksum(body, asset.Name)
		if err == nil {
			return sum, "release asset checksums.txt", nil
		}
	}
	sidecarName := asset.Name + ".sha256"
	sidecar := findReleaseAsset(rel.Assets, sidecarName)
	if sidecar == nil {
		return nil, "", fmt.Errorf("bundle fetch requires checksums.txt or %s on the selected release", sidecarName)
	}
	body, _, err := source.DownloadAsset(ctx, sidecar.BrowserDownloadURL)
	if err != nil {
		return nil, "", err
	}
	sum, err := parseBundleChecksum(body, asset.Name)
	if err != nil {
		return nil, "", fmt.Errorf("invalid checksum asset %s: %w", sidecarName, err)
	}
	return sum, "release asset " + sidecarName, nil
}

func verifyBundleChecksum(body, expected []byte) error {
	got := sha256.Sum256(body)
	if len(expected) != len(got) || !equalBytes(got[:], expected) {
		return fmt.Errorf("sha256 mismatch")
	}
	return nil
}

func parseBundleChecksum(body []byte, wantName string) ([]byte, error) {
	lines := strings.Split(strings.ReplaceAll(string(body), "\r\n", "\n"), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) == 1 && isHexChecksum(fields[0]) {
			return hex.DecodeString(fields[0])
		}
		if len(fields) < 2 || !isHexChecksum(fields[0]) {
			continue
		}
		name := strings.TrimPrefix(fields[len(fields)-1], "*")
		if wantName == "" || filepath.Base(name) == filepath.Base(wantName) || name == wantName {
			return hex.DecodeString(fields[0])
		}
	}
	if wantName != "" {
		return nil, fmt.Errorf("no checksum entry for %s", wantName)
	}
	return nil, fmt.Errorf("no checksum entry found")
}

func isHexChecksum(s string) bool {
	if len(strings.TrimSpace(s)) != 64 {
		return false
	}
	_, err := hex.DecodeString(strings.TrimSpace(s))
	return err == nil
}

func equalBytes(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
