package statemigration

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/777genius/plugin-kit-ai/install/integrationctl/adapters/atomicfile"
	"github.com/777genius/plugin-kit-ai/install/integrationctl/agentplugins/adapters/statev2"
	"github.com/777genius/plugin-kit-ai/install/integrationctl/agentplugins/domain"
)

type Migrator struct {
	LegacyPath string
	V2Store    statev2.Store
	Now        func() time.Time
	NewID      func() (string, error)
}

type Report struct {
	BackupPath           string
	Migrated             int
	NeedsRebind          int
	LegacyStateUntouched bool
}

type Plan struct {
	LegacyDigest  string `json:"legacy_digest"`
	Installations int    `json:"installations"`
	NeedsRebind   int    `json:"needs_rebind"`
}

type legacyState struct {
	SchemaVersion int                  `json:"schema_version"`
	Installations []legacyInstallation `json:"installations"`
}

type legacyInstallation struct {
	IntegrationID      string                  `json:"integration_id"`
	RequestedSourceRef legacySourceRef         `json:"requested_source_ref"`
	ResolvedSourceRef  legacySourceRef         `json:"resolved_source_ref"`
	ResolvedVersion    string                  `json:"resolved_version"`
	SourceDigest       string                  `json:"source_digest"`
	ManifestDigest     string                  `json:"manifest_digest"`
	Policy             legacyPolicy            `json:"policy"`
	WorkspaceRoot      string                  `json:"workspace_root"`
	Targets            map[string]legacyTarget `json:"targets"`
	LastCheckedAt      string                  `json:"last_checked_at"`
	LastUpdatedAt      string                  `json:"last_updated_at"`
}

type legacySourceRef struct {
	Kind  string `json:"kind"`
	Value string `json:"value"`
}

type legacyPolicy struct {
	Scope string `json:"scope"`
}

type legacyTarget struct {
	TargetID             string               `json:"target_id"`
	State                string               `json:"state"`
	ActivationState      string               `json:"activation_state"`
	InteractiveAuthState string               `json:"interactive_auth_state"`
	OwnedNativeObjects   []legacyNativeObject `json:"owned_native_objects"`
}

type legacyNativeObject struct {
	Kind            string `json:"kind"`
	Name            string `json:"name"`
	Path            string `json:"path"`
	ProtectionClass string `json:"protection_class"`
}

func (migrator Migrator) Plan() (Plan, error) {
	body, legacy, err := migrator.load()
	if err != nil {
		return Plan{}, err
	}
	plan := Plan{LegacyDigest: digest(body), Installations: len(legacy.Installations)}
	duplicates := duplicateLegacySourceBindings(legacy.Installations)
	for _, record := range legacy.Installations {
		if needsRebind(record) || duplicates[legacySourceBindingID(record)] {
			plan.NeedsRebind++
		}
	}
	return plan, nil
}

func (migrator Migrator) Migrate() (Report, error) {
	return migrator.MigrateExpected("")
}

func (migrator Migrator) MigrateExpected(expectedDigest string) (Report, error) {
	body, legacy, err := migrator.load()
	if err != nil {
		return Report{}, err
	}
	actualDigest := digest(body)
	if strings.TrimSpace(expectedDigest) != "" && expectedDigest != actualDigest {
		return Report{}, fmt.Errorf("legacy state changed after review; run migrate-state again")
	}
	now := time.Now().UTC()
	if migrator.Now != nil {
		now = migrator.Now().UTC()
	}
	newID := domain.NewInstallationID
	if migrator.NewID != nil {
		newID = migrator.NewID
	}
	state := domain.StateFileV2{SchemaVersion: domain.StateSchemaVersion}
	report := Report{LegacyStateUntouched: true}
	for _, record := range legacy.Installations {
		installation, err := migrateInstallation(record, now, newID)
		if err != nil {
			return report, err
		}
		if installation.NeedsRebind {
			report.NeedsRebind++
		}
		state.Installations = append(state.Installations, installation)
		report.Migrated++
	}
	deduplicateMigratedSourceBindings(state.Installations)
	report.NeedsRebind = 0
	for _, installation := range state.Installations {
		if installation.NeedsRebind {
			report.NeedsRebind++
		}
	}
	if err := statev2.Validate(state); err != nil {
		return report, fmt.Errorf("validate complete migrated state before backup: %w", err)
	}
	backupPath := migrator.LegacyPath + ".backup-agentplugins-" + now.Format("20060102T150405.000000000Z") + "-" + strings.TrimPrefix(actualDigest, "sha256:")[:12]
	if _, err := os.Lstat(backupPath); err == nil {
		return report, fmt.Errorf("legacy state backup already exists: %s", backupPath)
	} else if !os.IsNotExist(err) {
		return report, fmt.Errorf("inspect legacy state backup: %w", err)
	}
	if err := atomicfile.Write(backupPath, body, 0o600); err != nil {
		return report, fmt.Errorf("backup legacy state: %w", err)
	}
	report.BackupPath = backupPath
	if err := migrator.V2Store.Save(state); err != nil {
		return report, fmt.Errorf("commit migrated Agent Plugins state: %w", err)
	}
	return report, nil
}

