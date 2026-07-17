#!/usr/bin/env python3
"""Run repository-built capture safety adapters and publish sanitized evidence.

The adapters consume only the frozen synthetic corpus. They intentionally do
not claim native AEC/NS, signed-app, hardware, acoustic, CPU or memory proof.
"""

from __future__ import annotations

import argparse
import hashlib
import json
import os
import pathlib
import platform
import subprocess
import sys
import tempfile
import time
from typing import Any

import harness


ROOT = pathlib.Path(__file__).resolve().parents[2]
TASK = "TASK-260712-1023d7"
CONTRACT = "p3-capture-quality-integrated-regressions.v1"
ADAPTER_CONTRACT = "p3-capture-quality-platform-adapter.v1"
FIXTURE_IDS = [item["id"] for item in harness.contract()["fixtures"]]
WORKFLOWS = ["recorded_clip", "local_self_test", "live_ptt"]
ROUTES = ["speaker", "headphone", "unknown"]


class IntegratedError(RuntimeError):
    pass


def sha256_bytes(value: bytes) -> str:
    return hashlib.sha256(value).hexdigest()


def apple_environment() -> dict[str, str]:
    candidates = []
    if explicit := os.environ.get("DEVELOPER_DIR"):
        candidates.append(pathlib.Path(explicit))
    candidates.append(pathlib.Path("/Applications/Xcode.app/Contents/Developer"))
    for candidate in dict.fromkeys(candidates):
        if candidate.exists():
            return {"DEVELOPER_DIR": str(candidate)}
    raise IntegratedError("full Xcode is required for the macOS platform adapter")


def command_receipt(
    name: str,
    cwd: pathlib.Path,
    argv: list[str],
    extra_env: dict[str, str] | None = None,
) -> dict[str, Any]:
    environment = os.environ.copy()
    environment["PYTHONDONTWRITEBYTECODE"] = "1"
    if extra_env:
        environment.update(extra_env)
    started = time.monotonic()
    completed = subprocess.run(
        argv,
        cwd=cwd,
        env=environment,
        stdout=subprocess.PIPE,
        stderr=subprocess.STDOUT,
        text=False,
    )
    duration_ms = round((time.monotonic() - started) * 1000)
    if completed.returncode:
        sys.stderr.buffer.write(completed.stdout)
        raise IntegratedError(f"{name} failed with exit {completed.returncode}")
    return {
        "name": name,
        "cwd": cwd.relative_to(ROOT).as_posix(),
        "argv": argv,
        "exitCode": completed.returncode,
        "durationMS": duration_ms,
        "stdoutSHA256": sha256_bytes(completed.stdout),
    }


