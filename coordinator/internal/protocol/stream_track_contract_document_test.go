package protocol

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type streamTrackContractDocument struct {
	Contract                             string `json:"contract"`
	Capability                           string `json:"capability"`
	ProductionMode                       string `json:"productionMode"`
	ProductionDecoderRegistrationAllowed bool   `json:"productionDecoderRegistrationAllowed"`
	Commands                             []string
	Events                               []string
	Timing                               struct {
		MinimumBufferedMS, LoadReadyTimeoutMS, SeekReadyTimeoutMS, StartDeadlineMS int64
	}
	Ordering struct {
		SequenceGapsAllowed, ReadyBeforeResumeRequired, ReadyBelowBufferBarrierAllowed bool
		TerminalClosesOnlyExactGeneration, RebufferRequiresNewReadyAndResume           bool
	}
	Variant struct {
		Selection, Manifest                                                     string
		URLCredentialsAllowed, ManifestGrantsAccess, ManifestCredentialsAllowed bool
		ClientCodecNegotiationAllowed, OriginalUploadDecoderInputAllowed        bool
	}
	MixedVersion struct {
		SenderSelectedPolicies                                            []string
		ClipFallbackAllowed, LateAutoplayAfterUpgradeAllowed              bool
		UnsupportedIsVisible, SupportedTargetsBlockedByUnsupportedTargets bool
	}
	Compatibility struct {
		ProtocolMajor                              int
		AdditiveTypesOnly, LegacyGoldensUnchanged  bool
		AdvertiseBeforePlayerImplementationAllowed bool
	}
}

func TestStreamTrackContractDocumentAndNoGoRuntimeBoundary(t *testing.T) {
	root := filepath.Clean(filepath.Join(goldenDir(t), "..", ".."))
	raw, err := os.ReadFile(filepath.Join(root, "protocol", "stream-track-v1.json"))
	if err != nil {
		t.Fatal(err)
	}
	var contract streamTrackContractDocument
	if err := json.Unmarshal(raw, &contract); err != nil {
		t.Fatal(err)
	}
	if contract.Contract != "p2-stream-track-wire.v1" || contract.Capability != CapabilityStreamTrack ||
		contract.ProductionMode != "disabled-by-p2-codec-player-adr-handoff.v1" ||
		contract.ProductionDecoderRegistrationAllowed {
		t.Fatalf("unsafe contract identity/no-go: %+v", contract)
	}
	if contract.Timing.MinimumBufferedMS != StreamMinimumBufferedMS ||
		contract.Timing.LoadReadyTimeoutMS != StreamLoadReadyTimeoutMS ||
		contract.Timing.SeekReadyTimeoutMS != StreamSeekReadyTimeoutMS ||
		contract.Timing.StartDeadlineMS != StreamStartDeadlineMS {
		t.Fatalf("timing drift: %+v", contract.Timing)
	}
	wantCommands := []string{TypeStreamLoad, TypeStreamResumeAt, TypeStreamSeek, TypeStreamPause, TypeStreamCancel}
	wantEvents := []string{TypeStreamReady, TypeStreamStarted, TypeStreamProgress, TypeStreamRebuffer, TypeStreamFailed, TypeStreamEnded, TypeStreamCancelled}
	if strings.Join(contract.Commands, ",") != strings.Join(wantCommands, ",") ||
		strings.Join(contract.Events, ",") != strings.Join(wantEvents, ",") {
		t.Fatalf("message catalog drift commands=%v events=%v", contract.Commands, contract.Events)
	}
	if contract.Ordering.SequenceGapsAllowed || !contract.Ordering.ReadyBeforeResumeRequired ||
		contract.Ordering.ReadyBelowBufferBarrierAllowed || !contract.Ordering.TerminalClosesOnlyExactGeneration ||
		!contract.Ordering.RebufferRequiresNewReadyAndResume {
		t.Fatalf("ordering weakened: %+v", contract.Ordering)
	}
	if contract.Variant.Selection != "server_pinned_profile_only" ||
		contract.Variant.Manifest != "opaque_server_issued" || contract.Variant.URLCredentialsAllowed ||
		contract.Variant.ManifestGrantsAccess || contract.Variant.ManifestCredentialsAllowed ||
		contract.Variant.ClientCodecNegotiationAllowed || contract.Variant.OriginalUploadDecoderInputAllowed {
		t.Fatalf("variant authority widened: %+v", contract.Variant)
	}
	if strings.Join(contract.MixedVersion.SenderSelectedPolicies, ",") !=
		StreamMixedVersionRequireAll+","+StreamMixedVersionSupportedOnlyWithReceipts ||
		contract.MixedVersion.ClipFallbackAllowed || contract.MixedVersion.LateAutoplayAfterUpgradeAllowed ||
		!contract.MixedVersion.UnsupportedIsVisible || contract.MixedVersion.SupportedTargetsBlockedByUnsupportedTargets {
		t.Fatalf("mixed-version policy weakened: %+v", contract.MixedVersion)
	}
	if contract.Compatibility.ProtocolMajor != Version || !contract.Compatibility.AdditiveTypesOnly ||
		!contract.Compatibility.LegacyGoldensUnchanged || contract.Compatibility.AdvertiseBeforePlayerImplementationAllowed {
		t.Fatalf("compatibility drift: %+v", contract.Compatibility)
	}

	for _, path := range []string{
		filepath.Join(root, "pulsar-win", "main.go"),
		filepath.Join(root, "node-app", "Sources", "NodeCore", "PlayerCore.swift"),
	} {
		source, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(source), "CapabilityStreamTrack") ||
			strings.Contains(string(source), "streamTrackCapability") {
			t.Fatalf("production composition advertises no-go capability: %s", path)
		}
	}
}
