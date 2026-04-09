package journal

import (
	"context"
	"path/filepath"
	"testing"

	fsadapter "github.com/777genius/plugin-kit-ai/install/integrationctl/adapters/fs"
	"github.com/777genius/plugin-kit-ai/install/integrationctl/domain"
)

func TestFileJournalLifecycle(t *testing.T) {
	t.Parallel()
	j := FileJournal{
		FS:      fsadapter.OS{},
		BaseDir: filepath.Join(t.TempDir(), "ops"),
	}
	op := domain.OperationRecord{
		OperationID:   "op1",
		Type:          "add",
		IntegrationID: "demo",
		Status:        "in_progress",
		StartedAt:     "2026-04-09T00:00:00Z",
	}
	if err := j.Start(context.Background(), op); err != nil {
		t.Fatalf("start: %v", err)
	}
	if err := j.AppendStep(context.Background(), "op1", domain.JournalStep{Target: "claude", Action: "plan", Status: "ok"}); err != nil {
		t.Fatalf("append: %v", err)
	}
	open, err := j.ListOpen(context.Background())
	if err != nil {
		t.Fatalf("list open: %v", err)
	}
	if len(open) != 1 {
		t.Fatalf("open count = %d, want 1", len(open))
	}
	if err := j.Finish(context.Background(), "op1", "committed"); err != nil {
		t.Fatalf("finish: %v", err)
	}
	open, err = j.ListOpen(context.Background())
	if err != nil {
		t.Fatalf("list open after finish: %v", err)
	}
	if len(open) != 0 {
		t.Fatalf("open count after finish = %d, want 0", len(open))
	}
}
