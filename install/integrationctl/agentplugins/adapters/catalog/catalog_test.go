package catalog

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"
)

func TestCatalogLoadsStrictPinnedIndexAndResolvesExactSource(t *testing.T) {
	t.Parallel()
	body := validCatalog()
	sum := sha256.Sum256(body)
	expected := "sha256:" + hex.EncodeToString(sum[:])
	loaded, err := (Loader{CurrentCLIVersion: "0.1.0"}).Load(body, expected)
	if err != nil {
		t.Fatal(err)
	}
	resolved, err := loaded.Resolve("context7")
	if err != nil {
		t.Fatal(err)
	}
	want := "777genius/universal-agent-plugins@0123456789abcdef0123456789abcdef01234567//plugins/context7"
	if resolved.SourceReference != want || resolved.Entry.TreeDigest != digest("a") {
		t.Fatalf("resolution = %+v", resolved)
	}
	if resolved.Hints.OpenAIMCPAuth["context7"].BearerTokenEnvVar != "CONTEXT7_API_KEY" {
		t.Fatalf("hints = %+v", resolved.Hints)
	}
}

func TestCatalogRejectsChecksumUnknownFieldsTraversalAndSupplyChainConflict(t *testing.T) {
	t.Parallel()
	tests := map[string][]byte{
		"checksum":  validCatalog(),
		"unknown":   []byte(strings.Replace(string(validCatalog()), `"schema_version": 1,`, `"schema_version": 1, "future": true,`, 1)),
		"traversal": []byte(strings.Replace(string(validCatalog()), `"plugins/context7"`, `"../context7"`, 1)),
		"conflict":  conflictCatalog(),
	}
	for name, body := range tests {
		name, body := name, body
		t.Run(name, func(t *testing.T) {
			expected := ""
			if name == "checksum" {
				expected = digest("f")
			}
			if _, err := (Loader{CurrentCLIVersion: "0.1.0"}).Load(body, expected); err == nil {
				t.Fatal("invalid catalog accepted")
			}
		})
	}
}

func TestCatalogEnforcesMinimumCLIVersionAtResolution(t *testing.T) {
	t.Parallel()
	body := []byte(strings.Replace(
		string(validCatalog()),
		`"minimum_cli_version": "0.1.0"`,
		`"minimum_cli_version": "0.2.0"`,
		1,
	))
	loaded, err := (Loader{CurrentCLIVersion: "0.1.0"}).Load(body, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := loaded.Resolve("context7"); err == nil || !strings.Contains(err.Error(), "requires agentplugins") {
		t.Fatalf("resolution error = %v", err)
	}
}

func validCatalog() []byte {
	return []byte(`{
  "$schema": "https://github.com/777genius/universal-agent-plugins/schemas/catalog-v1.schema.json",
  "schema_version": 1,
  "catalog_version": "0.1.0",
  "repository": "777genius/universal-agent-plugins",
  "revision": "0123456789abcdef0123456789abcdef01234567",
  "published_at": "2026-08-08T12:00:00Z",
  "plugins": [{
    "name": "context7",
    "version": "1.0.0",
    "agent_plugins_schema": "https://agent-plugins.org/schemas/1.0.0/plugin.schema.json",
    "minimum_cli_version": "0.1.0",
    "source_path": "plugins/context7",
    "tree_digest": "` + digest("a") + `",
    "manifest_digest": "` + digest("b") + `",
    "components": ["mcp"],
    "compatibility": {"cursor": {"package":"native","verification":"tested","authentication":"not_required"}},
    "openai_mcp_auth": {"context7": {"bearer_token_env_var":"CONTEXT7_API_KEY"}}
  }]
}`)
}

func digest(value string) string {
	return "sha256:" + strings.Repeat(value, 64)
}

func conflictCatalog() []byte {
	duplicate := `, {"name":"context7","version":"1.0.0","agent_plugins_schema":"https://agent-plugins.org/schemas/1.0.0/plugin.schema.json","minimum_cli_version":"0.1.0","source_path":"plugins/context7-copy","tree_digest":"` + digest("c") + `","manifest_digest":"` + digest("b") + `","components":["mcp"],"compatibility":{}}`
	return []byte(strings.Replace(string(validCatalog()), "  }]\n}", "  }"+duplicate+"]\n}", 1))
}
