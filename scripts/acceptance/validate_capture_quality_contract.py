#!/usr/bin/env python3
"""Fail-closed validation for the frozen P3 capture-quality/C3 contract."""

from __future__ import annotations

import json
import pathlib


ROOT = pathlib.Path(__file__).resolve().parents[2]
PROTOCOL_PATH = ROOT / "protocol" / "capture-quality-v1.json"
EVIDENCE_PATH = ROOT / "acceptance" / "phase3" / "capture-quality-contract-v1.json"


class CaptureQualityContractError(ValueError):
    pass


def require(condition: bool, message: str) -> None:
    if not condition:
        raise CaptureQualityContractError(message)


def load(path: pathlib.Path) -> dict:
    return json.loads(path.read_text(encoding="utf-8"))


def validate_protocol(contract: dict) -> None:
    require(contract.get("schemaVersion") == 1, "unsupported protocol schema")
    require(contract.get("contract") == "capture-quality.v1", "wrong protocol contract")
    require(contract.get("task") == "TASK-260712-1gmsvh", "wrong task")
    require(contract.get("publishedAt") == "2026-07-17", "publication date drifted")
    require(
        contract.get("productionMode")
        == "disabled_pending_platform_implementation_and_c3_evidence",
        "production enabled before implementation/C3 evidence",
    )

    capability = contract.get("capability", {})
    require(capability.get("name") == "capture_quality_v1", "capability drifted")
    require(capability.get("currentlyAdvertised") is False, "capability advertised early")
    require(len(capability.get("advertiseOnlyWhen", [])) == 4, "advertisement gate incomplete")
    require(
        set(capability.get("doesNotAuthorize", []))
        == {
            "remote microphone start",
            "coordinator-selected route or effect settings",
            "coordinator-selected input or output ceiling",
            "an acoustic C3 or hardware claim",
        },
        "capability authority widened",
    )

    workflows = contract.get("workflows", [])
    require(
        [item.get("id") for item in workflows]
        == ["recorded_clip", "local_self_test", "live_ptt"],
        "microphone workflow coverage drifted",
    )
    require(workflows[0].get("maximumDurationMS") == 180000, "clip bound drifted")
    require(workflows[1].get("maximumDurationMS") == 5000, "self-test bound drifted")
    require(workflows[2].get("maximumDurationMS") == 300000, "live bound drifted")
    require(
        [item.get("networkDuringCapture") for item in workflows] == [False, False, True],
        "workflow network boundary drifted",
    )

    ownership = contract.get("ownership", {})
    require(
        set(ownership.get("realtimeForbidden", []))
        == {"blocking locks", "allocation", "filesystem I/O", "network I/O", "logging"},
        "realtime callback boundary weakened",
    )
    require("generation" in ownership.get("generationRule", ""), "generation rule missing")

    graph = contract.get("graph", {})
    expected_order = [
        "explicit_indicator_and_generation",
        "device_capture",
        "capture_clock_and_format_alignment",
        "render_reference_alignment",
        "acoustic_echo_cancellation",
        "noise_suppression",
        "bounded_input_agc",
        "workflow_sink",
        "terminal_teardown",
    ]
    require(graph.get("captureOrder") == expected_order, "shared graph order drifted")
    require(graph.get("commonProcessorRequired") is True, "common processor not required")
    require(
        graph.get("parallelWorkflowImplementationsAllowed") is False,
        "parallel workflow DSP implementations allowed",
    )
    stages = graph.get("stages", [])
    require([stage.get("id") for stage in stages] == expected_order, "graph stage inventory drifted")
    require(stages[0].get("mayBypass") is False, "indicator may be bypassed")
    require(stages[-1].get("mayBypass") is False, "teardown may be bypassed")

    reference = contract.get("renderReference", {})
    require(reference.get("owner") == "local playback render graph", "reference owner drifted")
    require("immediately before device submission" in reference.get("tap", ""), "reference tap drifted")
    alignment = reference.get("alignment", {})
    require(
        alignment
        == {
            "captureFormat": "48000 Hz mono float32",
            "maximumSearchWindowMS": 250,
            "maximumReferenceAgeMS": 100,
            "maximumClockDriftPPM": 200,
            "discontinuityAction": "stop committing output, advance generation and enter reconfiguring",
        },
        "reference timing/alignment drifted",
    )
    require("never logged" in reference.get("privacy", ""), "reference privacy weakened")

    routes = contract.get("routes", {})
    require(routes.get("requestedModes") == ["auto", "speaker", "headphone"], "route modes drifted")
    require(routes.get("resolvedModes") == ["speaker", "headphone", "unknown"], "resolved routes drifted")
    require(
        routes.get("speakerAcceptedRequires")
        == ["eligible synchronized render reference", "aec active", "ns active", "agc active"],
        "speaker acceptance weakened",
    )
    require("positively identified" in routes.get("headphoneAEC", ""), "headphone AEC bypass widened")
    require(routes.get("deviceOrRouteChange", {}).get("maximumRearmMS") == 1500, "re-arm bound drifted")
    require("never resume raw" in routes.get("deviceOrRouteChange", {}).get("onTimeout", ""), "raw route fallback allowed")

    states = contract.get("states", {})
    require(
        states.get("quality") == ["accepted", "degraded", "unsupported"],
        "quality vocabulary drifted",
    )
    require(
        states.get("effect") == ["active", "not_required", "unavailable", "faulted"],
        "effect vocabulary drifted",
    )
    require("preparing" in states.get("indicatorRule", ""), "indicator does not precede capture")
    require("clears only after" in states.get("indicatorRule", ""), "indicator clears before teardown")

    ceilings = contract.get("ceilings", {})
    require(
        ceilings.get("inputAGC")
        == {
            "stage": "before every workflow sink",
            "targetRMSDBFS": -20.0,
            "targetToleranceDB": 3.0,
            "peakCeilingDBFS": -3.0,
            "maximumDigitalGainDB": 12.0,
            "maximumGainChangeDBPerSecond": 3.0,
            "coordinatorMutable": False,
        },
        "input AGC contract drifted",
    )
    receiver = ceilings.get("receiverOutput", {})
    require(receiver.get("peakCeilingDBFS") == -1.0, "receiver ceiling drifted")
    require(receiver.get("controlledBy") == "recipient local application", "receiver ceiling authority drifted")
    require(receiver.get("inputAGCMayOverride") is False, "input AGC can override output ceiling")

    fallback = contract.get("fallback", {})
    require("per-session local consent" in fallback.get("degraded", ""), "degraded fallback is not explicit")
    require(fallback.get("unsupported") == "do not commit or send microphone samples", "unsupported capture allowed")
    require("automatic live-to-clip fallback is forbidden" in fallback.get("live_ptt", ""), "automatic live fallback allowed")

    surfaces = contract.get("surfaces", {})
    require(surfaces.get("registerCapability") == "capture_quality_v1", "surface capability drifted")
    heartbeat = surfaces.get("heartbeatExtension", {})
    require(
        heartbeat.get("requiredFields")
        == [
            "contract", "generation", "workflow", "requested_mode", "resolved_mode",
            "lifecycle", "quality", "aec", "ns", "agc", "input_health", "reason",
            "input_ceiling_dbfs", "updated_monotonic_ms",
        ],
        "heartbeat field contract drifted",
    )
    require(
        set(heartbeat.get("forbiddenFields", []))
        == {"audio bytes", "render-reference bytes", "device id", "device name", "absolute path", "filename", "transcript", "raw level samples"},
        "heartbeat privacy boundary drifted",
    )
    history = surfaces.get("history", {})
    require(history.get("newPersistentCaptureQualityObject") is False, "transient quality history persisted")
    require(len(history.get("allowedTerminalCodes", [])) == 4, "terminal code inventory drifted")

    privacy = contract.get("privacy", {})
    for key in (
        "remoteCaptureAuthority",
        "indicatorMayBeWeakened",
        "persistRawMicrophoneForDiagnostics",
        "persistRenderReference",
    ):
        require(privacy.get(key) is False, f"privacy invariant weakened: {key}")
    require(contract.get("rollback", {}).get("silentClaimRetentionAllowed") is False, "rollback retains false claim")


