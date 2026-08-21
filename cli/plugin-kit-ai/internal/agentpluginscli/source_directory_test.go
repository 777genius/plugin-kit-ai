package agentpluginscli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/777genius/plugin-kit-ai/install/integrationctl/agentplugins/adapters/directoryv1"
	"github.com/777genius/plugin-kit-ai/install/integrationctl/agentplugins/domain"
)

type fixedDirectoryClient struct {
	bundle directoryv1.VerifiedBundle
	calls  int
	floor  uint64
	err    error
}

func (client *fixedDirectoryClient) Load(_ context.Context, floor uint64) (directoryv1.VerifiedBundle, error) {
	client.calls++
	client.floor = floor
	return client.bundle, client.err
}

type localBackedSourceAcquirer struct {
	delegate       SourceAcquirer
	root           string
	revisionRoots  map[string]string
	revisionCalls  map[string]int
	localCalls     int
	directGitCalls int
	verifiedCalls  int
}

func TestDirectoryCompatibilityUsesStablePublicPackageModes(t *testing.T) {
	policy := domain.DirectoryReleasePolicy{Targets: []domain.DirectoryTarget{
		{Client: domain.ClientCodex, Delivery: "manual_activation", Authentication: domain.AuthenticationRequirementUnknown},
		{Client: domain.ClientCursor, Delivery: "managed", Authentication: domain.AuthenticationRequirementUnknown},
		{Client: domain.ClientVSCode, Delivery: "prepared", Authentication: domain.AuthenticationRequirementUnknown},
	}}
	loaded := loadedPackage{}
	if err := applyDirectoryCompatibility(&loaded, directoryv1.VerifiedBundle{Digest: "sha256:directory"}, domain.DirectorySelection{}, policy, directoryEvidenceEnvironment{}); err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"codex": "projected", "cursor": "native", "vscode": "prepared"}
	for client, packageMode := range want {
		if got := loaded.envelope.CatalogEvidence.Compatibility[client].Package; got != packageMode {
			t.Fatalf("%s package mode = %q, want %q", client, got, packageMode)
		}
	}
}

