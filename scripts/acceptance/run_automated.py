#!/usr/bin/env python3
"""Run reproducible Phase 1 repository acceptance suites.

This runner deliberately covers repository/CI evidence only. It cannot mark
physical Windows, audible output, WACK, Partner Center, or screenshots passed.
"""

from __future__ import annotations

import argparse
import datetime as dt
import hashlib
import json
import os
import pathlib
import platform
import re
import stat
import subprocess
import sys
from dataclasses import dataclass
from typing import Iterable


ROOT = pathlib.Path(__file__).resolve().parents[2]
PINS_PATH = ROOT / "acceptance" / "toolchains.json"
DEFAULT_ARTIFACT_ROOT = ROOT / ".temp" / "acceptance"
PREVIOUS_HEAD_PATTERN = (
    "^(TestR8ExactPreviousHEAD(AuthorityRoundTrip|TwoGenerationProjectionComposition|"
    "ConfigBootstrapContract)|TestMediaIngestExactPreviousHeadRollback|"
    "TestMediaUploadExactPreviousHeadRollback|TestMediaProcessingExactPreviousHeadRollback|"
    "TestMediaLifecycleExactPreviousHeadRollback|TestMediaIntegrationExactPreviousHeadRollback|"
    "TestTransmissionStoreExactPreviousHeadRollback|TestModerationExactPreviousHeadRollback|"
    "TestAutomationExactPreviousHeadRollback|"
    "TestAirExactPreviousCoordinatorLegacyServicePreservesPhase2Rows)$"
)

SECRET_PATTERNS = (
    re.compile(r"(?i)(authorization\s*[:=]\s*bearer\s+)[^\s,;]+"),
    re.compile(r"(?i)((?:access|refresh|recovery|playback|node)?_?token\s*[:=]\s*)[^\s,;]+"),
    re.compile(r"(?i)((?:password|secret|credential)\s*[:=]\s*)[^\s,;]+"),
)


@dataclass(frozen=True)
class Command:
    name: str
    cwd: pathlib.Path
    argv: tuple[str, ...]
    env: dict[str, str] | None = None


def utc_now() -> str:
    return dt.datetime.now(dt.timezone.utc).isoformat(timespec="seconds").replace("+00:00", "Z")


def load_pins() -> dict:
    with PINS_PATH.open(encoding="utf-8") as stream:
        return json.load(stream)


def sanitize(text: str) -> str:
    replacements = {
        str(ROOT): "<repo>",
        str(pathlib.Path.home()): "<home>",
    }
    result = text
    for source, target in sorted(replacements.items(), key=lambda item: -len(item[0])):
        if source:
            result = result.replace(source, target)
    for pattern in SECRET_PATTERNS:
        result = pattern.sub(r"\1<redacted>", result)
    return result


