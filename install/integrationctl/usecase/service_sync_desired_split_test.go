package usecase

import (
	"errors"
	"testing"

	"github.com/777genius/plugin-kit-ai/install/integrationctl/domain"
)

func TestClassifyDesiredSyncActionReturnsUpdateForVersionDrift(t *testing.T) {
	t.Parallel()

	action := classifyDesiredSyncAction(domain.InstallationRecord{
		RequestedSourceRef: domain.RequestedSourceRef{Value: "/tmp/demo"},
		ResolvedVersion:    "0.1.0",
		Policy:             domain.InstallPolicy{Scope: "project"},
		Targets: map[domain.TargetID]domain.TargetInstallation{
			domain.TargetCursor: {TargetID: domain.TargetCursor},
		},
	}, true, "/tmp/demo", domain.InstallPolicy{Scope: "project"}, []domain.TargetID{domain.TargetCursor}, "0.2.0")

	if action != desiredSyncActionUpdate {
		t.Fatalf("action = %q", action)
	}
}

func TestSyncDesiredWarningHelpersPreserveContractText(t *testing.T) {
	t.Parallel()

	err := errors.New("boom")
	if got := syncDesiredSourceWarning("./plugins/demo", err); got != "Sync skipped for source ./plugins/demo: boom" {
		t.Fatalf("source warning = %q", got)
	}
	if got := syncDesiredManifestWarning("demo", err); got != "Sync skipped for demo: boom" {
		t.Fatalf("manifest warning = %q", got)
	}
	if got := syncDesiredNoopWarning("demo"); got != "Sync no-op for demo: desired state already matches workspace intent" {
		t.Fatalf("noop warning = %q", got)
	}
}
