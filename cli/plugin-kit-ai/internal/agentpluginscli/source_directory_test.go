package agentpluginscli

import (
	"context"
	"fmt"
	"os"
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
	localCalls     int
	directGitCalls int
	verifiedCalls  int
}

func (acquirer *localBackedSourceAcquirer) AcquireLocal(ctx context.Context, path string) (domain.PackageSnapshot, error) {
	acquirer.localCalls++
	return acquirer.delegate.AcquireLocal(ctx, path)
}

func (acquirer *localBackedSourceAcquirer) gitSnapshot(ctx context.Context, repository, revision, path string) (domain.PackageSnapshot, error) {
	snapshot, err := acquirer.delegate.AcquireLocal(ctx, acquirer.root)
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
		Targets:         []domain.DirectoryTarget{{Client: domain.ClientCursor, Scopes: []domain.InstallScope{domain.ScopeUser}, Delivery: "managed"}},
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
