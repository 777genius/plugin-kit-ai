package directoryv1

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
)

type Cache struct {
	// Path is the single atomic schema-1 last-known-good record.
	Path string
	// BeforeRename is a disposable-fixture fault injection hook. Production
	// callers leave it nil.
	BeforeRename func(temporaryPath string) error
}

type cacheRecord struct {
	SchemaVersion int    `json:"schema_version"`
	Sequence      uint64 `json:"sequence"`
	Snapshot      string `json:"snapshot"`
	Envelope      string `json:"envelope"`
}

var ErrCacheRollback = errors.New("directory cache would roll back last-known-good sequence")

func (cache Cache) Load(trust TrustStore) (VerifiedBundle, error) {
	if cache.Path == "" {
		return VerifiedBundle{}, os.ErrNotExist
	}
	info, err := os.Lstat(cache.Path)
	if err != nil {
		return VerifiedBundle{}, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() || (runtime.GOOS != "windows" && info.Mode().Perm()&0077 != 0) {
		return VerifiedBundle{}, fmt.Errorf("directory cache must be a restrictive regular file")
	}
	file, err := os.Open(cache.Path)
	if err != nil {
		return VerifiedBundle{}, err
	}
	defer file.Close()
	// The cache record base64-encodes both exact artifacts, so its bound must be
	// calculated from their encoded lengths rather than their raw wire limits.
	maxRecordBytes := base64.StdEncoding.EncodedLen(MaxSnapshotBytes) + base64.StdEncoding.EncodedLen(MaxEnvelopeBytes) + 4096
	body, err := io.ReadAll(io.LimitReader(file, int64(maxRecordBytes)+1))
	if err != nil {
		return VerifiedBundle{}, err
	}
	if len(body) > maxRecordBytes {
		return VerifiedBundle{}, fmt.Errorf("invalid directory cache: record is oversized")
	}
	var record cacheRecord
	if err := strictDecode(body, &record, maxRecordBytes); err != nil {
		return VerifiedBundle{}, fmt.Errorf("invalid directory cache: %w", err)
	}
	if err := requireRootFields(body, "schema_version", "sequence", "snapshot", "envelope"); err != nil {
		return VerifiedBundle{}, fmt.Errorf("invalid directory cache: %w", err)
	}
	if record.SchemaVersion != SnapshotSchemaVersion || record.Sequence < 1 {
		return VerifiedBundle{}, fmt.Errorf("invalid directory cache identity")
	}
	snapshot, err := base64.StdEncoding.Strict().DecodeString(record.Snapshot)
	if err != nil {
		return VerifiedBundle{}, fmt.Errorf("invalid cached snapshot encoding: %w", err)
	}
	envelope, err := base64.StdEncoding.Strict().DecodeString(record.Envelope)
	if err != nil {
		return VerifiedBundle{}, fmt.Errorf("invalid cached envelope encoding: %w", err)
	}
	bundle, err := VerifyBundle(snapshot, envelope, trust)
	if err != nil {
		return VerifiedBundle{}, err
	}
	if bundle.Snapshot.Sequence != record.Sequence {
		return VerifiedBundle{}, ErrSequenceMismatch
	}
	bundle.Source = BundleSourceCache
	return bundle, nil
}

// Store re-verifies exact bytes immediately before an atomic replacement.
// Lower records never replace the current valid LKG. An equal sequence is
// idempotent only when it authenticates the same snapshot digest; otherwise it
// is publication equivocation and must fail closed.
func (cache Cache) Store(candidate VerifiedBundle, trust TrustStore) error {
	if cache.Path == "" {
		return errors.New("directory cache path is empty")
	}
	verified, err := VerifyBundle(candidate.SnapshotBytes, candidate.EnvelopeBytes, trust)
	if err != nil {
		return err
	}
	if current, loadErr := cache.Load(trust); loadErr == nil && current.Snapshot.Sequence >= verified.Snapshot.Sequence {
		if current.Snapshot.Sequence > verified.Snapshot.Sequence {
			return ErrCacheRollback
		}
		if current.Digest != verified.Digest {
			return fmt.Errorf("%w: sequence %d", ErrSequenceConflict, verified.Snapshot.Sequence)
		}
		return nil
	}
	record := cacheRecord{SchemaVersion: SnapshotSchemaVersion, Sequence: verified.Snapshot.Sequence, Snapshot: base64.StdEncoding.EncodeToString(verified.SnapshotBytes), Envelope: base64.StdEncoding.EncodeToString(verified.EnvelopeBytes)}
	body, err := json.Marshal(record)
	if err != nil {
		return err
	}
	body = append(body, '\n')
	directory := filepath.Dir(cache.Path)
	if err := os.MkdirAll(directory, 0700); err != nil {
		return err
	}
	if err := os.Chmod(directory, 0700); err != nil {
		return err
	}
	var nonce [8]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return err
	}
	temporary := filepath.Join(directory, "."+filepath.Base(cache.Path)+"."+hex.EncodeToString(nonce[:])+".tmp")
	file, err := os.OpenFile(temporary, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	ok := false
	defer func() {
		file.Close()
		if !ok {
			_ = os.Remove(temporary)
		}
	}()
	if _, err = file.Write(body); err != nil {
		return err
	}
	if err = file.Sync(); err != nil {
		return err
	}
	if err = file.Close(); err != nil {
		return err
	}
	if cache.BeforeRename != nil {
		if err := cache.BeforeRename(temporary); err != nil {
			return err
		}
	}
	if err = os.Rename(temporary, cache.Path); err != nil {
		return err
	}
	ok = true
	dir, err := os.Open(directory)
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}
