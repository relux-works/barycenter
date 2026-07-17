#!/usr/bin/env python3
"""Validate a private Phase 3 evidence campaign against the frozen contract."""

from __future__ import annotations

import argparse
import datetime as dt
import hashlib
import json
import pathlib
import re
from typing import Any

import validate_phase3_gate_matrix as matrix_validator


class Phase3CampaignError(ValueError):
    pass


def require(condition: bool, message: str) -> None:
    if not condition:
        raise Phase3CampaignError(message)


def safe_relative(value: str) -> pathlib.Path:
    path = pathlib.Path(value)
    require(value != "" and not path.is_absolute() and ".." not in path.parts,
            f"unsafe campaign-relative path: {value}")
    return path


def sha256(path: pathlib.Path) -> str:
    return hashlib.sha256(path.read_bytes()).hexdigest()


def load(path: pathlib.Path) -> dict[str, Any]:
    return json.loads(path.read_text(encoding="utf-8"))


def validate_result(
    result: dict[str, Any], contract: dict[str, Any], campaign: dict[str, Any], root: pathlib.Path
) -> None:
    require(result.get("schemaVersion") == 1 and result.get("contract") == "p3-gate-result.v1",
            "result identity mismatch")
    require(result.get("gateContract") == contract["contract"], "result gate contract mismatch")
    require(result.get("campaignId") == campaign["campaignId"], "result campaign mismatch")
    require(result.get("rootCommit") == campaign["rootCommit"], "result root commit mismatch")
    require(result.get("gateId") in contract["gates"], "unknown result gate")
    flag_id = result.get("featureFlagId")
    require(flag_id in {row["id"] for row in contract["featureFlagMatrix"]}, "unknown flag posture")
    require(flag_id == campaign.get("featureFlagId"), "result and campaign flag postures differ")
    require(result.get("status") in ("pass", "fail", "blocked"), "result status is not terminal")
    require(result.get("manualEvidence") in ("pass", "fail", "not-applicable"),
            "result manual boundary is incomplete")
    require(isinstance(result.get("commands"), list) and result["commands"], "command receipts missing")
    require(all(item.get("exitCode") is not None for item in result["commands"]),
            "command exit receipt missing")
    require(isinstance(result.get("samples"), list), "sample records missing")
    require(isinstance(result.get("blockers"), list), "blocker records missing")
    for artifact in result.get("artifacts", []):
        relative = safe_relative(artifact.get("path", ""))
        path = root / relative
        require(path.is_file() and not path.is_symlink(), f"artifact missing or symlinked: {relative}")
        require(artifact.get("bytes") == path.stat().st_size, f"artifact length mismatch: {relative}")
        require(artifact.get("sha256") == sha256(path), f"artifact hash mismatch: {relative}")
    if result["status"] == "pass":
        require(not result["blockers"], "passing result contains blockers")
        require(all(item.get("exitCode") == 0 for item in result["commands"]),
                "passing result contains a failed command")


def validate_campaign(
    contract: dict[str, Any], root: pathlib.Path, require_beta: bool = False
) -> dict[str, Any]:
    require(root.is_dir() and not root.is_symlink(), "campaign directory missing or symlinked")
    campaign_path = root / "campaign.json"
    require(campaign_path.is_file() and not campaign_path.is_symlink(), "campaign.json missing")
    campaign = load(campaign_path)
    require(campaign.get("contract") == "p3-evidence-campaign.v1", "campaign contract mismatch")
    require(re.fullmatch(r"p3-\d{8}T\d{6}Z-[0-9a-f]{12}", campaign.get("campaignId", "")) is not None,
            "campaign id mismatch")
    require(root.name == campaign["campaignId"], "campaign directory and id differ")
    require(re.fullmatch(r"[0-9a-f]{40}", campaign.get("rootCommit", "")) is not None,
            "campaign root commit mismatch")
    require(campaign.get("gateContractSHA256") == sha256(matrix_validator.MATRIX),
            "campaign gate contract hash mismatch")
    require(campaign.get("featureFlagId") in {row["id"] for row in contract["featureFlagMatrix"]},
            "campaign flag posture mismatch")
    results = []
    for path in sorted(root.rglob("result.json")):
        require(not path.is_symlink(), f"result may not be a symlink: {path.relative_to(root)}")
        result = load(path)
        validate_result(result, contract, campaign, root)
        results.append((path, result))
    require(results, "campaign has no result records")

    if require_beta:
        beta_results = [(path, value) for path, value in results if value["gateId"] == "NF-beta"]
        require(len(beta_results) == 7, "beta requires exactly seven daily records")
        dates = []
        for expected_day, (path, value) in enumerate(beta_results, start=1):
            require(pathlib.PurePath(path.relative_to(root)).parts[:2] == ("NF-beta", f"day-{expected_day:02d}"),
                    "beta day path/order mismatch")
            require(value.get("status") == "pass" and value.get("manualEvidence") == "pass",
                    "beta day did not pass manually")
            beta = value.get("beta", {})
            require(beta.get("day") == expected_day and beta.get("durationHours") >= 24,
                    "beta day or duration mismatch")
            require(beta.get("prohibitedIncidents") == [] and beta.get("resetTriggered") is False,
                    "beta contains a prohibited incident or reset")
            dates.append(dt.date.fromisoformat(beta.get("date")))
        require(all(right - left == dt.timedelta(days=1) for left, right in zip(dates, dates[1:])),
                "beta dates are not consecutive")
    return {"campaign": campaign, "results": len(results)}


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--contract", type=pathlib.Path, default=matrix_validator.MATRIX)
    parser.add_argument("--campaign", type=pathlib.Path, required=True)
    parser.add_argument("--require-beta", action="store_true")
    args = parser.parse_args()
    contract = load(args.contract)
    matrix_validator.validate(contract)
    summary = validate_campaign(contract, args.campaign, args.require_beta)
    print(json.dumps({
        "campaignId": summary["campaign"]["campaignId"],
        "results": summary["results"], "betaRequired": args.require_beta,
    }, sort_keys=True))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
