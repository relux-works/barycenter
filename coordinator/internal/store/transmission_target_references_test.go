package store

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"
)

func issueTargetReference(
	t *testing.T,
	st *Store,
	owner OnboardingCredentials,
	kind TransmissionAudienceSelectorKind,
	orbitID int64,
	slot string,
	now int64,
) string {
	t.Helper()
	reference, err := st.IssueTransmissionTargetReference(
		IssueTransmissionTargetReferenceParams{
			ExpectedActorID: owner.ActorID, Bearer: owner.ControlToken,
			Kind: kind, OrbitID: orbitID, Slot: slot, IssuedAt: now,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if !transmissionTargetReferencePattern.MatchString(reference) {
		t.Fatalf("reference=%q", reference)
	}
	return reference
}

func TestTransmissionTargetReferencesAreOpaqueCredentialBoundAndGenerationSafe(t *testing.T) {
	st, source := newMediaIngestTestStore(t)
	companion := addTransmissionInstallation(t, st, source, "companion")
	now := time.Now().UnixMilli()
	options, err := st.ListTransmissionTargetReferences(
		source.ActorID, source.ControlToken, now,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(options) != 3 {
		t.Fatalf("options=%+v", options)
	}
	for _, option := range options {
		if !transmissionTargetReferencePattern.MatchString(option.Reference) ||
			option.Label == "" || strings.Contains(option.Reference, "1/") {
			t.Fatalf("unsafe option=%+v", option)
		}
	}

	media := readyLifecycleMedia(
		t, st, source, now, now+int64((7*24*time.Hour)/time.Millisecond),
	)
	reference := issueTargetReference(
		t, st, source, TransmissionSelectorPulsar, source.OrbitID,
		companion.Slot, now+1,
	)
	params := resolvedTransmissionParams(source, media, now+2)
	params.AudienceKind = TransmissionAudienceExplicit
	params.Selectors = []TransmissionAudienceSelector{{Reference: reference}}
	params.Availability = []TransmissionTargetAvailability{
		fullTransmissionAvailability(companion, params.AcceptedAt),
	}
	created, err := st.CreateResolvedTransmission(params)
	if err != nil || len(created.Creation.Targets) != 1 ||
		created.Creation.Targets[0].ActorID != companion.ActorID {
		t.Fatalf("created=%+v err=%v", created, err)
	}

	other := params
	other.ExpectedActorID = companion.ActorID
	other.Bearer = companion.ControlToken
	other.IdempotencyKeyHash = strings.Repeat("7", 64)
	other.RequestHash = strings.Repeat("8", 64)
	other.AcceptedAt++
	if _, err := st.CreateResolvedTransmission(other); !errors.Is(err, ErrTransmissionAudienceNotFound) {
		t.Fatalf("cross-credential reference error=%v", err)
	}

	staleReference := issueTargetReference(
		t, st, source, TransmissionSelectorPulsar, source.OrbitID,
		companion.Slot, now+3,
	)
	if _, err := st.db.Exec(`UPDATE slots SET revoked_at = ?
WHERE orbit_id = ? AND slot = ?`, now+4, companion.OrbitID, companion.Slot); err != nil {
		t.Fatal(err)
	}
	stale := params
	stale.Selectors = []TransmissionAudienceSelector{{Reference: staleReference}}
	stale.IdempotencyKeyHash = strings.Repeat("9", 64)
	stale.RequestHash = strings.Repeat("a", 64)
	stale.AcceptedAt = now + 5
	if _, err := st.CreateResolvedTransmission(stale); !errors.Is(err, ErrTransmissionAudienceNotFound) {
		t.Fatalf("stale reference error=%v", err)
	}
}

func TestExplicitTargetsFailAtomicallyWithOpaqueSortedCapabilityDetails(t *testing.T) {
	st, source := newMediaIngestTestStore(t)
	companion := addTransmissionInstallation(t, st, source, "companion")
	now := time.Now().UnixMilli()
	media := readyLifecycleMedia(
		t, st, source, now, now+int64((7*24*time.Hour)/time.Millisecond),
	)
	reference := issueTargetReference(
		t, st, source, TransmissionSelectorBarycenter, source.OrbitID, "", now+1,
	)
	params := resolvedTransmissionParams(source, media, now+2)
	params.AudienceKind = TransmissionAudienceExplicit
	params.Selectors = []TransmissionAudienceSelector{{Reference: reference}}
	sourceAvailability := fullTransmissionAvailability(source, params.AcceptedAt)
	companionAvailability := fullTransmissionAvailability(companion, params.AcceptedAt)
	companionAvailability.MediaClipCapable = false
	companionAvailability.OverlayCapable = false
	params.Availability = []TransmissionTargetAvailability{
		sourceAvailability, companionAvailability,
	}
	_, err := st.CreateResolvedTransmission(params)
	var unsupported *TransmissionUnsupportedTargetsError
	if !errors.As(err, &unsupported) || len(unsupported.Targets) != 1 ||
		unsupported.Targets[0].Reference != reference ||
		!reflect.DeepEqual(unsupported.Targets[0].MissingCapabilities,
			[]string{TransmissionCapabilityMediaClip, TransmissionCapabilityOverlayMix}) {
		t.Fatalf("unsupported=%+v err=%v", unsupported, err)
	}
	var transmissions, requests int
	if err := st.db.QueryRow(`SELECT COUNT(*) FROM transmissions`).Scan(&transmissions); err != nil {
		t.Fatal(err)
	}
	if err := st.db.QueryRow(`SELECT COUNT(*) FROM transmission_requests`).Scan(&requests); err != nil {
		t.Fatal(err)
	}
	if transmissions != 0 || requests != 0 {
		t.Fatalf("partial create transmissions=%d requests=%d", transmissions, requests)
	}
}

func TestCommonExplicitCapabilityPolicyCoversClipAndTrack(t *testing.T) {
	tests := []struct {
		kind     MediaKind
		delivery TransmissionDelivery
		want     []string
	}{
		{MediaKindVoiceClip, TransmissionDeliveryAfterCurrent,
			[]string{TransmissionCapabilityMediaClip}},
		{MediaKindAudioClip, TransmissionDeliveryOverlay,
			[]string{TransmissionCapabilityMediaClip, TransmissionCapabilityOverlayMix}},
		{MediaKindBuiltinCue, TransmissionDeliveryInterrupt,
			[]string{TransmissionCapabilityInterrupt, TransmissionCapabilityMediaClip}},
		{MediaKindAudioTrack, TransmissionDelivery("queue"),
			[]string{TransmissionCapabilityAudioTrack, TransmissionCapabilityQueueReplace,
				TransmissionCapabilityStream}},
		{MediaKindAudioTrack, TransmissionDelivery("replace"),
			[]string{TransmissionCapabilityAudioTrack, TransmissionCapabilityQueueReplace,
				TransmissionCapabilityStream}},
	}
	for _, test := range tests {
		got, err := RequiredTransmissionCapabilities(test.kind, test.delivery)
		if err != nil || !reflect.DeepEqual(got, test.want) {
			t.Errorf("kind=%s delivery=%s got=%v want=%v err=%v",
				test.kind, test.delivery, got, test.want, err)
		}
	}
	if _, err := RequiredTransmissionCapabilities(
		MediaKindAudioTrack, TransmissionDeliveryOverlay,
	); !errors.Is(err, ErrTransmissionDeliveryKindMismatch) {
		t.Fatalf("track overlay error=%v", err)
	}
}
