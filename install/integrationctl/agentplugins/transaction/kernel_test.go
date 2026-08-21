package transaction

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/777genius/plugin-kit-ai/install/integrationctl/adapters/dirswap"
	"github.com/777genius/plugin-kit-ai/install/integrationctl/agentplugins/adapters/statev2"
	"github.com/777genius/plugin-kit-ai/install/integrationctl/agentplugins/domain"
)

func TestApplyDirectoryCommitsNativeTreeAndStateReceipt(t *testing.T) {
	t.Parallel()
	kernel, mutation, store := transactionFixture(t, "commit-op")
	receipt, err := kernel.ApplyDirectory(context.Background(), mutation)
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if receipt.Phase != ReceiptPhaseCommitted {
		t.Fatalf("receipt = %+v", receipt)
	}
	assertTransactionBody(t, mutation.ActivePath, "new")
	state, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	client := onlyClient(state.Installations[0])
	if client.Materialization != domain.MaterializationMaterialized || client.Verification != domain.VerificationInstalled || len(client.Receipts) != 1 || client.Receipts[0].Phase != ReceiptPhaseCommitted {
		t.Fatalf("client state = %+v", client)
	}
}

func TestApplyDirectoryRollsBackWhenVerificationFails(t *testing.T) {
	t.Parallel()
	kernel, mutation, store := transactionFixture(t, "verify-op")
	mutation.Verify = func(context.Context, string) error { return errors.New("verification failed") }
	if _, err := kernel.ApplyDirectory(context.Background(), mutation); err == nil {
		t.Fatal("verification failure was ignored")
	}
	assertTransactionBody(t, mutation.ActivePath, "old")
	state, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(onlyClient(state.Installations[0]).Receipts) != 0 {
		t.Fatal("failed mutation was persisted")
	}
}

func TestApplyDirectoryGroupRollsBackEveryTargetWhenSecondVerificationFails(t *testing.T) {
	t.Parallel()
	kernel, first, store := transactionFixture(t, "group-first")
	secondActive := filepath.Join(first.OwnedBase, "plugin-second")
	secondStaging := filepath.Join(first.OwnedBase, "plugin-second.staging")
	writeTransactionBody(t, secondActive, "old-second")
	writeTransactionBody(t, secondStaging, "new-second")
	desired := first.DesiredState
	installation := desired.Installations[0]
	secondClientID := domain.ComputeClientBindingID(installation.InstallationID, "cursor", "user", secondActive)
	secondClient := onlyClient(installation)
	secondClient.ClientBindingID = secondClientID
	secondClient.ClientID = "cursor"
	secondClient.TargetLocator = secondActive
	secondClient.PhysicalArtifact = domain.ComputePhysicalArtifactID("demo", installation.InstallationID+"-cursor")
	secondClient.Receipts = nil
	installation.Clients[secondClientID] = secondClient
	desired.Installations[0] = installation
	second := DirectoryMutation{OperationID: "group-second", InstallationID: installation.InstallationID, ClientBindingID: secondClientID,
		Sequence: 1, OwnedBase: first.OwnedBase, ActivePath: secondActive, StagingPath: secondStaging,
		Activation: domain.ActivationPrepared, Authentication: domain.AuthenticationNotRequired, Policy: domain.PolicyAllowed,
		Verification: domain.VerificationInstalled, Verify: func(context.Context, string) error { return errors.New("second verification failed") }}
	_, err := kernel.ApplyDirectoryGroup(context.Background(), DirectoryGroup{OperationGroupID: "group-op", Mutations: []DirectoryMutation{first, second}, DesiredState: desired})
	if err == nil {
		t.Fatal("group verification failure was ignored")
	}
	assertTransactionBody(t, first.ActivePath, "old")
	assertTransactionBody(t, secondActive, "old-second")
	state, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Installations[0].Clients) != 1 || len(onlyClient(state.Installations[0]).Receipts) != 0 {
		t.Fatalf("failed group changed state: %+v", state)
	}
}

func TestApplyDirectoryNeverPersistsDesiredStateBeforeDurableDirectorySuccess(t *testing.T) {
	t.Parallel()
	kernel, mutation, store := transactionFixture(t, "crash-before-state-op")
	mutation.DesiredState.Installations[0].Package.Version = "2.0.0"
	kernel.Directory.Fault = func(phase string) error {
		if phase == dirswap.PhaseActivated {
			return errors.New("simulated process interruption")
		}
		return nil
	}
	if _, err := kernel.ApplyDirectory(context.Background(), mutation); err == nil {
		t.Fatal("simulated interruption did not fail")
	}
	state, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if state.Installations[0].Package.Version == "2.0.0" {
		t.Fatal("desired state became authoritative before directory transaction succeeded")
	}
	assertTransactionBody(t, mutation.ActivePath, "old")
}

