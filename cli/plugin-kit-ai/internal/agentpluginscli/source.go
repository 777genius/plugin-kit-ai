package agentpluginscli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"

	"github.com/777genius/plugin-kit-ai/install/integrationctl/agentplugins/adapters/catalog"
	"github.com/777genius/plugin-kit-ai/install/integrationctl/agentplugins/adapters/directoryv1"
	"github.com/777genius/plugin-kit-ai/install/integrationctl/agentplugins/adapters/packagedigest"
	"github.com/777genius/plugin-kit-ai/install/integrationctl/agentplugins/domain"
	clientplanner "github.com/777genius/plugin-kit-ai/install/integrationctl/agentplugins/planner"
)

var (
	shortNamePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9.-]{0,63}$`)
	exactGitPattern  = regexp.MustCompile(`^(?:github:)?([A-Za-z0-9][A-Za-z0-9-]*/[A-Za-z0-9][A-Za-z0-9._-]*)@([0-9a-f]{40})//(.+)$`)
)

type loadedPackage struct {
	envelope              domain.PackageEnvelope
	hints                 domain.CompatibilityHints
	origin                domain.OriginMode
	directory             *domain.DirectoryOrigin
	distributionSuspended bool
	releaseRevoked        bool
	directorySelection    *domain.DirectorySelection
	cleanup               func() error
}

type packageResolutionRequest struct {
	Targets           []domain.ClientID
	Operation         domain.DirectoryOperation
	Recorded          *domain.RecordedDirectoryRelease
	Clients           map[domain.ClientID]domain.DetectedClient
	RetainRecorded    bool
	Selector          string
	RequestedSelector string
}

func (app App) addResolutionRequest(selector string, targets []domain.ClientID) packageResolutionRequest {
	return packageResolutionRequest{Targets: append([]domain.ClientID(nil), targets...), Operation: domain.DirectoryInstall, RetainRecorded: true}
}

func (app App) loadInstalledPackage(ctx context.Context, installation domain.Installation, targets []domain.ClientID, operation domain.DirectoryOperation, releaseSequence uint64, clients map[domain.ClientID]domain.DetectedClient) (loadedPackage, error) {
	if installation.OriginMode != domain.OriginModeDirectory || installation.Directory == nil {
		return app.loadPackageFor(ctx, updateSource(installation), packageResolutionRequest{Targets: targets, Operation: operation, Clients: clients})
	}
	sequence := releaseSequence
	if sequence == 0 {
		sequence = installation.Directory.DesiredReleaseSequence
	}
	return app.loadPackageFor(ctx, installation.Directory.ProductID, packageResolutionRequest{Targets: targets, Operation: operation, Clients: clients,
		RequestedSelector: strings.TrimSpace(installation.Source.RequestedSource), Recorded: &domain.RecordedDirectoryRelease{
			ProductID: installation.Directory.ProductID, DistributionID: installation.Directory.DistributionID, ReleaseSequence: sequence,
		}})
}

func withDetectedClients(request packageResolutionRequest, clients map[domain.ClientID]domain.DetectedClient) packageResolutionRequest {
	request.Clients = clients
	return request
}

func locallyMatchedInstallation(state domain.StateFileV2, selector string) (domain.Installation, bool) {
	selector = strings.TrimSpace(selector)
	var matches []domain.Installation
	for _, installation := range state.Installations {
		matched := installationMatchesSelector(installation, selector)
		if matched {
			matches = append(matches, installation)
		}
	}
	if len(matches) != 1 {
		return domain.Installation{}, false
	}
	return matches[0], true
}

// retainDirectoryRelease resolves mutable Directory selectors only far enough
// to establish product identity, then binds a re-add to the installation's
// recorded immutable release. Selecting today's default first would let an
// alias rotation change the distribution before local state was considered.
func retainDirectoryRelease(snapshot domain.DirectorySnapshot, state domain.StateFileV2, selector string, request packageResolutionRequest) (packageResolutionRequest, error) {
	if request.Recorded != nil || !request.RetainRecorded {
		return request, nil
	}
	productID, selectorErr := directorySelectorProductID(snapshot, selector)
	if selectorErr != nil && !errors.Is(selectorErr, domain.ErrDirectoryNotFound) {
		return request, selectorErr
	}
	installation, found, err := retainedDirectoryInstallation(state, selector, productID)
	if err != nil || !found {
		if selectorErr != nil {
			return request, selectorErr
		}
		return request, err
	}
	request.Operation = domain.DirectoryNewTarget
	request.Recorded = &domain.RecordedDirectoryRelease{
		ProductID:       installation.Directory.ProductID,
		DistributionID:  installation.Directory.DistributionID,
		ReleaseSequence: installation.Directory.DesiredReleaseSequence,
	}
	// Resolve the recorded qualified distribution, not a mutable or retired
	// alias. The recorded product identity is checked above before this rewrite.
	request.Selector = installation.Directory.DistributionID
	return request, nil
}

