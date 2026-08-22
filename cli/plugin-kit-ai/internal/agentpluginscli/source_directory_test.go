package agentpluginscli

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/777genius/plugin-kit-ai/install/integrationctl/agentplugins/adapters/directoryv1"
	"github.com/777genius/plugin-kit-ai/install/integrationctl/agentplugins/adapters/sourceacquisition"
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
	schema := intendedTrustedDirectoryEvidence(domain.DirectoryEvidence{SchemaVersion: 1, ID: "notion-schema", DistributionID: selection.DistributionID, ReleaseSequence: selection.ReleaseSequence, PackageTreeDigest: digest, Level: "schema", Outcome: "passed", Artifact: artifact})
	exact := intendedTrustedDirectoryEvidence(domain.DirectoryEvidence{SchemaVersion: 1, ID: "notion-oauth", DistributionID: selection.DistributionID, ReleaseSequence: selection.ReleaseSequence, PackageTreeDigest: digest, Level: "oauth", Outcome: "passed", Client: domain.ClientCursor, ClientVersion: "0.50.0", InstallerVersion: "1.2.3", OS: "linux", Architecture: "amd64", DependencyIdentity: "node@22", ObservedAt: "2026-08-21T00:00:00Z", Artifact: artifact})

	tests := []struct {
		name           string
		authentication domain.AuthenticationRequirement
		record         domain.DirectoryEvidence
		withoutVersion bool
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
		{name: "missing client version", authentication: domain.AuthenticationRequirementRequired, record: exact, withoutVersion: true, wantVerify: "not_tested", wantLevel: "oauth", wantOutcome: "not_tested"},
		{name: "wrong installer version", authentication: domain.AuthenticationRequirementRequired, record: withDirectoryEvidence(exact, func(e *domain.DirectoryEvidence) { e.InstallerVersion = "1.2.2" }), wantVerify: "not_tested", wantLevel: "oauth", wantOutcome: "not_tested"},
		{name: "wrong dependency", authentication: domain.AuthenticationRequirementRequired, record: withDirectoryEvidence(exact, func(e *domain.DirectoryEvidence) { e.DependencyIdentity = "node@20" }), wantVerify: "not_tested", wantLevel: "oauth", wantOutcome: "not_tested"},
		{name: "failed current materialization", authentication: domain.AuthenticationRequirementNotRequired, record: withDirectoryEvidence(exact, func(e *domain.DirectoryEvidence) {
			e.ID = "notion-materialization"
			e.Level = "materialization"
			e.Outcome = "failed"
			e.Trust = &domain.DirectoryEvidenceTrust{Kind: "github_actions", Workflow: e.Artifact.Repository + "/.github/workflows/directory.yml", SourceRef: "refs/heads/main", SourceDigest: e.Artifact.Revision}
		}), wantVerify: "not_tested", wantLevel: "materialization", wantOutcome: "failed", wantApplicable: 1},
		{name: "unknown authentication", authentication: domain.AuthenticationRequirementUnknown, record: schema, wantVerify: "schema_only", wantLevel: "schema", wantOutcome: "passed", wantApplicable: 1},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			testEnvironment := environment
			if test.withoutVersion {
				testEnvironment.ClientVersions = map[domain.ClientID]string{}
			}
			policy := domain.DirectoryReleasePolicy{Targets: []domain.DirectoryTarget{{Client: domain.ClientCursor, Delivery: "managed", Authentication: test.authentication}}, CurrentEvidence: []string{test.record.ID}}
			bundle := directoryv1.VerifiedBundle{Digest: "sha256:directory", Snapshot: domain.DirectorySnapshot{SourceCommit: strings.Repeat("e", 40), Evidence: []domain.DirectoryEvidence{test.record}}}
			loaded := loadedPackage{}
			if err := applyDirectoryCompatibility(&loaded, bundle, selection, policy, testEnvironment); err != nil {
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
	evidence := intendedTrustedDirectoryEvidence(domain.DirectoryEvidence{ID: "package-schema", DistributionID: selection.DistributionID, ReleaseSequence: selection.ReleaseSequence, PackageTreeDigest: digest, Level: "schema", Outcome: "passed"})
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

func TestUntrustedCurrentEvidenceCannotCreateTestedCompatibility(t *testing.T) {
	digest := "sha256:" + strings.Repeat("a", 64)
	selection := domain.DirectorySelection{DistributionID: "owner/demo", ReleaseSequence: 1, TreeDigest: digest}
	environment := directoryEvidenceEnvironment{InstallerVersion: "1.2.3", OS: "linux", Architecture: "amd64",
		ClientVersions: map[domain.ClientID]string{domain.ClientCursor: "0.50.0"}, DependencyIdentity: map[domain.ClientID]string{}}
	pass := domain.DirectoryEvidence{ID: "self-reported-runtime", DistributionID: selection.DistributionID, ReleaseSequence: 1,
		PackageTreeDigest: digest, Level: "runtime", Outcome: "passed", Client: domain.ClientCursor,
		ClientVersion: "0.50.0", InstallerVersion: "1.2.3", OS: "linux", Architecture: "amd64"}
	failure := pass
	failure.ID, failure.Outcome = "self-reported-failure", "failed"
	policy := domain.DirectoryReleasePolicy{Targets: []domain.DirectoryTarget{{Client: domain.ClientCursor, Delivery: "managed"}},
		CurrentEvidence: []string{pass.ID, failure.ID}}
	loaded := loadedPackage{}
	if err := applyDirectoryCompatibility(&loaded, directoryv1.VerifiedBundle{Snapshot: domain.DirectorySnapshot{Evidence: []domain.DirectoryEvidence{pass, failure}}}, selection, policy, environment); err != nil {
		t.Fatal(err)
	}
	compatibility := loaded.envelope.CatalogEvidence.Compatibility[string(domain.ClientCursor)]
	if compatibility.Verification != "not_tested" || len(compatibility.Evidence) != 0 || compatibility.EvidenceOutcomes["runtime"] != "not_tested" {
		t.Fatalf("untrusted evidence created compatibility claim: %+v", compatibility)
	}
	if len(loaded.envelope.CatalogEvidence.CurrentEvidence) != 2 {
		t.Fatalf("signed informational evidence was not retained with provenance: %+v", loaded.envelope.CatalogEvidence.CurrentEvidence)
	}
}

func TestRetainDirectoryReleaseAfterStableProductResolution(t *testing.T) {
	baseSnapshot := domain.DirectorySnapshot{
		Products: []domain.DirectoryProduct{
			{ID: "product-a", ManifestName: "demo", Aliases: []string{"stable-alias"}, DefaultDistribution: "new/demo", Distributions: []string{"old/demo", "new/demo"}},
			{ID: "product-b", ManifestName: "other", Aliases: []string{"other-alias"}, DefaultDistribution: "other/demo", Distributions: []string{"other/demo"}},
		},
		Distributions: []domain.DirectoryDistribution{
			{ID: "old/demo", ProductID: "product-a"},
			{ID: "new/demo", ProductID: "product-a"},
			{ID: "other/demo", ProductID: "product-b"},
		},
	}
	retained := domain.Installation{
		InstallationID: "install-a", DeclaredName: "demo", OriginMode: domain.OriginModeDirectory,
		Source:    domain.SourceBinding{RequestedSource: "stable-alias"},
		Directory: &domain.DirectoryOrigin{ProductID: "product-a", DistributionID: "old/demo", DesiredReleaseSequence: 7},
	}
	tests := []struct {
		name         string
		selector     string
		mutate       func(*domain.DirectorySnapshot, *domain.StateFileV2)
		wantRecorded bool
		wantProduct  string
		wantDist     string
		wantSequence uint64
		wantErr      string
	}{
		{name: "alias default rotation", selector: "stable-alias", wantRecorded: true, wantProduct: "product-a", wantDist: "old/demo", wantSequence: 7},
		{name: "alias reassignment to different product", selector: "stable-alias", mutate: func(snapshot *domain.DirectorySnapshot, _ *domain.StateFileV2) {
			snapshot.Products[0].Aliases = nil
			snapshot.Products[1].Aliases = []string{"stable-alias"}
		}, wantErr: "now resolves to product"},
		{name: "retired alias retains recorded identity", selector: "stable-alias", mutate: func(snapshot *domain.DirectorySnapshot, _ *domain.StateFileV2) {
			snapshot.Products[0].Aliases = nil
		}, wantRecorded: true, wantProduct: "product-a", wantDist: "old/demo", wantSequence: 7},
		{name: "direct product ID", selector: "product-a", wantRecorded: true, wantProduct: "product-a", wantDist: "old/demo", wantSequence: 7},
		{name: "declared name", selector: "demo", wantRecorded: true, wantProduct: "product-a", wantDist: "old/demo", wantSequence: 7},
		{name: "no retained state", selector: "stable-alias", mutate: func(_ *domain.DirectorySnapshot, state *domain.StateFileV2) {
			state.Installations = nil
		}},
		{name: "corrupt retained state", selector: "stable-alias", mutate: func(_ *domain.DirectorySnapshot, state *domain.StateFileV2) {
			state.Installations[0].Directory.DesiredReleaseSequence = 0
		}, wantErr: "incomplete or corrupt"},
		{name: "ambiguous retained state", selector: "stable-alias", mutate: func(_ *domain.DirectorySnapshot, state *domain.StateFileV2) {
			duplicate := state.Installations[0]
			duplicate.InstallationID = "install-b"
			state.Installations = append(state.Installations, duplicate)
		}, wantErr: "ambiguous"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			snapshot := baseSnapshot
			snapshot.Products = append([]domain.DirectoryProduct(nil), baseSnapshot.Products...)
			for index := range snapshot.Products {
				snapshot.Products[index].Aliases = append([]string(nil), baseSnapshot.Products[index].Aliases...)
			}
			state := domain.StateFileV2{SchemaVersion: domain.StateSchemaVersion, Installations: []domain.Installation{retained}}
			copyOrigin := *retained.Directory
			state.Installations[0].Directory = &copyOrigin
			if test.mutate != nil {
				test.mutate(&snapshot, &state)
			}
			request, err := retainDirectoryRelease(snapshot, state, test.selector, packageResolutionRequest{Operation: domain.DirectoryInstall, RetainRecorded: true})
			if test.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), test.wantErr) {
					t.Fatalf("error = %v, want containing %q", err, test.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if !test.wantRecorded {
				if request.Recorded != nil || request.Operation != domain.DirectoryInstall {
					t.Fatalf("request = %+v, want unrecorded install", request)
				}
				return
			}
			if request.Recorded == nil || request.Operation != domain.DirectoryNewTarget || request.Recorded.ProductID != test.wantProduct ||
				request.Recorded.DistributionID != test.wantDist || request.Recorded.ReleaseSequence != test.wantSequence {
				t.Fatalf("retained request = %+v", request)
			}
		})
	}
}

func TestDirectoryPostAcquisitionDependencyRecheckFailsClosed(t *testing.T) {
	t.Parallel()
	client := fixtureClient(t, domain.ClientCursor)
	client.Version = "0.50.0"
	fixture := newCLIFixture(t, []domain.DetectedClient{client})
	plugin := writeCLIPlugin(t)
	mcp := `{"$schema":"https://agent-plugins.org/schemas/1.0.0/mcp.schema.json","mcpServers":{"demo":{"type":"stdio","command":"npx"}}}`
	if err := os.WriteFile(filepath.Join(plugin, "mcp.json"), []byte(mcp), 0o644); err != nil {
		t.Fatal(err)
	}
	local, err := fixture.app.acquireLocal(context.Background(), plugin)
	if err != nil {
		t.Fatal(err)
	}
	treeDigest, manifestDigest := local.envelope.TreeDigest, local.envelope.ManifestDigest
	if err := local.cleanup(); err != nil {
		t.Fatal(err)
	}
	revision := strings.Repeat("a", 40)
	release := domain.DirectoryRelease{Sequence: 1, PackageVersion: "1.0.0", ManifestName: "demo",
		AgentPluginsSchema: "https://agent-plugins.org/schemas/1.0.0/plugin.schema.json", PackageSource: domain.DirectorySource{Repository: "owner/demo", Revision: revision, Path: "plugin"},
		TreeDigestAlgorithm: domain.TreeDigestAlgorithm, TreeDigest: treeDigest, ManifestDigest: manifestDigest, Components: []string{"mcp"}}
	passed := intendedTrustedDirectoryEvidence(domain.DirectoryEvidence{ID: "passed/materialization", DistributionID: "owner/demo", ReleaseSequence: 1, PackageTreeDigest: treeDigest,
		Level: "materialization", Outcome: "passed", Client: domain.ClientCursor})
	failed := intendedTrustedDirectoryEvidence(domain.DirectoryEvidence{ID: "failed/runtime/npx", DistributionID: "owner/demo", ReleaseSequence: 1, PackageTreeDigest: treeDigest,
		Level: "runtime", Outcome: "failed", Client: domain.ClientCursor, ClientVersion: client.Version, InstallerVersion: fixture.app.Version,
		OS: runtime.GOOS, Architecture: runtime.GOARCH, DependencyIdentity: "npx"})
	policy := domain.DirectoryReleasePolicy{ReleaseSequence: 1, Status: domain.ReleaseActive, MinimumInstallerVersion: "0.1.0",
		Targets: []domain.DirectoryTarget{{Client: domain.ClientCursor, Scopes: []domain.InstallScope{domain.ScopeUser}, Delivery: "managed"}}, CurrentEvidence: []string{passed.ID, failed.ID}}
	snapshot := domain.DirectorySnapshot{SnapshotSchemaVersion: 1, Sequence: 1,
		Products: []domain.DirectoryProduct{{SchemaVersion: 1, ID: "demo", ManifestName: "demo", DefaultDistribution: "owner/demo", Distributions: []string{"owner/demo"}, MinimumCapabilities: domain.DirectoryMinimumCapabilities{Skills: "optional", MCP: "optional"}}},
		Distributions: []domain.DirectoryDistribution{{SchemaVersion: 1, ID: "owner/demo", ProductID: "demo", Kind: domain.DistributionUpstream, Status: domain.DistributionActive,
			Releases: []domain.DirectoryRelease{release}, ReleasePolicies: []domain.DirectoryReleasePolicy{policy}}}, Evidence: []domain.DirectoryEvidence{passed, failed}}
	directory := &fixedDirectoryClient{bundle: directoryv1.VerifiedBundle{Snapshot: snapshot, Digest: "sha256:" + strings.Repeat("b", 64)}}
	acquirer := &localBackedSourceAcquirer{delegate: fixture.app.SourceAcquirer, root: plugin}
	fixture.app.DirectoryClient, fixture.app.SourceAcquirer = directory, acquirer
	_, err = fixture.app.acquireDirectory(context.Background(), "demo", packageResolutionRequest{Targets: []domain.ClientID{domain.ClientCursor},
		Clients: map[domain.ClientID]domain.DetectedClient{domain.ClientCursor: client}})
	if err == nil || !strings.Contains(err.Error(), "failed compatibility recheck") {
		t.Fatalf("exact dependency failure did not fail closed after acquisition: %v", err)
	}
	state, loadErr := fixture.store.Load()
	if loadErr != nil {
		t.Fatal(loadErr)
	}
	if acquirer.verifiedCalls != 1 || len(state.Installations) != 0 {
		t.Fatalf("dependency recheck acquisition/mutation = calls:%d state:%+v", acquirer.verifiedCalls, state)
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
		CanonicalSource: "https://github.com/" + repository + "@" + revision + "//" + path,
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
		CurrentEvidence: []string{"passed/materialization/cursor"},
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
		Evidence: []domain.DirectoryEvidence{intendedTrustedDirectoryEvidence(domain.DirectoryEvidence{ID: policy.CurrentEvidence[0], DistributionID: "owner/demo", ReleaseSequence: release.Sequence,
			PackageTreeDigest: release.TreeDigest, Level: "materialization", Outcome: "passed", Client: domain.ClientCursor})}, Revocations: []domain.DirectoryRevocation{},
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
	if err := os.RemoveAll(onlyCLIClient(installation).TargetLocator); err != nil {
		t.Fatal(err)
	}
	if _, _, err := fixture.execute(false, "repair", "demo-alias", "--target", "cursor"); err != nil {
		t.Fatal(err)
	}
	owner := &directory.bundle.Snapshot.Distributions[0]
	newerOwner := owner.Releases[0]
	newerOwner.Sequence = 4
	newerOwner.PackageSource.Revision = strings.Repeat("f", 40)
	owner.Releases = append(owner.Releases, newerOwner)
	newerOwnerPolicy := owner.ReleasePolicies[0]
	newerOwnerPolicy.ReleaseSequence = 4
	newerOwnerPolicy.CurrentEvidence = []string{"passed/materialization/cursor/4"}
	owner.ReleasePolicies = append(owner.ReleasePolicies, newerOwnerPolicy)
	directory.bundle.Snapshot.Evidence = append(directory.bundle.Snapshot.Evidence, intendedTrustedDirectoryEvidence(domain.DirectoryEvidence{
		ID: newerOwnerPolicy.CurrentEvidence[0], DistributionID: owner.ID, ReleaseSequence: 4, PackageTreeDigest: treeDigest,
		Level: "materialization", Outcome: "passed", Client: domain.ClientCursor,
	}))
	directory.bundle.Snapshot.Sequence = 18
	directory.bundle.Digest = "sha256:" + strings.Repeat("8", 64)
	if _, _, err := fixture.execute(false, "update", "demo-alias", "--target", "cursor"); err != nil {
		t.Fatal(err)
	}
	state, err = fixture.store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if state.Installations[0].Directory.DesiredReleaseSequence != 4 || state.Installations[0].Source.RequestedSource != "demo-alias" {
		t.Fatalf("alias update lost retained identity: %+v", state.Installations[0])
	}
	if _, _, err := fixture.execute(false, "remove", "demo-alias", "--target", "cursor"); err != nil {
		t.Fatal(err)
	}
	directory.bundle.Snapshot.Products[0].DefaultDistribution = "new/demo"
	directory.bundle.Snapshot.Products[0].Aliases = []string{"demo"}
	directory.bundle.Snapshot.Products[0].ReservedAliases = append(directory.bundle.Snapshot.Products[0].ReservedAliases, "demo-alias")
	directory.bundle.Snapshot.Sequence = 19
	directory.bundle.Digest = "sha256:" + strings.Repeat("d", 64)
	if _, _, err := fixture.execute(false, "add", "demo-alias", "--target", "cursor"); err != nil {
		t.Fatal(err)
	}
	state, err = fixture.store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if state.Installations[0].Directory.DistributionID != "owner/demo" || state.Installations[0].Directory.DesiredReleaseSequence != 4 {
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

func TestInstalledHistoricalSelectorSelectionFailsClosedOnCollision(t *testing.T) {
	installation := func(id string) domain.Installation {
		return domain.Installation{InstallationID: id, DeclaredName: "demo", OriginMode: domain.OriginModeDirectory,
			Source: domain.SourceBinding{RequestedSource: "demo-alias"}, Directory: &domain.DirectoryOrigin{ProductID: "demo", DistributionID: "owner/demo", DesiredReleaseSequence: 1}}
	}
	state := domain.StateFileV2{Installations: []domain.Installation{installation("install-a")}}
	selected, err := selectInstallation(state, "demo-alias")
	if err != nil || selected.InstallationID != "install-a" {
		t.Fatalf("historical selector did not select retained installation: %+v %v", selected, err)
	}
	state.Installations = append(state.Installations, installation("install-b"))
	if _, err := selectInstallation(state, "demo-alias"); err == nil || !strings.Contains(err.Error(), "ambiguous") {
		t.Fatalf("historical selector collision did not fail closed: %v", err)
	}
	selected, err = selectInstallation(state, "install-a")
	if err != nil || selected.InstallationID != "install-a" {
		t.Fatalf("exact installation ID did not disambiguate: %+v %v", selected, err)
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
			}, CurrentEvidence: []string{fmt.Sprintf("passed/materialization/cursor/%d", sequence), fmt.Sprintf("passed/materialization/kiro/%d", sequence)}}
	}
	distribution := domain.DirectoryDistribution{SchemaVersion: 1, ID: "owner/mixed", ProductID: "mixed-demo", Kind: domain.DistributionUpstream,
		Status: domain.DistributionActive, Packager: "owner", Releases: []domain.DirectoryRelease{release(1, v1)}, ReleasePolicies: []domain.DirectoryReleasePolicy{policy(1)}}
	snapshot := domain.DirectorySnapshot{SnapshotSchemaVersion: 1, Sequence: 1, SourceCommit: strings.Repeat("a", 40),
		Products: []domain.DirectoryProduct{{SchemaVersion: 1, ID: "mixed-demo", DisplayName: "Mixed Demo", Description: "Mixed Demo", ManifestName: "demo",
			Aliases: []string{"mixed-demo"}, ReservedAliases: []string{"mixed-demo"}, Categories: []string{},
			MinimumCapabilities: domain.DirectoryMinimumCapabilities{Skills: "optional", MCP: "optional"}, DefaultDistribution: "owner/mixed", Distributions: []string{"owner/mixed"}}},
		Distributions: []domain.DirectoryDistribution{distribution}, Evidence: []domain.DirectoryEvidence{
			intendedTrustedDirectoryEvidence(domain.DirectoryEvidence{ID: "passed/materialization/cursor/1", DistributionID: "owner/mixed", ReleaseSequence: 1, PackageTreeDigest: v1.tree, Level: "materialization", Outcome: "passed", Client: domain.ClientCursor}),
			intendedTrustedDirectoryEvidence(domain.DirectoryEvidence{ID: "passed/materialization/kiro/1", DistributionID: "owner/mixed", ReleaseSequence: 1, PackageTreeDigest: v1.tree, Level: "materialization", Outcome: "passed", Client: domain.ClientKiro}),
		}, Revocations: []domain.DirectoryRevocation{}}
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
	directory.bundle.Snapshot.Evidence = append(directory.bundle.Snapshot.Evidence,
		intendedTrustedDirectoryEvidence(domain.DirectoryEvidence{ID: "passed/materialization/cursor/2", DistributionID: "owner/mixed", ReleaseSequence: 2, PackageTreeDigest: v2.tree, Level: "materialization", Outcome: "passed", Client: domain.ClientCursor}),
		intendedTrustedDirectoryEvidence(domain.DirectoryEvidence{ID: "passed/materialization/kiro/2", DistributionID: "owner/mixed", ReleaseSequence: 2, PackageTreeDigest: v2.tree, Level: "materialization", Outcome: "passed", Client: domain.ClientKiro}))
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

func TestProductionGitHubAcquirerOutputResolvesAsDirectExactSource(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is unavailable")
	}
	repositoryRoot := t.TempDir()
	pluginRoot := filepath.Join(repositoryRoot, "plugins", "demo")
	if err := os.MkdirAll(pluginRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := `{"$schema":"https://agent-plugins.org/schemas/1.0.0/plugin.schema.json","name":"demo","version":"1.0.0"}`
	if err := os.WriteFile(filepath.Join(pluginRoot, "plugin.json"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit := func(args ...string) string {
		t.Helper()
		output, err := exec.Command("git", args...).CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v: %s", args, err, output)
		}
		return strings.TrimSpace(string(output))
	}
	runGit("init", "--quiet", "--initial-branch=main", repositoryRoot)
	runGit("-C", repositoryRoot, "config", "user.email", "fixture@example.invalid")
	runGit("-C", repositoryRoot, "config", "user.name", "Fixture")
	runGit("-C", repositoryRoot, "add", ".")
	runGit("-C", repositoryRoot, "commit", "--quiet", "-m", "fixture")
	revision := runGit("-C", repositoryRoot, "rev-parse", "HEAD")

	fixture := newCLIFixture(t, nil)
	fixture.app.SourceAcquirer = sourceacquisition.Acquirer{
		TempRoot: fixture.root,
		URLForRepo: func(repository string) string {
			if repository != "owner/repo" {
				t.Fatalf("unexpected repository %q", repository)
			}
			return repositoryRoot
		},
	}
	requested := "owner/repo@" + revision + "//plugins/demo"
	loaded, err := fixture.app.acquireGitHub(context.Background(), requested, "owner/repo", revision, "plugins/demo", "")
	if err != nil {
		t.Fatal(err)
	}
	defer loaded.cleanup()
	wantCanonical := "https://github.com/owner/repo@" + revision + "//plugins/demo"
	if loaded.envelope.Source.CanonicalSource != wantCanonical || loaded.envelope.Source.RequestedSource != requested {
		t.Fatalf("production acquisition identity = %+v", loaded.envelope.Source)
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
