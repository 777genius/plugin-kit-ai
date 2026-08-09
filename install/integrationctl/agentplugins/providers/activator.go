package providers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/777genius/plugin-kit-ai/install/integrationctl/adapters/pathpolicy"
	"github.com/777genius/plugin-kit-ai/install/integrationctl/agentplugins/domain"
	legacyports "github.com/777genius/plugin-kit-ai/install/integrationctl/ports"
)

type CommandRunner interface {
	Run(context.Context, legacyports.Command) (legacyports.CommandResult, error)
}

type Activator struct {
	Runner CommandRunner
}

func (activator Activator) Deactivate(ctx context.Context, request domain.DeactivationRequest) (domain.DeactivationOutcome, error) {
	if err := ctx.Err(); err != nil {
		return domain.DeactivationOutcome{}, err
	}
	outcome := domain.DeactivationOutcome{Activation: domain.ActivationNotRequired, ArtifactRemovalAllowed: true}
	switch request.Client.ClientID {
	case domain.ClientCursor:
		return outcome, nil
	case domain.ClientCodex:
		return requireExternalUninstall(outcome, request.ExternalUninstalled, "uninstall the plugin in Codex or ChatGPT, then rerun remove with `--external-uninstalled` (also use the flag if it was never activated)"), nil
	case domain.ClientKiro:
		return requireExternalUninstall(outcome, request.ExternalUninstalled, "remove the custom Power in Kiro, then rerun remove with `--external-uninstalled` (also use the flag if it was never imported)"), nil
	case domain.ClientCopilot, domain.ClientVSCode:
		if request.ExternalUninstalled {
			outcome.ExternalRemovalComplete = true
			return outcome, nil
		}
		if strings.TrimSpace(request.BackendExecutable) == "" || activator.Runner == nil {
			action := fmt.Sprintf("run `copilot plugin uninstall %s`, then rerun remove with `--external-uninstalled`", request.DeclaredName)
			if request.Client.ClientID == domain.ClientVSCode {
				action = "remove the plugin in VS Code, then rerun remove with `--external-uninstalled`"
			}
			return requireExternalUninstall(outcome, false, action), nil
		}
		if request.CurrentActivation != domain.ActivationActive {
			action := fmt.Sprintf("verify `%s@%s` is absent from GitHub Copilot CLI, then rerun remove with `--external-uninstalled`", request.DeclaredName, managedMarketplaceName(request.PhysicalArtifactID))
			return requireExternalUninstall(outcome, false, action), nil
		}
		if !request.Confirmed {
			outcome.UserActions = append(outcome.UserActions, "agentplugins will uninstall the plugin from GitHub Copilot CLI and VS Code automatically")
			return outcome, nil
		}
		if err := activator.deactivateCopilot(ctx, request); err != nil {
			return outcome, err
		}
		outcome.ExternalRemovalComplete = true
		return outcome, nil
	default:
		return domain.DeactivationOutcome{}, fmt.Errorf("unsupported deactivation client %q", request.Client.ClientID)
	}
}

func requireExternalUninstall(outcome domain.DeactivationOutcome, complete bool, action string) domain.DeactivationOutcome {
	if complete {
		outcome.ExternalRemovalComplete = true
		return outcome
	}
	outcome.Activation = domain.ActivationManual
	outcome.ArtifactRemovalAllowed = false
	outcome.UserActions = append(outcome.UserActions, action)
	return outcome
}

