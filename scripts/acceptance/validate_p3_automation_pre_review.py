#!/usr/bin/env python3
"""Fail-closed validation for the Phase 3 automation technical pre-review."""

from __future__ import annotations

import hashlib
import json
import pathlib


ROOT = pathlib.Path(__file__).resolve().parents[2]
REVIEW_PATH = ROOT / "acceptance/phase3/automation-technical-pre-review-v1.json"
ROOT_REVIEW_PATH = ROOT / "acceptance/phase3/root-line-review-v1.json"
HANDOFF_PATH = ROOT / "acceptance/phase3/automation-safety-evidence-v1.json"
GATE_MATRIX_PATH = ROOT / "acceptance/phase3/gate-matrix-v1.json"


class ReviewError(ValueError):
    pass


def require(condition: bool, message: str) -> None:
    if not condition:
        raise ReviewError(message)


def load(path: pathlib.Path = REVIEW_PATH) -> dict:
    return json.loads(path.read_text(encoding="utf-8"))


def digest(path: pathlib.Path) -> str:
    return hashlib.sha256(path.read_bytes()).hexdigest()


def validate(review: dict) -> None:
    require(review.get("schemaVersion") == 1, "unsupported schema")
    require(review.get("contract") == "p3-automation-technical-pre-review.v1", "wrong contract")
    require(review.get("task") == "TASK-260712-1x5jfo", "wrong task")
    require(review.get("reviewedAt") == "2026-07-17", "review date drifted")

    root = review.get("rootReview", {})
    require(root.get("sourceCommit") == "d94f51644a3acf37601b4a869b4247380372f9ec",
            "root-reviewed source drifted")
    require(root.get("sourceTree") == "4e4cca878db806650eda6f1e1642051b87a18b93",
            "root-reviewed tree drifted")
    root_packet = json.loads(ROOT_REVIEW_PATH.read_text(encoding="utf-8"))
    require(root_packet.get("reviewedSourceCommit") == root["sourceCommit"], "root packet source mismatch")
    require(root_packet.get("reviewedSourceTree") == root["sourceTree"], "root packet tree mismatch")

    reviewer = review.get("reviewer", {})
    require(reviewer.get("independenceRequired") is True, "independence requirement removed")
    require(reviewer.get("independenceSatisfied") is False, "inline review falsely claimed independence")
    require(reviewer.get("independentApprover") == "Ivan Oparin", "approval owner drifted")
    require(reviewer.get("independentApprovalTask") == "TASK-260717-1pyg62",
            "external approval task drifted")
    require(reviewer.get("approvalStatus") == "required", "external approval falsely completed")

    decision = review.get("decision", {})
    require(decision.get("result") == "engineering-pre-review-complete-external-and-manual-blocked",
            "decision drifted")
    require(decision.get("repositoryPreflightPassed") is True, "repository preflight lost")
    require(decision.get("criticalOrHighTechnicalFindingsOpen") is False,
            "critical/high technical finding remains")
    for key in ("automationActivationAllowed", "c7Accepted", "phase3PromotionAllowed", "manualEvidenceClaimed"):
        require(decision.get(key) is False, f"fail-closed decision violated: {key}")
    require(decision.get("nextEngineeringTaskMayStart") is True,
            "owner-authorized reversible continuation removed")

    anchors = review.get("sourceAnchors", [])
    require(len(anchors) == 23, "source inventory incomplete")
    require(len({item.get("path") for item in anchors}) == len(anchors), "duplicate source anchor")
    for item in anchors:
        path = ROOT / item.get("path", "")
        require(path.is_file(), f"source anchor missing: {item.get('path')}")
        require(digest(path) == item.get("sha256"), f"source digest mismatch: {item.get('path')}")

    require(set(review.get("reviewedInvariants", [])) == {
        "actor-and-orbit-authority-rechecked-on-every-mutation",
        "principal-secrets-are-one-time-hashed-and-revocable",
        "idempotency-conflicts-never-create-replacement-work",
        "schedule-dst-fold-gap-and-clock-jump-are-canonical-and-no-catch-up",
        "dnd-block-air-membership-and-target-capability-are-last-mile-rechecked",
        "principal-and-orbit-rates-concurrency-and-retention-are-bounded",
        "disable-revoke-delete-and-emergency-stop-use-canonical-cancellation",
        "telegram-callbacks-are-opaque-bound-one-shot-and-live-state-rechecked",
        "recipient-local-ceiling-remains-after-automation-mix",
        "soundboard-and-automation-never-enter-microphone-capture",
    }, "review invariant inventory drifted")

    reruns = review.get("reruns", [])
    require({item.get("id") for item in reruns} == {
        "coordinator-automation-race-stress", "windows-automation-race-stress",
        "macos-automation-suite", "automation-handoff-contracts",
    }, "rerun inventory drifted")
    require(all(item.get("status") == "pass" and item.get("command") for item in reruns),
            "representative rerun did not pass")
    attempts = review.get("nonAcceptedAttempts", [])
    require(len(attempts) == 1 and
            attempts[0].get("id") == "coordinator-broad-automation-store-race-times-ten" and
            attempts[0].get("status") == "timeout-not-counted" and
            attempts[0].get("selectedTests") == 54 and
            "unrelated transmission scheduler" in attempts[0].get("reason", ""),
            "non-accepted broad timeout was hidden or misrepresented")
    require(review.get("findings") == [], "unresolved review finding present")

    boundary = review.get("evidenceBoundary", {})
    require(len(boundary.get("automated", [])) == 6, "automated evidence boundary incomplete")
    require(len(boundary.get("notProved", [])) == 5, "manual evidence boundary incomplete")
    gates = review.get("openGates", [])
    require([gate.get("id") for gate in gates] == ["P3-AUTO-GATE-001", "P3-AUTO-GATE-002"],
            "open gate inventory drifted")
    require(gates[0].get("status") == "manual-blocking" and
            gates[0].get("ownerEpic") == "EPIC-260714-th54l3" and
            gates[0].get("task") == "TASK-260712-1gyohk" and
            gates[0].get("manualEvidence") == "not-run" and
            gates[0].get("environmentIdentity") is None, "manual evidence was falsely claimed")
    require(gates[1].get("status") == "external-review-blocking" and
            gates[1].get("ownerEpic") == "EPIC-260714-zmnd4n" and
            gates[1].get("task") == "TASK-260717-1pyg62" and
            gates[1].get("reviewerIdentity") is None and
            gates[1].get("approval") == "required", "independent approval was falsely claimed")

    handoff = json.loads(HANDOFF_PATH.read_text(encoding="utf-8"))
    require(handoff.get("decision", {}).get("c7Accepted") is False, "engineering handoff claims C7")
    require(handoff.get("frozenContract", {}).get("automationMayEnterCapture") is False,
            "automation entered capture")
    matrix = json.loads(GATE_MATRIX_PATH.read_text(encoding="utf-8"))
    require(matrix.get("gates", {}).get("C7", {}).get("status") == "manual-required",
            "C7 no longer manual-required")
    require(matrix.get("gates", {}).get("NF-automation-review", {}).get("status") ==
            "missing-independent-reviewer", "automation independent gate drifted")
    require(review.get("nextEngineeringTask") == "TASK-260712-7ng1vs", "strict next task drifted")


def main() -> int:
    review = load()
    validate(review)
    print(json.dumps({
        "contract": review["contract"],
        "decision": review["decision"]["result"],
        "sourceCommit": review["rootReview"]["sourceCommit"],
        "independenceSatisfied": False,
        "manualEvidence": "not-run",
        "status": "pass",
    }, sort_keys=True))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
