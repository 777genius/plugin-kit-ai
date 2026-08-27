package agentpluginscli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/777genius/plugin-kit-ai/install/integrationctl/agentplugins/adapters/directoryv1"
	"github.com/777genius/plugin-kit-ai/install/integrationctl/agentplugins/adapters/discoveryv1"
	"github.com/777genius/plugin-kit-ai/install/integrationctl/agentplugins/domain"
	"github.com/spf13/cobra"
)

type outdatedReleaseIdentity struct {
	DistributionID  string `json:"distribution_id"`
	ReleaseSequence uint64 `json:"release_sequence"`
	PackageVersion  string `json:"package_version,omitempty"`
	Revision        string `json:"revision,omitempty"`
	TreeDigest      string `json:"tree_digest,omitempty"`
	ManifestDigest  string `json:"manifest_digest,omitempty"`
}

type outdatedInstallation struct {
	InstallationID string                   `json:"installation_id"`
	Name           string                   `json:"name"`
	Status         string                   `json:"status"`
	Reason         string                   `json:"reason"`
	Targets        []domain.ClientID        `json:"targets,omitempty"`
	Installed      *outdatedReleaseIdentity `json:"installed,omitempty"`
	Available      *outdatedReleaseIdentity `json:"available,omitempty"`
	Warnings       []publicSafetyWarning    `json:"warnings,omitempty"`
}

type outdatedResult struct {
	ReadOnly              bool                   `json:"read_only"`
	Snapshot              uint64                 `json:"snapshot_sequence,omitempty"`
	SnapshotHash          string                 `json:"snapshot_digest,omitempty"`
	DiscoverySnapshot     uint64                 `json:"discovery_snapshot_sequence,omitempty"`
	DiscoverySnapshotHash string                 `json:"discovery_snapshot_digest,omitempty"`
	Outdated              int                    `json:"outdated"`
	Current               int                    `json:"current"`
	Blocked               int                    `json:"blocked"`
	Unknown               int                    `json:"unknown"`
	Unmanaged             int                    `json:"unmanaged"`
	Installations         []outdatedInstallation `json:"installations"`
}

func newOutdatedCommand(app App, opts *options) *cobra.Command {
	var all bool
	command := &cobra.Command{
		Use:   "outdated [name-or-installation-id]",
		Short: "Check immutable release identities without changing installations",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateCommonOptions(opts); err != nil {
				return err
			}
			if all && len(args) > 0 {
				return fmt.Errorf("choose either one installation or --all")
			}
			selector := ""
			if len(args) == 1 {
				selector = args[0]
			}
			return runOutdated(cmd.Context(), cmd, app, opts, selector)
		},
	}
	command.Flags().BoolVar(&all, "all", false, "check every tracked installation")
	return command
}

func runOutdated(ctx context.Context, cmd *cobra.Command, app App, opts *options, selector string) error {
	state, err := app.StateStore.Load()
	if err != nil {
		return err
	}
	installations := append([]domain.Installation(nil), state.Installations...)
	if strings.TrimSpace(selector) != "" {
		installation, err := selectInstallation(state, selector)
		if err != nil {
			return err
		}
		installations = []domain.Installation{installation}
	}
	sort.Slice(installations, func(i, j int) bool {
		left, right := strings.ToLower(installations[i].DeclaredName), strings.ToLower(installations[j].DeclaredName)
		if left != right {
			return left < right
		}
		return installations[i].InstallationID < installations[j].InstallationID
	})

	result := outdatedResult{ReadOnly: true, Installations: make([]outdatedInstallation, 0, len(installations))}
	needsDirectory := false
	for _, installation := range installations {
		if installation.OriginMode == domain.OriginModeDirectory && installation.Directory != nil {
			needsDirectory = true
			break
		}
	}
	var bundle directoryv1.VerifiedBundle
	bundleOK := false
	if needsDirectory {
		bundle, bundleOK, err = directoryBundleForRead(ctx, app, state, true)
		if err != nil {
			return fmt.Errorf("load signed Directory: %w", err)
		}
		if bundleOK {
			result.Snapshot, result.SnapshotHash = bundle.Snapshot.Sequence, bundle.Digest
		}
	}
	needsDiscovery := false
	for _, installation := range installations {
		if strings.HasPrefix(strings.TrimSpace(installation.Source.RequestedSource), "discovery:") {
			needsDiscovery = true
			break
		}
	}
	var discovery discoveryv1.VerifiedBundle
	var discoveryErr error
	if needsDiscovery {
		if app.DiscoveryClient == nil {
			discoveryErr = errors.New("signed Discovery Index dependencies are unavailable")
		} else {
			discovery, discoveryErr = app.DiscoveryClient.Load(ctx, 0)
			if discoveryErr == nil {
				result.DiscoverySnapshot, result.DiscoverySnapshotHash = discovery.Snapshot.Sequence, discovery.Digest
			}
		}
	}

	var detected map[domain.ClientID]domain.DetectedClient
	var detectionErr error
	if needsDirectory && bundleOK {
		clients, err := detectClientsForLifecycleResolution(ctx, app.Detector, false)
		if err != nil {
			detectionErr = fmt.Errorf("detect AI clients: %w", err)
		} else {
			detected = make(map[domain.ClientID]domain.DetectedClient, len(clients)+1)
			for _, client := range clients {
				detected[client.ClientID] = client
			}
		}
	}

	for _, installation := range installations {
		var item outdatedInstallation
		if strings.HasPrefix(strings.TrimSpace(installation.Source.RequestedSource), "discovery:") {
			item = inspectDiscoveryOutdated(discovery, discoveryErr, installation, opts.scope)
		} else {
			item = inspectOutdatedInstallation(bundle, bundleOK, detected, detectionErr, app.Version, installation, opts.scope)
		}
		result.Installations = append(result.Installations, item)
		switch item.Status {
		case "outdated":
			result.Outdated++
		case "current":
			result.Current++
		case "blocked":
			result.Blocked++
		case "unknown":
			result.Unknown++
		default:
			result.Unmanaged++
		}
	}
	if opts.format == "json" {
		return writeJSONOutput(cmd.OutOrStdout(), "outdated", result)
	}
	return renderOutdated(cmd.OutOrStdout(), result)
}

