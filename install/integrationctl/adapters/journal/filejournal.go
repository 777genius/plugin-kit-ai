package journal

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/777genius/plugin-kit-ai/install/integrationctl/domain"
	"github.com/777genius/plugin-kit-ai/install/integrationctl/ports"
)

type FileJournal struct {
	FS      ports.FileSystem
	BaseDir string
}

func (j FileJournal) Start(ctx context.Context, op domain.OperationRecord) error {
	return j.write(ctx, op)
}

func (j FileJournal) AppendStep(ctx context.Context, operationID string, step domain.JournalStep) error {
	op, err := j.read(ctx, operationID)
	if err != nil {
		return err
	}
	op.Steps = append(op.Steps, step)
	return j.write(ctx, op)
}

func (j FileJournal) Finish(ctx context.Context, operationID, status string) error {
	op, err := j.read(ctx, operationID)
	if err != nil {
		return err
	}
	op.Status = status
	return j.write(ctx, op)
}

func (j FileJournal) ListOpen(ctx context.Context) ([]domain.OperationRecord, error) {
	if err := j.FS.MkdirAll(ctx, j.BaseDir, 0o755); err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(j.BaseDir)
	if err != nil {
		return nil, err
	}
	var out []domain.OperationRecord
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		op, err := j.read(ctx, trimExt(entry.Name()))
		if err != nil {
			continue
		}
		if op.Status != "committed" && op.Status != "rolled_back" {
			out = append(out, op)
		}
	}
	return out, nil
}

func (j FileJournal) read(ctx context.Context, operationID string) (domain.OperationRecord, error) {
	data, err := j.FS.ReadFile(ctx, j.path(operationID))
	if err != nil {
		return domain.OperationRecord{}, err
	}
	var op domain.OperationRecord
	if err := json.Unmarshal(data, &op); err != nil {
		return domain.OperationRecord{}, err
	}
	return op, nil
}

func (j FileJournal) write(ctx context.Context, op domain.OperationRecord) error {
	if err := j.FS.MkdirAll(ctx, j.BaseDir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(op, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return j.FS.WriteFileAtomic(ctx, j.path(op.OperationID), data, 0o644)
}

func (j FileJournal) path(operationID string) string {
	return filepath.Join(j.BaseDir, operationID+".json")
}
func trimExt(name string) string { return name[:len(name)-len(filepath.Ext(name))] }
