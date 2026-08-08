package providers

import (
	"context"
	"fmt"
	"os"

	"github.com/777genius/plugin-kit-ai/install/integrationctl/adapters/pathpolicy"
	"github.com/777genius/plugin-kit-ai/install/integrationctl/agentplugins/domain"
)

type Activator struct{}

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
	case domain.ClientCopilot:
		return requireExternalUninstall(outcome, request.ExternalUninstalled, fmt.Sprintf("run `copilot plugin uninstall %s` in a terminal, then rerun remove with `--external-uninstalled` (also use the flag if it was never installed in the client)", request.DeclaredName)), nil
	case domain.ClientVSCode:
		return requireExternalUninstall(outcome, request.ExternalUninstalled, "remove the plugin through VS Code's Agent Plugins UI, then rerun remove with `--external-uninstalled` (also use the flag if it was never imported)"), nil
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
		outcome.UserActions = append(outcome.UserActions, "install the prepared marketplace plugin in Codex or ChatGPT")
		return outcome, nil
	case domain.ClientKiro:
		outcome.Activation = domain.ActivationManual
		outcome.UserActions = append(outcome.UserActions, "import the prepared folder with Kiro: Powers > Add Custom Power > Import from folder")
		return outcome, nil
	case domain.ClientCopilot:
		outcome.Activation = domain.ActivationManual
		outcome.UserActions = append(outcome.UserActions, fmt.Sprintf("run `copilot plugin install %s` in a terminal and review the client trust prompt", request.Delivery.ActivePath))
		return outcome, nil
	case domain.ClientVSCode:
		outcome.Activation = domain.ActivationManual
		outcome.UserActions = append(outcome.UserActions, fmt.Sprintf("open VS Code's Agent Plugins UI, import %s, then verify the plugin appears", request.Delivery.ActivePath))
		return outcome, nil
	default:
		return domain.ActivationOutcome{}, fmt.Errorf("unsupported activation client %q", request.Client.ClientID)
	}
}
