package providers

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestPluginDataReceiptPreservesAndPurgesOnlyOwnedDirectory(t *testing.T) {
	t.Parallel()
	manager := PluginDataManager{Base: filepath.Join(t.TempDir(), "data")}
	receipt, created, err := manager.EnsureData(context.Background(), "installation", "backend", "user")
	if err != nil || !created {
		t.Fatalf("ensure data: created=%v err=%v", created, err)
	}
	marker := filepath.Join(receipt.Locator, "persistent-value")
	if err := os.WriteFile(marker, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	reused, created, err := manager.EnsureData(context.Background(), "installation", "backend", "user")
	if err != nil || created || reused.DataReceiptID != receipt.DataReceiptID {
		t.Fatalf("reuse = %+v created=%v err=%v", reused, created, err)
	}
	if body, err := os.ReadFile(marker); err != nil || string(body) != "keep" {
		t.Fatalf("persistent data was replaced: %q %v", body, err)
	}
	stale := receipt
	stale.OwnershipDigest = "sha256:stale"
	if err := manager.PurgeData(context.Background(), stale); err == nil {
		t.Fatal("stale receipt purged data")
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("stale purge mutated data: %v", err)
	}
	if err := manager.PurgeData(context.Background(), receipt); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(receipt.Locator); !os.IsNotExist(err) {
		t.Fatalf("owned data survived purge: %v", err)
	}
}
