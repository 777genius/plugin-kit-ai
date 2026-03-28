package app

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"net/http"
	neturl "net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	gh "github.com/plugin-kit-ai/plugin-kit-ai/plugininstall/adapters/github"
	"github.com/plugin-kit-ai/plugin-kit-ai/plugininstall/domain"
)

const (
	defaultBundleFetchTimeout      = 15 * time.Minute
	defaultBundleFetchConnect      = 15 * time.Second
	defaultBundleFetchMaxRedirects = 10
	defaultBundleFetchMaxBytes     = 256 << 20 // 256 MiB
	bundleFetchTestCAFileEnv       = "PLUGIN_KIT_AI_TEST_CA_FILE"
)

type PluginBundleFetchOptions struct {
	URL           string
	Ref           string
	Tag           string
	Latest        bool
	Dest          string
	SHA256        string
	AssetName     string
	Platform      string
	Runtime       string
	GitHubToken   string
	GitHubAPIBase string
	Force         bool
}

type PluginBundleFetchResult struct {
	Lines []string
}

type bundleHTTPDownloader interface {
	Download(ctx context.Context, url string) ([]byte, string, error)
}

type bundleGitHubSource interface {
	GetReleaseByTag(ctx context.Context, owner, repo, tag string) (*domain.Release, error)
	GetLatestRelease(ctx context.Context, owner, repo string) (*domain.Release, error)
	FindReleaseByTag(ctx context.Context, owner, repo, tag string) (*domain.Release, error)
	DownloadAsset(ctx context.Context, url string) (body []byte, contentType string, err error)
}

type bundleFetchDeps struct {
	URLDownloader bundleHTTPDownloader
	GitHub        bundleGitHubSource
}

type bundleHTTPClient struct {
	Client   *http.Client
	MaxBytes int64
}

type bundleRemoteSource struct {
	ArchiveBytes   []byte
	BundleSource   string
	ChecksumSource string
}

func (PluginService) BundleFetch(ctx context.Context, opts PluginBundleFetchOptions) (PluginBundleFetchResult, error) {
	deps, err := defaultBundleFetchDeps(opts)
	if err != nil {
		return PluginBundleFetchResult{}, err
	}
	return bundleFetch(ctx, opts, deps)
}

func defaultBundleFetchDeps(opts PluginBundleFetchOptions) (bundleFetchDeps, error) {
	downloader, customHTTPClient, err := newDefaultBundleFetchHTTPClient()
	if err != nil {
		return bundleFetchDeps{}, err
	}
	client := gh.NewClient(strings.TrimSpace(opts.GitHubToken))
	if base := strings.TrimSpace(opts.GitHubAPIBase); base != "" {
		client.BaseURL = base
	}
	if customHTTPClient != nil {
		client.APIClient = customHTTPClient
		client.DLClient = customHTTPClient
	}
	return bundleFetchDeps{
		URLDownloader: downloader,
		GitHub:        client,
	}, nil
}

func bundleFetch(ctx context.Context, opts PluginBundleFetchOptions, deps bundleFetchDeps) (PluginBundleFetchResult, error) {
	if strings.TrimSpace(opts.Dest) == "" {
		return PluginBundleFetchResult{}, fmt.Errorf("bundle fetch requires --dest")
	}
	if err := validateBundleFetchMode(opts); err != nil {
		return PluginBundleFetchResult{}, err
	}
	source, err := resolveBundleRemoteSource(ctx, opts, deps)
	if err != nil {
		return PluginBundleFetchResult{}, err
	}
	archivePath, cleanup, err := writeTempBundleArchive(source.ArchiveBytes)
	if err != nil {
		return PluginBundleFetchResult{}, err
	}
	defer cleanup()

	metadata, err := inspectBundleArchive(archivePath)
	if err != nil {
		return PluginBundleFetchResult{}, err
	}
	if err := validateBundleMetadata(metadata); err != nil {
		return PluginBundleFetchResult{}, err
	}
	if err := validateFetchedBundleMatchesRequest(metadata, opts); err != nil {
		return PluginBundleFetchResult{}, err
	}

	installedPath, err := installBundleArchive(archivePath, opts.Dest, opts.Force)
	if err != nil {
		return PluginBundleFetchResult{}, err
	}

	lines := []string{
		fmt.Sprintf("Bundle: plugin=%s platform=%s runtime=%s manager=%s", metadata.PluginName, metadata.Platform, metadata.Runtime, displayBundleManager(metadata.Manager)),
		"Bundle source: " + source.BundleSource,
		"Checksum source: " + source.ChecksumSource,
		"Installed path: " + installedPath,
		"Next:",
	}
	for _, step := range resolvedBundleNext(metadata, installedPath) {
		lines = append(lines, "  "+step)
	}
	return PluginBundleFetchResult{Lines: lines}, nil
}