func (activator Activator) Activate(ctx context.Context, request domain.ActivationRequest) (domain.ActivationOutcome, error) {
	if err := ctx.Err(); err != nil {
		return domain.ActivationOutcome{}, err
	}
	if request.Plan.ClientID != request.Client.ClientID || request.Delivery.ClientID != request.Client.ClientID {
		return domain.ActivationOutcome{}, fmt.Errorf("activation client identity mismatch")
	}
	if request.Plan.ActivePath != request.Delivery.ActivePath {
		return domain.ActivationOutcome{}, fmt.Errorf("activation artifact path mismatch")
	}
	if strings.TrimSpace(request.DeclaredName) == "" {
		return domain.ActivationOutcome{}, fmt.Errorf("activation plugin name is required")
	}
	if err := pathpolicy.RequireContainedChild(request.Delivery.OwnedBase, request.Delivery.ActivePath); err != nil {
		return domain.ActivationOutcome{}, fmt.Errorf("unsafe activation artifact: %w", err)
	}
	info, err := os.Lstat(request.Delivery.ActivePath)
	if err != nil {
		return domain.ActivationOutcome{}, fmt.Errorf("inspect activation artifact: %w", err)
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return domain.ActivationOutcome{}, fmt.Errorf("activation artifact must be a real directory")
	}
	outcome := domain.ActivationOutcome{
		Authentication: request.Plan.Authentication,
		Policy:         domain.PolicyAllowed,
		Verification:   domain.VerificationPackageValid,
	}
	if request.ActivationComplete && !activationObservable(request, activator.Runner != nil) {
		outcome.Activation = domain.ActivationActive
		outcome.Verification = domain.VerificationInstalled
		outcome.ActivationAttested = true
		return outcome, nil
	}
	switch request.Client.ClientID {
	case domain.ClientCursor:
		if request.VerifyOnly {
			outcome.Activation = domain.ActivationManual
			outcome.UserActions = []string{"confirm that the plugin is visible and enabled in Cursor"}
			outcome.LocalActions = []string{fmt.Sprintf("open Cursor and verify the plugin from %s is visible and enabled", request.Delivery.ActivePath)}
			return outcome, nil
		}
		outcome.Activation = domain.ActivationManual
		outcome.UserActions = append(outcome.UserActions, "register the prepared package in Cursor and verify it is visible before using its components")
		outcome.LocalActions = append(outcome.LocalActions, fmt.Sprintf("open Cursor and register %s, then reload Cursor and verify %s is visible", request.Delivery.ActivePath, request.DeclaredName))
		return outcome, nil
	case domain.ClientCodex:
		if strings.TrimSpace(request.BackendExecutable) == "" || activator.Runner == nil {
			outcome.Activation = domain.ActivationManual
			outcome.UserActions = append(outcome.UserActions, "install the prepared plugin in the ChatGPT/Codex app, then verify it appears in Plugins > Personal")
			outcome.LocalActions = append(outcome.LocalActions, fmt.Sprintf("in the ChatGPT/Codex app, install %s from %s, then verify it appears in Plugins > Personal", request.DeclaredName, request.Delivery.ActivePath))
			return outcome, nil
		}
		if request.VerifyOnly {
			if err := activator.verifyCodex(ctx, request); err != nil {
				if errors.Is(err, errCodexListContractUnknown) {
					return manualCodexVerification(outcome, request), nil
				}
				return failedActivation(outcome, fmt.Sprintf("verify with `%s plugin list --json`", request.BackendExecutable), err)
			}
			outcome.Activation = domain.ActivationActive
			outcome.Verification = domain.VerificationInstalled
			return outcome, nil
		}
		if err := activator.activateCodex(ctx, request); err != nil {
			if errors.Is(err, errCodexListContractUnknown) {
				return manualCodexVerification(outcome, request), nil
			}
			return failedActivation(outcome, fmt.Sprintf("run Codex activation again for the prepared package at %s, then verify with `%s plugin list --json`", request.Delivery.ActivePath, request.BackendExecutable), err)
		}
		outcome.Activation = domain.ActivationActive
		outcome.Verification = domain.VerificationInstalled
		return outcome, nil
	case domain.ClientKiro:
		if !mcpOnly(request.Plan.Components) {
			outcome.Activation = domain.ActivationManual
			outcome.UserActions = append(outcome.UserActions, "import the prepared package as a custom Power in Kiro, then verify it is active")
			outcome.LocalActions = append(outcome.LocalActions, fmt.Sprintf("Kiro: Powers > Add Custom Power > Import power from a folder > select %s > Install, then verify %s is active", request.Delivery.ActivePath, request.DeclaredName))
			return outcome, nil
		}
		if request.VerifyOnly {
			if err := activator.verifyKiroMCP(ctx, request); err != nil {
				if errors.Is(err, errKiroStatusContractUnknown) {
					return manualKiroVerification(outcome, request), nil
				}
				return failedActivation(outcome, fmt.Sprintf("verify each server with `%s mcp status --name <server>`", request.BackendExecutable), err)
			}
			outcome.Activation = domain.ActivationActive
			outcome.Verification = domain.VerificationInstalled
			return outcome, nil
		}
		if !isKiroCLI(request.BackendExecutable) || activator.Runner == nil {
			outcome.Activation = domain.ActivationManual
			outcome.UserActions = append(outcome.UserActions, "install kiro-cli and rerun add to import and verify the prepared MCP configuration")
			outcome.LocalActions = append(outcome.LocalActions, fmt.Sprintf("install kiro-cli, rerun add for %s, then verify each MCP server with `kiro-cli mcp status --name <server>`", request.Delivery.ActivePath))
			return outcome, nil
		}
		if err := activator.activateKiroMCP(ctx, request); err != nil {
			if errors.Is(err, errKiroStatusContractUnknown) {
				return manualKiroVerification(outcome, request), nil
			}
			return failedActivation(outcome, fmt.Sprintf("rerun the MCP import from %s, then verify each server with `%s mcp status --name <server>`", filepath.Join(request.Delivery.ActivePath, "mcp.json"), request.BackendExecutable), err)
		}
		outcome.Activation = domain.ActivationActive
		outcome.Verification = domain.VerificationInstalled
		return outcome, nil
	case domain.ClientCopilot, domain.ClientVSCode:
		if strings.TrimSpace(request.BackendExecutable) == "" || activator.Runner == nil {
			outcome.Activation = domain.ActivationManual
			if request.Client.ClientID == domain.ClientVSCode {
				outcome.UserActions = append(outcome.UserActions, "register the prepared local plugin in VS Code")
				outcome.LocalActions = append(outcome.LocalActions, fmt.Sprintf(
					"VS Code: add %q to the `chat.pluginLocations` setting with value `true`, then reload VS Code and verify %s appears in the Plugins view",
					request.Delivery.ActivePath,
					request.DeclaredName,
				))
			} else {
				outcome.UserActions = append(outcome.UserActions, "install GitHub Copilot CLI, then rerun `agentplugins update` for this plugin")
				outcome.LocalActions = append(outcome.LocalActions, fmt.Sprintf("install GitHub Copilot CLI, rerun add for the prepared package at %s, then verify with `copilot plugin list`", request.Delivery.ActivePath))
			}
			return outcome, nil
		}
		if request.VerifyOnly {
			if err := activator.verifyCopilot(ctx, request); err != nil {
				if errors.Is(err, errCopilotListContractUnknown) {
					return manualCopilotVerification(outcome, request), nil
				}
				return failedActivation(outcome, fmt.Sprintf("verify with `%s plugin list`", request.BackendExecutable), err)
			}
			outcome.Activation = domain.ActivationActive
			outcome.Verification = domain.VerificationInstalled
			return outcome, nil
		}
		if err := activator.activateCopilot(ctx, request); err != nil {
			if errors.Is(err, errCopilotListContractUnknown) {
				return manualCopilotVerification(outcome, request), nil
			}
			return failedActivation(outcome, fmt.Sprintf("rerun add for the prepared package at %s, then verify with `%s plugin list`", request.Delivery.ActivePath, request.BackendExecutable), err)
		}
		outcome.Activation = domain.ActivationActive
		outcome.Verification = domain.VerificationInstalled
		return outcome, nil
	default:
		return domain.ActivationOutcome{}, fmt.Errorf("unsupported activation client %q", request.Client.ClientID)
	}
}

