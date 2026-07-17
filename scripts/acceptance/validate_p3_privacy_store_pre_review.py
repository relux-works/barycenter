#!/usr/bin/env python3
"""Validate the fail-closed Phase 3 privacy and Store technical pre-review."""

from __future__ import annotations

import hashlib
import json
import pathlib


ROOT = pathlib.Path(__file__).resolve().parents[2]
REVIEW_PATH = ROOT / "acceptance/phase3/privacy-store-technical-pre-review-v1.json"
ROOT_REVIEW_PATH = ROOT / "acceptance/phase3/root-line-review-v1.json"
LEGAL_INPUTS_PATH = ROOT / "docs/compliance/legal-ops-inputs.json"
POLICY_PACK_PATH = ROOT / "docs/compliance/policy-pack-2026-07-14.json"
STORE_POLICY_PATH = ROOT / "docs/compliance/store-policy-pre-submit.json"
SCREENSHOTS_PATH = ROOT / "docs/store/phase1/screenshots.json"
PARTNER_PACKAGE_PATH = ROOT / "docs/store/phase1/partner-center-package.json"
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
    require(review.get("contract") == "p3-privacy-store-technical-pre-review.v1", "wrong contract")
    require(review.get("task") == "TASK-260712-7ng1vs", "wrong task")
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
    require(reviewer.get("accountableOwner") == "Ivan Oparin", "accountable owner drifted")
    require(reviewer.get("independentApprovalTask") == "TASK-260717-35bll1",
            "external approval task drifted")
    require(reviewer.get("approvalStatus") == "required", "external approval falsely completed")

    decision = review.get("decision", {})
    require(decision.get("result") == "engineering-pre-review-complete-external-blocked",
            "decision drifted")
    require(decision.get("repositoryPreflightPassed") is True, "repository preflight lost")
    require(decision.get("criticalOrHighTechnicalFindingsOpen") is False,
            "critical/high technical finding remains")
    for key in (
        "storeSubmissionAllowed", "policyPublicationClaimed", "mailboxDeliveryClaimed",
        "partnerCenterEvidenceClaimed", "affectedPhase3FlagsAllowed",
        "phase3PromotionAllowed", "manualEvidenceClaimed",
    ):
        require(decision.get(key) is False, f"fail-closed decision violated: {key}")
    require(decision.get("nextEngineeringTaskMayStart") is True,
            "owner-authorized reversible continuation removed")

    defaults = review.get("approvedDefaults", {})
    require(defaults.get("source") == "docs/compliance/legal-ops-inputs.json",
            "approved default source drifted")
    require(defaults.get("accountableOwner") == "Ivan Oparin", "approved owner drifted")
    require(defaults.get("counselReviewRequired") is False, "counsel default drifted")
    legal = json.loads(LEGAL_INPUTS_PATH.read_text(encoding="utf-8"))
    require(legal.get("publication_gate", {}).get("state") == "approved",
            "legal input publication gate is not approved")
    require(all(item.get("status") == "approved" for item in legal.get("inputs", [])),
            "legal input is unresolved")

    anchors = review.get("sourceAnchors", [])
    require(len(anchors) == 32, "source inventory incomplete")
    require(len({item.get("path") for item in anchors}) == len(anchors), "duplicate source anchor")
    for item in anchors:
        path = ROOT / item.get("path", "")
        require(path.is_file(), f"source anchor missing: {item.get('path')}")
        require(digest(path) == item.get("sha256"), f"source digest mismatch: {item.get('path')}")

    require(set(review.get("reviewedInvariants", [])) == {
        "reporter-protection-never-censors-the-foreign-evidence-target",
        "report-evidence-access-requires-an-unrevoked-matching-evidence-capability",
        "evidence-read-and-decision-actions-append-content-free-audit-records",
        "free-form-report-text-and-evidence-access-expire-after-thirty-days",
        "content-policy-consent-is-versioned-actor-bound-and-rechecked-before-upload",
        "phase1-copy-discloses-coordinator-readable-non-e2ee-audio",
        "deletion-copy-preserves-recipient-integration-hold-and-backup-limits",
        "recovery-copy-does-not-promise-recovery-after-total-credential-loss",
        "telegram-is-optional-and-third-party-copies-remain-third-party-controlled",
        "partner-center-readiness-cannot-pass-with-placeholder-build-or-manual-evidence",
    }, "review invariant inventory drifted")

    claims = review.get("reviewedClaims", [])
    require([claim.get("id") for claim in claims] == [
        "encryption", "metadata", "deletion", "recovery", "telegram",
        "report-evidence", "ugc-and-store",
    ], "claim audit inventory drifted")
    require(all(claim.get("result") == "match" for claim in claims[:-1]),
            "proved privacy claim no longer matches")
    require(claims[-1].get("result") == "hold", "external Store claim was not held")

    reruns = review.get("reruns", [])
    require({item.get("id") for item in reruns} == {
        "coordinator-moderation-store-race-stress",
        "coordinator-moderation-service-http-race-stress",
        "coordinator-content-policy-race-stress",
        "moderation-exact-previous-head-rollback",
        "windows-privacy-policy-race-stress",
        "macos-privacy-policy-suite",
        "legal-policy-store-static-gates",
    }, "rerun inventory drifted")
    require(all(item.get("status") == "pass" and item.get("command") for item in reruns),
            "representative rerun did not pass")
    require(review.get("findings") == [], "unresolved review finding present")

    policy_pack = json.loads(POLICY_PACK_PATH.read_text(encoding="utf-8"))
    require(policy_pack.get("review", {}).get("publication_decision") == "proceed",
            "approved policy source no longer has proceed decision")
    store_policy = json.loads(STORE_POLICY_PATH.read_text(encoding="utf-8"))
    require(store_policy.get("decision") == "hold" and not store_policy.get("submission_tag"),
            "Store submission gate was falsely opened")
    screenshots = json.loads(SCREENSHOTS_PATH.read_text(encoding="utf-8")).get("screenshots", [])
    require(len(screenshots) == 12, "screenshot inventory drifted")
    require(all(item.get("status") == "manual-required" and item.get("sha256") == ""
                for item in screenshots), "manual screenshots were falsely claimed")
    partner = json.loads(PARTNER_PACKAGE_PATH.read_text(encoding="utf-8"))
    require(partner.get("state") == "engineering-ready-manual-hold",
            "Partner Center package hold drifted")

    boundary = review.get("evidenceBoundary", {})
    require(len(boundary.get("automated", [])) == 6, "automated evidence boundary incomplete")
    require(len(boundary.get("notProved", [])) == 5, "external evidence boundary incomplete")
    gates = review.get("openGates", [])
    require([gate.get("id") for gate in gates] == [
        "P3-PRIV-GATE-001", "P3-PRIV-GATE-002", "P3-PRIV-GATE-003", "P3-PRIV-GATE-004",
    ], "open gate inventory drifted")
    require(gates[0].get("task") == "TASK-260717-35bll1" and
            gates[0].get("reviewerIdentity") is None and gates[0].get("approval") == "required",
            "independent approval was falsely claimed")
    require(gates[1].get("task") == "TASK-260714-200ib8" and
            gates[1].get("deliveryEvidence") == "not-provided", "mail delivery was falsely claimed")
    require(gates[2].get("task") == "TASK-260715-24ube9" and
            gates[2].get("partnerCenterEvidence") == "not-provided",
            "Partner Center evidence was falsely claimed")
    require(gates[3].get("task") == "TASK-260712-3b7bp4" and
            gates[3].get("publicationEvidence") == "not-provided",
            "Phase 3 disclosure publication was falsely claimed")

    matrix = json.loads(GATE_MATRIX_PATH.read_text(encoding="utf-8"))
    require(matrix.get("gates", {}).get("NF-privacy-store-review", {}).get("status") ==
            "missing-independent-reviewer-and-publication-evidence", "privacy/Store gate drifted")
    require(matrix.get("gates", {}).get("NF-disclosures", {}).get("status") ==
            "external-publication-and-store-action-required", "disclosure gate drifted")
    capabilities = matrix.get("promotionCapabilities", {})
    require(capabilities.get("e2ee_media", {}).get("status") == "blocked-by-deferred-e2ee-and-review",
            "deferred E2EE was falsely enabled")
    for capability in ("soundboard_cues", "automation"):
        require(capabilities.get(capability, {}).get("status") == "not-ready",
                f"held capability was falsely enabled: {capability}")
    require(review.get("heldCapabilities") == ["e2ee_media", "soundboard_cues", "automation"],
            "held capability inventory drifted")
    require(review.get("nextEngineeringTask") == "TASK-260712-6mz9xg", "strict next task drifted")


def main() -> int:
    review = load()
    validate(review)
    print(json.dumps({
        "contract": review["contract"],
        "decision": review["decision"]["result"],
        "sourceCommit": review["rootReview"]["sourceCommit"],
        "independenceSatisfied": False,
        "storeSubmission": "hold",
        "status": "pass",
    }, sort_keys=True))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