func directorySelectorProductID(snapshot domain.DirectorySnapshot, rawSelector string) (string, error) {
	selector := strings.TrimSpace(rawSelector)
	if strings.Contains(selector, "/") {
		matches := []string{}
		for _, distribution := range snapshot.Distributions {
			if distribution.ID != selector {
				continue
			}
			for _, product := range snapshot.Products {
				if product.ID == distribution.ProductID && containsDirectoryValue(product.Distributions, selector) {
					matches = append(matches, product.ID)
				}
			}
		}
		if len(matches) == 0 {
			return "", fmt.Errorf("%w: distribution %q", domain.ErrDirectoryNotFound, selector)
		}
		if len(matches) != 1 {
			return "", fmt.Errorf("%w: qualified distribution %q", domain.ErrDirectoryAmbiguous, selector)
		}
		return matches[0], nil
	}
	matches := []string{}
	for _, product := range snapshot.Products {
		if product.ID == selector || product.ManifestName == selector || containsDirectoryValue(product.Aliases, selector) {
			matches = append(matches, product.ID)
		}
	}
	if len(matches) == 0 {
		return "", fmt.Errorf("%w: %q", domain.ErrDirectoryNotFound, selector)
	}
	if len(matches) != 1 {
		return "", fmt.Errorf("%w: %q", domain.ErrDirectoryAmbiguous, selector)
	}
	return matches[0], nil
}

func containsDirectoryValue(values []string, value string) bool {
	for _, candidate := range values {
		if candidate == value {
			return true
		}
	}
	return false
}

func retainedDirectoryInstallation(state domain.StateFileV2, selector, productID string) (domain.Installation, bool, error) {
	matches := []domain.Installation{}
	for _, installation := range state.Installations {
		directoryMatch := installation.Directory != nil && ((productID != "" && installation.Directory.ProductID == productID) || installation.Directory.DistributionID == selector)
		historicalSelectorMatch := strings.TrimSpace(installation.Source.RequestedSource) == selector
		declaredMatch := installation.InstallationID == selector || installation.DeclaredName == selector
		if !directoryMatch && !historicalSelectorMatch && !declaredMatch {
			continue
		}
		if installation.OriginMode != domain.OriginModeDirectory || installation.Directory == nil ||
			strings.TrimSpace(installation.Directory.ProductID) == "" || strings.TrimSpace(installation.Directory.DistributionID) == "" || installation.Directory.DesiredReleaseSequence < 1 {
			return domain.Installation{}, false, fmt.Errorf("retained installation %q has incomplete or corrupt Directory release identity", installation.InstallationID)
		}
		if productID != "" && installation.Directory.ProductID != productID {
			return domain.Installation{}, false, fmt.Errorf("Directory selector %q now resolves to product %q, but retained installation %q records product %q", selector, productID, installation.InstallationID, installation.Directory.ProductID)
		}
		matches = append(matches, installation)
	}
	if len(matches) == 0 {
		return domain.Installation{}, false, nil
	}
	if len(matches) != 1 {
		return domain.Installation{}, false, fmt.Errorf("retained Directory state for product %q is ambiguous; use installation_id", productID)
	}
	return matches[0], true, nil
}

func installationMatchesSelector(installation domain.Installation, selector string) bool {
	if installation.InstallationID == selector || installation.DeclaredName == selector ||
		strings.TrimSpace(installation.Source.RequestedSource) == selector {
		return true
	}
	if installation.Directory == nil {
		return false
	}
	return installation.Directory.ProductID == selector || installation.Directory.DistributionID == selector
}

func (app App) loadPackage(ctx context.Context, raw string) (loadedPackage, error) {
	return app.loadPackageFor(ctx, raw, packageResolutionRequest{Operation: domain.DirectoryInstall})
}

