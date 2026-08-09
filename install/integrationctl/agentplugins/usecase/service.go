package usecase

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/777genius/plugin-kit-ai/install/integrationctl/adapters/pathpolicy"
	"github.com/777genius/plugin-kit-ai/install/integrationctl/agentplugins/domain"
	"github.com/777genius/plugin-kit-ai/install/integrationctl/agentplugins/ports"
	"github.com/777genius/plugin-kit-ai/install/integrationctl/agentplugins/transaction"
	legacyports "github.com/777genius/plugin-kit-ai/install/integrationctl/ports"
	"golang.org/x/mod/semver"
)

type Service struct {
	StateStore transaction.StateStore
	Planner    ports.DeliveryPlanner
	Targets    ports.DeliveryTargetResolver
	Stager     ports.PackageStager
	Activator  ports.ClientActivator
	Legacy     ports.LegacyLifecycle
	LegacyLock legacyports.LockManager
	Lock       ports.MutationLock
	Kernel     transaction.Kernel
	Now        func() time.Time
}

type AddInput struct {
	Envelope           domain.PackageEnvelope
	Client             domain.DetectedClient
	Scope              domain.InstallScope
	DryRun             bool
	Confirmed          bool
	Interactive        bool
	Hints              domain.CompatibilityHints
	InstallationID     string
	OperationID        string
	BackendExecutable  string
	ActivationComplete bool
	AuthComplete       bool
	// PersistAuthoritativeObservations allows a read-only client verifier to
	// record negative evidence even during a plan-first CLI pass. It does not
	// authorize package, client, or user-requested lifecycle mutations.
	PersistAuthoritativeObservations bool
}

type AddResult struct {
	InstallationID       string                   `json:"installation_id"`
	Plan                 domain.DeliveryPlan      `json:"plan"`
	Activation           domain.ActivationOutcome `json:"activation,omitempty"`
	RequiresConfirmation bool                     `json:"requires_confirmation"`
	Mutated              bool                     `json:"mutated"`
	NoChange             bool                     `json:"no_change,omitempty"`
	Receipt              domain.MutationReceipt   `json:"-"`
}

func (service Service) Add(ctx context.Context, input AddInput) (AddResult, error) {
	return service.apply(ctx, input, false)
}

func (service Service) Update(ctx context.Context, input AddInput) (AddResult, error) {
	return service.apply(ctx, input, true)
}

