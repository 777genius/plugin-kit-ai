package safemutate

import (
	"context"
	"os"
	"path/filepath"

	"github.com/777genius/plugin-kit-ai/install/integrationctl/domain"
	"github.com/777genius/plugin-kit-ai/install/integrationctl/ports"
)

type OS struct{}

func (OS) MutateFile(ctx context.Context, in ports.SafeFileMutationInput) (ports.SafeFileMutationResult, error) {
	original, err := os.ReadFile(in.Path)
	exists := err == nil
	if err != nil && !os.IsNotExist(err) {
		return ports.SafeFileMutationResult{}, domain.NewError(domain.ErrMutationApply, "read mutation target", err)
	}
	next, err := in.Build(original, exists)
	if err != nil {
		return ports.SafeFileMutationResult{}, domain.NewError(domain.ErrMutationApply, "build safe mutation payload", err)
	}
	if in.ValidateBefore != nil {
		if err := in.ValidateBefore(next); err != nil {
			return ports.SafeFileMutationResult{}, domain.NewError(domain.ErrMutationApply, "pre-validate safe mutation payload", err)
		}
	}
	if err := os.MkdirAll(filepath.Dir(in.Path), 0o755); err != nil {
		return ports.SafeFileMutationResult{}, domain.NewError(domain.ErrMutationApply, "mkdir safe mutation target", err)
	}
	result := ports.SafeFileMutationResult{Path: in.Path, HadOriginal: exists}
	if exists {
		backup, err := os.CreateTemp(filepath.Dir(in.Path), filepath.Base(in.Path)+".bak-*")
		if err != nil {
			return ports.SafeFileMutationResult{}, domain.NewError(domain.ErrMutationApply, "create backup for safe mutation", err)
		}
		if _, err := backup.Write(original); err != nil {
			_ = backup.Close()
			_ = os.Remove(backup.Name())
			return ports.SafeFileMutationResult{}, domain.NewError(domain.ErrMutationApply, "write backup for safe mutation", err)
		}
		if err := backup.Close(); err != nil {
			_ = os.Remove(backup.Name())
			return ports.SafeFileMutationResult{}, domain.NewError(domain.ErrMutationApply, "close backup for safe mutation", err)
		}
		result.BackupPath = backup.Name()
	}
	restore := func() {
		if exists {
			_ = os.WriteFile(in.Path, original, os.FileMode(in.Mode))
			return
		}
		_ = os.Remove(in.Path)
	}
	tmp, err := os.CreateTemp(filepath.Dir(in.Path), filepath.Base(in.Path)+".tmp-*")
	if err != nil {
		if result.BackupPath != "" {
			_ = os.Remove(result.BackupPath)
		}
		return ports.SafeFileMutationResult{}, domain.NewError(domain.ErrMutationApply, "create temp file for safe mutation", err)
	}
	tmpName := tmp.Name()
	cleanupTmp := func() { _ = os.Remove(tmpName) }
	if _, err := tmp.Write(next); err != nil {
		_ = tmp.Close()
		cleanupTmp()
		if result.BackupPath != "" {
			_ = os.Remove(result.BackupPath)
		}
		return ports.SafeFileMutationResult{}, domain.NewError(domain.ErrMutationApply, "write temp file for safe mutation", err)
	}
	if err := tmp.Chmod(os.FileMode(in.Mode)); err != nil {
		_ = tmp.Close()
		cleanupTmp()
		if result.BackupPath != "" {
			_ = os.Remove(result.BackupPath)
		}
		return ports.SafeFileMutationResult{}, domain.NewError(domain.ErrMutationApply, "chmod temp file for safe mutation", err)
	}
	if err := tmp.Close(); err != nil {
		cleanupTmp()
		if result.BackupPath != "" {
			_ = os.Remove(result.BackupPath)
		}
		return ports.SafeFileMutationResult{}, domain.NewError(domain.ErrMutationApply, "close temp file for safe mutation", err)
	}
	if err := os.Rename(tmpName, in.Path); err != nil {
		cleanupTmp()
		if result.BackupPath != "" {
			_ = os.Remove(result.BackupPath)
		}
		return ports.SafeFileMutationResult{}, domain.NewError(domain.ErrMutationApply, "replace target file during safe mutation", err)
	}
	if in.ValidateAfter != nil {
		if err := in.ValidateAfter(ctx, in.Path, next); err != nil {
			restore()
			if result.BackupPath != "" {
				_ = os.Remove(result.BackupPath)
			}
			return ports.SafeFileMutationResult{}, domain.NewError(domain.ErrMutationApply, "post-validate safe mutation payload", err)
		}
	}
	return result, nil
}
