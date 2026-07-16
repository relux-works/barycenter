package automation

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func allowedAdmission() AdmissionInputs {
	return AdmissionInputs{
		FeatureEnabled: true, CredentialResolved: true, PrincipalState: PrincipalActive,
		ScopeAllowed: true, CueFound: true, CueReady: true, CueEligible: true,
		AudienceAllowed: true, AirPolicyAllowed: true,
		AutomationCapabilityReady: true, DeliveryCapabilityReady: true,
	}
}

func TestAdmissionPrecedenceIsFailClosed(t *testing.T) {
	in := allowedAdmission()
	if got := in.Denial(); got != "" {
		t.Fatalf("allowed admission denied as %q", got)
	}

	tests := []struct {
		name string
		set  func(*AdmissionInputs)
		want DenialReason
	}{
		{"feature", func(v *AdmissionInputs) { v.FeatureEnabled = false; v.CredentialResolved = false }, DenyAutomationDisabled},
		{"credential", func(v *AdmissionInputs) { v.CredentialResolved = false; v.ScopeAllowed = false }, DenyInvalidCredential},
		{"disabled", func(v *AdmissionInputs) { v.PrincipalState = PrincipalDisabled; v.IdempotencyConflict = true }, DenyPrincipalDisabled},
		{"revoked", func(v *AdmissionInputs) { v.PrincipalState = PrincipalRevoked; v.IdempotencyConflict = true }, DenyPrincipalRevoked},
		{"expired", func(v *AdmissionInputs) { v.PrincipalState = PrincipalExpired; v.IdempotencyConflict = true }, DenyPrincipalExpired},
		{"idempotency", func(v *AdmissionInputs) { v.IdempotencyConflict = true; v.RateLimited = true }, DenyIdempotencyConflict},
		{"rate", func(v *AdmissionInputs) { v.RateLimited = true; v.ScopeAllowed = false }, DenyTooManyAttempts},
		{"concurrency", func(v *AdmissionInputs) { v.ConcurrencyLimited = true; v.ScopeAllowed = false }, DenyExecutionInProgress},
		{"scope", func(v *AdmissionInputs) { v.ScopeAllowed = false; v.CueFound = false }, DenyInsufficientScope},
		{"cue_missing", func(v *AdmissionInputs) { v.CueFound = false; v.QuietHours = true }, DenyCueNotFound},
		{"cue_not_ready", func(v *AdmissionInputs) { v.CueReady = false; v.QuietHours = true }, DenyCueNotReady},
		{"cue_kind", func(v *AdmissionInputs) { v.CueEligible = false; v.QuietHours = true }, DenyCueNotEligible},
		{"quiet", func(v *AdmissionInputs) { v.QuietHours = true; v.AudienceAllowed = false }, DenyQuietHours},
		{"audience", func(v *AdmissionInputs) { v.AudienceAllowed = false; v.AirPolicyAllowed = false }, DenyAudienceNotAllowed},
		{"air_policy", func(v *AdmissionInputs) { v.AirPolicyAllowed = false; v.AutomationCapabilityReady = false }, DenyAirPolicy},
		{"automation_capability", func(v *AdmissionInputs) { v.AutomationCapabilityReady = false; v.DeliveryCapabilityReady = false }, DenyAutomationCapabilityMissing},
		{"delivery_capability", func(v *AdmissionInputs) { v.DeliveryCapabilityReady = false }, DenyDeliveryCapabilityMissing},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			candidate := allowedAdmission()
			test.set(&candidate)
			if got := candidate.Denial(); got != test.want {
				t.Fatalf("denial=%q want=%q", got, test.want)
			}
		})
	}
}

