package statev2

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/777genius/plugin-kit-ai/install/integrationctl/adapters/atomicfile"
	"github.com/777genius/plugin-kit-ai/install/integrationctl/adapters/pathpolicy"
	"github.com/777genius/plugin-kit-ai/install/integrationctl/agentplugins/domain"
)

type Store struct {
	Path string
}

func (store Store) Load() (domain.StateFileV2, error) {
	if strings.TrimSpace(store.Path) == "" {
		return domain.StateFileV2{}, fmt.Errorf("state v2 path is required")
	}
	body, err := os.ReadFile(store.Path)
	if os.IsNotExist(err) {
		return domain.StateFileV2{SchemaVersion: domain.StateSchemaVersion}, nil
	}
	if err != nil {
		return domain.StateFileV2{}, fmt.Errorf("read state v2: %w", err)
	}
	var header struct {
		SchemaVersion int `json:"schema_version"`
	}
	if err := json.Unmarshal(body, &header); err != nil {
		return domain.StateFileV2{}, fmt.Errorf("decode state v2 header: %w", err)
	}
	var state domain.StateFileV2
	switch header.SchemaVersion {
	case domain.LegacyStateSchemaVersion:
		state, err = decodeLegacyStateV2(body)
	case domain.StateSchemaVersion:
		err = decodeStrictJSON(body, &state)
	default:
		return domain.StateFileV2{}, fmt.Errorf("unsupported state schema_version %d; this build reads %d and %d and writes only %d", header.SchemaVersion, domain.LegacyStateSchemaVersion, domain.StateSchemaVersion, domain.StateSchemaVersion)
	}
	if err != nil {
		return domain.StateFileV2{}, err
	}
	if err := Validate(state); err != nil {
		return domain.StateFileV2{}, fmt.Errorf("validate state: %w", err)
	}
	return state, nil
}

func (store Store) Save(state domain.StateFileV2) error {
	if strings.TrimSpace(store.Path) == "" {
		return fmt.Errorf("state v2 path is required")
	}
	if state.SchemaVersion == 0 {
		state.SchemaVersion = domain.StateSchemaVersion
	}
	if err := Validate(state); err != nil {
		return fmt.Errorf("refuse invalid state: %w", err)
	}
	state.Installations = append([]domain.Installation(nil), state.Installations...)
	sort.Slice(state.Installations, func(i, j int) bool {
		return state.Installations[i].InstallationID < state.Installations[j].InstallationID
	})
	body, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	body = append(body, '\n')
	parent := filepath.Dir(store.Path)
	if err := os.MkdirAll(parent, 0o700); err != nil {
		return fmt.Errorf("create state v2 directory: %w", err)
	}
	if err := os.Chmod(parent, 0o700); err != nil {
		return fmt.Errorf("protect state v2 directory: %w", err)
	}
	return atomicfile.Write(store.Path, body, 0o600)
}

func decodeStrictJSON(body []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("decode state: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return fmt.Errorf("state contains trailing JSON values")
	}
	return nil
}