func (activator Activator) activateCopilot(ctx context.Context, request domain.ActivationRequest) error {
	marketplace := managedMarketplaceName(request.Plan.PhysicalArtifactID)
	updated := false
	if request.Replacing {
		if err := activator.runCopilot(ctx, request.BackendExecutable, "plugin", "marketplace", "update", marketplace); err != nil {
			if fallbackErr := activator.runCopilot(ctx, request.BackendExecutable, "plugin", "marketplace", "add", request.Delivery.ActivePath); fallbackErr != nil {
				return fmt.Errorf("refresh managed Copilot marketplace: %v; fallback registration: %w", err, fallbackErr)
			}
		}
		updated = activator.runCopilot(ctx, request.BackendExecutable, "plugin", "update", request.DeclaredName+"@"+marketplace) == nil
	} else if err := activator.runCopilot(ctx, request.BackendExecutable, "plugin", "marketplace", "add", request.Delivery.ActivePath); err != nil {
		if fallbackErr := activator.runCopilot(ctx, request.BackendExecutable, "plugin", "marketplace", "update", marketplace); fallbackErr != nil {
			return fmt.Errorf("register managed Copilot marketplace: %v; fallback refresh: %w", err, fallbackErr)
		}
	}
	pluginSpec := request.DeclaredName + "@" + marketplace
	if !updated {
		if err := activator.runCopilot(ctx, request.BackendExecutable, "plugin", "install", pluginSpec); err != nil {
			if !request.Replacing {
				_ = activator.runCopilot(ctx, request.BackendExecutable, "plugin", "marketplace", "remove", marketplace)
			}
			return err
		}
	}
	return activator.verifyCopilot(ctx, request)
}

