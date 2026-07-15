#!/usr/bin/env python3
"""Fail-closed validator for the Phase 1 engineering-readiness handoff."""

from __future__ import annotations

import argparse
import hashlib
import json
import re
import subprocess
from pathlib import Path


ROOT = Path(__file__).resolve().parents[2]
HEX40 = re.compile(r"^[0-9a-f]{40}$")
HEX64 = re.compile(r"^[0-9a-f]{64}$")
TASK = re.compile(r"^TASK-\d{6}-[a-z0-9]+$")
MANUAL_TASKS = ["TASK-260712-1vtwkl", "TASK-260712-2hodti", "TASK-260712-e5mfqj"]
EXTERNAL_TASKS = [
    "TASK-260714-200ib8",
    "TASK-260715-3ffm3r",
    "TASK-260715-s838ym",
    "TASK-260715-unbb7c",
    "TASK-260715-10ksxz",
    "TASK-260715-24ube9",
]


def load_strict(path: Path) -> dict:
    def no_duplicates(pairs: list[tuple[str, object]]) -> dict:
        result = {}
        for key, value in pairs:
            if key in result:
                raise ValueError(f"duplicate JSON key: {key}")
            result[key] = value
        return result

    return json.loads(path.read_text(), object_pairs_hook=no_duplicates)


def digest(path: Path) -> str:
    return hashlib.sha256(path.read_bytes()).hexdigest()


def git(*args: str) -> str:
    return subprocess.run(
        ["git", *args], cwd=ROOT, check=True, text=True, capture_output=True
    ).stdout.strip()


def require(condition: bool, message: str) -> None:
    if not condition:
        raise ValueError(message)


def task_card(task_id: str) -> Path:
    require(bool(TASK.fullmatch(task_id)), f"invalid task ID: {task_id}")
    matches = sorted(ROOT.glob(f".task-board/**/{task_id}_*/README.md"))
    require(len(matches) == 1, f"expected one board card for {task_id}, found {len(matches)}")
    return matches[0]


