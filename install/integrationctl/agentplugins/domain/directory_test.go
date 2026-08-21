package domain

import (
	"errors"
	"testing"
)

func testRelease(sequence uint64, version string) DirectoryRelease {
	return DirectoryRelease{Sequence: sequence, PackageVersion: version, ManifestName: "tool", AgentPluginsSchema: "https://agent-plugins.org/schemas/1.0.0/plugin.schema.json", PackageSource: DirectorySource{Repository: "owner/repo", Revision: "0123456789012345678901234567890123456789", Path: "plugin"}, TreeDigestAlgorithm: TreeDigestAlgorithm, TreeDigest: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", ManifestDigest: "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", Components: []string{"mcp", "skills"}, PublishedAt: "2026-08-20T00:00:00Z"}
}

func testPolicy(sequence uint64, clients ...ClientID) DirectoryReleasePolicy {
	targets := make([]DirectoryTarget, len(clients))
	for i, c := range clients {
		delivery, _ := ExpectedDirectoryDelivery(c)
		targets[i] = DirectoryTarget{Client: c, Scopes: []InstallScope{ScopeUser}, Delivery: delivery, Authentication: AuthenticationRequirementUnknown}
	}
	return DirectoryReleasePolicy{ReleaseSequence: sequence, Status: ReleaseActive, MinimumInstallerVersion: "1.0.0", Targets: targets, CurrentEvidence: []string{}}
}

func testDistribution(id string, kind DistributionKind, releases []DirectoryRelease, policies []DirectoryReleasePolicy) DirectoryDistribution {
	return DirectoryDistribution{SchemaVersion: 1, ID: id, ProductID: "tool", Kind: kind, Status: DistributionActive, Packager: "owner", Releases: releases, ReleasePolicies: policies}
}

func testDirectory() DirectorySnapshot {
	upstream := testDistribution("owner/tool", DistributionUpstream, []DirectoryRelease{testRelease(2, "0.1.0"), testRelease(3, "not-semver")}, []DirectoryReleasePolicy{testPolicy(2, ClientCodex), testPolicy(3, ClientCodex)})
	bridge := testDistribution("community/tool-bridge", DistributionCommunityBridge, []DirectoryRelease{testRelease(8, "99.0.0")}, []DirectoryReleasePolicy{testPolicy(8, ClientCodex, ClientCursor)})
	community := testDistribution("other/tool", DistributionCommunity, []DirectoryRelease{testRelease(1, "1.0.0")}, []DirectoryReleasePolicy{testPolicy(1, ClientCodex, ClientCursor)})
	product := DirectoryProduct{SchemaVersion: 1, ID: "tool", DisplayName: "Tool", Description: "Tool", ManifestName: "tool", Aliases: []string{"tool", "old-tool"}, ReservedAliases: []string{"tool"}, Categories: []string{"tools"}, MinimumCapabilities: DirectoryMinimumCapabilities{MCP: "required", Skills: "optional"}, DefaultDistribution: "owner/tool", Distributions: []string{"other/tool", "community/tool-bridge", "owner/tool"}}
	return DirectorySnapshot{SnapshotSchemaVersion: 1, Sequence: 42, Products: []DirectoryProduct{product}, Distributions: []DirectoryDistribution{community, bridge, upstream}, Evidence: []DirectoryEvidence{}, Revocations: []DirectoryRevocation{}}
}

func request(selector string, targets ...ClientID) DirectoryResolveRequest {
	return DirectoryResolveRequest{Selector: selector, Targets: targets, Scope: ScopeUser, InstallerVersion: "1.2.3", ClientVersions: map[ClientID]string{ClientCodex: "test-client"}, OS: "linux", Architecture: "amd64", SchemaVersion: "1.0.0", Operation: DirectoryInstall}
}

func TestResolveDirectoryDeclaredDefaultFallbackQualifiedAndSequence(t *testing.T) {
	s := testDirectory()
	got, err := ResolveDirectory(s, request("old-tool", ClientCodex))
	if err != nil || got.DistributionID != "owner/tool" || got.ReleaseSequence != 3 || got.Fallback {
		t.Fatalf("default: %+v %v", got, err)
	}
	got, err = ResolveDirectory(s, request("tool", ClientCodex, ClientCursor))
	if err != nil || got.DistributionID != "community/tool-bridge" || !got.Fallback || len(got.Diagnostics) == 0 {
		t.Fatalf("fallback: %+v %v", got, err)
	}
	got, err = ResolveDirectory(s, request("other/tool", ClientCursor))
	if err != nil || got.DistributionID != "other/tool" {
		t.Fatalf("qualified: %+v %v", got, err)
	}
}

func TestResolveDirectoryRejectsUnknownAndMismatchedDelivery(t *testing.T) {
	for _, test := range []struct {
		name, delivery string
	}{
		{name: "unknown", delivery: "future_delivery"},
		{name: "mismatched", delivery: "managed"},
	} {
		t.Run(test.name, func(t *testing.T) {
			s := testDirectory()
			s.Distributions[2].ReleasePolicies[0].Status = ReleaseSuperseded
			s.Distributions[2].ReleasePolicies[1].Targets[0].Delivery = test.delivery
			if _, err := ResolveDirectory(s, request("owner/tool", ClientCodex)); !errors.Is(err, ErrDirectoryIneligible) {
				t.Fatalf("delivery %q was accepted: %v", test.delivery, err)
			}
		})
	}
}

func TestResolveDirectoryDeclaredDefaultPrecedesKindAndFallbackUsesKindOrder(t *testing.T) {
	s := testDirectory()
	s.Products[0].DefaultDistribution = "other/tool"
	got, err := ResolveDirectory(s, request("tool", ClientCodex))
	if err != nil || got.DistributionID != "other/tool" || got.Fallback {
		t.Fatalf("eligible declared community default was implicitly promoted away: %+v %v", got, err)
	}
	s.Distributions[0].ReleasePolicies[0].Targets = []DirectoryTarget{{Client: ClientCursor, Scopes: []InstallScope{ScopeUser}, Delivery: "managed", Authentication: AuthenticationRequirementUnknown}}
	got, err = ResolveDirectory(s, request("tool", ClientCodex))
	if err != nil || got.DistributionID != "owner/tool" || !got.Fallback {
		t.Fatalf("fallback did not prefer eligible upstream: %+v %v", got, err)
	}
}

func TestResolveDirectoryNoMixingAndAmbiguity(t *testing.T) {
	s := testDirectory()
	s.Distributions[0].ReleasePolicies[0].Targets = []DirectoryTarget{{Client: ClientCursor, Scopes: []InstallScope{ScopeUser}, Delivery: "managed", Authentication: AuthenticationRequirementUnknown}}
	s.Distributions[1].ReleasePolicies[0].Targets = []DirectoryTarget{{Client: ClientCursor, Scopes: []InstallScope{ScopeUser}, Delivery: "managed", Authentication: AuthenticationRequirementUnknown}}
	if _, err := ResolveDirectory(s, request("tool", ClientCodex, ClientCursor)); !errors.Is(err, ErrDirectoryIneligible) {
		t.Fatalf("mixing: %v", err)
	}
	p := s.Products[0]
	p.ID = "tool-two"
	p.ManifestName = "tool-two"
	p.DefaultDistribution = "other/tool-two"
	p.Distributions = []string{"other/tool-two"}
	d := testDistribution("other/tool-two", DistributionCommunity, []DirectoryRelease{testRelease(1, "1")}, []DirectoryReleasePolicy{testPolicy(1, ClientCodex)})
	d.ProductID = "tool-two"
	d.Releases[0].ManifestName = "tool-two"
	s.Products = append(s.Products, p)
	s.Distributions = append(s.Distributions, d)
	if _, err := ResolveDirectory(s, request("old-tool", ClientCodex)); !errors.Is(err, ErrDirectoryAmbiguous) {
		t.Fatalf("ambiguity: %v", err)
	}
}

func TestResolveDirectoryOperationMatrixAndTopLevelRevocation(t *testing.T) {
	s := testDirectory()
	d := &s.Distributions[2]
	recorded := &RecordedDirectoryRelease{ProductID: "tool", DistributionID: "owner/tool", ReleaseSequence: 3}
	base := request("owner/tool", ClientCodex)
	base.Recorded = recorded
	s.Revocations = []DirectoryRevocation{{DistributionID: "owner/tool", ReleaseSequence: 3}}
	for _, op := range []DirectoryOperation{DirectoryInstall, DirectoryNewTarget, DirectoryRepair, DirectoryRematerialize} {
		r := base
		r.Operation = op
		if _, err := ResolveDirectory(s, r); err == nil {
			t.Fatalf("%s accepted revoked", op)
		}
	}
	r := base
	r.Operation = DirectoryRemove
	if _, err := ResolveDirectory(s, r); err != nil {
		t.Fatalf("remove: %v", err)
	}
	d.Releases = append(d.Releases, testRelease(4, "0.0.1"))
	d.ReleasePolicies = append(d.ReleasePolicies, testPolicy(4, ClientCodex))
	r = base
	r.Operation = DirectoryUpdate
	if got, err := ResolveDirectory(s, r); err != nil || got.ReleaseSequence != 4 {
		t.Fatalf("safe update: %+v %v", got, err)
	}
	s.Revocations = nil
	d.ReleasePolicies[1].Status = ReleaseSuperseded
	for _, op := range []DirectoryOperation{DirectoryInstall, DirectoryNewTarget, DirectoryRepair, DirectoryRematerialize, DirectoryReproduce} {
		r = base
		r.Operation = op
		if got, err := ResolveDirectory(s, r); err != nil || got.ReleaseSequence != recorded.ReleaseSequence {
			t.Fatalf("exact recorded superseded %s: %+v %v", op, got, err)
		}
	}
	d.ReleasePolicies[1].Status = ReleaseActive
	d.Status = DistributionSuspended
	for _, op := range []DirectoryOperation{DirectoryInstall, DirectoryNewTarget} {
		r.Operation = op
		if got, err := ResolveDirectory(s, r); err != nil || got.ReleaseSequence != recorded.ReleaseSequence {
			t.Fatalf("suspended exact recorded %s: %+v %v", op, got, err)
		}
	}
	r.Operation = DirectoryUpdate
	if _, err := ResolveDirectory(s, r); err == nil {
		t.Fatal("suspended update accepted")
	}
	r.Recorded = nil
	for _, op := range []DirectoryOperation{DirectoryInstall, DirectoryNewTarget} {
		r.Operation = op
		if _, err := ResolveDirectory(s, r); err == nil {
			t.Fatalf("suspended %s created unrelated exposure", op)
		}
	}
	r.Recorded = recorded
	r.Operation = DirectoryRepair
	if _, err := ResolveDirectory(s, r); err != nil {
		t.Fatalf("suspended repair: %v", err)
	}
	r.Operation = DirectoryRematerialize
	if _, err := ResolveDirectory(s, r); err != nil {
		t.Fatalf("suspended rematerialization: %v", err)
	}
}

func TestResolveDirectoryRecordedReAddRetainsExactRelease(t *testing.T) {
	s := testDirectory()
	recorded := &RecordedDirectoryRelease{ProductID: "tool", DistributionID: "owner/tool", ReleaseSequence: 2}
	r := request("tool", ClientCodex)
	r.Recorded = recorded
	got, err := ResolveDirectory(s, r)
	if err != nil || got.DistributionID != recorded.DistributionID || got.ReleaseSequence != recorded.ReleaseSequence {
		t.Fatalf("recorded re-add moved release: %+v %v", got, err)
	}
}

func TestResolveDirectoryGates(t *testing.T) {
	s := testDirectory()
	p := &s.Distributions[2].ReleasePolicies[1]
	s.Distributions[2].ReleasePolicies[0].Status = ReleaseSuperseded
	p.MinimumInstallerVersion = "2.0.0"
	if _, err := ResolveDirectory(s, request("owner/tool", ClientCodex)); err == nil {
		t.Fatal("installer gate")
	}
	p.MinimumInstallerVersion = "1.0.0"
	s.Evidence = []DirectoryEvidence{{ID: "failed/runtime", DistributionID: "owner/tool", ReleaseSequence: 3, PackageTreeDigest: testRelease(3, "").TreeDigest, Level: "runtime", Outcome: "failed", Client: ClientCodex, ClientVersion: "test-client", InstallerVersion: "1.2.3", OS: "linux", Architecture: "amd64"}}
	p.CurrentEvidence = []string{"failed/runtime"}
	if _, err := ResolveDirectory(s, request("owner/tool", ClientCodex)); err == nil {
		t.Fatal("evidence gate")
	}
	p.CurrentEvidence = nil
	s.Evidence = nil
	r := request("owner/tool", ClientCodex)
	r.RequiredComponents = []string{"extensions"}
	if _, err := ResolveDirectory(s, r); err == nil {
		t.Fatal("component gate")
	}
	r = request("owner/tool", ClientCodex)
	r.Scope = ScopeProject
	if _, err := ResolveDirectory(s, r); err == nil {
		t.Fatal("scope gate")
	}

	s = testDirectory()
	p = &s.Distributions[2].ReleasePolicies[1]
	s.Distributions[2].ReleasePolicies[0].Status = ReleaseSuperseded
	s.Evidence = []DirectoryEvidence{{ID: "failed/schema", DistributionID: "owner/tool", ReleaseSequence: 3, PackageTreeDigest: testRelease(3, "").TreeDigest, Level: "schema", Outcome: "failed"}}
	p.CurrentEvidence = []string{"failed/schema"}
	if _, err := ResolveDirectory(s, request("owner/tool", ClientCodex)); err == nil {
		t.Fatal("schema evidence gate")
	}
}
