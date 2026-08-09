package planner

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/777genius/plugin-kit-ai/install/integrationctl/adapters/pathpolicy"
	"github.com/777genius/plugin-kit-ai/install/integrationctl/agentplugins/domain"
)

type Planner struct {
	ManagedRoot string
	Detected    map[domain.ClientID]domain.DetectedClient
}

func (planner Planner) Plan(
	ctx context.Context,
	envelope domain.PackageEnvelope,
	client domain.DetectedClient,
	scope domain.InstallScope,
	physicalArtifactID string,
) (domain.DeliveryPlan, error) {
	if err := ctx.Err(); err != nil {
		return domain.DeliveryPlan{}, err
	}
	if err := pathpolicy.ValidateLeafID(physicalArtifactID); err != nil {
		return domain.DeliveryPlan{}, fmt.Errorf("invalid physical artifact id: %w", err)
	}
	capabilities, ok := Capabilities(client.ClientID)
	if !ok {
		return domain.DeliveryPlan{}, fmt.Errorf("unsupported client %q", client.ClientID)
	}
	plan := domain.DeliveryPlan{
		ClientID:    client.ClientID,
		Scope:       scope,
		Status:      statusFor(capabilities.ActivationMode),
		PackageMode: capabilities.PackageMode,
		Activation:  activationFor(capabilities.ActivationMode),
		// A package loaded without catalog evidence has unknown authentication
		// requirements. Only affirmative per-client evidence may mark it as not
		// required.
		Authentication:     domain.AuthenticationNotChecked,
		Policy:             domain.PolicyAllowed,
		Verification:       domain.VerificationPackageValid,
		PhysicalArtifactID: physicalArtifactID,
	}
	if client.Status != domain.DetectionDetected {
		plan.Status = domain.PlanUnsupported
		plan.Activation = domain.ActivationFailed
		plan.Warnings = append(plan.Warnings, "client_not_detected")
		return plan, nil
	}
	if !supportsScope(capabilities.Scopes, scope) {
		plan.Status = domain.PlanUnsupported
		plan.Activation = domain.ActivationFailed
		plan.Warnings = append(plan.Warnings, "scope_not_supported")
		return plan, nil
	}
	target, err := planner.ResolveTarget(ctx, client, scope, physicalArtifactID)
	if err != nil {
		return domain.DeliveryPlan{}, err
	}
	plan.TargetRoot = target.TargetRoot
	plan.TargetAnchor = target.TargetAnchor
	plan.ActivePath = target.ActivePath
	plan.Components = componentDecisions(envelope, capabilities)
	applyCatalogCompatibility(&plan, envelope.CatalogEvidence)
	hasComponentErrors := false
	for _, diagnostic := range envelope.Diagnostics {
		plan.Diagnostics = append(plan.Diagnostics, diagnostic)
		plan.Warnings = appendUnique(plan.Warnings, diagnostic.Code)
		if diagnostic.Severity == domain.SeverityError {
			if diagnostic.Boundary == domain.BoundaryMCP || diagnostic.Boundary == domain.BoundaryMCPServer || diagnostic.Boundary == domain.BoundarySkill || diagnostic.Boundary == domain.BoundaryExtension {
				hasComponentErrors = true
			}
		}
	}
	if plan.Authentication == domain.AuthenticationPending {
		plan.UserActions = append(plan.UserActions, "complete authentication for this plugin in the selected client")
	} else if plan.Authentication == domain.AuthenticationNotChecked {
		plan.UserActions = append(plan.UserActions, "verify the plugin's authentication requirements before using it")
	}
	if !hasComponents(plan.Components) && hasComponentErrors {
		plan.Status = domain.PlanUnsupported
		plan.Activation = domain.ActivationFailed
		plan.Warnings = append(plan.Warnings, "no_valid_components")
	} else if hasComponents(plan.Components) && !hasSupportedComponent(plan.Components) {
		plan.Status = domain.PlanUnsupported
		plan.Activation = domain.ActivationFailed
		plan.Warnings = append(plan.Warnings, "no_supported_components")
	}
	if plan.Status != domain.PlanUnsupported && planner.hasNativeCopilotBackend(client) {
		plan.Status = domain.PlanReady
		plan.Activation = domain.ActivationPrepared
	}

	switch client.ClientID {
	case domain.ClientCodex:
		plan.UserActions = append(plan.UserActions, "finish installation in Codex or ChatGPT Plugins, then start a new session")
	case domain.ClientCursor:
		plan.UserActions = append(plan.UserActions, "reload Cursor, then verify the plugin appears before using its components")
	case domain.ClientCopilot:
		if strings.TrimSpace(client.ExecutablePath) != "" {
			plan.UserActions = append(plan.UserActions, "agentplugins will install and verify the plugin through GitHub Copilot CLI automatically")
		} else {
			plan.UserActions = append(plan.UserActions, "GitHub Copilot CLI is required for automatic activation")
		}
	case domain.ClientKiro:
		plan.UserActions = append(plan.UserActions, "finish the prepared Power installation in Kiro")
	case domain.ClientVSCode:
		if strings.TrimSpace(planner.Detected[domain.ClientCopilot].ExecutablePath) != "" {
			plan.UserActions = append(plan.UserActions, "agentplugins will install through GitHub Copilot CLI; VS Code discovers it automatically")
		} else {
			plan.UserActions = append(plan.UserActions, "register the prepared local plugin in VS Code after installation")
		}
	}
	return plan, nil
}