func (service Service) apply(ctx context.Context, input AddInput, replace bool) (AddResult, error) {
	if service.StateStore == nil || service.Planner == nil || service.Stager == nil || service.Activator == nil {
		return AddResult{}, fmt.Errorf("agentplugins service dependencies are incomplete")
	}
	if input.Envelope.LoaderKind != domain.LoaderKindAgentPlugins {
		return AddResult{}, fmt.Errorf("agentplugins add accepts only standard Agent Plugins packages")
	}
	release, err := service.beginMutation(ctx, input.DryRun, input.Confirmed)
	if err != nil {
		return AddResult{}, err
	}
	if release != nil {
		defer func() { _ = release() }()
	}
	state, err := service.StateStore.Load()
	if err != nil {
		return AddResult{}, err
	}
	sourceBindingID := domain.ComputeSourceBindingID(input.Envelope.Source)
	installationIndex, existing := findSourceInstallation(state, sourceBindingID)
	installationID := strings.TrimSpace(input.InstallationID)
	if existing {
		installation := state.Installations[installationIndex]
		if installation.NeedsRebind {
			return AddResult{}, fmt.Errorf("installation %s requires explicit rebind", installation.InstallationID)
		}
		if input.Envelope.Manifest.Name != installation.DeclaredName {
			return AddResult{}, fmt.Errorf("refuse package identity change from %q to %q; remove all targets and use explicit rebind", installation.DeclaredName, input.Envelope.Manifest.Name)
		}
		if installation.Package.LoaderKind != input.Envelope.LoaderKind || installation.Package.FormatID != input.Envelope.FormatID {
			return AddResult{}, fmt.Errorf("source is bound to a different package format; use migrate-format")
		}
		installationID = installation.InstallationID
	}
	if installationID == "" {
		installationID, err = domain.NewInstallationID()
		if err != nil {
			return AddResult{}, err
		}
	}
	if replace && !existing {
		return AddResult{}, fmt.Errorf("update source is not bound to an existing installation; use add or rebind")
	}
	physicalID := domain.ComputePhysicalArtifactID(input.Envelope.Manifest.Name, installationID)
	plan, err := service.Planner.Plan(ctx, input.Envelope, input.Client, input.Scope, physicalID)
	if err != nil {
		return AddResult{}, err
	}
	if openAIOAuthApplies(input.Client.ClientID, input.Envelope, input.Hints) {
		plan.Authentication = domain.AuthenticationPending
	}
	result := AddResult{InstallationID: installationID, Plan: plan}
	if plan.Status == domain.PlanUnsupported {
		return result, fmt.Errorf("delivery plan for %s is unsupported", plan.ClientID)
	}
	if err := rejectNativeNameCollision(state, installationID, input.Envelope.Manifest.Name, input.Client.ClientID); err != nil {
		return result, err
	}
	if existing {
		if err := rejectSharedCopilotBackendDuplicate(state.Installations[installationIndex], input.Client.ClientID); err != nil {
			return result, err
		}
	}
	clientBindingID := domain.ComputeClientBindingID(installationID, string(input.Client.ClientID), string(input.Scope), plan.ActivePath)
	if existing {
		if err := validatePackageTransition(state.Installations[installationIndex], input.Envelope); err != nil {
			return result, err
		}
	}
	isMaterialized := existing && materializedClient(state.Installations[installationIndex], clientBindingID)
	if !isMaterialized && (input.ActivationComplete || input.AuthComplete) {
		return result, fmt.Errorf("lifecycle completion flags require an already materialized package; run add first, then rerun add with the completion flags")
	}
	if isMaterialized && !replace {
		current := state.Installations[installationIndex].Clients[clientBindingID]
		result.Activation = lifecycleOutcome(current)
		if !packageRevisionMatches(current.PackageRevision, input.Envelope) {
			return result, fmt.Errorf("plugin is already materialized for %s at a different revision; use update", input.Client.ClientID)
		}
		if lifecycleConverged(current) {
			if err := service.verifyManagedTarget(ctx, input.Client, input.Scope, current, "no-change check"); err != nil {
				return result, err
			}
			verified, verifyErr := service.verifyClientReadOnly(ctx, input, result, current)
			if verifyErr != nil {
				if verified.Activation != "" && !input.DryRun && (input.Confirmed || input.PersistAuthoritativeObservations && verified.AuthoritativeObservation) {
					result.Activation = verified
					changed, updateErr := service.persistAuthoritativeObservation(ctx, input, installationID, clientBindingID, current, verified)
					result.Mutated = changed
					if updateErr != nil {
						return result, fmt.Errorf("client verification failed: %v; persist negative verification evidence: %w", verifyErr, updateErr)
					}
				}
				return result, verifyErr
			}
			if verified.Activation != "" && !sameLifecycleOutcome(verified, lifecycleOutcome(current)) {
				return service.persistObservedLifecycle(input, result, installationID, clientBindingID, verified)
			}
			result.NoChange = true
			return result, nil
		}
		return service.resume(ctx, input, result, installationID, clientBindingID, current)
	}
	if replace && !isMaterialized {
		return result, fmt.Errorf("plugin is not materialized for %s; use add", input.Client.ClientID)
	}
	if replace {
		current := state.Installations[installationIndex]
		previousClient := current.Clients[clientBindingID]
		result.Activation = lifecycleOutcome(previousClient)
		if err := service.verifyManagedTarget(ctx, input.Client, input.Scope, previousClient, "update"); err != nil {
			return result, err
		}
		if packageRevisionMatches(previousClient.PackageRevision, input.Envelope) {
			if lifecycleConverged(previousClient) {
				verified, verifyErr := service.verifyClientReadOnly(ctx, input, result, previousClient)
				if verifyErr != nil {
					if verified.Activation != "" && !input.DryRun && (input.Confirmed || input.PersistAuthoritativeObservations && verified.AuthoritativeObservation) {
						result.Activation = verified
						changed, updateErr := service.persistAuthoritativeObservation(ctx, input, installationID, clientBindingID, previousClient, verified)
						result.Mutated = changed
						if updateErr != nil {
							return result, fmt.Errorf("client verification failed: %v; persist negative verification evidence: %w", verifyErr, updateErr)
						}
					}
					return result, verifyErr
				}
				if verified.Activation != "" && !sameLifecycleOutcome(verified, lifecycleOutcome(previousClient)) {
					return service.persistObservedLifecycle(input, result, installationID, clientBindingID, verified)
				}
				result.NoChange = true
				return result, nil
			}
			return service.resume(ctx, input, result, installationID, clientBindingID, previousClient)
		}
	}
	if input.DryRun {
		return result, nil
	}
	if !input.Confirmed {
		result.RequiresConfirmation = true
		return result, nil
	}
	operationID := strings.TrimSpace(input.OperationID)
	if operationID == "" {
		operationID, err = newOperationID()
		if err != nil {
			return result, err
		}
	}
	delivery, err := service.Stager.Stage(ctx, input.Envelope, plan, operationID, input.Hints)
	if err != nil {
		return result, err
	}
	previousClient := domain.ClientBinding{}
	if existing {
		previousClient = state.Installations[installationIndex].Clients[clientBindingID]
	}
	state, installationIndex = upsertPreparedInstallation(state, installationIndex, existing, input, plan, installationID, clientBindingID, sourceBindingID, service.now())
	kernel := service.Kernel
	kernel.StateStore = service.StateStore
	initialActivation, initialVerification := initialLifecycle(plan)
	receipt, applyErr := kernel.ApplyDirectory(ctx, transaction.DirectoryMutation{
		OperationID:     operationID,
		InstallationID:  installationID,
		ClientBindingID: clientBindingID,
		Sequence:        nextSequence(state.Installations[installationIndex].Clients[clientBindingID]),
		OwnedBase:       delivery.OwnedBase,
		ActivePath:      delivery.ActivePath,
		StagingPath:     delivery.StagingPath,
		BeforeDigest:    managedDigest(previousClient),
		AfterDigest:     delivery.ArtifactDigest,
		NativeObjects:   delivery.NativeObjects,
		Activation:      initialActivation,
		Authentication:  plan.Authentication,
		Policy:          domain.PolicyAllowed,
		Verification:    initialVerification,
		DesiredState:    state,
		Verify: func(verifyContext context.Context, activePath string) error {
			return service.Stager.Verify(verifyContext, activePath, delivery.ArtifactDigest)
		},
	})
	result.Receipt = receipt
	if applyErr != nil {
		_ = service.Stager.Discard(context.Background(), delivery)
		return result, applyErr
	}
	result.Mutated = true
	outcome, activationErr := service.Activator.Activate(ctx, domain.ActivationRequest{
		Client: input.Client, Plan: plan, Delivery: domain.StagedDelivery{
			ClientID: delivery.ClientID, OwnedBase: delivery.OwnedBase, ActivePath: delivery.ActivePath,
			ArtifactDigest: delivery.ArtifactDigest, NativeObjects: delivery.NativeObjects,
		},
		DeclaredName: input.Envelope.Manifest.Name, Replacing: replace,
		Interactive: input.Interactive, BackendExecutable: input.BackendExecutable,
	})
	result.Activation = outcome
	if activationErr != nil && outcome.Activation == "" {
		outcome = domain.ActivationOutcome{
			Activation: domain.ActivationFailed, Authentication: plan.Authentication,
			Policy: domain.PolicyAllowed, Verification: domain.VerificationFailed,
		}
		result.Activation = outcome
	}
	if _, updateErr := service.updateLifecycle(installationID, clientBindingID, outcome); updateErr != nil {
		if activationErr != nil {
			return result, fmt.Errorf("activate client: %v; persist activation state: %w", activationErr, updateErr)
		}
		return result, updateErr
	}
	if activationErr != nil {
		return result, activationErr
	}
	return result, nil
}