func (app App) loadPackageFor(ctx context.Context, raw string, request packageResolutionRequest) (loadedPackage, error) {
	requested := strings.TrimSpace(raw)
	if requested == "" {
		return loadedPackage{}, fmt.Errorf("plugin name or source is required")
	}
	if explicitLocalPath(requested) {
		return app.acquireLocal(ctx, requested)
	}
	if match := exactGitPattern.FindStringSubmatch(requested); match != nil {
		return app.acquireGitHub(ctx, requested, match[1], match[2], match[3], "")
	}
	if !isDirectorySelector(requested) {
		return loadedPackage{}, fmt.Errorf("source must be a local path, an exact owner/repo@FULL_SHA//path source, or a Directory selector")
	}
	return app.acquireDirectory(ctx, requested, request)
}

func (app App) acquireLocal(ctx context.Context, requested string) (loadedPackage, error) {
	if app.SourceAcquirer == nil {
		return loadedPackage{}, fmt.Errorf("package source acquirer is unavailable")
	}
	snapshot, err := app.SourceAcquirer.AcquireLocal(ctx, requested)
	if err != nil {
		return loadedPackage{}, err
	}
	if _, err := directoryv1.ResolveDirectExact(snapshot.Source); err != nil {
		_ = packagedigest.Remove(snapshot)
		return loadedPackage{}, err
	}
	loaded, err := app.loadSnapshot(ctx, snapshot, domain.OriginModeDirect, nil, nil)
	if err == nil {
		app.warnDirectRevocation(snapshot.TreeDigest)
	}
	return loaded, err
}

func (app App) acquireGitHub(ctx context.Context, requested, repository, revision, subpath, expectedDigest string) (loadedPackage, error) {
	if app.SourceAcquirer == nil {
		return loadedPackage{}, fmt.Errorf("package source acquirer is unavailable")
	}
	var snapshot domain.PackageSnapshot
	var err error
	if expectedDigest == "" {
		snapshot, err = app.SourceAcquirer.AcquireGitHub(ctx, repository, revision, subpath)
	} else {
		snapshot, err = app.SourceAcquirer.AcquireGitHubVerified(ctx, repository, revision, subpath, expectedDigest)
	}
	if err != nil {
		return loadedPackage{}, err
	}
	snapshot.Source.RequestedSource = requested
	if expectedDigest == "" {
		if _, err := directoryv1.ResolveDirectExact(snapshot.Source); err != nil {
			_ = packagedigest.Remove(snapshot)
			return loadedPackage{}, err
		}
	}
	loaded, err := app.loadSnapshot(ctx, snapshot, domain.OriginModeDirect, nil, nil)
	if err == nil {
		app.warnDirectRevocation(snapshot.TreeDigest)
	}
	return loaded, err
}

func (app App) warnDirectRevocation(treeDigest string) {
	local, ok := app.DirectoryClient.(localDirectoryReader)
	if !ok {
		return
	}
	floor := uint64(0)
	if app.StateStore != nil {
		if state, err := app.StateStore.Load(); err == nil {
			floor = installedDirectoryFloor(state)
		}
	}
	bundle, err := local.LoadLocal(floor)
	if err != nil {
		return
	}
	for _, warning := range exactDigestWarnings(bundle.Snapshot, treeDigest, "the direct source") {
		_, _ = fmt.Fprintf(app.errorOutput(), "WARNING [%s]: %s\n", warning.Code, warning.Message)
	}
}

