package agentpluginscli

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/777genius/plugin-kit-ai/install/integrationctl/agentplugins/adapters/directoryv1"
	"github.com/777genius/plugin-kit-ai/install/integrationctl/agentplugins/domain"
)

type rolloutDirectoryFixture struct {
	cli       cliFixture
	directory *fixedDirectoryClient
	acquirer  *localBackedSourceAcquirer
	v1        rolloutRevision
	v2        rolloutRevision
}

type rolloutRevision struct {
	root, revision, tree, manifest, version string
}

func newRolloutDirectoryFixture(t *testing.T, clients, releaseTwoTargets []domain.ClientID) rolloutDirectoryFixture {
	t.Helper()
	detected := make([]domain.DetectedClient, 0, len(clients))
	for _, clientID := range clients {
		detected = append(detected, fixtureClient(t, clientID))
	}
	fixture := newCLIFixture(t, detected)
	loadRevision := func(version, revision string) rolloutRevision {
		root := writeCLIPlugin(t)
		if version != "1.0.0" {
			manifest := `{"$schema":"https://agent-plugins.org/schemas/1.0.0/plugin.schema.json","name":"demo","version":"` + version + `","description":"rollout"}`
			if err := os.WriteFile(filepath.Join(root, "plugin.json"), []byte(manifest), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		loaded, err := fixture.app.acquireLocal(context.Background(), root)
		if err != nil {
			t.Fatal(err)
		}
		result := rolloutRevision{root: root, revision: revision, tree: loaded.envelope.TreeDigest, manifest: loaded.envelope.ManifestDigest, version: version}
		if err := loaded.cleanup(); err != nil {
			t.Fatal(err)
		}
		return result
	}
	v1 := loadRevision("1.0.0", strings.Repeat("1", 40))
	v2 := loadRevision("2.0.0", strings.Repeat("2", 40))
	release := func(sequence uint64, revision rolloutRevision) domain.DirectoryRelease {
		return domain.DirectoryRelease{Sequence: sequence, PackageVersion: revision.version, ManifestName: "demo",
			AgentPluginsSchema:  "https://agent-plugins.org/schemas/1.0.0/plugin.schema.json",
			PackageSource:       domain.DirectorySource{Repository: "owner/rollout", Revision: revision.revision, Path: "plugin"},
			TreeDigestAlgorithm: domain.TreeDigestAlgorithm, TreeDigest: revision.tree, ManifestDigest: revision.manifest,
			Components: []string{}, PublishedAt: "2026-08-21T00:00:00Z"}
	}
	policy := func(sequence uint64, targets []domain.ClientID) domain.DirectoryReleasePolicy {
		entries := make([]domain.DirectoryTarget, 0, len(targets))
		evidence := make([]string, 0, len(targets))
		for _, target := range targets {
			delivery, _ := domain.ExpectedDirectoryDelivery(target)
			entries = append(entries, domain.DirectoryTarget{Client: target, Scopes: []domain.InstallScope{domain.ScopeUser}, Delivery: delivery, Authentication: domain.AuthenticationRequirementUnknown})
			evidence = append(evidence, "passed/materialization/"+string(target)+"/"+strconv.FormatUint(sequence, 10))
		}
		status := domain.ReleaseActive
		if sequence == 2 {
			status = domain.ReleaseSuperseded
		}
		return domain.DirectoryReleasePolicy{ReleaseSequence: sequence, Status: status, MinimumInstallerVersion: "0.1.0", Targets: entries, CurrentEvidence: evidence}
	}
	distribution := domain.DirectoryDistribution{SchemaVersion: 1, ID: "owner/rollout", ProductID: "rollout-demo", Kind: domain.DistributionUpstream,
		Status: domain.DistributionActive, Packager: "owner", Releases: []domain.DirectoryRelease{release(1, v1), release(2, v2)},
		ReleasePolicies: []domain.DirectoryReleasePolicy{policy(1, clients), policy(2, releaseTwoTargets)}}
	snapshot := domain.DirectorySnapshot{SnapshotSchemaVersion: 1, Sequence: 2, SourceCommit: strings.Repeat("a", 40),
		Products: []domain.DirectoryProduct{{SchemaVersion: 1, ID: "rollout-demo", DisplayName: "Rollout Demo", Description: "Rollout Demo", ManifestName: "demo",
			Aliases: []string{"rollout-demo"}, ReservedAliases: []string{"rollout-demo"}, Categories: []string{},
			MinimumCapabilities: domain.DirectoryMinimumCapabilities{Skills: "optional", MCP: "optional"}, DefaultDistribution: "owner/rollout", Distributions: []string{"owner/rollout"}}},
		Distributions: []domain.DirectoryDistribution{distribution}, Evidence: []domain.DirectoryEvidence{}, Revocations: []domain.DirectoryRevocation{}}
	for releaseIndex, release := range distribution.Releases {
		for targetIndex, target := range distribution.ReleasePolicies[releaseIndex].Targets {
			snapshot.Evidence = append(snapshot.Evidence, intendedTrustedDirectoryEvidence(domain.DirectoryEvidence{
				ID: distribution.ReleasePolicies[releaseIndex].CurrentEvidence[targetIndex], DistributionID: distribution.ID,
				ReleaseSequence: release.Sequence, PackageTreeDigest: release.TreeDigest,
				Level: "materialization", Outcome: "passed", Client: target.Client,
			}))
		}
	}
	directory := &fixedDirectoryClient{bundle: directoryv1.VerifiedBundle{Snapshot: snapshot, Digest: "sha256:" + strings.Repeat("a", 64)}}
	acquirer := &localBackedSourceAcquirer{delegate: fixture.app.SourceAcquirer,
		revisionRoots: map[string]string{v1.revision: v1.root, v2.revision: v2.root}, revisionCalls: map[string]int{}}
	fixture.app.DirectoryClient = directory
	fixture.app.SourceAcquirer = acquirer
	return rolloutDirectoryFixture{cli: fixture, directory: directory, acquirer: acquirer, v1: v1, v2: v2}
}

func (fixture rolloutDirectoryFixture) publishV2() {
	fixture.directory.bundle.Snapshot.Distributions[0].ReleasePolicies[1].Status = domain.ReleaseActive
}

func TestPartialDirectoryUpdateConvergesToAlreadyAcceptedDesiredRelease(t *testing.T) {
	t.Parallel()
	rollout := newRolloutDirectoryFixture(t,
		[]domain.ClientID{domain.ClientCodex, domain.ClientCursor},
		[]domain.ClientID{domain.ClientCodex, domain.ClientCursor})
	if _, _, err := rollout.cli.execute(false, "add", "rollout-demo", "--target", "codex,cursor"); err != nil {
		t.Fatal(err)
	}
	rollout.publishV2()
	if _, _, err := rollout.cli.execute(false, "update", "demo", "--target", "codex"); err != nil {
		t.Fatal(err)
	}
	state, err := rollout.cli.store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if state.Installations[0].Directory.DesiredReleaseSequence != 2 || bindingRelease(state.Installations[0], domain.ClientCodex) != 2 || bindingRelease(state.Installations[0], domain.ClientCursor) != 1 {
		t.Fatalf("partial rollout = %+v", state.Installations[0])
	}
	if state.Installations[0].Source.ResolvedRevision != rollout.v2.revision || bindingResolvedRevision(state.Installations[0], domain.ClientCodex) != rollout.v2.revision || bindingResolvedRevision(state.Installations[0], domain.ClientCursor) != rollout.v1.revision {
		t.Fatalf("higher release did not preserve baseline A and bind update B correctly: %+v", state.Installations[0])
	}
	rollout.acquirer.revisionCalls = map[string]int{}
	if _, _, err := rollout.cli.execute(false, "update", "demo", "--target", "cursor"); err != nil {
		t.Fatalf("converge remaining binding to desired release: %v", err)
	}
	state, err = rollout.cli.store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if bindingRelease(state.Installations[0], domain.ClientCodex) != 2 || bindingRelease(state.Installations[0], domain.ClientCursor) != 2 || rollout.acquirer.revisionCalls[rollout.v2.revision] != 1 {
		t.Fatalf("desired-release convergence = state:%+v acquisitions:%v", state.Installations[0], rollout.acquirer.revisionCalls)
	}
}

func TestDesiredReleaseConvergenceStillRejectsRevokedRelease(t *testing.T) {
	t.Parallel()
	rollout := newRolloutDirectoryFixture(t,
		[]domain.ClientID{domain.ClientCodex, domain.ClientCursor},
		[]domain.ClientID{domain.ClientCodex, domain.ClientCursor})
	if _, _, err := rollout.cli.execute(false, "add", "rollout-demo", "--target", "codex,cursor"); err != nil {
		t.Fatal(err)
	}
	rollout.publishV2()
	if _, _, err := rollout.cli.execute(false, "update", "demo", "--target", "codex"); err != nil {
		t.Fatal(err)
	}
	rollout.directory.bundle.Snapshot.Revocations = []domain.DirectoryRevocation{{DistributionID: "owner/rollout", ReleaseSequence: 2}}
	if _, _, err := rollout.cli.execute(false, "update", "demo", "--target", "cursor"); err == nil || !strings.Contains(err.Error(), "revoked") {
		t.Fatalf("revoked desired convergence = %v", err)
	}
	state, err := rollout.cli.store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if bindingRelease(state.Installations[0], domain.ClientCursor) != 1 || state.Installations[0].Directory.DesiredReleaseSequence != 2 {
		t.Fatalf("revoked convergence mutated state: %+v", state.Installations[0])
	}
}

func TestDirectoryInvalidDeliveryFailsBeforeAcquisitionOrMutation(t *testing.T) {
	for _, test := range []struct {
		name, delivery string
	}{
		{name: "unknown", delivery: "future_delivery"},
		{name: "mismatched", delivery: "prepared"},
	} {
		t.Run(test.name, func(t *testing.T) {
			rollout := newRolloutDirectoryFixture(t, []domain.ClientID{domain.ClientCursor}, []domain.ClientID{domain.ClientCursor})
			rollout.directory.bundle.Snapshot.Distributions[0].ReleasePolicies[0].Targets[0].Delivery = test.delivery
			if _, _, err := rollout.cli.execute(false, "add", "rollout-demo", "--target", "cursor"); err == nil || !strings.Contains(err.Error(), "delivery") {
				t.Fatalf("delivery %q error = %v", test.delivery, err)
			}
			state, err := rollout.cli.store.Load()
			if err != nil {
				t.Fatal(err)
			}
			if rollout.acquirer.verifiedCalls != 0 || len(state.Installations) != 0 {
				t.Fatalf("invalid delivery acquired or mutated: acquisitions=%d state=%+v", rollout.acquirer.verifiedCalls, state)
			}
		})
	}
}

func TestSharedSurfaceUpdateRejectsDirectoryPolicyThatOmitsCopilot(t *testing.T) {
	t.Parallel()
	rollout := newRolloutDirectoryFixture(t,
		[]domain.ClientID{domain.ClientCopilot, domain.ClientVSCode},
		[]domain.ClientID{domain.ClientVSCode})
	if _, _, err := rollout.cli.execute(false, "add", "rollout-demo", "--target", "copilot,vscode"); err != nil {
		t.Fatal(err)
	}
	rollout.publishV2()
	if _, _, err := rollout.cli.execute(false, "update", "demo", "--target", "vscode"); err == nil || !strings.Contains(err.Error(), "missing copilot") {
		t.Fatalf("surface-omitting policy was accepted: %v", err)
	}
	state, err := rollout.cli.store.Load()
	if err != nil {
		t.Fatal(err)
	}
	binding := onlyCLIClient(state.Installations[0])
	if binding.PackageRevision.ReleaseSequence != 1 || !reflect.DeepEqual(binding.AffectedSurfaces, []string{"copilot", "vscode"}) || len(binding.Receipts) != 1 {
		t.Fatalf("rejected shared policy mutated binding: %+v", binding)
	}
}

func TestSingleVSCodeAddRequiresDirectoryCompatibilityForCopilotSurface(t *testing.T) {
	t.Parallel()
	rollout := newRolloutDirectoryFixture(t,
		[]domain.ClientID{domain.ClientCopilot, domain.ClientVSCode},
		[]domain.ClientID{domain.ClientCopilot, domain.ClientVSCode})
	rollout.directory.bundle.Snapshot.Distributions[0].ReleasePolicies[0].Targets = []domain.DirectoryTarget{{
		Client: domain.ClientVSCode, Scopes: []domain.InstallScope{domain.ScopeUser}, Delivery: "prepared",
	}}
	if _, _, err := rollout.cli.execute(false, "add", "rollout-demo", "--target", "vscode"); err == nil || !strings.Contains(err.Error(), "missing copilot") {
		t.Fatalf("single-surface add accepted incomplete shared policy: %v", err)
	}
	state, err := rollout.cli.store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Installations) != 0 {
		t.Fatalf("incomplete shared add policy mutated state: %+v", state.Installations)
	}
}

func TestInteractiveSharedSurfaceAddRequiresSignedPeerEligibilityBeforeAcquisition(t *testing.T) {
	for _, test := range []struct {
		name, input       string
		selected, missing domain.ClientID
	}{
		{name: "copilot only", input: "1\n", selected: domain.ClientCopilot, missing: domain.ClientVSCode},
		{name: "vscode only", input: "2\n", selected: domain.ClientVSCode, missing: domain.ClientCopilot},
	} {
		t.Run(test.name, func(t *testing.T) {
			rollout := newRolloutDirectoryFixture(t,
				[]domain.ClientID{domain.ClientCopilot, domain.ClientVSCode},
				[]domain.ClientID{domain.ClientCopilot, domain.ClientVSCode})
			delivery, _ := domain.ExpectedDirectoryDelivery(test.selected)
			rollout.directory.bundle.Snapshot.Distributions[0].ReleasePolicies[0].Targets = []domain.DirectoryTarget{{
				Client: test.selected, Scopes: []domain.InstallScope{domain.ScopeUser}, Delivery: delivery,
			}}
			if _, _, err := rollout.cli.executeInput(true, test.input, "add", "rollout-demo"); err == nil || !strings.Contains(err.Error(), "missing "+string(test.missing)) {
				t.Fatalf("interactive %s selection accepted incomplete peer policy: %v", test.selected, err)
			}
			state, err := rollout.cli.store.Load()
			if err != nil {
				t.Fatal(err)
			}
			if rollout.acquirer.verifiedCalls != 0 || len(state.Installations) != 0 {
				t.Fatalf("interactive rejection acquired or mutated: acquisitions=%d state=%+v", rollout.acquirer.verifiedCalls, state)
			}
			if _, err := os.Stat(filepath.Join(rollout.cli.root, "data", "managed")); !os.IsNotExist(err) {
				t.Fatalf("interactive rejection created a managed artifact root: %v", err)
			}
		})
	}
}

func TestDirectoryRepairRejectsRecordedTuplePackageSourceRebindBeforeAcquisition(t *testing.T) {
	rollout := newRolloutDirectoryFixture(t, []domain.ClientID{domain.ClientCursor}, []domain.ClientID{domain.ClientCursor})
	if _, _, err := rollout.cli.execute(false, "add", "rollout-demo", "--target", "cursor"); err != nil {
		t.Fatal(err)
	}
	before, err := rollout.cli.store.Load()
	if err != nil {
		t.Fatal(err)
	}
	binding := onlyCLIClient(before.Installations[0])
	marker := filepath.Join(binding.TargetLocator, "rebind-must-not-touch.txt")
	if err := os.WriteFile(marker, []byte("unchanged"), 0o600); err != nil {
		t.Fatal(err)
	}
	rebound := strings.Repeat("b", 40)
	rollout.directory.bundle.Snapshot.Sequence++
	rollout.directory.bundle.Digest = "sha256:" + strings.Repeat("b", 64)
	rollout.directory.bundle.Snapshot.Distributions[0].Releases[0].PackageSource.Revision = rebound
	rollout.acquirer.revisionRoots[rebound] = rollout.v1.root
	rollout.acquirer.verifiedCalls = 0
	rollout.acquirer.revisionCalls = map[string]int{}
	if _, _, err := rollout.cli.execute(false, "repair", "demo", "--target", "cursor"); err == nil || !strings.Contains(err.Error(), "package-source revision") {
		t.Fatalf("repair accepted rebound package source: %v", err)
	}
	after, err := rollout.cli.store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if rollout.acquirer.verifiedCalls != 0 || !reflect.DeepEqual(after, before) {
		t.Fatalf("rejected repair acquired or mutated state: acquisitions=%d before=%+v after=%+v", rollout.acquirer.verifiedCalls, before, after)
	}
	if body, err := os.ReadFile(marker); err != nil || string(body) != "unchanged" {
		t.Fatalf("rejected repair changed managed artifact: body=%q err=%v", body, err)
	}
}

func TestRetainedDirectoryNewTargetRejectsRecordedTuplePackageSourceRebind(t *testing.T) {
	rollout := newRolloutDirectoryFixture(t,
		[]domain.ClientID{domain.ClientCursor, domain.ClientKiro},
		[]domain.ClientID{domain.ClientCursor, domain.ClientKiro})
	if _, _, err := rollout.cli.execute(false, "add", "rollout-demo", "--target", "cursor"); err != nil {
		t.Fatal(err)
	}
	before, err := rollout.cli.store.Load()
	if err != nil {
		t.Fatal(err)
	}
	rebound := strings.Repeat("b", 40)
	rollout.directory.bundle.Snapshot.Sequence++
	rollout.directory.bundle.Digest = "sha256:" + strings.Repeat("b", 64)
	rollout.directory.bundle.Snapshot.Distributions[0].Releases[0].PackageSource.Revision = rebound
	rollout.acquirer.revisionRoots[rebound] = rollout.v1.root
	rollout.acquirer.verifiedCalls = 0
	if _, _, err := rollout.cli.execute(false, "add", "rollout-demo", "--target", "kiro"); err == nil || !strings.Contains(err.Error(), "package-source revision") {
		t.Fatalf("new-target add accepted rebound package source: %v", err)
	}
	after, err := rollout.cli.store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if rollout.acquirer.verifiedCalls != 0 || !reflect.DeepEqual(after, before) {
		t.Fatalf("rejected new-target add acquired or mutated state: acquisitions=%d before=%+v after=%+v", rollout.acquirer.verifiedCalls, before, after)
	}
	if after.Installations[0].Source.ResolvedRevision != rollout.v1.revision || onlyCLIClient(after.Installations[0]).PackageRevision.ResolvedRevision != rollout.v1.revision {
		t.Fatalf("rejected new-target add rewrote recorded revision: %+v", after.Installations[0])
	}
}

func TestSharedVSCodeRepairRequiresDirectoryCompatibilityForCopilotSurface(t *testing.T) {
	t.Parallel()
	rollout := newRolloutDirectoryFixture(t,
		[]domain.ClientID{domain.ClientCopilot, domain.ClientVSCode},
		[]domain.ClientID{domain.ClientCopilot, domain.ClientVSCode})
	if _, _, err := rollout.cli.execute(false, "add", "rollout-demo", "--target", "vscode"); err != nil {
		t.Fatal(err)
	}
	state, err := rollout.cli.store.Load()
	if err != nil {
		t.Fatal(err)
	}
	binding := onlyCLIClient(state.Installations[0])
	if !reflect.DeepEqual(binding.AffectedSurfaces, []string{"copilot", "vscode"}) {
		t.Fatalf("single VS Code add did not record the complete physical surface set: %+v", binding)
	}
	if err := os.RemoveAll(binding.TargetLocator); err != nil {
		t.Fatal(err)
	}
	rollout.directory.bundle.Snapshot.Distributions[0].ReleasePolicies[0].Targets = []domain.DirectoryTarget{{
		Client: domain.ClientVSCode, Scopes: []domain.InstallScope{domain.ScopeUser}, Delivery: "prepared",
	}}
	if _, _, err := rollout.cli.execute(false, "repair", "demo", "--target", "vscode"); err == nil || !strings.Contains(err.Error(), "missing copilot") {
		t.Fatalf("shared repair accepted incomplete current policy: %v", err)
	}
	state, err = rollout.cli.store.Load()
	if err != nil {
		t.Fatal(err)
	}
	binding = onlyCLIClient(state.Installations[0])
	if len(binding.Receipts) != 1 {
		t.Fatalf("failed shared repair mutated binding: %+v", binding)
	}
}

func bindingRelease(installation domain.Installation, target domain.ClientID) uint64 {
	for _, binding := range installation.Clients {
		if bindingAffectsTarget(binding, target) && binding.PackageRevision != nil {
			return binding.PackageRevision.ReleaseSequence
		}
	}
	return 0
}

func bindingResolvedRevision(installation domain.Installation, target domain.ClientID) string {
	for _, binding := range installation.Clients {
		if bindingAffectsTarget(binding, target) && binding.PackageRevision != nil {
			return binding.PackageRevision.ResolvedRevision
		}
	}
	return ""
}