func (activator Activator) verifyCopilot(ctx context.Context, request domain.ActivationRequest) error {
	pluginSpec := request.DeclaredName + "@" + managedMarketplaceName(request.Plan.PhysicalArtifactID)
	listed, err := activator.runCopilotResult(ctx, request.BackendExecutable, "plugin", "list")
	if err != nil {
		return fmt.Errorf("verify Copilot plugin listing: %w", err)
	}
	switch copilotPluginStatus(listed.Stdout, pluginSpec) {
	case copilotStatusInstalled:
		return nil
	case copilotStatusAbsent:
		return fmt.Errorf("%w: verify Copilot plugin listing: %s is not listed", errRecognizedNegativeEvidence, pluginSpec)
	default:
		return fmt.Errorf("%w: verify Copilot plugin listing", errCopilotListContractUnknown)
	}
}

func (activator Activator) activateCodex(ctx context.Context, request domain.ActivationRequest) error {
	marketplace := managedMarketplaceName(request.Plan.PhysicalArtifactID)
	if request.Replacing {
		if _, err := activator.runClientResult(ctx, "Codex CLI", request.BackendExecutable, "plugin", "marketplace", "update", marketplace, "--json"); err != nil {
			if _, fallbackErr := activator.runClientResult(ctx, "Codex CLI", request.BackendExecutable, "plugin", "marketplace", "add", request.Delivery.ActivePath, "--json"); fallbackErr != nil {
				return fmt.Errorf("refresh Codex marketplace: %v; fallback registration: %w", err, fallbackErr)
			}
		}
	} else if _, err := activator.runClientResult(ctx, "Codex CLI", request.BackendExecutable, "plugin", "marketplace", "add", request.Delivery.ActivePath, "--json"); err != nil {
		return fmt.Errorf("register Codex marketplace: %w", err)
	}
	pluginSpec := request.DeclaredName + "@" + marketplace
	if _, err := activator.runClientResult(ctx, "Codex CLI", request.BackendExecutable, "plugin", "add", pluginSpec, "--json"); err != nil {
		if !request.Replacing {
			_, _ = activator.runClientResult(ctx, "Codex CLI", request.BackendExecutable, "plugin", "marketplace", "remove", marketplace, "--json")
		}
		return fmt.Errorf("activate Codex plugin: %w", err)
	}
	return activator.verifyCodex(ctx, request)
}