func TestCueAndAudienceSafetyBoundary(t *testing.T) {
	if !EligibleCue(CueAudioClip, CueSourceApp, true, true, false) ||
		!EligibleCue(CueBuiltin, CueSourceSystem, true, true, true) {
		t.Fatal("eligible saved cues were rejected")
	}
	for _, kind := range []CueKind{"voice_clip", "audio_track", "live_ptt", "microphone"} {
		if EligibleCue(kind, CueSourceApp, true, true, true) {
			t.Fatalf("unsafe cue kind %q was admitted", kind)
		}
	}
	if EligibleCue(CueAudioClip, CueSourceApp, true, false, false) ||
		EligibleCue(CueAudioClip, "telegram", true, true, false) ||
		EligibleCue(CueBuiltin, CueSourceApp, true, true, true) ||
		EligibleCue(CueBuiltin, CueSourceSystem, true, true, false) {
		t.Fatal("uncommitted or unpinned cue was admitted")
	}

	if !SupportedAudience(AudienceOwnBarycenter, 0) ||
		!SupportedAudience(AudienceCurrentAir, 0) ||
		!SupportedAudience(AudienceExplicit, MaxExplicitSelectors) {
		t.Fatal("supported audience was rejected")
	}
	for _, test := range []struct {
		kind  AudienceKind
		count int
	}{{"this_pulsar", 0}, {AudienceExplicit, 0}, {AudienceExplicit, 65}, {AudienceOwnBarycenter, 1}} {
		if SupportedAudience(test.kind, test.count) {
			t.Fatalf("unsafe audience kind=%q count=%d was admitted", test.kind, test.count)
		}
	}
}

func TestCredentialStateIsCollapsedAtPublicBoundary(t *testing.T) {
	for _, reason := range []DenialReason{DenyPrincipalDisabled, DenyPrincipalRevoked, DenyPrincipalExpired} {
		if got := PublicCredentialReason(reason); got != DenyInvalidCredential {
			t.Fatalf("public reason for %q is %q", reason, got)
		}
	}
	if got := PublicCredentialReason(DenyInsufficientScope); got != DenyInsufficientScope {
		t.Fatalf("non-credential reason changed to %q", got)
	}
}

func TestNormativeContractExamplesDecisionsAndEntryPoints(t *testing.T) {
	repositoryRoot := filepath.Join("..", "..", "..")
	path := filepath.Join(repositoryRoot, "docs", "analysis", "p3-automation-safety-contract-v1.md")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read automation contract: %v", err)
	}
	contract := string(raw)
	normalizedContract := strings.Join(strings.Fields(contract), " ")

	remaining := contract
	examples := 0
	for {
		start := strings.Index(remaining, "```json\n")
		if start < 0 {
			break
		}
		remaining = remaining[start+len("```json\n"):]
		end := strings.Index(remaining, "\n```")
		if end < 0 {
			t.Fatal("unterminated JSON example in automation contract")
		}
		if !json.Valid([]byte(remaining[:end])) {
			t.Fatalf("invalid JSON example %d:\n%s", examples+1, remaining[:end])
		}
		examples++
		remaining = remaining[end+len("\n```"):]
	}
	if examples != 1 {
		t.Fatalf("JSON example count=%d want=1; update guard intentionally", examples)
	}

	for _, decision := range []string{
		ContractVersion, TriggerPath, "There is no loopback listener and no webhook receiver in v1",
		"voice_clip` from any source", "V1 automation delivery is only `overlay`",
		"`this_pulsar` is intentionally absent", "block-before-DND-before-online precedence",
		"A spring-forward local minute that does not exist is skipped",
		"only the first occurrence (the earlier UTC instant)", "never caught up",
		"five authenticated non-replay attempts per principal per rolling minute",
		"twenty per owner orbit per rolling hour", "automation_cue_v1",
		"There is no legacy `play_voice`", "EPIC-260714-th54l3",
	} {
		if !strings.Contains(normalizedContract, decision) {
			t.Errorf("automation contract lost required decision %q", decision)
		}
	}

	for _, entry := range []string{"docs/protocol.md", "docs/spec-self-contained-audio.md"} {
		entryRaw, readErr := os.ReadFile(filepath.Join(repositoryRoot, filepath.FromSlash(entry)))
		if readErr != nil {
			t.Errorf("read entry point %q: %v", entry, readErr)
			continue
		}
		if !strings.Contains(string(entryRaw), "p3-automation-safety-contract-v1.md") {
			t.Errorf("%s does not link the normative automation contract", entry)
		}
	}
}