func (app App) acquireDirectory(ctx context.Context, selector string, request packageResolutionRequest) (loadedPackage, error) {
	if app.DirectoryClient == nil || app.SourceAcquirer == nil {
		return loadedPackage{}, fmt.Errorf("signed Directory dependencies are unavailable; use a direct local or exact full-SHA source")
	}
	state, err := app.StateStore.Load()
	if err != nil {
		return loadedPackage{}, err
	}
	bundle, err := app.DirectoryClient.Load(ctx, installedDirectoryFloor(state))
	if err != nil {
		return loadedPackage{}, fmt.Errorf("load signed Directory: %w", err)
	}
	request, err = retainDirectoryRelease(bundle.Snapshot, state, selector, request)
	if err != nil {
		return loadedPackage{}, err
	}
	operation := request.Operation
	if operation == "" {
		operation = domain.DirectoryInstall
	}
	environment := directoryEnvironment(request.Clients)
	environment.InstallerVersion = app.Version
	resolveSelector := selector
	if request.Selector != "" {
		resolveSelector = request.Selector
	}
	resolveRequest := domain.DirectoryResolveRequest{
		Selector: resolveSelector, Targets: append([]domain.ClientID(nil), request.Targets...), Scope: domain.ScopeUser,
		InstallerVersion: app.Version, ClientVersions: environment.ClientVersions, OS: environment.OS, Architecture: environment.Architecture,
		DependencyIdentity: environment.DependencyIdentity,
		SchemaVersion:      "1.0.0", Operation: operation, Recorded: request.Recorded,
	}
	selection, err := domain.ResolveDirectory(bundle.Snapshot, resolveRequest)
	if err != nil {
		return loadedPackage{}, err
	}
	product, distribution, release, policy := directoryRecords(bundle.Snapshot, selection)
	if product == nil || distribution == nil || release == nil || policy == nil {
		return loadedPackage{}, fmt.Errorf("signed Directory selection cannot be reproduced from its snapshot")
	}
	if err := validateDirectoryCompatibilityPolicy(*policy); err != nil {
		return loadedPackage{}, err
	}
	snapshot, err := app.SourceAcquirer.AcquireGitHubVerified(ctx, selection.Source.Repository, selection.Source.Revision, selection.Source.Path, selection.TreeDigest)
	if err != nil {
		return loadedPackage{}, err
	}
	snapshot.Source.RequestedSource = selector
	if request.RequestedSelector != "" {
		snapshot.Source.RequestedSource = request.RequestedSelector
	}
	snapshot.Source.SourceBindingHint = "directory-v1"
	origin := &domain.DirectoryOrigin{
		ProductID: selection.ProductID, DistributionID: selection.DistributionID, DistributionKind: selection.DistributionKind,
		DesiredReleaseSequence: selection.ReleaseSequence, SnapshotSchema: directoryv1.SnapshotSchemaVersion,
		SnapshotSequence: selection.SnapshotSequence, SnapshotDigest: bundle.Digest,
	}
	loaded, err := app.loadSnapshot(ctx, snapshot, domain.OriginModeDirectory, origin, &selection)
	if err != nil {
		return loadedPackage{}, err
	}
	if loaded.envelope.ManifestDigest != selection.ManifestDigest {
		_ = loaded.cleanup()
		return loadedPackage{}, fmt.Errorf("acquired package manifest digest %s does not match signed Directory digest %s", loaded.envelope.ManifestDigest, selection.ManifestDigest)
	}
	if loaded.envelope.Manifest.Name != product.ManifestName || loaded.envelope.Manifest.Name != release.ManifestName {
		_ = loaded.cleanup()
		return loadedPackage{}, fmt.Errorf("signed Directory identity does not match acquired package manifest")
	}
	environment.DependencyIdentity = deterministicDependencyIdentity(loaded.envelope, request.Targets)
	exactRequest := resolveRequest
	exactRequest.Selector = selection.DistributionID
	exactRequest.Operation = domain.DirectoryNewTarget
	exactRequest.Recorded = &domain.RecordedDirectoryRelease{ProductID: selection.ProductID, DistributionID: selection.DistributionID, ReleaseSequence: selection.ReleaseSequence}
	exactRequest.DependencyIdentity = environment.DependencyIdentity
	checked, err := domain.ResolveDirectory(bundle.Snapshot, exactRequest)
	if err != nil || checked.DistributionID != selection.DistributionID || checked.ReleaseSequence != selection.ReleaseSequence {
		_ = loaded.cleanup()
		if err != nil {
			return loadedPackage{}, fmt.Errorf("acquired exact Directory release failed compatibility recheck: %w", err)
		}
		return loadedPackage{}, fmt.Errorf("acquired exact Directory release changed during compatibility recheck")
	}
	loaded.distributionSuspended = distribution.Status == domain.DistributionSuspended
	loaded.releaseRevoked = policy.Status == domain.ReleaseRevoked || snapshotRevokes(bundle.Snapshot, selection)
	if err := applyDirectoryCompatibility(&loaded, bundle, selection, *policy, environment); err != nil {
		_ = loaded.cleanup()
		return loadedPackage{}, err
	}
	return loaded, nil
}

