package main

import (
	"testing"

	protocol "relux.works/duet/pulsar-win/wire"
)

func acceptedCaptureQualityState() *protocol.CaptureQualityState {
	age, overruns := int64(18), int64(0)
	return &protocol.CaptureQualityState{
		Contract: protocol.CaptureQualityContract, Generation: 7,
		Workflow: protocol.CaptureWorkflowLivePTT, RequestedMode: protocol.CaptureRouteAuto,
		ResolvedMode: protocol.CaptureRouteSpeaker, Lifecycle: protocol.CaptureLifecycleCapturing,
		Quality: protocol.CaptureQualityAccepted, AEC: protocol.CaptureEffectActive,
		NS: protocol.CaptureEffectActive, AGC: protocol.CaptureEffectActive,
		InputHealth: protocol.CaptureHealthOK, Reason: "none",
		InputCeilingDBFS: protocol.CaptureInputCeilingDBFS, UpdatedMonotonicMS: 42420,
		ReferenceAgeMS: &age, ProcessorOverruns: &overruns,
	}
}

func TestWindowsCaptureQualityStateIsValidatedCopiedAndProjected(t *testing.T) {
	player, _, _ := newTestPlayer(t, newFakeDaemon(), fixedClock{ok: true})
	state := acceptedCaptureQualityState()
	if err := player.SetCaptureQualityState(state); err != nil {
		t.Fatal(err)
	}
	state.Quality = protocol.CaptureQualityUnsupported
	got := player.StatePayload(20).CaptureQuality
	if got == nil || got.Quality != protocol.CaptureQualityAccepted || got.ReferenceAgeMS == state.ReferenceAgeMS {
		t.Fatalf("capture state was not defensively copied: %+v", got)
	}
	capabilities, err := protocol.ParseCapabilitySet([]string{protocol.CapabilityCaptureQuality})
	if err != nil {
		t.Fatal(err)
	}
	presentation := player.CaptureQualityPresentation(capabilities)
	if !presentation.Available || presentation.ResolvedMode != protocol.CaptureRouteSpeaker ||
		presentation.AEC != protocol.CaptureEffectActive || presentation.InputCeilingDBFS != -3 ||
		presentation.OutputCeilingDBFS != -1 {
		t.Fatalf("incomplete Windows shell projection: %+v", presentation)
	}

	invalid := acceptedCaptureQualityState()
	invalid.ResolvedMode = protocol.CaptureRouteUnknown
	if err := player.SetCaptureQualityState(invalid); err == nil {
		t.Fatal("accepted unknown route was stored")
	}
}

func TestWindowsHeartbeatWithholdsUnadvertisedCaptureQuality(t *testing.T) {
	client := NewWSClient("ws://127.0.0.1/ws", Identity{
		NodeID: "a", Token: "token", AppVersion: "test",
		Capabilities: []string{protocol.CapabilitySeamlessAdoption},
	}, testLogger())
	client.StateProvider = func() protocol.StatePayload {
		return protocol.StatePayload{Playback: "stopped", Speakers: []protocol.Speaker{}, CaptureQuality: acceptedCaptureQualityState()}
	}
	if got := client.stateForHeartbeat(); got.CaptureQuality != nil {
		t.Fatal("unadvertised capture quality leaked into heartbeat")
	}
	client.identity.Capabilities = []string{protocol.CapabilityCaptureQuality}
	if got := client.stateForHeartbeat(); got.CaptureQuality == nil {
		t.Fatal("advertised valid capture quality was removed")
	}
}
