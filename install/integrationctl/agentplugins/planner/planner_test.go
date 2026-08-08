package planner

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/777genius/plugin-kit-ai/install/integrationctl/agentplugins/domain"
)

func TestPlannerNegotiatesPartialSupportWithoutRejectingSupportedComponents(t *testing.T) {
	t.Parallel()
	planner := Planner{ManagedRoot: t.TempDir()}
	envelope := testEnvelope()
	client := detectedClient(domain.ClientCodex, filepath.Join(t.TempDir(), ".codex"))
	plan, err := planner.Plan(context.Background(), envelope, client, domain.ScopeUser, "demo-0123456789ab")
	if err != nil {
		t.Fatal(err)
	}
	if plan.Status != domain.PlanManualActivationRequired || plan.PackageMode != domain.PackageProjection {
		t.Fatalf("plan = %+v", plan)
	}
	if supportOf(plan, domain.ComponentSkill, "docs") != domain.SupportProjected {
		t.Fatal("skill was not projected")
	}
	if supportOf(plan, domain.ComponentExtension, "cursor") != domain.SupportUnsupported {
		t.Fatal("unsupported extension was not isolated")
	}
	if plan.Authentication != domain.AuthenticationNotChecked {
		t.Fatalf("authentication = %s", plan.Authentication)
	}
}

func TestPlannerUsesNativeCursorTargetAndDoesNotExposeItInJSON(t *testing.T) {
	t.Parallel()
	config := filepath.Join(t.TempDir(), ".cursor")
	planner := Planner{ManagedRoot: t.TempDir()}
	plan, err := planner.Plan(context.Background(), testEnvelope(), detectedClient(domain.ClientCursor, config), domain.ScopeUser, "demo-0123456789ab")
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(config, "plugins", "local", "demo-0123456789ab")
	if plan.ActivePath != want || plan.Status != domain.PlanManualActivationRequired || plan.PackageMode != domain.PackageNative {
		t.Fatalf("plan = %+v, want active %s", plan, want)
	}
	body, err := json.Marshal(plan)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), config) {
		t.Fatalf("public JSON leaked path: %s", body)
	}
}

func TestPlannerKeepsVSCodeManualEvenWhenCopilotIsDetected(t *testing.T) {
	t.Parallel()
	client := detectedClient(domain.ClientVSCode, filepath.Join(t.TempDir(), "Code", "User"))
	withoutBridge := Planner{ManagedRoot: t.TempDir()}
	manual, err := withoutBridge.Plan(context.Background(), testEnvelope(), client, domain.ScopeUser, "demo-0123456789ab")
	if err != nil {
		t.Fatal(err)
	}
	if manual.Status != domain.PlanManualActivationRequired || manual.PackageMode != domain.PackagePrepared {
		t.Fatalf("manual plan = %+v", manual)
	}
	withBridge := Planner{
		ManagedRoot: t.TempDir(),
		Detected: map[domain.ClientID]domain.DetectedClient{
			domain.ClientCopilot: {
				ClientID: domain.ClientCopilot, Status: domain.DetectionDetected, ExecutablePath: "/test/bin/copilot",
			},
		},
	}
	bridged, err := withBridge.Plan(context.Background(), testEnvelope(), client, domain.ScopeUser, "demo-0123456789ab")
	if err != nil {
		t.Fatal(err)
	}
	if bridged.Status != domain.PlanManualActivationRequired || bridged.PackageMode != domain.PackagePrepared || bridged.Activation != domain.ActivationManual {
		t.Fatalf("bridged plan = %+v", bridged)
	}
}

func TestPlannerFailsClosedForUndetectedClientAndUnsupportedScope(t *testing.T) {
	t.Parallel()
	planner := Planner{ManagedRoot: t.TempDir()}
	client := domain.DetectedClient{ClientID: domain.ClientCursor, Status: domain.DetectionNotDetected}
	plan, err := planner.Plan(context.Background(), testEnvelope(), client, domain.ScopeUser, "demo-0123456789ab")
	if err != nil {
		t.Fatal(err)
	}
	if plan.Status != domain.PlanUnsupported || !contains(plan.Warnings, "client_not_detected") {
		t.Fatalf("undetected plan = %+v", plan)
	}
	client = detectedClient(domain.ClientCursor, filepath.Join(t.TempDir(), ".cursor"))
	plan, err = planner.Plan(context.Background(), testEnvelope(), client, domain.ScopeProject, "demo-0123456789ab")
	if err != nil {
		t.Fatal(err)
	}
	if plan.Status != domain.PlanUnsupported || !contains(plan.Warnings, "scope_not_supported") {
		t.Fatalf("project plan = %+v", plan)
	}
}

func TestPlannerRejectsMetadataOnlyProjectionWhenAllDeclaredComponentsAreInvalid(t *testing.T) {
	t.Parallel()
	for name, diagnostic := range map[string]domain.Diagnostic{
		"malformed_only_mcp": {
			Severity: domain.SeverityError, Boundary: domain.BoundaryMCP,
			Code: "mcp_malformed", Message: "malformed MCP document",
		},
		"all_invalid_skills": {
			Severity: domain.SeverityError, Boundary: domain.BoundarySkill,
			Code: "skill_invalid", Message: "all skills are invalid",
		},
	} {
		name, diagnostic := name, diagnostic
		t.Run(name, func(t *testing.T) {
			envelope := domain.PackageEnvelope{
				Manifest:    domain.PluginManifest{Name: "broken"},
				Diagnostics: []domain.Diagnostic{diagnostic},
			}
			plan, err := (Planner{ManagedRoot: t.TempDir()}).Plan(
				context.Background(), envelope,
				detectedClient(domain.ClientCursor, filepath.Join(t.TempDir(), ".cursor")),
				domain.ScopeUser, "broken-0123456789ab",
			)
			if err != nil {
				t.Fatal(err)
			}
			if plan.Status != domain.PlanUnsupported || plan.Activation != domain.ActivationFailed || !contains(plan.Warnings, "no_valid_components") {
				t.Fatalf("plan = %+v", plan)
			}
		})
	}
}

func testEnvelope() domain.PackageEnvelope {
	return domain.PackageEnvelope{
		Manifest: domain.PluginManifest{
			Name:       "demo",
			Extensions: map[string]json.RawMessage{"cursor": json.RawMessage(`{"enabled":true}`)},
		},
		MCP: domain.MCPComponent{
			Present: true, Enabled: true,
			Servers: map[string]domain.MCPServer{
				"remote": {Name: "remote", Type: "streamable-http"},
			},
		},
		Skills: map[string]domain.Skill{"docs": {Name: "docs"}},
	}
}

func detectedClient(id domain.ClientID, config string) domain.DetectedClient {
	return domain.DetectedClient{ClientID: id, Status: domain.DetectionDetected, ConfigRoot: config}
}

func supportOf(plan domain.DeliveryPlan, kind domain.ComponentKind, name string) domain.SupportLevel {
	for _, component := range plan.Components {
		if component.Kind == kind && component.Name == name {
			return component.Support
		}
	}
	return ""
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