func (app App) loadSnapshot(ctx context.Context, snapshot domain.PackageSnapshot, origin domain.OriginMode, directory *domain.DirectoryOrigin, selection *domain.DirectorySelection) (loadedPackage, error) {
	fail := func(err error) (loadedPackage, error) {
		_ = packagedigest.Remove(snapshot)
		return loadedPackage{}, err
	}
	envelope, err := app.loadAcquiredSnapshot(ctx, domain.LoadInput{SnapshotRoot: snapshot.Root, TreeDigest: snapshot.TreeDigest, ExecutableFiles: snapshot.ExecutableFiles, Source: snapshot.Source})
	if err != nil {
		return fail(err)
	}
	if envelope.TreeDigest != snapshot.TreeDigest {
		return fail(fmt.Errorf("loader changed the acquired immutable package digest"))
	}
	return loadedPackage{envelope: envelope, origin: origin, directory: cloneDirectoryOrigin(directory), directorySelection: cloneDirectorySelection(selection), cleanup: func() error { return packagedigest.Remove(snapshot) }}, nil
}

// loadAcquiredSnapshot selects a manifest format from the immutable snapshot.
// A root plugin.json has absolute precedence over compatibility imports.
func (app App) loadAcquiredSnapshot(ctx context.Context, input domain.LoadInput) (domain.PackageEnvelope, error) {
	if app.PackageLoader == nil || app.NativePackageLoader == nil {
		return domain.PackageEnvelope{}, fmt.Errorf("package loaders are unavailable")
	}
	if _, err := os.Lstat(filepath.Join(input.SnapshotRoot, "plugin.json")); err == nil || !os.IsNotExist(err) {
		return app.PackageLoader.Load(ctx, input)
	}
	return app.NativePackageLoader.Load(ctx, input)
}

func stableCatalogPackageMode(mode domain.PackageMode) (string, bool) {
	switch mode {
	case domain.PackageNative:
		return "native", true
	case domain.PackageProjection:
		return "projected", true
	case domain.PackagePrepared:
		return "prepared", true
	default:
		return "", false
	}
}

func validateDirectoryCompatibilityPolicy(policy domain.DirectoryReleasePolicy) error {
	for _, target := range policy.Targets {
		capabilities, ok := clientplanner.Capabilities(target.Client)
		if !ok {
			return fmt.Errorf("signed Directory target %q is unsupported", target.Client)
		}
		packageMode, ok := stableCatalogPackageMode(capabilities.PackageMode)
		if !ok {
			return fmt.Errorf("signed Directory target %q has unsupported package mode %q", target.Client, capabilities.PackageMode)
		}
		expectedDelivery, ok := domain.ExpectedDirectoryDelivery(target.Client)
		if !ok || target.Delivery != expectedDelivery {
			return fmt.Errorf("signed Directory delivery %q is incompatible with target %q; expected %q", target.Delivery, target.Client, expectedDelivery)
		}
		deliveryPackageMode := map[string]string{"managed": "native", "prepared": "prepared", "manual_activation": "projected"}[target.Delivery]
		if deliveryPackageMode == "" || deliveryPackageMode != packageMode {
			return fmt.Errorf("signed Directory delivery %q does not match package mode %q for target %q", target.Delivery, packageMode, target.Client)
		}
	}
	return nil
}

type directoryEvidenceEnvironment struct {
	InstallerVersion   string
	OS                 string
	Architecture       string
	ClientVersions     map[domain.ClientID]string
	DependencyIdentity map[domain.ClientID]string
}

func directoryEnvironment(clients map[domain.ClientID]domain.DetectedClient) directoryEvidenceEnvironment {
	versions := make(map[domain.ClientID]string, len(clients))
	for id, client := range clients {
		if client.ClientID == id && client.Version != "" {
			versions[id] = client.Version
		}
	}
	return directoryEvidenceEnvironment{OS: runtime.GOOS, Architecture: runtime.GOARCH, ClientVersions: versions, DependencyIdentity: map[domain.ClientID]string{}}
}

