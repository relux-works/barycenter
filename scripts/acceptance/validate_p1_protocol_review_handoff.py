#!/usr/bin/env python3
"""Fail-closed validation for the independent Phase 1 protocol handoff."""

from __future__ import annotations

import hashlib
import json
import pathlib
import subprocess


ROOT = pathlib.Path(__file__).resolve().parents[2]
PACKET_PATH = ROOT / "acceptance/phase1/protocol-independent-review-handoff-v2.json"
REPORT_PATH = ROOT / "docs/reviews/p1-independent-protocol-review-handoff-v2.md"


class HandoffError(ValueError):
    pass


def require(condition: bool, message: str) -> None:
    if not condition:
        raise HandoffError(message)


def load(path: pathlib.Path = PACKET_PATH) -> dict:
    return json.loads(path.read_text(encoding="utf-8"))


def git(*args: str, check: bool = True) -> bytes:
    return subprocess.run(
        ["git", *args], cwd=ROOT, check=check, capture_output=True
    ).stdout


def rev_parse(spec: str) -> str:
    return git("rev-parse", spec).decode().strip()


def golden_names(commit: str) -> list[str]:
    return git("ls-tree", "-r", "--name-only", commit, "protocol/golden").decode().splitlines()


def digest(raw: bytes) -> str:
    return hashlib.sha256(raw).hexdigest()