func applyCatalogCompatibility(plan *domain.DeliveryPlan, evidence *domain.CatalogEvidence) {
	if evidence == nil {
		plan.Warnings = appendUnique(plan.Warnings, "authentication_not_catalog_verified")
		return
	}
	compatibility, ok := evidence.Compatibility[string(plan.ClientID)]
	if !ok {
		plan.Status = domain.PlanUnsupported
		plan.Activation = domain.ActivationFailed
		plan.Warnings = appendUnique(plan.Warnings, "client_compatibility_not_catalog_verified")
		plan.UserActions = append(plan.UserActions, "choose a client present in the pinned catalog evidence")
		return
	}
	switch compatibility.Authentication {
	case domain.AuthenticationRequirementNotRequired:
		plan.Authentication = domain.AuthenticationNotRequired
	case domain.AuthenticationRequirementRequired:
		plan.Authentication = domain.AuthenticationPending
	default:
		plan.Authentication = domain.AuthenticationNotChecked
		plan.Warnings = appendUnique(plan.Warnings, "authentication_requirement_unknown")
	}
	if compatibility.Package == "unsupported" {
		plan.Status = domain.PlanUnsupported
		plan.Activation = domain.ActivationFailed
		plan.Warnings = appendUnique(plan.Warnings, "catalog_client_unsupported")
		plan.UserActions = append(plan.UserActions, "choose a client marked compatible by the pinned catalog")
	} else if !catalogPackageMatches(compatibility.Package, plan.PackageMode) {
		plan.Status = domain.PlanUnsupported
		plan.Activation = domain.ActivationFailed
		plan.Warnings = appendUnique(plan.Warnings, "catalog_package_mode_mismatch")
		plan.UserActions = append(plan.UserActions, "update agentplugins or choose a client whose catalog package mode is supported")
	}
	if compatibility.Verification == "schema_only" || compatibility.Verification == "not_tested" {
		plan.Warnings = appendUnique(plan.Warnings, "catalog_"+compatibility.Verification)
		plan.UserActions = append(plan.UserActions, "verify the plugin in the selected client before relying on it")
	}
}

func catalogPackageMatches(value string, mode domain.PackageMode) bool {
	switch value {
	case "native":
		return mode == domain.PackageNative
	case "projected":
		return mode == domain.PackageProjection
	case "prepared":
		return mode == domain.PackagePrepared
	default:
		return false
	}
}

