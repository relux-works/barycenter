package main

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	protocol "relux.works/duet/pulsar-win/wire"
)

var captureQualityIntegratedFixtureIDs = []string{
	"far_end_only", "near_end_only", "double_talk", "echo_path_change",
	"route_change", "clock_drift", "clipping", "too_quiet", "silence",
	"device_loss", "processor_overrun", "missing_reference", "effect_failure",
	"live_packet_cancel",
}

func TestWindowsCaptureQualityIntegratedAdapter(t *testing.T) {
	corpus := os.Getenv("CAPTURE_QUALITY_CORPUS")
	output := os.Getenv("CAPTURE_QUALITY_ADAPTER_OUTPUT")
	if corpus == "" || output == "" {
		t.Skip("run through scripts/capture_quality/run_integrated.py")
	}
	if protocol.CaptureInputCeilingDBFS != -3 || protocol.CaptureOutputCeilingDBFS != -1 {
		t.Fatalf("capture/receiver ceilings inverted or changed: input=%v output=%v",
			protocol.CaptureInputCeilingDBFS, protocol.CaptureOutputCeilingDBFS)
	}
	started := time.Now()
	latencies := make([]float64, 0, 4096)
	cells := make([]map[string]any, 0, 9)
	generations := map[int64]bool{}
	for _, workflow := range []string{
		protocol.CaptureWorkflowRecordedClip,
		protocol.CaptureWorkflowLocalSelfTest,
		protocol.CaptureWorkflowLivePTT,
	} {
		for _, route := range []string{
			protocol.CaptureRouteSpeaker,
			protocol.CaptureRouteHeadphone,
			protocol.CaptureRouteUnknown,
		} {
			request := WindowsCaptureQualityRequest{Mode: WindowsCaptureQualityAuto, ProcessingRequested: true}
			decision := evaluateWindowsCaptureQuality(request, route, false)
			if decision.quality != protocol.CaptureQualityDegraded || decision.reason != "aec_unavailable" {
				t.Fatalf("production-unverified decision for %s/%s=%+v", workflow, route, decision)
			}
			session := newWindowsCaptureQualitySession(workflow, request, route, false)
			if generations[session.generation] {
				t.Fatalf("generation %d reused", session.generation)
			}
			generations[session.generation] = true
			for _, state := range []*protocol.CaptureQualityState{
				session.state(protocol.CaptureLifecyclePreparing),
				session.failedState(protocol.CaptureHealthOK, "aec_unavailable"),
			} {
				if err := protocol.ValidateCaptureQualityState(state); err != nil {
					t.Fatalf("%s/%s state invalid: %v", workflow, route, err)
				}
			}
			consented := request
			consented.DegradedConsent = true
			consentedSession := newWindowsCaptureQualitySession(workflow, consented, route, false)
			for _, lifecycle := range []string{
				protocol.CaptureLifecyclePreparing,
				protocol.CaptureLifecycleCapturing,
				protocol.CaptureLifecycleStopping,
			} {
				if err := protocol.ValidateCaptureQualityState(consentedSession.state(lifecycle)); err != nil {
					t.Fatalf("%s/%s consented %s invalid: %v", workflow, route, lifecycle, err)
				}
			}

			cases := make([]map[string]any, 0, len(captureQualityIntegratedFixtureIDs))
			for _, fixtureID := range captureQualityIntegratedFixtureIDs {
				samples := readCaptureQualityFloat32Fixture(t, filepath.Join(corpus, fixtureID+".capture.f32le"))
				processor := newWindowsCaptureInputSafetyProcessor(48_000)
				hash := sha256.New()
				maximumPeak := 0.0
				maximumGain := 0.0
				maximumSlew := 0.0
				clipped := 0
				previousGain := processor.gainDB
				for start := 0; start < len(samples); start += 480 {
					end := min(start+480, len(samples))
					block := samples[start:end]
					before := time.Now()
					metrics := processor.process(block)
					latencies = append(latencies, float64(time.Since(before).Nanoseconds())/1_000_000)
					maximumPeak = math.Max(maximumPeak, math.Pow(10, metrics.peakDBFS/20))
					maximumGain = math.Max(maximumGain, processor.gainDB)
					seconds := float64(len(block)) / 48_000
					if seconds > 0 {
						maximumSlew = math.Max(maximumSlew, math.Abs(processor.gainDB-previousGain)/seconds)
					}
					previousGain = processor.gainDB
					for _, sample := range block {
						if !isFiniteCaptureSample(sample) || math.Abs(float64(sample)) > math.Pow(10, -3.0/20.0)+0.000_001 {
							t.Fatalf("%s/%s/%s escaped finite -3 dBFS ceiling", workflow, route, fixtureID)
						}
						if math.Abs(float64(sample)) >= math.Pow(10, -3.0/20.0)-1e-7 {
							clipped++
						}
						var bytes [4]byte
						binary.LittleEndian.PutUint32(bytes[:], math.Float32bits(sample))
						_, _ = hash.Write(bytes[:])
					}
				}
				maximumPeakDBFS := windowsCaptureDB(maximumPeak)
				safetyPassed := maximumPeakDBFS <= windowsCaptureInputCeilingDBFS+0.001 &&
					maximumGain <= windowsCaptureMaximumGainDB+0.001 &&
					maximumSlew <= windowsCaptureMaximumGainChangeDBPerSec+0.001
				if !safetyPassed {
					t.Fatalf("%s/%s/%s safety peak=%f gain=%f slew=%f", workflow, route, fixtureID, maximumPeakDBFS, maximumGain, maximumSlew)
				}
				cases = append(cases, map[string]any{
					"id": fixtureID, "sampleCount": len(samples),
					"processedSHA256":            hex.EncodeToString(hash.Sum(nil)),
					"maximumPeakDBFS":            roundCaptureMetric(maximumPeakDBFS),
					"maximumAppliedGainDB":       roundCaptureMetric(maximumGain),
					"maximumGainSlewDBPerSecond": roundCaptureMetric(maximumSlew),
					"clippedFraction":            roundCaptureMetric(float64(clipped) / float64(max(1, len(samples)))),
					"safetyStagePassed":          true,
					"c3Status":                   "unsupported-native-effects-not-exercised",
				})
			}
			cells = append(cells, map[string]any{
				"workflow": workflow, "route": route,
				"quality": decision.quality, "reason": decision.reason,
				"supported": false, "blocker": "native-effects-unverified",
				"failClosedWithoutConsent": true, "freshGeneration": session.generation,
				"cases": cases,
			})
		}
	}

	probe := make([]float32, 480)
	processor := newWindowsCaptureInputSafetyProcessor(48_000)
	allocations := testing.AllocsPerRun(50, func() { processor.process(probe) })
	if allocations != 0 {
		t.Fatalf("Windows safety processor allocated %.2f times per callback-sized block", allocations)
	}
	sort.Float64s(latencies)
	report := map[string]any{
		"schemaVersion":     1,
		"contract":          "p3-capture-quality-platform-adapter.v1",
		"platform":          "windows",
		"build":             os.Getenv("CAPTURE_QUALITY_BUILD"),
		"fixtureLockSHA256": os.Getenv("CAPTURE_QUALITY_FIXTURE_LOCK_SHA256"),
		"manualEvidence":    "not-run",
		"cells":             cells,
		"runtime": map[string]any{
			"adapterDurationMS":            time.Since(started).Milliseconds(),
			"processorBlockLatencyP95MS":   capturePercentile(latencies, 0.95),
			"callbackAllocations":          allocations,
			"callbackBlockingWaits":        0,
			"measurementSource":            "repository-test-adapter",
			"physicalCPUAndMemoryEvidence": "not-run",
		},
	}
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(output, append(data, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
}

func readCaptureQualityFloat32Fixture(t *testing.T, path string) []float32 {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(data)%4 != 0 {
		t.Fatalf("%s is not float32 little-endian", filepath.Base(path))
	}
	result := make([]float32, len(data)/4)
	for index := range result {
		result[index] = math.Float32frombits(binary.LittleEndian.Uint32(data[index*4:]))
		if !isFiniteCaptureSample(result[index]) || math.Abs(float64(result[index])) > 1 {
			t.Fatalf("%s contains invalid sample", filepath.Base(path))
		}
	}
	return result
}

func isFiniteCaptureSample(value float32) bool {
	return !math.IsNaN(float64(value)) && !math.IsInf(float64(value), 0)
}

func roundCaptureMetric(value float64) float64 {
	return math.Round(value*1_000_000) / 1_000_000
}

func capturePercentile(values []float64, fraction float64) float64 {
	if len(values) == 0 {
		return 0
	}
	index := int(math.Floor(float64(len(values)-1) * fraction))
	return roundCaptureMetric(values[index])
}
