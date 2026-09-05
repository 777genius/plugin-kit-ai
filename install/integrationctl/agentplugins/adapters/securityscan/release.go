package securityscan

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/777genius/plugin-kit-ai/install/integrationctl/agentplugins/domain"
)

const (
	defaultReleaseOrigin = "https://github.com/777genius/lintai/releases/download/v0.1.2/"
	maxReleaseAsset      = 32 << 20
)

type releaseAsset struct {
	Name   string
	Digest string
	Zip    bool
}

var releaseAssets = map[string]releaseAsset{
	"darwin/arm64":     {"lintai-v0.1.2-aarch64-apple-darwin.tar.gz", "b02b40ad25ae6a2c9358400b1560ec4c358a82ff9f3d2ad6daf0bcbf0a426277", false},
	"darwin/amd64":     {"lintai-v0.1.2-x86_64-apple-darwin.tar.gz", "dbc74549d6f4da4e4460d4a671f8da7933298d9a82831b058f37a210e7291eb9", false},
	"linux/arm64":      {"lintai-v0.1.2-aarch64-unknown-linux-gnu.tar.gz", "31df7193cfe8298bec99af9b843f38379e93ab82fee6dd577debdc55a5c1e051", false},
	"linux/amd64":      {"lintai-v0.1.2-x86_64-unknown-linux-gnu.tar.gz", "c06fcb33a13f1483f20f4cb30d2c701cd01c6b8ba43e3b7322ebab3c1ae11e72", false},
	"linux/amd64-musl": {"lintai-v0.1.2-x86_64-unknown-linux-musl.tar.gz", "c4f6b45cb959855107d9b1425eed1e4409c5c7d9f3e15c1784f7e2c2569c9792", false},
	"windows/amd64":    {"lintai-v0.1.2-x86_64-pc-windows-msvc.zip", "4cc96dfbefb0a213767ed591bd6f78ab83fd06e9b40af1fad4590e53457c96c4", true},
	"windows/arm64":    {"lintai-v0.1.2-aarch64-pc-windows-msvc.zip", "ff85139ee2968a829daf0e4f8d701c7dbe3ec1078ca8bb81029057be75b86629", true},
}

type ReleaseScanner struct {
	Root       string
	HTTPClient *http.Client
	Origin     string
	GOOS       string
	GOARCH     string
}

func (scanner ReleaseScanner) Scan(ctx context.Context, packageRoot string) ([]byte, error) {
	executable, err := scanner.resolve(ctx)
	if err != nil {
		return nil, err
	}
	return (CommandScanner{Executable: executable}).Scan(ctx, packageRoot)
}

func (scanner ReleaseScanner) resolve(ctx context.Context) (string, error) {
	goos, goarch := scanner.GOOS, scanner.GOARCH
	if goos == "" {
		goos = runtime.GOOS
	}
	if goarch == "" {
		goarch = runtime.GOARCH
	}
	key := goos + "/" + goarch
	if goos == "linux" && goarch == "amd64" && usesMusl() {
		key += "-musl"
	}
	asset, ok := releaseAssets[key]
	if !ok {
		return "", fmt.Errorf("lintai security scanner is not released for %s", key)
	}
	if scanner.Root == "" {
		return "", errors.New("lintai security scanner cache root is unavailable")
	}
	binaryName := "lintai"
	if goos == "windows" {
		binaryName += ".exe"
	}
	directory := filepath.Join(scanner.Root, domain.SecurityScannerVersion, strings.ReplaceAll(key, "/", "-"))
	executable := filepath.Join(directory, binaryName)
	if info, err := os.Lstat(executable); err == nil && info.Mode().IsRegular() {
		return executable, nil
	}
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return "", fmt.Errorf("create lintai cache: %w", err)
	}
	origin := scanner.Origin
	if origin == "" {
		origin = defaultReleaseOrigin
	}
	client := scanner.HTTPClient
	if client == nil {
		client = &http.Client{}
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, origin+asset.Name, nil)
	if err != nil {
		return "", err
	}
	response, err := client.Do(request)
	if err != nil {
		return "", fmt.Errorf("download lintai security scanner: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download lintai security scanner: HTTP %d", response.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, maxReleaseAsset+1))
	if err != nil || len(body) > maxReleaseAsset {
		return "", errors.New("downloaded lintai release asset is unreadable or oversized")
	}
	digest := sha256.Sum256(body)
	if hex.EncodeToString(digest[:]) != asset.Digest {
		return "", errors.New("downloaded lintai release asset failed checksum verification")
	}
	binary, err := extractReleaseBinary(body, asset.Zip, binaryName)
	if err != nil {
		return "", err
	}
	temporary, err := os.CreateTemp(directory, ".lintai-*.tmp")
	if err != nil {
		return "", err
	}
	temporaryName := temporary.Name()
	defer os.Remove(temporaryName)
	if err := temporary.Chmod(0o700); err != nil {
		_ = temporary.Close()
		return "", err
	}
	if _, err := temporary.Write(binary); err != nil {
		_ = temporary.Close()
		return "", err
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return "", err
	}
	if err := temporary.Close(); err != nil {
		return "", err
	}
	if err := os.Rename(temporaryName, executable); err != nil {
		if info, statErr := os.Lstat(executable); statErr == nil && info.Mode().IsRegular() {
			return executable, nil
		}
		return "", fmt.Errorf("commit lintai security scanner: %w", err)
	}
	return executable, nil
}

func extractReleaseBinary(body []byte, zipAsset bool, binaryName string) ([]byte, error) {
	if zipAsset {
		archive, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
		if err != nil {
			return nil, fmt.Errorf("open lintai release archive: %w", err)
		}
		for _, entry := range archive.File {
			if filepath.Base(filepath.ToSlash(entry.Name)) != binaryName || !entry.Mode().IsRegular() || entry.UncompressedSize64 > maxReleaseAsset {
				continue
			}
			reader, err := entry.Open()
			if err != nil {
				return nil, err
			}
			value, readErr := io.ReadAll(io.LimitReader(reader, maxReleaseAsset+1))
			closeErr := reader.Close()
			if readErr != nil || closeErr != nil || len(value) > maxReleaseAsset {
				return nil, errors.New("read lintai binary from release archive")
			}
			return value, nil
		}
		return nil, errors.New("lintai release archive does not contain the expected binary")
	}
	gzipReader, err := gzip.NewReader(bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("open lintai release archive: %w", err)
	}
	defer gzipReader.Close()
	tarReader := tar.NewReader(gzipReader)
	for {
		header, err := tarReader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("read lintai release archive: %w", err)
		}
		if filepath.Base(filepath.ToSlash(header.Name)) != binaryName || header.Typeflag != tar.TypeReg || header.Size < 0 || header.Size > maxReleaseAsset {
			continue
		}
		value, err := io.ReadAll(io.LimitReader(tarReader, maxReleaseAsset+1))
		if err != nil || len(value) > maxReleaseAsset {
			return nil, errors.New("read lintai binary from release archive")
		}
		return value, nil
	}
	return nil, errors.New("lintai release archive does not contain the expected binary")
}

func usesMusl() bool {
	if runtime.GOOS != "linux" {
		return false
	}
	for _, candidate := range []string{"/lib/ld-musl-x86_64.so.1", "/lib64/ld-musl-x86_64.so.1"} {
		if _, err := os.Stat(candidate); err == nil {
			return true
		}
	}
	return false
}