func appendUnique(values []string, value string) []string {
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

func (planner Planner) hasNativeCopilotBackend(client domain.DetectedClient) bool {
	switch client.ClientID {
	case domain.ClientCopilot:
		return strings.TrimSpace(client.ExecutablePath) != ""
	case domain.ClientVSCode:
		return strings.TrimSpace(planner.Detected[domain.ClientCopilot].ExecutablePath) != ""
	default:
		return false
	}
}

func (planner Planner) ResolveTarget(
	ctx context.Context,
	client domain.DetectedClient,
	scope domain.InstallScope,
	physicalArtifactID string,
) (domain.DeliveryTarget, error) {
	if err := ctx.Err(); err != nil {
		return domain.DeliveryTarget{}, err
	}
	if err := pathpolicy.ValidateLeafID(physicalArtifactID); err != nil {
		return domain.DeliveryTarget{}, fmt.Errorf("invalid physical artifact id: %w", err)
	}
	capabilities, ok := Capabilities(client.ClientID)
	if !ok {
		return domain.DeliveryTarget{}, fmt.Errorf("unsupported client %q", client.ClientID)
	}
	if !supportsScope(capabilities.Scopes, scope) {
		return domain.DeliveryTarget{}, fmt.Errorf("scope %q is unsupported for %s", scope, client.ClientID)
	}
	targetAnchor, targetRoot, err := planner.targetRoot(client, capabilities.PackageMode)
	if err != nil {
		return domain.DeliveryTarget{}, err
	}
	activePath := filepath.Join(targetRoot, physicalArtifactID)
	if err := pathpolicy.RequireContainedChild(targetRoot, activePath); err != nil {
		return domain.DeliveryTarget{}, fmt.Errorf("unsafe client target path: %w", err)
	}
	return domain.DeliveryTarget{TargetAnchor: targetAnchor, TargetRoot: targetRoot, ActivePath: activePath}, nil
}

func Capabilities(clientID domain.ClientID) (domain.ClientCapabilities, bool) {
	allMCP := map[string]domain.SupportLevel{
		"stdio":           domain.SupportNative,
		"streamable-http": domain.SupportNative,
		"sse":             domain.SupportNative,
	}
	switch clientID {
	case domain.ClientCodex:
		return domain.ClientCapabilities{
			ClientID: clientID, PackageMode: domain.PackageProjection, ActivationMode: domain.ActivationByUser,
			Scopes: []domain.InstallScope{domain.ScopeUser}, SkillSupport: domain.SupportProjected,
			MCPTransports: mapSupport(allMCP, domain.SupportProjected), ExtensionSupport: domain.SupportUnsupported,
		}, true
	case domain.ClientCursor:
		return domain.ClientCapabilities{
			ClientID: clientID, PackageMode: domain.PackageNative, ActivationMode: domain.ActivationByUser,
			Scopes: []domain.InstallScope{domain.ScopeUser}, SkillSupport: domain.SupportNative,
			MCPTransports: allMCP, ExtensionSupport: domain.SupportNative,
		}, true
	case domain.ClientCopilot:
		return domain.ClientCapabilities{
			ClientID: clientID, PackageMode: domain.PackageNative, ActivationMode: domain.ActivationByUser,
			Scopes: []domain.InstallScope{domain.ScopeUser}, SkillSupport: domain.SupportNative,
			MCPTransports: allMCP, ExtensionSupport: domain.SupportNative,
		}, true
	case domain.ClientVSCode:
		return domain.ClientCapabilities{
			ClientID: clientID, PackageMode: domain.PackagePrepared, ActivationMode: domain.ActivationByUser,
			Scopes: []domain.InstallScope{domain.ScopeUser}, SkillSupport: domain.SupportPrepared,
			MCPTransports: mapSupport(allMCP, domain.SupportPrepared), ExtensionSupport: domain.SupportPrepared,
		}, true
	case domain.ClientKiro:
		return domain.ClientCapabilities{
			ClientID: clientID, PackageMode: domain.PackageNative, ActivationMode: domain.ActivationByUser,
			Scopes: []domain.InstallScope{domain.ScopeUser}, SkillSupport: domain.SupportNative,
			MCPTransports: allMCP, ExtensionSupport: domain.SupportUnsupported,
		}, true
	default:
		return domain.ClientCapabilities{}, false
	}
}

func (planner Planner) targetRoot(client domain.DetectedClient, mode domain.PackageMode) (string, string, error) {
	var anchor, root string
	if client.ClientID == domain.ClientCursor && mode == domain.PackageNative {
		if strings.TrimSpace(client.ConfigRoot) == "" {
			return "", "", fmt.Errorf("Cursor config root is unavailable")
		}
		anchor = client.ConfigRoot
		root = filepath.Join(client.ConfigRoot, "plugins", "local")
	} else {
		if strings.TrimSpace(planner.ManagedRoot) == "" {
			return "", "", fmt.Errorf("managed client root is required")
		}
		anchor = planner.ManagedRoot
		root = filepath.Join(planner.ManagedRoot, "clients", string(client.ClientID))
	}
	absoluteAnchor, err := filepath.Abs(anchor)
	if err != nil {
		return "", "", err
	}
	absolute, err := filepath.Abs(root)
	if err != nil {
		return "", "", err
	}
	absoluteAnchor, absolute = filepath.Clean(absoluteAnchor), filepath.Clean(absolute)
	if err := pathpolicy.RequireContainedChild(absoluteAnchor, absolute); err != nil {
		return "", "", fmt.Errorf("unsafe client target root: %w", err)
	}
	return absoluteAnchor, absolute, nil
}

func componentDecisions(envelope domain.PackageEnvelope, capabilities domain.ClientCapabilities) []domain.ComponentDecision {
	decisions := make([]domain.ComponentDecision, 0, len(envelope.Skills)+len(envelope.MCP.Servers)+len(envelope.Manifest.Extensions))
	skillNames := sortedKeys(envelope.Skills)
	for _, name := range skillNames {
		decisions = append(decisions, decision(domain.ComponentSkill, name, capabilities.SkillSupport))
	}
	serverNames := sortedKeys(envelope.MCP.Servers)
	for _, name := range serverNames {
		server := envelope.MCP.Servers[name]
		support, ok := capabilities.MCPTransports[server.Type]
		if !ok {
			support = domain.SupportUnsupported
		}
		decisions = append(decisions, decision(domain.ComponentMCPServer, name, support))
	}
	extensionNames := sortedKeys(envelope.Manifest.Extensions)
	for _, name := range extensionNames {
		decisions = append(decisions, decision(domain.ComponentExtension, name, capabilities.ExtensionSupport))
	}
	return decisions
}

func decision(kind domain.ComponentKind, name string, support domain.SupportLevel) domain.ComponentDecision {
	value := domain.ComponentDecision{Kind: kind, Name: name, Support: support}
	if support == domain.SupportUnsupported {
		value.Reason = "component_not_supported_by_client"
	}
	return value
}

func sortedKeys[T any](values map[string]T) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func mapSupport(values map[string]domain.SupportLevel, support domain.SupportLevel) map[string]domain.SupportLevel {
	result := make(map[string]domain.SupportLevel, len(values))
	for key := range values {
		result[key] = support
	}
	return result
}

func supportsScope(scopes []domain.InstallScope, requested domain.InstallScope) bool {
	for _, scope := range scopes {
		if scope == requested {
			return true
		}
	}
	return false
}

func statusFor(mode domain.ActivationMode) domain.PlanStatus {
	switch mode {
	case domain.ActivationAutomatic, domain.ActivationByClient:
		return domain.PlanReady
	case domain.ActivationByUser:
		return domain.PlanManualActivationRequired
	default:
		return domain.PlanUnsupported
	}
}

func activationFor(mode domain.ActivationMode) domain.ActivationState {
	switch mode {
	case domain.ActivationAutomatic, domain.ActivationByClient:
		return domain.ActivationActive
	case domain.ActivationByUser:
		return domain.ActivationManual
	default:
		return domain.ActivationFailed
	}
}

func hasComponents(decisions []domain.ComponentDecision) bool {
	return len(decisions) > 0
}

func hasSupportedComponent(decisions []domain.ComponentDecision) bool {
	for _, item := range decisions {
		if item.Support != domain.SupportUnsupported {
			return true
		}
	}
	return false
}