func lifecycleConverged(client domain.ClientBinding) bool {
	authComplete := client.Authentication == domain.AuthenticationNotRequired || client.Authentication == domain.AuthenticationComplete
	return client.Materialization == domain.MaterializationMaterialized &&
		client.Activation == domain.ActivationActive &&
		client.Verification == domain.VerificationInstalled && authComplete
}

// resume retries only the external client lifecycle against an existing,
// digest-verified managed package. It deliberately creates no directory
// transaction or receipt, so repeating add cannot replace or duplicate the
// materialized artifact.
func (service Service) resume(
	ctx context.Context,
	input AddInput,
	result AddResult,
	installationID, clientBindingID string,
	client domain.ClientBinding,
) (AddResult, error) {
	if input.DryRun {
		return result, nil
	}
	if err := service.verifyManagedTarget(ctx, input.Client, input.Scope, client, "resume"); err != nil {
		return result, err
	}
	result.Activation = lifecycleOutcome(client)
	if !input.Confirmed {
		result.RequiresConfirmation = true
		return result, nil
	}
	delivery := domain.StagedDelivery{
		ClientID: input.Client.ClientID, OwnedBase: result.Plan.TargetRoot,
		ActivePath: client.TargetLocator, ArtifactDigest: managedDigest(client),
		NativeObjects: append([]domain.NativeObjectOwnership(nil), client.NativeObjects...),
	}
	outcome, activationErr := service.Activator.Activate(ctx, domain.ActivationRequest{
		Client: input.Client, Plan: result.Plan, Delivery: delivery,
		DeclaredName: input.Envelope.Manifest.Name, Replacing: true,
		Interactive: input.Interactive, BackendExecutable: input.BackendExecutable,
		VerifyOnly: true, ActivationComplete: input.ActivationComplete,
	})
	// Authentication completion is a separate phase. Client installation/list
	// evidence must never silently complete it.
	if activationErr == nil && !clientVerifierAvailable(input, result.Plan) && client.Activation == domain.ActivationActive && client.Verification == domain.VerificationInstalled {
		outcome.Activation = client.Activation
		outcome.Verification = client.Verification
	}
	if (client.Authentication == domain.AuthenticationPending || client.Authentication == domain.AuthenticationNotChecked) && input.AuthComplete {
		outcome.Authentication = domain.AuthenticationComplete
		outcome.AuthenticationAttested = true
	} else if client.Authentication != "" {
		outcome.Authentication = client.Authentication
	}
	result.Activation = outcome
	if activationErr != nil && outcome.Activation == "" {
		outcome = domain.ActivationOutcome{
			Activation: domain.ActivationFailed, Authentication: result.Plan.Authentication,
			Policy: domain.PolicyAllowed, Verification: domain.VerificationFailed,
		}
		result.Activation = outcome
	}
	changed, updateErr := service.updateLifecycle(installationID, clientBindingID, outcome)
	result.Mutated = changed
	if updateErr != nil {
		if activationErr != nil {
			return result, fmt.Errorf("resume client activation: %v; persist activation state: %w", activationErr, updateErr)
		}
		return result, updateErr
	}
	if activationErr != nil {
		return result, activationErr
	}
	return result, nil
}

