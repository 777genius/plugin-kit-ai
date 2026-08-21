package usecase

import (
	"context"
	"errors"
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

func TestGroupedAddReportsCommittedActivationAndExternalPartialFailuresFromReceipts(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name      string
		failCall  int
		wantPhase GroupPhase
		wantFirst GroupTargetPhase
	}{
		{name: "first activation", failCall: 1, wantPhase: GroupPhaseManagedActivationFailed, wantFirst: GroupTargetExternalFailed},
		{name: "second activation", failCall: 2, wantPhase: GroupPhaseExternalPartialFailure, wantFirst: GroupTargetExternalCompleted},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			service, store, cursor := serviceFixture(t)
			service.Activator = &failNthGroupActivator{failCall: test.failCall}
			kiro := domain.DetectedClient{ClientID: domain.ClientKiro, Status: domain.DetectionDetected, ConfigRoot: filepath.Join(t.TempDir(), ".kiro")}
			result, err := service.AddGroup(context.Background(), GroupInput{Targets: []AddInput{
				addInput(t, cursor, "https://example.com/phase-aware"),
				addInput(t, kiro, "https://example.com/phase-aware"),
			}, OperationGroupID: "phase-aware-" + strings.ReplaceAll(test.name, " ", "-"), Confirmed: true})
			if err == nil {
				t.Fatal("activation failure was ignored")
			}
			if result.Phase != test.wantPhase || len(result.Receipts) != 2 || result.Targets[0].GroupPhase != test.wantFirst {
				t.Fatalf("phase-aware result = %+v", result)
			}
			if result.Targets[test.failCall-1].GroupPhase != GroupTargetExternalFailed {
				t.Fatalf("failed target outcome = %+v", result.Targets[test.failCall-1])
			}
			state, loadErr := store.Load()
			if loadErr != nil {
				t.Fatal(loadErr)
			}
			if len(state.Installations) != 1 || len(state.Installations[0].Clients) != 2 {
				t.Fatalf("managed commit was not authoritative: %+v", state)
			}
		})
	}
}

func TestGroupedRepairAtomicallyRestoresHeterogeneousRecordedRevisions(t *testing.T) {
	t.Parallel()
	service, store, cursor := serviceFixture(t)
	kiro := domain.DetectedClient{ClientID: domain.ClientKiro, Status: domain.DetectionDetected, ConfigRoot: filepath.Join(t.TempDir(), ".kiro")}
	cursorV1 := addInput(t, cursor, "https://example.com/mixed-repair")
	kiroV1 := addInput(t, kiro, "https://example.com/mixed-repair")
	added, err := service.AddGroup(context.Background(), GroupInput{Targets: []AddInput{cursorV1, kiroV1}, OperationGroupID: "mixed-add", Confirmed: true})
	if err != nil {
		t.Fatal(err)
	}
	cursorV2 := addInput(t, cursor, "https://example.com/mixed-repair")
	kiroV2 := addInput(t, kiro, "https://example.com/mixed-repair")
	setEnvelopeVersion(t, &cursorV2.Envelope, "2.0.0", "sha256:mixed-v2", "sha256:mixed-manifest-v2")
	setEnvelopeVersion(t, &kiroV2.Envelope, "2.0.0", "sha256:mixed-v2", "sha256:mixed-manifest-v2")
	if _, err := service.UpdateGroup(context.Background(), GroupInput{Targets: []AddInput{cursorV2}, CompatibilityChecks: []AddInput{cursorV2, kiroV2}, OperationGroupID: "mixed-update", Confirmed: true}); err != nil {
		t.Fatal(err)
	}
	cursorV2.InstallationID = added.InstallationID
	kiroV1.InstallationID = added.InstallationID
	repaired, err := service.RepairGroup(context.Background(), GroupInput{Targets: []AddInput{cursorV2, kiroV1}, OperationGroupID: "mixed-repair", Confirmed: true, Repair: true})
	if err != nil {
		t.Fatal(err)
	}
	if repaired.Phase != GroupPhaseCompleted || len(repaired.Receipts) != 2 {
		t.Fatalf("heterogeneous repair result = %+v", repaired)
	}
	state, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	var versions = map[string]string{}
	for _, binding := range state.Installations[0].Clients {
		versions[binding.ClientID] = binding.PackageRevision.Version
	}
	if versions[string(domain.ClientCursor)] != "2.0.0" || versions[string(domain.ClientKiro)] == "2.0.0" {
		t.Fatalf("repair did not preserve exact per-target revisions: %+v", versions)
	}
}

type fixedNativeObserver struct {
	observation domain.NativeIdentityObservation
	err         error
}

type failNthGroupActivator struct {
	calls    int
	failCall int
}

func (activator *failNthGroupActivator) Activate(context.Context, domain.ActivationRequest) (domain.ActivationOutcome, error) {
	activator.calls++
	outcome := domain.ActivationOutcome{Activation: domain.ActivationActive, Authentication: domain.AuthenticationNotRequired,
		Policy: domain.PolicyAllowed, Verification: domain.VerificationInstalled}
	if activator.calls == activator.failCall {
		outcome.Activation = domain.ActivationFailed
		outcome.Verification = domain.VerificationFailed
		return outcome, errors.New("injected activation failure")
	}
	return outcome, nil
}

func (*failNthGroupActivator) Deactivate(context.Context, domain.DeactivationRequest) (domain.DeactivationOutcome, error) {
	return domain.DeactivationOutcome{Activation: domain.ActivationNotRequired, ArtifactRemovalAllowed: true, ExternalRemovalComplete: true}, nil
}

func (observer fixedNativeObserver) ObserveNativeIdentity(context.Context, domain.DetectedClient, domain.DeliveryPlan, *domain.ClientBinding) (domain.NativeIdentityObservation, error) {
	return observer.observation, observer.err
}
