package ports

import (
	"context"
	"time"

	"github.com/777genius/plugin-kit-ai/install/integrationctl/domain"
)

type Capabilities struct {
	InstallMode               string
	SupportsNativeUpdate      bool
	SupportsNativeRemove      bool
	SupportsLinkMode          bool
	SupportsAutoUpdatePolicy  bool
	SupportsScopeUser         bool
	SupportsScopeProject      bool
	SupportsScopeLocal        bool
	SupportsRepair            bool
	RequiresRestart           bool
	RequiresReload            bool
	RequiresNewThread         bool
	MayTriggerInteractiveAuth bool
	SupportedSourceKinds      []string
	EvidenceKey               string
}

type InspectInput struct {
	IntegrationID string
	Record *domain.InstallationRecord
	Scope  string
}

type InspectResult struct {
	TargetID                 domain.TargetID
	Installed                bool
	State                    domain.InstallState
	ActivationState          domain.ActivationState
	InteractiveAuthState     string
	CatalogPolicy            *domain.CatalogPolicySnapshot
	ConfigPrecedenceContext  []string
	EnvironmentRestrictions  []domain.EnvironmentRestrictionCode
	VolatileOverrideDetected bool
	TrustResolutionSource    string
	SourceAccessState        string
	OwnedNativeObjects       []domain.NativeObjectRef
	ObservedNativeObjects    []domain.NativeObjectRef
	SettingsFiles            []string
	Warnings                 []string
	EvidenceClass            domain.EvidenceClass
}

type PlanInstallInput struct {
	Manifest domain.IntegrationManifest
	Policy   domain.InstallPolicy
	Inspect  InspectResult
}

type PlanUpdateInput struct {
	CurrentRecord domain.InstallationRecord
	NextManifest  domain.IntegrationManifest
	Inspect       InspectResult
}

type PlanRemoveInput struct {
	Record  domain.InstallationRecord
	Inspect InspectResult
}

type PlanToggleInput struct {
	Record  domain.InstallationRecord
	Inspect InspectResult
}

type RepairInput struct {
	Record         domain.InstallationRecord
	Inspect        InspectResult
	Manifest       *domain.IntegrationManifest
	ResolvedSource *ResolvedSource
}

type AdapterPlan struct {
	TargetID           domain.TargetID
	ActionClass        string
	Summary            string
	Commands           []string
	PathsTouched       []string
	OwnedNativeObjects []domain.NativeObjectRef
	RestartRequired    bool
	ReloadRequired     bool
	NewThreadRequired  bool
	ManualSteps        []string
	Blocking           bool
	EvidenceKey        string
}

type ApplyInput struct {
	Plan           AdapterPlan
	Manifest       domain.IntegrationManifest
	ResolvedSource *ResolvedSource
	Policy         domain.InstallPolicy
	Inspect        InspectResult
	Record         *domain.InstallationRecord
}

type ApplyResult struct {
	TargetID                 domain.TargetID
	State                    domain.InstallState
	ActivationState          domain.ActivationState
	InteractiveAuthState     string
	OwnedNativeObjects       []domain.NativeObjectRef
	Warnings                 []string
	ManualSteps              []string
	RestartRequired          bool
	ReloadRequired           bool
	NewThreadRequired        bool
	SourceAccessState        string
	EnvironmentRestrictions  []domain.EnvironmentRestrictionCode
	VolatileOverrideDetected bool
	EvidenceClass            domain.EvidenceClass
	AdapterMetadata          map[string]any
}

type TargetAdapter interface {
	ID() domain.TargetID
	Capabilities(context.Context) (Capabilities, error)
	Inspect(context.Context, InspectInput) (InspectResult, error)
	PlanInstall(context.Context, PlanInstallInput) (AdapterPlan, error)
	ApplyInstall(context.Context, ApplyInput) (ApplyResult, error)
	PlanUpdate(context.Context, PlanUpdateInput) (AdapterPlan, error)
	ApplyUpdate(context.Context, ApplyInput) (ApplyResult, error)
	PlanRemove(context.Context, PlanRemoveInput) (AdapterPlan, error)
	ApplyRemove(context.Context, ApplyInput) (ApplyResult, error)
	Repair(context.Context, RepairInput) (ApplyResult, error)
}

type ToggleTargetAdapter interface {
	PlanEnable(context.Context, PlanToggleInput) (AdapterPlan, error)
	ApplyEnable(context.Context, ApplyInput) (ApplyResult, error)
	PlanDisable(context.Context, PlanToggleInput) (AdapterPlan, error)
	ApplyDisable(context.Context, ApplyInput) (ApplyResult, error)
}

type ResolvedSource struct {
	Kind         string
	Requested    domain.RequestedSourceRef
	Resolved     domain.ResolvedSourceRef
	LocalPath    string
	CleanupPath  string
	SourceDigest string
	ImportRoots  []string
	FailureClass string
}

type SourceResolver interface {
	Resolve(context.Context, domain.IntegrationRef) (ResolvedSource, error)
}

type ManifestLoader interface {
	Load(context.Context, ResolvedSource) (domain.IntegrationManifest, error)
}

type StateFile struct {
	SchemaVersion int                         `json:"schema_version"`
	Installations []domain.InstallationRecord `json:"installations"`
}

type StateStore interface {
	Load(context.Context) (StateFile, error)
	Save(context.Context, StateFile) error
}

type WorkspaceLockStore interface {
	Load(context.Context) (domain.WorkspaceLock, error)
	Save(context.Context, domain.WorkspaceLock) error
	Path() string
}

type UnlockFunc func() error

type LockManager interface {
	Acquire(context.Context, string) (UnlockFunc, error)
}

type OperationJournal interface {
	Start(context.Context, domain.OperationRecord) error
	AppendStep(context.Context, string, domain.JournalStep) error
	Finish(context.Context, string, string) error
	ListOpen(context.Context) ([]domain.OperationRecord, error)
}

type EvidenceEntry struct {
	Key           string   `json:"key"`
	Claim         string   `json:"claim"`
	EvidenceClass string   `json:"evidence_class"`
	URLs          []string `json:"urls"`
}

type EvidenceRegistry interface {
	Get(context.Context, string) (EvidenceEntry, error)
	List(context.Context) ([]EvidenceEntry, error)
}

type PathInfo struct {
	Exists bool
	IsDir  bool
}

type FileSystem interface {
	ReadFile(context.Context, string) ([]byte, error)
	WriteFileAtomic(context.Context, string, []byte, uint32) error
	MkdirAll(context.Context, string, uint32) error
	Stat(context.Context, string) (PathInfo, error)
	Remove(context.Context, string) error
}

type SafeFileMutationInput struct {
	Path           string
	Mode           uint32
	Build          func(original []byte, exists bool) ([]byte, error)
	ValidateBefore func(next []byte) error
	ValidateAfter  func(context.Context, string, []byte) error
}

type SafeFileMutationResult struct {
	Path        string
	BackupPath  string
	HadOriginal bool
}

type SafeFileMutator interface {
	MutateFile(context.Context, SafeFileMutationInput) (SafeFileMutationResult, error)
}

type Command struct {
	Argv []string
	Env  []string
	Dir  string
}

type CommandResult struct {
	ExitCode int
	Stdout   []byte
	Stderr   []byte
}

type ProcessRunner interface {
	Run(context.Context, Command) (CommandResult, error)
}

type Clock interface {
	Now() time.Time
}