func (activator Activator) verifyCodex(ctx context.Context, request domain.ActivationRequest) error {
	marketplace := managedMarketplaceName(request.Plan.PhysicalArtifactID)
	pluginSpec := request.DeclaredName + "@" + marketplace
	listed, err := activator.runClientResult(ctx, "Codex CLI", request.BackendExecutable, "plugin", "list", "--json")
	if err != nil {
		return fmt.Errorf("verify Codex plugin listing: %w", err)
	}
	switch codexPluginStatus(listed.Stdout, request.DeclaredName, marketplace) {
	case codexStatusInstalled:
		return nil
	case codexStatusAbsent:
		return fmt.Errorf("%w: verify Codex plugin listing: %s is not installed and enabled", errRecognizedNegativeEvidence, pluginSpec)
	default:
		return fmt.Errorf("%w: verify Codex plugin listing", errCodexListContractUnknown)
	}
}

func (activator Activator) activateKiroMCP(ctx context.Context, request domain.ActivationRequest) error {
	config := filepath.Join(request.Delivery.ActivePath, "mcp.json")
	if _, err := activator.runClientResult(ctx, "Kiro CLI", request.BackendExecutable, "mcp", "import", "--file", config, "global", "--force"); err != nil {
		return fmt.Errorf("import Kiro MCP configuration: %w", err)
	}
	return activator.verifyKiroMCP(ctx, request)
}

func (activator Activator) verifyKiroMCP(ctx context.Context, request domain.ActivationRequest) error {
	var firstCommandErr error
	var firstUnknown string
	var firstUnhealthy string
	for _, component := range request.Plan.Components {
		if component.Kind != domain.ComponentMCPServer {
			continue
		}
		status, err := activator.runClientResult(ctx, "Kiro CLI", request.BackendExecutable, "mcp", "status", "--name", component.Name)
		if err != nil {
			if firstCommandErr == nil {
				firstCommandErr = fmt.Errorf("verify Kiro MCP server %s: %w", component.Name, err)
			}
			continue
		}
		switch kiroMCPStatus(status.Stdout, component.Name) {
		case kiroStatusHealthy:
			continue
		case kiroStatusUnhealthy:
			if firstUnhealthy == "" {
				firstUnhealthy = component.Name
			}
		default:
			if firstUnknown == "" {
				firstUnknown = component.Name
			}
		}
	}
	if firstUnhealthy != "" {
		return fmt.Errorf("%w: verify Kiro MCP server %s: server is not connected", errRecognizedNegativeEvidence, firstUnhealthy)
	}
	if firstCommandErr != nil {
		return firstCommandErr
	}
	if firstUnknown != "" {
		return fmt.Errorf("%w for server %s", errKiroStatusContractUnknown, firstUnknown)
	}
	return nil
}

func (activator Activator) deactivateCopilot(ctx context.Context, request domain.DeactivationRequest) error {
	if strings.TrimSpace(request.PhysicalArtifactID) == "" {
		return fmt.Errorf("managed Copilot marketplace identity is missing")
	}
	marketplace := managedMarketplaceName(request.PhysicalArtifactID)
	uninstall, err := activator.runCopilotResult(ctx, request.BackendExecutable, "plugin", "uninstall", request.DeclaredName+"@"+marketplace)
	if err != nil && !commandOutputContains(uninstall, "is not installed") {
		return err
	}
	remove, err := activator.runCopilotResult(ctx, request.BackendExecutable, "plugin", "marketplace", "remove", marketplace)
	if err != nil && !commandOutputContains(remove, "is not registered") {
		return err
	}
	return nil
}

func (activator Activator) runCopilot(ctx context.Context, executable string, args ...string) error {
	_, err := activator.runCopilotResult(ctx, executable, args...)
	return err
}

func (activator Activator) runCopilotResult(ctx context.Context, executable string, args ...string) (legacyports.CommandResult, error) {
	return activator.runClientResult(ctx, "GitHub Copilot CLI", executable, args...)
}

