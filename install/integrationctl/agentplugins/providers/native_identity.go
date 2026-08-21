package providers

import (
	"context"
	"errors"
	"os"

	"github.com/777genius/plugin-kit-ai/install/integrationctl/agentplugins/domain"
	"github.com/777genius/plugin-kit-ai/install/integrationctl/agentplugins/ports"
)

// NativeIdentityObserver fails closed on a pre-existing package directory.
// The backend does not infer ownership from matching bytes: state plus the
// recorded managed digest are both required, so automatic adoption is
// impossible.
type NativeIdentityObserver struct{ Stager Stager }

func (observer NativeIdentityObserver) ObserveNativeIdentity(ctx context.Context, _ domain.DetectedClient, plan domain.DeliveryPlan, managed *domain.ClientBinding) (domain.NativeIdentityObservation, error) {
	if err := ctx.Err(); err != nil {
		return domain.NativeIdentityObservation{}, err
	}
	_, statErr := os.Lstat(plan.ActivePath)
	if os.IsNotExist(statErr) {
		return domain.NativeIdentityObservation{State: domain.NativeIdentityAbsent}, nil
	}
	if statErr != nil {
		return domain.NativeIdentityObservation{State: domain.NativeIdentityIndeterminate}, statErr
	}
	if managed == nil {
		return domain.NativeIdentityObservation{State: domain.NativeIdentityUnmanaged}, nil
	}
	expected := managedPackageDigest(*managed)
	if expected == "" {
		return domain.NativeIdentityObservation{State: domain.NativeIdentityIndeterminate}, nil
	}
	if err := observer.Stager.Verify(ctx, plan.ActivePath, expected); err != nil {
		var verification *ports.VerificationError
		if errors.As(err, &verification) && verification.Kind == ports.VerificationDigestMismatch {
			return domain.NativeIdentityObservation{State: domain.NativeIdentityIndeterminate, Digest: verification.ActualDigest}, nil
		}
		return domain.NativeIdentityObservation{State: domain.NativeIdentityIndeterminate}, err
	}
	return domain.NativeIdentityObservation{State: domain.NativeIdentityManaged, Digest: expected}, nil
}

func managedPackageDigest(client domain.ClientBinding) string {
	for _, object := range client.NativeObjects {
		if object.Kind == "managed_package_directory" {
			return object.ManagedDigest
		}
	}
	return ""
}