func validateBundleFetchMode(opts PluginBundleFetchOptions) error {
	if strings.TrimSpace(opts.URL) != "" {
		if strings.TrimSpace(opts.Ref) != "" {
			return fmt.Errorf("bundle fetch accepts either --url or owner/repo, not both")
		}
		if strings.TrimSpace(opts.Tag) != "" || opts.Latest {
			return fmt.Errorf("bundle fetch URL mode does not accept --tag or --latest")
		}
		if strings.TrimSpace(opts.AssetName) != "" || strings.TrimSpace(opts.Platform) != "" || strings.TrimSpace(opts.Runtime) != "" {
			return fmt.Errorf("bundle fetch URL mode does not use --asset-name, --platform, or --runtime")
		}
		return nil
	}
	if strings.TrimSpace(opts.Ref) == "" {
		return fmt.Errorf("bundle fetch requires --url or owner/repo")
	}
	if opts.Latest && strings.TrimSpace(opts.Tag) != "" {
		return fmt.Errorf("bundle fetch does not use --tag together with --latest")
	}
	if !opts.Latest && strings.TrimSpace(opts.Tag) == "" {
		return fmt.Errorf("bundle fetch GitHub mode requires --tag or --latest")
	}
	if (strings.TrimSpace(opts.Platform) == "") != (strings.TrimSpace(opts.Runtime) == "") {
		return fmt.Errorf("bundle fetch GitHub mode requires --platform and --runtime together")
	}
	return nil
}

func resolveBundleRemoteSource(ctx context.Context, opts PluginBundleFetchOptions, deps bundleFetchDeps) (bundleRemoteSource, error) {
	if strings.TrimSpace(opts.URL) != "" {
		return resolveBundleURLSource(ctx, opts, deps.URLDownloader)
	}
	return resolveBundleGitHubSource(ctx, opts, deps.GitHub)
}

func resolveBundleURLSource(ctx context.Context, opts PluginBundleFetchOptions, downloader bundleHTTPDownloader) (bundleRemoteSource, error) {
	rawURL := strings.TrimSpace(opts.URL)
	parsed, err := neturl.Parse(rawURL)
	if err != nil {
		return bundleRemoteSource{}, fmt.Errorf("bundle fetch invalid URL: %w", err)
	}
	if parsed.Scheme != "https" {
		return bundleRemoteSource{}, fmt.Errorf("bundle fetch supports only https:// bundle URLs")
	}
	if !strings.HasSuffix(strings.ToLower(parsed.Path), ".tar.gz") {
		return bundleRemoteSource{}, fmt.Errorf("bundle fetch URL must point to a .tar.gz bundle")
	}
	if downloader == nil {
		return bundleRemoteSource{}, fmt.Errorf("bundle fetch downloader is required")
	}

	body, _, err := downloader.Download(ctx, rawURL)
	if err != nil {
		return bundleRemoteSource{}, err
	}
	sum, checksumSource, err := resolveURLBundleChecksum(ctx, downloader, rawURL, strings.TrimSpace(opts.SHA256))
	if err != nil {
		return bundleRemoteSource{}, err
	}
	if err := verifyBundleChecksum(body, sum); err != nil {
		return bundleRemoteSource{}, fmt.Errorf("bundle fetch checksum verification failed: %w", err)
	}
	return bundleRemoteSource{
		ArchiveBytes:   body,
		BundleSource:   rawURL,
		ChecksumSource: checksumSource,
	}, nil
}

