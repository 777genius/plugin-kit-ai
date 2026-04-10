package usecase

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/777genius/plugin-kit-ai/install/integrationctl/domain"
	"github.com/777genius/plugin-kit-ai/install/integrationctl/ports"
)

func (s Service) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now().UTC()
}

func resolveRequestedTargets(manifest domain.IntegrationManifest, requested []string) ([]domain.TargetID, error) {
	if len(requested) == 0 {
		targets := make([]domain.TargetID, 0, len(manifest.Deliveries))
		for _, delivery := range manifest.Deliveries {
			targets = append(targets, delivery.TargetID)
		}
		return targets, nil
	}
	out := make([]domain.TargetID, 0, len(requested))
	available := map[domain.TargetID]struct{}{}
	for _, delivery := range manifest.Deliveries {
		available[delivery.TargetID] = struct{}{}
	}
	for _, raw := range requested {
		target := domain.TargetID(strings.ToLower(strings.TrimSpace(raw)))
		if _, ok := available[target]; !ok {
			return nil, domain.NewError(domain.ErrUnsupportedTarget, "manifest does not expose target "+string(target), nil)
		}
		out = append(out, target)
	}
	return out, nil
}

func findDelivery(items []domain.Delivery, target domain.TargetID) *domain.Delivery {
	for i := range items {
		if items[i].TargetID == target {
			return &items[i]
		}
	}
	return nil
}

func findInstallation(items []domain.InstallationRecord, name string) (domain.InstallationRecord, bool) {
	name = strings.TrimSpace(name)
	for _, item := range items {
		if item.IntegrationID == name {
			return item, true
		}
	}
	return domain.InstallationRecord{}, false
}

func findInstallationMutable(items []domain.InstallationRecord, name string) (domain.InstallationRecord, bool) {
	return findInstallation(items, name)
}

func sortedTargets(m map[domain.TargetID]domain.TargetInstallation) []domain.TargetID {
	out := make([]domain.TargetID, 0, len(m))
	for id := range m {
		out = append(out, id)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func upsertInstallation(items []domain.InstallationRecord, next domain.InstallationRecord) []domain.InstallationRecord {
	for i := range items {
		if items[i].IntegrationID == next.IntegrationID {
			items[i] = next
			return items
		}
	}
	return append(items, next)
}

func removeInstallation(items []domain.InstallationRecord, name string) []domain.InstallationRecord {
	out := items[:0]
	for _, item := range items {
		if item.IntegrationID != name {
			out = append(out, item)
		}
	}
	return out
}

func cloneMetadata(in map[string]any) map[string]any {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]any, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

func cloneInstallationRecord(in domain.InstallationRecord) domain.InstallationRecord {
	out := in
	if len(in.Targets) == 0 {
		out.Targets = map[domain.TargetID]domain.TargetInstallation{}
		return out
	}
	out.Targets = make(map[domain.TargetID]domain.TargetInstallation, len(in.Targets))
	for key, value := range in.Targets {
		out.Targets[key] = cloneTargetInstallation(value)
	}
	return out
}

func cloneTargetInstallation(in domain.TargetInstallation) domain.TargetInstallation {
	out := in
	out.CapabilitySurface = append([]string(nil), in.CapabilitySurface...)
	out.CatalogPolicy = cloneCatalogPolicy(in.CatalogPolicy)
	out.EnvironmentRestrictions = append([]domain.EnvironmentRestrictionCode(nil), in.EnvironmentRestrictions...)
	out.OwnedNativeObjects = append([]domain.NativeObjectRef(nil), in.OwnedNativeObjects...)
	out.AdapterMetadata = cloneMetadata(in.AdapterMetadata)
	return out
}

func cloneCatalogPolicy(in *domain.CatalogPolicySnapshot) *domain.CatalogPolicySnapshot {
	if in == nil {
		return nil
	}
	out := *in
	return &out
}

func firstNonNilCatalogPolicy(values ...*domain.CatalogPolicySnapshot) *domain.CatalogPolicySnapshot {
	for _, value := range values {
		if value != nil {
			return value
		}
	}
	return nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func defaultString(v, fallback string) string {
	if strings.TrimSpace(v) == "" {
		return fallback
	}
	return strings.TrimSpace(v)
}

func defaultBool(v *bool, fallback bool) bool {
	if v == nil {
		return fallback
	}
	return *v
}

func operationID(prefix, integrationID string, t time.Time) string {
	return fmt.Sprintf("%s_%s_%d", prefix, sanitizeID(integrationID), t.Unix())
}

func cleanupResolvedSource(source ports.ResolvedSource) {
	if strings.TrimSpace(source.CleanupPath) == "" {
		return
	}
	_ = os.RemoveAll(source.CleanupPath)
}

func cleanupPlannedExisting(items []plannedExistingTarget) {
	for _, item := range items {
		if item.Resolved != nil {
			cleanupResolvedSource(*item.Resolved)
		}
	}
}

func desiredPolicyFromLock(in domain.InstallPolicy) domain.InstallPolicy {
	return domain.InstallPolicy{
		Scope:           defaultString(in.Scope, "project"),
		AutoUpdate:      in.AutoUpdate,
		AdoptNewTargets: defaultString(in.AdoptNewTargets, "manual"),
		AllowPrerelease: in.AllowPrerelease,
	}
}

func resolveWorkspaceLockSource(lockPath, source string) string {
	source = strings.TrimSpace(source)
	if source == "" || filepath.IsAbs(source) {
		return source
	}
	if strings.Contains(source, ":") && !strings.HasPrefix(source, ".") && !strings.HasPrefix(source, "..") {
		return source
	}
	return filepath.Join(filepath.Dir(lockPath), source)
}

func targetIDsToStrings(items []domain.TargetID) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		out = append(out, string(item))
	}
	return out
}

func boolPtr(v bool) *bool { return &v }

func (s Service) workspaceRootForPolicy(policy domain.InstallPolicy) string {
	if !strings.EqualFold(strings.TrimSpace(policy.Scope), "project") {
		return ""
	}
	if root := strings.TrimSpace(s.CurrentWorkspaceRoot); root != "" {
		return filepath.Clean(root)
	}
	return ""
}

func syncNeedsReplace(record domain.InstallationRecord, source string, desiredPolicy domain.InstallPolicy, desiredTargets []domain.TargetID, desiredVersion string) bool {
	if strings.TrimSpace(record.RequestedSourceRef.Value) != strings.TrimSpace(source) {
		return true
	}
	if record.Policy.Scope != desiredPolicy.Scope {
		return true
	}
	if len(record.Targets) != len(desiredTargets) {
		return true
	}
	currentTargets := sortedTargets(record.Targets)
	sort.Slice(desiredTargets, func(i, j int) bool { return desiredTargets[i] < desiredTargets[j] })
	for i := range currentTargets {
		if currentTargets[i] != desiredTargets[i] {
			return true
		}
	}
	if desiredVersion != "" && strings.TrimSpace(record.ResolvedVersion) != strings.TrimSpace(desiredVersion) {
		return false
	}
	return false
}

func syncNeedsUpdate(record domain.InstallationRecord, source, desiredVersion string) bool {
	if strings.TrimSpace(record.RequestedSourceRef.Value) != strings.TrimSpace(source) {
		return false
	}
	if strings.TrimSpace(desiredVersion) == "" {
		return false
	}
	return strings.TrimSpace(record.ResolvedVersion) != strings.TrimSpace(desiredVersion)
}

func sanitizeID(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	if b.Len() == 0 {
		return "integration"
	}
	return b.String()
}