// deterministicDependencyIdentity returns a value only when the immutable
// package declares exactly one external stdio command. It inspects manifest
// metadata only; it does not resolve or execute the dependency.
func deterministicDependencyIdentity(envelope domain.PackageEnvelope, targets []domain.ClientID) map[domain.ClientID]string {
	commands := map[string]struct{}{}
	for _, server := range envelope.MCP.Servers {
		if requirement := server.StdioRequirement; requirement != nil && requirement.Kind == domain.ExecutableBare && strings.TrimSpace(requirement.Command) != "" {
			commands[requirement.Command] = struct{}{}
		}
	}
	if len(commands) != 1 {
		return map[domain.ClientID]string{}
	}
	var identity string
	for command := range commands {
		identity = command
	}
	result := make(map[domain.ClientID]string, len(targets))
	for _, target := range targets {
		result[target] = identity
	}
	return result
}

func applyDirectoryCompatibility(loaded *loadedPackage, bundle directoryv1.VerifiedBundle, selection domain.DirectorySelection, policy domain.DirectoryReleasePolicy, environment directoryEvidenceEnvironment) error {
	if err := validateDirectoryCompatibilityPolicy(policy); err != nil {
		return err
	}
	compatibility := make(map[string]domain.CatalogCompatibility, len(policy.Targets))
	current := currentDirectoryEvidence(bundle.Snapshot.Evidence, policy.CurrentEvidence)
	for _, target := range policy.Targets {
		capabilities, _ := clientplanner.Capabilities(target.Client)
		packageMode, _ := stableCatalogPackageMode(capabilities.PackageMode)
		applicable := make([]domain.DirectoryEvidence, 0, len(current))
		for _, evidence := range current {
			if evidence.HasTrustedEligibilityProvenance() && directoryEvidenceApplies(evidence, selection, target.Client, environment) {
				applicable = append(applicable, evidence)
			}
		}
		entry := domain.CatalogCompatibility{Package: packageMode, Verification: directoryVerification(applicable), Authentication: target.Authentication, Evidence: applicable, EvidenceOutcomes: directoryEvidenceOutcomes(applicable)}
		if target.AppBinding != nil {
			mcpURL := ""
			if server, ok := loaded.envelope.MCP.Servers[target.AppBinding.MCPServer]; ok {
				mcpURL, _ = server.Decoded["url"].(string)
			}
			entry.AppBinding = &domain.CatalogAppBinding{AppKey: target.AppBinding.AppKey, ID: target.AppBinding.ID, MCPServer: target.AppBinding.MCPServer, MCPURL: mcpURL}
		}
		compatibility[string(target.Client)] = entry
	}
	loaded.hints.Compatibility = cloneCatalogCompatibility(compatibility)
	loaded.envelope.CatalogEvidence = &domain.CatalogEvidence{
		SchemaVersion: 1, CatalogVersion: "directory-snapshot-1", Digest: bundle.Digest,
		Revision: bundle.Snapshot.SourceCommit, MinimumCLIVersion: policy.MinimumInstallerVersion,
		AgentPluginsSchema: loaded.envelope.SchemaURI, Compatibility: cloneCatalogCompatibility(compatibility),
		CurrentEvidence: append([]domain.DirectoryEvidence(nil), current...),
	}
	return nil
}

func currentDirectoryEvidence(history []domain.DirectoryEvidence, ids []string) []domain.DirectoryEvidence {
	byID := make(map[string]domain.DirectoryEvidence, len(history))
	for _, evidence := range history {
		byID[evidence.ID] = evidence
	}
	current := make([]domain.DirectoryEvidence, 0, len(ids))
	for _, id := range ids {
		if evidence, ok := byID[id]; ok && evidence.HasTrustedProvenance() {
			current = append(current, evidence)
		}
	}
	return current
}

func directoryEvidenceApplies(evidence domain.DirectoryEvidence, selection domain.DirectorySelection, client domain.ClientID, environment directoryEvidenceEnvironment) bool {
	if evidence.DistributionID != selection.DistributionID || evidence.ReleaseSequence != selection.ReleaseSequence || evidence.PackageTreeDigest != selection.TreeDigest {
		return false
	}
	if evidence.Level == "schema" {
		return evidence.Client == ""
	}
	if evidence.Client != client || !directoryEvidenceDimensionMatches(evidence.InstallerVersion, environment.InstallerVersion) ||
		!directoryEvidenceDimensionMatches(evidence.OS, environment.OS) || !directoryEvidenceDimensionMatches(evidence.Architecture, environment.Architecture) {
		return false
	}
	if evidence.ClientVersion != "" && environment.ClientVersions[client] != evidence.ClientVersion {
		return false
	}
	if evidence.DependencyIdentity != "" && environment.DependencyIdentity[client] != evidence.DependencyIdentity {
		return false
	}
	return true
}