func TestDirectoryCompatibilityPreservesEvidenceAndExactApplicability(t *testing.T) {
	t.Parallel()
	digest := "sha256:" + strings.Repeat("a", 64)
	selection := domain.DirectorySelection{DistributionID: "owner/notion", ReleaseSequence: 7, TreeDigest: digest}
	environment := directoryEvidenceEnvironment{
		InstallerVersion: "1.2.3", OS: "linux", Architecture: "amd64",
		ClientVersions:     map[domain.ClientID]string{domain.ClientCursor: "0.50.0"},
		DependencyIdentity: map[domain.ClientID]string{domain.ClientCursor: "node@22"},
	}
	artifact := domain.DirectoryEvidenceArtifact{Repository: "owner/evidence", Revision: strings.Repeat("b", 40), Path: "evidence/result.json", Digest: "sha256:" + strings.Repeat("c", 64)}
	schema := domain.DirectoryEvidence{SchemaVersion: 1, ID: "notion-schema", DistributionID: selection.DistributionID, ReleaseSequence: selection.ReleaseSequence, PackageTreeDigest: digest, Level: "schema", Outcome: "passed", Artifact: artifact}
	exact := domain.DirectoryEvidence{SchemaVersion: 1, ID: "notion-oauth", DistributionID: selection.DistributionID, ReleaseSequence: selection.ReleaseSequence, PackageTreeDigest: digest, Level: "oauth", Outcome: "passed", Client: domain.ClientCursor, ClientVersion: "0.50.0", InstallerVersion: "1.2.3", OS: "linux", Architecture: "amd64", DependencyIdentity: "node@22", ObservedAt: "2026-08-21T00:00:00Z", Artifact: artifact}

	tests := []struct {
		name           string
		authentication domain.AuthenticationRequirement
		record         domain.DirectoryEvidence
		wantVerify     string
		wantLevel      string
		wantOutcome    string
		wantApplicable int
	}{
		{name: "Notion schema-only with authentication required", authentication: domain.AuthenticationRequirementRequired, record: schema, wantVerify: "schema_only", wantLevel: "schema", wantOutcome: "passed", wantApplicable: 1},
		{name: "OAuth exact target pass", authentication: domain.AuthenticationRequirementRequired, record: exact, wantVerify: "tested", wantLevel: "oauth", wantOutcome: "passed", wantApplicable: 1},
		{name: "stale digest", authentication: domain.AuthenticationRequirementRequired, record: withDirectoryEvidence(exact, func(e *domain.DirectoryEvidence) { e.PackageTreeDigest = "sha256:" + strings.Repeat("d", 64) }), wantVerify: "not_tested", wantLevel: "oauth", wantOutcome: "not_tested"},
		{name: "wrong client", authentication: domain.AuthenticationRequirementRequired, record: withDirectoryEvidence(exact, func(e *domain.DirectoryEvidence) { e.Client = domain.ClientCodex }), wantVerify: "not_tested", wantLevel: "oauth", wantOutcome: "not_tested"},
		{name: "wrong OS", authentication: domain.AuthenticationRequirementRequired, record: withDirectoryEvidence(exact, func(e *domain.DirectoryEvidence) { e.OS = "darwin" }), wantVerify: "not_tested", wantLevel: "oauth", wantOutcome: "not_tested"},
		{name: "wrong client version", authentication: domain.AuthenticationRequirementRequired, record: withDirectoryEvidence(exact, func(e *domain.DirectoryEvidence) { e.ClientVersion = "0.49.0" }), wantVerify: "not_tested", wantLevel: "oauth", wantOutcome: "not_tested"},
		{name: "wrong installer version", authentication: domain.AuthenticationRequirementRequired, record: withDirectoryEvidence(exact, func(e *domain.DirectoryEvidence) { e.InstallerVersion = "1.2.2" }), wantVerify: "not_tested", wantLevel: "oauth", wantOutcome: "not_tested"},
		{name: "wrong dependency", authentication: domain.AuthenticationRequirementRequired, record: withDirectoryEvidence(exact, func(e *domain.DirectoryEvidence) { e.DependencyIdentity = "node@20" }), wantVerify: "not_tested", wantLevel: "oauth", wantOutcome: "not_tested"},
		{name: "failed current materialization", authentication: domain.AuthenticationRequirementNotRequired, record: withDirectoryEvidence(exact, func(e *domain.DirectoryEvidence) {
			e.ID = "notion-materialization"
			e.Level = "materialization"
			e.Outcome = "failed"
		}), wantVerify: "not_tested", wantLevel: "materialization", wantOutcome: "failed", wantApplicable: 1},
		{name: "unknown authentication", authentication: domain.AuthenticationRequirementUnknown, record: schema, wantVerify: "schema_only", wantLevel: "schema", wantOutcome: "passed", wantApplicable: 1},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			policy := domain.DirectoryReleasePolicy{Targets: []domain.DirectoryTarget{{Client: domain.ClientCursor, Delivery: "managed", Authentication: test.authentication}}, CurrentEvidence: []string{test.record.ID}}
			bundle := directoryv1.VerifiedBundle{Digest: "sha256:directory", Snapshot: domain.DirectorySnapshot{SourceCommit: strings.Repeat("e", 40), Evidence: []domain.DirectoryEvidence{test.record}}}
			loaded := loadedPackage{}
			if err := applyDirectoryCompatibility(&loaded, bundle, selection, policy, environment); err != nil {
				t.Fatal(err)
			}
			compatibility := loaded.envelope.CatalogEvidence.Compatibility[string(domain.ClientCursor)]
			if compatibility.Authentication != test.authentication || compatibility.Verification != test.wantVerify || len(compatibility.Evidence) != test.wantApplicable {
				t.Fatalf("compatibility = %+v", compatibility)
			}
			if got := compatibility.EvidenceOutcomes[test.wantLevel]; got != test.wantOutcome {
				t.Fatalf("%s outcome = %q, want %q", test.wantLevel, got, test.wantOutcome)
			}
			if compatibility.EvidenceOutcomes["runtime"] == "" || compatibility.EvidenceOutcomes["oauth"] == "" {
				t.Fatalf("missing level outcomes: %+v", compatibility.EvidenceOutcomes)
			}
			catalogEvidence := loaded.envelope.CatalogEvidence
			if len(catalogEvidence.CurrentEvidence) != 1 || catalogEvidence.CurrentEvidence[0].ID != test.record.ID {
				t.Fatalf("current signed evidence not preserved: %+v", catalogEvidence)
			}
		})
	}
}

