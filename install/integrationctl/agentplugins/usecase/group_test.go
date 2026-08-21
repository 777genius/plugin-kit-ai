package usecase

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/777genius/plugin-kit-ai/install/integrationctl/agentplugins/domain"
)

func TestGroupedAddUpdateAndRemoveUseOneCommitDecision(t *testing.T) {
	t.Parallel()
	service, store, cursor := serviceFixture(t)
	kiro := domain.DetectedClient{ClientID: domain.ClientKiro, Status: domain.DetectionDetected, ConfigRoot: filepath.Join(t.TempDir(), ".kiro")}

	cursorAdd := addInput(t, cursor, "https://example.com/grouped")
	kiroAdd := addInput(t, kiro, "https://example.com/grouped")
	added, err := service.AddGroup(context.Background(), GroupInput{
		Targets: []AddInput{cursorAdd, kiroAdd}, OperationGroupID: "group-add", Confirmed: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !added.Mutated || len(added.Receipts) != 2 {
		t.Fatalf("grouped add = %+v", added)
	}
	state, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Installations) != 1 || len(state.Installations[0].Clients) != 2 || state.Installations[0].OperationGroupID != "group-add" {
		t.Fatalf("grouped add state = %+v", state.Installations)
	}
	for _, binding := range state.Installations[0].Clients {
		if binding.PackageRevision == nil || binding.PackageRevision.TreeDigest != "sha256:source-tree" || len(binding.Receipts) != 1 || binding.Receipts[0].OperationGroupID != "group-add" {
			t.Fatalf("grouped add binding = %+v", binding)
		}
	}

	cursorUpdate := addInput(t, cursor, "https://example.com/grouped")
	kiroUpdate := addInput(t, kiro, "https://example.com/grouped")
	setEnvelopeVersion(t, &cursorUpdate.Envelope, "2.0.0", "sha256:group-tree-v2", "sha256:group-manifest-v2")
	setEnvelopeVersion(t, &kiroUpdate.Envelope, "2.0.0", "sha256:group-tree-v2", "sha256:group-manifest-v2")
	updated, err := service.UpdateGroup(context.Background(), GroupInput{
		Targets: []AddInput{cursorUpdate, kiroUpdate}, CompatibilityChecks: []AddInput{cursorUpdate, kiroUpdate},
		OperationGroupID: "group-update", Confirmed: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !updated.Mutated || len(updated.Receipts) != 2 {
		t.Fatalf("grouped update = %+v", updated)
	}
	state, err = store.Load()
	if err != nil {
		t.Fatal(err)
	}
	for _, binding := range state.Installations[0].Clients {
		if binding.PackageRevision == nil || binding.PackageRevision.Version != "2.0.0" || binding.PackageRevision.TreeDigest != "sha256:group-tree-v2" || len(binding.Receipts) != 2 {
			t.Fatalf("grouped update binding = %+v", binding)
		}
	}

	removed, err := service.RemoveGroup(context.Background(), RemoveGroupInput{
		Selector: added.InstallationID,
		Targets: []RemoveInput{
			{Client: cursor, Scope: domain.ScopeUser, ExternalUninstalled: true},
			{Client: kiro, Scope: domain.ScopeUser, ExternalUninstalled: true},
		},
		OperationGroupID: "group-remove", Confirmed: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !removed.Mutated || len(removed.Receipts) != 2 {
		t.Fatalf("grouped remove = %+v", removed)
	}
	state, err = store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Installations) != 1 || len(state.Installations[0].Clients) != 0 || !state.Installations[0].DataRetained {
		t.Fatalf("grouped remove state = %+v", state.Installations)
	}
	for _, target := range added.Targets {
		if _, err := os.Lstat(target.Plan.ActivePath); !os.IsNotExist(err) {
			t.Fatalf("grouped remove retained %s: %v", target.Plan.ActivePath, err)
		}
	}
}

func TestGroupedPreflightFailureMutatesNoTarget(t *testing.T) {
	t.Parallel()
	service, store, cursor := serviceFixture(t)
	kiro := domain.DetectedClient{ClientID: domain.ClientKiro, Status: domain.DetectionDetected, ConfigRoot: filepath.Join(t.TempDir(), ".kiro")}
	first := addInput(t, cursor, "https://example.com/no-partial")
	unsupportedScope := addInput(t, kiro, "https://example.com/no-partial")
	unsupportedScope.Scope = domain.ScopeProject
	result, err := service.AddGroup(context.Background(), GroupInput{
		Targets: []AddInput{first, unsupportedScope}, OperationGroupID: "group-preflight", Confirmed: true,
	})
	if err == nil || !strings.Contains(err.Error(), "scope is not proven") {
		t.Fatalf("group preflight error = %v", err)
	}
	state, loadErr := store.Load()
	if loadErr != nil {
		t.Fatal(loadErr)
	}
	if len(state.Installations) != 0 {
		t.Fatalf("failed group preflight changed state: %+v", state.Installations)
	}
	if len(result.Targets) > 0 && result.Targets[0].Plan.ActivePath != "" {
		if _, statErr := os.Lstat(result.Targets[0].Plan.ActivePath); !os.IsNotExist(statErr) {
			t.Fatalf("failed group preflight changed first target: %v", statErr)
		}
	}
}

func TestGroupedRepairAllowsRecordedObjectToBeAbsentButNeverAdoptsForeignIdentity(t *testing.T) {
	t.Parallel()
	managed := &domain.ClientBinding{NativeObjects: []domain.NativeObjectOwnership{{Kind: "managed_package_directory", ManagedDigest: "sha256:owned"}}}
	client := domain.DetectedClient{ClientID: domain.ClientCursor}
	plan := domain.DeliveryPlan{ClientID: domain.ClientCursor}

	service := Service{NativeObserver: fixedNativeObserver{observation: domain.NativeIdentityObservation{State: domain.NativeIdentityAbsent}}}
	if err := service.observeGroupNativeIdentity(context.Background(), client, plan, managed, true); err != nil {
		t.Fatalf("absent recorded repair target was rejected: %v", err)
	}

	service.NativeObserver = fixedNativeObserver{observation: domain.NativeIdentityObservation{State: domain.NativeIdentityUnmanaged}}
	if err := service.observeGroupNativeIdentity(context.Background(), client, plan, managed, true); err == nil || !strings.Contains(err.Error(), "automatic adoption is disabled") {
		t.Fatalf("foreign repair target was accepted: %v", err)
	}

	service.NativeObserver = fixedNativeObserver{observation: domain.NativeIdentityObservation{State: domain.NativeIdentityManaged, Digest: "sha256:different"}}
	if err := service.observeGroupNativeIdentity(context.Background(), client, plan, managed, true); err == nil || !strings.Contains(err.Error(), "does not match") {
		t.Fatalf("stale managed repair target was accepted: %v", err)
	}
}

type fixedNativeObserver struct {
	observation domain.NativeIdentityObservation
	err         error
}

func (observer fixedNativeObserver) ObserveNativeIdentity(context.Context, domain.DetectedClient, domain.DeliveryPlan, *domain.ClientBinding) (domain.NativeIdentityObservation, error) {
	return observer.observation, observer.err
}