def validate_evidence(evidence: dict) -> None:
    require(evidence.get("schemaVersion") == 1, "unsupported evidence schema")
    require(
        evidence.get("contract") == "p3-capture-quality-contract-evidence.v1",
        "wrong evidence contract",
    )
    require(evidence.get("task") == "TASK-260712-1gmsvh", "wrong evidence task")
    decision = evidence.get("decision", {})
    require(
        decision
        == {
            "result": "contract-frozen-implementation-and-manual-c3-required",
            "protocolFrozen": True,
            "productionReady": False,
            "c3Accepted": False,
            "manualEvidence": "not-run",
            "manualEpic": "EPIC-260714-th54l3",
        },
        "decision invents readiness or evidence",
    )

    expected_requirements = {
        "p3.2-aec", "p3.2-ns", "p3.2-agc", "p3.2-routes", "p3.2-health",
        "p3.2-parity", "c3-no-return-echo", "c3-preserve-near-end", "c3-visible-degradation",
    }
    requirements = evidence.get("requirements", [])
    require({item.get("id") for item in requirements} == expected_requirements, "requirement mapping incomplete")
    for item in requirements:
        for key in ("graphSeam", "state", "negative", "evidence"):
            require(item.get(key), f"requirement {item.get('id')} lacks {key}")

    metrics = evidence.get("objectiveRubric", {}).get("metrics", {})
    require(
        metrics
        == {
            "farEndOnly": {"convergenceMS": 2000, "medianERLEMinDB": 18.0, "p10ERLEMinDB": 10.0, "residualRMSMaxDBFS": -45.0},
            "nearEndOnly": {"absoluteLevelChangeMaxDB": 3.0, "stoiDeltaMin": -0.05},
            "doubleTalk": {"nearEndAttenuationMaxDB": 3.0, "stoiDeltaMin": -0.05, "medianERLEMinDB": 6.0},
            "noiseSuppression": {"snrImprovementMinDB": 6.0, "nearEndAttenuationMaxDB": 3.0},
            "inputAGC": {"targetRMSDBFS": -20.0, "toleranceDB": 3.0, "peakMaxDBFS": -3.0, "maximumGainDB": 12.0, "maximumGainChangeDBPerSecond": 3.0},
            "clipping": {"clippedSampleFractionMax": 0.001},
            "processor": {"addedLatencyP95MaxMS": 20.0, "callbackAllocationMax": 0, "callbackBlockingWaitMax": 0},
            "routeChange": {"committedSamplesDuringReconfigureMax": 0, "maximumRearmMS": 1500},
            "clockDrift": {"fixtureRangePPM": 200, "referenceAgeMaxMS": 100},
        },
        "objective C3 thresholds drifted",
    )
    require("never averaged" in evidence.get("objectiveRubric", {}).get("rule", ""), "failed cells may be averaged away")

    listening = evidence.get("blindedListeningRubric", {})
    require(listening.get("status") == "not-run", "blinded result invented")
    require(listening.get("minimumIndependentListeners") == 3, "listener count drifted")
    require(listening.get("repetitionsPerCell") == 2, "listening repetitions drifted")
    require("two consecutive words" in listening.get("intelligibleEchoDefinition", ""), "echo definition drifted")
    require("zero intelligible-echo" in listening.get("acceptedCellPass", ""), "accepted listening gate weakened")

    expected_cases = {
        "far_end_only", "near_end_only", "double_talk", "echo_path_change", "route_change",
        "clock_drift", "clipping", "too_quiet", "silence", "device_loss", "processor_overrun",
        "missing_reference", "effect_failure", "platform_route_matrix",
    }
    matrix = evidence.get("matrix", [])
    require({item.get("id") for item in matrix} == expected_cases, "C3/negative matrix incomplete")
    all_workflows = {"recorded_clip", "local_self_test", "live_ptt"}
    for item in matrix:
        require(set(item.get("workflows", [])) == all_workflows, f"workflow omitted: {item.get('id')}")
        require(item.get("routes") and item.get("objective"), f"matrix cell incomplete: {item.get('id')}")

    manual = evidence.get("manualEvidence", {})
    require(manual.get("status") == "not-run", "manual status invented")
    require(set(manual.values()) == {"not-run"}, "manual evidence invented")
    owners = evidence.get("downstreamOwners", {})
    require(owners.get("manualC3Evidence") == "TASK-260712-2e80pr", "manual C3 owner drifted")
    require(set(owners.get("platformProbes", [])) == {"TASK-260712-265o0f", "TASK-260712-2gaswa"}, "probe ownership drifted")