func inspectDiscoveryOutdated(bundle discoveryv1.VerifiedBundle, bundleErr error, installation domain.Installation, scope string) outdatedInstallation {
	item := outdatedInstallation{InstallationID: installation.InstallationID, Name: installation.DeclaredName, Targets: installationTargets(installation, scope)}
	selector := strings.TrimSpace(installation.Source.RequestedSource)
	item.Installed = &outdatedReleaseIdentity{DistributionID: selector, PackageVersion: installation.Package.Version,
		Revision: installation.Source.ResolvedRevision, TreeDigest: installation.Source.TreeDigest, ManifestDigest: installation.Package.ManifestDigest}
	if installation.NeedsRebind {
		item.Status, item.Reason = "blocked", "installation requires explicit rebind before update"
		return item
	}
	if bundleErr != nil {
		item.Status, item.Reason = "unknown", "no authenticated Discovery snapshot is available: "+bundleErr.Error()
		return item
	}
	var matches []discoveryv1.Record
	for _, record := range bundle.Search.Records {
		if record.Slug == selector {
			matches = append(matches, record)
		}
	}
	if len(matches) != 1 {
		item.Status, item.Reason = "blocked", "recorded Discovery selector is absent or ambiguous"
		return item
	}
	record := matches[0]
	item.Available = &outdatedReleaseIdentity{DistributionID: record.Slug, Revision: record.Revision, TreeDigest: record.TreeDigest, ManifestDigest: record.ManifestDigest}
	if record.Version != nil {
		item.Available.PackageVersion = *record.Version
	}
	if record.Availability != "available" {
		item.Status, item.Reason = "blocked", "Discovery source is unavailable"
		return item
	}
	if record.Repository != installation.Source.Repository || record.PackagePath != installation.Source.PackageSubpath {
		item.Status, item.Reason = "blocked", "Discovery selector changed repository or package path; explicit switch required"
		return item
	}
	if record.Revision == installation.Source.ResolvedRevision && record.TreeDigest == installation.Source.TreeDigest && record.ManifestDigest == installation.Package.ManifestDigest {
		item.Status, item.Reason = "current", "the signed Discovery record matches the installed immutable package"
		item.Available = nil
		return item
	}
	item.Status, item.Reason = "outdated", "the same Discovery source has a newer immutable package identity"
	return item
}