func TestDirectoryPackageSchemaEvidenceAppliesToEveryTarget(t *testing.T) {
	digest := "sha256:" + strings.Repeat("a", 64)
	selection := domain.DirectorySelection{DistributionID: "owner/package", ReleaseSequence: 1, TreeDigest: digest}
	evidence := domain.DirectoryEvidence{ID: "package-schema", DistributionID: selection.DistributionID, ReleaseSequence: selection.ReleaseSequence, PackageTreeDigest: digest, Level: "schema", Outcome: "passed"}
	policy := domain.DirectoryReleasePolicy{Targets: []domain.DirectoryTarget{
		{Client: domain.ClientCursor, Delivery: "managed", Authentication: domain.AuthenticationRequirementNotRequired},
		{Client: domain.ClientCodex, Delivery: "manual_activation", Authentication: domain.AuthenticationRequirementRequired},
	}, CurrentEvidence: []string{evidence.ID}}
	loaded := loadedPackage{}
	if err := applyDirectoryCompatibility(&loaded, directoryv1.VerifiedBundle{Snapshot: domain.DirectorySnapshot{Evidence: []domain.DirectoryEvidence{evidence}}}, selection, policy, directoryEvidenceEnvironment{}); err != nil {
		t.Fatal(err)
	}
	for _, client := range []domain.ClientID{domain.ClientCursor, domain.ClientCodex} {
		compatibility := loaded.envelope.CatalogEvidence.Compatibility[string(client)]
		if compatibility.Verification != "schema_only" || compatibility.EvidenceOutcomes["runtime"] != "not_tested" || compatibility.EvidenceOutcomes["oauth"] != "not_tested" {
			t.Fatalf("%s compatibility = %+v", client, compatibility)
		}
	}
}

func withDirectoryEvidence(source domain.DirectoryEvidence, mutate func(*domain.DirectoryEvidence)) domain.DirectoryEvidence {
	mutate(&source)
	return source
}

func (acquirer *localBackedSourceAcquirer) AcquireLocal(ctx context.Context, path string) (domain.PackageSnapshot, error) {
	acquirer.localCalls++
	return acquirer.delegate.AcquireLocal(ctx, path)
}

func (acquirer *localBackedSourceAcquirer) gitSnapshot(ctx context.Context, repository, revision, path string) (domain.PackageSnapshot, error) {
	root := acquirer.root
	if revisionRoot := acquirer.revisionRoots[revision]; revisionRoot != "" {
		root = revisionRoot
	}
	if acquirer.revisionCalls != nil {
		acquirer.revisionCalls[revision]++
	}
	snapshot, err := acquirer.delegate.AcquireLocal(ctx, root)
	if err != nil {
		return domain.PackageSnapshot{}, err
	}
	snapshot.Source = domain.SourceIdentity{
		RequestedSource: repository + "@" + revision + "//" + path,
		CanonicalSource: repository + "@" + revision + "//" + path,
		Repository:      repository, PackageSubpath: path, ResolvedRevision: revision,
	}
	return snapshot, nil
}