func clientVerifierAvailable(input AddInput, plan domain.DeliveryPlan) bool {
	if strings.TrimSpace(input.BackendExecutable) == "" {
		return false
	}
	switch input.Client.ClientID {
	case domain.ClientCodex, domain.ClientCopilot, domain.ClientVSCode:
		return true
	case domain.ClientKiro:
		if !strings.Contains(strings.ToLower(input.BackendExecutable), "kiro") || len(plan.Components) == 0 {
			return false
		}
		for _, component := range plan.Components {
			if component.Support == domain.SupportUnsupported || component.Kind != domain.ComponentMCPServer {
				return false
			}
		}
		return true
	default:
		return false
	}
}

func (service Service) persistObservedLifecycle(input AddInput, result AddResult, installationID, clientBindingID string, outcome domain.ActivationOutcome) (AddResult, error) {
	result.Activation = outcome
	if input.DryRun {
		return result, nil
	}
	if !input.Confirmed {
		result.RequiresConfirmation = true
		return result, nil
	}
	changed, err := service.updateLifecycle(installationID, clientBindingID, outcome)
	result.Mutated = changed
	return result, err
}

func (service Service) persistAuthoritativeObservation(ctx context.Context, input AddInput, installationID, clientBindingID string, observed domain.ClientBinding, outcome domain.ActivationOutcome) (bool, error) {
	if input.Confirmed {
		return service.updateLifecycle(installationID, clientBindingID, outcome)
	}
	release, err := service.beginMutation(ctx, false, true)
	if err != nil {
		return false, err
	}
	defer func() { _ = release() }()
	state, err := service.StateStore.Load()
	if err != nil {
		return false, err
	}
	for _, installation := range state.Installations {
		if installation.InstallationID != installationID {
			continue
		}
		latest, ok := installation.Clients[clientBindingID]
		if !ok || !reflect.DeepEqual(latest, observed) {
			return false, fmt.Errorf("stale client binding observation; state changed before negative evidence could be persisted, retry the command")
		}
		return service.updateLifecycle(installationID, clientBindingID, outcome)
	}
	return false, fmt.Errorf("stale client binding observation; installation changed before negative evidence could be persisted, retry the command")
}

