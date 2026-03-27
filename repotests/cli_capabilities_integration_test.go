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
	if !strings.Contains(table, "claude") || !strings.Contains(table, "codex") || !strings.Contains(table, "MATURITY") || !strings.Contains(table, "CONTRACT") {
		t.Fatalf("unexpected capabilities table output:\n%s", table)
	}
	if strings.Contains(strings.ToLower(table), "gemini") {
		t.Fatalf("capabilities table should stay runtime-only and exclude gemini:\n%s", table)
	}

	jsonCmd := exec.Command(pluginKitAIBin, "capabilities", "--format", "json", "--platform", "claude")
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
}
