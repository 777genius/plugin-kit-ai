package packagesnapshot

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/777genius/plugin-kit-ai/install/integrationctl/adapters/pathpolicy"
	"golang.org/x/text/cases"
	"golang.org/x/text/unicode/norm"
)

const (
	defaultMaxFiles     = 10_000
	defaultMaxFileBytes = int64(64 << 20)
	defaultMaxTreeBytes = int64(256 << 20)
	defaultMaxDepth     = 64
)

type Limits struct {
	MaxFiles     int
	MaxFileBytes int64
	MaxTreeBytes int64
	MaxDepth     int
}

type Snapshot struct {
	Root            string
	Digest          string
	FileCount       int
	TotalBytes      int64
	ExecutableFiles []string
	cleanup         func() error
}

func (snapshot Snapshot) Close() error {
	if snapshot.cleanup == nil {
		return nil
	}
	return snapshot.cleanup()
}

type Builder struct {
	TempRoot   string
	Limits     Limits
	beforeCopy func(string) error
}

type entry struct {
	rel        string
	sourcePath string
	isDir      bool
	mode       os.FileMode
	size       int64
	info       os.FileInfo
}

func (builder Builder) Build(ctx context.Context, sourceRoot string) (Snapshot, error) {
	if err := ctx.Err(); err != nil {
		return Snapshot{}, err
	}
	limits := builder.limits()
	entries, fileCount, totalBytes, err := inspectTree(ctx, sourceRoot, limits)
	if err != nil {
		return Snapshot{}, err
	}
	tempRoot, err := os.MkdirTemp(builder.TempRoot, "agentplugins-snapshot-*")
	if err != nil {
		return Snapshot{}, fmt.Errorf("create package snapshot root: %w", err)
	}
	cleanup := func() error { return removeSnapshot(tempRoot) }
	snapshotRoot := filepath.Join(tempRoot, "package")
	if err := os.Mkdir(snapshotRoot, 0o700); err != nil {
		_ = cleanup()
		return Snapshot{}, fmt.Errorf("create package snapshot: %w", err)
	}
	hash := sha256.New()
	var executableFiles []string
	for _, item := range entries {
		if err := ctx.Err(); err != nil {
			_ = cleanup()
			return Snapshot{}, err
		}
		destination := filepath.Join(snapshotRoot, filepath.FromSlash(item.rel))
		if item.isDir {
			if err := os.Mkdir(destination, 0o700); err != nil {
				_ = cleanup()
				return Snapshot{}, fmt.Errorf("create snapshot directory %q: %w", item.rel, err)
			}
			writeDigestHeader(hash, "dir", item.rel, false, 0)
			continue
		}
		executable := item.mode&0o111 != 0
		if executable {
			executableFiles = append(executableFiles, item.rel)
		}
		if builder.beforeCopy != nil {
			if err := builder.beforeCopy(item.rel); err != nil {
				_ = cleanup()
				return Snapshot{}, err
			}
		}
		written, err := copyRegularFile(sourceRoot, item, destination, executable, hash)
		if err != nil {
			_ = cleanup()
			return Snapshot{}, err
		}
		if written != item.size {
			_ = cleanup()
			return Snapshot{}, fmt.Errorf("source file %q changed size while snapshotting", item.rel)
		}
	}
	if err := makeReadOnly(snapshotRoot, entries); err != nil {
		_ = cleanup()
		return Snapshot{}, err
	}
	return Snapshot{
		Root:            snapshotRoot,
		Digest:          "sha256:" + hex.EncodeToString(hash.Sum(nil)),
		FileCount:       fileCount,
		TotalBytes:      totalBytes,
		ExecutableFiles: executableFiles,
		cleanup:         cleanup,
	}, nil
}

func (builder Builder) limits() Limits {
	limits := builder.Limits
	if limits.MaxFiles <= 0 {
		limits.MaxFiles = defaultMaxFiles
	}
	if limits.MaxFileBytes <= 0 {
		limits.MaxFileBytes = defaultMaxFileBytes
	}
	if limits.MaxTreeBytes <= 0 {
		limits.MaxTreeBytes = defaultMaxTreeBytes
	}
	if limits.MaxDepth <= 0 {
		limits.MaxDepth = defaultMaxDepth
	}
	return limits
}