func openAIOAuthApplies(clientID domain.ClientID, envelope domain.PackageEnvelope, hints domain.CompatibilityHints) bool {
	if clientID != domain.ClientCodex {
		return false
	}
	if envelope.CatalogEvidence != nil {
		if _, present := envelope.CatalogEvidence.Compatibility[string(clientID)]; present {
			return false
		}
	}
	if _, present := hints.Compatibility[string(clientID)]; present {
		return false
	}
	for serverName := range envelope.MCP.Servers {
		if strings.TrimSpace(hints.OpenAIMCPAuth[serverName].OAuthResource) != "" {
			return true
		}
	}
	return false
}

func (service Service) beginMutation(ctx context.Context, dryRun, confirmed bool) (ports.UnlockFunc, error) {
	if dryRun || !confirmed {
		return nil, nil
	}
	if service.Lock == nil {
		return nil, fmt.Errorf("agentplugins mutation lock is required")
	}
	release, err := service.Lock.Acquire(ctx)
	if err != nil {
		return nil, err
	}
	kernel := service.Kernel
	kernel.StateStore = service.StateStore
	if err := kernel.Recover(ctx); err != nil {
		_ = release()
		return nil, fmt.Errorf("recover interrupted mutation: %w", err)
	}
	return release, nil
}

func (service Service) updateLifecycle(installationID, clientBindingID string, outcome domain.ActivationOutcome) (bool, error) {
	state, err := service.StateStore.Load()
	if err != nil {
		return false, err
	}
	for installationIndex, installation := range state.Installations {
		if installation.InstallationID != installationID {
			continue
		}
		client, ok := installation.Clients[clientBindingID]
		if !ok {
			return false, fmt.Errorf("client binding disappeared during activation")
		}
		if client.Activation == outcome.Activation && client.Authentication == outcome.Authentication &&
			client.Policy == outcome.Policy && client.Verification == outcome.Verification {
			return false, nil
		}
		client.Activation = outcome.Activation
		client.Authentication = outcome.Authentication
		client.Policy = outcome.Policy
		client.Verification = outcome.Verification
		client.UpdatedAt = service.now().Format(time.RFC3339Nano)
		installation.Clients[clientBindingID] = client
		installation.UpdatedAt = client.UpdatedAt
		state.Installations[installationIndex] = installation
		if err := service.StateStore.Save(state); err != nil {
			return false, err
		}
		return true, nil
	}
	return false, fmt.Errorf("installation disappeared during activation")
}