func directoryEvidenceDimensionMatches(recorded, actual string) bool {
	return recorded == "" || recorded == actual
}

func directoryVerification(evidence []domain.DirectoryEvidence) string {
	schemaPassed := false
	for _, record := range evidence {
		if record.Outcome != "passed" {
			continue
		}
		if record.Level == "schema" {
			schemaPassed = true
			continue
		}
		if record.Level == "runtime" {
			return "tested"
		}
	}
	if schemaPassed {
		return "schema_only"
	}
	return "not_tested"
}

func directoryEvidenceOutcomes(evidence []domain.DirectoryEvidence) map[string]string {
	outcomes := map[string]string{"schema": "not_tested", "materialization": "not_tested", "discovery": "not_tested", "runtime": "not_tested", "oauth": "not_tested"}
	for _, record := range evidence {
		outcomes[record.Level] = record.Outcome
	}
	return outcomes
}

func directoryRecords(snapshot domain.DirectorySnapshot, selection domain.DirectorySelection) (*domain.DirectoryProduct, *domain.DirectoryDistribution, *domain.DirectoryRelease, *domain.DirectoryReleasePolicy) {
	var product *domain.DirectoryProduct
	for index := range snapshot.Products {
		if snapshot.Products[index].ID == selection.ProductID {
			product = &snapshot.Products[index]
			break
		}
	}
	var distribution *domain.DirectoryDistribution
	for index := range snapshot.Distributions {
		if snapshot.Distributions[index].ID == selection.DistributionID {
			distribution = &snapshot.Distributions[index]
			break
		}
	}
	if distribution == nil {
		return product, nil, nil, nil
	}
	var release *domain.DirectoryRelease
	var policy *domain.DirectoryReleasePolicy
	for index := range distribution.Releases {
		if distribution.Releases[index].Sequence == selection.ReleaseSequence {
			release = &distribution.Releases[index]
		}
	}
	for index := range distribution.ReleasePolicies {
		if distribution.ReleasePolicies[index].ReleaseSequence == selection.ReleaseSequence {
			policy = &distribution.ReleasePolicies[index]
		}
	}
	return product, distribution, release, policy
}

func snapshotRevokes(snapshot domain.DirectorySnapshot, selection domain.DirectorySelection) bool {
	for _, revocation := range snapshot.Revocations {
		if revocation.DistributionID == selection.DistributionID && revocation.ReleaseSequence == selection.ReleaseSequence {
			return true
		}
	}
	return false
}

func installedDirectoryFloor(state domain.StateFileV2) uint64 {
	var floor uint64
	for _, installation := range state.Installations {
		if installation.Directory != nil && installation.Directory.SnapshotSchema == directoryv1.SnapshotSchemaVersion && installation.Directory.SnapshotSequence > floor {
			floor = installation.Directory.SnapshotSequence
		}
	}
	return floor
}

