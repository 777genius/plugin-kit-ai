package main

import (
	"context"
	"encoding/base64"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/777genius/plugin-kit-ai/install/integrationctl/agentplugins/adapters/directoryv1"
)

type failingRoundTripper struct{}

func (failingRoundTripper) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, errors.New("publication unavailable")
}

func TestMainUsesProductionDirectoryTrustAndReleaseBootstrap(t *testing.T) {
	t.Setenv("AGENTPLUGINS_DIRECTORY_KEY_ID", "test-current")
	t.Setenv("AGENTPLUGINS_DIRECTORY_PUBLIC_KEY", "11qYAYKxCrfVS/7TyWQHOg7hcvPapiMlrwIaaPcHURo=")
	client, err := newDirectoryClient(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if len(client.Trust.Keys) != 1 || client.Trust.Keys[0].ID != "uap-directory-2026-01" || client.Trust.Keys[0].State != directoryv1.KeyCurrent {
		t.Fatalf("production Directory trust = %+v", client.Trust.Keys)
	}
	if encoded := base64.StdEncoding.EncodeToString(client.Trust.Keys[0].PublicKey); encoded != "HalXARjat+v3ylTPLMAnvuavRo4ZfrF+DbWwsjlp2bI=" {
		t.Fatalf("unexpected production Directory public key: %x", client.Trust.Keys[0].PublicKey)
	}
	if !client.RequireEmbeddedBootstrap {
		t.Fatal("production Directory client did not require a release-bound bootstrap")
	}
	embedded, ready, err := directoryv1.DecodeReleaseBootstrap(generatedProductionDirectoryBootstrap, client.Trust)
	if err != nil || !ready {
		t.Fatalf("generated production bootstrap readiness = %v, %v", ready, err)
	}
	bundle, err := embedded.Verify(client.Trust)
	if err != nil || bundle.Snapshot.Sequence != 2 || bundle.Digest != "sha256:fe6422853423f447d797a54c5c2af0b0eda6f89c23815f8945f5b6f48d50a460" {
		t.Fatalf("generated production bootstrap identity: sequence=%d digest=%q err=%v", bundle.Snapshot.Sequence, bundle.Digest, err)
	}
	client.HTTPClient.Transport = failingRoundTripper{}
	client.Now = func() time.Time { return time.Date(2026, 8, 23, 0, 0, 0, 0, time.UTC) }
	fallback, err := client.Load(context.Background(), 0)
	if err != nil {
		t.Fatalf("offline release bootstrap fallback: %v", err)
	}
	if fallback.Snapshot.Sequence != 2 || fallback.Digest != bundle.Digest {
		t.Fatalf("offline release bootstrap identity: sequence=%d digest=%q", fallback.Snapshot.Sequence, fallback.Digest)
	}
}

func TestHardenedHTTPClientRedirectPolicy(t *testing.T) {
	check := hardenedHTTPClient("Directory").CheckRedirect
	original, _ := http.NewRequest(http.MethodGet, "https://directory.example/registry/latest.json", nil)
	first, _ := http.NewRequest(http.MethodGet, "https://directory.example/registry/one.json", nil)
	first.Header.Set("Authorization", "secret")
	first.Header.Set("Cookie", "secret=yes")
	first.Header.Set("Proxy-Authorization", "secret")
	if err := check(first, []*http.Request{original}); err != nil {
		t.Fatalf("first same-origin redirect: %v", err)
	}
	if first.Header.Get("Authorization") != "" || first.Header.Get("Cookie") != "" || first.Header.Get("Proxy-Authorization") != "" {
		t.Fatal("credentials were forwarded on redirect")
	}
	second, _ := http.NewRequest(http.MethodGet, "https://directory.example/registry/two.json", nil)
	if err := check(second, []*http.Request{original, first}); err != nil {
		t.Fatalf("second same-origin redirect: %v", err)
	}
	third, _ := http.NewRequest(http.MethodGet, "https://directory.example/registry/three.json", nil)
	if err := check(third, []*http.Request{original, first, second}); err == nil {
		t.Fatal("third redirect was accepted")
	}
	crossOrigin, _ := http.NewRequest(http.MethodGet, "https://other.example/registry/latest.json", nil)
	if err := check(crossOrigin, []*http.Request{original}); err == nil {
		t.Fatal("cross-origin redirect was accepted")
	}
	downgrade, _ := http.NewRequest(http.MethodGet, "http://directory.example/registry/latest.json", nil)
	if err := check(downgrade, []*http.Request{original}); err == nil {
		t.Fatal("HTTPS downgrade redirect was accepted")
	}
	discoveryCheck := hardenedHTTPClient("Discovery").CheckRedirect
	if err := discoveryCheck(crossOrigin, []*http.Request{original}); err == nil || !strings.Contains(err.Error(), "Discovery redirect") {
		t.Fatalf("Discovery redirect diagnostic = %v", err)
	}
}

func TestInvalidScopeAndTargetDoNotCreateDataRoot(t *testing.T) {
	originalArgs := os.Args
	defer func() { os.Args = originalArgs }()
	t.Setenv("HOME", t.TempDir())
	for name, args := range map[string][]string{
		"project-scope":  {"agentplugins", "add", "demo", "--scope", "project"},
		"invalid-target": {"agentplugins", "add", "demo", "--target", "not-a-client"},
	} {
		t.Run(name, func(t *testing.T) {
			dataRoot := filepath.Join(t.TempDir(), "must-not-exist")
			t.Setenv("AGENTPLUGINS_HOME", dataRoot)
			os.Args = args
			err := run()
			if err == nil || (name == "project-scope" && !strings.Contains(err.Error(), "user scope")) {
				t.Fatalf("invalid invocation error = %v", err)
			}
			if _, statErr := os.Lstat(dataRoot); !os.IsNotExist(statErr) {
				t.Fatalf("invalid invocation mutated data root: %v", statErr)
			}
		})
	}
}