func lifecycleOutcome(client domain.ClientBinding) domain.ActivationOutcome {
	return domain.ActivationOutcome{Activation: client.Activation, Authentication: client.Authentication, Policy: client.Policy, Verification: client.Verification}
}

func (service Service) verifyClientReadOnly(ctx context.Context, input AddInput, result AddResult, client domain.ClientBinding) (domain.ActivationOutcome, error) {
	if strings.TrimSpace(input.BackendExecutable) == "" {
		return domain.ActivationOutcome{}, nil
	}
	switch input.Client.ClientID {
	case domain.ClientCodex, domain.ClientCopilot, domain.ClientVSCode:
	case domain.ClientKiro:
		if !strings.Contains(strings.ToLower(input.BackendExecutable), "kiro") {
			return domain.ActivationOutcome{}, nil
		}
	default:
		return domain.ActivationOutcome{}, nil
	}
	delivery := domain.StagedDelivery{ClientID: input.Client.ClientID, OwnedBase: result.Plan.TargetRoot, ActivePath: client.TargetLocator, ArtifactDigest: managedDigest(client), NativeObjects: client.NativeObjects}
	outcome, err := service.Activator.Activate(ctx, domain.ActivationRequest{Client: input.Client, Plan: result.Plan, Delivery: delivery, DeclaredName: input.Envelope.Manifest.Name, Replacing: true, BackendExecutable: input.BackendExecutable, VerifyOnly: true})
	if outcome.Authentication == "" || outcome.Authentication == domain.AuthenticationNotChecked {
		outcome.Authentication = client.Authentication
	}
	return outcome, err
}

func (service Service) now() time.Time {
	if service.Now != nil {
		return service.Now().UTC()
	}
	return time.Now().UTC()
}

func upsertPreparedInstallation(
	state domain.StateFileV2,
	installationIndex int,
	existing bool,
	input AddInput,
	plan domain.DeliveryPlan,
	installationID, clientBindingID, sourceBindingID string,
	now time.Time,
) (domain.StateFileV2, int) {
	timestamp := now.Format(time.RFC3339Nano)
	installation := domain.Installation{
		InstallationID: installationID,
		DeclaredName:   input.Envelope.Manifest.Name,
		Source: domain.SourceBinding{
			SourceBindingID:  sourceBindingID,
			RequestedSource:  input.Envelope.Source.RequestedSource,
			CanonicalSource:  input.Envelope.Source.CanonicalSource,
			Repository:       input.Envelope.Source.Repository,
			PackageSubpath:   input.Envelope.Source.PackageSubpath,
			ResolvedRevision: input.Envelope.Source.ResolvedRevision,
			TreeDigest:       input.Envelope.TreeDigest,
		},
		Package: domain.PackageBinding{
			LoaderKind: input.Envelope.LoaderKind, FormatID: input.Envelope.FormatID,
			SchemaURI: input.Envelope.SchemaURI, DeclaredName: input.Envelope.Manifest.Name,
			Version: input.Envelope.Manifest.Version, ManifestDigest: input.Envelope.ManifestDigest,
			Inventory: input.Envelope.Inventory,
		},
		Clients:   map[string]domain.ClientBinding{},
		CreatedAt: timestamp,
		UpdatedAt: timestamp,
	}
	if existing {
		installation = state.Installations[installationIndex]
		installation.DeclaredName = input.Envelope.Manifest.Name
		installation.Source.ResolvedRevision = input.Envelope.Source.ResolvedRevision
		installation.Source.TreeDigest = input.Envelope.TreeDigest
		installation.Package = domain.PackageBinding{
			LoaderKind: input.Envelope.LoaderKind, FormatID: input.Envelope.FormatID,
			SchemaURI: input.Envelope.SchemaURI, DeclaredName: input.Envelope.Manifest.Name,
			Version: input.Envelope.Manifest.Version, ManifestDigest: input.Envelope.ManifestDigest,
			Inventory: input.Envelope.Inventory,
		}
		installation.UpdatedAt = timestamp
		if installation.Clients == nil {
			installation.Clients = map[string]domain.ClientBinding{}
		}
	}
	previousClient := installation.Clients[clientBindingID]
	installation.Clients[clientBindingID] = domain.ClientBinding{
		ClientBindingID: clientBindingID, ClientID: string(input.Client.ClientID), Scope: string(input.Scope),
		TargetLocator: plan.ActivePath, PhysicalArtifact: plan.PhysicalArtifactID,
		Materialization: domain.MaterializationStaged, Activation: domain.ActivationPrepared,
		Authentication: plan.Authentication, Policy: domain.PolicyAllowed,
		Verification: domain.VerificationPackageValid, UpdatedAt: timestamp,
		PackageRevision: packageRevisionFromEnvelope(input.Envelope),
		Receipts:        append([]domain.MutationReceipt(nil), previousClient.Receipts...),
		NativeObjects:   append([]domain.NativeObjectOwnership(nil), previousClient.NativeObjects...),
	}
	if existing {
		state.Installations[installationIndex] = installation
		return state, installationIndex
	}
	state.Installations = append(state.Installations, installation)
	return state, len(state.Installations) - 1
}

