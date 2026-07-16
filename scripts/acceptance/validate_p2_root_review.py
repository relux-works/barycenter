#!/usr/bin/env python3
"""Fail-closed validation for the Phase 2 root integration review."""

from __future__ import annotations

import collections
import json
import pathlib
import subprocess
import tempfile


ROOT = pathlib.Path(__file__).resolve().parents[2]
REVIEW_PATH = ROOT / "acceptance/phase2/root-integration-review-v1.json"
MANIFEST_PATH = ROOT / "docs/analysis/p2-root-review-manifest.json"
GENERATOR = ROOT / "scripts/review/generate_p2_root_manifest.py"


class ReviewError(ValueError):
    pass


def require(condition: bool, message: str) -> None:
    if not condition:
        raise ReviewError(message)


def load(path: pathlib.Path = REVIEW_PATH) -> dict:
    return json.loads(path.read_text(encoding="utf-8"))


def git(*args: str) -> str:
    return subprocess.run(
        ["git", *args], cwd=ROOT, check=True, text=True, capture_output=True
    ).stdout.strip()


def validate(review: dict, verify_manifest: bool = True) -> None:
    require(review.get("schemaVersion") == 1, "unsupported schema")
    require(review.get("contract") == "p2-root-integration-review.v1", "wrong contract")
    require(review.get("task") == "TASK-260712-1kfnpu", "wrong task")
    require(review.get("reviewedAt") == "2026-07-16", "review date drifted")

    baseline = review.get("baselineCommit", "")
    candidate = review.get("reviewedSourceCommit", "")
    tree = review.get("reviewedSourceTree", "")
    require(git("rev-parse", baseline) == baseline, "baseline commit unavailable")
    require(git("rev-parse", candidate) == candidate, "reviewed source unavailable")
    require(git("rev-parse", f"{candidate}^{{tree}}") == tree, "reviewed tree drifted")

    reviewer = review.get("reviewer", {})
    require(reviewer.get("rootReviewPerformed") is True, "root review not recorded")
    require(reviewer.get("independentReviewer") is False, "inline review claimed independence")
    require(reviewer.get("independentApprover") == "Ivan Oparin", "approver drifted")
    require(reviewer.get("independentApprovalStatus") == "required", "approval falsely closed")

    decision = review.get("decision", {})
    require(decision.get("result") == "engineering-baseline-accepted-production-blocked", "decision drifted")
    require(decision.get("reversibleEngineeringContinuationAllowed") is True, "engineering continuation blocked")
    for key in ("phase2ProductionAccepted", "phase2PromotionAllowed", "betaAllowed", "manualEvidenceClaimed"):
        require(decision.get(key) is False, f"fail-closed decision lost: {key}")
    require(decision.get("codecSelection") == "no-go", "codec no-go lost")

    accepted = review.get("acceptedArtifacts", {})
    require(accepted.get("sourceCommit") == candidate, "accepted source commit drifted")
    require(accepted.get("sourceTree") == tree, "accepted source tree drifted")
    for key in ("productionBuildSha256", "productionPackageSha256", "productionRuntimeConfigSha256"):
        require(accepted.get(key) is None, f"unaccepted production artifact recorded: {key}")

    manifest = json.loads(MANIFEST_PATH.read_text(encoding="utf-8"))
    require(manifest.get("baseline") == baseline, "manifest baseline drifted")
    require(manifest.get("reviewed_candidate") == candidate, "manifest candidate drifted")
    require(manifest.get("review_decision") == "engineering-baseline-accepted-production-blocked", "manifest decision drifted")
    boundary = manifest.get("acceptance_boundary", {})
    require(boundary.get("production_phase2") == "blocked", "manifest allowed production")
    require(boundary.get("codec_selection") == "no-go", "manifest codec gate drifted")
    require(boundary.get("accepted_build") is None and boundary.get("accepted_package") is None,
            "manifest recorded an unaccepted binary")

    inventory = review.get("inventory", {})
    totals = manifest.get("totals", {})
    expected_totals = {
        "firstParentIntervals": "first_parent_intervals",
        "phase2Tasks": "unique_phase2_tasks",
        "repositoryContextIntervals": "repository_context_intervals",
        "changedPathsNoRenames": "changed_files",
        "addedLinesNoRenames": "added_lines",
        "deletedLinesNoRenames": "deleted_lines",
        "unmappedPaths": "unmapped_files",
    }
    for review_key, manifest_key in expected_totals.items():
        require(inventory.get(review_key) == totals.get(manifest_key), f"inventory drifted: {review_key}")
    actual_classes = dict(sorted(collections.Counter(item["classification"] for item in manifest["files"]).items()))
    require(inventory.get("classifications") == actual_classes, "classification inventory drifted")

    if verify_manifest:
        with tempfile.TemporaryDirectory() as directory:
            generated = pathlib.Path(directory) / "manifest.json"
            subprocess.run([
                "python3", str(GENERATOR), "--baseline", baseline,
                "--candidate", candidate, "--output", str(generated),
            ], cwd=ROOT, check=True, capture_output=True, text=True)
            require(generated.read_bytes() == MANIFEST_PATH.read_bytes(), "manifest is not deterministic")

    expected_reviews = {
        "acceptance/codec-spike/independent-supply-review-v1.json": (
            "block-phase2", set(), {f"P2-CODEC-SUPPLY-{index:03d}" for index in range(1, 7)}
        ),
        "acceptance/phase2/stream-performance-review-v1.json": (
            "engineering-review-complete-production-blocked", {"P2-PERF-001"},
            {"P2-PERF-002", "P2-PERF-003", "P2-PERF-004"}
        ),
        "acceptance/phase2/air-migration-review-v1.json": (
            "engineering-review-complete-production-blocked", {"P2-AIR-001", "P2-AIR-002"},
            {"P2-AIR-003", "P2-AIR-004"}
        ),
        "acceptance/phase2/target-security-review-v1.json": (
            "engineering-review-complete-production-blocked", {"P2-TGT-001", "P2-TGT-002"},
            {"P2-TGT-003", "P2-TGT-004"}
        ),
    }
    records = {item.get("path"): item for item in review.get("independentReviews", [])}
    require(set(records) == set(expected_reviews), "independent review inventory drifted")
    open_high: set[str] = set()
    for path, (expected_decision, expected_fixed, expected_open) in expected_reviews.items():
        record = records[path]
        source = json.loads((ROOT / path).read_text(encoding="utf-8"))
        require(source.get("decision", {}).get("result") == expected_decision, f"source decision drifted: {path}")
        findings = {item["id"]: item for item in source.get("findings", [])}
        require(set(record.get("fixedFindings", [])) == expected_fixed, f"fixed findings drifted: {path}")
        require(set(record.get("openHighFindings", [])) == expected_open, f"open findings drifted: {path}")
        for finding_id in expected_fixed:
            require(findings[finding_id]["status"] == "fixed-re-reviewed", f"fix reopened: {finding_id}")
        for finding_id in expected_open:
            require(findings[finding_id]["severity"] == "high", f"severity lowered: {finding_id}")
            require(findings[finding_id]["status"].startswith("open-"), f"finding falsely closed: {finding_id}")
        open_high.update(expected_open)

    holds = review.get("manualAndExternalHolds", {})
    require(holds.get("manualEpic") == "EPIC-260714-th54l3", "manual epic drifted")
    require(holds.get("externalEpic") == "EPIC-260714-zmnd4n", "external epic drifted")
    require(holds.get("openHighFindingCount") == len(open_high) == 13, "open high count drifted")
    require(holds.get("acceptedProductionBuild") is None, "production build falsely accepted")
    require(holds.get("observabilityEngineeringComplete") is True, "observability engineering reopened")
    require(holds.get("observabilityCampaignEvidenceComplete") is False, "manual observability evidence falsely complete")
    require(holds.get("betaSevenDayGateComplete") is False, "seven-day beta falsely complete")

    require({item.get("id") for item in review.get("verification", [])} == {
        "coordinator-full-race", "windows-full-race", "macos-full-suite",
        "acceptance-contract-suite", "hosted-four-job-ci",
    }, "verification inventory drifted")
    require(all(item.get("status") == "pass" for item in review["verification"]), "verification failure recorded")
    require(review.get("nextEngineeringTask") == "TASK-260712-3a0cf9", "strict next task drifted")


def main() -> int:
    review = load()
    validate(review)
    print(json.dumps({
        "contract": review["contract"],
        "decision": review["decision"]["result"],
        "openHigh": review["manualAndExternalHolds"]["openHighFindingCount"],
        "productionBuild": None,
        "status": "pass",
    }, sort_keys=True))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
