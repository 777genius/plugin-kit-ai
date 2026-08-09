package domain

type ClientID string
type DetectionStatus string
type InstallScope string
type PackageMode string
type ActivationMode string
type SupportLevel string
type PlanStatus string
type ComponentKind string

const (
	ClientCodex   ClientID = "codex"
	ClientCursor  ClientID = "cursor"
	ClientCopilot ClientID = "copilot"
	ClientVSCode  ClientID = "vscode"
	ClientKiro    ClientID = "kiro"

	DetectionNotDetected DetectionStatus = "not_detected"
	DetectionDetected    DetectionStatus = "detected"

	ScopeUser    InstallScope = "user"
	ScopeProject InstallScope = "project"

	PackageNative     PackageMode = "native"
	PackageProjection PackageMode = "compatibility_projection"
	PackageBridge     PackageMode = "client_bridge"
	PackagePrepared   PackageMode = "prepared_package"

	ActivationAutomatic ActivationMode = "automatic"
	ActivationByClient  ActivationMode = "client_managed"
	ActivationByUser    ActivationMode = "manual"

	SupportNative      SupportLevel = "native"
	SupportProjected   SupportLevel = "projected"
	SupportPrepared    SupportLevel = "prepared"
	SupportUnsupported SupportLevel = "unsupported"

	PlanReady                    PlanStatus = "ready"
	PlanPrepared                 PlanStatus = "prepared"
	PlanManualActivationRequired PlanStatus = "manual_activation_required"
	PlanUnsupported              PlanStatus = "unsupported"

	ComponentSkill     ComponentKind = "skill"
	ComponentMCPServer ComponentKind = "mcp_server"
	ComponentExtension ComponentKind = "extension"
)

type ClientSurface struct {
	ID       string `json:"id"`
	Detected bool   `json:"detected"`
	Evidence string `json:"evidence,omitempty"`
}

type DetectedClient struct {
	ClientID    ClientID        `json:"client_id"`
	DisplayName string          `json:"display_name"`
	Status      DetectionStatus `json:"status"`
	Surfaces    []ClientSurface `json:"surfaces,omitempty"`
	// ExecutablePath and ConfigRoot are operational locators. They must never be
	// emitted by the public JSON renderer because they can reveal the user home.
	ExecutablePath string `json:"-"`
	ConfigRoot     string `json:"-"`
}

type ClientCapabilities struct {
	ClientID         ClientID                `json:"client_id"`
	PackageMode      PackageMode             `json:"package_mode"`
	ActivationMode   ActivationMode          `json:"activation_mode"`
	Scopes           []InstallScope          `json:"scopes"`
	SkillSupport     SupportLevel            `json:"skill_support"`
	MCPTransports    map[string]SupportLevel `json:"mcp_transports,omitempty"`
	ExtensionSupport SupportLevel            `json:"extension_support"`
}

type ComponentDecision struct {
	Kind    ComponentKind `json:"kind"`
	Name    string        `json:"name"`
	Support SupportLevel  `json:"support"`
	Reason  string        `json:"reason,omitempty"`
}

type DeliveryPlan struct {
	ClientID           ClientID            `json:"client_id"`
	Scope              InstallScope        `json:"scope"`
	Status             PlanStatus          `json:"status"`
	PackageMode        PackageMode         `json:"package_mode"`
	Activation         ActivationState     `json:"activation"`
	Authentication     AuthenticationState `json:"authentication"`
	Policy             PolicyState         `json:"policy"`
	Verification       VerificationState   `json:"verification"`
	PhysicalArtifactID string              `json:"physical_artifact_id"`
	Components         []ComponentDecision `json:"components,omitempty"`
	UserActions        []string            `json:"user_actions,omitempty"`
	// LocalActions can contain operational paths and are rendered only in
	// human-readable output. They must never be emitted by the public JSON API.
	LocalActions []string     `json:"-"`
	Warnings     []string     `json:"warnings,omitempty"`
	Diagnostics  []Diagnostic `json:"diagnostics,omitempty"`
	// TargetRoot and ActivePath are intentionally excluded from public JSON.
	TargetAnchor string `json:"-"`
	TargetRoot   string `json:"-"`
	ActivePath   string `json:"-"`
}

// DeliveryTarget contains deterministic operational paths computed from the
// configured client roots. Persisted state must be checked against this value
// before any destructive operation.
type DeliveryTarget struct {
	TargetAnchor string `json:"-"`
	TargetRoot   string `json:"-"`
	ActivePath   string `json:"-"`
}

type OpenAIMCPAuthHint struct {
	OAuthResource     string `json:"oauth_resource,omitempty"`
	BearerTokenEnvVar string `json:"bearer_token_env_var,omitempty"`
}

type CompatibilityHints struct {
	// Compatibility preserves generic, per-client catalog requirements. The
	// OpenAI map remains for legacy projection consumers and is not authoritative
	// for whether authentication is required.
	Compatibility map[string]CatalogCompatibility `json:"compatibility,omitempty"`
	OpenAIMCPAuth map[string]OpenAIMCPAuthHint    `json:"openai_mcp_auth,omitempty"`
}

type StagedDelivery struct {
	ClientID       ClientID                `json:"client_id"`
	OwnedBase      string                  `json:"-"`
	ActivePath     string                  `json:"-"`
	StagingPath    string                  `json:"-"`
	ArtifactDigest string                  `json:"artifact_digest"`
	NativeObjects  []NativeObjectOwnership `json:"native_objects,omitempty"`
}

type ActivationRequest struct {
	Client            DetectedClient `json:"client"`
	Plan              DeliveryPlan   `json:"plan"`
	Delivery          StagedDelivery `json:"delivery"`
	DeclaredName      string         `json:"declared_name"`
	Replacing         bool           `json:"replacing"`
	Interactive       bool           `json:"interactive"`
	BackendExecutable string         `json:"-"`
}

type ActivationOutcome struct {
	Activation     ActivationState     `json:"activation"`
	Authentication AuthenticationState `json:"authentication"`
	Policy         PolicyState         `json:"policy"`
	Verification   VerificationState   `json:"verification"`
	UserActions    []string            `json:"user_actions,omitempty"`
	LocalActions   []string            `json:"-"`
}

type DeactivationRequest struct {
	Client              DetectedClient  `json:"client"`
	DeclaredName        string          `json:"declared_name"`
	CurrentActivation   ActivationState `json:"current_activation"`
	Interactive         bool            `json:"interactive"`
	ExternalUninstalled bool            `json:"external_uninstalled"`
	Confirmed           bool            `json:"confirmed"`
	PhysicalArtifactID  string          `json:"physical_artifact_id"`
	BackendExecutable   string          `json:"-"`
}

type DeactivationOutcome struct {
	Activation              ActivationState `json:"activation"`
	ArtifactRemovalAllowed  bool            `json:"artifact_removal_allowed"`
	ExternalRemovalComplete bool            `json:"external_removal_complete"`
	UserActions             []string        `json:"user_actions,omitempty"`
	LocalActions            []string        `json:"-"`
}
