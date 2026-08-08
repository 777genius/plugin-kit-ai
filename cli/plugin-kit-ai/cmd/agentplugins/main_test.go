package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"testing"

	"github.com/777genius/plugin-kit-ai/install/integrationctl/agentplugins/adapters/catalog"
)

func TestEmbeddedCatalogIsPinnedAndContainsAllLaunchPlugins(t *testing.T) {
	sum := sha256.Sum256(embeddedCatalog)
	digest := "sha256:" + hex.EncodeToString(sum[:])
	if digest != defaultCatalogDigest {
		t.Fatalf("embedded catalog digest = %s, want %s", digest, defaultCatalogDigest)
	}
	loaded, err := (catalog.Loader{CurrentCLIVersion: "0.1.0"}).Load(embeddedCatalog, defaultCatalogDigest)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Catalog.Plugins) != 26 {
		t.Fatalf("catalog plugins = %d", len(loaded.Catalog.Plugins))
	}
	resolution, err := loaded.Resolve("context7")
	if err != nil {
		t.Fatal(err)
	}
	want := fmt.Sprintf("777genius/universal-agent-plugins@%s//plugins/context7", loaded.Catalog.Revision)
	if resolution.SourceReference != want {
		t.Fatalf("context7 source = %s", resolution.SourceReference)
	}
}
