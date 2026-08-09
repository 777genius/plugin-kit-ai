package domain

const StateSchemaVersion = 2

type MaterializationState string
type ActivationState string
type AuthenticationState string
type PolicyState string
type VerificationState string

const (
	MaterializationAbsent       MaterializationState = "absent"
	MaterializationStaged       MaterializationState = "staged"
	MaterializationMaterialized MaterializationState = "materialized"
	MaterializationDegraded     MaterializationState = "degraded"

	ActivationNotRequired ActivationState = "not_required"
	ActivationPrepared    ActivationState = "prepared"
	ActivationManual      ActivationState = "manual_activation_required"
	ActivationActive      ActivationState = "active"
	ActivationFailed      ActivationState = "failed"

	AuthenticationNotRequired AuthenticationState = "not_required"
	AuthenticationNotChecked  AuthenticationState = "not_checked"
	AuthenticationPending     AuthenticationState = "auth_pending"
	AuthenticationComplete    AuthenticationState = "authenticated"
	AuthenticationFailed      AuthenticationState = "failed"

	PolicyAllowed          PolicyState = "allowed"
	PolicyBlocked          PolicyState = "blocked"
	PolicyApprovalRequired PolicyState = "approval_required"

	VerificationNotRun       VerificationState = "not_run"
	VerificationPackageValid VerificationState = "package_validated"
	VerificationInstalled    VerificationState = "installation_verified"
	VerificationRuntime      VerificationState = "runtime_verified"
	VerificationFailed       VerificationState = "failed"
)

type SourceBinding struct {
	SourceBindingID  string `json:"source_binding_id"`
	RequestedSource  string `json:"requested_source"`
	CanonicalSource  string `json:"canonical_source"`
	Repository       string `json:"repository,omitempty"`
	PackageSubpath   string `json:"package_subpath,omitempty"`
	ResolvedRevision string `json:"resolved_revision"`
	TreeDigest       string `json:"tree_digest"`
	Publisher        string `json:"publisher,omitempty"`
}

type PackageBinding struct {
	LoaderKind       string             `json:"loader_kind"`
	FormatID         string             `json:"format_id"`
	SchemaURI        string             `json:"schema_uri"`
	SchemaVersion    string             `json:"schema_version,omitempty"`
	ManifestSchema   *SchemaIdentity    `json:"manifest_schema,omitempty"`
	ManifestDocument *VersionedDocument `json:"manifest_document,omitempty"`
	CatalogEvidence  *CatalogEvidence   `json:"catalog_evidence,omitempty"`
	Diagnostics      []Diagnostic       `json:"diagnostics,omitempty"`
	DeclaredName     string             `json:"declared_name"`
	Version          string             `json:"version,omitempty"`
	ManifestDigest   string             `json:"manifest_digest"`
	Inventory        ComponentInventory `json:"inventory"`
}

type NativeObjectOwnership struct {
	ObjectID        string `json:"object_id"`
	Kind            string `json:"kind"`
	LogicalName     string `json:"logical_name,omitempty"`
	Path            string `json:"path,omitempty"`
	BeforeDigest    string `json:"before_digest,omitempty"`
	ManagedDigest   string `json:"managed_digest,omitempty"`
	ProtectionClass string `json:"protection_class"`
	UserModified    bool   `json:"user_modified,omitempty"`
}

type MutationReceipt struct {
	OperationID     string `json:"operation_id"`
	Sequence        int    `json:"sequence"`
	MutationType    string `json:"mutation_type"`
	ClientBindingID string `json:"client_binding_id"`
	ActivePath      string `json:"active_path,omitempty"`
	StagingPath     string `json:"staging_path,omitempty"`
	BackupPath      string `json:"backup_path,omitempty"`
	BeforeDigest    string `json:"before_digest,omitempty"`
	AfterDigest     string `json:"after_digest,omitempty"`
	Phase           string `json:"phase"`
}

// ClientPackageRevision records the exact portable package revision that was
// projected into one client. Installation.Package is the latest accepted
// revision, while individual clients may temporarily converge one at a time.
type ClientPackageRevision struct {
	Version          string `json:"version,omitempty"`
	ResolvedRevision string `json:"resolved_revision,omitempty"`
	TreeDigest       string `json:"tree_digest"`
	ManifestDigest   string `json:"manifest_digest"`
}

type ClientBinding struct {
	ClientBindingID  string                  `json:"client_binding_id"`
	ClientID         string                  `json:"client_id"`
	Scope            string                  `json:"scope"`
	TargetLocator    string                  `json:"target_locator"`
	PhysicalArtifact string                  `json:"physical_artifact_id"`
	Materialization  MaterializationState    `json:"materialization"`
	Activation       ActivationState         `json:"activation"`
	Authentication   AuthenticationState     `json:"authentication"`
	Policy           PolicyState             `json:"policy"`
	Verification     VerificationState       `json:"verification"`
	PackageRevision  *ClientPackageRevision  `json:"package_revision,omitempty"`
	NativeObjects    []NativeObjectOwnership `json:"native_objects,omitempty"`
	Receipts         []MutationReceipt       `json:"receipts,omitempty"`
	UpdatedAt        string                  `json:"updated_at"`
}

type Installation struct {
	InstallationID string                   `json:"installation_id"`
	DeclaredName   string                   `json:"declared_name"`
	Source         SourceBinding            `json:"source"`
	Package        PackageBinding           `json:"package"`
	Clients        map[string]ClientBinding `json:"clients"`
	NeedsRebind    bool                     `json:"needs_rebind,omitempty"`
	CreatedAt      string                   `json:"created_at"`
	UpdatedAt      string                   `json:"updated_at"`
}

type StateFileV2 struct {
	SchemaVersion int            `json:"schema_version"`
	Installations []Installation `json:"installations"`
}
