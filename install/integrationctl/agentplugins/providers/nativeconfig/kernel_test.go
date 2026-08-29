package nativeconfig

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestMCPServersAddPreservesUnrelatedConfigAndResolvesExplicitPaths(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "client.json")
	original := `{"theme":"night","mcpServers":{"foreign":{"url":"https://foreign.test"}}}`
	mustWrite(t, path, original)

	receipt, err := New().Apply(Request{
		Paths:        Paths{JSON: path, JSONC: filepath.Join(root, "client.jsonc")},
		Codec:        CodecMCPServers,
		Action:       ActionAdd,
		Name:         "owned",
		Server:       Server{Type: "stdio", Command: "node", Args: []string{"${PLUGIN_ROOT}/server.js", "--data=${PLUGIN_DATA}"}, Env: map[string]string{"ROOT": "${package.root}"}},
		Placeholders: Placeholders{PackageRoot: "/explicit/pkg", DataRoot: "/explicit/data"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if receipt.Version != "1" || receipt.Path != path || receipt.Codec != CodecMCPServers || receipt.Name != "owned" || !strings.HasPrefix(receipt.Digest, "sha256:") {
		t.Fatalf("unexpected receipt: %#v", receipt)
	}
	var doc map[string]any
	mustReadJSON(t, path, &doc)
	if doc["theme"] != "night" {
		t.Fatalf("unrelated config lost: %#v", doc)
	}
	servers := doc["mcpServers"].(map[string]any)
	if servers["foreign"].(map[string]any)["url"] != "https://foreign.test" {
		t.Fatalf("foreign entry changed: %#v", servers)
	}
	owned := servers["owned"].(map[string]any)
	if owned["command"] != "node" || owned["args"].([]any)[0] != "/explicit/pkg/server.js" || owned["args"].([]any)[1] != "--data=/explicit/data" || owned["env"].(map[string]any)["ROOT"] != "/explicit/pkg" {
		t.Fatalf("wrong projection: %#v", owned)
	}
	info, _ := os.Stat(path)
	if info.Mode().Perm() != 0o640 {
		t.Fatalf("existing mode changed: %o", info.Mode().Perm())
	}
}

func TestOpenCodeJSONCAddPreservesCommentsAndProjectsBothTransports(t *testing.T) {
	root := t.TempDir()
	jsonPath := filepath.Join(root, "opencode.json")
	jsoncPath := filepath.Join(root, "opencode.jsonc")
	mustWriteMode(t, jsoncPath, `{
  // keep this theme comment
  "theme": "night",
  "mcp": {
    // foreign entry comment
    "foreign": {"type":"remote", "url":"https://foreign.test"},
  },
}
`, 0o600)
	kernel := New()
	local, err := kernel.Apply(Request{
		Paths: Paths{JSON: jsonPath, JSONC: jsoncPath}, Codec: CodecOpenCode, Action: ActionAdd, Name: "local",
		Server:       Server{Type: "stdio", Command: "node", Args: []string{"${PLUGIN_ROOT}/run.js"}, Env: map[string]string{"DATA": "${PLUGIN_DATA}"}},
		Placeholders: Placeholders{PackageRoot: "/pkg", DataRoot: "/data"},
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = kernel.Apply(Request{
		Paths: Paths{JSON: jsonPath, JSONC: jsoncPath}, Codec: CodecOpenCode, Action: ActionAdd, Name: "remote",
		Server: Server{Type: "remote", URL: "https://mcp.test", Headers: map[string]string{"Authorization": "Bearer token"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	body := mustRead(t, jsoncPath)
	for _, comment := range []string{"// keep this theme comment", "// foreign entry comment"} {
		if !strings.Contains(body, comment) {
			t.Fatalf("comment %q was lost:\n%s", comment, body)
		}
	}
	doc, err := parseDocument([]byte(body), true)
	if err != nil {
		t.Fatal(err)
	}
	mcp, _ := collection(doc, "mcp", false)
	localMember, _ := objectMember(mcp, "local")
	var projected map[string]any
	standard, _ := entryCanonical(localMember)
	_ = json.Unmarshal(standard, &projected)
	if projected["type"] != "local" || projected["command"].([]any)[1] != "/pkg/run.js" || projected["environment"].(map[string]any)["DATA"] != "/data" {
		t.Fatalf("wrong OpenCode local projection: %#v", projected)
	}
	if local.Path != jsoncPath {
		t.Fatalf("receipt selected wrong config: %#v", local)
	}
	remoteMember, _ := objectMember(mcp, "remote")
	standard, _ = entryCanonical(remoteMember)
	_ = json.Unmarshal(standard, &projected)
	if projected["type"] != "remote" || projected["url"] != "https://mcp.test" {
		t.Fatalf("wrong OpenCode remote projection: %#v", projected)
	}
}

func TestCollisionUpdateAndRemoveRequireExactOwnership(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "config.json")
	mustWrite(t, path, `{"mcpServers":{"docs":{"command":"foreign"}}}`)
	kernel := New()
	req := Request{Paths: Paths{JSON: path}, Codec: CodecMCPServers, Action: ActionAdd, Name: "docs", Server: Server{Type: "stdio", Command: "node"}}
	before := mustRead(t, path)
	if _, err := kernel.Apply(req); !errors.Is(err, ErrCollision) {
		t.Fatalf("expected collision, got %v", err)
	}
	assertBytes(t, path, before)

	req.Name = "owned"
	receipt, err := kernel.Apply(req)
	if err != nil {
		t.Fatal(err)
	}
	forged := receipt
	forged.Digest = "sha256:00"
	update := req
	update.Action, update.Owned, update.Server = ActionUpdate, &forged, Server{Type: "stdio", Command: "deno"}
	before = mustRead(t, path)
	if _, err := kernel.Apply(update); !errors.Is(err, ErrNotOwned) {
		t.Fatalf("expected ownership rejection, got %v", err)
	}
	assertBytes(t, path, before)
	update.Owned = &receipt
	updated, err := kernel.Apply(update)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Digest == receipt.Digest {
		t.Fatal("updated entry digest did not change")
	}
	remove := update
	remove.Action, remove.Owned = ActionRemove, &receipt
	if _, err := kernel.Apply(remove); !errors.Is(err, ErrNotOwned) {
		t.Fatalf("stale receipt removed updated entry: %v", err)
	}
	remove.Owned = &updated
	if _, err := kernel.Apply(remove); err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	mustReadJSON(t, path, &doc)
	servers := doc["mcpServers"].(map[string]any)
	if _, exists := servers["owned"]; exists || servers["docs"].(map[string]any)["command"] != "foreign" {
		t.Fatalf("remove touched wrong entry: %#v", servers)
	}
}

func TestApplyBatchWritesAllEntriesOnceAndFailsClosedOnCollision(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "settings.json")
	mustWrite(t, path, `{"theme":"night","mcpServers":{"foreign":{"url":"https://foreign.test"}}}`)
	kernel := New()
	requests := []Request{
		{Paths: Paths{JSON: path}, Codec: CodecGemini, Action: ActionAdd, Name: "local", Server: Server{Type: "stdio", Command: "node", CWD: "${PLUGIN_ROOT}/server"}, Placeholders: Placeholders{PackageRoot: "/pkg"}},
		{Paths: Paths{JSON: path}, Codec: CodecGemini, Action: ActionAdd, Name: "docs", Server: Server{Type: "remote", URL: "https://docs.test/mcp", RemoteTransport: "streamable-http"}},
		{Paths: Paths{JSON: path}, Codec: CodecGemini, Action: ActionAdd, Name: "events", Server: Server{Type: "remote", URL: "https://events.test/sse", RemoteTransport: "sse"}},
	}
	receipts, err := kernel.ApplyBatch(requests)
	if err != nil {
		t.Fatal(err)
	}
	if len(receipts) != 3 || receipts[0].Codec != CodecGemini {
		t.Fatalf("unexpected receipts: %#v", receipts)
	}
	var doc map[string]any
	mustReadJSON(t, path, &doc)
	servers := doc["mcpServers"].(map[string]any)
	if servers["local"].(map[string]any)["cwd"] != "/pkg/server" {
		t.Fatalf("Gemini cwd was not projected: %#v", servers["local"])
	}
	if servers["docs"].(map[string]any)["httpUrl"] != "https://docs.test/mcp" {
		t.Fatalf("Gemini streamable HTTP key was not projected: %#v", servers["docs"])
	}
	if servers["events"].(map[string]any)["url"] != "https://events.test/sse" {
		t.Fatalf("Gemini SSE key was not projected: %#v", servers["events"])
	}
	present, owned, err := kernel.Inspect(Paths{JSON: path}, CodecGemini, "docs", &receipts[1])
	if err != nil || !present || !owned {
		t.Fatalf("exact Gemini ownership was not observed: present=%v owned=%v err=%v", present, owned, err)
	}
	forged := receipts[1]
	forged.Digest = "sha256:00"
	_, owned, err = kernel.Inspect(Paths{JSON: path}, CodecGemini, "docs", &forged)
	if err != nil || owned {
		t.Fatalf("forged Gemini ownership was accepted: owned=%v err=%v", owned, err)
	}

	before := mustRead(t, path)
	_, err = kernel.ApplyBatch([]Request{
		{Paths: Paths{JSON: path}, Codec: CodecGemini, Action: ActionAdd, Name: "new", Server: Server{Type: "stdio", Command: "node"}},
		{Paths: Paths{JSON: path}, Codec: CodecGemini, Action: ActionAdd, Name: "foreign", Server: Server{Type: "stdio", Command: "node"}},
	})
	if !errors.Is(err, ErrCollision) {
		t.Fatalf("expected batch collision, got %v", err)
	}
	assertBytes(t, path, before)
}

func TestMalformedDuplicateAndAmbiguousInputsFailClosed(t *testing.T) {
	tests := []string{
		`{"mcpServers":{},"mcpServers":{}}`,
		`{"mcpServers":{"x":{"env":{"A":"1","A":"2"}}}}`,
		`{"mcpServers":`,
		`[]`,
		`{"mcpServers":[]}`,
	}
	for i, body := range tests {
		t.Run(string(rune('a'+i)), func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.json")
			mustWrite(t, path, body)
			before := mustRead(t, path)
			_, err := New().Apply(Request{Paths: Paths{JSON: path}, Codec: CodecMCPServers, Action: ActionAdd, Name: "owned", Server: Server{Type: "stdio", Command: "node"}})
			if !errors.Is(err, ErrMalformed) {
				t.Fatalf("expected malformed error, got %v", err)
			}
			assertBytes(t, path, before)
		})
	}
	root := t.TempDir()
	jsonPath, jsoncPath := filepath.Join(root, "config.json"), filepath.Join(root, "config.jsonc")
	mustWrite(t, jsonPath, `{}`)
	mustWrite(t, jsoncPath, `{}`)
	_, err := New().Apply(Request{Paths: Paths{JSON: jsonPath, JSONC: jsoncPath}, Codec: CodecMCPServers, Action: ActionAdd, Name: "owned", Server: Server{Type: "stdio", Command: "node"}})
	if !errors.Is(err, ErrAmbiguousConfig) {
		t.Fatalf("expected ambiguity error, got %v", err)
	}
}

func TestPlaceholdersAndSchemaValidationFailBeforeWrite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	mustWrite(t, path, `{"unrelated":true}`)
	before := mustRead(t, path)
	cases := []Server{
		{Type: "stdio", Command: "node", Args: []string{"${PLUGIN_ROOT}/run.js"}},
		{Type: "stdio", Command: "node", Args: []string{"${HOME}/run.js"}},
		{Type: "stdio", Command: "node", Headers: map[string]string{"X": "ignored"}},
		{Type: "remote", URL: "https://mcp.test", Command: "node"},
		{Type: "stdio", Command: "", URL: "https://mcp.test"},
	}
	for _, server := range cases {
		_, err := New().Apply(Request{Paths: Paths{JSON: path}, Codec: CodecMCPServers, Action: ActionAdd, Name: "owned", Server: server})
		if err == nil {
			t.Fatalf("expected projection failure for %#v", server)
		}
		assertBytes(t, path, before)
	}
}

func TestRelativePathsAreRejected(t *testing.T) {
	_, err := New().Apply(Request{Paths: Paths{JSON: "config.json"}, Codec: CodecMCPServers, Action: ActionAdd, Name: "owned", Server: Server{Type: "stdio", Command: "node"}})
	if err == nil || !strings.Contains(err.Error(), "absolute") {
		t.Fatalf("expected absolute path rejection, got %v", err)
	}
}

type interleavingIO struct {
	osFiles
	reads int
}

func (io *interleavingIO) ReadNoFollow(path string) ([]byte, os.FileMode, bool, error) {
	io.reads++
	if io.reads == 2 {
		if err := os.WriteFile(path, []byte(`{"client":"concurrent"}`), 0o640); err != nil {
			return nil, 0, false, err
		}
	}
	return io.osFiles.ReadNoFollow(path)
}

func TestConcurrentChangeBeforeReplaceIsNotOverwritten(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	mustWrite(t, path, `{"client":"original"}`)
	_, err := NewWithFileIO(&interleavingIO{}).Apply(Request{Paths: Paths{JSON: path}, Codec: CodecMCPServers, Action: ActionAdd, Name: "owned", Server: Server{Type: "stdio", Command: "node"}})
	if !errors.Is(err, ErrConcurrentChange) {
		t.Fatalf("expected concurrent change, got %v", err)
	}
	assertBytes(t, path, `{"client":"concurrent"}`)
}

func TestNoFollowRefusesSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation is not generally available to unprivileged Windows tests")
	}
	root := t.TempDir()
	target := filepath.Join(root, "target.json")
	link := filepath.Join(root, "config.json")
	mustWrite(t, target, `{"safe":true}`)
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	_, err := New().Apply(Request{Paths: Paths{JSON: link}, Codec: CodecMCPServers, Action: ActionAdd, Name: "owned", Server: Server{Type: "stdio", Command: "node"}})
	if err == nil {
		t.Fatal("expected symlink refusal")
	}
	assertBytes(t, target, `{"safe":true}`)
}

type faultIO struct {
	osFiles
	mode       string
	writeCount int
}

func (io *faultIO) WriteAtomic(path string, body []byte, mode os.FileMode) error {
	io.writeCount++
	if io.writeCount == 1 {
		if err := os.WriteFile(path, []byte("corrupt"), mode); err != nil {
			return err
		}
		if io.mode == "error" {
			return errors.New("injected post-write failure")
		}
		return nil
	}
	return io.osFiles.WriteAtomic(path, body, mode)
}

func TestFailedOrIncorrectWriteRestoresExactOriginalBytes(t *testing.T) {
	for _, mode := range []string{"error", "incorrect"} {
		t.Run(mode, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.jsonc")
			original := "{\n  // exact bytes must return\n  \"theme\": \"night\",\n}\n"
			mustWrite(t, path, original)
			files := &faultIO{mode: mode}
			_, err := NewWithFileIO(files).Apply(Request{Paths: Paths{JSON: filepath.Join(filepath.Dir(path), "config.json"), JSONC: path}, Codec: CodecOpenCode, Action: ActionAdd, Name: "owned", Server: Server{Type: "stdio", Command: "node"}})
			if err == nil {
				t.Fatal("expected injected write failure")
			}
			assertBytes(t, path, original)
			if files.writeCount != 2 {
				t.Fatalf("expected write plus rollback, got %d writes", files.writeCount)
			}
		})
	}
}

func TestNewFileUsesPrivateModeAndStableDigest(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "config.json")
	req := Request{Paths: Paths{JSON: path}, Codec: CodecMCPServers, Action: ActionAdd, Name: "owned", Server: Server{Type: "stdio", Command: "node", Env: map[string]string{"B": "2", "A": "1"}}}
	receipt, err := New().Apply(req)
	if err != nil {
		t.Fatal(err)
	}
	info, _ := os.Stat(path)
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("new config mode is %o", info.Mode().Perm())
	}
	update := req
	update.Action, update.Owned = ActionUpdate, &receipt
	next, err := New().Apply(update)
	if err != nil {
		t.Fatal(err)
	}
	if next.Digest != receipt.Digest {
		t.Fatalf("semantically identical entry digest changed: %s != %s", next.Digest, receipt.Digest)
	}
}

func mustWrite(t *testing.T, path, body string) { t.Helper(); mustWriteMode(t, path, body, 0o640) }

func mustWriteMode(t *testing.T, path, body string, mode os.FileMode) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), mode); err != nil {
		t.Fatal(err)
	}
}

func mustRead(t *testing.T, path string) string {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}

func assertBytes(t *testing.T, path, want string) {
	t.Helper()
	if got := mustRead(t, path); got != want {
		t.Fatalf("bytes changed:\nwant %q\n got %q", want, got)
	}
}

func mustReadJSON(t *testing.T, path string, out any) {
	t.Helper()
	if err := json.Unmarshal([]byte(mustRead(t, path)), out); err != nil {
		t.Fatal(err)
	}
}