func Validate(state domain.StateFileV2) error {
	if state.SchemaVersion != domain.StateSchemaVersion {
		return fmt.Errorf("schema_version must be %d", domain.StateSchemaVersion)
	}
	installationIDs := map[string]struct{}{}
	sourceBindingIDs := map[string]struct{}{}
	clientBindingIDs := map[string]struct{}{}
	operationIDs := map[string]struct{}{}
	for index, installation := range state.Installations {
		prefix := fmt.Sprintf("installations[%d]", index)
		if err := pathpolicy.ValidateLeafID(installation.InstallationID); err != nil {
			return fmt.Errorf("%s has invalid installation_id: %w", prefix, err)
		}
		if _, duplicate := installationIDs[installation.InstallationID]; duplicate {
			return fmt.Errorf("duplicate installation_id %q", installation.InstallationID)
		}
		installationIDs[installation.InstallationID] = struct{}{}
		if strings.TrimSpace(installation.DeclaredName) == "" || installation.DeclaredName != installation.Package.DeclaredName {
			return fmt.Errorf("%s declared_name does not match package binding", prefix)
		}
		if strings.TrimSpace(installation.Source.SourceBindingID) == "" {
			return fmt.Errorf("%s source_binding_id is required", prefix)
		}
		if _, duplicate := sourceBindingIDs[installation.Source.SourceBindingID]; duplicate {
			return fmt.Errorf("duplicate source_binding_id %q", installation.Source.SourceBindingID)
		}
		sourceBindingIDs[installation.Source.SourceBindingID] = struct{}{}
		if installation.Package.LoaderKind == domain.LoaderKindAgentPlugins {
			if installation.Package.FormatID == "" || installation.Source.TreeDigest == "" || installation.Package.ManifestDigest == "" {
				return fmt.Errorf("%s standard package binding is incomplete", prefix)
			}
			if installation.Package.FormatID == domain.FormatIDAgentPluginsV1 && installation.Package.SchemaURI == "" {
				return fmt.Errorf("%s portable standard package binding has no schema URI", prefix)
			}
		}
		for mapKey, client := range installation.Clients {
			if mapKey == "" || mapKey != client.ClientBindingID {
				return fmt.Errorf("%s client map key does not match client_binding_id", prefix)
			}
			if err := pathpolicy.ValidateLeafID(client.ClientBindingID); err != nil {
				return fmt.Errorf("%s has invalid client_binding_id: %w", prefix, err)
			}
			if _, duplicate := clientBindingIDs[client.ClientBindingID]; duplicate {
				return fmt.Errorf("duplicate client_binding_id %q", client.ClientBindingID)
			}
			clientBindingIDs[client.ClientBindingID] = struct{}{}
			if strings.TrimSpace(client.ClientID) == "" || strings.TrimSpace(client.TargetLocator) == "" {
				return fmt.Errorf("%s client binding %q is incomplete", prefix, client.ClientBindingID)
			}
			if err := pathpolicy.ValidateLeafID(client.PhysicalArtifact); err != nil {
				return fmt.Errorf("%s client binding %q has invalid physical artifact id: %w", prefix, client.ClientBindingID, err)
			}
			if !validMaterializationState(client.Materialization) ||
				!validActivationState(client.Activation) ||
				!validAuthenticationState(client.Authentication) ||
				!validPolicyState(client.Policy) ||
				!validVerificationState(client.Verification) {
				return fmt.Errorf("%s client binding %q contains an unknown lifecycle state", prefix, client.ClientBindingID)
			}
			if client.PackageRevision != nil && (strings.TrimSpace(client.PackageRevision.TreeDigest) == "" || strings.TrimSpace(client.PackageRevision.ManifestDigest) == "") {
				return fmt.Errorf("%s client binding %q has an incomplete package revision", prefix, client.ClientBindingID)
			}
			objectIDs := map[string]struct{}{}
			for _, object := range client.NativeObjects {
				if strings.TrimSpace(object.ObjectID) == "" || strings.TrimSpace(object.Kind) == "" {
					return fmt.Errorf("%s client binding %q has incomplete native ownership", prefix, client.ClientBindingID)
				}
				if _, duplicate := objectIDs[object.ObjectID]; duplicate {
					return fmt.Errorf("%s client binding %q has duplicate object_id %q", prefix, client.ClientBindingID, object.ObjectID)
				}
				objectIDs[object.ObjectID] = struct{}{}
			}
			for receiptIndex, receipt := range client.Receipts {
				if err := pathpolicy.ValidateLeafID(receipt.OperationID); err != nil {
					return fmt.Errorf("%s client binding %q receipt[%d] has invalid operation id: %w", prefix, client.ClientBindingID, receiptIndex, err)
				}
				if receipt.ClientBindingID != client.ClientBindingID || receipt.Sequence < 1 || receipt.Phase == "" {
					return fmt.Errorf("%s client binding %q receipt[%d] is incomplete", prefix, client.ClientBindingID, receiptIndex)
				}
				if _, duplicate := operationIDs[receipt.OperationID]; duplicate {
					return fmt.Errorf("duplicate receipt operation_id %q", receipt.OperationID)
				}
				operationIDs[receipt.OperationID] = struct{}{}
			}
		}
	}
	return nil
}

func validMaterializationState(value domain.MaterializationState) bool {
	switch value {
	case domain.MaterializationAbsent,
		domain.MaterializationStaged,
		domain.MaterializationMaterialized,
		domain.MaterializationDegraded:
		return true
	default:
		return false
	}
}

func validActivationState(value domain.ActivationState) bool {
	switch value {
	case domain.ActivationNotRequired,
		domain.ActivationPrepared,
		domain.ActivationManual,
		domain.ActivationActive,
		domain.ActivationFailed:
		return true
	default:
		return false
	}
}

func validAuthenticationState(value domain.AuthenticationState) bool {
	switch value {
	case domain.AuthenticationNotRequired,
		domain.AuthenticationNotChecked,
		domain.AuthenticationPending,
		domain.AuthenticationComplete,
		domain.AuthenticationFailed:
		return true
	default:
		return false
	}
}

func validPolicyState(value domain.PolicyState) bool {
	switch value {
	case domain.PolicyAllowed,
		domain.PolicyBlocked,
		domain.PolicyApprovalRequired:
		return true
	default:
		return false
	}
}

func validVerificationState(value domain.VerificationState) bool {
	switch value {
	case domain.VerificationNotRun,
		domain.VerificationPackageValid,
		domain.VerificationInstalled,
		domain.VerificationRuntime,
		domain.VerificationFailed:
		return true
	default:
		return false
	}
}
