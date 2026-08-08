package statemigration

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/777genius/plugin-kit-ai/install/integrationctl/agentplugins/adapters/statev2"
	"github.com/777genius/plugin-kit-ai/install/integrationctl/agentplugins/domain"
)

func TestMigrateCopiesLegacyStateAndPreservesLegacyLoaderBinding(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	legacyPath := filepath.Join(root, "state.json")
	v2Path := filepath.Join(root, "state-v2.json")
	original := `{
  "schema_version": 1,
  "installations": [{
    "integration_id": "context7",
    "requested_source_ref": {"kind":"github_repo_path","value":"context7"},
    "resolved_source_ref": {"kind":"git_commit","value":"https://example.com/catalog@abc"},
    "resolved_version": "1.0.0",
    "source_digest": "sha256:tree",
    "manifest_digest": "sha256:manifest",
    "policy": {"scope":"user"},
    "targets": {
      "codex": {
        "target_id":"codex",
        "state":"activation_pending",
        "activation_state":"native_pending",
        "owned_native_objects":[{"kind":"plugin_root","name":"context7","path":"/test/plugin","protection_class":"user_mutable"}]
      }
    },
    "last_checked_at":"2026-08-01T00:00:00Z",
    "last_updated_at":"2026-08-02T00:00:00Z"
  }]
}`
	if err := os.WriteFile(legacyPath, []byte(original), 0o600); err != nil {
		t.Fatal(err)
	}
	migrator := Migrator{
		LegacyPath: legacyPath,
		V2Store:    statev2.Store{Path: v2Path},
		Now:        func() time.Time { return time.Date(2026, 8, 8, 10, 0, 0, 0, time.UTC) },
		NewID:      func() (string, error) { return "00000000-0000-4000-8000-000000000001", nil },
	}
	report, err := migrator.Migrate()
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if report.Migrated != 1 || report.NeedsRebind != 0 || !report.LegacyStateUntouched {
		t.Fatalf("report = %+v", report)
	}
	backup, err := os.ReadFile(report.BackupPath)
	if err != nil {
		t.Fatal(err)
	}
	legacyAfter, err := os.ReadFile(legacyPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(backup) != original || string(legacyAfter) != original {
		t.Fatal("legacy state or backup changed")
	}
	state, err := migrator.V2Store.Load()
	if err != nil {
		t.Fatal(err)
	}
	installation := state.Installations[0]
	if installation.Package.LoaderKind != domain.LoaderKindLegacy || installation.Package.FormatID != domain.FormatIDLegacyV1 {
		t.Fatalf("legacy binding changed: %+v", installation.Package)
	}
	if installation.NeedsRebind || len(installation.Clients) != 1 {
		t.Fatalf("installation = %+v", installation)
	}
	for _, client := range installation.Clients {
		if client.Materialization != domain.MaterializationMaterialized || client.Activation != domain.ActivationManual || client.Verification != domain.VerificationNotRun {
			t.Fatalf("client states = %+v", client)
		}
	}
}

func TestMigrateMarksAmbiguousSourceForRebind(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	legacyPath := filepath.Join(root, "state.json")
	if err := os.WriteFile(legacyPath, []byte(`{"schema_version":1,"installations":[{"integration_id":"demo","requested_source_ref":{"value":"demo"},"resolved_source_ref":{},"policy":{"scope":"user"},"targets":{}}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	migrator := Migrator{
		LegacyPath: legacyPath,
		V2Store:    statev2.Store{Path: filepath.Join(root, "state-v2.json")},
		NewID:      func() (string, error) { return "00000000-0000-4000-8000-000000000001", nil },
	}
	report, err := migrator.Migrate()
	if err != nil {
		t.Fatal(err)
	}
	if report.NeedsRebind != 1 {
		t.Fatalf("report = %+v", report)
	}
	state, err := migrator.V2Store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if !state.Installations[0].NeedsRebind {
		t.Fatal("ambiguous installation was not marked needs_rebind")
	}
}

func TestMigrateSeparatesDuplicateLegacySourceBindings(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	legacyPath := filepath.Join(root, "state.json")
	body := `{"schema_version":1,"installations":[
{"integration_id":"first","requested_source_ref":{"value":"same"},"resolved_source_ref":{"value":"https://example.com/same@abc"},"source_digest":"sha256:one","policy":{"scope":"user"},"targets":{}},
{"integration_id":"second","requested_source_ref":{"value":"same"},"resolved_source_ref":{"value":"https://example.com/same@abc"},"source_digest":"sha256:two","policy":{"scope":"user"},"targets":{}}
]}`
	if err := os.WriteFile(legacyPath, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	next := 0
	migrator := Migrator{
		LegacyPath: legacyPath, V2Store: statev2.Store{Path: filepath.Join(root, "state-v2.json")},
		NewID: func() (string, error) {
			next++
			return fmt.Sprintf("00000000-0000-4000-8000-%012d", next), nil
		},
	}
	plan, err := migrator.Plan()
	if err != nil || plan.NeedsRebind != 2 {
		t.Fatalf("plan = %+v, err=%v", plan, err)
	}
	report, err := migrator.MigrateExpected(plan.LegacyDigest)
	if err != nil || report.NeedsRebind != 2 {
		t.Fatalf("report = %+v, err=%v", report, err)
	}
	state, err := migrator.V2Store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if state.Installations[0].Source.SourceBindingID == state.Installations[1].Source.SourceBindingID || !state.Installations[0].NeedsRebind || !state.Installations[1].NeedsRebind {
		t.Fatalf("installations = %+v", state.Installations)
	}
}

func TestMigrateRefusesToOverwriteExistingV2State(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	legacyPath := filepath.Join(root, "state.json")
	v2Path := filepath.Join(root, "state-v2.json")
	if err := os.WriteFile(legacyPath, []byte(`{"schema_version":1,"installations":[]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(v2Path, []byte(`{"schema_version":2,"installations":[]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := (Migrator{LegacyPath: legacyPath, V2Store: statev2.Store{Path: v2Path}}).Migrate()
	if err == nil {
		t.Fatal("existing v2 state was overwritten")
	}
}

func TestMigrateExpectedRejectsLegacyStateChangedAfterPlan(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	legacyPath := filepath.Join(root, "state.json")
	if err := os.WriteFile(legacyPath, []byte(`{"schema_version":1,"installations":[]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	migrator := Migrator{LegacyPath: legacyPath, V2Store: statev2.Store{Path: filepath.Join(root, "state-v2.json")}}
	plan, err := migrator.Plan()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(legacyPath, []byte(`{"schema_version":1,"installations":[]} `), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := migrator.MigrateExpected(plan.LegacyDigest); err == nil {
		t.Fatal("changed legacy state was migrated without a new review")
	}
	if _, err := os.Stat(migrator.V2Store.Path); !os.IsNotExist(err) {
		t.Fatalf("state v2 was created: %v", err)
	}
}
