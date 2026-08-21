package main

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/777genius/plugin-kit-ai/install/integrationctl/agentplugins/adapters/directoryv1"
	"github.com/777genius/plugin-kit-ai/install/integrationctl/agentplugins/domain"
)

func TestDirectoryConformanceConfigurationIsCompleteExplicitAndVerified(t *testing.T) {
	for _, name := range conformanceDirectoryVariables {
		if name == "AGENTPLUGINS_DIRECTORY_ORIGIN" {
			continue
		}
		t.Run("partial-"+name, func(t *testing.T) {
			clearConformanceEnvironment(t)
			t.Setenv(name, "1")
			if _, err := newDirectoryClient(t.TempDir()); err == nil || !strings.Contains(err.Error(), "partial test-only") {
				t.Fatalf("partial %s configuration error = %v", name, err)
			}
		})
	}

	t.Run("complete verified fixture", func(t *testing.T) {
		clearConformanceEnvironment(t)
		root := t.TempDir()
		environment := writeConformanceFixture(t, root)
		for name, value := range environment {
			t.Setenv(name, value)
		}
		client, err := newDirectoryClient(filepath.Join(root, "production-cache-must-not-be-used"))
		if err != nil {
			t.Fatal(err)
		}
		if client.Origin != environment["AGENTPLUGINS_DIRECTORY_ORIGIN"] || client.Cache.Path != environment["AGENTPLUGINS_DIRECTORY_CACHE"] || len(client.Trust.Keys) != 1 || client.Trust.Keys[0].ID != "launch-conformance-test" {
			t.Fatalf("test-only Directory tuple was not consumed coherently: %+v", client)
		}
		client.Now = func() time.Time { return time.Date(2026, 8, 21, 0, 0, 0, 0, time.UTC) }
		bundle, err := client.Load(context.Background(), 0)
		if err != nil || bundle.Source != directoryv1.BundleSourceRemote || bundle.Snapshot.Sequence != 41 {
			t.Fatalf("verified conformance fixture load = sequence %d source %q err %v", bundle.Snapshot.Sequence, bundle.Source, err)
		}
		if _, err := os.Stat(environment["AGENTPLUGINS_DIRECTORY_CACHE"]); err != nil {
			t.Fatalf("verified fixture was not reconciled to configured cache: %v", err)
		}
	})

	t.Run("tampered tuple", func(t *testing.T) {
		clearConformanceEnvironment(t)
		root := t.TempDir()
		environment := writeConformanceFixture(t, root)
		for name, value := range environment {
			t.Setenv(name, value)
		}
		if err := os.WriteFile(environment["AGENTPLUGINS_DIRECTORY_SNAPSHOT"], []byte("{}"), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := newDirectoryClient(t.TempDir()); err == nil || !strings.Contains(err.Error(), "verify Directory bootstrap inputs") {
			t.Fatalf("tampered conformance tuple error = %v", err)
		}
	})

	for name, change := range map[string]func(map[string]string){
		"non-exact opt-in": func(environment map[string]string) { environment["AGENTPLUGINS_DIRECTORY_CONFORMANCE_ONLY"] = "true" },
		"unsafe origin": func(environment map[string]string) {
			environment["AGENTPLUGINS_DIRECTORY_ORIGIN"] = "https://user@example.invalid/registry/"
		},
		"relative cache": func(environment map[string]string) { environment["AGENTPLUGINS_DIRECTORY_CACHE"] = "relative-cache" },
	} {
		t.Run(name, func(t *testing.T) {
			clearConformanceEnvironment(t)
			environment := writeConformanceFixture(t, t.TempDir())
			change(environment)
			for variable, value := range environment {
				t.Setenv(variable, value)
			}
			if _, err := newDirectoryClient(t.TempDir()); err == nil {
				t.Fatal("malformed test-only Directory tuple was accepted")
			}
		})
	}
}

func TestOrdinaryDirectoryOriginPreservesProductionTrustAndBootstrap(t *testing.T) {
	clearConformanceEnvironment(t)
	t.Setenv("AGENTPLUGINS_DIRECTORY_ORIGIN", "https://mirror.example/registry/schemas/1/")
	client, err := newDirectoryClient(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if client.Origin != "https://mirror.example/registry/schemas/1/" || len(client.Trust.Keys) != 1 || client.Trust.Keys[0].ID != defaultDirectoryKeyID || len(client.Embedded.Snapshot) != 0 || !client.RequireEmbeddedBootstrap {
		t.Fatalf("ordinary origin changed production trust/bootstrap: %+v", client)
	}
}

func TestMalformedConformanceTupleFailsBeforeNetworkOrMutation(t *testing.T) {
	clearConformanceEnvironment(t)
	t.Setenv("AGENTPLUGINS_DIRECTORY_CONFORMANCE_ONLY", "1")
	t.Setenv("HOME", t.TempDir())
	dataRoot := filepath.Join(t.TempDir(), "must-not-exist")
	t.Setenv("AGENTPLUGINS_HOME", dataRoot)
	originalArgs := os.Args
	os.Args = []string{"agentplugins", "add", "tool", "--target", "cursor"}
	t.Cleanup(func() { os.Args = originalArgs })
	if err := run(); err == nil || !strings.Contains(err.Error(), "partial test-only") {
		t.Fatalf("malformed tuple error = %v", err)
	}
	if _, err := os.Lstat(dataRoot); !os.IsNotExist(err) {
		t.Fatalf("malformed tuple mutated agentplugins home: %v", err)
	}
}

func clearConformanceEnvironment(t *testing.T) {
	t.Helper()
	for _, name := range conformanceDirectoryVariables {
		value, existed := os.LookupEnv(name)
		if err := os.Unsetenv(name); err != nil {
			t.Fatal(err)
		}
		restoreName, restoreValue, restoreExisted := name, value, existed
		t.Cleanup(func() {
			if restoreExisted {
				_ = os.Setenv(restoreName, restoreValue)
			} else {
				_ = os.Unsetenv(restoreName)
			}
		})
	}
}

func writeConformanceFixture(t *testing.T, root string) map[string]string {
	t.Helper()
	seed := make([]byte, ed25519.SeedSize)
	for i := range seed {
		seed[i] = 17
	}
	privateKey := ed25519.NewKeyFromSeed(seed)
	publicKey := privateKey.Public().(ed25519.PublicKey)
	release := domain.DirectoryRelease{Sequence: 1, PackageVersion: "1.0.0", ManifestName: "tool", AgentPluginsSchema: "https://agent-plugins.org/schemas/1.0.0/plugin.schema.json", PackageSource: domain.DirectorySource{Repository: "owner/repo", Revision: "0123456789012345678901234567890123456789", Path: "plugin"}, TreeDigestAlgorithm: domain.TreeDigestAlgorithm, TreeDigest: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", ManifestDigest: "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", Components: []string{"mcp"}, PublishedAt: "2026-08-20T10:00:00Z"}
	policy := domain.DirectoryReleasePolicy{ReleaseSequence: 1, Status: domain.ReleaseActive, MinimumInstallerVersion: "1.0.0", Targets: []domain.DirectoryTarget{{Client: domain.ClientCursor, Scopes: []domain.InstallScope{domain.ScopeUser}, Delivery: "managed"}}, CurrentEvidence: []string{}}
	product := domain.DirectoryProduct{SchemaVersion: 1, ID: "tool", DisplayName: "Tool", Description: "Tool description", ManifestName: "tool", Aliases: []string{"tool"}, ReservedAliases: []string{"tool"}, Categories: []string{"tools"}, MinimumCapabilities: domain.DirectoryMinimumCapabilities{Skills: "optional", MCP: "required"}, DefaultDistribution: "owner/tool", Distributions: []string{"owner/tool"}}
	distribution := domain.DirectoryDistribution{SchemaVersion: 1, ID: "owner/tool", ProductID: "tool", Kind: domain.DistributionUpstream, Status: domain.DistributionActive, Packager: "owner", Releases: []domain.DirectoryRelease{release}, ReleasePolicies: []domain.DirectoryReleasePolicy{policy}}
	snapshot := domain.DirectorySnapshot{SnapshotSchemaVersion: 1, Sequence: 41, PublicationID: "launch-conformance-41", SourceCommit: "abcdefabcdefabcdefabcdefabcdefabcdefabcd", GeneratedAt: "2026-08-20T11:00:00Z", ExpiresAt: "2026-09-19T11:00:00Z", Products: []domain.DirectoryProduct{product}, Distributions: []domain.DirectoryDistribution{distribution}, Evidence: []domain.DirectoryEvidence{}, Revocations: []domain.DirectoryRevocation{}}
	snapshotBytes, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	snapshotBytes = append(snapshotBytes, '\n')
	digest := sha256.Sum256(snapshotBytes)
	signature := ed25519.Sign(privateKey, append([]byte(directoryv1.SignatureDomain+"\x00"), snapshotBytes...))
	envelope := directoryv1.Envelope{EnvelopeSchemaVersion: 1, SnapshotSchemaVersion: 1, Sequence: 41, KeyID: "launch-conformance-test", Algorithm: "Ed25519", SignatureDomain: directoryv1.SignatureDomain, SnapshotDigest: "sha256:" + hex.EncodeToString(digest[:]), Signature: base64.StdEncoding.EncodeToString(signature)}
	envelopeBytes, err := json.Marshal(envelope)
	if err != nil {
		t.Fatal(err)
	}
	paths := map[string]string{
		"AGENTPLUGINS_DIRECTORY_SNAPSHOT":         filepath.Join(root, "snapshot.json"),
		"AGENTPLUGINS_DIRECTORY_ENVELOPE":         filepath.Join(root, "envelope.json"),
		"AGENTPLUGINS_DIRECTORY_TRUST":            filepath.Join(root, "trusted-keys.json"),
		"AGENTPLUGINS_DIRECTORY_CACHE":            filepath.Join(root, "directory-cache"),
		"AGENTPLUGINS_DIRECTORY_ORIGIN":           "https://conformance.invalid/registry/schemas/1/",
		"AGENTPLUGINS_DIRECTORY_CONFORMANCE_ONLY": "1",
	}
	trustBytes := []byte(fmt.Sprintf(`{"schema_version":1,"keys":[{"key_id":"launch-conformance-test","public_key":%q}]}`, base64.StdEncoding.EncodeToString(publicKey)))
	for path, body := range map[string][]byte{paths["AGENTPLUGINS_DIRECTORY_SNAPSHOT"]: snapshotBytes, paths["AGENTPLUGINS_DIRECTORY_ENVELOPE"]: envelopeBytes, paths["AGENTPLUGINS_DIRECTORY_TRUST"]: trustBytes} {
		if err := os.WriteFile(path, body, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return paths
}
