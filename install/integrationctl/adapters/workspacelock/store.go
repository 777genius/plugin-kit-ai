package workspacelock

import (
	"context"
	"errors"
	"os"
	"path/filepath"

	"github.com/777genius/plugin-kit-ai/install/integrationctl/domain"
	"github.com/777genius/plugin-kit-ai/install/integrationctl/ports"
	"gopkg.in/yaml.v3"
)

type Store struct {
	FS   ports.FileSystem
	File string
}

func (s Store) Path() string {
	return s.File
}

func (s Store) Load(ctx context.Context) (domain.WorkspaceLock, error) {
	if s.File == "" {
		return domain.WorkspaceLock{}, errors.New("workspace lock path required")
	}
	body, err := s.FS.ReadFile(ctx, s.File)
	if err != nil {
		return domain.WorkspaceLock{}, err
	}
	var out domain.WorkspaceLock
	if err := yaml.Unmarshal(body, &out); err != nil {
		return domain.WorkspaceLock{}, err
	}
	if out.APIVersion == "" {
		out.APIVersion = "v1"
	}
	return out, nil
}

func (s Store) Save(ctx context.Context, lock domain.WorkspaceLock) error {
	if s.File == "" {
		return errors.New("workspace lock path required")
	}
	if lock.APIVersion == "" {
		lock.APIVersion = "v1"
	}
	body, err := yaml.Marshal(lock)
	if err != nil {
		return err
	}
	if err := s.FS.MkdirAll(ctx, filepath.Dir(s.File), 0o755); err != nil {
		return err
	}
	return s.FS.WriteFileAtomic(ctx, s.File, body, 0o644)
}

func IsNotExist(err error) bool {
	return os.IsNotExist(err)
}
