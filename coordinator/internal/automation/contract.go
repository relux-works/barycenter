// Package automation freezes the security-sensitive vocabulary shared by the
// later soundboard persistence, HTTP, scheduler and history tasks. It contains
// no network listener or runtime: automation remains production-dark until the
// separate implementation and rollout tasks deliberately compose those seams.
package automation

const (
	ContractVersion = "automation-safety-v1"
	TriggerPath     = "/v1/automation/triggers"

	MaxExplicitSelectors      = 64
	MaxAcceptedPerMinute      = 5
	MaxAcceptedPerOrbitHour   = 20
	MaxConcurrentPerPrincipal = 1
	MaxConcurrentPerOrbit     = 2
)

type TriggerKind string

const (
	TriggerManualSoundboard TriggerKind = "manual_soundboard"
	TriggerScopedAPI        TriggerKind = "scoped_api"
	TriggerSchedule         TriggerKind = "schedule"
)

type CueKind string

const (
	CueBuiltin   CueKind = "builtin_cue"
	CueAudioClip CueKind = "audio_clip"
)

type CueSource string

const (
	CueSourceApp    CueSource = "app"
	CueSourceSystem CueSource = "system"
)

type Delivery string

const (
	// Automation deliberately has one non-interactive delivery. Manual
	// soundboard actions remain on the ordinary transmission contract.
	DeliveryOverlay Delivery = "overlay"
)

type AudienceKind string

const (
	AudienceOwnBarycenter AudienceKind = "own_barycenter"
	AudienceCurrentAir    AudienceKind = "current_air"
	AudienceExplicit      AudienceKind = "explicit"
)

type PrincipalState string

const (
	PrincipalActive   PrincipalState = "active"
	PrincipalDisabled PrincipalState = "disabled"
	PrincipalRevoked  PrincipalState = "revoked"
	PrincipalExpired  PrincipalState = "expired"
)

// DenialReason is the immutable internal history/audit vocabulary. HTTP
// deliberately collapses all credential-state reasons to
// invalid_automation_credential so token state cannot be probed remotely.
type DenialReason string

const (
	DenyAutomationDisabled          DenialReason = "automation_disabled"
	DenyInvalidCredential           DenialReason = "invalid_automation_credential"
	DenyPrincipalDisabled           DenialReason = "principal_disabled"
	DenyPrincipalRevoked            DenialReason = "principal_revoked"
	DenyPrincipalExpired            DenialReason = "principal_expired"
	DenyIdempotencyConflict         DenialReason = "idempotency_conflict"
	DenyInsufficientScope           DenialReason = "insufficient_scope"
	DenyCueNotFound                 DenialReason = "cue_not_found"
	DenyCueNotReady                 DenialReason = "cue_not_ready"
	DenyCueNotEligible              DenialReason = "cue_not_eligible"
	DenyQuietHours                  DenialReason = "quiet_hours"
	DenyTooManyAttempts             DenialReason = "too_many_attempts"
	DenyExecutionInProgress         DenialReason = "execution_in_progress"
	DenyAudienceNotAllowed          DenialReason = "audience_not_allowed"
	DenyAirPolicy                   DenialReason = "air_policy_denied"
	DenyAutomationCapabilityMissing DenialReason = "automation_capability_missing"
	DenyDeliveryCapabilityMissing   DenialReason = "delivery_capability_missing"
)

// AdmissionInputs represents the ordered fail-closed checks that can be made
// before target rows enter the existing block -> DND -> online/capability
// transmission policy. It is intentionally pure so every later adapter can
// share one precedence instead of growing a bypass.
type AdmissionInputs struct {
	FeatureEnabled            bool
	CredentialResolved        bool
	PrincipalState            PrincipalState
	IdempotencyConflict       bool
	ScopeAllowed              bool
	CueFound                  bool
	CueReady                  bool
	CueEligible               bool
	QuietHours                bool
	RateLimited               bool
	ConcurrencyLimited        bool
	AudienceAllowed           bool
	AirPolicyAllowed          bool
	AutomationCapabilityReady bool
	DeliveryCapabilityReady   bool
}

func (in AdmissionInputs) Denial() DenialReason {
	switch {
	case !in.FeatureEnabled:
		return DenyAutomationDisabled
	case !in.CredentialResolved:
		return DenyInvalidCredential
	case in.PrincipalState == PrincipalDisabled:
		return DenyPrincipalDisabled
	case in.PrincipalState == PrincipalRevoked:
		return DenyPrincipalRevoked
	case in.PrincipalState == PrincipalExpired:
		return DenyPrincipalExpired
	case in.PrincipalState != PrincipalActive:
		return DenyInvalidCredential
	case in.IdempotencyConflict:
		return DenyIdempotencyConflict
	case in.RateLimited:
		return DenyTooManyAttempts
	case in.ConcurrencyLimited:
		return DenyExecutionInProgress
	case !in.ScopeAllowed:
		return DenyInsufficientScope
	case !in.CueFound:
		return DenyCueNotFound
	case !in.CueReady:
		return DenyCueNotReady
	case !in.CueEligible:
		return DenyCueNotEligible
	case in.QuietHours:
		return DenyQuietHours
	case !in.AudienceAllowed:
		return DenyAudienceNotAllowed
	case !in.AirPolicyAllowed:
		return DenyAirPolicy
	case !in.AutomationCapabilityReady:
		return DenyAutomationCapabilityMissing
	case !in.DeliveryCapabilityReady:
		return DenyDeliveryCapabilityMissing
	default:
		return ""
	}
}

func EligibleCue(kind CueKind, source CueSource, ready, saved, builtinHashPinned bool) bool {
	switch kind {
	case CueAudioClip:
		return source == CueSourceApp && ready && saved
	case CueBuiltin:
		return source == CueSourceSystem && ready && saved && builtinHashPinned
	default:
		return false
	}
}

func SupportedAudience(kind AudienceKind, explicitSelectors int) bool {
	switch kind {
	case AudienceOwnBarycenter, AudienceCurrentAir:
		return explicitSelectors == 0
	case AudienceExplicit:
		return explicitSelectors > 0 && explicitSelectors <= MaxExplicitSelectors
	default:
		return false
	}
}

func PublicCredentialReason(reason DenialReason) DenialReason {
	switch reason {
	case DenyPrincipalDisabled, DenyPrincipalRevoked, DenyPrincipalExpired:
		return DenyInvalidCredential
	default:
		return reason
	}
}