def validate_adapter(value: dict, platform_name: str, lock_hash: str, build: str) -> None:
    if value.get("schemaVersion") != 1 or value.get("contract") != ADAPTER_CONTRACT:
        raise IntegratedError(f"{platform_name} adapter contract mismatch")
    if value.get("platform") != platform_name or value.get("build") != build:
        raise IntegratedError(f"{platform_name} adapter identity mismatch")
    if value.get("fixtureLockSHA256") != lock_hash or value.get("manualEvidence") != "not-run":
        raise IntegratedError(f"{platform_name} adapter fixture/manual boundary mismatch")
    cells = value.get("cells")
    if not isinstance(cells, list) or len(cells) != len(WORKFLOWS) * len(ROUTES):
        raise IntegratedError(f"{platform_name} adapter cell count mismatch")
    expected_cells = {(workflow, route) for workflow in WORKFLOWS for route in ROUTES}
    actual_cells = {(cell.get("workflow"), cell.get("route")) for cell in cells}
    if actual_cells != expected_cells:
        raise IntegratedError(f"{platform_name} adapter matrix mismatch")
    generations: set[int] = set()
    for cell in cells:
        if cell.get("supported") is not False or cell.get("quality") != "degraded":
            raise IntegratedError(f"{platform_name} adapter invented supported quality")
        if cell.get("reason") != "aec_unavailable" or not cell.get("failClosedWithoutConsent"):
            raise IntegratedError(f"{platform_name} adapter did not fail closed")
        generation = cell.get("freshGeneration")
        if not isinstance(generation, int) or generation <= 0 or generation in generations:
            raise IntegratedError(f"{platform_name} adapter reused a generation")
        generations.add(generation)
        cases = cell.get("cases")
        if not isinstance(cases, list) or [case.get("id") for case in cases] != FIXTURE_IDS:
            raise IntegratedError(f"{platform_name} adapter fixture order/coverage mismatch")
        for case in cases:
            if case.get("safetyStagePassed") is not True:
                raise IntegratedError(f"{platform_name}/{cell.get('workflow')}/{cell.get('route')} safety failed")
            digest = case.get("processedSHA256")
            if not isinstance(digest, str) or len(digest) != 64:
                raise IntegratedError(f"{platform_name} adapter output digest missing")
            if case.get("c3Status") != "unsupported-native-effects-not-exercised":
                raise IntegratedError(f"{platform_name} adapter invented C3 status")
    runtime = value.get("runtime", {})
    if runtime.get("measurementSource") != "repository-test-adapter" or runtime.get("physicalCPUAndMemoryEvidence") != "not-run":
        raise IntegratedError(f"{platform_name} adapter runtime boundary mismatch")
    if runtime.get("callbackBlockingWaits") != 0:
        raise IntegratedError(f"{platform_name} adapter reported callback blocking")
    if platform_name == "windows" and runtime.get("callbackAllocations") != 0:
        raise IntegratedError("windows adapter reported callback allocations")
    if platform_name == "macos" and runtime.get("callbackAllocationMeasurement") != "source-guard-only":
        raise IntegratedError("macOS allocation boundary was overstated")


def load_adapter(path: pathlib.Path, platform_name: str, lock_hash: str, build: str) -> dict:
    value = json.loads(path.read_text(encoding="utf-8"))
    validate_adapter(value, platform_name, lock_hash, build)
    return value


def fault_coverage() -> list[dict[str, str]]:
    return [
        {"fault": "2-percent-loss", "status": "synthetic-pass", "source": "live_packet_cancel fixture plus platform live sender/receiver tests"},
        {"fault": "jitter-and-slow-recipient", "status": "deterministic-pass", "source": "platform live sender/receiver tests"},
        {"fault": "route-and-default-device-change", "status": "synthetic-pass-physical-not-run", "source": "route_change and device_loss fixtures plus platform lifecycle tests"},
        {"fault": "bluetooth-profile-change", "status": "synthetic-route-transition-pass-physical-not-run", "source": "unknown-route fail-closed matrix; physical profile transition is manual"},
        {"fault": "clock-drift", "status": "synthetic-pass-native-reference-not-run", "source": "clock_drift fixture at frozen 200 ppm"},
        {"fault": "clipping-and-silence", "status": "safety-stage-pass", "source": "every platform/workflow/route cell"},
        {"fault": "permission-revoke-and-device-loss", "status": "deterministic-pass", "source": "platform capture lifecycle tests"},
        {"fault": "cancel-lock-sleep-and-reconnect", "status": "deterministic-pass", "source": "platform live node and workflow tests"},
        {"fault": "feature-rollback", "status": "fail-closed-pass", "source": "capture_quality capability remains unadvertised and native effects unsupported"},
    ]


