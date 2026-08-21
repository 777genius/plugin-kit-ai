package agentpluginscli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
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
	Targets   []domain.ClientID
	Operation domain.DirectoryOperation
	Recorded  *domain.RecordedDirectoryRelease
}

func (app App) addResolutionRequest(selector string, targets []domain.ClientID) packageResolutionRequest {
	request := packageResolutionRequest{Targets: append([]domain.ClientID(nil), targets...), Operation: domain.DirectoryInstall}
	state, err := app.StateStore.Load()
	if err != nil {
		return request
	}
	installation, ok := locallyMatchedInstallation(state, selector)
	if !ok || installation.Directory == nil {
		return request
	}
	request.Operation = domain.DirectoryNewTarget
	request.Recorded = &domain.RecordedDirectoryRelease{ProductID: installation.Directory.ProductID, DistributionID: installation.Directory.DistributionID, ReleaseSequence: installation.Directory.DesiredReleaseSequence}
	return request
}

func (app App) loadInstalledPackage(ctx context.Context, installation domain.Installation, targets []domain.ClientID, operation domain.DirectoryOperation, releaseSequence uint64) (loadedPackage, error) {
	if installation.OriginMode != domain.OriginModeDirectory || installation.Directory == nil {
		return app.loadPackageFor(ctx, updateSource(installation), packageResolutionRequest{Targets: targets, Operation: operation})
	}
	sequence := releaseSequence
	if sequence == 0 {
		sequence = installation.Directory.DesiredReleaseSequence
	}
	return app.loadPackageFor(ctx, installation.Directory.ProductID, packageResolutionRequest{Targets: targets, Operation: operation, Recorded: &domain.RecordedDirectoryRelease{
		ProductID: installation.Directory.ProductID, DistributionID: installation.Directory.DistributionID, ReleaseSequence: sequence,
	}})
}

func locallyMatchedInstallation(state domain.StateFileV2, selector string) (domain.Installation, bool) {
	selector = strings.TrimSpace(selector)
	var matches []domain.Installation
	for _, installation := range state.Installations {
		matched := installation.InstallationID == selector || installation.DeclaredName == selector
		if installation.Directory != nil {
			matched = matched || installation.Directory.ProductID == selector || installation.Directory.DistributionID == selector
		}
		if matched {
			matches = append(matches, installation)
		}
	}
	if len(matches) != 1 {
		return domain.Installation{}, false
	}
	return matches[0], true
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
	return app.loadSnapshot(ctx, snapshot, domain.OriginModeDirect, nil, nil)
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
	return app.loadSnapshot(ctx, snapshot, domain.OriginModeDirect, nil, nil)
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
	operation := request.Operation
	if operation == "" {
		operation = domain.DirectoryInstall
	}
	selection, err := domain.ResolveDirectory(bundle.Snapshot, domain.DirectoryResolveRequest{
		Selector: selector, Targets: append([]domain.ClientID(nil), request.Targets...), Scope: domain.ScopeUser,
		InstallerVersion: app.Version, SchemaVersion: "1.0.0", Operation: operation, Recorded: request.Recorded,
	})
	if err != nil {
		return loadedPackage{}, err
	}
	snapshot, err := app.SourceAcquirer.AcquireGitHubVerified(ctx, selection.Source.Repository, selection.Source.Revision, selection.Source.Path, selection.TreeDigest)
	if err != nil {
		return loadedPackage{}, err
	}
	snapshot.Source.RequestedSource = selector
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
	product, distribution, release, policy := directoryRecords(bundle.Snapshot, selection)
	if product == nil || distribution == nil || release == nil || policy == nil {
		_ = loaded.cleanup()
		return loadedPackage{}, fmt.Errorf("signed Directory selection cannot be reproduced from its snapshot")
	}
	if loaded.envelope.Manifest.Name != product.ManifestName || loaded.envelope.Manifest.Name != release.ManifestName {
		_ = loaded.cleanup()
		return loadedPackage{}, fmt.Errorf("signed Directory identity does not match acquired package manifest")
	}
	loaded.distributionSuspended = distribution.Status == domain.DistributionSuspended
	loaded.releaseRevoked = policy.Status == domain.ReleaseRevoked || snapshotRevokes(bundle.Snapshot, selection)
	applyDirectoryCompatibility(&loaded, bundle, *policy)
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

func applyDirectoryCompatibility(loaded *loadedPackage, bundle directoryv1.VerifiedBundle, policy domain.DirectoryReleasePolicy) {
	compatibility := make(map[string]domain.CatalogCompatibility, len(policy.Targets))
	for _, target := range policy.Targets {
		capabilities, ok := clientplanner.Capabilities(target.Client)
		if !ok {
			continue
		}
		verification := "not_tested"
		for _, evidenceID := range policy.CurrentEvidence {
			for _, evidence := range bundle.Snapshot.Evidence {
				if evidence.ID == evidenceID && (evidence.Client == "" || evidence.Client == target.Client) && evidence.Outcome == "passed" {
					verification = "tested"
				}
			}
		}
		entry := domain.CatalogCompatibility{Package: string(capabilities.PackageMode), Verification: verification, Authentication: domain.AuthenticationRequirementUnknown}
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
	}
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
	if loaded == nil || loaded.envelope.CatalogEvidence != nil || binding.PackageRevision == nil || binding.PackageRevision.CatalogEvidence == nil {
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