func validatePackageTransition(installation domain.Installation, envelope domain.PackageEnvelope) error {
	incomingVersion := envelope.Manifest.Version
	currentVersion := installation.Package.Version
	if incomingVersion != currentVersion {
		comparison, comparable := versionCompare(incomingVersion, currentVersion)
		if !comparable {
			return fmt.Errorf("refuse incomparable package version transition from %q to %q; use an explicit reviewed migration or rebind flow", currentVersion, incomingVersion)
		}
		if comparison < 0 {
			return fmt.Errorf("refuse package downgrade from %s to %s", currentVersion, incomingVersion)
		}
	}
	if incomingVersion != "" && currentVersion == incomingVersion && installation.Source.TreeDigest != "" && installation.Source.TreeDigest != envelope.TreeDigest {
		return fmt.Errorf("supply-chain conflict: version %s now has a different package digest", incomingVersion)
	}
	for _, client := range installation.Clients {
		revision := client.PackageRevision
		if revision == nil || revision.Version == "" || revision.Version != incomingVersion {
			continue
		}
		if revision.TreeDigest != "" && revision.TreeDigest != envelope.TreeDigest {
			return fmt.Errorf("supply-chain conflict: version %s differs from the revision applied to %s", incomingVersion, client.ClientID)
		}
	}
	return nil
}

func packageRevisionFromEnvelope(envelope domain.PackageEnvelope) *domain.ClientPackageRevision {
	return &domain.ClientPackageRevision{
		Version: envelope.Manifest.Version, ResolvedRevision: envelope.Source.ResolvedRevision,
		TreeDigest: envelope.TreeDigest, ManifestDigest: envelope.ManifestDigest,
	}
}

func packageRevisionMatches(revision *domain.ClientPackageRevision, envelope domain.PackageEnvelope) bool {
	return revision != nil && revision.TreeDigest == envelope.TreeDigest && revision.ManifestDigest == envelope.ManifestDigest
}

func findSourceInstallation(state domain.StateFileV2, sourceBindingID string) (int, bool) {
	for index, installation := range state.Installations {
		if installation.Source.SourceBindingID == sourceBindingID {
			return index, true
		}
	}
	return -1, false
}

func materializedClient(installation domain.Installation, clientBindingID string) bool {
	client, ok := installation.Clients[clientBindingID]
	return ok && client.Materialization != domain.MaterializationAbsent
}