func (acquirer *localBackedSourceAcquirer) AcquireGitHub(ctx context.Context, repository, revision, path string) (domain.PackageSnapshot, error) {
	acquirer.directGitCalls++
	return acquirer.gitSnapshot(ctx, repository, revision, path)
}

func (acquirer *localBackedSourceAcquirer) AcquireGitHubVerified(ctx context.Context, repository, revision, path, digest string) (domain.PackageSnapshot, error) {
	acquirer.verifiedCalls++
	snapshot, err := acquirer.gitSnapshot(ctx, repository, revision, path)
	if err != nil {
		return domain.PackageSnapshot{}, err
	}
	if snapshot.TreeDigest != digest {
		return domain.PackageSnapshot{}, fmt.Errorf("verified tree digest mismatch")
	}
	return snapshot, nil
}

func TestSignedDirectorySelectionAcquiresOnceAndPersistsFullOrigin(t *testing.T) {
	t.Parallel()
	fixture := newCLIFixture(t, []domain.DetectedClient{fixtureClient(t, domain.ClientCursor)})
	plugin := writeCLIPlugin(t)
	loaded, err := fixture.app.acquireLocal(context.Background(), plugin)
	if err != nil {
		t.Fatal(err)
	}
	treeDigest, manifestDigest := loaded.envelope.TreeDigest, loaded.envelope.ManifestDigest
	if err := loaded.cleanup(); err != nil {
		t.Fatal(err)
	}
	revision := strings.Repeat("a", 40)
	release := domain.DirectoryRelease{
		Sequence: 3, PackageVersion: "1.0.0", ManifestName: "demo",
		AgentPluginsSchema:  "https://agent-plugins.org/schemas/1.0.0/plugin.schema.json",
		PackageSource:       domain.DirectorySource{Repository: "owner/demo", Revision: revision, Path: "plugin"},
		TreeDigestAlgorithm: domain.TreeDigestAlgorithm, TreeDigest: treeDigest, ManifestDigest: manifestDigest,
		Components: []string{}, PublishedAt: "2026-08-21T00:00:00Z",
	}
	policy := domain.DirectoryReleasePolicy{
		ReleaseSequence: 3, Status: domain.ReleaseActive, MinimumInstallerVersion: "0.1.0",
		Targets:         []domain.DirectoryTarget{{Client: domain.ClientCursor, Scopes: []domain.InstallScope{domain.ScopeUser}, Delivery: "managed", Authentication: domain.AuthenticationRequirementUnknown}},
		CurrentEvidence: []string{},
	}
	snapshot := domain.DirectorySnapshot{
		SnapshotSchemaVersion: 1, Sequence: 17, SourceCommit: strings.Repeat("b", 40),
		Products: []domain.DirectoryProduct{{
			SchemaVersion: 1, ID: "demo", DisplayName: "Demo", Description: "Demo", ManifestName: "demo",
			Aliases: []string{"demo", "demo-alias"}, ReservedAliases: []string{"demo"}, Categories: []string{},
			MinimumCapabilities: domain.DirectoryMinimumCapabilities{Skills: "optional", MCP: "optional"},
			DefaultDistribution: "owner/demo", Distributions: []string{"owner/demo", "new/demo"},
		}},
		Distributions: []domain.DirectoryDistribution{{
			SchemaVersion: 1, ID: "owner/demo", ProductID: "demo", Kind: domain.DistributionUpstream,
			Status: domain.DistributionActive, Packager: "owner", Releases: []domain.DirectoryRelease{release}, ReleasePolicies: []domain.DirectoryReleasePolicy{policy},
		}, {
			SchemaVersion: 1, ID: "new/demo", ProductID: "demo", Kind: domain.DistributionCommunity,
			Status: domain.DistributionActive, Packager: "new", Releases: []domain.DirectoryRelease{{
				Sequence: 1, PackageVersion: release.PackageVersion, ManifestName: release.ManifestName, AgentPluginsSchema: release.AgentPluginsSchema,
				PackageSource:       domain.DirectorySource{Repository: "new/demo", Revision: strings.Repeat("d", 40), Path: "plugin"},
				TreeDigestAlgorithm: release.TreeDigestAlgorithm, TreeDigest: release.TreeDigest, ManifestDigest: release.ManifestDigest,
				Components: []string{}, PublishedAt: release.PublishedAt,
			}}, ReleasePolicies: []domain.DirectoryReleasePolicy{{
				ReleaseSequence: 1, Status: domain.ReleaseActive, MinimumInstallerVersion: "0.1.0",
				Targets: policy.Targets, CurrentEvidence: []string{},
			}},
		}},
		Evidence: []domain.DirectoryEvidence{}, Revocations: []domain.DirectoryRevocation{},
	}
	directory := &fixedDirectoryClient{bundle: directoryv1.VerifiedBundle{Snapshot: snapshot, Digest: "sha256:" + strings.Repeat("c", 64)}}
	acquirer := &localBackedSourceAcquirer{delegate: fixture.app.SourceAcquirer, root: plugin}
	fixture.app.DirectoryClient = directory
	fixture.app.SourceAcquirer = acquirer
	stdout, _, err := fixture.execute(false, "add", "demo-alias", "--target", "cursor", "--format", "json")
	if err != nil {
		t.Fatal(err)
	}
	assertVersionedJSON(t, stdout, "add")
	state, err := fixture.store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if directory.calls != 1 || acquirer.verifiedCalls != 1 || acquirer.localCalls != 0 || acquirer.directGitCalls != 0 || len(state.Installations) != 1 {
		t.Fatalf("Directory boundary calls/state = directory:%d verified:%d local:%d direct:%d state:%+v", directory.calls, acquirer.verifiedCalls, acquirer.localCalls, acquirer.directGitCalls, state)
	}
	installation := state.Installations[0]
	if installation.OriginMode != domain.OriginModeDirectory || installation.Directory == nil ||
		installation.Directory.ProductID != "demo" || installation.Directory.DistributionID != "owner/demo" ||
		installation.Directory.DistributionKind != domain.DistributionUpstream || installation.Directory.DesiredReleaseSequence != 3 ||
		installation.Directory.SnapshotSchema != 1 || installation.Directory.SnapshotSequence != 17 || installation.Directory.SnapshotDigest != directory.bundle.Digest ||
		installation.Source.Repository != "owner/demo" || installation.Source.ResolvedRevision != revision ||
		installation.Source.TreeDigest != treeDigest || installation.Package.ManifestDigest != manifestDigest {
		t.Fatalf("persisted Directory origin = %+v", installation)
	}
	if _, _, err := fixture.execute(false, "remove", "demo", "--target", "cursor"); err != nil {
		t.Fatal(err)
	}
	directory.bundle.Snapshot.Products[0].DefaultDistribution = "new/demo"
	directory.bundle.Snapshot.Sequence = 18
	directory.bundle.Digest = "sha256:" + strings.Repeat("d", 64)
	if _, _, err := fixture.execute(false, "add", "demo", "--target", "cursor"); err != nil {
		t.Fatal(err)
	}
	state, err = fixture.store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if state.Installations[0].Directory.DistributionID != "owner/demo" || state.Installations[0].Directory.DesiredReleaseSequence != 3 {
		t.Fatalf("re-add moved to changed Directory default: %+v", state.Installations[0].Directory)
	}
	if _, _, err := fixture.execute(false, "switch", "demo", "--to", "new/demo", "--format", "json"); err != nil {
		t.Fatal(err)
	}
	state, err = fixture.store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if state.Installations[0].Directory.DistributionID != "new/demo" || state.Installations[0].Directory.DesiredReleaseSequence != 1 {
		t.Fatalf("explicit Directory switch did not change distribution: %+v", state.Installations[0].Directory)
	}
	if err := os.RemoveAll(onlyCLIClient(state.Installations[0]).TargetLocator); err != nil {
		t.Fatal(err)
	}
	if _, _, err := fixture.execute(false, "repair", "demo", "--target", "cursor"); err != nil {
		t.Fatal(err)
	}
	distribution := &directory.bundle.Snapshot.Distributions[1]
	newer := distribution.Releases[0]
	newer.Sequence = 2
	newer.PackageSource.Revision = strings.Repeat("e", 40)
	distribution.Releases = append(distribution.Releases, newer)
	newerPolicy := distribution.ReleasePolicies[0]
	newerPolicy.ReleaseSequence = 2
	distribution.ReleasePolicies = append(distribution.ReleasePolicies, newerPolicy)
	directory.bundle.Snapshot.Sequence = 19
	directory.bundle.Digest = "sha256:" + strings.Repeat("e", 64)
	if _, _, err := fixture.execute(false, "update", "demo", "--target", "cursor"); err != nil {
		t.Fatal(err)
	}
	state, err = fixture.store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if state.Installations[0].Directory.DistributionID != "new/demo" || state.Installations[0].Directory.DesiredReleaseSequence != 2 ||
		state.Installations[0].Directory.SnapshotSequence != 19 || state.Installations[0].Directory.SnapshotDigest != directory.bundle.Digest || state.Installations[0].Package.Version != "1.0.0" {
		t.Fatalf("sticky sequence update changed distribution or required SemVer change: %+v", state.Installations[0])
	}
}