func (activator Activator) runClientResult(ctx context.Context, client, executable string, args ...string) (legacyports.CommandResult, error) {
	if activator.Runner == nil {
		return legacyports.CommandResult{}, fmt.Errorf("%s runner is unavailable", client)
	}
	result, err := activator.Runner.Run(ctx, legacyports.Command{Argv: append([]string{executable}, args...)})
	if err != nil {
		return result, fmt.Errorf("start %s: %w", client, err)
	}
	if result.ExitCode != 0 {
		return result, fmt.Errorf("%s command failed with exit code %d", client, result.ExitCode)
	}
	return result, nil
}

func failedActivation(outcome domain.ActivationOutcome, next string, err error) (domain.ActivationOutcome, error) {
	outcome.Activation = domain.ActivationFailed
	outcome.Verification = domain.VerificationFailed
	outcome.AuthoritativeObservation = errors.Is(err, errRecognizedNegativeEvidence)
	outcome.UserActions = []string{"retry client activation and verify client visibility"}
	outcome.LocalActions = []string{next}
	return outcome, err
}

func mcpOnly(components []domain.ComponentDecision) bool {
	found := false
	for _, component := range components {
		if component.Support == domain.SupportUnsupported || component.Kind != domain.ComponentMCPServer {
			return false
		}
		found = true
	}
	return found
}

func isKiroCLI(executable string) bool {
	base := strings.ToLower(filepath.Base(strings.TrimSpace(executable)))
	return base == "kiro-cli" || base == "kiro-cli.exe" || base == "kiro" || base == "kiro.exe"
}

type codexStatus int

const (
	codexStatusUnknown codexStatus = iota
	codexStatusInstalled
	codexStatusAbsent
)

var errCodexListContractUnknown = errors.New("Codex plugin list output is not recognized")

func codexPluginStatus(body []byte, name, marketplace string) codexStatus {
	if len(body) == 0 {
		return codexStatusUnknown
	}
	decoder := json.NewDecoder(strings.NewReader(string(body)))
	decoder.UseNumber()
	parsed, err := decodeUniqueJSONValue(decoder)
	if err != nil {
		return codexStatusUnknown
	}
	if _, tokenErr := decoder.Token(); tokenErr == nil || !errors.Is(tokenErr, io.EOF) {
		return codexStatusUnknown
	}
	document, ok := parsed.(map[string]any)
	if !ok {
		return codexStatusUnknown
	}
	installedValue, ok := document["installed"]
	if !ok {
		return codexStatusUnknown
	}
	entries, ok := installedValue.([]any)
	if !ok {
		return codexStatusUnknown
	}
	expectedID := name + "@" + marketplace
	identities := make(map[string]struct{}, len(entries))
	foundExpected := false
	expectedActive := false
	required := []string{"pluginId", "name", "marketplaceName", "installed", "enabled"}
	for _, value := range entries {
		entry, ok := value.(map[string]any)
		if !ok || len(entry) != len(required) {
			return codexStatusUnknown
		}
		for _, field := range required {
			if _, present := entry[field]; !present {
				return codexStatusUnknown
			}
		}
		pluginID, pluginIDOK := entry["pluginId"].(string)
		entryName, nameOK := entry["name"].(string)
		marketplaceName, marketplaceOK := entry["marketplaceName"].(string)
		installed, installedOK := entry["installed"].(bool)
		enabled, enabledOK := entry["enabled"].(bool)
		if !pluginIDOK || !nameOK || !marketplaceOK || !installedOK || !enabledOK ||
			pluginID == "" || entryName == "" || marketplaceName == "" ||
			pluginID != entryName+"@"+marketplaceName {
			return codexStatusUnknown
		}
		if _, duplicate := identities[pluginID]; duplicate {
			return codexStatusUnknown
		}
		identities[pluginID] = struct{}{}
		if pluginID == expectedID {
			foundExpected = true
			expectedActive = installed && enabled
		}
	}
	if foundExpected && expectedActive {
		return codexStatusInstalled
	}
	return codexStatusAbsent
}