func inspectTree(ctx context.Context, sourceRoot string, limits Limits) ([]entry, int, int64, error) {
	root, err := filepath.Abs(sourceRoot)
	if err != nil {
		return nil, 0, 0, fmt.Errorf("resolve source root: %w", err)
	}
	root = filepath.Clean(root)
	rootInfo, err := os.Lstat(root)
	if err != nil {
		return nil, 0, 0, fmt.Errorf("inspect source root: %w", err)
	}
	if !rootInfo.IsDir() || rootInfo.Mode()&os.ModeSymlink != 0 {
		return nil, 0, 0, fmt.Errorf("source root must be a real directory")
	}
	var entries []entry
	fileCount := 0
	var totalBytes int64
	portablePaths := map[string]string{}
	err = filepath.WalkDir(root, func(path string, dirEntry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if path == root {
			return nil
		}
		relNative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel := filepath.ToSlash(relNative)
		base := filepath.Base(path)
		if dirEntry.IsDir() && base == ".git" {
			return filepath.SkipDir
		}
		if !dirEntry.IsDir() && base == ".plugin-kit-ai.lock" {
			return nil
		}
		if err := validateRelativePath(rel, limits.MaxDepth, portablePaths); err != nil {
			return err
		}
		info, err := os.Lstat(path)
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("snapshot source contains symlink %q", rel)
		}
		if info.IsDir() {
			entries = append(entries, entry{rel: rel, sourcePath: path, isDir: true, mode: info.Mode()})
			return nil
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("snapshot source contains special file %q", rel)
		}
		fileCount++
		if fileCount > limits.MaxFiles {
			return fmt.Errorf("snapshot source exceeds %d files", limits.MaxFiles)
		}
		if info.Size() > limits.MaxFileBytes {
			return fmt.Errorf("snapshot source file %q exceeds %d bytes", rel, limits.MaxFileBytes)
		}
		totalBytes += info.Size()
		if totalBytes > limits.MaxTreeBytes {
			return fmt.Errorf("snapshot source exceeds %d total bytes", limits.MaxTreeBytes)
		}
		entries = append(entries, entry{rel: rel, sourcePath: path, mode: info.Mode(), size: info.Size(), info: info})
		return nil
	})
	if err != nil {
		return nil, 0, 0, fmt.Errorf("inspect package tree: %w", err)
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].rel == entries[j].rel {
			return entries[i].isDir && !entries[j].isDir
		}
		return entries[i].rel < entries[j].rel
	})
	return entries, fileCount, totalBytes, nil
}

func validateRelativePath(rel string, maxDepth int, seen map[string]string) error {
	if rel == "" || rel == "." || strings.HasPrefix(rel, "/") || !utf8.ValidString(rel) || !norm.NFC.IsNormalString(rel) {
		return fmt.Errorf("snapshot source contains non-portable path %q", rel)
	}
	parts := strings.Split(rel, "/")
	if len(parts) > maxDepth {
		return fmt.Errorf("snapshot source path %q exceeds depth %d", rel, maxDepth)
	}
	for _, part := range parts {
		if err := pathpolicy.ValidatePortablePathSegment(part); err != nil {
			return fmt.Errorf("snapshot source contains unsafe path %q", rel)
		}
	}
	folded := cases.Fold().String(rel)
	if previous, exists := seen[folded]; exists && previous != rel {
		return fmt.Errorf("snapshot source paths %q and %q collide on a portable filesystem", previous, rel)
	}
	seen[folded] = rel
	return nil
}

func copyRegularFile(sourceRoot string, item entry, destination string, executable bool, hash io.Writer) (int64, error) {
	source, err := openSnapshotRegular(sourceRoot, item.rel)
	if err != nil {
		return 0, fmt.Errorf("open snapshot source file %q: %w", item.rel, err)
	}
	defer source.Close()
	openedInfo, err := source.Stat()
	if err != nil {
		return 0, fmt.Errorf("inspect opened snapshot source file %q: %w", item.rel, err)
	}
	if !openedInfo.Mode().IsRegular() || item.info == nil || !item.info.Mode().IsRegular() || !os.SameFile(openedInfo, item.info) {
		return 0, fmt.Errorf("snapshot source file %q changed identity while opening", item.rel)
	}
	if err := requireSingleLink(source); err != nil {
		return 0, fmt.Errorf("snapshot source file %q: %w", item.rel, err)
	}
	mode := os.FileMode(0o600)
	if executable {
		mode = 0o700
	}
	destinationFile, err := os.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return 0, fmt.Errorf("create snapshot file %q: %w", item.rel, err)
	}
	writeDigestHeader(hash, "file", item.rel, executable, item.size)
	written, copyErr := io.Copy(io.MultiWriter(destinationFile, hash), io.LimitReader(source, item.size+1))
	closeErr := destinationFile.Close()
	if copyErr != nil {
		return written, fmt.Errorf("copy snapshot file %q: %w", item.rel, copyErr)
	}
	if closeErr != nil {
		return written, fmt.Errorf("close snapshot file %q: %w", item.rel, closeErr)
	}
	if written > item.size {
		return written, fmt.Errorf("source file %q grew while snapshotting", item.rel)
	}
	return written, nil
}

func writeDigestHeader(writer io.Writer, kind, rel string, executable bool, size int64) {
	_, _ = fmt.Fprintf(writer, "%s\x00%s\x00%t\x00%d\x00", kind, rel, executable, size)
}

func makeReadOnly(root string, entries []entry) error {
	for index := len(entries) - 1; index >= 0; index-- {
		item := entries[index]
		path := filepath.Join(root, filepath.FromSlash(item.rel))
		mode := os.FileMode(0o444)
		if item.isDir {
			mode = 0o555
		} else if item.mode&0o111 != 0 {
			mode = 0o555
		}
		if err := os.Chmod(path, mode); err != nil {
			return fmt.Errorf("seal snapshot path %q: %w", item.rel, err)
		}
	}
	if err := os.Chmod(root, 0o555); err != nil {
		return fmt.Errorf("seal snapshot root: %w", err)
	}
	return nil
}

func removeSnapshot(root string) error {
	_ = filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err == nil && entry.IsDir() {
			_ = os.Chmod(path, 0o700)
		}
		return nil
	})
	return os.RemoveAll(root)
}
