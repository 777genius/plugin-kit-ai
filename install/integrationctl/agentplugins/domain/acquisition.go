package domain

import "time"

const (
	TreeDigestAlgorithm = "agentplugins-tree-sha256-v1"
	DefaultMaxFiles     = 10_000
	DefaultMaxFileBytes = int64(64 << 20)
	DefaultMaxTreeBytes = int64(256 << 20)
	DefaultMaxDepth     = 64
)

type PackageLimits struct {
	MaxFiles     int   `json:"max_files"`
	MaxFileBytes int64 `json:"max_file_bytes"`
	MaxTreeBytes int64 `json:"max_tree_bytes"`
	MaxDepth     int   `json:"max_depth"`
}

type PackageSnapshot struct {
	Root            string         `json:"-"`
	TreeDigest      string         `json:"tree_digest"`
	DigestAlgorithm string         `json:"digest_algorithm"`
	FileCount       int            `json:"file_count"`
	TotalBytes      int64          `json:"total_bytes"`
	ExecutableFiles []string       `json:"executable_files,omitempty"`
	Source          SourceIdentity `json:"source"`
	AcquiredAt      time.Time      `json:"-"`
}