func cloneDirectoryOrigin(value *domain.DirectoryOrigin) *domain.DirectoryOrigin {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func cloneDirectorySelection(value *domain.DirectorySelection) *domain.DirectorySelection {
	if value == nil {
		return nil
	}
	copy := *value
	copy.Diagnostics = append([]domain.DirectoryDiagnostic(nil), value.Diagnostics...)
	return &copy
}

func cloneCatalogCompatibility(source map[string]domain.CatalogCompatibility) map[string]domain.CatalogCompatibility {
	if len(source) == 0 {
		return nil
	}
	result := make(map[string]domain.CatalogCompatibility, len(source))
	for client, compatibility := range source {
		compatibility.Evidence = append([]domain.DirectoryEvidence(nil), compatibility.Evidence...)
		if compatibility.EvidenceOutcomes != nil {
			compatibility.EvidenceOutcomes = make(map[string]string, len(compatibility.EvidenceOutcomes))
			for level, outcome := range source[client].EvidenceOutcomes {
				compatibility.EvidenceOutcomes[level] = outcome
			}
		}
		if compatibility.AppBinding != nil {
			binding := *compatibility.AppBinding
			compatibility.AppBinding = &binding
		}
		result[client] = compatibility
	}
	return result
}

func prepareLoadedPackageForClient(loaded *loadedPackage, clientID domain.ClientID) error {
	if loaded == nil || clientID != domain.ClientChatGPT {
		return nil
	}
	compatibility, ok := loaded.hints.Compatibility[string(domain.ClientChatGPT)]
	if !ok || compatibility.AppBinding == nil {
		return nil
	}
	binding := *compatibility.AppBinding
	if err := catalog.ValidateAppBinding(binding); err != nil {
		return fmt.Errorf("Directory ChatGPT app binding is invalid: %w", err)
	}
	server, ok := loaded.envelope.MCP.Servers[binding.MCPServer]
	if !ok || !loaded.envelope.MCP.Enabled {
		return fmt.Errorf("Directory ChatGPT app binding references missing MCP server %q", binding.MCPServer)
	}
	serverURL, ok := server.Decoded["url"].(string)
	if !ok || serverURL != binding.MCPURL {
		return fmt.Errorf("Directory ChatGPT app binding URL does not match MCP server %q", binding.MCPServer)
	}
	if loaded.envelope.App.Present {
		existing, matches := loaded.envelope.App.Bindings[binding.AppKey]
		if !loaded.envelope.App.Enabled || !matches || len(loaded.envelope.App.Bindings) != 1 || existing.ID != binding.ID {
			return fmt.Errorf("package .app.json does not exactly match the Directory ChatGPT app binding")
		}
		return nil
	}
	entry := struct {
		ID string `json:"id"`
	}{ID: binding.ID}
	document := struct {
		Apps map[string]any `json:"apps"`
	}{Apps: map[string]any{binding.AppKey: entry}}
	raw, err := json.Marshal(document)
	if err != nil {
		return err
	}
	entryRaw, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	loaded.envelope.App = domain.AppComponent{Present: true, Declared: true, Enabled: true, Raw: raw, Bindings: map[string]domain.AppBinding{binding.AppKey: {Alias: binding.AppKey, ID: binding.ID, Raw: entryRaw}}}
	loaded.envelope.Inventory.AppPresent = true
	loaded.envelope.Inventory.AppBindings = []string{binding.AppKey}
	return nil
}

func restoreCatalogEvidence(loaded *loadedPackage, binding domain.ClientBinding) {
	if loaded == nil || binding.PackageRevision == nil {
		return
	}
	if binding.PackageRevision.CatalogEvidence == nil {
		loaded.envelope.CatalogEvidence = nil
		loaded.hints.Compatibility = nil
		return
	}
	evidence := *binding.PackageRevision.CatalogEvidence
	evidence.Compatibility = cloneCatalogCompatibility(evidence.Compatibility)
	loaded.envelope.CatalogEvidence = &evidence
	loaded.hints.Compatibility = cloneCatalogCompatibility(evidence.Compatibility)
}

func isDirectorySelector(value string) bool {
	if isShortName(value) {
		return true
	}
	parts := strings.Split(value, "/")
	return len(parts) == 2 && isShortName(parts[0]) && isShortName(parts[1])
}

func isShortName(value string) bool {
	return shortNamePattern.MatchString(value) && !strings.ContainsAny(value, `/\\`)
}

func explicitLocalPath(value string) bool {
	if filepath.IsAbs(value) || strings.HasPrefix(value, "./") || strings.HasPrefix(value, "../") || strings.HasPrefix(value, `.\`) || strings.HasPrefix(value, `..\`) {
		return true
	}
	// filepath.IsAbs follows the host OS. Recognize only genuinely absolute
	// Windows spellings as well so a source copied between shells is stable;
	// drive-relative forms such as C:plugin intentionally remain selectors.
	if len(value) >= 3 && ((value[0] >= 'A' && value[0] <= 'Z') || (value[0] >= 'a' && value[0] <= 'z')) && value[1] == ':' && (value[2] == '\\' || value[2] == '/') {
		return true
	}
	if !strings.HasPrefix(value, `\\`) {
		return false
	}
	unc := strings.TrimPrefix(value, `\\`)
	server, share, ok := strings.Cut(unc, `\`)
	return ok && server != "" && strings.Trim(share, `\`) != ""
}
