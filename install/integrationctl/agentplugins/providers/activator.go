package providers

import (
	"context"
	"fmt"
	"os"
	"runtime"
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
	switch request.Client.ClientID {
	case domain.ClientCursor:
		outcome.Activation = domain.ActivationManual
		outcome.UserActions = append(outcome.UserActions, "reload Cursor, then verify the plugin appears before using its components")
		return outcome, nil
	case domain.ClientCodex:
		outcome.Activation = domain.ActivationManual
		outcome.UserActions = append(outcome.UserActions, "finish installation in Codex or ChatGPT Plugins, then start a new session")
		marketplace := managedMarketplaceName(request.Plan.PhysicalArtifactID)
		outcome.LocalActions = append(outcome.LocalActions, fmt.Sprintf(
			"Codex CLI: run `codex plugin marketplace add %s --json`, then `codex plugin add %s --json`; ChatGPT/Codex app: open Plugins > Personal and install %s",
			shellQuoteForHint(request.Delivery.ActivePath), request.DeclaredName+"@"+marketplace, request.DeclaredName,
		))
		return outcome, nil
	case domain.ClientKiro:
		outcome.Activation = domain.ActivationManual
		outcome.UserActions = append(outcome.UserActions, "finish the prepared Power installation in Kiro")
		outcome.LocalActions = append(outcome.LocalActions, fmt.Sprintf(
			"Kiro: Powers > Add Custom Power > Import power from a folder > select %s > Install",
			request.Delivery.ActivePath,
		))
		return outcome, nil
	case domain.ClientCopilot, domain.ClientVSCode:
		if strings.TrimSpace(request.BackendExecutable) == "" || activator.Runner == nil {
			outcome.Activation = domain.ActivationManual
			if request.Client.ClientID == domain.ClientVSCode {
				outcome.UserActions = append(outcome.UserActions, "register the prepared local plugin in VS Code")
				outcome.LocalActions = append(outcome.LocalActions, fmt.Sprintf(
					"VS Code: add %q to the `chat.pluginLocations` setting with value `true`, then reload VS Code",
					request.Delivery.ActivePath,
				))
			} else {
				outcome.UserActions = append(outcome.UserActions, "install GitHub Copilot CLI, then rerun `agentplugins update` for this plugin")
			}
			return outcome, nil
		}
		if err := activator.activateCopilot(ctx, request); err != nil {
			return outcome, err
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
	if request.Replacing {
		if err := activator.runCopilot(ctx, request.BackendExecutable, "plugin", "marketplace", "update", marketplace); err != nil {
			if fallbackErr := activator.runCopilot(ctx, request.BackendExecutable, "plugin", "marketplace", "add", request.Delivery.ActivePath); fallbackErr != nil {
				return fmt.Errorf("refresh managed Copilot marketplace: %v; fallback registration: %w", err, fallbackErr)
			}
		}
		if err := activator.runCopilot(ctx, request.BackendExecutable, "plugin", "update", request.DeclaredName+"@"+marketplace); err == nil {
			return nil
		}
	} else if err := activator.runCopilot(ctx, request.BackendExecutable, "plugin", "marketplace", "add", request.Delivery.ActivePath); err != nil {
		if fallbackErr := activator.runCopilot(ctx, request.BackendExecutable, "plugin", "marketplace", "update", marketplace); fallbackErr != nil {
			return fmt.Errorf("register managed Copilot marketplace: %v; fallback refresh: %w", err, fallbackErr)
		}
	}
	pluginSpec := request.DeclaredName + "@" + marketplace
	if err := activator.runCopilot(ctx, request.BackendExecutable, "plugin", "install", pluginSpec); err != nil {
		if !request.Replacing {
			_ = activator.runCopilot(ctx, request.BackendExecutable, "plugin", "marketplace", "remove", marketplace)
		}
		return err
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
	if activator.Runner == nil {
		return legacyports.CommandResult{}, fmt.Errorf("GitHub Copilot CLI runner is unavailable")
	}
	result, err := activator.Runner.Run(ctx, legacyports.Command{Argv: append([]string{executable}, args...)})
	if err != nil {
		return result, fmt.Errorf("start GitHub Copilot CLI: %w", err)
	}
	if result.ExitCode != 0 {
		return result, fmt.Errorf("GitHub Copilot CLI command failed with exit code %d", result.ExitCode)
	}
	return result, nil
}

func commandOutputContains(result legacyports.CommandResult, fragment string) bool {
	output := string(result.Stdout) + "\n" + string(result.Stderr)
	return strings.Contains(strings.ToLower(output), strings.ToLower(fragment))
}

func shellQuoteForHint(value string) string {
	if runtime.GOOS == "windows" {
		return "'" + strings.ReplaceAll(value, "'", "''") + "'"
	}
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}
