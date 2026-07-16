#!/usr/bin/env python3
"""Fail-closed validation for the final Phase 2 engineering handoff."""

from __future__ import annotations

import hashlib
import json
import pathlib
import subprocess


ROOT = pathlib.Path(__file__).resolve().parents[2]
HANDOFF_PATH = ROOT / "acceptance/phase2/engineering-handoff-v1.json"


class HandoffError(ValueError):
    pass


def require(condition: bool, message: str) -> None:
    if not condition:
        raise HandoffError(message)


def load(path: pathlib.Path = HANDOFF_PATH) -> dict:
    return json.loads(path.read_text(encoding="utf-8"))


def git(*args: str, binary: bool = False):
    return subprocess.run(
        ["git", *args], cwd=ROOT, check=True, capture_output=True,
        text=not binary,
    ).stdout


def snapshot_bytes(commit: str, path: str) -> bytes:
    return git("show", f"{commit}:{path}", binary=True)


def snapshot_json(commit: str, path: str) -> dict:
    return json.loads(snapshot_bytes(commit, path))


def validate(handoff: dict) -> None:
    require(handoff.get("schemaVersion") == 1, "unsupported schema")
    require(handoff.get("contract") == "p2-engineering-handoff.v1", "wrong contract")
    require(handoff.get("task") == "TASK-260712-3a0cf9", "wrong task")
    require(handoff.get("publishedAt") == "2026-07-16", "publication date drifted")

    baseline = handoff.get("handoffBaselineCommit", "")
    root_source = handoff.get("rootReviewedSourceCommit", "")
    root_tree = handoff.get("rootReviewedSourceTree", "")
    require(git("rev-parse", baseline).strip() == baseline, "handoff baseline unavailable")
    require(git("merge-base", "--is-ancestor", baseline, "HEAD").strip() == "", "handoff baseline is not an ancestor")
    require(git("rev-parse", root_source).strip() == root_source, "root source unavailable")
    require(git("rev-parse", f"{root_source}^{{tree}}").strip() == root_tree, "root source tree drifted")

    decision = handoff.get("decision", {})
    require(decision.get("result") == "phase2-engineering-complete-promotion-blocked", "handoff decision drifted")
    require(decision.get("opensP3ReversibleDevelopmentOnly") is True, "P3 boundary drifted")
    for key in ("phase2ProductionAccepted", "phase2PromotionAllowed", "betaAccepted"):
        require(decision.get(key) is False, f"false acceptance recorded: {key}")
    require(decision.get("codecSelection") == "no-go", "codec no-go lost")
    require(decision.get("maximumRolloutStage") == 4, "rollout stage exceeded dark deployment")
    require(decision.get("openRepositoryCriticalOrHighFindings") == [], "repository critical/high finding remains")
    require(decision.get("productionBlockingHighFindings") == 13, "production High count drifted")

    artifacts = handoff.get("artifacts", {})
    require(artifacts.get("acceptedSourceCommit") == root_source, "accepted source commit drifted")
    require(artifacts.get("acceptedSourceTree") == root_tree, "accepted source tree drifted")
    for key in (
        "productionBuildSha256", "productionWindowsPackageSha256",
        "productionMacOSPackageSha256", "productionRuntimeConfigSha256",
        "productionDatabaseSnapshotSha256", "productionFixtureLockSha256",
    ):
        require(artifacts.get(key) is None, f"unaccepted production hash recorded: {key}")

    anchors = handoff.get("sourceAnchors", [])
    require(len(anchors) == 27, "source anchor inventory drifted")
    require(len({item.get("path") for item in anchors}) == len(anchors), "duplicate source anchor")
    for item in anchors:
        raw = snapshot_bytes(baseline, item.get("path", ""))
        require(hashlib.sha256(raw).hexdigest() == item.get("sha256"),
                f"snapshot digest mismatch: {item.get('path')}")

    root = snapshot_json(baseline, "acceptance/phase2/root-integration-review-v1.json")
    require(root.get("decision", {}).get("result") == "engineering-baseline-accepted-production-blocked",
            "root engineering decision drifted")
    require(root.get("reviewedSourceCommit") == root_source, "root reviewed commit mismatch")
    require(root.get("inventory", {}).get("changedPathsNoRenames") == 624, "root path inventory drifted")
    require(root.get("inventory", {}).get("unmappedPaths") == 0, "root has unmapped paths")
    require(root.get("manualAndExternalHolds", {}).get("openHighFindingCount") == 13,
            "root open High count drifted")
    require(root.get("manualAndExternalHolds", {}).get("betaSevenDayGateComplete") is False,
            "root falsely completed beta")

    review_paths = [item["path"] for item in root.get("independentReviews", [])]
    open_high = []
    for path in review_paths:
        review = snapshot_json(baseline, path)
        for finding in review.get("findings", []):
            if finding.get("severity") == "high" and finding.get("status", "").startswith("open-"):
                open_high.append(finding["id"])
    require(len(open_high) == 13 and len(set(open_high)) == 13, "independent open High inventory drifted")

    streamed = snapshot_json(baseline, "acceptance/streamed-track-rollout-handoff-v1.json")
    activation = streamed.get("productionActivation", {})
    stream_authority = handoff.get("featureAuthorities", {}).get("streamedTracks", {})
    require(activation.get("currentValue") is False, "upstream streamed tracks enabled")
    require(activation.get("activationAllowedNow") is False, "upstream activation allowed")
    require(activation.get("currentMaximumRolloutStage") == 4, "upstream rollout stage drifted")
    require(activation.get("productionDecoderRegistry") == [], "upstream decoder registry is not empty")
    require(stream_authority.get("currentState") == "disabled", "handoff streamed state drifted")
    require(stream_authority.get("runtimeFlagImplementation") == "absent", "invented streamed runtime flag")
    require(stream_authority.get("productionSelectionEnabled") is False, "production selection enabled")
    require(stream_authority.get("productionEncoderRegistry") == [] and
            stream_authority.get("productionDecoderRegistry") == [], "handoff registries are not empty")
    require(stream_authority.get("wireCapabilityAdvertised") is False, "wire capability falsely advertised")

    quota = handoff.get("quotaModel", {})
    require(quota.get("calibrationState") == "engineering-defaults-only", "quota defaults represented as calibrated")
    require(quota.get("actor") == streamed.get("quotaDefaults", {}).get("actor"), "actor quotas drifted")
    require(quota.get("orbit") == streamed.get("quotaDefaults", {}).get("orbit"), "orbit quotas drifted")
    require(quota.get("betaCalibrationTask") == "TASK-260712-2pnc5a", "quota beta owner drifted")

    feature_source = snapshot_bytes(baseline, "coordinator/internal/store/stream_track_schema.go").decode()
    air_source = snapshot_bytes(baseline, "coordinator/internal/store/air_schema.go").decode()
    require("CHECK(production_selection_enabled = 0 AND selected_profile = '')" in feature_source,
            "stream policy no-go guard missing")
    for state in ("links_authoritative", "airs_shadow", "airs_authoritative", "rollback_hold"):
        require(state in air_source, f"Air authority state missing: {state}")
    air = handoff.get("featureAuthorities", {}).get("airRooms", {})
    require(air.get("enabledOnlyWhen") == "airs_authoritative", "Air enable authority drifted")
    require(air.get("rollbackState") == "rollback_hold", "Air rollback state drifted")
    require(air.get("legacyAndAirConcurrentDeliveryAllowed") is False, "dual Air/link delivery allowed")
    targets = handoff.get("featureAuthorities", {}).get("targetsInboxRights", {})
    require(targets.get("runtimeFlag") is None, "invented target runtime flag")
    require(targets.get("broadcastFallbackAllowed") is False, "target broadcast fallback enabled")

    expected_gates = {"B1", "B2", "B3", "B4", "B5", "B6", "B7", "20.5", "18-rollout", "20.6-beta"}
    gates = handoff.get("gateIndex", {})
    require(set(gates) == expected_gates, "gate index incomplete")
    for gate, record in gates.items():
        require(record.get("repositoryPreflight"), f"repository evidence missing: {gate}")
        require("manual" in record.get("finalStatus", "") or gate == "20.6-beta",
                f"final gate not visibly deferred: {gate}")
        require(record.get("manualTasks"), f"manual owner missing: {gate}")

    rollout = handoff.get("rolloutOrder", [])
    require([item.get("stage") for item in rollout] == list(range(1, 9)), "rollout order drifted")
    require(all(item.get("allowedNow") is True and item.get("executionClaimed") is False for item in rollout[:4]),
            "dark rollout boundary drifted")
    require(all(item.get("allowedNow") is False and item.get("blockedBy") for item in rollout[4:]),
            "production rollout stage was allowed")

    manual = handoff.get("manualEpic", {})
    require(manual.get("id") == "EPIC-260714-th54l3", "manual epic drifted")
    require(manual.get("totalPendingTasksAtHandoff") == 19, "manual epic total drifted")
    require(set(manual.get("phase2PendingTasks", [])) == {
        "TASK-260712-1fpb9q", "TASK-260712-21kz3b", "TASK-260712-2bdi4a",
        "TASK-260712-2pnc5a", "TASK-260712-3qybi2", "TASK-260712-3u5cdn",
    }, "Phase 2 manual task inventory drifted")
    external = handoff.get("externalApprovals", {})
    require(external.get("epic") == "EPIC-260714-zmnd4n", "external epic drifted")
    require(external.get("owner") == "Ivan Oparin", "external owner drifted")
    require(set(external.get("phase2Tasks", [])) == {
        "TASK-260716-19g4gd", "TASK-260716-2l5j1a",
        "TASK-260716-3voo6j", "TASK-260716-tlxe3s",
    }, "Phase 2 external approval inventory drifted")

    evidence = handoff.get("engineeringEvidence", [])
    require({item.get("id") for item in evidence} == {
        "root-clean-acceptance", "air-synthetic-regression", "target-rights-contracts",
        "stream-rollout-contract", "phase2-observability-contract",
    }, "engineering evidence inventory drifted")
    require(all(item.get("manualClaim") is False for item in evidence), "repository evidence claimed manual execution")
    require(handoff.get("nextEngineeringTask") == "TASK-260712-lo7a68", "strict next engineering task drifted")
    require(handoff.get("deferredBeforeNextTask") == "TASK-260712-9wivva", "manual routing seam drifted")


def main() -> int:
    handoff = load()
    validate(handoff)
    print(json.dumps({
        "contract": handoff["contract"],
        "decision": handoff["decision"]["result"],
        "maximumRolloutStage": handoff["decision"]["maximumRolloutStage"],
        "productionArtifacts": None,
        "status": "pass",
    }, sort_keys=True))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
