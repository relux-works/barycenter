package main

import (
	"math"
	"sync/atomic"
	"time"

	protocol "relux.works/duet/pulsar-win/wire"
)

type WindowsCaptureQualityMode string

const (
	WindowsCaptureQualityAuto      WindowsCaptureQualityMode = "auto"
	WindowsCaptureQualitySpeaker   WindowsCaptureQualityMode = "speaker"
	WindowsCaptureQualityHeadphone WindowsCaptureQualityMode = "headphone"
)

type WindowsCaptureQualityRequest struct {
	Mode                WindowsCaptureQualityMode
	ProcessingRequested bool
	DegradedConsent     bool
}

var WindowsCaptureQualityLegacy = WindowsCaptureQualityRequest{
	Mode: WindowsCaptureQualityAuto, ProcessingRequested: false, DegradedConsent: true,
}

func windowsLiveCaptureQualityRequest() WindowsCaptureQualityRequest {
	return WindowsCaptureQualityRequest{Mode: WindowsCaptureQualityAuto, ProcessingRequested: true}
}

type WindowsCaptureQualityRouteResolver interface {
	ResolvedCaptureQualityMode() string
}

type windowsUnknownCaptureQualityRoute struct{}

func (windowsUnknownCaptureQualityRoute) ResolvedCaptureQualityMode() string { return "unknown" }

type windowsCaptureQualityDecision struct {
	quality string
	aec     string
	ns      string
	agc     string
	reason  string
}

func evaluateWindowsCaptureQuality(
	request WindowsCaptureQualityRequest,
	resolvedMode string,
	nativeEffectsVerified bool,
) windowsCaptureQualityDecision {
	if !request.ProcessingRequested {
		return windowsCaptureQualityDecision{
			quality: protocol.CaptureQualityDegraded,
			aec:     protocol.CaptureEffectUnavailable, ns: protocol.CaptureEffectUnavailable,
			agc: protocol.CaptureEffectUnavailable, reason: "user_selected_unprocessed",
		}
	}
	if !nativeEffectsVerified {
		return windowsCaptureQualityDecision{
			quality: protocol.CaptureQualityDegraded,
			aec:     protocol.CaptureEffectUnavailable, ns: protocol.CaptureEffectUnavailable,
			agc: protocol.CaptureEffectActive, reason: "aec_unavailable",
		}
	}
	if request.Mode != WindowsCaptureQualityAuto && string(request.Mode) != resolvedMode {
		return windowsCaptureQualityDecision{
			quality: protocol.CaptureQualityDegraded,
			aec:     protocol.CaptureEffectActive, ns: protocol.CaptureEffectActive,
			agc: protocol.CaptureEffectActive, reason: "route_excluded",
		}
	}
	if resolvedMode == "unknown" {
		return windowsCaptureQualityDecision{
			quality: protocol.CaptureQualityDegraded,
			aec:     protocol.CaptureEffectActive, ns: protocol.CaptureEffectActive,
			agc: protocol.CaptureEffectActive, reason: "route_unknown",
		}
	}
	if resolvedMode == "speaker" {
		return windowsCaptureQualityDecision{
			quality: protocol.CaptureQualityDegraded,
			aec:     protocol.CaptureEffectActive, ns: protocol.CaptureEffectActive,
			agc: protocol.CaptureEffectActive, reason: "reference_unavailable",
		}
	}
	return windowsCaptureQualityDecision{
		quality: protocol.CaptureQualityAccepted,
		aec:     protocol.CaptureEffectActive, ns: protocol.CaptureEffectActive,
		agc: protocol.CaptureEffectActive, reason: "none",
	}
}

var windowsCaptureQualityGeneration atomic.Int64
var windowsCaptureQualityStartedAt = time.Now()

type windowsCaptureQualitySession struct {
	generation   int64
	workflow     string
	request      WindowsCaptureQualityRequest
	resolvedMode string
	decision     windowsCaptureQualityDecision
}

func newWindowsCaptureQualitySession(
	workflow string,
	request WindowsCaptureQualityRequest,
	resolvedMode string,
	nativeEffectsVerified bool,
) windowsCaptureQualitySession {
	if request.Mode == "" {
		request = WindowsCaptureQualityLegacy
	}
	if resolvedMode != "speaker" && resolvedMode != "headphone" {
		resolvedMode = "unknown"
	}
	return windowsCaptureQualitySession{
		generation: windowsCaptureQualityGeneration.Add(1), workflow: workflow,
		request: request, resolvedMode: resolvedMode,
		decision: evaluateWindowsCaptureQuality(request, resolvedMode, nativeEffectsVerified),
	}
}

