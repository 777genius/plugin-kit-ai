package archive

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/777genius/plugin-kit-ai/plugininstall/domain"
	"github.com/777genius/plugin-kit-ai/plugininstall/ports"
)

// TarGzExtractor extracts a single plugin binary from the root of a .tar.gz (GoReleaser layout).
type TarGzExtractor struct{}

var _ ports.ArchiveExtractor = (*TarGzExtractor)(nil)

var skipRootNames = map[string]struct{}{
	"readme": {}, "readme.md": {}, "license": {}, "copying": {},
}

func skipName(base string) bool {
	b := strings.ToLower(base)
	if _, ok := skipRootNames[b]; ok {
		return true
	}
	if strings.HasSuffix(b, ".txt") || strings.HasSuffix(b, ".md") {
		return true
	}
	return false
}

// ExtractRootExecutable implements ports.ArchiveExtractor.
func (TarGzExtractor) ExtractRootExecutable(ctx context.Context, r io.Reader) (name string, data []byte, err error) {
	zr, err := gzip.NewReader(r)
	if err != nil {
		return "", nil, domain.NewError(domain.ExitAmbiguous, "archive: not a valid gzip")
	}
	defer zr.Close()

	tr := tar.NewReader(zr)
	var candidates []struct {
		name string
		data []byte
	}

	for {
		select {
		case <-ctx.Done():
			return "", nil, domain.NewError(domain.ExitNetwork, ctx.Err().Error())
		default:
		}
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", nil, domain.NewError(domain.ExitAmbiguous, "archive: corrupt tar: "+err.Error())
		}
		if hdr.Typeflag == tar.TypeDir {
			continue
		}
		if hdr.Typeflag == tar.TypeSymlink || hdr.Typeflag == tar.TypeLink {
			return "", nil, domain.NewError(domain.ExitAmbiguous, "archive: symlinks/hardlinks are not allowed")
		}
		if hdr.Typeflag != tar.TypeReg {
			if _, skipErr := io.CopyN(io.Discard, tr, hdr.Size); skipErr != nil {
				return "", nil, domain.NewError(domain.ExitAmbiguous, "archive: skip entry: "+skipErr.Error())
			}
			continue
		}
		clean := filepath.ToSlash(filepath.Clean(hdr.Name))
		if clean == "." || strings.HasPrefix(clean, "..") || strings.Contains(clean, "/../") {
			return "", nil, domain.NewError(domain.ExitAmbiguous, "archive: invalid path "+hdr.Name)
		}
		if strings.Contains(clean, "/") {
			if _, skipErr := io.CopyN(io.Discard, tr, hdr.Size); skipErr != nil {
				return "", nil, domain.NewError(domain.ExitAmbiguous, "archive: skip nested: "+skipErr.Error())
			}
			continue
		}
		base := filepath.Base(clean)
		if skipName(base) {
			if _, skipErr := io.CopyN(io.Discard, tr, hdr.Size); skipErr != nil {
				return "", nil, domain.NewError(domain.ExitAmbiguous, "archive: skip file: "+skipErr.Error())
			}
			continue
		}
		if hdr.Size < 0 || hdr.Size > 512<<20 {
			return "", nil, domain.NewError(domain.ExitAmbiguous, "archive: unreasonable file size for "+base)
		}
		buf := make([]byte, hdr.Size)
		if _, err := io.ReadFull(tr, buf); err != nil {
			return "", nil, domain.NewError(domain.ExitAmbiguous, "archive: read "+base+": "+err.Error())
		}
		candidates = append(candidates, struct {
			name string
			data []byte
		}{name: base, data: buf})
	}

	if len(candidates) == 0 {
		return "", nil, domain.NewError(domain.ExitAmbiguous, "archive: no plugin binary in tarball root (expected one file, e.g. from GoReleaser)")
	}
	if len(candidates) > 1 {
		var names []string
		for _, c := range candidates {
			names = append(names, c.name)
		}
		return "", nil, domain.NewError(domain.ExitAmbiguous, fmt.Sprintf("archive: multiple root files after filtering: %v", names))
	}
	out := make([]byte, len(candidates[0].data))
	copy(out, candidates[0].data)
	return candidates[0].name, out, nil
}
