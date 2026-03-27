package github

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/plugin-kit-ai/plugin-kit-ai/plugininstall/domain"
)

func requireBindTests(t *testing.T) {
	t.Helper()
	if os.Getenv("PLUGIN_KIT_AI_BIND_TESTS") == "1" {
		return
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("requires loopback bind support or PLUGIN_KIT_AI_BIND_TESTS=1: %v", err)
	}
	_ = ln.Close()
}

func TestClient_GetReleaseByTag_notFound(t *testing.T) {
	t.Parallel()
	requireBindTests(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	t.Cleanup(srv.Close)

	c := NewClient("")
	c.BaseURL = srv.URL
	c.APIClient = srv.Client()

	_, err := c.GetReleaseByTag(context.Background(), "o", "r", "v1")
	if err == nil {
		t.Fatal("expected error")
	}
	de, ok := err.(*domain.Error)
	if !ok || de.Code != domain.ExitRelease {
		t.Fatalf("got %v", err)
	}
}

func TestClient_GetReleaseByTag_forbidden(t *testing.T) {
	t.Parallel()
	requireBindTests(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	t.Cleanup(srv.Close)

	c := NewClient("")
	c.BaseURL = srv.URL
	c.APIClient = srv.Client()

	_, err := c.GetReleaseByTag(context.Background(), "o", "r", "v1")
	if err == nil {
		t.Fatal("expected error")
	}
	de, ok := err.(*domain.Error)
	if !ok || de.Code != domain.ExitNetwork {
		t.Fatalf("got %v", err)
	}
}

func TestClient_GetReleaseByTag_ok(t *testing.T) {
	t.Parallel()
	requireBindTests(t)
	payload := map[string]any{
		"tag_name":   "v1",
		"draft":      false,
		"prerelease": false,
		"assets":     []any{},
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(payload)
	}))
	t.Cleanup(srv.Close)

	c := NewClient("")
	c.BaseURL = srv.URL
	c.APIClient = srv.Client()

	rel, err := c.GetReleaseByTag(context.Background(), "o", "r", "v1")
	if err != nil {
		t.Fatal(err)
	}
	if rel.TagName != "v1" {
		t.Fatalf("tag %q", rel.TagName)
	}
}

func TestClient_GetLatestRelease_ok(t *testing.T) {
	t.Parallel()
	requireBindTests(t)
	payload := map[string]any{
		"tag_name":   "v9.0.0",
		"draft":      false,
		"prerelease": false,
		"assets":     []any{},
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/o/r/releases/latest" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(payload)
	}))
	t.Cleanup(srv.Close)

	c := NewClient("")
	c.BaseURL = srv.URL
	c.APIClient = srv.Client()

	rel, err := c.GetLatestRelease(context.Background(), "o", "r")
	if err != nil {
		t.Fatal(err)
	}
	if rel.TagName != "v9.0.0" {
		t.Fatalf("tag %q", rel.TagName)
	}
}

func TestClient_GetLatestRelease_notFound(t *testing.T) {
	t.Parallel()
	requireBindTests(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	t.Cleanup(srv.Close)

	c := NewClient("")
	c.BaseURL = srv.URL
	c.APIClient = srv.Client()

	_, err := c.GetLatestRelease(context.Background(), "o", "r")
	if err == nil {
		t.Fatal("expected error")
	}
	de, ok := err.(*domain.Error)
	if !ok || de.Code != domain.ExitRelease {
		t.Fatalf("got %v", err)
	}
}

func TestClient_GetReleaseByTag_responseTooLarge(t *testing.T) {
	t.Parallel()
	requireBindTests(t)
	const maxBytes int64 = 500
	huge := strings.Repeat("a", int(maxBytes)+1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(huge))
	}))
	t.Cleanup(srv.Close)

	c := NewClient("")
	c.BaseURL = srv.URL
	c.APIClient = srv.Client()
	c.ReleaseJSONMaxBytes = maxBytes

	_, err := c.GetReleaseByTag(context.Background(), "o", "r", "v1")
	if err == nil {
		t.Fatal("expected error")
	}
	de, ok := err.(*domain.Error)
	if !ok || de.Code != domain.ExitNetwork {
		t.Fatalf("got %v", err)
	}
	if !strings.Contains(de.Message, "exceeds") {
		t.Fatalf("message %q", de.Message)
	}
}

func TestClient_DownloadAsset_followsHTTPRedirect(t *testing.T) {
	t.Parallel()
	requireBindTests(t)
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/a":
			http.Redirect(w, r, srv.URL+"/b", http.StatusFound)
		case "/b":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("final-body"))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	c := NewClient("")
	c.DLClient = srv.Client()
	body, _, err := c.DownloadAsset(context.Background(), srv.URL+"/a")
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "final-body" {
		t.Fatalf("got %q", body)
	}
}