func (s windowsCaptureQualitySession) state(lifecycle string) *protocol.CaptureQualityState {
	return &protocol.CaptureQualityState{
		Contract: protocol.CaptureQualityContract, Generation: s.generation,
		Workflow: s.workflow, RequestedMode: string(s.request.Mode),
		ResolvedMode: s.resolvedMode, Lifecycle: lifecycle,
		Quality: s.decision.quality, AEC: s.decision.aec, NS: s.decision.ns,
		AGC: s.decision.agc, InputHealth: protocol.CaptureHealthOK,
		Reason: s.decision.reason, InputCeilingDBFS: protocol.CaptureInputCeilingDBFS,
		UpdatedMonotonicMS: max(int64(1), time.Since(windowsCaptureQualityStartedAt).Milliseconds()),
	}
}

func (s windowsCaptureQualitySession) withNativeResult(
	communicationsCategoryActive bool,
	nativeEffectsVerified bool,
) windowsCaptureQualitySession {
	// Category activation is useful engineering telemetry, but it is not used
	// as evidence of AEC/NS. Only the independently verified flag may promote a
	// route. Keep the parameter explicit so callers cannot conflate the two.
	_ = communicationsCategoryActive
	s.decision = evaluateWindowsCaptureQuality(s.request, s.resolvedMode, nativeEffectsVerified)
	return s
}

func (s windowsCaptureQualitySession) failedState(inputHealth, reason string) *protocol.CaptureQualityState {
	state := s.state(protocol.CaptureLifecycleFailed)
	state.Quality = protocol.CaptureQualityDegraded
	state.InputHealth = inputHealth
	state.Reason = reason
	return state
}

type windowsCaptureInputMetrics struct {
	rmsDBFS         float64
	peakDBFS        float64
	appliedGainDB   float64
	clippedFraction float64
}

const (
	windowsCaptureTargetRMSDBFS             = -20.0
	windowsCaptureTargetToleranceDB         = 3.0
	windowsCaptureMaximumGainDB             = 12.0
	windowsCaptureMaximumGainChangeDBPerSec = 3.0
	windowsCaptureInputCeilingDBFS          = -3.0
)

type windowsCaptureInputSafetyProcessor struct {
	sampleRate float64
	gainDB     float64
}

func newWindowsCaptureInputSafetyProcessor(sampleRate float64) *windowsCaptureInputSafetyProcessor {
	if sampleRate <= 0 {
		sampleRate = WindowsCaptureSampleRate
	}
	return &windowsCaptureInputSafetyProcessor{sampleRate: sampleRate}
}

func (p *windowsCaptureInputSafetyProcessor) reset() { p.gainDB = 0 }

func (p *windowsCaptureInputSafetyProcessor) process(samples []float32) windowsCaptureInputMetrics {
	if len(samples) == 0 {
		return windowsCaptureInputMetrics{rmsDBFS: -240, peakDBFS: -240, appliedGainDB: p.gainDB}
	}
	var sum, inputPeak float64
	for _, sample := range samples {
		value := float64(sample)
		sum += value * value
		inputPeak = math.Max(inputPeak, math.Abs(value))
	}
	inputRMS := math.Sqrt(sum / float64(len(samples)))
	desired := math.Min(windowsCaptureMaximumGainDB, windowsCaptureTargetRMSDBFS-windowsCaptureDB(inputRMS))
	maximumStep := windowsCaptureMaximumGainChangeDBPerSec * float64(len(samples)) / p.sampleRate
	p.gainDB += math.Min(math.Max(desired-p.gainDB, -maximumStep), maximumStep)
	gain := math.Pow(10, p.gainDB/20)
	ceiling := math.Pow(10, windowsCaptureInputCeilingDBFS/20)
	ceilingScale := 1.0
	if inputPeak > 0 {
		ceilingScale = math.Min(1, ceiling/(inputPeak*gain))
	}
	finalGain := gain * ceilingScale
	var outputSum, outputPeak float64
	var clipped int
	for index, sample := range samples {
		value := math.Min(ceiling, math.Max(-ceiling, float64(sample)*finalGain))
		samples[index] = float32(value)
		outputSum += value * value
		outputPeak = math.Max(outputPeak, math.Abs(value))
		if math.Abs(value) >= ceiling-1e-7 {
			clipped++
		}
	}
	return windowsCaptureInputMetrics{
		rmsDBFS:         windowsCaptureDB(math.Sqrt(outputSum / float64(len(samples)))),
		peakDBFS:        windowsCaptureDB(outputPeak),
		appliedGainDB:   p.gainDB + 20*math.Log10(math.Max(ceilingScale, 1e-12)),
		clippedFraction: float64(clipped) / float64(len(samples)),
	}
}

func windowsCaptureDB(value float64) float64 {
	return 20 * math.Log10(math.Max(value, 1e-12))
}