func TestDirectoryMultiTargetRepairReacquiresEachRecordedRevisionOnce(t *testing.T) {
	t.Parallel()
	fixture := newCLIFixture(t, []domain.DetectedClient{fixtureClient(t, domain.ClientCursor), fixtureClient(t, domain.ClientKiro)})
	v1Root := writeCLIPlugin(t)
	v2Root := writeCLIPlugin(t)
	v2Manifest := `{"$schema":"https://agent-plugins.org/schemas/1.0.0/plugin.schema.json","name":"demo","version":"2.0.0","description":"Demo plugin v2"}`
	if err := os.WriteFile(filepath.Join(v2Root, "plugin.json"), []byte(v2Manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	type exactRevision struct {
		root, revision, tree, manifest, version string
	}
	loadRevision := func(root, revision, version string) exactRevision {
		loaded, err := fixture.app.acquireLocal(context.Background(), root)
		if err != nil {
			t.Fatal(err)
		}
		result := exactRevision{root: root, revision: revision, tree: loaded.envelope.TreeDigest, manifest: loaded.envelope.ManifestDigest, version: version}
		if err := loaded.cleanup(); err != nil {
			t.Fatal(err)
		}
		return result
	}
	v1 := loadRevision(v1Root, strings.Repeat("1", 40), "1.0.0")
	v2 := loadRevision(v2Root, strings.Repeat("2", 40), "2.0.0")
	release := func(sequence uint64, revision exactRevision) domain.DirectoryRelease {
		return domain.DirectoryRelease{Sequence: sequence, PackageVersion: revision.version, ManifestName: "demo",
			AgentPluginsSchema:  "https://agent-plugins.org/schemas/1.0.0/plugin.schema.json",
			PackageSource:       domain.DirectorySource{Repository: "owner/mixed", Revision: revision.revision, Path: "plugin"},
			TreeDigestAlgorithm: domain.TreeDigestAlgorithm, TreeDigest: revision.tree, ManifestDigest: revision.manifest,
			Components: []string{}, PublishedAt: "2026-08-21T00:00:00Z"}
	}
	policy := func(sequence uint64) domain.DirectoryReleasePolicy {
		return domain.DirectoryReleasePolicy{ReleaseSequence: sequence, Status: domain.ReleaseActive, MinimumInstallerVersion: "0.1.0",
			Targets: []domain.DirectoryTarget{
				{Client: domain.ClientCursor, Scopes: []domain.InstallScope{domain.ScopeUser}, Delivery: "managed"},
				{Client: domain.ClientKiro, Scopes: []domain.InstallScope{domain.ScopeUser}, Delivery: "managed"},
			}, CurrentEvidence: []string{}}
	}
	distribution := domain.DirectoryDistribution{SchemaVersion: 1, ID: "owner/mixed", ProductID: "mixed-demo", Kind: domain.DistributionUpstream,
		Status: domain.DistributionActive, Packager: "owner", Releases: []domain.DirectoryRelease{release(1, v1)}, ReleasePolicies: []domain.DirectoryReleasePolicy{policy(1)}}
	snapshot := domain.DirectorySnapshot{SnapshotSchemaVersion: 1, Sequence: 1, SourceCommit: strings.Repeat("a", 40),
		Products: []domain.DirectoryProduct{{SchemaVersion: 1, ID: "mixed-demo", DisplayName: "Mixed Demo", Description: "Mixed Demo", ManifestName: "demo",
			Aliases: []string{"mixed-demo"}, ReservedAliases: []string{"mixed-demo"}, Categories: []string{},
			MinimumCapabilities: domain.DirectoryMinimumCapabilities{Skills: "optional", MCP: "optional"}, DefaultDistribution: "owner/mixed", Distributions: []string{"owner/mixed"}}},
		Distributions: []domain.DirectoryDistribution{distribution}, Evidence: []domain.DirectoryEvidence{}, Revocations: []domain.DirectoryRevocation{}}
	directory := &fixedDirectoryClient{bundle: directoryv1.VerifiedBundle{Snapshot: snapshot, Digest: "sha256:" + strings.Repeat("a", 64)}}
	acquirer := &localBackedSourceAcquirer{delegate: fixture.app.SourceAcquirer,
		revisionRoots: map[string]string{v1.revision: v1.root, v2.revision: v2.root}, revisionCalls: map[string]int{}}
	fixture.app.DirectoryClient = directory
	fixture.app.SourceAcquirer = acquirer
	if _, _, err := fixture.execute(false, "add", "mixed-demo", "--target", "cursor,kiro"); err != nil {
		t.Fatal(err)
	}
	directory.bundle.Snapshot.Distributions[0].Releases = append(directory.bundle.Snapshot.Distributions[0].Releases, release(2, v2))
	directory.bundle.Snapshot.Distributions[0].ReleasePolicies = append(directory.bundle.Snapshot.Distributions[0].ReleasePolicies, policy(2))
	directory.bundle.Snapshot.Sequence = 2
	directory.bundle.Digest = "sha256:" + strings.Repeat("b", 64)
	if _, _, err := fixture.execute(false, "update", "demo", "--target", "cursor"); err != nil {
		t.Fatal(err)
	}
	state, err := fixture.store.Load()
	if err != nil {
		t.Fatal(err)
	}
	for _, binding := range state.Installations[0].Clients {
		if err := os.RemoveAll(binding.TargetLocator); err != nil {
			t.Fatal(err)
		}
	}
	acquirer.revisionCalls = map[string]int{}
	acquirer.verifiedCalls = 0
	if _, _, err := fixture.execute(false, "repair", "demo", "--target", "cursor,kiro"); err != nil {
		t.Fatal(err)
	}
	if acquirer.verifiedCalls != 2 || acquirer.revisionCalls[v1.revision] != 1 || acquirer.revisionCalls[v2.revision] != 1 {
		t.Fatalf("repair acquisitions = total:%d by revision:%v", acquirer.verifiedCalls, acquirer.revisionCalls)
	}
	state, err = fixture.store.Load()
	if err != nil {
		t.Fatal(err)
	}
	versions := map[string]string{}
	for _, binding := range state.Installations[0].Clients {
		versions[binding.ClientID] = binding.PackageRevision.Version
		if _, err := os.Stat(binding.TargetLocator); err != nil {
			t.Fatalf("repair did not restore %s: %v", binding.ClientID, err)
		}
	}
	if versions[string(domain.ClientCursor)] != "2.0.0" || versions[string(domain.ClientKiro)] != "1.0.0" {
		t.Fatalf("repair changed recorded revisions: %v", versions)
	}
	if state.Installations[0].Package.Version != "2.0.0" || state.Installations[0].Directory.DesiredReleaseSequence != 2 {
		t.Fatalf("repair regressed installation-wide desired revision: %+v", state.Installations[0])
	}
}

func TestDirectLocalAndFullSHASourcesBypassDirectory(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name, selector string
		directGit      bool
	}{
		{name: "local", selector: "local"},
		{name: "full-sha", selector: "owner/repo@" + strings.Repeat("d", 40) + "//plugin", directGit: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newCLIFixture(t, []domain.DetectedClient{fixtureClient(t, domain.ClientCursor)})
			plugin := writeCLIPlugin(t)
			directory := &fixedDirectoryClient{err: fmt.Errorf("Directory must not be called")}
			acquirer := &localBackedSourceAcquirer{delegate: fixture.app.SourceAcquirer, root: plugin}
			fixture.app.DirectoryClient = directory
			fixture.app.SourceAcquirer = acquirer
			selector := test.selector
			if !test.directGit {
				selector = plugin
			}
			if _, _, err := fixture.execute(false, "add", selector, "--target", "cursor"); err != nil {
				t.Fatal(err)
			}
			if directory.calls != 0 || (test.directGit && acquirer.directGitCalls != 1) || (!test.directGit && acquirer.localCalls != 1) {
				t.Fatalf("direct source consulted Directory or wrong acquirer: directory=%d local=%d git=%d", directory.calls, acquirer.localCalls, acquirer.directGitCalls)
			}
			state, err := fixture.store.Load()
			if err != nil {
				t.Fatal(err)
			}
			if len(state.Installations) != 1 || state.Installations[0].OriginMode != domain.OriginModeDirect || state.Installations[0].Directory != nil {
				t.Fatalf("direct source received Directory provenance: %+v", state)
			}
		})
	}
}

