#!/usr/bin/env python3
"""Validate the comparative matrix without permitting score-based selection."""

from __future__ import annotations

import argparse
import json
from pathlib import Path
from typing import Any

import generate_comparative_matrix as generator


MATRIX_PATH = generator.OUTPUT
EXPECTED_COMBINATIONS = {
    "bundled-ffmpeg-both-platforms",
    "native-mf-plus-avfoundation",
    "pure-go-both-platforms",
}
EXPECTED_GATES = {
    "all_required_formats",
    "start_before_full_download",
    "pause_seek_resume_drain_cancel",
    "scheduled_start_p95_30",
    "seek_to_audio_p95_30",
    "peak_rss_2h",
    "rss_growth_2h",
    "rss_slope_2h",
    "range_fault_cache_reuse",
    "hostile_input",
    "store_sandbox_release_package",
    "license_release_disposition",
}


class MatrixError(ValueError):
    pass


def require(condition: bool, message: str) -> None:
    if not condition:
        raise MatrixError(message)


def forbidden_score_keys(value: Any, path: str = "$") -> list[str]:
    found: list[str] = []
    if isinstance(value, dict):
        for key, child in value.items():
            normalized = key.lower().replace("_", "").replace("-", "")
            if normalized.endswith("score") and key != "scoreAveragingAllowed":
                found.append(f"{path}.{key}")
            found.extend(forbidden_score_keys(child, f"{path}.{key}"))
    elif isinstance(value, list):
        for index, child in enumerate(value):
            found.extend(forbidden_score_keys(child, f"{path}[{index}]"))
    return found


def validate(matrix: dict[str, Any], *, compare_generated: bool = True) -> None:
    require(matrix.get("schemaVersion") == 1, "unsupported matrix schema")
    require(matrix.get("contract") == "p2-codec-player-comparative-matrix.v1", "wrong matrix contract")
    require(matrix.get("generatedFromRubric") == "p2-codec-spike-rubric.v1", "wrong parent rubric")
    require(matrix.get("manualEvidence") == generator.MANUAL_EPIC, "manual evidence must remain in the manual epic")
    rules = matrix.get("rules", {})
    require(rules.get("requiredPairings") == generator.PAIRINGS, "required pairings changed")
    require(rules.get("scoreAveragingAllowed") is False, "score averaging must be forbidden")
    require(not forbidden_score_keys(matrix), f"score fields are forbidden: {forbidden_score_keys(matrix)}")

    artifacts = matrix.get("artifacts", [])
    require({row.get("id") for row in artifacts} == set(generator.SOURCES), "artifact index is incomplete")
    for row in artifacts:
        require(row.get("path") == generator.SOURCES[row["id"]], f"artifact path changed: {row['id']}")
        require(row.get("sha256") == generator.digest(row["path"]), f"artifact digest mismatch: {row['id']}")

    combinations = matrix.get("combinations", [])
    require({row.get("id") for row in combinations} == EXPECTED_COMBINATIONS, "combination set changed")
    required_fixtures = rules.get("requiredFixtures")
    selectable: list[str] = []
    for combination in combinations:
        format_rows = combination.get("formatRows", [])
        require([row.get("fixtureId") for row in format_rows] == required_fixtures,
                f"fixture rows changed: {combination.get('id')}")
        gates = combination.get("hardGates", [])
        require({row.get("id") for row in gates} == EXPECTED_GATES,
                f"hard-gate set changed: {combination.get('id')}")
        require(all(row.get("status") in {"pass", "fail", "not-run"} for row in gates),
                f"invalid gate status: {combination.get('id')}")
        require(all(row.get("manualEpic") == generator.MANUAL_EPIC for row in gates if row.get("status") == "not-run"),
                f"not-run gate escaped manual routing: {combination.get('id')}")
        pairing_rows = combination.get("pairings", [])
        require([row.get("id") for row in pairing_rows] == generator.PAIRINGS,
                f"pairing rows changed: {combination.get('id')}")
        all_pass = (
            all(row.get("formatPass") is True for row in format_rows)
            and all(row.get("status") == "pass" for row in gates)
            and all(row.get("status") == "pass" for row in pairing_rows)
        )
        if all_pass:
            selectable.append(combination["id"])
        require((combination.get("conclusion") == "selected") == all_pass,
                f"combination conclusion does not follow hard gates: {combination.get('id')}")

    failures = matrix.get("rawFailures", [])
    require({row.get("combination") for row in failures} == EXPECTED_COMBINATIONS,
            "every rejected combination needs raw failure evidence")
    artifact_ids = {row["id"] for row in artifacts}
    require(all(row.get("artifact") in artifact_ids and str(row.get("jsonPointer", "")).startswith("/")
                and "observed" in row for row in failures), "raw failure evidence is not reproducible")

    selection = matrix.get("selection", {})
    require(selection.get("allowed") == bool(selectable), "selection flag does not follow all hard gates")
    require(selection.get("selectedCombination") == (selectable[0] if len(selectable) == 1 else None),
            "selected combination does not follow the unique passing row")
    require(selection.get("productionImplementationMayProceed") == bool(selectable),
            "production implementation gate does not follow selection")
    if compare_generated:
        require(matrix == generator.build_matrix(), "checked-in matrix is stale; regenerate it")


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("path", nargs="?", type=Path, default=MATRIX_PATH)
    args = parser.parse_args()
    matrix = json.loads(args.path.read_text(encoding="utf-8"))
    validate(matrix)
    print(json.dumps({
        "status": "pass",
        "contract": matrix["contract"],
        "selectionAllowed": matrix["selection"]["allowed"],
        "combinations": len(matrix["combinations"]),
        "pairingsPerCombination": len(generator.PAIRINGS),
        "manualEvidence": matrix["manualEvidence"],
    }, sort_keys=True))


if __name__ == "__main__":
    main()