func resolveBundleGitHubSource(ctx context.Context, opts PluginBundleFetchOptions, source bundleGitHubSource) (bundleRemoteSource, error) {
	if source == nil {
		return bundleRemoteSource{}, fmt.Errorf("bundle fetch GitHub source is required")
	}
	owner, repo, err := splitOwnerRepo(opts.Ref)
	if err != nil {
		return bundleRemoteSource{}, err
	}
	var rel *domain.Release
	if opts.Latest {
		rel, err = source.GetLatestRelease(ctx, owner, repo)
	} else {
		rel, err = source.FindReleaseByTag(ctx, owner, repo, strings.TrimSpace(opts.Tag))
	}
	if err != nil {
		return bundleRemoteSource{}, err
	}
	asset, err := selectBundleReleaseAsset(rel, opts.AssetName, opts.Platform, opts.Runtime)
	if err != nil {
		return bundleRemoteSource{}, err
	}
	body, _, err := source.DownloadAsset(ctx, asset.BrowserDownloadURL)
	if err != nil {
		return bundleRemoteSource{}, err
	}
	sum, checksumSource, err := resolveGitHubBundleChecksum(ctx, source, rel, *asset)
	if err != nil {
		return bundleRemoteSource{}, err
	}
	if err := verifyBundleChecksum(body, sum); err != nil {
		return bundleRemoteSource{}, fmt.Errorf("bundle fetch checksum verification failed: %w", err)
	}

	releaseRef := strings.TrimSpace(rel.TagName)
	if releaseRef == "" {
		releaseRef = strings.TrimSpace(opts.Tag)
	}
	refLabel := owner + "/" + repo
	if releaseRef != "" {
		refLabel += "@" + releaseRef
	}
	if opts.Latest {
		refLabel += " (latest)"
	} else {
		refLabel += " (tag)"
	}
	return bundleRemoteSource{
		ArchiveBytes:   body,
		BundleSource:   fmt.Sprintf("github release %s asset=%s", refLabel, asset.Name),
		ChecksumSource: checksumSource,
	}, nil
}

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

func selectBundleReleaseAsset(rel *domain.Release, assetName, platform, runtime string) (*domain.Asset, error) {
	assetName = strings.TrimSpace(assetName)
	if assetName != "" {
		asset := findReleaseAsset(rel.Assets, assetName)
		if asset == nil {
			return nil, fmt.Errorf("bundle fetch release has no asset named %q", assetName)
		}
		return asset, nil
	}

	platform = strings.TrimSpace(platform)
	runtime = strings.TrimSpace(runtime)
	candidates := bundleReleaseCandidates(rel.Assets)
	if platform != "" && runtime != "" {
		suffix := fmt.Sprintf("_%s_%s_bundle.tar.gz", platform, runtime)
		matches := make([]domain.Asset, 0, len(candidates))
		for _, asset := range candidates {
			if strings.HasSuffix(asset.Name, suffix) {
				matches = append(matches, asset)
			}
		}
		if len(matches) == 1 {
			return &matches[0], nil
		}
		if len(matches) == 0 {
			return nil, fmt.Errorf("bundle fetch release has no bundle asset matching %s", suffix)
		}
		return nil, fmt.Errorf("bundle fetch release has multiple bundle assets matching %s: %s", suffix, joinAssetNames(matches))
	}

	if len(candidates) == 1 {
		return &candidates[0], nil
	}
	if len(candidates) == 0 {
		return nil, fmt.Errorf("bundle fetch release has no *_bundle.tar.gz assets")
	}
	return nil, fmt.Errorf("bundle fetch release bundle assets are ambiguous; use --asset-name or --platform with --runtime: %s", joinAssetNames(candidates))
}

func bundleReleaseCandidates(assets []domain.Asset) []domain.Asset {
	out := make([]domain.Asset, 0, len(assets))
	for _, asset := range assets {
		name := strings.ToLower(asset.Name)
		if strings.HasSuffix(name, "_bundle.tar.gz") {
			out = append(out, asset)
		}
	}
	return out
}

func joinAssetNames(assets []domain.Asset) string {
	names := make([]string, 0, len(assets))
	for _, asset := range assets {
		names = append(names, asset.Name)
	}
	return strings.Join(names, ", ")
}

func findReleaseAsset(assets []domain.Asset, name string) *domain.Asset {
	name = strings.TrimSpace(name)
	for i := range assets {
		if assets[i].Name == name {
			return &assets[i]
		}
	}
	return nil
}

func splitOwnerRepo(ref string) (string, string, error) {
	parts := strings.SplitN(strings.TrimSpace(ref), "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("bundle fetch GitHub mode requires owner/repo")
	}
	return parts[0], parts[1], nil
}

func bundleSidecarURL(rawURL string) (string, error) {
	u, err := neturl.Parse(rawURL)
	if err != nil {
		return "", fmt.Errorf("bundle fetch invalid URL: %w", err)
	}
	u.Path += ".sha256"
	return u.String(), nil
}

