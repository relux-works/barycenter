#!/usr/bin/env python3
"""Fail-closed validation for the Phase 2 Air migration technical review."""

from __future__ import annotations

import hashlib
import json
import pathlib
import re


ROOT = pathlib.Path(__file__).resolve().parents[2]
REVIEW_PATH = ROOT / "acceptance/phase2/air-migration-review-v1.json"
CONTRACT_PATH = ROOT / "protocol/air-lifecycle-policy-v1.json"
GATE_MATRIX_PATH = ROOT / "acceptance/phase2/gate-matrix-v1.json"
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


def validate(review: dict) -> None:
    require(review.get("schemaVersion") == 1, "unsupported review schema")
    require(review.get("contract") == "p2-air-migration-concurrency-technical-review.v1",
            "wrong review contract")
    require(review.get("task") == "TASK-260712-2sicfs", "wrong review task")
    require(review.get("reviewedAt") == "2026-07-16", "review date drifted")
    require(re.fullmatch(r"[0-9a-f]{40}", review.get("reviewedBaseCommit", "")) is not None,
            "reviewed base commit missing")

    reviewer = review.get("reviewer", {})
    require(reviewer.get("independenceRequired") is True, "independent review requirement removed")
    require(reviewer.get("independenceSatisfied") is False, "current root session cannot claim independence")
    require(reviewer.get("independentApprovalTask") == "TASK-260716-19g4gd", "approval task drifted")
    require(reviewer.get("independentApprover") == "Ivan Oparin", "independent approver drifted")
    require(reviewer.get("approvalStatus") == "required", "independent approval falsely completed")

    decision = review.get("decision", {})
    require(decision.get("result") == "engineering-review-complete-production-blocked",
            "technical review result drifted")
    require(decision.get("repositoryPreflightPassed") is True, "repository preflight was lost")
    require(decision.get("productionAirAllowed") is False, "production Air was allowed")
    require(decision.get("phase2PromotionAllowed") is False, "Phase 2 promotion was allowed")
    require(decision.get("manualMigrationClaim") is False, "repository review claimed manual migration")
    require(decision.get("nextEngineeringTaskMayStart") is True,
            "owner-authorized strict engineering continuation was removed")

    contract = json.loads(CONTRACT_PATH.read_text(encoding="utf-8"))
    limits = contract["limits"]
    frozen = review.get("frozenContract", {})
    require(frozen.get("barycentersPerAir") == limits["barycenters_per_air"], "Air capacity drifted")
    require(frozen.get("onlinePulsarsPerActiveAir") == limits["online_pulsars_per_active_air"],
            "Pulsar capacity drifted")
    require(frozen.get("inviteEntropyBits") == limits["invite_entropy_bits"], "invite entropy drifted")
    require(frozen.get("inviteTTLSeconds") == limits["invite_ttl_seconds"], "invite TTL drifted")
    require(frozen.get("inviteConsumeFailuresPerActorOrIPPerMinute") ==
            limits["invite_consume_failures_per_actor_or_ip_per_minute"], "invite limit drifted")
    require(frozen.get("authorityModes") == contract["statuses"]["authority"], "authority modes drifted")
    require(frozen.get("savedMembershipSeparateFromActivePointer") is True,
            "saved membership and active pointer were conflated")
    require(frozen.get("parkedAirRetainedAndLazy") is True, "parked Air retention was removed")
    require(frozen.get("linksAndAirsSimultaneouslyAuthoritative") is False,
            "dual link and Air authority was allowed")

    anchors = review.get("sourceAnchors", [])
    required_paths = {
        "protocol/air-lifecycle-policy-v1.json",
        "coordinator/internal/store/air_schema.go",
        "coordinator/internal/store/air.go",
        "coordinator/internal/store/air_control.go",
        "coordinator/internal/store/air_approach_alias.go",
        "coordinator/cmd/duet-coordinator/loop.go",
        "coordinator/cmd/duet-coordinator/air_runtime_control.go",
        "coordinator/cmd/duet-coordinator/air_http.go",
        "coordinator/cmd/duet-coordinator/onboarding.go",
        "coordinator/cmd/duet-coordinator/telegram_air.go",
        "scripts/acceptance/run_air_regression.py",
        "acceptance/phase2/gate-matrix-v1.json",
    }
    require({item.get("path") for item in anchors} == required_paths, "review source inventory drifted")
    for item in anchors:
        path = ROOT / item["path"]
        require(path.is_file(), f"review source missing: {item['path']}")
        require(SHA256.fullmatch(item.get("sha256", "")) is not None, "review source digest malformed")
        require(digest(path) == item["sha256"], f"review source digest mismatch: {item['path']}")

    http_source = (ROOT / "coordinator/cmd/duet-coordinator/air_http.go").read_text(encoding="utf-8")
    limiter_source = (ROOT / "coordinator/cmd/duet-coordinator/onboarding.go").read_text(encoding="utf-8")
    http_test = (ROOT / "coordinator/cmd/duet-coordinator/air_http_test.go").read_text(encoding="utf-8")
    reserve_at = http_source.index("reserveReleasable(", http_source.index("func (api *onboardingAPI) consumeAirInvite"))
    consume_at = http_source.index("ConsumeAuthorizedAirInvite", reserve_at)
    require(reserve_at < consume_at, "invite limiter no longer admits before store mutation")
    require("if !errors.Is(err, store.ErrAirInviteUnavailable)" in http_source,
            "successful and non-guess reservations are not released")
    require("func (l *attemptLimiter) release(reservation attemptReservation)" in limiter_source,
            "exact limiter reservation release missing")
    require("TestAirHTTPRejectsLooseShapesAndRateLimitsUnavailableInvites" in http_test and
            "valid invite was consumed before limiter admission" in http_test,
            "valid-after-limit regression coverage missing")

    gate_matrix = json.loads(GATE_MATRIX_PATH.read_text(encoding="utf-8"))
    require(gate_matrix.get("productionGate", {}).get("status") == "blocked", "Phase 2 gate no longer blocked")

    reruns = review.get("reruns", [])
    require({item.get("id") for item in reruns} == {
        "air-regression-rehearsal", "air-store-runtime-alias-telegram-race",
        "exact-previous-coordinator", "transactional-winner-stress",
        "invite-pre-admission-race-and-stress",
    }, "representative rerun inventory drifted")
    require(all(item.get("command") and item.get("claim") for item in reruns), "rerun evidence incomplete")
    require({item.get("status") for item in reruns} <= {"pass", "pass-after-fix"},
            "rerun status is not accepted")

    findings = review.get("findings", [])
    require([item.get("id") for item in findings] == [f"P2-AIR-{number:03d}" for number in range(1, 5)],
            "finding inventory drifted")
    require(findings[0].get("severity") == "high" and findings[0].get("status") == "fixed-re-reviewed",
            "invite admission High was reopened or lowered")
    require(findings[1].get("severity") == "medium" and findings[1].get("status") == "fixed-re-reviewed",
            "store error-classification finding drifted")
    require([item.get("status") for item in findings[2:]] == [
        "open-manual-blocking", "open-external-review",
    ], "open High finding was silently closed")
    require(review.get("productionBlockers") == ["P2-AIR-003", "P2-AIR-004"],
            "production blocker inventory drifted")

    boundary = review.get("automatedEvidenceBoundary", {})
    require(len(boundary.get("proved", [])) == 8, "automated proof inventory incomplete")
    require(len(boundary.get("notProved", [])) == 5, "manual proof boundary incomplete")
    require(review.get("manualEpic") == "EPIC-260714-th54l3", "manual evidence seam drifted")
    require(review.get("nextEngineeringTask") == "TASK-260712-n11rg6", "strict next task drifted")


def main() -> int:
    review = load()
    validate(review)
    print(json.dumps({
        "contract": review["contract"],
        "decision": review["decision"]["result"],
        "fixedHigh": 1,
        "fixedMedium": 1,
        "openHigh": 2,
        "independenceSatisfied": False,
        "productionAirAllowed": False,
        "status": "pass",
    }, sort_keys=True))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
