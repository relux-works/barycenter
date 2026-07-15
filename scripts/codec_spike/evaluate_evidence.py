#!/usr/bin/env python3
"""Evaluate codec-spike evidence without dropping failed or slow samples."""

from __future__ import annotations

import argparse
import hashlib
import json
import math
import pathlib
import re
import sys
from collections import defaultdict


ROOT = pathlib.Path(__file__).resolve().parents[2]
RUBRIC_PATH = ROOT / "acceptance" / "codec-spike" / "rubric-v1.json"
SHA256 = re.compile(r"^[0-9a-f]{64}$")
GIT_SHA = re.compile(r"^[0-9a-f]{40}$")


def nearest_rank_p95(values: list[float]) -> float:
    if not values:
        raise ValueError("p95 requires samples")
    ordered = sorted(values)
    return ordered[math.ceil(0.95 * len(ordered)) - 1]


def load_json(path: pathlib.Path) -> dict:
    return json.loads(path.read_text(encoding="utf-8"))


def expected_environments(evidence: dict, pairings: list[str]) -> tuple[dict[str, list[str]], list[str]]:
    errors: list[str] = []
    environments = evidence.get("environments", [])
    by_pairing: dict[str, list[str]] = {}
    expected_platforms = {
        "windows_windows": ["windows", "windows"],
        "windows_macos": ["macos", "windows"],
        "macos_macos": ["macos", "macos"],
    }
    for environment in environments:
        pairing = environment.get("pairing")
        nodes = environment.get("nodes", [])
        if pairing in by_pairing:
            errors.append(f"duplicate environment {pairing}")
            continue
        ids = [node.get("id") for node in nodes]
        platforms = sorted(node.get("platform") for node in nodes)
        if len(ids) != 2 or len(set(ids)) != 2 or platforms != expected_platforms.get(pairing):
            errors.append(f"invalid two-node topology for {pairing}")
        for node in nodes:
            if not node.get("osBuild") or not node.get("arch") or not SHA256.fullmatch(node.get("packageSha256", "")):
                errors.append(f"incomplete node provenance for {pairing}/{node.get('id')}")
        by_pairing[pairing] = ids
    if set(by_pairing) != set(pairings):
        errors.append("environment pairing coverage is incomplete")
    return by_pairing, errors


def expected_groups(metric: str, rubric: dict, nodes: dict[str, list[str]]) -> set[tuple]:
    fixtures = rubric["fixtureClasses"]
    all_ids = [item["id"] for item in fixtures]
    seek_ids = [item["id"] for item in fixtures if item["seekHeavy"]]
    one_hour_ids = [item["id"] for item in fixtures if item["durationSeconds"] == 3600]
    groups: set[tuple] = set()
    for pairing in rubric["requiredPairings"]:
        if metric == "scheduled_skew_ms":
            groups.update((pairing, fixture_id) for fixture_id in one_hour_ids)
        elif metric in ("duration_rss_growth_mib", "duration_rss_slope_mib_per_hour"):
            groups.update((pairing, node_id) for node_id in nodes.get(pairing, []))
        else:
            ids = seek_ids if metric == "seek_to_audio_ms" else all_ids
            groups.update((pairing, node_id, fixture_id)
                          for node_id in nodes.get(pairing, []) for fixture_id in ids)
    return groups


def sample_group(metric: str, sample: dict) -> tuple:
    pairing = sample.get("pairing")
    if metric == "scheduled_skew_ms":
        return pairing, sample.get("fixtureId")
    if metric in ("duration_rss_growth_mib", "duration_rss_slope_mib_per_hour"):
        return pairing, sample.get("nodeId")
    return pairing, sample.get("nodeId"), sample.get("fixtureId")


