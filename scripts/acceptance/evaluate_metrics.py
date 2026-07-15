#!/usr/bin/env python3
"""Evaluate manually captured Phase 1 metrics without manufacturing samples."""

from __future__ import annotations

import argparse
import csv
import json
import math
import pathlib
import sys


LIMITS = {
    "stop_to_audible_ms": 4000.0,
    "scheduled_skew_ms": 100.0,
    "peak_memory_mib": 250.0,
}
MIN_SAMPLES = 30
WARMUPS = 3


def nearest_rank_p95(values: list[float]) -> float:
    if not values:
        raise ValueError("p95 needs at least one value")
    ordered = sorted(values)
    return ordered[math.ceil(0.95 * len(ordered)) - 1]


def evaluate(path: pathlib.Path) -> dict:
    samples: dict[str, list[float]] = {key: [] for key in LIMITS}
    failures = 0
    warmups: dict[str, int] = {key: 0 for key in LIMITS}
    with path.open(newline="", encoding="utf-8") as stream:
        reader = csv.DictReader(stream)
        required = {"kind", "value", "warmup", "success"}
        if not reader.fieldnames or not required.issubset(reader.fieldnames):
            raise ValueError("CSV columns must include kind,value,warmup,success")
        for row in reader:
            kind = row["kind"].strip()
            if kind not in LIMITS:
                raise ValueError(f"unknown metric kind: {kind}")
            successful = row["success"].strip().lower() == "true"
            if not successful:
                failures += 1
                continue
            value = float(row["value"])
            if row["warmup"].strip().lower() == "true":
                warmups[kind] += 1
                continue
            samples[kind].append(value)
    metrics = {}
    overall = failures == 0
    for kind, values in samples.items():
        if kind == "peak_memory_mib":
            value = max(values) if values else None
            method = "maximum"
            enough = len(values) >= 1
        else:
            value = nearest_rank_p95(values) if values else None
            method = "nearest-rank-p95"
            enough = len(values) >= MIN_SAMPLES and warmups[kind] >= WARMUPS
        passed = enough and value is not None and value <= LIMITS[kind]
        metrics[kind] = {
            "method": method,
            "sampleCount": len(values),
            "value": value,
            "limit": LIMITS[kind],
            "pass": passed,
        }
        overall = overall and passed
    return {
        "schemaVersion": 1,
        "status": "pass" if overall else "fail",
        "warmupCount": warmups,
        "failedSampleCount": failures,
        "metrics": metrics,
    }


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("csv", type=pathlib.Path)
    parser.add_argument("--output", type=pathlib.Path)
    args = parser.parse_args()
    result = evaluate(args.csv)
    encoded = json.dumps(result, indent=2, sort_keys=True) + "\n"
    if args.output:
        args.output.write_text(encoded, encoding="utf-8")
    else:
        sys.stdout.write(encoded)
    return 0 if result["status"] == "pass" else 1


if __name__ == "__main__":
    raise SystemExit(main())