func decodeUniqueJSONValue(decoder *json.Decoder) (any, error) {
	token, err := decoder.Token()
	if err != nil {
		return nil, err
	}
	delimiter, isDelimiter := token.(json.Delim)
	if !isDelimiter {
		return token, nil
	}
	switch delimiter {
	case '{':
		object := make(map[string]any)
		var keys []string
		for decoder.More() {
			keyToken, keyErr := decoder.Token()
			if keyErr != nil {
				return nil, keyErr
			}
			key, ok := keyToken.(string)
			if !ok {
				return nil, fmt.Errorf("JSON object key is not a string")
			}
			for _, prior := range keys {
				if strings.EqualFold(prior, key) {
					return nil, fmt.Errorf("duplicate or case-ambiguous JSON object key %q", key)
				}
			}
			keys = append(keys, key)
			value, valueErr := decodeUniqueJSONValue(decoder)
			if valueErr != nil {
				return nil, valueErr
			}
			object[key] = value
		}
		closing, closingErr := decoder.Token()
		if closingErr != nil || closing != json.Delim('}') {
			return nil, fmt.Errorf("invalid JSON object")
		}
		return object, nil
	case '[':
		var array []any
		for decoder.More() {
			value, valueErr := decodeUniqueJSONValue(decoder)
			if valueErr != nil {
				return nil, valueErr
			}
			array = append(array, value)
		}
		closing, closingErr := decoder.Token()
		if closingErr != nil || closing != json.Delim(']') {
			return nil, fmt.Errorf("invalid JSON array")
		}
		return array, nil
	default:
		return nil, fmt.Errorf("unexpected JSON delimiter %q", delimiter)
	}
}

var copilotInstalledEntry = regexp.MustCompile(`^[ \t]+•[ \t]+([A-Za-z0-9][A-Za-z0-9._-]*@[A-Za-z0-9][A-Za-z0-9._-]*)[ \t]+\(v[0-9]+\.[0-9]+\.[0-9]+(?:[-+][0-9A-Za-z.-]+)?\)[ \t]*$`)

type copilotStatus int

const (
	copilotStatusUnknown copilotStatus = iota
	copilotStatusInstalled
	copilotStatusAbsent
)

var errCopilotListContractUnknown = errors.New("Copilot plugin list output is not recognized")
var errRecognizedNegativeEvidence = errors.New("recognized negative client evidence")

func copilotPluginStatus(stdout []byte, expected string) copilotStatus {
	inInstalledSection := false
	recognizedSection := false
	recognizedEntry := false
	matches := 0
	for _, rawLine := range strings.Split(strings.ReplaceAll(string(stdout), "\r\n", "\n"), "\n") {
		if strings.TrimSpace(rawLine) == "Installed plugins:" {
			if inInstalledSection {
				return copilotStatusUnknown
			}
			inInstalledSection = true
			recognizedSection = true
			continue
		}
		if !inInstalledSection {
			continue
		}
		if rawLine != "" && rawLine[0] != ' ' && rawLine[0] != '\t' {
			inInstalledSection = false
			continue
		}
		entry := copilotInstalledEntry.FindStringSubmatch(rawLine)
		if len(entry) == 2 {
			recognizedEntry = true
			if entry[1] == expected {
				matches++
			}
			continue
		}
		trimmed := strings.TrimSpace(rawLine)
		if trimmed == "" {
			continue
		}
		if trimmed == "No plugins installed." || trimmed == "No plugins installed" {
			recognizedEntry = true
			continue
		}
		lower := strings.ToLower(trimmed)
		if strings.Contains(trimmed, expected) {
			for _, state := range []string{"pending", "disconnected", "disabled", "auth-required", "auth required", "authentication required", "error", "failed", "failure"} {
				if strings.Contains(lower, state) {
					return copilotStatusAbsent
				}
			}
			return copilotStatusUnknown
		}
	}
	if matches == 1 {
		return copilotStatusInstalled
	}
	if matches > 1 {
		return copilotStatusUnknown
	}
	if recognizedSection && recognizedEntry {
		return copilotStatusAbsent
	}
	return copilotStatusUnknown
}

