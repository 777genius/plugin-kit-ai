package jsonstate

import (
	"context"
	"path/filepath"
	"testing"

	fsadapter "github.com/777genius/plugin-kit-ai/install/integrationctl/adapters/fs"
	"github.com/777genius/plugin-kit-ai/install/integrationctl/ports"
)

func TestStoreLoadSaveRoundTrip(t *testing.T) {
	t.Parallel()
	store := Store{
		FS:   fsadapter.OS{},
		Path: filepath.Join(t.TempDir(), "state.json"),
	}
	state := ports.StateFile{SchemaVersion: 1}
	if err := store.Save(context.Background(), state); err != nil {
		t.Fatalf("save: %v", err)
	}
	got, err := store.Load(context.Background())
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got.SchemaVersion != 1 {
		t.Fatalf("schema_version = %d, want 1", got.SchemaVersion)
	}
}
