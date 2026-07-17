#!/usr/bin/env python3
"""Fail-closed validation for the non-E2EE Phase 3 root line review."""

from __future__ import annotations

import collections
import json
import pathlib
import subprocess
import tempfile


ROOT = pathlib.Path(__file__).resolve().parents[2]
REVIEW_PATH = ROOT / "acceptance/phase3/root-line-review-v1.json"
MANIFEST_PATH = ROOT / "docs/analysis/p3-root-review-manifest.json"
GENERATOR = ROOT / "scripts/review/generate_p3_root_manifest.py"


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
    require(review.get("contract") == "p3-root-line-review.v1", "wrong contract")
    require(review.get("task") == "TASK-260712-3g0axs", "wrong task")
    require(review.get("reviewedAt") == "2026-07-17", "review date drifted")

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
    require(
        decision.get("result") == "non-e2ee-engineering-baseline-accepted-production-blocked",
        "decision drifted",
    )
    require(decision.get("reversibleEngineeringContinuationAllowed") is True,
            "engineering continuation blocked")
    for key in (
        "phase3ProductionAccepted", "phase3PromotionAllowed", "betaAllowed",
        "manualEvidenceClaimed", "e2eeAccepted",
    ):
        require(decision.get(key) is False, f"fail-closed decision lost: {key}")

    accepted = review.get("acceptedArtifacts", {})
    require(accepted.get("sourceCommit") == candidate, "accepted source commit drifted")
    require(accepted.get("sourceTree") == tree, "accepted source tree drifted")
    for key in ("productionBuildSha256", "productionPackageSha256", "productionRuntimeConfigSha256"):
        require(accepted.get(key) is None, f"unaccepted production artifact recorded: {key}")

    manifest = json.loads(MANIFEST_PATH.read_text(encoding="utf-8"))
    require(manifest.get("baseline") == baseline, "manifest baseline drifted")
    require(manifest.get("reviewed_candidate") == candidate, "manifest candidate drifted")
    require(manifest.get("review_decision") == "engineering-baseline-accepted-production-blocked",
            "manifest decision drifted")
    boundary = manifest.get("acceptance_boundary", {})
    require(boundary.get("non_e2ee_phase3_engineering") == "accepted-for-reversible-continuation",
            "manifest engineering boundary drifted")
    require(boundary.get("manual_real_app_hardware") == "open in EPIC-260714-th54l3",
            "manifest manual boundary drifted")
    require(boundary.get("deferred_e2ee") == "excluded and owned by EPIC-260716-3qsztl",
            "manifest E2EE boundary drifted")
    require(boundary.get("production_promotion") == "blocked", "manifest allowed production")
    require(boundary.get("accepted_source_candidate") == candidate, "manifest source drifted")
    require(boundary.get("accepted_source_tree") == tree, "manifest tree drifted")
    require(boundary.get("accepted_build") is None and boundary.get("accepted_package") is None,
            "manifest recorded unaccepted binary")

    inventory = review.get("inventory", {})
    totals = manifest.get("totals", {})
    expected_totals = {
        "firstParentIntervals": "first_parent_intervals",
        "reviewedIntervals": "reviewed_intervals",
        "deferredE2EEIntervals": "deferred_e2ee_intervals",
        "repositoryContextIntervals": "repository_context_intervals",
        "reviewedNonE2EETasks": "unique_reviewed_tasks",
        "deferredE2EETasks": "unique_deferred_e2ee_tasks",
        "changedPathsNoRenames": "changed_files",
        "aggregateIntervalHunks": "aggregate_interval_hunks",
        "addedLinesNoRenames": "added_lines_no_renames",
        "deletedLinesNoRenames": "deleted_lines_no_renames",
        "unmappedPaths": "unmapped_files",
    }
    for review_key, manifest_key in expected_totals.items():
        require(inventory.get(review_key) == totals.get(manifest_key),
                f"inventory drifted: {review_key}")
    actual_classes = dict(sorted(collections.Counter(
        item["classification"] for item in manifest["files"]
    ).items()))
    require(inventory.get("classifications") == actual_classes, "classification inventory drifted")
    require(totals.get("unmapped_files") == 0, "unmapped files present")

    tasks = manifest.get("tasks", {})
    deferred = {key for key, value in tasks.items() if value.get("review_scope") == "deferred-e2ee"}
    require(deferred == {
        "TASK-260712-2e2ymn", "TASK-260712-16xmy2",
        "TASK-260712-3er89x", "TASK-260712-2ys1ww",
    }, "deferred E2EE inventory drifted")
    require("TASK-260712-3g0axs" in tasks, "root remediation interval missing")

    if verify_manifest:
        with tempfile.TemporaryDirectory() as directory:
            generated = pathlib.Path(directory) / "manifest.json"
            subprocess.run([
                "python3", str(GENERATOR), "--baseline", baseline,
                "--candidate", candidate, "--output", str(generated),
            ], cwd=ROOT, check=True, capture_output=True, text=True)
            require(generated.read_bytes() == MANIFEST_PATH.read_bytes(),
                    "manifest is not deterministic")

    findings = review.get("findings", [])
    require({item.get("id") for item in findings} == {
        "P3-ROOT-001", "P3-ROOT-002", "P3-ROOT-003", "P3-ROOT-004",
    }, "finding inventory drifted")
    require(not any(
        item.get("severity") in {"critical", "high"}
        and item.get("status") != "fixed-re-reviewed"
        for item in findings
    ), "unresolved critical/high finding")
    for finding in findings:
        fix = finding.get("fixCommit", "")
        require(git("merge-base", "--is-ancestor", fix, candidate) == "",
                f"finding fix is outside candidate: {finding.get('id')}")

    holds = review.get("openHolds", {})
    require(holds.get("manualEpic") == "EPIC-260714-th54l3", "manual epic drifted")
    require(holds.get("externalEpic") == "EPIC-260714-zmnd4n", "external epic drifted")
    require(holds.get("e2eeEpic") == "EPIC-260716-3qsztl", "E2EE epic drifted")
    require(holds.get("manualRealAppHardwareStatus") == "not-run", "manual evidence falsely closed")
    require(holds.get("e2eeStatus") == "deferred-unavailable", "E2EE falsely closed")
    require(holds.get("independentReviewStatus") == "required", "independent review falsely closed")
    require(holds.get("acceptedProductionBuild") is None, "production build falsely accepted")
    require(holds.get("betaStatus") == "not-run", "beta falsely closed")

    require(review.get("requiredIndependentReviewTasks") == [
        "TASK-260712-1ulshp", "TASK-260712-3j4a06",
        "TASK-260712-1x5jfo", "TASK-260712-7ng1vs",
    ], "independent review order drifted")
    require({item.get("id") for item in review.get("verification", [])} == {
        "coordinator-full-race", "windows-full-race", "macos-full-suite",
        "acceptance-contract-suite",
    }, "verification inventory drifted")
    require(all(item.get("status") == "pass" for item in review["verification"]),
            "verification failure recorded")
    require(all(item.get("candidate") == candidate for item in review["verification"]),
            "verification candidate drifted")
    require(review.get("nextEngineeringTask") == "TASK-260712-1ulshp",
            "strict next task drifted")


def main() -> int:
    review = load()
    validate(review)
    print(json.dumps({
        "contract": review["contract"],
        "decision": review["decision"]["result"],
        "manual": "not-run",
        "productionBuild": None,
        "status": "pass",
    }, sort_keys=True))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
