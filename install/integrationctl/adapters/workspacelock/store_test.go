package workspacelock

import (
	"context"
	"path/filepath"
	"testing"

	fsadapter "github.com/777genius/plugin-kit-ai/install/integrationctl/adapters/fs"
	"github.com/777genius/plugin-kit-ai/install/integrationctl/domain"
)

func TestStoreSaveAndLoad(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), ".plugin-kit-ai.lock")
	store := Store{FS: fsadapter.OS{}, File: path}
	want := domain.WorkspaceLock{
		APIVersion: "v1",
		Integrations: []domain.WorkspaceLockIntegration{
			{
				Source:  "./plugins/cursor-demo",
				Version: "1.2.3",
				Targets: []string{"cursor"},
				Policy: domain.InstallPolicy{
					Scope:           "project",
					AutoUpdate:      true,
					AdoptNewTargets: "manual",
				},
			},
		},
	}
	if err := store.Save(context.Background(), want); err != nil {
		t.Fatalf("save: %v", err)
	}
	got, err := store.Load(context.Background())
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got.APIVersion != want.APIVersion {
		t.Fatalf("api_version = %q, want %q", got.APIVersion, want.APIVersion)
	}
	if len(got.Integrations) != 1 {
		t.Fatalf("integration count = %d, want 1", len(got.Integrations))
	}
	if got.Integrations[0].Source != want.Integrations[0].Source {
		t.Fatalf("source = %q, want %q", got.Integrations[0].Source, want.Integrations[0].Source)
	}
	if got.Integrations[0].Policy.Scope != "project" || !got.Integrations[0].Policy.AutoUpdate {
		t.Fatalf("policy = %+v", got.Integrations[0].Policy)
	}
}
