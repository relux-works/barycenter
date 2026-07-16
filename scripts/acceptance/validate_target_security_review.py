#!/usr/bin/env python3
"""Fail-closed validation for the Phase 2 target/range/rights review."""

import hashlib
import json
import pathlib
import re

ROOT = pathlib.Path(__file__).resolve().parents[2]
PATH = ROOT / "acceptance/phase2/target-security-review-v1.json"
SHA256 = re.compile(r"[0-9a-f]{64}")


class ReviewError(ValueError):
    pass


def require(value, message):
    if not value:
        raise ReviewError(message)


def load(path=PATH):
    return json.loads(path.read_text(encoding="utf-8"))


def validate(review):
    require(review.get("schemaVersion") == 1, "unsupported schema")
    require(review.get("contract") == "p2-target-range-rights-security-technical-review.v1", "wrong contract")
    require(review.get("task") == "TASK-260712-n11rg6", "wrong task")
    reviewer = review.get("reviewer", {})
    require(reviewer.get("independenceRequired") is True, "independence requirement removed")
    require(reviewer.get("independenceSatisfied") is False, "current root cannot claim independence")
    require(reviewer.get("independentApprovalTask") == "TASK-260716-2l5j1a", "approval task drifted")
    require(reviewer.get("independentApprover") == "Ivan Oparin", "approver drifted")
    require(reviewer.get("approvalStatus") == "required", "approval falsely completed")
    decision = review.get("decision", {})
    require(decision.get("result") == "engineering-review-complete-production-blocked", "decision drifted")
    require(decision.get("productionTargetsAllowed") is False, "production targets allowed")
    require(decision.get("phase2PromotionAllowed") is False, "Phase 2 promotion allowed")
    require(decision.get("manualSecurityClaim") is False, "manual security falsely claimed")
    anchors = review.get("sourceAnchors", [])
    require(len(anchors) == 12, "source inventory incomplete")
    for item in anchors:
        path = ROOT / item["path"]
        require(path.is_file(), f"missing source: {item['path']}")
        require(SHA256.fullmatch(item.get("sha256", "")), "malformed digest")
        require(hashlib.sha256(path.read_bytes()).hexdigest() == item["sha256"], f"digest mismatch: {item['path']}")
    contract = json.loads((ROOT / "acceptance/targets-inbox-contract-v1.json").read_text())
    require(contract["strictJSON"]["duplicateFields"] == "reject", "duplicate-field rule drifted")
    require(contract["targetSnapshot"]["laterMemberExpansionAllowed"] is False, "snapshot expansion allowed")
    require(contract["mediaDelivery"]["targetedTrack"]["broadcastFallbackAllowed"] is False, "broadcast fallback allowed")
    require(contract["inboxEligibility"]["autoPlayOnReconnect"] is False, "late autoplay allowed")
    require(contract["moderation"]["reportImmediateGlobalEffect"] == "none", "report gained global effect")
    source = (ROOT / "coordinator/cmd/duet-coordinator/content_policy_http.go").read_text()
    require("decodeStrictJSON(w, r, 1024, &request)" in source, "strict consent decoder missing")
    findings = review.get("findings", [])
    require([x.get("id") for x in findings] == [f"P2-TGT-{i:03d}" for i in range(1, 5)], "finding inventory drifted")
    require(findings[0].get("severity") == "high" and findings[0].get("status") == "fixed-re-reviewed", "consent High drifted")
    require(findings[1].get("status") == "fixed-re-reviewed", "cursor finding drifted")
    require([x.get("status") for x in findings[2:]] == ["open-manual-blocking", "open-external-review"], "open High silently closed")
    require(review.get("productionBlockers") == ["P2-TGT-003", "P2-TGT-004"], "blockers drifted")
    require(len(review.get("reruns", [])) == 6 and all(x.get("status") in {"pass", "pass-after-fix"} for x in review["reruns"]), "reruns incomplete")
    require(len(review["automatedEvidenceBoundary"]["proved"]) == 8, "proof inventory incomplete")
    require(len(review["automatedEvidenceBoundary"]["notProved"]) == 5, "manual boundary incomplete")
    require(review.get("manualEpic") == "EPIC-260714-th54l3", "manual epic drifted")
    require(review.get("nextEngineeringTask") == "TASK-260712-qi81vf", "strict next task drifted")


if __name__ == "__main__":
    data = load()
    validate(data)
    print(json.dumps({"contract": data["contract"], "fixedHigh": 1, "fixedMedium": 1,
                      "openHigh": 2, "productionTargetsAllowed": False, "status": "pass"}, sort_keys=True))
