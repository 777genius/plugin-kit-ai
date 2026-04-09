package jsonstate

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

	"github.com/777genius/plugin-kit-ai/install/integrationctl/ports"
)

type Store struct {
	FS   ports.FileSystem
	Path string
}

func (s Store) Load(ctx context.Context) (ports.StateFile, error) {
	if s.Path == "" {
		return ports.StateFile{}, errors.New("state path required")
	}
	data, err := s.FS.ReadFile(ctx, s.Path)
	if err != nil {
		if os.IsNotExist(err) {
			return ports.StateFile{SchemaVersion: 1}, nil
		}
		return ports.StateFile{}, err
	}
	var out ports.StateFile
	if err := json.Unmarshal(data, &out); err != nil {
		return ports.StateFile{}, err
	}
	if out.SchemaVersion == 0 {
		out.SchemaVersion = 1
	}
	return out, nil
}

func (s Store) Save(ctx context.Context, state ports.StateFile) error {
	if state.SchemaVersion == 0 {
		state.SchemaVersion = 1
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if err := s.FS.MkdirAll(ctx, filepath.Dir(s.Path), 0o755); err != nil {
		return err
	}
	return s.FS.WriteFileAtomic(ctx, s.Path, data, 0o644)
}
