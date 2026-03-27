package usecase

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"

	"github.com/plugin-kit-ai/plugin-kit-ai/plugininstall/domain"
	"github.com/plugin-kit-ai/plugin-kit-ai/plugininstall/ports"
)

// Input is the install use case input.
type Input struct {
	Owner, Repo, Tag string
	UseLatest        bool
	InstallDir       string
	Force            bool
	AllowPrerelease  bool
	OutputName       string
	Target           ports.TargetPlatform
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

// Installer wires ports.
type Installer struct {
	GitHub    ports.ReleaseSource
	Archive   ports.ArchiveExtractor
	FS        ports.FileSystem
	Resolver  ports.InstallDirResolver
	Selector  ports.AssetSelector
	Checksums ports.ChecksumVerifier
	Perms     ports.PermissionPolicy
}

// Run performs install.
func (in *Installer) Run(ctx context.Context, input Input) (Result, error) {
	if err := in.validateDeps(); err != nil {
		return Result{}, err
	}
	if input.Owner == "" || input.Repo == "" {
		return Result{}, domain.NewError(domain.ExitUsage, "owner and repo are required")
	}
	tag := strings.TrimSpace(input.Tag)
	switch {
	case tag != "" && input.UseLatest:
		return Result{}, domain.NewError(domain.ExitUsage, "use either --tag or --latest, not both")
	case tag == "" && !input.UseLatest:
		return Result{}, domain.NewError(domain.ExitUsage, "set --tag or use --latest")
	}
	if strings.TrimSpace(input.Target.GOOS) == "" || strings.TrimSpace(input.Target.GOARCH) == "" {
		return Result{}, domain.NewError(domain.ExitUsage, "target GOOS/GOARCH are required")
	}

	dir := input.InstallDir
	if dir == "" {
		dir = "bin"
	}
	absDir, err := in.Resolver.Resolve(dir)
	if err != nil {
		return Result{}, domain.NewError(domain.ExitFS, "install dir: "+err.Error())
	}
	dirInfo, err := in.FS.PathInfo(ctx, absDir)
	if err != nil {
		return Result{}, err
	}
	if dirInfo.Exists && !dirInfo.IsDir {
		return Result{}, domain.NewError(domain.ExitFS, "install dir is an existing file: "+absDir)
	}

	var rel *domain.Release
	if input.UseLatest {
		rel, err = in.GitHub.GetLatestRelease(ctx, input.Owner, input.Repo)
	} else {
		rel, err = in.GitHub.GetReleaseByTag(ctx, input.Owner, input.Repo, tag)
	}
	if err != nil {
		return Result{}, err
	}
	if rel.Prerelease && !input.AllowPrerelease {
		return Result{}, domain.NewError(domain.ExitRelease, "release is prerelease (refused; use plugin-kit-ai install --pre)")
	}
	if err := validateOutputName(input.OutputName); err != nil {
		return Result{}, err
	}

	checksumsAsset := findAsset(rel.Assets, "checksums.txt")
	if checksumsAsset == nil {
		return Result{}, domain.NewError(domain.ExitChecksum, "release has no checksums.txt (required for verified install)")
	}

	payload, fromTarGz, err := in.Selector.Pick(rel.Assets, input.Target)
	if err != nil {
		return Result{}, err
	}

	sumBody, _, err := in.GitHub.DownloadAsset(ctx, checksumsAsset.BrowserDownloadURL)
	if err != nil {
		return Result{}, err
	}
	expected, err := in.Checksums.Expected(sumBody, payload.Name)
	if err != nil {
		return Result{}, domain.NewError(domain.ExitChecksum, "checksums.txt: "+err.Error())
	}

	payloadBody, _, err := in.GitHub.DownloadAsset(ctx, payload.BrowserDownloadURL)
	if err != nil {
		return Result{}, err
	}
	if err := in.Checksums.Verify(payloadBody, expected, payload.Name); err != nil {
		return Result{}, err
	}

	var binName string
	var binData []byte
	if fromTarGz {
		binName, binData, err = in.Archive.ExtractRootExecutable(ctx, bytes.NewReader(payloadBody))
		if err != nil {
			return Result{}, err
		}
	} else {
		binName = filepath.Base(payload.Name)
		binData = payloadBody
	}

	outName := strings.TrimSpace(input.OutputName)
	if outName == "" {
		outName = binName
	}

	destPath := filepath.Join(absDir, outName)
	destInfo, err := in.FS.PathInfo(ctx, destPath)
	if err != nil {
		return Result{}, err
	}
	if destInfo.Exists && destInfo.IsDir {
		return Result{}, domain.NewError(domain.ExitFS, "destination is an existing directory: "+destPath)
	}
	if destInfo.Exists && !input.Force {
		return Result{}, domain.NewError(domain.ExitFS, destPath+" already exists (use --force to overwrite)")
	}
	if destInfo.Exists && input.Force {
		_ = in.FS.RemoveBestEffort(ctx, destPath)
	}

	r := bytes.NewReader(binData)
	if err := in.FS.WriteFileAtomic(ctx, absDir, outName, r, int64(len(binData)), in.Perms.FileMode(input.Target)); err != nil {
		return Result{}, err
	}
	releaseSource := "tag"
	if input.UseLatest {
		releaseSource = "latest"
	}
	payloadKind := "raw"
	if fromTarGz {
		payloadKind = "tar.gz"
	}
	return Result{
		ResolvedInstallPath: destPath,
		InstalledFileName:   outName,
		ReleaseRef:          strings.TrimSpace(rel.TagName),
		ReleaseSource:       releaseSource,
		AssetName:           payload.Name,
		TargetGOOS:          input.Target.GOOS,
		TargetGOARCH:        input.Target.GOARCH,
		Overwrote:           destInfo.Exists,
		PayloadKind:         payloadKind,
	}, nil
}

func (in *Installer) validateDeps() error {
	switch {
	case in.GitHub == nil:
		return domain.NewError(domain.ExitUsage, "installer github source is required")
	case in.Archive == nil:
		return domain.NewError(domain.ExitUsage, "installer archive extractor is required")
	case in.FS == nil:
		return domain.NewError(domain.ExitUsage, "installer file system is required")
	case in.Resolver == nil:
		return domain.NewError(domain.ExitUsage, "installer path resolver is required")
	case in.Selector == nil:
		return domain.NewError(domain.ExitUsage, "installer asset selector is required")
	case in.Checksums == nil:
		return domain.NewError(domain.ExitUsage, "installer checksum verifier is required")
	case in.Perms == nil:
		return domain.NewError(domain.ExitUsage, "installer permission policy is required")
	default:
		return nil
	}
}

func validateOutputName(s string) error {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	if s == "." || s == ".." {
		return domain.NewError(domain.ExitUsage, "invalid --output-name")
	}
	if strings.Contains(s, "/") || strings.ContainsRune(s, '\\') {
		return domain.NewError(domain.ExitUsage, "--output-name must be a single file name (no path separators)")
	}
	if filepath.Base(s) != s {
		return domain.NewError(domain.ExitUsage, "--output-name must be a base file name only")
	}
	return nil
}

func findAsset(assets []domain.Asset, wantName string) *domain.Asset {
	w := strings.ToLower(wantName)
	for i := range assets {
		if strings.ToLower(assets[i].Name) == w {
			return &assets[i]
		}
	}
	return nil
}
