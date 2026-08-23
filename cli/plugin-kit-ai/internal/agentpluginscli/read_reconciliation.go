package agentpluginscli

import (
	"context"
	"path/filepath"
	"sort"
	"strings"

	"github.com/777genius/plugin-kit-ai/install/integrationctl/agentplugins/domain"
	"github.com/777genius/plugin-kit-ai/install/integrationctl/agentplugins/providers"
)

func reconcileInstalledInfo(ctx context.Context, app App, installation domain.Installation, targetOption string, public *publicInstallation) error {
	targets, err := parseTargetOption(targetOption)
	if err != nil {
		return err
	}
	selected := make(map[domain.ClientID]bool, len(targets))
	for _, target := range targets {
		selected[target] = true
	}
	clients, err := detectClientsForLifecycleResolution(ctx, app.Detector, true)
	if err != nil {
		return err
	}
	detected := make(map[domain.ClientID]domain.DetectedClient, len(clients))
	for _, client := range clients {
		detected[client.ClientID] = client
	}

	bindings := make(map[string]domain.ClientBinding, len(installation.Clients))
	for _, binding := range installation.Clients {
		bindings[binding.ClientID] = binding
	}
	filtered := make([]publicClient, 0, len(public.Clients))
	for _, view := range public.Clients {
		clientID := domain.ClientID(view.ClientID)
		if len(selected) > 0 && !selected[clientID] {
			continue
		}
		binding := bindings[view.ClientID]
		client, detectedOK := detected[clientID]
		result := reconcileClientIdentity(ctx, app, installation, binding, client, detectedOK)
		view.ReceiptReconciled = boolPointer(result.receiptReconciled)
		view.NativeDiscoveryReconciled = boolPointer(result.nativeDiscoveryReconciled)
		view.NativeIdentityState = result.state
		view.ClientVersion = result.clientVersion
		view.NativeDiscoveryEvidence = result.evidence
		filtered = append(filtered, view)
	}
	sort.Slice(filtered, func(i, j int) bool { return filtered[i].ClientID < filtered[j].ClientID })
	public.Clients = filtered
	return nil
}

type clientIdentityReconciliation struct {
	receiptReconciled         bool
	nativeDiscoveryReconciled bool
	state                     domain.NativeIdentityState
	clientVersion             string
	evidence                  *publicNativeDiscoveryEvidence
}

func reconcileClientIdentity(ctx context.Context, app App, installation domain.Installation, binding domain.ClientBinding, client domain.DetectedClient, detected bool) clientIdentityReconciliation {
	result := clientIdentityReconciliation{state: domain.NativeIdentityIndeterminate}
	if detected {
		result.clientVersion = strings.TrimSpace(client.Version)
	}
	if !detected || client.Status != domain.DetectionDetected || app.Lifecycle.Targets == nil || app.Lifecycle.NativeObserver == nil {
		return result
	}
	expectedArtifact := domain.ComputePhysicalArtifactID(installation.DeclaredName, installation.InstallationID)
	if strings.TrimSpace(binding.PhysicalArtifact) != expectedArtifact {
		return result
	}
	result.evidence = nativeCommandEvidence(client, installation.DeclaredName, expectedArtifact, result.clientVersion, false)
	if !hasReconciledOwnershipReceipt(binding, expectedArtifact, installation.DeclaredName) {
		return result
	}
	target, err := app.Lifecycle.Targets.ResolveTarget(ctx, client, domain.InstallScope(binding.Scope), expectedArtifact)
	if err != nil || filepath.Clean(target.ActivePath) != filepath.Clean(binding.TargetLocator) {
		return result
	}
	plan := domain.DeliveryPlan{
		ClientID: client.ClientID, Scope: domain.InstallScope(binding.Scope), DeclaredName: installation.DeclaredName,
		PhysicalArtifactID: expectedArtifact, TargetAnchor: target.TargetAnchor, TargetRoot: target.TargetRoot, ActivePath: target.ActivePath,
		NativeRegistryRoot: client.ConfigRoot, NativeRegistryExecutable: client.ExecutablePath,
	}
	observation, observeErr := app.Lifecycle.NativeObserver.ObserveNativeIdentity(ctx, client, plan, &binding)
	result.receiptReconciled = observation.ReceiptReconciled
	result.nativeDiscoveryReconciled = observation.NativeDiscoveryReconciled
	if result.evidence != nil {
		result.evidence.DiscoveryOperation.Discovered = result.nativeDiscoveryReconciled
	}
	if observeErr != nil {
		return result
	}
	result.state = observation.NativeDiscoveryState
	if result.state == "" {
		result.state = observation.State
	}
	if result.nativeDiscoveryReconciled {
		result.state = domain.NativeIdentityIndeterminate
		if result.receiptReconciled {
			result.state = domain.NativeIdentityManaged
		}
	}
	return result
}

func hasReconciledOwnershipReceipt(binding domain.ClientBinding, physicalArtifact, declaredName string) bool {
	digest := ""
	expectedObjectID := "package:" + binding.ClientID + ":" + physicalArtifact
	for _, object := range binding.NativeObjects {
		if object.Kind != "managed_package_directory" {
			continue
		}
		if digest != "" || object.ObjectID != expectedObjectID || object.LogicalName != declaredName || strings.TrimSpace(object.ManagedDigest) == "" {
			return false
		}
		digest = object.ManagedDigest
	}
	if digest == "" {
		return false
	}
	latestSequence := 0
	latestDigest := ""
	latestPhase := ""
	sequences := make(map[int]struct{}, len(binding.Receipts))
	for _, receipt := range binding.Receipts {
		if receipt.ClientBindingID != binding.ClientBindingID || receipt.Sequence < 1 || strings.TrimSpace(receipt.OperationID) == "" {
			return false
		}
		if _, duplicate := sequences[receipt.Sequence]; duplicate {
			return false
		}
		sequences[receipt.Sequence] = struct{}{}
		if receipt.Sequence > latestSequence {
			latestSequence = receipt.Sequence
			latestDigest = strings.TrimSpace(receipt.AfterDigest)
			latestPhase = strings.TrimSpace(receipt.Phase)
		}
	}
	return latestSequence > 0 && latestDigest == digest && latestPhase == "committed"
}

func nativeCommandEvidence(client domain.DetectedClient, declaredName, physicalArtifact, version string, discovered bool) *publicNativeDiscoveryEvidence {
	if client.ClientID != domain.ClientCopilot || strings.TrimSpace(client.ExecutablePath) == "" || strings.TrimSpace(physicalArtifact) == "" {
		return nil
	}
	return &publicNativeDiscoveryEvidence{
		Basis: "native_client_command",
		VersionOperation: publicVersionOperation{
			Argv: []string{"copilot", "--version"}, ObservedClientVersion: version,
		},
		DiscoveryOperation: publicDiscoveryOperation{
			Argv: []string{"copilot", "plugin", "list"}, Discovered: discovered,
			ProductID: strings.TrimSpace(declaredName) + "@" + providers.ManagedMarketplaceName(physicalArtifact),
		},
	}
}

func boolPointer(value bool) *bool { return &value }
