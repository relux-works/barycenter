#!/usr/bin/env python3
"""Fail-closed validation for the Phase 2 streamed-track technical review."""

from __future__ import annotations

import hashlib
import json
import pathlib
import re


ROOT = pathlib.Path(__file__).resolve().parents[2]
REVIEW_PATH = ROOT / "acceptance/phase2/stream-performance-review-v1.json"
RUBRIC_PATH = ROOT / "acceptance/codec-spike/rubric-v1.json"
GATE_MATRIX_PATH = ROOT / "acceptance/phase2/gate-matrix-v1.json"
HANDOFF_PATH = ROOT / "acceptance/streamed-track-rollout-handoff-v1.json"
SHA256 = re.compile(r"[0-9a-f]{64}")


class ReviewError(ValueError):
    pass


def require(condition: bool, message: str) -> None:
    if not condition:
        raise ReviewError(message)


def load(path: pathlib.Path = REVIEW_PATH) -> dict:
    return json.loads(path.read_text(encoding="utf-8"))


def digest(path: pathlib.Path) -> str:
    return hashlib.sha256(path.read_bytes()).hexdigest()


def rubric_thresholds() -> dict[str, int]:
    rubric = json.loads(RUBRIC_PATH.read_text(encoding="utf-8"))
    gates = {item["metric"]: item for item in rubric["hardGates"]}
    return {
        "trackStartP95MS": gates["track_start_ms"]["limit"],
        "seekToAudioP95MS": gates["seek_to_audio_ms"]["limit"],
        "scheduledSkewP95MS": gates["scheduled_skew_ms"]["limit"],
        "peakRSSMiB": gates["peak_rss_mib"]["limit"],
        "durationRSSGrowthMiB": gates["duration_rss_growth_mib"]["limit"],
        "durationRSSSlopeMiBPerHour": gates["duration_rss_slope_mib_per_hour"]["limit"],
        "maximumDurationMS": max(item["durationSeconds"] for item in rubric["fixtureClasses"]) * 1000,
        "minimumMeasuredSamples": rubric["samplePlan"]["measuredSamplesPerTimedGroup"],
        "warmupsPerTimedGroup": rubric["samplePlan"]["warmupsPerTimedGroup"],
    }


