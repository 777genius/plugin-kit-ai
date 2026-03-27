package plugininstall

import (
	"context"
	"crypto/sha256"
	"errors"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/plugin-kit-ai/plugin-kit-ai/plugininstall/adapters/archive"
	"github.com/plugin-kit-ai/plugin-kit-ai/plugininstall/adapters/fs"
	gh "github.com/plugin-kit-ai/plugin-kit-ai/plugininstall/adapters/github"
	"github.com/plugin-kit-ai/plugin-kit-ai/plugininstall/domain"
	"github.com/plugin-kit-ai/plugin-kit-ai/plugininstall/internal/checksum"
	"github.com/plugin-kit-ai/plugin-kit-ai/plugininstall/ports"
	"github.com/plugin-kit-ai/plugin-kit-ai/plugininstall/usecase"
)

// Params configures Install (plugin binary from GitHub Releases).
type Params struct {
	Owner, Repo, Tag string
	UseLatest        bool   // if true, Tag is ignored; use GitHub releases/latest
	InstallDir       string // default "bin" (relative to cwd resolved to abs)
	Force            bool
	Token            string // optional; also read from GITHUB_TOKEN in CLI
	AllowPrerelease  bool   // default false
	OutputName       string // empty = name from archive
	GitHubBaseURL    string // empty = default
	GOOS             string // optional explicit target override
	GOARCH           string // optional explicit target override
}

type Result struct {
	ResolvedInstallPath string
	InstalledFileName   string
	ReleaseRef          string
	ReleaseSource       string
	AssetName           string
	TargetGOOS          string
	TargetGOARCH        string
	Overwrote           bool
	PayloadKind         string
}

// Install downloads and verifies a release tarball and writes the binary to InstallDir.
// This file is the module composition root: the only place that wires concrete adapters into usecase.Installer.
func Install(ctx context.Context, p Params) (Result, error) {
	client := gh.NewClient(p.Token)
	if p.GitHubBaseURL != "" {
		client.BaseURL = strings.TrimSuffix(p.GitHubBaseURL, "/")
	}
	inst := &usecase.Installer{
		GitHub:    client,
		Archive:   archive.TarGzExtractor{},
		FS:        fs.OS{},
		Resolver:  hostPathResolver{},
		Selector:  hostAssetSelector{},
		Checksums: hostChecksumVerifier{},
		Perms:     hostPermissionPolicy{},
	}
	got, err := inst.Run(ctx, usecase.Input{
		Owner:           p.Owner,
		Repo:            p.Repo,
		Tag:             p.Tag,
		UseLatest:       p.UseLatest,
		InstallDir:      p.InstallDir,
		Force:           p.Force,
		AllowPrerelease: p.AllowPrerelease,
		OutputName:      p.OutputName,
		Target:          hostTarget(p.GOOS, p.GOARCH),
	})
	if err != nil {
		return Result{}, err
	}
	return Result{
		ResolvedInstallPath: got.ResolvedInstallPath,
		InstalledFileName:   got.InstalledFileName,
		ReleaseRef:          got.ReleaseRef,
		ReleaseSource:       got.ReleaseSource,
		AssetName:           got.AssetName,
		TargetGOOS:          got.TargetGOOS,
		TargetGOARCH:        got.TargetGOARCH,
		Overwrote:           got.Overwrote,
		PayloadKind:         got.PayloadKind,
	}, nil
}

// ExitCodeFromErr maps domain errors to shell exit codes; unknown returns 1.
func ExitCodeFromErr(err error) int {
	if err == nil {
		return 0
	}
	var de *domain.Error
	if errors.As(err, &de) {
		return int(de.Code)
	}
	if errors.Is(err, context.Canceled) {
		return 1
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return 3
	}
	return 1
}

type hostPathResolver struct{}

func (hostPathResolver) Resolve(path string) (string, error) {
	return filepath.Abs(path)
}

type hostAssetSelector struct{}

func (hostAssetSelector) Pick(assets []domain.Asset, target ports.TargetPlatform) (*domain.Asset, bool, error) {
	return domain.PickInstallAsset(assets, target.GOOS, target.GOARCH)
}

type hostChecksumVerifier struct{}

func (hostChecksumVerifier) Expected(checksumsFile []byte, assetName string) ([]byte, error) {
	return checksum.ExpectedSum(checksumsFile, assetName)
}

func (hostChecksumVerifier) Verify(payload []byte, expected []byte, assetName string) error {
	got := sha256.Sum256(payload)
	if len(expected) != len(got) {
		return domain.NewError(domain.ExitChecksum, "internal checksum length")
	}
	for i := range expected {
		if expected[i] != got[i] {
			return domain.NewError(domain.ExitChecksum, "sha256 mismatch for "+assetName)
		}
	}
	return nil
}

type hostPermissionPolicy struct{}

func (hostPermissionPolicy) FileMode(target ports.TargetPlatform) uint32 {
	if target.GOOS == "windows" {
		return 0o644
	}
	return 0o755
}

func hostTarget(goos, goarch string) ports.TargetPlatform {
	if strings.TrimSpace(goos) == "" {
		goos = runtime.GOOS
	}
	if strings.TrimSpace(goarch) == "" {
		goarch = runtime.GOARCH
	}
	return ports.TargetPlatform{
		GOOS:   strings.TrimSpace(goos),
		GOARCH: strings.TrimSpace(goarch),
	}
}