func TestRemoveDirectoryCommitsAbsenceAndStateReceipt(t *testing.T) {
	t.Parallel()
	kernel, mutation, store := transactionFixture(t, "remove-op")
	if err := os.RemoveAll(mutation.StagingPath); err != nil {
		t.Fatal(err)
	}
	receipt, err := kernel.RemoveDirectory(context.Background(), DirectoryRemoval{
		OperationID: mutation.OperationID, InstallationID: mutation.InstallationID,
		ClientBindingID: mutation.ClientBindingID, Sequence: 1,
		OwnedBase: mutation.OwnedBase, ActivePath: mutation.ActivePath,
		Verify: func(_ context.Context, active string) error {
			body, err := os.ReadFile(filepath.Join(active, "body"))
			if err != nil || string(body) != "old" {
				return errors.New("unexpected active directory")
			}
			return nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if receipt.MutationType != "directory_remove" || receipt.Phase != ReceiptPhaseCommitted {
		t.Fatalf("receipt = %+v", receipt)
	}
	if _, err := os.Lstat(mutation.ActivePath); !os.IsNotExist(err) {
		t.Fatalf("active path survived removal: %v", err)
	}
	state, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	client := onlyClient(state.Installations[0])
	if client.Materialization != domain.MaterializationAbsent || client.Activation != domain.ActivationNotRequired || client.Verification != domain.VerificationNotRun {
		t.Fatalf("client state = %+v", client)
	}
}

func TestRecoverUsesDurableStateReceiptAsCommitDecision(t *testing.T) {
	t.Parallel()
	for _, stateCommitted := range []bool{false, true} {
		stateCommitted := stateCommitted
		t.Run(map[bool]string{false: "rollback", true: "commit"}[stateCommitted], func(t *testing.T) {
			kernel, mutation, store := transactionFixture(t, "recover-op")
			manager := kernel.Directory
			manager.Fault = func(phase string) error {
				if phase == dirswap.PhaseActivated {
					return errors.New("simulated crash")
				}
				return nil
			}
			directoryReceipt, err := manager.Apply(context.Background(), dirswap.Input{
				OperationID: mutation.OperationID, ClientBindingID: mutation.ClientBindingID, Sequence: mutation.Sequence,
				OwnedBase: mutation.OwnedBase, ActivePath: mutation.ActivePath, StagingPath: mutation.StagingPath,
			})
			if err == nil {
				t.Fatal("simulated crash did not occur")
			}
			if stateCommitted {
				state, loadErr := store.Load()
				if loadErr != nil {
					t.Fatal(loadErr)
				}
				installation := state.Installations[0]
				client := onlyClient(installation)
				client.Receipts = append(client.Receipts, domain.MutationReceipt{
					OperationID: mutation.OperationID, Sequence: 1, MutationType: "directory_swap",
					ClientBindingID: client.ClientBindingID, ActivePath: directoryReceipt.ActivePath,
					StagingPath: directoryReceipt.StagingPath, BackupPath: directoryReceipt.BackupPath,
					Phase: ReceiptPhaseStateCommitted,
				})
				installation.Clients[client.ClientBindingID] = client
				state.Installations[0] = installation
				if saveErr := store.Save(state); saveErr != nil {
					t.Fatal(saveErr)
				}
			}
			kernel.Directory.Fault = nil
			if err := kernel.Recover(context.Background()); err != nil {
				t.Fatalf("recover: %v", err)
			}
			want := "old"
			if stateCommitted {
				want = "new"
			}
			assertTransactionBody(t, mutation.ActivePath, want)
		})
	}
}

func TestRecoverFinalizesStateReceiptAfterJournalWasAlreadyRemoved(t *testing.T) {
	t.Parallel()
	kernel, mutation, store := transactionFixture(t, "post-journal-op")
	state, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	installation := state.Installations[0]
	client := onlyClient(installation)
	client.Receipts = append(client.Receipts, domain.MutationReceipt{
		OperationID: mutation.OperationID, Sequence: 1, MutationType: "directory_swap",
		ClientBindingID: client.ClientBindingID, Phase: ReceiptPhaseStateCommitted,
	})
	installation.Clients[client.ClientBindingID] = client
	state.Installations[0] = installation
	if err := store.Save(state); err != nil {
		t.Fatal(err)
	}
	if err := kernel.Recover(context.Background()); err != nil {
		t.Fatal(err)
	}
	state, err = store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if got := onlyClient(state.Installations[0]).Receipts[0].Phase; got != ReceiptPhaseCommitted {
		t.Fatalf("receipt phase = %q", got)
	}
}

func TestApplyLeavesJournalWhenStateSaveDurabilityRemainsAmbiguous(t *testing.T) {
	t.Parallel()
	kernel, mutation, store := transactionFixture(t, "ambiguous-apply-op")
	faultStore := &postCommitErrorStore{delegate: store, failures: 2}
	kernel.StateStore = faultStore
	if _, err := kernel.ApplyDirectory(context.Background(), mutation); err == nil {
		t.Fatal("ambiguous post-rename save failure was ignored")
	}
	assertTransactionBody(t, mutation.ActivePath, "new")
	open, err := kernel.Directory.ListOpen()
	if err != nil || len(open) != 1 {
		t.Fatalf("open journals = %+v, err=%v", open, err)
	}
	state, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if got := onlyClient(state.Installations[0]).Receipts[0].Phase; got != ReceiptPhaseStateCommitted {
		t.Fatalf("visible receipt phase = %q", got)
	}
	faultStore.failures = 0
	if err := kernel.Recover(context.Background()); err != nil {
		t.Fatal(err)
	}
	assertTransactionBody(t, mutation.ActivePath, "new")
	state, err = store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if got := onlyClient(state.Installations[0]).Receipts[0].Phase; got != ReceiptPhaseCommitted {
		t.Fatalf("recovered receipt phase = %q", got)
	}
}

func TestApplyRetriesVisibleStateAfterStoreSortsNewInstallation(t *testing.T) {
	t.Parallel()
	kernel, mutation, store := transactionFixture(t, "sorted-visible-op")
	newInstallation := mutation.DesiredState.Installations[0]
	newInstallation.InstallationID = "00000000-0000-4000-8000-000000000000"
	newInstallation.Source.SourceBindingID = "src_sorted_new"
	newInstallation.Clients = map[string]domain.ClientBinding{}
	newClientID := domain.ComputeClientBindingID(newInstallation.InstallationID, "codex", "user", mutation.ActivePath)
	client := onlyClient(mutation.DesiredState.Installations[0])
	client.ClientBindingID = newClientID
	client.TargetLocator = mutation.ActivePath
	client.Receipts = nil
	newInstallation.Clients[newClientID] = client
	// Keep the desired slice intentionally unsorted. Store.Save sorts a copy.
	mutation.DesiredState.Installations = append(mutation.DesiredState.Installations, newInstallation)
	mutation.InstallationID = newInstallation.InstallationID
	mutation.ClientBindingID = newClientID
	faultStore := &postCommitErrorStore{delegate: store, failures: 1}
	kernel.StateStore = faultStore
	receipt, err := kernel.ApplyDirectory(context.Background(), mutation)
	if err != nil {
		t.Fatalf("visible sorted state was treated as ambiguous: %v", err)
	}
	if receipt.Phase != ReceiptPhaseCommitted {
		t.Fatalf("receipt = %+v", receipt)
	}
	state, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Installations) != 2 || state.Installations[0].InstallationID != newInstallation.InstallationID {
		t.Fatalf("sorted committed state = %+v", state.Installations)
	}
}

func TestRemoveLeavesJournalWhenStateSaveDurabilityRemainsAmbiguous(t *testing.T) {
	t.Parallel()
	kernel, mutation, store := transactionFixture(t, "ambiguous-remove-op")
	faultStore := &postCommitErrorStore{delegate: store, failures: 2}
	kernel.StateStore = faultStore
	_, err := kernel.RemoveDirectory(context.Background(), DirectoryRemoval{
		OperationID: mutation.OperationID, InstallationID: mutation.InstallationID,
		ClientBindingID: mutation.ClientBindingID, Sequence: mutation.Sequence,
		OwnedBase: mutation.OwnedBase, ActivePath: mutation.ActivePath,
	})
	if err == nil {
		t.Fatal("ambiguous post-rename removal save failure was ignored")
	}
	if _, err := os.Lstat(mutation.ActivePath); !os.IsNotExist(err) {
		t.Fatalf("ambiguous removal restored active path: %v", err)
	}
	open, err := kernel.Directory.ListOpen()
	if err != nil || len(open) != 1 {
		t.Fatalf("open journals = %+v, err=%v", open, err)
	}
	faultStore.failures = 0
	if err := kernel.Recover(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(mutation.ActivePath); !os.IsNotExist(err) {
		t.Fatalf("recovered removal restored active path: %v", err)
	}
	state, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	client := onlyClient(state.Installations[0])
	if client.Materialization != domain.MaterializationAbsent || client.Receipts[0].Phase != ReceiptPhaseCommitted {
		t.Fatalf("recovered removal state = %+v", client)
	}
}

func TestApplyRejectsReusedCompletedOperationIDBeforeFilesystemMutation(t *testing.T) {
	t.Parallel()
	kernel, mutation, store := transactionFixture(t, "reused-op")
	if _, err := kernel.ApplyDirectory(context.Background(), mutation); err != nil {
		t.Fatal(err)
	}
	writeTransactionBody(t, mutation.StagingPath, "second")
	mutation.Sequence = 2
	mutation.DesiredState, _ = store.Load()
	if _, err := kernel.ApplyDirectory(context.Background(), mutation); err == nil {
		t.Fatal("completed operation id was reused")
	}
	assertTransactionBody(t, mutation.ActivePath, "new")
	assertTransactionBody(t, mutation.StagingPath, "second")
	open, err := kernel.Directory.ListOpen()
	if err != nil || len(open) != 0 {
		t.Fatalf("reuse created a journal: %+v, err=%v", open, err)
	}
}

func transactionFixture(t *testing.T, operationID string) (Kernel, DirectoryMutation, statev2.Store) {
	t.Helper()
	root := t.TempDir()
	base := filepath.Join(root, "managed")
	active := filepath.Join(base, "plugin")
	staging := filepath.Join(base, "plugin.staging")
	writeTransactionBody(t, active, "old")
	writeTransactionBody(t, staging, "new")
	store := statev2.Store{Path: filepath.Join(root, "state-v2.json")}
	installationID := "00000000-0000-4000-8000-000000000001"
	clientID := domain.ComputeClientBindingID(installationID, "codex", "user", root)
	state := domain.StateFileV2{
		SchemaVersion: domain.StateSchemaVersion,
		Installations: []domain.Installation{{
			InstallationID: installationID,
			DeclaredName:   "demo",
			Source: domain.SourceBinding{
				SourceBindingID: "src_demo", RequestedSource: "demo", CanonicalSource: "https://example.com/demo",
				ResolvedRevision: "abc", TreeDigest: "sha256:tree",
			},
			Package: domain.PackageBinding{
				LoaderKind: domain.LoaderKindAgentPlugins, FormatID: domain.FormatIDAgentPluginsV1,
				SchemaURI: domain.PluginSchemaV1, DeclaredName: "demo", ManifestDigest: "sha256:manifest",
			},
			Clients: map[string]domain.ClientBinding{
				clientID: {
					ClientBindingID: clientID, ClientID: "codex", Scope: "user", TargetLocator: root,
					PhysicalArtifact: domain.ComputePhysicalArtifactID("demo", installationID),
					Materialization:  domain.MaterializationStaged, Activation: domain.ActivationPrepared,
					Authentication: domain.AuthenticationNotRequired, Policy: domain.PolicyAllowed,
					Verification: domain.VerificationPackageValid,
				},
			},
		}},
	}
	if err := store.Save(state); err != nil {
		t.Fatal(err)
	}
	return Kernel{StateStore: store, Directory: dirswap.Manager{JournalDir: filepath.Join(root, "operations-v2")}}, DirectoryMutation{
		OperationID: operationID, InstallationID: installationID, ClientBindingID: clientID, Sequence: 1,
		OwnedBase: base, ActivePath: active, StagingPath: staging,
		Activation: domain.ActivationPrepared, Authentication: domain.AuthenticationNotRequired,
		Policy: domain.PolicyAllowed, Verification: domain.VerificationInstalled,
		DesiredState: state,
		Verify: func(_ context.Context, activePath string) error {
			body, err := os.ReadFile(filepath.Join(activePath, "body"))
			if err != nil || string(body) != "new" {
				return errors.New("new body not active")
			}
			return nil
		},
	}, store
}

func onlyClient(installation domain.Installation) domain.ClientBinding {
	for _, client := range installation.Clients {
		return client
	}
	return domain.ClientBinding{}
}

func writeTransactionBody(t *testing.T, root, value string) {
	t.Helper()
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "body"), []byte(value), 0o644); err != nil {
		t.Fatal(err)
	}
}

func assertTransactionBody(t *testing.T, root, want string) {
	t.Helper()
	body, err := os.ReadFile(filepath.Join(root, "body"))
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != want {
		t.Fatalf("body = %q, want %q", body, want)
	}
}

type postCommitErrorStore struct {
	delegate StateStore
	failures int
}

func (store *postCommitErrorStore) Load() (domain.StateFileV2, error) {
	return store.delegate.Load()
}

func (store *postCommitErrorStore) Save(state domain.StateFileV2) error {
	if err := store.delegate.Save(state); err != nil {
		return err
	}
	if store.failures > 0 {
		store.failures--
		return errors.New("simulated parent directory sync failure after state rename")
	}
	return nil
}