func rejectNativeNameCollision(state domain.StateFileV2, installationID, declaredName string, clientID domain.ClientID) error {
	for _, installation := range state.Installations {
		if installation.InstallationID == installationID || installation.DeclaredName != declaredName {
			continue
		}
		for _, client := range installation.Clients {
			boundClientID := domain.ClientID(client.ClientID)
			if sameNativeBackend(boundClientID, clientID) && client.Materialization != domain.MaterializationAbsent {
				return fmt.Errorf("native client name collision for %q on %s; remove or rebind the existing source first", declaredName, clientID)
			}
		}
	}
	return nil
}

func rejectSharedCopilotBackendDuplicate(installation domain.Installation, requested domain.ClientID) error {
	if !sameNativeBackend(requested, domain.ClientCopilot) {
		return nil
	}
	for _, client := range installation.Clients {
		bound := domain.ClientID(client.ClientID)
		if bound != requested && sameNativeBackend(bound, requested) && client.Materialization != domain.MaterializationAbsent {
			return fmt.Errorf("plugin is already installed through %s and is available in both GitHub Copilot CLI and VS Code", bound)
		}
	}
	return nil
}

func sameNativeBackend(first, second domain.ClientID) bool {
	if first == second {
		return true
	}
	return (first == domain.ClientCopilot || first == domain.ClientVSCode) &&
		(second == domain.ClientCopilot || second == domain.ClientVSCode)
}

func initialLifecycle(plan domain.DeliveryPlan) (domain.ActivationState, domain.VerificationState) {
	return plan.Activation, plan.Verification
}

func nextSequence(client domain.ClientBinding) int {
	maximum := 0
	for _, receipt := range client.Receipts {
		if receipt.Sequence > maximum {
			maximum = receipt.Sequence
		}
	}
	return maximum + 1
}

func managedDigest(client domain.ClientBinding) string {
	for _, object := range client.NativeObjects {
		if object.Kind == "managed_package_directory" && object.ManagedDigest != "" {
			return object.ManagedDigest
		}
	}
	return ""
}

func newOperationID() (string, error) {
	var random [8]byte
	if _, err := rand.Read(random[:]); err != nil {
		return "", fmt.Errorf("generate operation id: %w", err)
	}
	return "op-" + hex.EncodeToString(random[:]), nil
}

func versionCompare(left, right string) (int, bool) {
	normalize := func(value string) string {
		value = strings.TrimSpace(value)
		if value != "" && !strings.HasPrefix(value, "v") {
			value = "v" + value
		}
		return value
	}
	left, right = normalize(left), normalize(right)
	if !semver.IsValid(left) || !semver.IsValid(right) {
		return 0, false
	}
	return semver.Compare(left, right), true
}

func (service Service) verifyManagedTarget(
	ctx context.Context,
	client domain.DetectedClient,
	scope domain.InstallScope,
	binding domain.ClientBinding,
	operation string,
) error {
	if service.Targets == nil {
		return fmt.Errorf("delivery target resolver is required for %s", operation)
	}
	expectedDigest := managedDigest(binding)
	if expectedDigest == "" {
		return fmt.Errorf("managed package digest is missing; refusing %s and retaining state for reviewed recovery", operation)
	}
	target, err := service.Targets.ResolveTarget(ctx, client, scope, binding.PhysicalArtifact)
	if err != nil {
		return fmt.Errorf("resolve managed %s target: %w", operation, err)
	}
	if err := pathpolicy.RequireExactPath(target.ActivePath, binding.TargetLocator); err != nil {
		return fmt.Errorf("refuse %s from untrusted persisted target: %w", operation, err)
	}
	if err := service.Stager.Verify(ctx, binding.TargetLocator, expectedDigest); err != nil {
		return fmt.Errorf("managed package was changed or is missing; refusing silent %s and retaining state: %w", operation, err)
	}
	return nil
}
