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

	"github.com/777genius/plugin-kit-ai/install/integrationctl/agentplugins/adapters/directoryv1"
)

type failingRoundTripper struct{}

func (failingRoundTripper) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, errors.New("publication unavailable")
}

func TestMainUsesProductionDirectoryTrustAndFailsClosedWithoutBootstrap(t *testing.T) {
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
	if len(client.Embedded.Snapshot) != 0 || len(client.Embedded.Envelope) != 0 {
		t.Fatal("a non-production bootstrap was embedded")
	}
	client.HTTPClient.Transport = failingRoundTripper{}
	if _, err := client.Load(context.Background(), 0); err == nil {
		t.Fatal("short-name Directory load succeeded without a published snapshot or cache")
	}
}

func TestHardenedHTTPClientRedirectPolicy(t *testing.T) {
	check := hardenedHTTPClient().CheckRedirect
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