def validate_evidence(value: dict[str, Any]) -> None:
    if value.get("schemaVersion") != 1 or value.get("contract") != CONTRACT or value.get("task") != TASK:
        raise IntegratedError("integrated evidence identity mismatch")
    decision = value.get("decision", {})
    if decision.get("engineeringRegressionPassed") is not True:
        raise IntegratedError("engineering regression result is not pass")
    if decision.get("c3Accepted") is not False or decision.get("manualEvidence") != "not-run":
        raise IntegratedError("manual/C3 boundary was overstated")
    adapters = value.get("adapters")
    if not isinstance(adapters, list) or {item.get("platform") for item in adapters} != {"windows", "macos"}:
        raise IntegratedError("both platform adapters are required")
    matrix = value.get("matrix", {})
    if matrix.get("cells") != 18 or matrix.get("fixtureRuns") != 252:
        raise IntegratedError("integrated matrix count mismatch")
    if matrix.get("fixtures") != FIXTURE_IDS or matrix.get("workflows") != WORKFLOWS or matrix.get("routes") != ROUTES:
        raise IntegratedError("integrated matrix identity mismatch")
    for adapter in adapters:
        validate_adapter(
            adapter, adapter.get("platform"), value.get("fixtureLockSHA256"),
            value.get("build"))
    ceilings = value.get("ceilings", {})
    if ceilings.get("captureInputDBFS") != -3.0 or ceilings.get("receiverPostMixDBFS") != -1.0:
        raise IntegratedError("capture and receiver ceilings are inverted or mutable")
    if ceilings.get("editable") is not False:
        raise IntegratedError("capture ceiling unexpectedly became editable")
    if value.get("retention", {}).get("syntheticAudioRetained") is not False:
        raise IntegratedError("synthetic adapter audio must not be retained")
    if value.get("retention", {}).get("privateOrUserAudioAccepted") is not False:
        raise IntegratedError("private audio boundary changed")
    commands = value.get("commands")
    if not isinstance(commands, list) or len(commands) != 4 or any(item.get("exitCode") != 0 for item in commands):
        raise IntegratedError("platform adapter/lifecycle command coverage incomplete")
    unsupported = value.get("unsupportedRoutes")
    expected_unsupported = {
        (platform_name, route) for platform_name in ("windows", "macos") for route in ROUTES
    }
    if not isinstance(unsupported, list) or {
        (item.get("platform"), item.get("route")) for item in unsupported
    } != expected_unsupported or any(item.get("status") != "unsupported-pending-manual-c3" for item in unsupported):
        raise IntegratedError("unsupported route blockers are incomplete")
    if len(value.get("faultCoverage", [])) != 9 or not value.get("blockers"):
        raise IntegratedError("fault or blocker coverage incomplete")
    serialized = json.dumps(value, sort_keys=True).lower()
    for forbidden in ("/users/", "c:\\\\", "transcript", "device_id", "device_name", "token", "secret"):
        if forbidden in serialized:
            raise IntegratedError(f"forbidden metadata in sanitized evidence: {forbidden}")


