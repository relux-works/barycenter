#!/usr/bin/env python3
"""Validate the fail-closed Phase 3 migration/recovery technical pre-review."""

from __future__ import annotations

import hashlib
import json
import pathlib


ROOT = pathlib.Path(__file__).resolve().parents[2]
REVIEW_PATH = ROOT / "acceptance/phase3/migration-recovery-technical-pre-review-v1.json"
ROOT_REVIEW_PATH = ROOT / "acceptance/phase3/root-line-review-v1.json"
GATE_MATRIX_PATH = ROOT / "acceptance/phase3/gate-matrix-v1.json"
E2EE_LIFECYCLE_PATH = ROOT / "acceptance/phase3/e2ee-protocol-key-lifecycle-v1.json"


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
    require(review.get("contract") == "p3-migration-recovery-technical-pre-review.v1", "wrong contract")
    require(review.get("task") == "TASK-260712-6mz9xg", "wrong task")
    require(review.get("reviewedAt") == "2026-07-17", "review date drifted")

    root = review.get("rootReview", {})
    require(root.get("sourceCommit") == "d94f51644a3acf37601b4a869b4247380372f9ec",
            "root-reviewed source drifted")
    require(root.get("sourceTree") == "4e4cca878db806650eda6f1e1642051b87a18b93",
            "root-reviewed tree drifted")
    root_packet = json.loads(ROOT_REVIEW_PATH.read_text(encoding="utf-8"))
    require(root_packet.get("reviewedSourceCommit") == root["sourceCommit"], "root packet source mismatch")
    require(root_packet.get("reviewedSourceTree") == root["sourceTree"], "root packet tree mismatch")

    deltas = review.get("deltaReviews", [])
    require(len(deltas) == 1, "migration delta review inventory drifted")
    delta = deltas[0]
    require(delta.get("reviewedAt") == "2026-07-19", "migration delta review date drifted")
    require(delta.get("triggerTask") == "TASK-260712-1xkn75", "migration delta task drifted")
    require(delta.get("producerCommit") == "831d6d7671f9e8964cf70d1856cbd501dd3e5e0e",
            "migration delta producer drifted")
    require(delta.get("finding") == "P1-MIG-003", "migration delta finding drifted")
    require(delta.get("result") == "producer-fix-validated-independent-re-review-approved",
            "migration delta result drifted")
    require(delta.get("sourcePath") == "coordinator/internal/store/store.go",
            "migration delta source drifted")
    require(delta.get("previousSha256") ==
            "fb26e0809acb8ce7cfa336cfb9c8d887a88120fef9f6ef54c969181f12edd9e4",
            "migration delta predecessor digest drifted")
    require(delta.get("currentSha256") == digest(ROOT / delta["sourcePath"]),
            "migration delta current digest drifted")
    require(delta.get("evidence") == [
        "focused-migration-race-pass",
        "full-coordinator-pass",
        "full-coordinator-race-pass",
        "previoushead-tagged-store-race-pass",
    ], "migration delta evidence drifted")
    require(delta.get("independentReviewTask") == "TASK-260715-unbb7c" and
            delta.get("independentApproval") == "approved" and
            delta.get("approvalRevision") ==
            "aafcfc222518e7a32e2acaf365a1af4d247cc03c" and
            delta.get("approvalRun") == "RUN-260719-c83d59",
            "migration delta independent-review boundary drifted")

    reviewer = review.get("reviewer", {})
    require(reviewer.get("independenceRequired") is True, "independence requirement removed")
    require(reviewer.get("independenceSatisfied") is False, "inline review falsely claimed independence")
    require(reviewer.get("accountableOwner") == "Ivan Oparin", "accountable owner drifted")
    require(reviewer.get("independentApprovalTask") == "TASK-260717-1sgb5r",
            "external approval task drifted")
    require(reviewer.get("approvalStatus") == "required", "external approval falsely completed")

    decision = review.get("decision", {})
    require(decision.get("result") ==
            "engineering-pre-review-complete-external-manual-and-e2ee-blocked", "decision drifted")
    require(decision.get("repositoryPreflightPassed") is True, "repository preflight lost")
    require(decision.get("criticalOrHighTechnicalFindingsOpen") is False,
            "critical/high technical finding remains")
    for key in (
        "productionRestoreClaimed", "destructiveDrillClaimed", "e2eeRecoveryClaimed",
        "affectedPhase3FlagsAllowed", "rolloutRecoveryAccepted", "betaAllowed",
        "phase3PromotionAllowed", "manualEvidenceClaimed",
    ):
        require(decision.get(key) is False, f"fail-closed decision violated: {key}")
    require(decision.get("nextEngineeringTaskMayStart") is True,
            "owner-authorized reversible continuation removed")

    anchors = review.get("sourceAnchors", [])
    require(len(anchors) == 29, "source inventory incomplete")
    require(len({item.get("path") for item in anchors}) == len(anchors), "duplicate source anchor")
    for item in anchors:
        path = ROOT / item.get("path", "")
        require(path.is_file(), f"source anchor missing: {item.get('path')}")
        require(digest(path) == item.get("sha256"), f"source digest mismatch: {item.get('path')}")

    require(set(review.get("reviewedInvariants", [])) == {
        "schema-changes-are-additive-transactional-and-retryable",
        "exact-predecessor-rollback-preserves-known-and-unknown-current-rows",
        "unsafe-air-rollback-enters-hold-instead-of-resurrecting-links",
        "identity-recovery-is-generation-bound-idempotent-and-audited",
        "recovery-promotion-preserves-node-authority-and-never-restores-secrets",
        "live-ptt-disable-rejects-new-capture-and-releases-active-routing",
        "automation-disable-emergency-stop-and-revoke-cancel-canonical-work",
        "e2ee-fork-transfer-and-key-loss-remain-contract-only-and-production-disabled",
        "backup-restore-never-overwrites-an-existing-database-automatically",
        "restored-state-requires-current-migrations-and-retention-reconciliation-before-service",
        "beta-restarts-after-any-reviewed-path-schema-config-or-fixture-delta",
    }, "review invariant inventory drifted")

    reruns = review.get("reruns", [])
    require({item.get("id") for item in reruns} == {
        "coordinator-migration-recovery-race-stress",
        "coordinator-command-kill-race-stress",
        "coordinator-exact-predecessor-matrix",
        "windows-migration-recovery-race-stress",
        "macos-migration-recovery-suite",
        "phase3-e2ee-fail-closed-contracts",
    }, "rerun inventory drifted")
    require(all(item.get("status") == "pass" and item.get("command") for item in reruns),
            "representative rerun did not pass")
    require(review.get("findings") == [], "unresolved review finding present")

    boundary = review.get("evidenceBoundary", {})
    require(len(boundary.get("automated", [])) == 6, "automated evidence boundary incomplete")
    require(len(boundary.get("notProved", [])) == 5, "manual evidence boundary incomplete")
    gates = review.get("openGates", [])
    require([gate.get("id") for gate in gates] == [
        "P3-MIG-GATE-001", "P3-MIG-GATE-002", "P3-MIG-GATE-003", "P3-MIG-GATE-004",
    ], "open gate inventory drifted")
    require(gates[0].get("task") == "TASK-260712-30xwu2" and
            gates[0].get("manualEvidence") == "not-run" and
            gates[0].get("environmentIdentity") is None, "manual drill evidence was falsely claimed")
    require(gates[1].get("task") == "TASK-260717-1sgb5r" and
            gates[1].get("reviewerIdentity") is None and gates[1].get("approval") == "required",
            "independent approval was falsely claimed")
    require(gates[2].get("task") == "TASK-260712-aniuyy" and
            gates[2].get("e2eeEvidence") == "deferred-unavailable",
            "deferred E2EE recovery was falsely claimed")
    require(gates[3].get("task") == "TASK-260712-1actom" and
            gates[3].get("betaEvidence") == "not-run", "beta evidence was falsely claimed")

    matrix = json.loads(GATE_MATRIX_PATH.read_text(encoding="utf-8"))
    require(matrix.get("gates", {}).get("NF-migration-recovery-review", {}).get("status") ==
            "missing-independent-reviewer", "migration/recovery gate drifted")
    require(matrix.get("gates", {}).get("NF-rollout-recovery", {}).get("status") ==
            "manual-required", "rollout/recovery gate drifted")
    require(matrix.get("gates", {}).get("NF-beta", {}).get("status") ==
            "manual-required-after-all-reviews-and-drills", "beta gate drifted")
    capabilities = matrix.get("promotionCapabilities", {})
    require(capabilities.get("e2ee_media", {}).get("status") == "blocked-by-deferred-e2ee-and-review",
            "deferred E2EE was falsely enabled")
    for capability in ("live_ptt", "soundboard_cues", "automation"):
        require(capabilities.get(capability, {}).get("status") == "not-ready",
                f"held capability was falsely enabled: {capability}")
    lifecycle = json.loads(E2EE_LIFECYCLE_PATH.read_text(encoding="utf-8"))
    lifecycle_decision = lifecycle.get("decision", {})
    require(lifecycle_decision.get("implementationAuthorized") is False and
            lifecycle_decision.get("capabilityAdvertised") is False,
            "E2EE lifecycle falsely permits production")
    require(review.get("heldCapabilities") ==
            ["live_ptt", "e2ee_media", "soundboard_cues", "automation"],
            "held capability inventory drifted")
    require(review.get("nextEngineeringTask") == "TASK-260712-3b7bp4", "strict next task drifted")


def main() -> int:
    review = load()
    validate(review)
    print(json.dumps({
        "contract": review["contract"],
        "decision": review["decision"]["result"],
        "sourceCommit": review["rootReview"]["sourceCommit"],
        "independenceSatisfied": False,
        "manualDrills": "not-run",
        "productionE2EE": "disabled",
        "status": "pass",
    }, sort_keys=True))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
