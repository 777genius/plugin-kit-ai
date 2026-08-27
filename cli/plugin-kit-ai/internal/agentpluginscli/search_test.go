package agentpluginscli

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/777genius/plugin-kit-ai/install/integrationctl/agentplugins/adapters/discoveryv1"
)

type fixedDiscoveryClient struct {
	bundle discoveryv1.VerifiedBundle
	err    error
	calls  int
}

func (client *fixedDiscoveryClient) Load(context.Context, uint64) (discoveryv1.VerifiedBundle, error) {
	client.calls++
	return client.bundle, client.err
}

func discoverySearchBundle() discoveryv1.VerifiedBundle {
	version := "1.0.0"
	record := discoveryv1.Record{
		Slug: "discovery:upstream/demo//plugin", Name: "demo", Description: "unreviewed demo", Owner: "upstream",
		Repository: "upstream/demo", PackagePath: "plugin", Revision: strings.Repeat("a", 40), Version: &version,
		SchemaVersion: "1.0.0", Components: discoveryv1.Components{MCP: 1}, MCPTransports: []string{"streamable-http"},
		CompatibleClients: []string{"codex", "cursor"}, Authentication: "unknown", Status: "conformant_unreviewed",
		TreeDigest: "sha256:" + strings.Repeat("1", 64), ManifestDigest: "sha256:" + strings.Repeat("2", 64),
		RepositoryUpdatedAt: "2026-08-27T00:00:00Z", Availability: "available",
	}
	return discoveryv1.VerifiedBundle{
		Snapshot: discoveryv1.Snapshot{Sequence: 7}, Search: discoveryv1.Search{Sequence: 7, Records: []discoveryv1.Record{record}},
		Digest: "sha256:" + strings.Repeat("3", 64),
	}
}

func TestSearchReviewedDirectoryIsDeterministicReadOnlyAndFiltered(t *testing.T) {
	t.Parallel()
	fixture := newCLIFixture(t, nil)
	fixture.app.DirectoryClient = &fixedDirectoryClient{bundle: readModelBundle()}
	fixture.app.DiscoveryClient = &fixedDiscoveryClient{bundle: discoveryv1.VerifiedBundle{Snapshot: discoveryv1.Snapshot{Sequence: 1}, Search: discoveryv1.Search{Records: []discoveryv1.Record{}}, Digest: "sha256:" + strings.Repeat("3", 64)}}

	stdout, _, err := fixture.execute(false, "search", "demo", "--client", "cursor", "--owner", "owner", "--format", "json")
	if err != nil {
		t.Fatal(err)
	}
	assertVersionedJSON(t, stdout, "search")
	var envelope struct {
		Data searchResponse `json:"data"`
	}
	if err := json.Unmarshal([]byte(stdout), &envelope); err != nil {
		t.Fatal(err)
	}
	if len(envelope.Data.Results) == 0 || envelope.Data.Results[0].TrustState != "reviewed" || envelope.Data.Results[0].InstallSelector != "demo" {
		t.Fatalf("search results = %+v", envelope.Data.Results)
	}
	state, err := fixture.store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Installations) != 0 {
		t.Fatalf("search mutated state: %+v", state)
	}

	repeat, _, err := fixture.execute(false, "search", "demo", "--client", "cursor", "--owner", "owner", "--format", "json")
	if err != nil || stdout != repeat {
		t.Fatalf("search is not deterministic: err=%v\nfirst=%s\nsecond=%s", err, stdout, repeat)
	}
	empty, _, err := fixture.execute(false, "search", "demo", "--trust", "unreviewed", "--format", "json")
	if err != nil || !strings.Contains(empty, `"results":[]`) {
		t.Fatalf("unreviewed filter = %q err=%v", empty, err)
	}
}

func TestSearchMergesReviewedAndSignedUnreviewedWithTrustRanking(t *testing.T) {
	t.Parallel()
	fixture := newCLIFixture(t, nil)
	fixture.app.DirectoryClient = &fixedDirectoryClient{bundle: readModelBundle()}
	fixture.app.DiscoveryClient = &fixedDiscoveryClient{bundle: discoverySearchBundle()}
	stdout, _, err := fixture.execute(false, "search", "demo", "--client", "cursor", "--format", "json")
	if err != nil {
		t.Fatal(err)
	}
	var envelope struct {
		Data searchResponse `json:"data"`
	}
	if err := json.Unmarshal([]byte(stdout), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Data.DiscoveryStatus != "available" || envelope.Data.DiscoverySnapshotSequence != 7 || len(envelope.Data.Results) < 2 {
		t.Fatalf("search response = %+v", envelope.Data)
	}
	unreviewedIndex := -1
	for index, result := range envelope.Data.Results {
		if result.TrustState == "unreviewed" {
			unreviewedIndex = index
			break
		}
	}
	if unreviewedIndex < 1 || envelope.Data.Results[0].TrustState != "reviewed" ||
		envelope.Data.Results[unreviewedIndex].InstallSelector != "discovery:upstream/demo//plugin" {
		t.Fatalf("ranked results = %+v", envelope.Data.Results)
	}
	unreviewed, _, err := fixture.execute(false, "search", "demo", "--trust", "unreviewed", "--owner", "upstream", "--component", "mcp", "--auth", "unknown", "--format", "json")
	if err != nil || !strings.Contains(unreviewed, `"trust_state":"unreviewed"`) || strings.Contains(unreviewed, `"trust_state":"reviewed"`) {
		t.Fatalf("unreviewed search = %q err=%v", unreviewed, err)
	}
}

func TestHumanSearchShowsExactProvenanceTrustAndRunnableInstallCommand(t *testing.T) {
	t.Parallel()
	fixture := newCLIFixture(t, nil)
	fixture.app.DirectoryClient = &fixedDirectoryClient{bundle: readModelBundle()}
	fixture.app.DiscoveryClient = &fixedDiscoveryClient{bundle: discoverySearchBundle()}
	stdout, _, err := fixture.execute(false, "search", "upstream/demo", "--trust", "unreviewed", "--client", "cursor")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"[unreviewed, available]",
		"source: upstream/demo@" + strings.Repeat("a", 40) + "//plugin",
		"schema: Agent Plugins 1.0; runtime: not reviewed",
		"npx universal-agent-plugins install discovery:upstream/demo//plugin --target cursor",
	} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("human search omitted %q:\n%s", want, stdout)
		}
	}
}

