#!/usr/bin/env python3
"""Fail-closed validation for the Phase 2 codec supply review."""

from __future__ import annotations

import hashlib
import json
import pathlib
import re


ROOT = pathlib.Path(__file__).resolve().parents[2]
REVIEW_PATH = ROOT / "acceptance/codec-spike/independent-supply-review-v1.json"
HANDOFF_PATH = ROOT / "acceptance/codec-spike/player-handoff-v1.json"
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
    require(review.get("contract") == "p2-independent-codec-supply-review.v1", "wrong review contract")
    require(review.get("task") == "TASK-260712-2g3fkt", "wrong review task")
    require(review.get("reviewedAt") == "2026-07-16", "review date drifted")
    require(re.fullmatch(r"[0-9a-f]{40}", review.get("reviewedBaseCommit", "")) is not None,
            "reviewed base commit missing")

    reviewer = review.get("reviewer", {})
    require(reviewer.get("independenceRequired") is True, "independent review requirement removed")
    require(reviewer.get("independenceSatisfied") is False, "current root session cannot claim independence")
    require(reviewer.get("independentApprover") == "Ivan Oparin", "independent approver drifted")
    require(reviewer.get("approvalStatus") == "required", "independent approval falsely completed")

    decision = review.get("decision", {})
    require(decision.get("result") == "block-phase2", "review must remain fail-closed")
    require(decision.get("acceptedCombination") is None, "review cannot select a codec combination")
    require(decision.get("productionPlaybackAllowed") is False, "production playback was allowed")
    require(decision.get("phase2ProductionBlocked") is True, "Phase 2 block was removed")
    require(decision.get("legalAdvice") is False, "engineering review cannot claim legal advice")
    require(review.get("nextTaskMayStart") is False, "strict sequence bypassed blocked review")

    handoff = json.loads(HANDOFF_PATH.read_text(encoding="utf-8"))
    require(handoff.get("decision", {}).get("production") == "no-go", "player handoff no longer no-go")
    require(handoff.get("decision", {}).get("selectedCombination") is None, "player handoff selected a combination")
    require(handoff.get("decision", {}).get("phase2ProductionBlocked") is True, "player handoff block removed")

    inputs = review.get("inputs", [])
    required_inputs = {
        "acceptance/codec-spike/rubric-v1.json",
        "acceptance/codec-spike/license-audit-v1.json",
        "acceptance/codec-spike/comparative-matrix-v1.json",
        "acceptance/codec-spike/player-handoff-v1.json",
    }
    require({item.get("path") for item in inputs} == required_inputs, "review input inventory drifted")
    for item in inputs:
        path = ROOT / item["path"]
        require(path.is_file(), f"review input missing: {item['path']}")
        require(SHA256.fullmatch(item.get("sha256", "")) is not None, "review input digest malformed")
        require(digest(path) == item["sha256"], f"review input digest mismatch: {item['path']}")

    sources = review.get("authoritativeSources", [])
    source_ids = {item.get("id") for item in sources}
    required_sources = {
        "ffmpeg-legal", "ffmpeg-security", "microsoft-codecs", "apple-code-signing",
        "apple-notarization", "apple-review", "aac-pool", "opus-license", "go-vuln-db",
    }
    require(source_ids == required_sources, "authoritative source inventory drifted")
    require(all(item.get("url", "").startswith("https://") for item in sources), "non-HTTPS source")
    require(all(item.get("retrievedAt") == review["reviewedAt"] for item in sources),
            "source retrieval date missing")
    require(all(item.get("disposition") for item in sources), "source disposition missing")

    reruns = review.get("reruns", [])
    rerun_ids = {item.get("id") for item in reruns}
    require(rerun_ids == {
        "codec-contract-validators", "codec-contract-tests", "pure-go-rebuild",
        "macos-native-rebuild", "pure-go-vulnerability-scan",
    }, "representative rerun inventory drifted")
    require(all(item.get("command") and item.get("claim") for item in reruns), "rerun evidence incomplete")
    require({item.get("status") for item in reruns} <= {
        "pass", "expected-rejection", "pass-called-symbols-only",
    }, "rerun status falsely claims release acceptance")

    findings = review.get("findings", [])
    finding_ids = [item.get("id") for item in findings]
    require(finding_ids == [f"P2-CODEC-SUPPLY-{number:03d}" for number in range(1, 7)],
            "blocking finding inventory drifted")
    require(all(item.get("severity") == "high" for item in findings), "blocking severity was lowered")
    require(all(item.get("status") == "open-blocking" for item in findings),
            "unreviewed high finding was closed")
    require(all(item.get("requiredClosure") and item.get("owner") == "Ivan Oparin" for item in findings),
            "finding closure or owner missing")

    reopen = set(review.get("reopenRequirements", []))
    require(len(reopen) == 8, "reopen requirements incomplete")
    for required in (
        "select-one-exact-combination-that-passes-every-hard-gate",
        "resolve-all-critical-and-high-findings-and-re-review-them",
        "obtain-independent-review-of-the-exact-candidate-commit",
    ):
        require(required in reopen, f"missing reopen requirement: {required}")
    require(review.get("manualEvidence") == "EPIC-260714-th54l3", "manual evidence seam drifted")


def main() -> int:
    validate(load())
    print(json.dumps({
        "contract": "p2-independent-codec-supply-review.v1",
        "decision": "block-phase2",
        "highFindings": 6,
        "independenceSatisfied": False,
        "status": "pass",
    }, sort_keys=True))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
