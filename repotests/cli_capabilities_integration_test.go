package pluginkitairepo_test

import (
	"encoding/json"
	"os/exec"
	"strings"
	"testing"
)

func TestPluginKitAICapabilities(t *testing.T) {
	pluginKitAIBin := buildPluginKitAI(t)

	tableCmd := exec.Command(pluginKitAIBin, "capabilities")
	tableOut, err := tableCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("capabilities table: %v\n%s", err, tableOut)
	}
	table := string(tableOut)
	if !strings.Contains(table, "claude") || !strings.Contains(table, "codex-package") || !strings.Contains(table, "codex-runtime") || !strings.Contains(table, "gemini") || !strings.Contains(table, "cursor") || !strings.Contains(table, "TARGET") || !strings.Contains(table, "CLASS") {
		t.Fatalf("unexpected capabilities table output:\n%s", table)
	}

	jsonCmd := exec.Command(pluginKitAIBin, "capabilities", "--mode", "runtime", "--format", "json", "--platform", "claude")
	jsonOut, err := jsonCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("capabilities json: %v\n%s", err, jsonOut)
	}
	var entries []map[string]any
	if err := json.Unmarshal(jsonOut, &entries); err != nil {
		t.Fatalf("parse capabilities json: %v\n%s", err, jsonOut)
	}
	if len(entries) == 0 {
		t.Fatal("expected at least one capabilities entry")
	}
	for _, entry := range entries {
		if entry["platform"] != "claude" {
			t.Fatalf("unexpected platform entry: %+v", entry)
		}
		if entry["maturity"] == "" {
			t.Fatalf("missing maturity entry: %+v", entry)
		}
		if entry["contract_class"] == "" {
			t.Fatalf("missing contract_class entry: %+v", entry)
		}
		if entry["scaffold_support"] != true || entry["validate_support"] != true {
			t.Fatalf("expected scaffold/validate support in entry: %+v", entry)
		}
	}

	targetJSONCmd := exec.Command(pluginKitAIBin, "capabilities", "--mode", "targets", "--format", "json", "--platform", "codex-package")
	targetJSONOut, err := targetJSONCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("target capabilities json: %v\n%s", err, targetJSONOut)
	}
	var targetEntries []map[string]any
	if err := json.Unmarshal(targetJSONOut, &targetEntries); err != nil {
		t.Fatalf("parse target capabilities json: %v\n%s", err, targetJSONOut)
	}
	if len(targetEntries) != 1 {
		t.Fatalf("expected one target capabilities entry, got %d: %s", len(targetEntries), targetJSONOut)
	}
	target := targetEntries[0]
	if target["target"] != "codex-package" {
		t.Fatalf("unexpected target entry: %+v", target)
	}
	nativeDocs, ok := target["native_docs"].([]any)
	if !ok || len(nativeDocs) == 0 {
		t.Fatalf("missing native_docs entry: %+v", target)
	}
	nativeDocPaths, ok := target["native_doc_paths"].(map[string]any)
	if !ok {
		t.Fatalf("missing native_doc_paths entry: %+v", target)
	}
	if nativeDocPaths["interface"] != "targets/codex-package/interface.json" {
		t.Fatalf("native_doc_paths[interface] = %v", nativeDocPaths["interface"])
	}
	if nativeDocPaths["package_metadata"] != "targets/codex-package/package.yaml" {
		t.Fatalf("native_doc_paths[package_metadata] = %v", nativeDocPaths["package_metadata"])
	}
	nativeSurfaceTiers, ok := target["native_surface_tiers"].(map[string]any)
	if !ok {
		t.Fatalf("missing native_surface_tiers entry: %+v", target)
	}
	if nativeSurfaceTiers["interface"] != "stable" {
		t.Fatalf("native_surface_tiers[interface] = %v", nativeSurfaceTiers["interface"])
	}
	if nativeSurfaceTiers["app_manifest"] != "beta" {
		t.Fatalf("native_surface_tiers[app_manifest] = %v", nativeSurfaceTiers["app_manifest"])
	}
}