func TestExistingRelativeDirectoryDoesNotOverrideShortNameForAddOrSwitch(t *testing.T) {
	fixture := newCLIFixture(t, []domain.DetectedClient{fixtureClient(t, domain.ClientCursor)})
	plugin := writeCLIPlugin(t)
	t.Chdir(filepath.Dir(plugin))
	shortName := filepath.Base(plugin)
	counter := &countingSourceAcquirer{delegate: fixture.app.SourceAcquirer}
	fixture.app.SourceAcquirer = counter

	if _, _, err := fixture.execute(false, "add", shortName, "--target", "cursor"); err == nil || !strings.Contains(err.Error(), "signed Directory dependencies are unavailable") {
		t.Fatalf("existing relative directory changed add selector meaning: %v", err)
	}
	if counter.calls != 0 {
		t.Fatalf("bare short name acquired local directory %d time(s)", counter.calls)
	}
	if _, _, err := fixture.execute(false, "add", "./"+shortName, "--target", "cursor"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := fixture.execute(false, "switch", "demo", "--to", shortName); err == nil || !strings.Contains(err.Error(), "short name") {
		t.Fatalf("existing relative directory changed switch selector meaning: %v", err)
	}
}

func TestExplicitLocalPathRecognizesOnlyPortableExplicitAndAbsoluteWindowsForms(t *testing.T) {
	for _, value := range []string{"./plugin", "../plugin", `.\plugin`, `..\plugin`, `/plugin`, `C:\plugin`, `d:/plugin`, `\\server\share\plugin`} {
		if !explicitLocalPath(value) {
			t.Errorf("explicitLocalPath(%q) = false", value)
		}
	}
	for _, value := range []string{"plugin", "existing-plugin", `C:plugin`, `owner\\plugin`, `\\server`} {
		if explicitLocalPath(value) {
			t.Errorf("explicitLocalPath(%q) = true", value)
		}
	}
}
