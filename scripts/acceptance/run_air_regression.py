#!/usr/bin/env python3
"""Run the repository-only Air lifecycle regression rehearsal.

This harness intentionally does not claim real Windows/macOS/Telegram or
audible playback evidence. Those checks belong to the manual-test epic.
"""

from __future__ import annotations

import argparse
import datetime as dt
import json
import pathlib
import re
import subprocess
import sys


ROOT = pathlib.Path(__file__).resolve().parents[2]
COORDINATOR = ROOT / "coordinator"
DEFAULT_OUTPUT = ROOT / ".temp" / "acceptance" / "air-regression-rehearsal.json"
METRICS_PATTERN = re.compile(r"AIR_REGRESSION_METRICS (\{[^\n]+\})")
EXPECTED_METRICS = {
    "barycenters": 8,
    "pulsars": 20,
    "load_commands": 20,
    "unique_targets": 20,
    "duplicate_commands": 0,
    "runtime_instances": 1,
    "legacy_groups": 0,
}


def utc_now() -> str:
    return dt.datetime.now(dt.timezone.utc).isoformat(timespec="seconds").replace("+00:00", "Z")


def command(name: str, package: str, pattern: str) -> dict:
    argv = ("go", "test", "-v", "-count=1", package, "-run", pattern)
    completed = subprocess.run(
        argv,
        cwd=COORDINATOR,
        stdout=subprocess.PIPE,
        stderr=subprocess.STDOUT,
        text=True,
        errors="replace",
    )
    sys.stdout.write(completed.stdout)
    return {
        "name": name,
        "argv": list(argv),
        "exitCode": completed.returncode,
        "output": completed.stdout,
    }


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--output", type=pathlib.Path, default=DEFAULT_OUTPUT)
    args = parser.parse_args()

    started = utc_now()
    records = [
        command(
            "air-store-lifecycle-migration-and-boundaries",
            "./internal/store",
            "^(TestAir|TestAuthorizedAir|TestMigratedApproachAlias|TestActiveLinkBackfill|"
            "TestConcurrentAir|TestUnsafeAirRollback|TestConflictingLegacyLinks|TestTelegramAir)",
        ),
        command(
            "air-loop-runtime-alias-and-capacity",
            "./cmd/duet-coordinator",
            "^(TestAir|TestApproach|TestLinkedVoice|TestProviderResolve|TestLastMemberLeave|"
            "TestMemberLeaveDuringLink|TestSlotPairedDuringLink|TestClaimantCanDecline|TestTelegramAir)",
        ),
    ]
    combined = "\n".join(record["output"] for record in records)
    matches = METRICS_PATTERN.findall(combined)
    metrics = json.loads(matches[-1]) if matches else None
    status = "pass"
    failures: list[str] = []
    if any(record["exitCode"] for record in records):
        status = "fail"
        failures.append("one or more Go regression commands failed")
    if metrics != EXPECTED_METRICS:
        status = "fail"
        failures.append(f"8/20 metrics mismatch: expected {EXPECTED_METRICS}, got {metrics}")

    artifact = {
        "schemaVersion": 1,
        "scope": "repository-automated-only",
        "manualEvidence": "not-run",
        "status": status,
        "startedAt": started,
        "finishedAt": utc_now(),
        "metrics": metrics,
        "expectedMetrics": EXPECTED_METRICS,
        "failures": failures,
        "commands": [
            {key: value for key, value in record.items() if key != "output"}
            for record in records
        ],
        "deferred": [
            "real Windows and macOS application playback",
            "real Telegram transport and callback interaction",
            "audible and physical-device verification",
            "streamed-track catch-up and performance",
            "explicit-target ACL and inbox parity",
        ],
    }
    args.output.parent.mkdir(parents=True, exist_ok=True)
    args.output.write_text(json.dumps(artifact, indent=2, sort_keys=True) + "\n", encoding="utf-8")
    print(f"air regression artifact: {args.output}")
    return 0 if status == "pass" else 1


if __name__ == "__main__":
    raise SystemExit(main())