def validate(review: dict) -> None:
    require(review.get("schemaVersion") == 1, "unsupported review schema")
    require(review.get("contract") == "p2-stream-performance-technical-review.v1", "wrong review contract")
    require(review.get("task") == "TASK-260712-28mn7w", "wrong review task")
    require(review.get("reviewedAt") == "2026-07-16", "review date drifted")
    require(re.fullmatch(r"[0-9a-f]{40}", review.get("reviewedBaseCommit", "")) is not None,
            "reviewed base commit missing")

    reviewer = review.get("reviewer", {})
    require(reviewer.get("independenceRequired") is True, "independent review requirement removed")
    require(reviewer.get("independenceSatisfied") is False, "current root session cannot claim independence")
    require(reviewer.get("independentApprovalTask") == "TASK-260716-3voo6j", "approval task drifted")
    require(reviewer.get("independentApprover") == "Ivan Oparin", "independent approver drifted")
    require(reviewer.get("approvalStatus") == "required", "independent approval falsely completed")

    decision = review.get("decision", {})
    require(decision.get("result") == "engineering-review-complete-production-blocked",
            "technical review result drifted")
    require(decision.get("repositoryPreflightPassed") is True, "repository preflight was lost")
    require(decision.get("productionPlaybackAllowed") is False, "production playback was allowed")
    require(decision.get("phase2PromotionAllowed") is False, "Phase 2 promotion was allowed")
    require(decision.get("manualPerformanceClaim") is False, "repository review claimed manual performance")
    require(decision.get("nextEngineeringTaskMayStart") is True,
            "owner-authorized strict engineering continuation was removed")

    expected_thresholds = rubric_thresholds()
    thresholds = review.get("thresholds", {})
    for key, expected in expected_thresholds.items():
        require(thresholds.get(key) == expected, f"threshold drifted: {key}")
    rubric = json.loads(RUBRIC_PATH.read_text(encoding="utf-8"))
    require(thresholds.get("requiredPairings") == rubric["requiredPairings"], "pairing matrix drifted")

    anchors = review.get("sourceAnchors", [])
    required_paths = {
        "acceptance/codec-spike/rubric-v1.json",
        "acceptance/codec-spike/stream-contract-v1.json",
        "acceptance/streamed-track-rollout-handoff-v1.json",
        "acceptance/phase2/gate-matrix-v1.json",
        "coordinator/internal/session/stream_flow.go",
        "coordinator/internal/store/stream_accounting.go",
        "pulsar-win/stream_player.go",
        "pulsar-win/stream_cache.go",
        "node-app/Sources/NodeCore/MacStreamTrackPlayer.swift",
        "node-app/Sources/NodeCore/MacStreamTrackCache.swift",
    }
    require({item.get("path") for item in anchors} == required_paths, "review source inventory drifted")
    for item in anchors:
        path = ROOT / item["path"]
        require(path.is_file(), f"review source missing: {item['path']}")
        require(SHA256.fullmatch(item.get("sha256", "")) is not None, "review source digest malformed")
        require(digest(path) == item["sha256"], f"review source digest mismatch: {item['path']}")

    handoff = json.loads(HANDOFF_PATH.read_text(encoding="utf-8"))
    activation = handoff.get("productionActivation", {})
    require(activation.get("currentValue") is False, "streamed-track production flag enabled")
    require(activation.get("activationAllowedNow") is False, "streamed-track activation allowed")
    require(activation.get("productionDecoderRegistry") == [], "production decoder registry is not empty")
    require(activation.get("wireCapabilityAdvertised") is False, "production capability advertised")
    gate_matrix = json.loads(GATE_MATRIX_PATH.read_text(encoding="utf-8"))
    require(gate_matrix.get("productionGate", {}).get("status") == "blocked", "Phase 2 gate no longer blocked")
    require(gate_matrix.get("productionGate", {}).get("streamedTracksActivationAllowed") is False,
            "Phase 2 matrix allows streamed tracks")

    player_source = (ROOT / "pulsar-win/stream_player.go").read_text(encoding="utf-8")
    player_test = (ROOT / "pulsar-win/stream_player_test.go").read_text(encoding="utf-8")
    require("player.decoderMu.Lock()\n\t\tplayer.decoderMu.Unlock()" in player_source,
            "Windows Close no longer joins decoder cleanup")
    require("TestWindowsStreamCandidateCloseJoinsDecoderCleanup" in player_test,
            "Windows Close regression test missing")

    reruns = review.get("reruns", [])
    require({item.get("id") for item in reruns} == {
        "windows-close-and-clock-stress", "windows-full-race",
        "coordinator-stream-accounting-race", "macos-stream-player",
        "stream-contracts", "hosted-windows-packaged-probe",
    }, "representative rerun inventory drifted")
    require(all(item.get("command") and item.get("claim") for item in reruns), "rerun evidence incomplete")
    require({item.get("status") for item in reruns} <= {"pass", "pass-after-related-fix"},
            "rerun status is not accepted")

    findings = review.get("findings", [])
    require([item.get("id") for item in findings] == [f"P2-PERF-{number:03d}" for number in range(1, 5)],
            "finding inventory drifted")
    require(all(item.get("severity") == "high" for item in findings), "high severity was lowered")
    require(findings[0].get("status") == "fixed-re-reviewed", "Windows lifecycle fix was reopened")
    require([item.get("status") for item in findings[1:]] == [
        "open-production-blocking", "open-manual-blocking", "open-external-review",
    ], "open high finding was silently closed")
    require(review.get("productionBlockers") == ["P2-PERF-002", "P2-PERF-003", "P2-PERF-004"],
            "production blocker inventory drifted")

    boundary = review.get("automatedEvidenceBoundary", {})
    require(len(boundary.get("proved", [])) == 6, "automated proof inventory incomplete")
    require(len(boundary.get("notProved", [])) == 5, "manual proof boundary incomplete")
    require(review.get("manualEpic") == "EPIC-260714-th54l3", "manual evidence seam drifted")
    require(review.get("nextEngineeringTask") == "TASK-260712-2sicfs", "strict next task drifted")


def main() -> int:
    review = load()
    validate(review)
    print(json.dumps({
        "contract": review["contract"],
        "decision": review["decision"]["result"],
        "fixedHigh": 1,
        "openHigh": 3,
        "independenceSatisfied": False,
        "productionPlaybackAllowed": False,
        "status": "pass",
    }, sort_keys=True))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