def evaluate(path: pathlib.Path, engineering: bool = False) -> dict:
    rubric = load_json(RUBRIC_PATH)
    evidence = load_json(path)
    errors: list[str] = []
    if evidence.get("schemaVersion") != 1 or evidence.get("rubric") != rubric["contract"]:
        errors.append("unsupported evidence schema or rubric")
    candidate_ids = {item["id"] for item in rubric["candidates"]}
    if evidence.get("candidateId") not in candidate_ids:
        errors.append("candidate is outside the frozen shortlist")
    build = evidence.get("build", {})
    if not GIT_SHA.fullmatch(build.get("gitCommit", "")):
        errors.append("git commit provenance invalid")
    for field in ("buildSha256", "sbomSha256"):
        if not SHA256.fullmatch(build.get(field, "")):
            errors.append(f"{field} provenance invalid")
    if not SHA256.fullmatch(evidence.get("corpus", {}).get("lockSha256", "")):
        errors.append("fixture-lock provenance invalid")

    claim_class = evidence.get("claimClass")
    final_claim = claim_class == "real-packaged-hardware"
    if not engineering and not final_claim:
        errors.append("final evaluation requires real-packaged-hardware evidence")
    nodes, environment_errors = expected_environments(evidence, rubric["requiredPairings"])
    errors.extend(environment_errors)
    if final_claim:
        for environment in evidence.get("environments", []):
            if any(node.get("realHardware") is not True for node in environment.get("nodes", [])):
                errors.append(f"{environment.get('pairing')} is not real hardware")

    coverage = evidence.get("coverage", {})
    all_fixture_ids = (
        {item["id"] for item in rubric["fixtureClasses"]} |
        set(rubric["smokeFixtures"]) |
        {item["id"] for item in rubric["hostileFixtures"]}
    )
    if set(coverage.get("pairings", [])) != set(rubric["requiredPairings"]):
        errors.append("declared pairing coverage incomplete")
    if set(coverage.get("fixtureIds", [])) != all_fixture_ids:
        errors.append("declared fixture coverage incomplete")
    if set(coverage.get("rangeProfiles", [])) != set(rubric["rangeProfiles"]):
        errors.append("declared range-profile coverage incomplete")

    artifacts = evidence.get("artifacts", [])
    artifact_kinds = [item.get("kind") for item in artifacts]
    if set(artifact_kinds) != set(rubric["requiredArtifactKinds"]) or len(artifact_kinds) != len(set(artifact_kinds)):
        errors.append("required artifact inventory incomplete or duplicated")
    for artifact in artifacts:
        relative = pathlib.PurePosixPath(artifact.get("path", ""))
        if (relative.is_absolute() or ".." in relative.parts or not relative.parts or
                not SHA256.fullmatch(artifact.get("sha256", "")) or artifact.get("bytes", 0) <= 0):
            errors.append(f"invalid artifact provenance: {artifact.get('kind')}")

    samples_by_metric: dict[str, list[dict]] = defaultdict(list)
    known_metrics = {gate["metric"] for gate in rubric["hardGates"]}
    for sample in evidence.get("samples", []):
        samples_by_metric[sample.get("metric", "")].append(sample)
    if set(samples_by_metric) - known_metrics:
        errors.append("evidence contains unknown metrics")
    metric_results: dict[str, dict] = {}
    for gate in rubric["hardGates"]:
        metric = gate["metric"]
        groups: dict[tuple, list[dict]] = defaultdict(list)
        for sample in samples_by_metric.get(metric, []):
            groups[sample_group(metric, sample)].append(sample)
        expected = expected_groups(metric, rubric, nodes)
        unexpected = set(groups) - expected
        missing = expected - set(groups)
        if unexpected:
            errors.append(f"{metric} has unexpected groups")
        if missing:
            errors.append(f"{metric} missing {len(missing)} groups")
        group_results = []
        for group in sorted(expected & set(groups), key=str):
            rows = groups[group]
            warmups = [row for row in rows if row.get("warmup") is True]
            measured = [row for row in rows if row.get("warmup") is False]
            group_errors = []
            if len(warmups) != gate["warmups"] or len(measured) != gate["samples"]:
                group_errors.append("sample count mismatch")
            if {row.get("iteration") for row in warmups} != set(range(gate["warmups"])):
                group_errors.append("warmup iteration sequence mismatch")
            if {row.get("iteration") for row in measured} != set(range(gate["samples"])):
                group_errors.append("measured iteration sequence mismatch")
            if any(row.get("unit") != gate["unit"] for row in rows):
                group_errors.append("unit mismatch")
            if any(row.get("success") is not True for row in rows):
                group_errors.append("failed sample retained")
            values = [float(row["value"]) for row in measured if row.get("success") is True]
            if metric != "duration_rss_slope_mib_per_hour" and any(value < 0 for value in values):
                group_errors.append("negative metric value")
            value = None
            if values:
                if gate["method"] == "nearest-rank-p95":
                    value = nearest_rank_p95(values)
                elif gate["method"] == "maximum":
                    value = max(values)
                elif gate["method"] == "absolute-maximum":
                    value = max(abs(item) for item in values)
            passed = not group_errors and value is not None and value <= gate["limit"]
            if not passed:
                errors.append(f"{metric} failed group {group}")
            group_results.append({"group": group, "value": value, "limit": gate["limit"],
                                  "pass": passed, "errors": group_errors})
        metric_results[metric] = {"method": gate["method"], "groups": group_results,
                                  "pass": len(missing) == 0 and all(item["pass"] for item in group_results)}

    checks = evidence.get("checks", [])
    check_map = {item.get("id"): item for item in checks}
    if set(check_map) != set(rubric["zeroFailureChecks"]):
        errors.append("zero-failure check coverage incomplete")
    for check_id, check in check_map.items():
        if check.get("passed") is not True or check.get("failureCount") != 0:
            errors.append(f"zero-failure check failed: {check_id}")
    if evidence.get("failures"):
        errors.append("evidence contains retained failures")

    status = "fail"
    if not errors:
        status = "pass" if final_claim else "engineering-pass"
    encoded = path.read_bytes()
    return {
        "schemaVersion": 1,
        "rubric": rubric["contract"],
        "status": status,
        "finalClaim": status == "pass",
        "evidenceSha256": hashlib.sha256(encoded).hexdigest(),
        "errors": errors,
        "metrics": metric_results,
    }


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("evidence", type=pathlib.Path)
    parser.add_argument("--output", type=pathlib.Path)
    parser.add_argument("--engineering", action="store_true")
    args = parser.parse_args()
    result = evaluate(args.evidence, args.engineering)
    encoded = json.dumps(result, indent=2, sort_keys=True) + "\n"
    if args.output:
        args.output.parent.mkdir(parents=True, exist_ok=True)
        args.output.write_text(encoded, encoding="utf-8")
    else:
        sys.stdout.write(encoded)
    return 0 if result["status"] in ("pass", "engineering-pass") else 1


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except (OSError, ValueError, KeyError, json.JSONDecodeError) as error:
        print(f"codec evidence evaluation: {error}", file=sys.stderr)
        raise SystemExit(1)