func (migrator Migrator) load() ([]byte, legacyState, error) {
	if strings.TrimSpace(migrator.LegacyPath) == "" || strings.TrimSpace(migrator.V2Store.Path) == "" {
		return nil, legacyState{}, fmt.Errorf("legacy and Agent Plugins state paths are required")
	}
	if _, err := os.Stat(migrator.V2Store.Path); err == nil {
		return nil, legacyState{}, fmt.Errorf("Agent Plugins state already exists; migration is not idempotent over authoritative state")
	} else if !os.IsNotExist(err) {
		return nil, legacyState{}, err
	}
	body, err := os.ReadFile(migrator.LegacyPath)
	if err != nil {
		return nil, legacyState{}, fmt.Errorf("read legacy state: %w", err)
	}
	var legacy legacyState
	if err := json.Unmarshal(body, &legacy); err != nil {
		return nil, legacyState{}, fmt.Errorf("decode legacy state: %w", err)
	}
	if legacy.SchemaVersion != 0 && legacy.SchemaVersion != 1 {
		return nil, legacyState{}, fmt.Errorf("unsupported legacy state schema_version %d", legacy.SchemaVersion)
	}
	return body, legacy, nil
}

func digest(body []byte) string {
	sum := sha256.Sum256(body)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func needsRebind(record legacyInstallation) bool {
	return strings.TrimSpace(record.RequestedSourceRef.Value) == "" ||
		strings.TrimSpace(record.ResolvedSourceRef.Value) == "" ||
		strings.TrimSpace(record.SourceDigest) == ""
}

func legacySourceBindingID(record legacyInstallation) string {
	requested := strings.TrimSpace(record.RequestedSourceRef.Value)
	canonical := strings.TrimSpace(record.ResolvedSourceRef.Value)
	if canonical == "" {
		canonical = requested
	}
	return domain.ComputeSourceBindingID(domain.SourceIdentity{RequestedSource: requested, CanonicalSource: canonical})
}

func duplicateLegacySourceBindings(records []legacyInstallation) map[string]bool {
	counts := map[string]int{}
	for _, record := range records {
		counts[legacySourceBindingID(record)]++
	}
	duplicates := map[string]bool{}
	for sourceID, count := range counts {
		duplicates[sourceID] = count > 1
	}
	return duplicates
}

func deduplicateMigratedSourceBindings(installations []domain.Installation) {
	counts := map[string]int{}
	for _, installation := range installations {
		counts[installation.Source.SourceBindingID]++
	}
	for index := range installations {
		installation := &installations[index]
		if counts[installation.Source.SourceBindingID] < 2 {
			continue
		}
		original := installation.Source.SourceBindingID
		sum := sha256.Sum256([]byte(original + "\x00" + installation.InstallationID))
		installation.Source.SourceBindingID = "legacy_src_" + hex.EncodeToString(sum[:12])
		installation.NeedsRebind = true
	}
}

func migrateInstallation(record legacyInstallation, now time.Time, newID func() (string, error)) (domain.Installation, error) {
	installationID, err := newID()
	if err != nil {
		return domain.Installation{}, err
	}
	requested := strings.TrimSpace(record.RequestedSourceRef.Value)
	canonical := strings.TrimSpace(record.ResolvedSourceRef.Value)
	if canonical == "" {
		canonical = requested
	}
	sourceBindingID := legacySourceBindingID(record)
	requiresRebind := needsRebind(record)
	clients := map[string]domain.ClientBinding{}
	targetNames := make([]string, 0, len(record.Targets))
	for targetName := range record.Targets {
		targetNames = append(targetNames, targetName)
	}
	sort.Strings(targetNames)
	for _, targetName := range targetNames {
		target := record.Targets[targetName]
		clientID := strings.TrimSpace(target.TargetID)
		if clientID == "" {
			clientID = targetName
		}
		targetLocator := "user"
		if strings.EqualFold(record.Policy.Scope, "project") && strings.TrimSpace(record.WorkspaceRoot) != "" {
			targetLocator = filepath.Clean(record.WorkspaceRoot)
		}
		bindingID := domain.ComputeClientBindingID(installationID, clientID, record.Policy.Scope, targetLocator)
		physicalID := domain.ComputePhysicalArtifactID(record.IntegrationID, installationID+"\x00"+clientID)
		client := domain.ClientBinding{
			ClientBindingID:  bindingID,
			ClientID:         clientID,
			Scope:            record.Policy.Scope,
			TargetLocator:    targetLocator,
			PhysicalArtifact: physicalID,
			Materialization:  migrateMaterialization(target.State),
			Activation:       migrateActivation(target.ActivationState),
			Authentication:   migrateAuthentication(target.InteractiveAuthState),
			Policy:           domain.PolicyAllowed,
			Verification:     domain.VerificationNotRun,
			UpdatedAt:        firstNonEmpty(record.LastUpdatedAt, now.Format(time.RFC3339)),
		}
		for _, object := range target.OwnedNativeObjects {
			client.NativeObjects = append(client.NativeObjects, domain.NativeObjectOwnership{
				ObjectID:        nativeObjectID(bindingID, object),
				Kind:            object.Kind,
				LogicalName:     object.Name,
				Path:            object.Path,
				ProtectionClass: object.ProtectionClass,
			})
		}
		clients[bindingID] = client
	}
	createdAt := firstNonEmpty(record.LastCheckedAt, record.LastUpdatedAt, now.Format(time.RFC3339))
	updatedAt := firstNonEmpty(record.LastUpdatedAt, record.LastCheckedAt, now.Format(time.RFC3339))
	return domain.Installation{
		InstallationID: installationID,
		DeclaredName:   record.IntegrationID,
		Source: domain.SourceBinding{
			SourceBindingID:  sourceBindingID,
			RequestedSource:  requested,
			CanonicalSource:  canonical,
			ResolvedRevision: record.ResolvedSourceRef.Value,
			TreeDigest:       record.SourceDigest,
		},
		Package: domain.PackageBinding{
			LoaderKind:     domain.LoaderKindLegacy,
			FormatID:       domain.FormatIDLegacyV1,
			SchemaURI:      "plugin.yaml/v1",
			DeclaredName:   record.IntegrationID,
			Version:        record.ResolvedVersion,
			ManifestDigest: record.ManifestDigest,
		},
		Clients:     clients,
		NeedsRebind: requiresRebind,
		CreatedAt:   createdAt,
		UpdatedAt:   updatedAt,
	}, nil
}

func nativeObjectID(bindingID string, object legacyNativeObject) string {
	sum := sha256.Sum256([]byte(strings.Join([]string{bindingID, object.Kind, object.Name, object.Path}, "\x00")))
	return "object_" + hex.EncodeToString(sum[:12])
}

func migrateMaterialization(state string) domain.MaterializationState {
	switch strings.ToLower(strings.TrimSpace(state)) {
	case "removed", "absent":
		return domain.MaterializationAbsent
	case "installed", "activation_pending", "auth_pending", "prepared":
		return domain.MaterializationMaterialized
	default:
		return domain.MaterializationDegraded
	}
}

func migrateActivation(state string) domain.ActivationState {
	switch strings.ToLower(strings.TrimSpace(state)) {
	case "active":
		return domain.ActivationActive
	case "not_required", "":
		return domain.ActivationNotRequired
	case "native_pending", "reload_pending", "restart_pending":
		return domain.ActivationManual
	default:
		return domain.ActivationPrepared
	}
}

func migrateAuthentication(state string) domain.AuthenticationState {
	switch strings.ToLower(strings.TrimSpace(state)) {
	case "auth_pending", "pending":
		return domain.AuthenticationPending
	case "authenticated", "complete":
		return domain.AuthenticationComplete
	case "failed":
		return domain.AuthenticationFailed
	default:
		return domain.AuthenticationNotRequired
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