func validateFetchedBundleMatchesRequest(metadata exportMetadata, opts PluginBundleFetchOptions) error {
	if platform := strings.TrimSpace(opts.Platform); platform != "" && metadata.Platform != platform {
		return fmt.Errorf("bundle fetch selected asset does not match requested platform %q", platform)
	}
	if runtime := strings.TrimSpace(opts.Runtime); runtime != "" && metadata.Runtime != runtime {
		return fmt.Errorf("bundle fetch selected asset does not match requested runtime %q", runtime)
	}
	return nil
}

func writeTempBundleArchive(body []byte) (string, func(), error) {
	f, err := os.CreateTemp("", ".plugin-kit-ai-bundle-fetch-*.tar.gz")
	if err != nil {
		return "", nil, err
	}
	if _, err := f.Write(body); err != nil {
		_ = f.Close()
		_ = os.Remove(f.Name())
		return "", nil, err
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(f.Name())
		return "", nil, err
	}
	cleanup := func() { _ = os.Remove(f.Name()) }
	return f.Name(), cleanup, nil
}

func newDefaultBundleFetchHTTPClient() (bundleHTTPClient, *http.Client, error) {
	client, err := newBundleHTTPClient(bundleHTTPClientConfig{
		AdditionalRootsFile: strings.TrimSpace(os.Getenv(bundleFetchTestCAFileEnv)),
	})
	if err != nil {
		return bundleHTTPClient{}, nil, err
	}
	if strings.TrimSpace(os.Getenv(bundleFetchTestCAFileEnv)) == "" {
		return client, nil, nil
	}
	return client, client.Client, nil
}

type bundleHTTPClientConfig struct {
	AdditionalRootsFile string
}

func newBundleHTTPClient(cfg bundleHTTPClientConfig) (bundleHTTPClient, error) {
	t := http.DefaultTransport.(*http.Transport).Clone()
	t.DialContext = (&net.Dialer{Timeout: defaultBundleFetchConnect}).DialContext
	if strings.TrimSpace(cfg.AdditionalRootsFile) != "" {
		pool, err := loadBundleFetchAdditionalRoots(cfg.AdditionalRootsFile)
		if err != nil {
			return bundleHTTPClient{}, err
		}
		if t.TLSClientConfig == nil {
			t.TLSClientConfig = &tls.Config{}
		}
		t.TLSClientConfig.RootCAs = pool
	}
	return bundleHTTPClient{
		Client: &http.Client{
			Timeout:   defaultBundleFetchTimeout,
			Transport: t,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if len(via) >= defaultBundleFetchMaxRedirects {
					return http.ErrUseLastResponse
				}
				if req.URL.Scheme != "https" {
					return http.ErrUseLastResponse
				}
				prev := via[len(via)-1].URL
				if prev.Scheme != "https" {
					return http.ErrUseLastResponse
				}
				return nil
			},
		},
		MaxBytes: defaultBundleFetchMaxBytes,
	}, nil
}

func loadBundleFetchAdditionalRoots(path string) (*x509.CertPool, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("bundle fetch test root CA file %q: %w", path, err)
	}
	pool, err := x509.SystemCertPool()
	if err != nil || pool == nil {
		pool = x509.NewCertPool()
	}
	if !pool.AppendCertsFromPEM(body) {
		return nil, fmt.Errorf("bundle fetch test root CA file %q does not contain valid PEM certificates", path)
	}
	return pool, nil
}

func (c bundleHTTPClient) Download(ctx context.Context, url string) ([]byte, string, error) {
	if c.Client == nil {
		defaultClient, _, err := newDefaultBundleFetchHTTPClient()
		if err != nil {
			return nil, "", err
		}
		c = defaultClient
	}
	max := c.MaxBytes
	if max <= 0 {
		max = defaultBundleFetchMaxBytes
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, "", fmt.Errorf("bundle fetch request: %w", err)
	}
	resp, err := c.Client.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("bundle fetch download: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, "", fmt.Errorf("bundle fetch download: status %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}
	if resp.ContentLength > max {
		return nil, "", fmt.Errorf("bundle fetch download: content-length %d exceeds limit %d", resp.ContentLength, max)
	}
	limit := max
	if resp.ContentLength > 0 {
		limit = resp.ContentLength
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, "", fmt.Errorf("bundle fetch download read: %w", err)
	}
	if int64(len(body)) > max {
		return nil, "", fmt.Errorf("bundle fetch download exceeds limit %d bytes", max)
	}
	return body, resp.Header.Get("Content-Type"), nil
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
