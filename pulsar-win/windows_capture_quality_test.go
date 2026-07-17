package main

import (
	"math"
	"testing"

	protocol "relux.works/duet/pulsar-win/wire"
)

func TestWindowsCaptureQualityBoundedAGCAppliesCeilingLast(t *testing.T) {
	processor := newWindowsCaptureInputSafetyProcessor(48_000)
	loud := make([]float32, 48_000)
	for index := range loud {
		loud[index] = 1
	}
	metrics := processor.process(loud)
	if metrics.peakDBFS > -2.999 || metrics.appliedGainDB > 0.001 {
		t.Fatalf("input ceiling/gain drift: %+v", metrics)
	}
	ceiling := float32(math.Pow(10, windowsCaptureInputCeilingDBFS/20))
	for _, sample := range loud {
		if float32(math.Abs(float64(sample))) > ceiling+0.000001 {
			t.Fatal("sample escaped final input ceiling")
		}
	}

	processor.reset()
	var settled windowsCaptureInputMetrics
	for iteration := 0; iteration < 5; iteration++ {
		quiet := make([]float32, 48_000)
		for index := range quiet {
			quiet[index] = 0.001
		}
		settled = processor.process(quiet)
		if settled.appliedGainDB > float64(iteration+1)*windowsCaptureMaximumGainChangeDBPerSec+0.001 {
			t.Fatal("gain slew exceeded")
		}
	}
	if math.Abs(settled.appliedGainDB-windowsCaptureMaximumGainDB) > 0.001 {
		t.Fatalf("gain did not stop at maximum: %+v", settled)
	}
}

func TestWindowsCaptureQualityDecisionsStayHonest(t *testing.T) {
	request := windowsLiveCaptureQualityRequest()
	tests := []struct {
		name, route string
		verified    bool
		quality     string
		reason      string
	}{
		{"verified headphone", "headphone", true, protocol.CaptureQualityAccepted, "none"},
		{"unverified headphone", "headphone", false, protocol.CaptureQualityDegraded, "aec_unavailable"},
		{"verified speaker", "speaker", true, protocol.CaptureQualityDegraded, "reference_unavailable"},
		{"verified unknown", "unknown", true, protocol.CaptureQualityDegraded, "route_unknown"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			decision := evaluateWindowsCaptureQuality(request, test.route, test.verified)
			if decision.quality != test.quality || decision.reason != test.reason {
				t.Fatalf("unexpected decision: %+v", decision)
			}
			state := newWindowsCaptureQualitySession("live_ptt", request, test.route, test.verified).state("capturing")
			if err := protocol.ValidateCaptureQualityState(state); err != nil {
				t.Fatalf("decision produced invalid shared state: %v", err)
			}
		})
	}
	mismatch := evaluateWindowsCaptureQuality(
		WindowsCaptureQualityRequest{Mode: WindowsCaptureQualitySpeaker, ProcessingRequested: true, DegradedConsent: true},
		"headphone", true)
	if mismatch.reason != "route_excluded" {
		t.Fatalf("explicit mismatch was inferred accepted: %+v", mismatch)
	}
	legacy := evaluateWindowsCaptureQuality(WindowsCaptureQualityLegacy, "unknown", false)
	if legacy.reason != "user_selected_unprocessed" || legacy.agc != protocol.CaptureEffectUnavailable {
		t.Fatalf("legacy path was mislabeled: %+v", legacy)
	}
}

func TestWindowsCaptureQualityGenerationsAreFresh(t *testing.T) {
	first := newWindowsCaptureQualitySession("recorded_clip", WindowsCaptureQualityLegacy, "unknown", false)
	second := newWindowsCaptureQualitySession("local_self_test", WindowsCaptureQualityLegacy, "unknown", false)
	if second.generation != first.generation+1 {
		t.Fatalf("generation reused: %d then %d", first.generation, second.generation)
	}
}