func inspectOutdatedInstallation(bundle directoryv1.VerifiedBundle, bundleOK bool, detected map[domain.ClientID]domain.DetectedClient, detectionErr error, installerVersion string, installation domain.Installation, scope string) outdatedInstallation {
	item := outdatedInstallation{InstallationID: installation.InstallationID, Name: installation.DeclaredName}
	if installation.NeedsRebind {
		item.Status, item.Reason = "blocked", "installation requires explicit rebind before update"
		return item
	}
	if installation.Package.LoaderKind != domain.LoaderKindAgentPlugins {
		item.Status, item.Reason = "unmanaged", "legacy plugin.yaml installation has no Agent Plugins release identity"
		return item
	}
	if installation.OriginMode != domain.OriginModeDirectory || installation.Directory == nil {
		item.Status, item.Reason = "unmanaged", "direct source has no signed update channel; use an explicit source switch"
		return item
	}
	origin := installation.Directory
	item.Targets = installationTargets(installation, scope)
	item.Installed = &outdatedReleaseIdentity{DistributionID: origin.DistributionID, ReleaseSequence: origin.DesiredReleaseSequence,
		PackageVersion: installation.Package.Version, Revision: installation.Source.ResolvedRevision,
		TreeDigest: installation.Source.TreeDigest, ManifestDigest: installation.Package.ManifestDigest}
	if !bundleOK {
		item.Status, item.Reason = "unknown", "no authenticated Directory snapshot is available"
		return item
	}
	item.Warnings = installedDirectoryWarnings(bundle.Snapshot, installation)
	if detectionErr != nil {
		item.Status, item.Reason = "unknown", detectionErr.Error()
		return item
	}
	if len(item.Targets) == 0 {
		item.Status, item.Reason = "blocked", "installation has no materialized target in the selected scope"
		return item
	}
	if selectedTargetsNeedDesiredRelease(installation, item.Targets, scope) {
		item.Status, item.Reason = "outdated", "one or more installed clients have not converged to the recorded desired release"
		item.Available = cloneOutdatedIdentity(item.Installed)
		return item
	}
	_, clientMap, err := preflightSelectedTargets(context.Background(), App{Detector: staticDetectedClientSource(detected)}, item.Targets, detectedClientValues(detected), false)
	if err != nil {
		item.Status, item.Reason = "blocked", err.Error()
		return item
	}
	environment := directoryEnvironment(clientMap)
	environment.InstallerVersion = installerVersion
	selection, err := domain.ResolveDirectory(bundle.Snapshot, domain.DirectoryResolveRequest{
		Selector: origin.ProductID, Targets: expandAffectedSurfaceTargets(item.Targets), Scope: domain.ScopeUser,
		InstallerVersion: installerVersion, ClientVersions: environment.ClientVersions, OS: environment.OS,
		Architecture: environment.Architecture, DependencyIdentity: environment.DependencyIdentity,
		SchemaVersion: "1.0.0", Operation: domain.DirectoryUpdate,
		Recorded: recordedDirectoryRelease(installation, origin.DesiredReleaseSequence, installation.Source.ResolvedRevision),
	})
	if err == nil {
		item.Status, item.Reason = "outdated", "a later eligible immutable release is available"
		item.Available = &outdatedReleaseIdentity{DistributionID: selection.DistributionID, ReleaseSequence: selection.ReleaseSequence,
			PackageVersion: selection.PackageVersion, Revision: selection.Source.Revision,
			TreeDigest: selection.TreeDigest, ManifestDigest: selection.ManifestDigest}
		return item
	}
	if errors.Is(err, domain.ErrDirectoryIneligible) && errors.Is(err, domain.ErrDirectoryNoSafeUpdate) {
		if len(item.Warnings) > 0 {
			item.Status, item.Reason = "blocked", "installed release is unsafe and no later eligible release is available"
		} else {
			item.Status, item.Reason = "current", "no later eligible immutable release exists"
		}
		return item
	}
	item.Status, item.Reason = "blocked", err.Error()
	return item
}

// staticDetectedClientSource is used only to reuse the exact client-surface
// preflight rules for a previously detected, read-only snapshot.
type staticDetectedClientSource map[domain.ClientID]domain.DetectedClient

func (source staticDetectedClientSource) Detect(context.Context) ([]domain.DetectedClient, error) {
	return detectedClientValues(source), nil
}

func cloneOutdatedIdentity(identity *outdatedReleaseIdentity) *outdatedReleaseIdentity {
	if identity == nil {
		return nil
	}
	clone := *identity
	return &clone
}

func renderOutdated(writer io.Writer, result outdatedResult) error {
	if len(result.Installations) == 0 {
		_, err := fmt.Fprintln(writer, "No tracked Agent Plugins installations.")
		return err
	}
	for _, installation := range result.Installations {
		if _, err := fmt.Fprintf(writer, "%s: %s - %s\n", installation.Name, installation.Status, installation.Reason); err != nil {
			return err
		}
		if installation.Available != nil {
			label := fmt.Sprintf("%s release %d", installation.Available.DistributionID, installation.Available.ReleaseSequence)
			if installation.Available.ReleaseSequence == 0 {
				label = installation.Available.DistributionID
			}
			if _, err := fmt.Fprintf(writer, "  Available: %s (%s)\n", label, installation.Available.Revision); err != nil {
				return err
			}
		}
	}
	return nil
}
