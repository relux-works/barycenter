#!/usr/bin/env python3
"""Fail-closed validation for the Phase 3 engineering handoff."""

from __future__ import annotations

import hashlib
import json
import pathlib
import subprocess


ROOT = pathlib.Path(__file__).resolve().parents[2]
HANDOFF_PATH = ROOT / "acceptance/phase3/engineering-handoff-v1.json"
STORE_DRAFT_PATH = ROOT / "docs/store/phase3/disclosure-draft-v1.json"
DISCLOSURE_PATH = ROOT / "docs/compliance/phase3-disclosure-delta.md"


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
    require(handoff.get("contract") == "p3-engineering-handoff.v1", "wrong contract")
    require(handoff.get("task") == "TASK-260712-3b7bp4", "wrong task")
    require(handoff.get("publishedAt") == "2026-07-17", "publication date drifted")

    baseline = handoff.get("handoffBaselineCommit", "")
    root_source = handoff.get("rootReviewedSourceCommit", "")
    root_tree = handoff.get("rootReviewedSourceTree", "")
    require(git("rev-parse", baseline).strip() == baseline, "handoff baseline unavailable")
    require(git("merge-base", "--is-ancestor", baseline, "HEAD").strip() == "",
            "handoff baseline is not an ancestor")
    require(git("rev-parse", root_source).strip() == root_source, "root source unavailable")
    require(git("rev-parse", f"{root_source}^{{tree}}").strip() == root_tree,
            "root source tree drifted")

    decision = handoff.get("decision", {})
    require(decision.get("result") ==
            "phase3-handoff-ready-root-audit-and-external-manual-gates-required",
            "handoff decision drifted")
    require(decision.get("repositoryHandoffComplete") is True, "handoff not complete")
    require(decision.get("opensRootEngineeringCompletionAuditOnly") is True,
            "root-audit-only boundary drifted")
    for key in (
        "phase3EngineeringComplete", "phase3ProductionAccepted",
        "phase3PromotionAllowed", "betaAccepted", "manualEvidenceClaimed",
        "independentReviewClaimed",
    ):
        require(decision.get(key) is False, f"false acceptance recorded: {key}")
    require(decision.get("openRootReviewedNonE2EECriticalOrHighFindings") == [],
            "root-reviewed Critical/High finding remains")
    require(handoff.get("releaseAuthorityGranted") is False, "release authority invented")

    artifacts = handoff.get("artifacts", {})
    require(artifacts.get("acceptedSourceCommit") == root_source, "source commit drifted")
    require(artifacts.get("acceptedSourceTree") == root_tree, "source tree drifted")
    for key in (
        "productionBuildSha256", "productionWindowsPackageSha256",
        "productionMacOSPackageSha256", "productionRuntimeConfigSha256",
        "productionDatabaseSnapshotSha256", "productionFixtureLockSha256",
    ):
        require(artifacts.get(key) is None, f"unaccepted production hash recorded: {key}")

    anchors = handoff.get("sourceAnchors", [])
    require(len(anchors) == 35, "source anchor inventory drifted")
    require(len({item.get("path") for item in anchors}) == len(anchors),
            "duplicate source anchor")
    for item in anchors:
        raw = snapshot_bytes(baseline, item.get("path", ""))
        require(hashlib.sha256(raw).hexdigest() == item.get("sha256"),
                f"snapshot digest mismatch: {item.get('path')}")

    root_review = snapshot_json(baseline, "acceptance/phase3/root-line-review-v1.json")
    require(root_review.get("reviewedSourceCommit") == root_source,
            "root reviewed source mismatch")
    require(root_review.get("reviewedSourceTree") == root_tree,
            "root reviewed tree mismatch")
    require(root_review.get("decision", {}).get("result") ==
            "non-e2ee-engineering-baseline-accepted-production-blocked",
            "root review boundary drifted")
    require(root_review.get("inventory", {}).get("changedPathsNoRenames") == 420,
            "root path inventory drifted")
    require(root_review.get("inventory", {}).get("unmappedPaths") == 0,
            "root review has unmapped paths")
    unresolved = [
        item.get("id") for item in root_review.get("findings", [])
        if item.get("severity") in {"critical", "high"}
        and item.get("status") != "fixed-re-reviewed"
    ]
    require(unresolved == [], "unresolved root Critical/High finding")

    packets = handoff.get("technicalReviewPackets", [])
    require({item.get("domain") for item in packets} ==
            {"realtime", "automation", "privacy-store", "migration-recovery"},
            "technical review packet inventory drifted")
    for item in packets:
        packet = snapshot_json(baseline, item.get("contract", ""))
        packet_decision = packet.get("decision", {})
        require(packet.get("task") == item.get("task"),
                f"packet task mismatch: {item.get('domain')}")
        require(packet.get("rootReview", {}).get("sourceCommit") == root_source,
                f"packet root mismatch: {item.get('domain')}")
        require(packet_decision.get("repositoryPreflightPassed") is True,
                f"packet preflight missing: {item.get('domain')}")
        require(packet_decision.get("criticalOrHighTechnicalFindingsOpen") is False,
                f"packet Critical/High remains: {item.get('domain')}")
        require(packet_decision.get("phase3PromotionAllowed") is False,
                f"packet promoted Phase 3: {item.get('domain')}")
        require(packet_decision.get("manualEvidenceClaimed") is False,
                f"packet claimed manual evidence: {item.get('domain')}")
        require(item.get("technicalCriticalOrHighOpen") is False,
                f"handoff packet finding drift: {item.get('domain')}")
        require(item.get("independentApprovalTask"),
                f"independent approval owner missing: {item.get('domain')}")

    capabilities = handoff.get("capabilityRecommendations", {})
    require(set(capabilities) == {"live_ptt", "e2ee_media", "soundboard_cues", "automation"},
            "capability recommendation inventory drifted")
    for capability, record in capabilities.items():
        require(record.get("recommendation") in {
            "engineering-reviewed-hold", "deferred-unavailable-hold"
        }, f"capability not held: {capability}")
        require(record.get("productionCapabilityAdvertised") is False,
                f"capability advertised: {capability}")
        require(record.get("productionActivationAllowed") is False,
                f"capability activation allowed: {capability}")
        require(record.get("e2eeClaimAllowed") is False,
                f"E2EE claim allowed: {capability}")
        require(record.get("holds"), f"capability holds missing: {capability}")
    require("not end-to-end encrypted" in capabilities["live_ptt"].get("disclosure", ""),
            "Live PTT non-E2EE disclosure missing")
    require("absent and disabled" in capabilities["e2ee_media"].get("disclosure", ""),
            "E2EE unavailable disclosure missing")

    expected_gates = {
        "C1", "C2", "C3", "C4", "C5", "C6", "C7", "NF-jitter",
        "NF-reconnect", "NF-secure-storage", "NF-external-security-review",
        "NF-root-review", "NF-realtime-review", "NF-automation-review",
        "NF-privacy-store-review", "NF-migration-recovery-review",
        "NF-disclosures", "NF-rollout-recovery", "NF-beta",
    }
    gates = handoff.get("gateIndex", {})
    require(set(gates) == expected_gates, "gate index incomplete")
    for gate, record in gates.items():
        require(record.get("artifact"), f"artifact missing: {gate}")
        require(record.get("repositoryEvidence"), f"repository evidence missing: {gate}")
        require(record.get("closureTasks"), f"closure task missing: {gate}")
        status = record.get("finalStatus", "")
        require(any(fragment in status for fragment in
                    ("not-run", "blocked", "withheld", "production-withheld")),
                f"gate falsely final-passed: {gate}")

    rollback = handoff.get("rollbackIndex", {})
    require(rollback.get("executionClaimed") is False, "rollback execution fabricated")
    require(rollback.get("manualDrillTask") == "TASK-260712-30xwu2",
            "rollback manual task drifted")
    require("unset DUET_LIVE_PTT" in rollback.get("live_ptt", []),
            "Live PTT kill command missing")
    require("keep capability absent" in rollback.get("e2ee_media", []),
            "E2EE fail-closed rollback missing")

    manual_tasks = {
        value for values in handoff.get("manualEpic", {}).get("tasks", {}).values()
        for value in values
    }
    expected_manual = {
        "TASK-260712-1vtwkl", "TASK-260712-2hodti", "TASK-260712-e5mfqj",
        "TASK-260712-1fpb9q", "TASK-260712-21kz3b", "TASK-260712-2bdi4a",
        "TASK-260712-2pnc5a", "TASK-260712-3qybi2", "TASK-260712-3u5cdn",
        "TASK-260712-9wivva", "TASK-260712-1rzqh9", "TASK-260712-265o0f",
        "TASK-260712-2gaswa", "TASK-260712-2e80pr", "TASK-260712-flaiie",
        "TASK-260712-yj668d", "TASK-260712-1gyohk", "TASK-260712-30xwu2",
        "TASK-260712-1actom",
    }
    manual = handoff.get("manualEpic", {})
    require(manual.get("id") == "EPIC-260714-th54l3", "manual epic drifted")
    require(manual.get("totalPendingTasksAtHandoff") == 19, "manual count drifted")
    require(manual.get("allStatuses") == "backlog-not-run", "manual status drifted")
    require(manual_tasks == expected_manual, "manual task inventory drifted")

    expected_e2ee = {
        "TASK-260712-aniuyy", "TASK-260712-3w1cst", "TASK-260712-20j5tm",
        "TASK-260712-1yz5ca", "TASK-260712-1x9ruo", "TASK-260712-25dzp4",
        "TASK-260712-2i0w6x", "TASK-260712-1rziyo", "TASK-260712-2kcduo",
        "TASK-260712-tcwn44", "TASK-260712-3980vy", "TASK-260712-28zhpl",
        "TASK-260712-1u57qz", "TASK-260712-39vjzd", "TASK-260712-2nppt6",
        "TASK-260712-2q4jbu", "TASK-260712-1bcpda", "TASK-260712-1ulshp",
    }
    deferred = handoff.get("deferredE2EE", {})
    require(deferred.get("epic") == "EPIC-260716-3qsztl", "E2EE epic drifted")
    require(set(deferred.get("tasks", [])) == expected_e2ee, "deferred E2EE inventory drifted")

    external = handoff.get("externalApprovals", {})
    require(external.get("epic") == "EPIC-260714-zmnd4n", "external epic drifted")
    require(external.get("owner") == "Ivan Oparin", "external owner drifted")
    require(set(external.get("phase3IndependentTasks", [])) == {
        "TASK-260717-3dbi2v", "TASK-260717-1pyg62",
        "TASK-260717-35bll1", "TASK-260717-1sgb5r",
    }, "independent approval inventory drifted")
    require(external.get("allStatuses") == "backlog-not-approved",
            "external approval falsely recorded")

    disclosure = handoff.get("disclosureIndex", {})
    require(set(disclosure.get("surfaces", {})) == {"product", "privacy", "store", "iarc"},
            "disclosure surface inventory drifted")
    require(disclosure.get("publicationClaimed") is False, "publication fabricated")
    require(disclosure.get("submissionClaimed") is False, "submission fabricated")
    require(disclosure.get("owner") == "Ivan Oparin", "disclosure owner drifted")
    legal = snapshot_json(baseline, "docs/compliance/legal-ops-inputs.json")
    require(legal.get("publication_gate", {}).get("state") == "approved",
            "approved legal inputs lost")
    require(all(item.get("status") == "approved" for item in legal.get("inputs", [])),
            "legal input approval drifted")

    store = json.loads(STORE_DRAFT_PATH.read_text(encoding="utf-8"))
    require(store.get("contract") == "p3-store-disclosure-draft.v1", "wrong Store draft")
    require(store.get("eligibleForSubmission") is False, "Store draft became submittable")
    for key in (
        "partnerCenterMutationClaimed", "policyPublicationClaimed",
        "screenshotEvidenceClaimed", "wackEvidenceClaimed",
        "iarcPortalEvidenceClaimed", "buildCertificationEvidenceClaimed",
    ):
        require(store.get(key) is False, f"external Store evidence fabricated: {key}")
    en = store.get("conditionalListingDelta", {}).get("en-US", {})
    ru = store.get("conditionalListingDelta", {}).get("ru-RU", {})
    require("not end-to-end encrypted" in en.get("livePTT", ""),
            "EN Live PTT disclosure drifted")
    require("не защищено сквозным шифрованием" in ru.get("livePTT", ""),
            "RU Live PTT disclosure drifted")
    require("does not provide or claim" in en.get("e2ee", ""),
            "EN E2EE hold disclosure drifted")
    require(store.get("iarcDelta", {}).get("answerChangesAllowedNow") is False,
            "IARC reduction allowed")

    document = DISCLOSURE_PATH.read_text(encoding="utf-8")
    for fragment in (
        "publication and Store submission are on hold",
        "Live PTT is coordinator-routed readable audio and is **not end-to-end encrypted**",
        "No production crypto library, suite, protected-media implementation",
        "Missing evidence is `pending` or `not-run`, never `pass`",
    ):
        require(fragment in document, f"disclosure boundary missing: {fragment}")

    commands = handoff.get("reproductionCommands", [])
    require(len(commands) == 9, "reproduction command inventory drifted")
    require(all(command.startswith("python3 ") for command in commands),
            "non-reproducible command recorded")
    require(handoff.get("nextEngineeringTask") == "TASK-260712-2b5685",
            "strict next engineering task drifted")


def main() -> int:
    handoff = load()
    validate(handoff)
    print(json.dumps({
        "contract": handoff["contract"],
        "decision": handoff["decision"]["result"],
        "gates": len(handoff["gateIndex"]),
        "manualTasks": handoff["manualEpic"]["totalPendingTasksAtHandoff"],
        "productionArtifacts": None,
        "status": "pass",
    }, sort_keys=True))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
