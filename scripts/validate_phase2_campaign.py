#!/usr/bin/env python3
"""Validate content-addressed Phase 2 campaign results against the frozen matrix."""

from __future__ import annotations

import argparse
import hashlib
import json
import re
from pathlib import Path

import validate_phase2_gate_matrix as matrix_validator


class CampaignError(ValueError):
    pass


def require(condition: bool, message: str) -> None:
    if not condition:
        raise CampaignError(message)


def sha256(path: Path) -> str:
    return hashlib.sha256(path.read_bytes()).hexdigest()


def validate_result(path: Path, campaign: Path, contract: dict, contract_hash: str) -> dict:
    value = json.loads(path.read_text(encoding="utf-8"))
    require(value.get("schemaVersion") == 1, f"unsupported result schema: {path}")
    require(value.get("contract") == "p2-gate-result.v1", f"wrong result contract: {path}")
    require(value.get("gateContract") == contract["contract"], f"wrong gate contract: {path}")
    require(value.get("gateContractSha256") == contract_hash, f"gate contract hash drift: {path}")
    require(value.get("campaignId") == campaign.name, f"campaign id mismatch: {path}")
    require(value.get("gateId") in contract["gates"], f"unknown gate: {path}")
    require(value.get("claimClass") in ("repository-automated-preflight", "manual-final", "beta-final"),
            f"unknown claim class: {path}")
    require(value.get("status") in ("not-run", "blocked", "fail", "pass"), f"bad status: {path}")
    artifacts = value.get("artifacts", [])
    if value.get("status") == "pass":
        require(value.get("startedAt") and value.get("finishedAt") and value.get("operator"),
                f"passing result lacks execution identity: {path}")
        require(value.get("samples"), f"passing result lacks samples: {path}")
        require(artifacts, f"passing result lacks artifacts: {path}")
        provenance = value.get("provenance", {})
        require(all(provenance.get(field) for field in (
            "rootGitCommit", "coordinatorSha256", "configurationSha256", "fixtureLockSha256",
        )), f"passing result lacks provenance: {path}")
        environment = value.get("environment", {})
        require(environment.get("pairingOrTopology") and environment.get("nodes") and
                environment.get("clockSyncSource") and environment.get("networkProfile"),
                f"passing result lacks environment: {path}")
        before = environment.get("clockOffsetBeforeMS")
        after = environment.get("clockOffsetAfterMS")
        require(isinstance(before, (int, float)) and isinstance(after, (int, float)) and
                abs(before) <= 10 and abs(after) <= 10, f"passing clock offset exceeds 10 ms: {path}")
        if value.get("claimClass") in ("manual-final", "beta-final"):
            require(provenance.get("windowsMSIXSha256") and provenance.get("macOSAppSha256"),
                    f"final result lacks packaged platform provenance: {path}")
        require(value.get("sanitizationReportSha256"), f"passing result lacks sanitization hash: {path}")
    artifact_hashes: set[str] = set()
    for artifact in artifacts:
        relative = Path(artifact.get("path", ""))
        require(relative.parts and not relative.is_absolute() and ".." not in relative.parts,
                f"unsafe artifact path: {path}: {relative}")
        target = campaign / relative
        require(target.is_file() and not target.is_symlink(), f"missing artifact: {relative}")
        require(artifact.get("bytes") == target.stat().st_size, f"artifact size drift: {relative}")
        require(artifact.get("sha256") == sha256(target), f"artifact hash drift: {relative}")
        artifact_hashes.add(artifact["sha256"])
    if value.get("status") == "pass":
        require(value["sanitizationReportSha256"] in artifact_hashes,
                f"sanitization report is not a bound artifact: {path}")
    return value


def validate_campaign(contract_path: Path, campaign: Path, require_beta: bool = False) -> dict:
    contract = json.loads(contract_path.read_text(encoding="utf-8"))
    matrix_validator.validate(contract, matrix_validator.ROOT)
    require(campaign.is_dir() and not campaign.is_symlink(), "campaign directory missing or unsafe")
    require(re.fullmatch(r"p2-\d{8}T\d{6}Z-[0-9a-f]{12}", campaign.name) is not None,
            "campaign id does not match frozen pattern")
    results = [validate_result(path, campaign, contract, sha256(contract_path))
               for path in sorted(campaign.rglob("result.json"))]
    require(results, "campaign has no results")
    if require_beta:
        beta = [item for item in results if item.get("gateId") == "20.6-beta"]
        require(len(beta) == 7, "beta requires exactly seven daily results")
        require(all(item.get("claimClass") == "beta-final" and item.get("status") == "pass"
                    for item in beta), "every beta day must be a passing beta-final result")
        require(not any(item.get("blockers") for item in beta), "beta result contains a blocker")
    return {"results": len(results), "passed": sum(item.get("status") == "pass" for item in results)}


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--contract", type=Path, default=matrix_validator.MATRIX)
    parser.add_argument("--campaign", type=Path, required=True)
    parser.add_argument("--require-beta", action="store_true")
    args = parser.parse_args()
    summary = validate_campaign(args.contract.resolve(), args.campaign.resolve(), args.require_beta)
    print(json.dumps({"status": "pass", **summary}, sort_keys=True))


if __name__ == "__main__":
    main()