def validate_repository_boundaries() -> None:
    runtime_capability_sources = [
        ROOT / "coordinator" / "internal" / "protocol" / "protocol.go",
        ROOT / "pulsar-win" / "wire" / "protocol.go",
        ROOT / "pulsar-win" / "main.go",
        ROOT / "node-app" / "Sources" / "NodeCore" / "Protocol.swift",
        ROOT / "node-app" / "Sources" / "NodeCore" / "PlayerCore.swift",
    ]
    for path in runtime_capability_sources:
        require("capture_quality_v1" not in path.read_text(encoding="utf-8"), f"runtime advertises frozen-only capability: {path.relative_to(ROOT)}")

    windows = (ROOT / "pulsar-win" / "media_clip.go").read_text(encoding="utf-8")
    macos = (ROOT / "node-app" / "Sources" / "NodeCore" / "AudioEngine.swift").read_text(encoding="utf-8")
    require("LimiterCeilingDB: -1" in windows, "Windows receiver ceiling anchor drifted")
    require("local -1 dBFS ceiling" in macos, "macOS receiver ceiling anchor drifted")

    document = (ROOT / "docs" / "analysis" / "p3-capture-quality-contract-v1.md").read_text(encoding="utf-8")
    for fragment in (
        "One processor, three workflows",
        "Render-reference ownership and timing",
        "Two ceilings",
        "Protocol, heartbeat and history decision",
        "Those are acceptance targets, not current results",
        "EPIC-260714-th54l3",
    ):
        require(fragment in document, f"decision disclosure missing: {fragment}")

    specification = (ROOT / "docs" / "spec-self-contained-audio.md").read_text(encoding="utf-8")
    require(
        "p3-capture-quality-contract-v1.md" in specification,
        "authoritative capture-quality contract link missing from specification",
    )

    diagram = (ROOT / "docs" / "diagrams" / "p3-capture-quality-shared-graph.puml").read_text(encoding="utf-8")
    require(diagram.startswith("@startuml") and diagram.rstrip().endswith("@enduml"), "diagram boundary invalid")
    for fragment in (
        "Clock + 48 kHz mono",
        "Reference alignment",
        "AEC",
        "Noise suppression",
        "Bounded input AGC",
        "-1 dBFS ceiling",
        "Recorded clip",
        "Five-second local",
        "Live PTT",
    ):
        require(fragment in diagram, f"shared graph diagram incomplete: {fragment}")


def main() -> int:
    protocol = load(PROTOCOL_PATH)
    evidence = load(EVIDENCE_PATH)
    validate_protocol(protocol)
    validate_evidence(evidence)
    validate_repository_boundaries()
    print(json.dumps({
        "contract": protocol["contract"],
        "decision": evidence["decision"]["result"],
        "manualEvidence": evidence["decision"]["manualEvidence"],
        "status": "pass",
    }, sort_keys=True))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