def validate(packet: dict) -> None:
    require(packet.get("schemaVersion") == 1, "unsupported schema")
    require(packet.get("contract") == "p1-independent-protocol-review-handoff.v2",
            "wrong contract")
    require(packet.get("preparationTask") == "TASK-260715-3ffm3r",
            "wrong preparation task")
    require(packet.get("originalTask") == "TASK-260712-176b74",
            "wrong original task")

    decision = packet.get("decision", {})
    require(decision.get("packetReady") is True, "packet not ready")
    for key in (
        "independentReviewComplete", "originalTaskAccepted",
        "productionOrStoreAuthorityGranted",
    ):
        require(decision.get(key) is False, f"fabricated acceptance: {key}")
    require(decision.get("reviewerIdentity") is None, "reviewer identity was invented")
    require(decision.get("reviewerDecision") == "not-recorded",
            "reviewer verdict was invented")

    review = packet.get("reviewRange", {})
    baseline = review.get("phase1AcceptedMerge", "")
    candidate = review.get("currentMainCandidate", "")
    require(rev_parse(baseline) == baseline, "baseline commit unavailable")
    require(rev_parse(candidate) == candidate, "candidate commit unavailable")
    require(rev_parse(f"{baseline}^{{tree}}") == review.get("phase1AcceptedTree"),
            "baseline tree drifted")
    require(rev_parse(f"{candidate}^{{tree}}") == review.get("currentMainTree"),
            "candidate tree drifted")
    ancestor = subprocess.run(
        ["git", "merge-base", "--is-ancestor", candidate, "HEAD"], cwd=ROOT
    )
    require(ancestor.returncode == 0, "candidate is not an ancestor of HEAD")

    paths = review.get("reviewPathSet", [])
    require(len(paths) == 7 and len(set(paths)) == 7, "review path set drifted")
    name_status = git("diff", "--name-status", f"{baseline}..{candidate}", "--", *paths)
    numstat = git("diff", "--numstat", f"{baseline}..{candidate}", "--", *paths)
    require(digest(name_status) == review.get("nameStatusSHA256"),
            "name/status digest drifted")
    require(digest(numstat) == review.get("numstatSHA256"), "numstat digest drifted")
    require(len(name_status.decode().splitlines()) == review.get("changedPathCount") == 51,
            "changed path count drifted")
    additions = deletions = 0
    for line in numstat.decode().splitlines():
        added, deleted, _ = line.split("\t", 2)
        additions += 0 if added == "-" else int(added)
        deletions += 0 if deleted == "-" else int(deleted)
    require(additions == review.get("addedLines") == 4610, "addition count drifted")
    require(deletions == review.get("deletedLines") == 25, "deletion count drifted")

    inventory = packet.get("protocolInventory", {})
    baseline_names = golden_names(baseline)
    candidate_names = golden_names(candidate)
    require(len(baseline_names) == inventory.get("baselineGoldenCount") == 39,
            "baseline golden count drifted")
    require(len(candidate_names) == inventory.get("currentGoldenCount") == 59,
            "candidate golden count drifted")
    require(digest(("\n".join(baseline_names) + "\n").encode()) ==
            inventory.get("baselineGoldenNameListSHA256"), "baseline name digest drifted")
    require(digest(("\n".join(candidate_names) + "\n").encode()) ==
            inventory.get("currentGoldenNameListSHA256"), "candidate name digest drifted")
    baseline_set, candidate_set = set(baseline_names), set(candidate_names)
    require(sorted(baseline_set - candidate_set) == inventory.get("removedOriginalGoldenPaths") == [],
            "an original golden was removed")
    require(sorted(candidate_set - baseline_set) == inventory.get("additiveGoldenPaths"),
            "additive golden inventory drifted")
    modified = git(
        "diff", "--name-only", "--diff-filter=M", f"{baseline}..{candidate}",
        "--", "protocol/golden",
    ).decode().splitlines()
    require(modified == inventory.get("modifiedOriginalGoldenPaths") ==
            ["protocol/golden/state.json"], "modified original golden inventory drifted")
    require(inventory.get("originalGoldenPreservedCount") == 39,
            "original preservation count drifted")
    require(inventory.get("byteUnchangedOriginalGoldenCount") == 38,
            "unchanged original count drifted")
    state = inventory.get("stateGoldenDelta", {})
    require(rev_parse(f"{baseline}:protocol/golden/state.json") == state.get("baselineBlob"),
            "baseline state golden drifted")
    require(rev_parse(f"{candidate}:protocol/golden/state.json") == state.get("candidateBlob"),
            "candidate state golden drifted")
    require(state.get("classification") == "additive-optional-capture-quality-object",
            "state delta classification drifted")
    require(state.get("p1FieldsRemovedOrRenamed") is False,
            "state delta claims a destructive Phase 1 change")

    objects = packet.get("candidateAuthorityObjects", {})
    object_specs = {
        "protocolGoldenTree": "protocol/golden",
        "coordinatorProtocolTree": "coordinator/internal/protocol",
        "windowsWireTree": "pulsar-win/wire",
        "swiftProtocolBlob": "node-app/Sources/NodeCore/Protocol.swift",
        "swiftContractTestsBlob": "node-app/Tests/NodeCoreTests/ProtocolContractTests.swift",
        "coordinatorContractTestsBlob": "coordinator/internal/protocol/codec_test.go",
        "windowsContractTestsBlob": "pulsar-win/wire/golden_test.go",
        "protocolDocumentBlob": "docs/protocol.md",
        "p1ClipContractBlob": "docs/analysis/p1-clip-transmission-wire-contract.md",
    }
    require(set(objects) == set(object_specs), "authority object inventory drifted")
    for key, path in object_specs.items():
        require(rev_parse(f"{candidate}:{path}") == objects.get(key),
                f"authority object drifted: {key}")

    requirements = packet.get("requiredIndependentReview", [])
    require(len(requirements) == 6, "independent review checklist drifted")
    require(any("reviewer identity" in item.lower() for item in requirements),
            "reviewer identity requirement missing")
    boundary = packet.get("evidenceBoundary", {})
    require(boundary.get("repositoryChecksMayBeClaimed") is True,
            "repository evidence boundary missing")
    for key in (
        "independentVerdictMayBeClaimed", "manualRealAppEvidenceMayBeClaimed",
        "storeSubmissionMayBeClaimed",
    ):
        require(boundary.get(key) is False, f"external evidence fabricated: {key}")
    require(boundary.get("remainingManualEpic") == "EPIC-260714-th54l3",
            "manual epic boundary drifted")

    report = REPORT_PATH.read_text(encoding="utf-8")
    for value in (baseline, candidate, packet["preparationTask"], packet["originalTask"]):
        require(value in report, f"report omits authority value: {value}")


def main() -> int:
    try:
        validate(load())
    except (HandoffError, OSError, subprocess.CalledProcessError, json.JSONDecodeError) as error:
        print(f"p1 protocol review handoff invalid: {error}")
        return 1
    print("p1 protocol independent-review handoff valid; external verdict remains open")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