def sha256(path: pathlib.Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as stream:
        for block in iter(lambda: stream.read(1024 * 1024), b""):
            digest.update(block)
    return digest.hexdigest()


def git(*args: str) -> str:
    return subprocess.check_output(("git", *args), cwd=ROOT, text=True).strip()


def ensure_safe_artifact_dir(path: pathlib.Path) -> None:
    for parent in (ROOT / ".temp", DEFAULT_ARTIFACT_ROOT):
        if parent.is_symlink():
            raise ValueError(f"artifact parent may not be a symlink: {parent}")
    resolved_root = DEFAULT_ARTIFACT_ROOT.resolve()
    resolved = path.resolve()
    if resolved_root != resolved and resolved_root not in resolved.parents:
        raise ValueError(f"artifact directory must be beneath {resolved_root}")
    if path.exists() and path.is_symlink():
        raise ValueError("artifact directory may not be a symlink")
    path.mkdir(parents=True, mode=0o700, exist_ok=False)
    path.chmod(stat.S_IRWXU)


def tool_output(argv: Iterable[str], env: dict[str, str] | None = None) -> str:
    merged = os.environ.copy()
    if env:
        merged.update(env)
    return subprocess.check_output(tuple(argv), cwd=ROOT, env=merged, text=True, stderr=subprocess.STDOUT).strip()


def resolve_apple_toolchain(pins: dict) -> tuple[dict[str, str], dict[str, str]]:
    expected = pins["apple"]
    candidates = []
    explicit = os.environ.get("DEVELOPER_DIR")
    if explicit:
        candidates.append(pathlib.Path(explicit))
    candidates.extend(
        (
            pathlib.Path(f"/Applications/Xcode_{expected['xcodeVersion']}.app/Contents/Developer"),
            pathlib.Path("/Applications/Xcode.app/Contents/Developer"),
        )
    )
    diagnostics: list[str] = []
    for candidate in dict.fromkeys(candidates):
        if not candidate.exists():
            diagnostics.append(f"{candidate}: missing")
            continue
        env = {"DEVELOPER_DIR": str(candidate)}
        try:
            xcode = tool_output(("xcodebuild", "-version"), env)
            swift = tool_output(("xcrun", "swift", "--version"), env)
        except (OSError, subprocess.CalledProcessError) as error:
            diagnostics.append(f"{candidate}: {error}")
            continue
        version_match = re.search(r"Xcode\s+(\S+)", xcode)
        build_match = re.search(r"Build version\s+(\S+)", xcode)
        swift_match = re.search(r"Swift version\s+(\S+)", swift)
        actual = {
            "developerDir": str(candidate),
            "xcodeVersion": version_match.group(1) if version_match else "unknown",
            "xcodeBuild": build_match.group(1) if build_match else "unknown",
            "swiftVersion": swift_match.group(1) if swift_match else "unknown",
        }
        if all(actual[key] == expected[key] for key in ("xcodeVersion", "xcodeBuild", "swiftVersion")):
            return env, actual
        diagnostics.append(f"{candidate}: {actual}")
    raise RuntimeError("pinned full Xcode toolchain unavailable; " + "; ".join(diagnostics))


def verify_go(pins: dict) -> tuple[dict[str, str], str]:
    toolchain = pins["go"]["toolchain"]
    env = {"GOTOOLCHAIN": toolchain}
    raw = tool_output(("go", "version"), env)
    actual = next((line for line in reversed(raw.splitlines()) if line.startswith("go version ")), raw)
    if f"go version {toolchain} " not in actual:
        raise RuntimeError(f"Go toolchain mismatch: expected {toolchain}, got {actual}")
    for module in (
        ROOT / "coordinator" / "go.mod",
        ROOT / "pulsar-win" / "go.mod",
        ROOT / "scripts" / "e2ee_container" / "probe" / "go.mod",
    ):
        match = re.search(r"^go\s+(\S+)$", module.read_text(encoding="utf-8"), re.MULTILINE)
        if not match or match.group(1) != pins["go"]["version"]:
            raise RuntimeError(f"{module.relative_to(ROOT)} does not pin Go {pins['go']['version']}")
    return env, actual


def suite_commands(suite: str, go_env: dict[str, str] | None, apple_env: dict[str, str] | None) -> list[Command]:
    go_env = go_env or {}
    contract = [
        Command(
            "acceptance-contract-tests",
            ROOT,
            (
                "python3", "-m", "unittest",
                "scripts/acceptance/test_acceptance.py",
                "scripts/acceptance/test_p1_protocol_review_handoff.py",
                "scripts/acceptance/test_targets_inbox_parity.py",
                "scripts/acceptance/test_targets_inbox_rollout.py",
                "scripts/acceptance/test_streamed_track_rollout.py",
                "scripts/acceptance/test_phase2_gate_matrix.py",
                "scripts/acceptance/test_stream_performance_review.py",
                "scripts/acceptance/test_air_migration_review.py",
                "scripts/acceptance/test_target_security_review.py",
                "scripts/acceptance/test_phase2_observability.py",
                "scripts/acceptance/test_p2_root_review.py",
                "scripts/acceptance/test_phase2_engineering_handoff.py",
                "scripts/acceptance/test_automation_safety_handoff.py",
                "scripts/acceptance/test_e2ee_threat_model.py",
                "scripts/acceptance/test_protected_media_container_spike.py",
                "scripts/acceptance/test_group_crypto_library_spike.py",
                "scripts/acceptance/test_e2ee_protocol_key_lifecycle.py",
                "scripts/acceptance/test_e2ee_schema_epoch_foundation.py",
                "scripts/acceptance/test_e2ee_coordinator_routing_rotation.py",
                "scripts/acceptance/test_e2ee_opaque_media_router.py",
                "scripts/acceptance/test_macos_e2ee_key_state.py",
                "scripts/acceptance/test_windows_e2ee_key_state.py",
                "scripts/acceptance/test_capture_quality_contract.py",
                "scripts/acceptance/test_phase3_gate_matrix.py",
                "scripts/acceptance/test_phase3_observability.py",
                "scripts/acceptance/test_p3_root_review.py",
                "scripts/acceptance/test_p3_realtime_pre_review.py",
                "scripts/acceptance/test_p3_automation_pre_review.py",
                "scripts/acceptance/test_p3_privacy_store_pre_review.py",
                "scripts/acceptance/test_p3_migration_recovery_pre_review.py",
                "scripts/acceptance/test_phase3_engineering_handoff.py",
                "scripts/acceptance/test_p3_final_engineering_audit.py",
                "scripts/capture_quality/test_harness.py",
                "scripts/capture_quality/test_integrated.py",
                "scripts/live_ptt/test_transport_model.py",
                "scripts/live_ptt/test_codec_transport.py",
                "scripts/codec_spike/test_codec_spike.py",
                "scripts/codec_spike/test_independent_supply_review.py",
            ),
            {"PYTHONDONTWRITEBYTECODE": "1"},
        )
    ]
    container_probe = [
        Command(
            "protected-media-container-probe-tests",
            ROOT / "scripts/e2ee_container/probe",
            ("go", "test", "./..."),
            go_env,
        ),
        Command(
            "protected-media-container-probe-race",
            ROOT / "scripts/e2ee_container/probe",
            ("go", "test", "-race", "./..."),
            go_env,
        ),
    ]
    container_probe_windows = [
        Command(
            "protected-media-container-probe-windows-amd64",
            ROOT / "scripts/e2ee_container/probe",
            ("go", "build", "./..."),
            {**go_env, "CGO_ENABLED": "0", "GOOS": "windows", "GOARCH": "amd64"},
        ),
        Command(
            "protected-media-container-probe-windows-arm64",
            ROOT / "scripts/e2ee_container/probe",
            ("go", "build", "./..."),
            {**go_env, "CGO_ENABLED": "0", "GOOS": "windows", "GOARCH": "arm64"},
        ),
    ]
    coordinator = [
        Command("coordinator-vet", ROOT / "coordinator", ("go", "vet", "./..."), go_env),
        Command("coordinator-tests", ROOT / "coordinator", ("go", "test", "./..."), go_env),
        Command("moderation-contract", ROOT / "coordinator", ("go", "run", "./cmd/moderation-ops-check"), go_env),
        Command(
            "previous-head-rollback",
            ROOT / "coordinator",
            ("go", "test", "-tags", "previoushead", "-count=1", "./internal/store", "-run", PREVIOUS_HEAD_PATTERN),
            go_env,
        ),
    ]
    windows = [
        Command("windows-vet", ROOT / "pulsar-win", ("go", "vet", "./..."), go_env),
        Command("windows-tests", ROOT / "pulsar-win", ("go", "test", "./..."), go_env),
        Command("windows-race", ROOT / "pulsar-win", ("go", "test", "-race", "./..."), go_env),
        Command(
            "windows-cross-vet-amd64",
            ROOT / "pulsar-win",
            ("go", "vet", "./..."),
            {**go_env, "CGO_ENABLED": "0", "GOOS": "windows", "GOARCH": "amd64"},
        ),
        Command(
            "windows-cross-build-amd64",
            ROOT / "pulsar-win",
            ("go", "build", "./..."),
            {**go_env, "CGO_ENABLED": "0", "GOOS": "windows", "GOARCH": "amd64"},
        ),
        Command(
            "windows-cross-build-arm64",
            ROOT / "pulsar-win",
            ("go", "build", "./..."),
            {**go_env, "CGO_ENABLED": "0", "GOOS": "windows", "GOARCH": "arm64"},
        ),
    ]
    swift = []
    if apple_env is not None:
        swift.append(Command("swift-tests", ROOT / "node-app", ("xcrun", "swift", "test"), apple_env))
    return {
        "coordinator": contract + container_probe + coordinator,
        "windows": contract + container_probe + container_probe_windows + windows,
        "swift": contract + swift,
        "all": contract + container_probe + container_probe_windows + coordinator + windows + swift,
    }[suite]


def artifact_record(path: pathlib.Path, run_dir: pathlib.Path) -> dict:
    return {
        "path": path.relative_to(run_dir).as_posix(),
        "bytes": path.stat().st_size,
        "sha256": sha256(path),
    }


def run(args: argparse.Namespace) -> int:
    pins = load_pins()
    start_status = git("status", "--porcelain")
    if args.require_clean and start_status:
        raise RuntimeError("--require-clean rejected a dirty worktree")
    go_env: dict[str, str] | None = None
    go_version: str | None = None
    if args.suite in ("coordinator", "windows", "all"):
        go_env, go_version = verify_go(pins)
    apple_env: dict[str, str] | None = None
    apple_version: dict[str, str] | None = None
    if args.suite in ("swift", "all"):
        apple_env, apple_version = resolve_apple_toolchain(pins)

    run_id = args.run_id or dt.datetime.now(dt.timezone.utc).strftime("%Y%m%dT%H%M%SZ")
    if not re.fullmatch(r"[A-Za-z0-9][A-Za-z0-9._-]{0,79}", run_id):
        raise ValueError("run id must be 1-80 safe filename characters")
    run_dir = DEFAULT_ARTIFACT_ROOT / run_id
    ensure_safe_artifact_dir(run_dir)
    commands = suite_commands(args.suite, go_env, apple_env)
    started = utc_now()
    records: list[dict] = []
    status = "pass"
    for index, command in enumerate(commands, start=1):
        log_path = run_dir / f"{index:02d}-{command.name}.log"
        merged = os.environ.copy()
        if command.env:
            merged.update(command.env)
        print(f"[{index}/{len(commands)}] {command.name}", flush=True)
        completed = subprocess.run(
            command.argv,
            cwd=command.cwd,
            env=merged,
            stdout=subprocess.PIPE,
            stderr=subprocess.STDOUT,
            text=True,
            errors="replace",
        )
        log_path.write_text(sanitize(completed.stdout), encoding="utf-8")
        log_path.chmod(stat.S_IRUSR | stat.S_IWUSR)
        records.append(
            {
                "name": command.name,
                "cwd": command.cwd.relative_to(ROOT).as_posix(),
                "argv": list(command.argv),
                "exitCode": completed.returncode,
                "log": log_path.name,
            }
        )
        if completed.returncode:
            status = "fail"
            break

    manifest_path = run_dir / "manifest.json"
    end_status = git("status", "--porcelain")
    if args.require_clean and end_status:
        status = "fail"
    manifest = {
        "schemaVersion": 1,
        "scope": "repository-automated-only",
        "manualEvidence": "not-run",
        "suite": args.suite,
        "status": status,
        "startedAt": started,
        "finishedAt": utc_now(),
        "git": {
            "head": git("rev-parse", "HEAD"),
            "dirty": bool(end_status),
            "startDirty": bool(start_status),
            "endDirty": bool(end_status),
            "endDirtyPaths": [sanitize(line) for line in end_status.splitlines()],
        },
        "host": {"system": platform.system(), "release": platform.release(), "machine": platform.machine()},
        "toolchains": {"go": go_version, "apple": apple_version},
        "commands": records,
        "artifacts": [artifact_record(path, run_dir) for path in sorted(run_dir.glob("*.log"))],
    }
    manifest_path.write_text(json.dumps(manifest, indent=2, sort_keys=True) + "\n", encoding="utf-8")
    manifest_path.chmod(stat.S_IRUSR | stat.S_IWUSR)
    print(f"manifest: {manifest_path}")
    return 0 if status == "pass" else 1


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser()
    parser.add_argument("--suite", choices=("coordinator", "windows", "swift", "all"), default="all")
    parser.add_argument("--run-id")
    parser.add_argument("--require-clean", action="store_true")
    return parser.parse_args()


if __name__ == "__main__":
    try:
        raise SystemExit(run(parse_args()))
    except (OSError, ValueError, RuntimeError, subprocess.CalledProcessError) as error:
        print(f"acceptance harness: {sanitize(str(error))}", file=sys.stderr)
        raise SystemExit(2)