def run(output: pathlib.Path) -> dict[str, Any]:
    if output.exists():
        raise IntegratedError(f"output already exists: {output}")
    output.parent.mkdir(parents=True, exist_ok=True)
    build = subprocess.check_output(
        ["git", "rev-parse", "HEAD"], cwd=ROOT, text=True).strip()
    apple_env = apple_environment()
    with tempfile.TemporaryDirectory(prefix="capture-quality-integrated-") as temporary:
        root = pathlib.Path(temporary)
        corpus, generated_lock = root / "corpus", root / "lock.json"
        harness.generate_corpus(corpus, generated_lock)
        if generated_lock.read_bytes() != harness.LOCK_PATH.read_bytes():
            raise IntegratedError("generated corpus differs from frozen fixture lock")
        lock_hash = harness.sha256_file(generated_lock)
        windows_path, macos_path = root / "windows.json", root / "macos.json"
        common = {
            "CAPTURE_QUALITY_CORPUS": str(corpus),
            "CAPTURE_QUALITY_BUILD": build,
            "CAPTURE_QUALITY_FIXTURE_LOCK_SHA256": lock_hash,
        }
        commands = [
            command_receipt(
                "windows-platform-adapter", ROOT / "pulsar-win",
                ["go", "test", "-count=1", "-run", "^TestWindowsCaptureQualityIntegratedAdapter$", "."],
                {**common, "CAPTURE_QUALITY_ADAPTER_OUTPUT": str(windows_path)},
            ),
            command_receipt(
                "macos-platform-adapter", ROOT / "node-app",
                ["xcrun", "swift", "test", "--filter", "MacCaptureQualityIntegratedAdapterTests"],
                {**common, **apple_env, "CAPTURE_QUALITY_ADAPTER_OUTPUT": str(macos_path)},
            ),
            command_receipt(
                "windows-hostile-lifecycle", ROOT / "pulsar-win",
                ["go", "test", "-count=1", "-run", "TestWindows(CaptureWorkflow|MicrophoneCaptureQuality|LiveCapture|LivePTT|CaptureQuality)", "./..."],
            ),
            command_receipt(
                "macos-hostile-lifecycle", ROOT / "node-app",
                ["xcrun", "swift", "test", "--filter", "Mac(CaptureWorkflowController|MicrophoneCaptureEngine|LiveCaptureSender|LivePTTNode|CaptureQualityProcessor)Tests"],
                apple_env,
            ),
        ]
        adapters = [
            load_adapter(windows_path, "windows", lock_hash, build),
            load_adapter(macos_path, "macos", lock_hash, build),
        ]
        evidence: dict[str, Any] = {
            "schemaVersion": 1,
            "contract": CONTRACT,
            "task": TASK,
            "publishedAt": time.strftime("%Y-%m-%d", time.gmtime()),
            "build": build,
            "fixtureLockSHA256": lock_hash,
            "decision": {
                "result": "engineering-regressions-pass-native-c3-unsupported-manual-proof-required",
                "engineeringRegressionPassed": True,
                "productionReady": False,
                "captureQualityCapabilityAdvertised": False,
                "c3Accepted": False,
                "manualEvidence": "not-run",
                "manualEpic": "EPIC-260714-th54l3",
            },
            "matrix": {
                "platforms": 2,
                "workflows": WORKFLOWS,
                "routes": ROUTES,
                "fixtures": FIXTURE_IDS,
                "cells": 18,
                "fixtureRuns": 18 * len(FIXTURE_IDS),
                "aggregation": "none; every platform/workflow/route/case remains independent",
            },
            "ceilings": {
                "captureInputDBFS": -3.0,
                "receiverPostMixDBFS": -1.0,
                "ordering": "platform safety adapter applies -3 dBFS last; receiver -1 dBFS remains a separate playback contract",
                "editable": False,
            },
            "adapters": adapters,
            "commands": commands,
            "faultCoverage": fault_coverage(),
            "unsupportedRoutes": [
                {
                    "platform": adapter["platform"],
                    "route": route,
                    "workflows": WORKFLOWS,
                    "reason": "native-effects-unverified" if adapter["platform"] == "windows" else "signed-vpio-not-exercised",
                    "status": "unsupported-pending-manual-c3",
                }
                for adapter in adapters for route in ROUTES
            ],
            "blockers": [
                "Windows native AEC and noise suppression remain unverified by the current helper",
                "macOS signed VPIO and physical route behavior are not exercised by a repository test adapter",
                "speaker render-reference age is not proven on either platform",
                "canonical STOI, acoustic ERLE/SNR, blinded listening, physical resources and accessibility remain manual",
            ],
            "retention": {
                "syntheticAudioRetained": False,
                "privateOrUserAudioAccepted": False,
                "retainedData": "content-free metrics, processed SHA-256, commands and categorical blockers only",
            },
            "toolchains": {
                "host": f"{platform.system()} {platform.machine()}",
                "go": subprocess.check_output(["go", "version"], text=True).strip(),
                "swift": subprocess.check_output(
                    ["xcrun", "swift", "--version"], text=True,
                    env={**os.environ, **apple_env}).splitlines()[0],
                "python": platform.python_version(),
            },
        }
        validate_evidence(evidence)
        output.write_text(json.dumps(evidence, indent=2, sort_keys=True) + "\n", encoding="utf-8")
        return evidence


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--output", type=pathlib.Path, required=True)
    args = parser.parse_args()
    evidence = run(args.output)
    print(json.dumps({
        "output": str(args.output),
        "cells": evidence["matrix"]["cells"],
        "fixtureRuns": evidence["matrix"]["fixtureRuns"],
        "manualEvidence": evidence["decision"]["manualEvidence"],
    }, sort_keys=True))
    return 0


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except (IntegratedError, OSError, ValueError, KeyError, json.JSONDecodeError) as error:
        print(f"capture-quality integrated regressions: {error}", file=sys.stderr)
        raise SystemExit(2)