def validate(data: dict) -> None:
    require(data.get("schemaVersion") == 1, "unsupported schemaVersion")
    require(data.get("task") == "TASK-260712-1xik11", "wrong handoff task")
    require(
        data.get("decision") == "engineering-ready-for-reversible-p2-coding",
        "decision must remain engineering-only",
    )
    for field in ("releaseAccepted", "storeSubmissionAuthorized", "partnerCenterMutated"):
        require(data.get(field) is False, f"{field} must be false")

    candidate = data["candidate"]
    for field, value in candidate.items():
        require(bool(HEX40.fullmatch(value)), f"candidate.{field} must be a full Git hash")
    require(
        git("rev-parse", f"{candidate['engineeringSourceCommit']}^{{tree}}")
        == candidate["engineeringSourceTree"],
        "engineering source tree mismatch",
    )
    require(
        git("rev-parse", f"{candidate['rootReviewPacketCommit']}^{{tree}}")
        == candidate["rootReviewPacketTree"],
        "root-review packet tree mismatch",
    )
    subprocess.run(
        ["git", "merge-base", "--is-ancestor", candidate["engineeringSourceCommit"], "HEAD"],
        cwd=ROOT,
        check=True,
        capture_output=True,
    )

    review = data["rootReview"]
    require(review["status"] == "engineering-pass-release-hold", "root release hold missing")
    for path_field, hash_field in (("report", "reportSha256"), ("manifest", "manifestSha256")):
        path = ROOT / review[path_field]
        require(path.is_file(), f"missing root-review file: {path}")
        require(bool(HEX64.fullmatch(review[hash_field])), f"invalid {hash_field}")
        require(digest(path) == review[hash_field], f"hash mismatch for {path}")
    root_manifest = load_strict(ROOT / review["manifest"])
    require(
        root_manifest["reviewed_candidate"] == candidate["engineeringSourceCommit"],
        "root manifest candidate mismatch",
    )
    require(root_manifest["totals"]["unmapped_files"] == 0, "root manifest has unmapped files")
    require(review["unmappedPaths"] == 0, "readiness index has unmapped root paths")
    require(review["openCriticalOrHighEngineeringFindings"] == 0, "critical/high finding open")

    local = data["localAcceptance"]
    require(local["status"] == "pass", "local acceptance not passing")
    require(local["scope"] == "repository-automated-only", "local scope overclaims")
    require(local["commit"] == candidate["rootReviewPacketCommit"], "local commit mismatch")
    require(local["commandsPassed"] == 12, "local acceptance must contain all 12 commands")
    require(local["startDirty"] is False and local["endDirty"] is False, "local run was dirty")
    require(local["manualEvidence"] == "not-run", "local run cannot claim manual evidence")
    require(bool(HEX64.fullmatch(local["manifestSha256"])), "invalid local manifest hash")

    hosted = data["hostedAcceptance"]
    require(hosted["status"] == "pass", "hosted acceptance not passing")
    require(hosted["event"] == "pull_request", "unexpected hosted event")
    require(hosted["apiHeadSha"] == candidate["rootReviewPacketCommit"], "Actions API head mismatch")
    require(hosted["baseSha"] == candidate["engineeringSourceCommit"], "PR base mismatch")
    require(hosted["checkoutKind"] == "synthetic-pr-merge-ref", "checkout provenance hidden")
    require(hosted["checkoutSha"] != hosted["apiHeadSha"], "synthetic checkout must stay distinct")
    require(
        {job["name"] for job in hosted["jobs"]}
        == {"coordinator", "node-core", "pulsar-win", "pulsar-win-packaged-probe"},
        "hosted job inventory mismatch",
    )
    for job in hosted["jobs"]:
        require(job["status"] == "pass", f"hosted job not passing: {job['name']}")
        require(job["artifactId"] > 0 and job["artifactName"], "missing hosted artifact identity")
        if job["name"] != "pulsar-win-packaged-probe":
            require(job["recordedCheckoutSha"] == hosted["checkoutSha"], "checkout SHA mismatch")
            require(job["startDirty"] is False and job["endDirty"] is False, "hosted run dirty")
            require(bool(HEX64.fullmatch(job["manifestSha256"])), "invalid hosted manifest hash")

    probe = data["signedProbe"]
    require(probe["status"] == "engineering-pass-not-production-candidate", "probe overclaimed")
    require("not real hardware" in probe["verificationBoundary"], "probe boundary incomplete")
    require(probe["signerKind"] == "ephemeral-test-certificate", "test signer not explicit")
    require(probe["privateSigningMaterialIncluded"] is False, "private signer material claimed")
    require(probe["signatureValidAfterTemporaryTrust"] is True, "probe signature not validated")
    require(probe["cleanupPassed"] is True, "probe cleanup not passing")
    for field in ("packageSha256", "packageManifestSha256", "installReceiptSha256", "cleanupReceiptSha256"):
        require(bool(HEX64.fullmatch(probe[field])), f"invalid signedProbe.{field}")

    require(
        data["targetedGates"]
        == {
            "legalOperations": "approved",
            "policyPackExactHashes": "approved",
            "publicPolicyLiveHashesAndCache": "pass",
            "storePolicyBaseline": "valid-hold",
            "storeListingEngineeringShape": "pass",
            "storeListingReady": "expected-fail-manual-owner-required",
            "moderationOperations": "pass",
            "moderationMailboxDelivery": "expected-fail-external-action-required",
            "coordinatorGovulncheckGo1_25_12": "no-vulnerabilities-found",
            "windowsGovulncheckGo1_25_12": "no-vulnerabilities-found",
        },
        "targeted gate inventory or hold changed",
    )

    production = data["productionCandidate"]
    require(production["status"] == "manual-and-owner-required", "production hold missing")
    for field in ("signedMsixSha256", "version", "signerSubject"):
        require(production[field] is None, f"unproven production field populated: {field}")
    require(production["ownerProceed"] == "hold", "owner proceed must remain hold")
    require(production["manualTask"] in MANUAL_TASKS, "production manual task not routed")
    require(production["ownerTask"] in EXTERNAL_TASKS, "production owner task not routed")

    scenarios = data["scenarios"]
    require(set(scenarios) == {f"A{i}" for i in range(1, 9)}, "A1-A8 inventory mismatch")
    referenced_manual = set()
    for scenario, state in scenarios.items():
        require(state["automated"].startswith("engineering-pass"), f"{scenario} automation open")
        require(state["manual"] == "manual-required", f"{scenario} manual result overclaimed")
        require(state["manualTasks"], f"{scenario} lacks manual task")
        for task_id in state["manualTasks"]:
            require(task_id in MANUAL_TASKS, f"{scenario} references non-P1 manual task")
            referenced_manual.add(task_id)
    require(referenced_manual == set(MANUAL_TASKS), "not every P1 manual task is referenced")

    manual = data["manualP1Program"]
    require(manual["epic"] == "EPIC-260714-th54l3", "wrong manual epic")
    require(manual["story"] == "STORY-260714-36vmp0", "wrong manual P1 story")
    require(manual["strictOrder"] == MANUAL_TASKS, "manual task order changed")
    require(manual["allResults"] == "manual-required", "manual result overclaimed")
    for task_id in MANUAL_TASKS:
        card = task_card(task_id)
        require("EPIC-260714-th54l3" in str(card), f"{task_id} is outside manual epic")

    external = data["externalOwnerHolds"]
    require(external["epic"] == "EPIC-260714-zmnd4n", "wrong owner epic")
    require(external["owner"] == "Ivan Oparin", "wrong common owner")
    require(external["status"] == "open", "external holds silently closed")
    require(external["tasks"] == EXTERNAL_TASKS, "external hold inventory/order mismatch")
    for task_id in EXTERNAL_TASKS:
        card = task_card(task_id)
        require("EPIC-260714-zmnd4n" in str(card), f"{task_id} is outside owner epic")

    reviews = data["independentReviews"]
    require(len(reviews) == 4, "independent review inventory mismatch")
    require(
        {item["ownerTask"] for item in reviews}
        == {"TASK-260715-3ffm3r", "TASK-260715-s838ym", "TASK-260715-unbb7c", "TASK-260715-10ksxz"},
        "independent owner task mismatch",
    )
    for item in reviews:
        require(item["engineeringAudit"] == "pass-with-closed-high", "engineering audit open")
        require(item["independentSignoff"] == "open", "independence silently claimed")


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument(
        "path",
        nargs="?",
        type=Path,
        default=ROOT / "acceptance/phase1-engineering-readiness.json",
    )
    args = parser.parse_args()
    path = args.path if args.path.is_absolute() else ROOT / args.path
    validate(load_strict(path))
    print(f"phase1 engineering readiness valid: {path.relative_to(ROOT)}")


if __name__ == "__main__":
    main()