func TestHumanSearchUsesShellSafeTargetPlaceholderWithoutClient(t *testing.T) {
	t.Parallel()
	fixture := newCLIFixture(t, nil)
	fixture.app.DirectoryClient = &fixedDirectoryClient{bundle: readModelBundle()}
	fixture.app.DiscoveryClient = &fixedDiscoveryClient{bundle: discoverySearchBundle()}
	stdout, _, err := fixture.execute(false, "search", "upstream/demo", "--trust", "unreviewed")
	if err != nil {
		t.Fatal(err)
	}
	want := "npx universal-agent-plugins install discovery:upstream/demo//plugin --target YOUR_AGENT"
	if !strings.Contains(stdout, want) || strings.Contains(stdout, "--target <agent>") {
		t.Fatalf("human search target placeholder is not shell-safe:\n%s", stdout)
	}
}

func TestHumanSearchDoesNotOfferInstallForUnavailablePackage(t *testing.T) {
	t.Parallel()
	fixture := newCLIFixture(t, nil)
	fixture.app.DirectoryClient = &fixedDirectoryClient{bundle: readModelBundle()}
	bundle := discoverySearchBundle()
	bundle.Search.Records[0].Availability = "unavailable"
	fixture.app.DiscoveryClient = &fixedDiscoveryClient{bundle: bundle}
	stdout, _, err := fixture.execute(false, "search", "upstream/demo", "--trust", "unreviewed", "--client", "cursor")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout, "install: unavailable at indexed source") || strings.Contains(stdout, "npx universal-agent-plugins install") {
		t.Fatalf("human search offered installation for an unavailable package:\n%s", stdout)
	}
}

func TestSearchKeepsReviewedResultsWhenDiscoveryIsUnavailable(t *testing.T) {
	t.Parallel()
	fixture := newCLIFixture(t, nil)
	fixture.app.DirectoryClient = &fixedDirectoryClient{bundle: readModelBundle()}
	fixture.app.DiscoveryClient = &fixedDiscoveryClient{err: errors.New("offline")}
	stdout, stderr, err := fixture.execute(false, "search", "demo", "--format", "json")
	if err != nil || !strings.Contains(stdout, `"discovery_status":"unavailable"`) || !strings.Contains(stdout, `"trust_state":"reviewed"`) ||
		!strings.Contains(stderr, "discovery_unavailable") {
		t.Fatalf("graceful search = stdout:%q stderr:%q err:%v", stdout, stderr, err)
	}
}

func TestSearchUsesStarsOnlyAfterTrustAndTextRelevance(t *testing.T) {
	t.Parallel()
	fixture := newCLIFixture(t, nil)
	fixture.app.DirectoryClient = &fixedDirectoryClient{bundle: readModelBundle()}
	bundle := discoverySearchBundle()
	popular := bundle.Search.Records[0]
	popular.Slug = "discovery:zeta/popular//plugin"
	popular.Repository = "zeta/popular"
	popular.Owner = "zeta"
	popular.Stars = 100
	quiet := popular
	quiet.Slug = "discovery:alpha/quiet//plugin"
	quiet.Repository = "alpha/quiet"
	quiet.Owner = "alpha"
	quiet.Stars = 1
	bundle.Search.Records = []discoveryv1.Record{quiet, popular}
	fixture.app.DiscoveryClient = &fixedDiscoveryClient{bundle: bundle}

	stdout, _, err := fixture.execute(false, "search", "demo", "--trust", "unreviewed", "--format", "json")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Index(stdout, popular.Slug) > strings.Index(stdout, quiet.Slug) || !strings.Contains(stdout, `"stars":100`) {
		t.Fatalf("stars were not the final deterministic tie-breaker:\n%s", stdout)
	}
}

func TestSearchRuntimeReviewIsScopedToSelectedClient(t *testing.T) {
	t.Parallel()
	bundle := readModelBundle()
	distribution := bundle.Snapshot.Distributions[0]
	policy := distribution.ReleasePolicies[1]
	if !searchHasRuntimeEvidence(bundle.Snapshot, distribution.ID, 2, policy, "") ||
		!searchHasRuntimeEvidence(bundle.Snapshot, distribution.ID, 2, policy, "cursor") ||
		searchHasRuntimeEvidence(bundle.Snapshot, distribution.ID, 2, policy, "kiro") {
		t.Fatal("runtime review leaked across client identities")
	}
}