func activationObservable(request domain.ActivationRequest, runnerAvailable bool) bool {
	if !runnerAvailable || strings.TrimSpace(request.BackendExecutable) == "" {
		return false
	}
	switch request.Client.ClientID {
	case domain.ClientCodex, domain.ClientCopilot, domain.ClientVSCode:
		return true
	case domain.ClientKiro:
		return mcpOnly(request.Plan.Components) && isKiroCLI(request.BackendExecutable)
	default:
		return false
	}
}

type kiroStatus int

const (
	kiroStatusUnknown kiroStatus = iota
	kiroStatusHealthy
	kiroStatusUnhealthy
)

var errKiroStatusContractUnknown = errors.New("Kiro MCP status output is not recognized")

func kiroMCPStatus(stdout []byte, expected string) kiroStatus {
	contract := strings.ReplaceAll(string(stdout), "\r\n", "\n")
	contract = strings.TrimSuffix(contract, "\n")
	status := ""
	if prefix := expected + ": "; strings.HasPrefix(contract, prefix) && !strings.Contains(contract, "\n") {
		status = strings.TrimPrefix(contract, prefix)
	} else {
		lines := strings.Split(contract, "\n")
		if len(lines) != 2 || lines[0] != "Name: "+expected || !strings.HasPrefix(lines[1], "Status: ") {
			return kiroStatusUnknown
		}
		status = strings.TrimPrefix(lines[1], "Status: ")
	}
	if status == "connected" {
		return kiroStatusHealthy
	}
	for _, negative := range []string{"pending", "disconnected", "disabled", "auth-required", "auth required", "authentication required", "error", "failed", "failure"} {
		if status == negative {
			return kiroStatusUnhealthy
		}
	}
	return kiroStatusUnknown
}

func attestedUnknownVerification(outcome domain.ActivationOutcome, request domain.ActivationRequest) (domain.ActivationOutcome, bool) {
	if !request.ActivationComplete {
		return outcome, false
	}
	outcome.Activation = domain.ActivationActive
	outcome.Verification = domain.VerificationInstalled
	outcome.ActivationAttested = true
	return outcome, true
}

func manualCodexVerification(outcome domain.ActivationOutcome, request domain.ActivationRequest) domain.ActivationOutcome {
	if attested, ok := attestedUnknownVerification(outcome, request); ok {
		return attested
	}
	outcome.Activation = domain.ActivationManual
	outcome.UserActions = []string{"confirm the managed plugin is installed and enabled in Codex"}
	outcome.LocalActions = []string{fmt.Sprintf("the `%s plugin list --json` output contract was not recognized; inspect %s manually", request.BackendExecutable, request.DeclaredName)}
	return outcome
}

func manualKiroVerification(outcome domain.ActivationOutcome, request domain.ActivationRequest) domain.ActivationOutcome {
	if attested, ok := attestedUnknownVerification(outcome, request); ok {
		return attested
	}
	outcome.Activation = domain.ActivationManual
	outcome.UserActions = []string{"confirm each imported MCP server is connected in Kiro"}
	outcome.LocalActions = []string{fmt.Sprintf("Kiro's documented `%s mcp status --name <server>` output contract was not recognized; inspect each server manually", request.BackendExecutable)}
	return outcome
}

func manualCopilotVerification(outcome domain.ActivationOutcome, request domain.ActivationRequest) domain.ActivationOutcome {
	if attested, ok := attestedUnknownVerification(outcome, request); ok {
		return attested
	}
	outcome.Activation = domain.ActivationManual
	outcome.UserActions = []string{"confirm the managed plugin is installed and enabled in GitHub Copilot"}
	outcome.LocalActions = []string{fmt.Sprintf("the `%s plugin list` output contract was not recognized; inspect %s manually", request.BackendExecutable, request.DeclaredName)}
	return outcome
}

func commandOutputContains(result legacyports.CommandResult, fragment string) bool {
	output := string(result.Stdout) + "\n" + string(result.Stderr)
	return strings.Contains(strings.ToLower(output), strings.ToLower(fragment))
}
