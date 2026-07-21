#!/usr/bin/env python3
"""Run deterministic desktop UI gates without mutating frozen phase packets."""

from __future__ import annotations

import argparse
import datetime as dt
import json
import os
import pathlib
import platform
import re
import stat
import subprocess
import sys

sys.dont_write_bytecode = True
import run_automated as base


SOURCE_PATHS = (
    "node-app/Package.swift",
    "node-app/Package.resolved",
    "node-app/Sources/NodeAppUI/PulsarDesktopStyle.swift",
    "node-app/Sources/NodeAppUI/PulsarMainWindow.swift",
    "node-app/Tests/NodeAppUITests/PulsarDesktopStyleTests.swift",
    "pulsar-win/main.go",
    "pulsar-win/main_window_windows.go",
    "pulsar-win/main_theme_windows.go",
    "pulsar-win/windows_visual_contract.go",
    "pulsar-win/windows_visual_contract_test.go",
    "pulsar-win/winres/winres.json",
    "pulsar-win/probe-msix/AppxManifest.xml.in",
    "pulsar-win/probe-msix/build-probe.ps1",
    "pulsar-win/probe-msix/register-hidden-interactive-task.ps1",
    "scripts/build-app.sh",
)


def commands(
    run_dir: pathlib.Path,
    go_env: dict[str, str],
    apple_env: dict[str, str],
) -> list[base.Command]:
    windows = [
        command
        for command in base.suite_commands("windows", go_env, None)
        if command.name.startswith("windows-")
    ]
    artifacts = run_dir / "build"
    return [
        base.Command(
            "desktop-contract-tests",
            base.ROOT,
            ("python3", "-m", "unittest", "scripts/acceptance/test_desktop_ui.py"),
            {"PYTHONDONTWRITEBYTECODE": "1"},
        ),
        *windows,
        base.Command(
            "windows-gui-build-amd64",
            base.ROOT / "pulsar-win",
            (
                "go", "build", "-trimpath", "-ldflags", "-H=windowsgui",
                "-o", str(artifacts / "Pulsar-windows-amd64.exe"), ".",
            ),
            {**go_env, "CGO_ENABLED": "0", "GOOS": "windows", "GOARCH": "amd64"},
        ),
        base.Command(
            "windows-gui-build-arm64",
            base.ROOT / "pulsar-win",
            (
                "go", "build", "-trimpath", "-ldflags", "-H=windowsgui",
                "-o", str(artifacts / "Pulsar-windows-arm64.exe"), ".",
            ),
            {**go_env, "CGO_ENABLED": "0", "GOOS": "windows", "GOARCH": "arm64"},
        ),
        base.Command("swift-tests", base.ROOT / "node-app", ("xcrun", "swift", "test"), apple_env),
        base.Command(
            "swift-release-build",
            base.ROOT / "node-app",
            ("xcrun", "swift", "build", "-c", "release"),
            apple_env,
        ),
    ]


def safe_run_id(value: str | None) -> str:
    result = value or dt.datetime.now(dt.timezone.utc).strftime("desktop-ui-%Y%m%dT%H%M%SZ")
    if not re.fullmatch(r"[A-Za-z0-9][A-Za-z0-9._-]{0,79}", result):
        raise ValueError("run id must be 1-80 safe filename characters")
    return result


def source_records() -> list[dict]:
    records = []
    for relative in SOURCE_PATHS:
        path = base.ROOT / relative
        if not path.is_file() or path.is_symlink():
            raise RuntimeError(f"required desktop source is missing or symlinked: {relative}")
        records.append({"path": relative, "bytes": path.stat().st_size, "sha256": base.sha256(path)})
    return records


def pe_subsystem(path: pathlib.Path) -> int:
    data = path.read_bytes()
    if data[:2] != b"MZ" or len(data) < 0x40:
        raise RuntimeError(f"not a PE image: {path.name}")
    pe_offset = int.from_bytes(data[0x3C:0x40], "little")
    if data[pe_offset:pe_offset + 4] != b"PE\0\0":
        raise RuntimeError(f"missing PE signature: {path.name}")
    optional = pe_offset + 24
    subsystem = int.from_bytes(data[optional + 68:optional + 70], "little")
    if subsystem != 2:
        raise RuntimeError(f"{path.name} subsystem is {subsystem}, expected GUI (2)")
    return subsystem


def run(args: argparse.Namespace) -> int:
    start_status = base.git("status", "--porcelain")
    if args.require_clean and start_status:
        raise RuntimeError("--require-clean rejected a dirty worktree")
    pins = base.load_pins()
    go_env, go_version = base.verify_go(pins)
    apple_env, apple_version = base.resolve_apple_toolchain(pins)
    run_dir = base.DEFAULT_ARTIFACT_ROOT / safe_run_id(args.run_id)
    base.ensure_safe_artifact_dir(run_dir)
    (run_dir / "build").mkdir(mode=0o700)

    records = []
    status = "pass"
    started = base.utc_now()
    for index, command in enumerate(commands(run_dir, go_env, apple_env), start=1):
        log_path = run_dir / f"{index:02d}-{command.name}.log"
        merged = os.environ.copy()
        if command.env:
            merged.update(command.env)
        print(f"[{index}] {command.name}", flush=True)
        completed = subprocess.run(
            command.argv,
            cwd=command.cwd,
            env=merged,
            stdout=subprocess.PIPE,
            stderr=subprocess.STDOUT,
            text=True,
            errors="replace",
        )
        log_path.write_text(base.sanitize(completed.stdout), encoding="utf-8")
        log_path.chmod(stat.S_IRUSR | stat.S_IWUSR)
        records.append({
            "name": command.name,
            "cwd": command.cwd.relative_to(base.ROOT).as_posix(),
            "argv": [base.sanitize(value) for value in command.argv],
            "exitCode": completed.returncode,
            "log": log_path.name,
        })
        if completed.returncode:
            status = "fail"
            break

    build_artifacts = []
    if status == "pass":
        for path in sorted((run_dir / "build").glob("*.exe")):
            build_artifacts.append({**base.artifact_record(path, run_dir), "peSubsystem": pe_subsystem(path)})
        mac_binary = base.ROOT / "node-app/.build/release/NodeApp"
        if not mac_binary.is_file():
            raise RuntimeError("Swift release build did not produce NodeApp")
        build_artifacts.append({
            "path": "node-app/.build/release/NodeApp",
            "bytes": mac_binary.stat().st_size,
            "sha256": base.sha256(mac_binary),
            "kind": "unbundled-release-binary",
        })

    end_status = base.git("status", "--porcelain")
    if args.require_clean and end_status:
        status = "fail"
    manifest = {
        "schemaVersion": 1,
        "scope": "desktop-ui-repository-automated-only",
        "manualEvidence": "not-run",
        "status": status,
        "startedAt": started,
        "finishedAt": base.utc_now(),
        "git": {
            "head": base.git("rev-parse", "HEAD"),
            "tree": base.git("rev-parse", "HEAD^{tree}"),
            "startDirty": bool(start_status),
            "endDirty": bool(end_status),
            "endDirtyPaths": [base.sanitize(line) for line in end_status.splitlines()],
        },
        "host": {"system": platform.system(), "release": platform.release(), "machine": platform.machine()},
        "toolchains": {"go": go_version, "apple": apple_version},
        "commands": records,
        "sourceArtifacts": source_records(),
        "buildArtifacts": build_artifacts,
        "logs": [base.artifact_record(path, run_dir) for path in sorted(run_dir.glob("*.log"))],
        "limitations": [
            "No physical DPI, Retina, Narrator, VoiceOver, microphone, speaker or audible result is claimed.",
            "The macOS binary is not a notarized application bundle; release signing remains a release-workflow gate.",
            "Windows signed-package evidence is supplied separately by the exact-head hosted CI artifact.",
        ],
    }
    manifest_path = run_dir / "manifest.json"
    manifest_path.write_text(json.dumps(manifest, indent=2, sort_keys=True) + "\n", encoding="utf-8")
    manifest_path.chmod(stat.S_IRUSR | stat.S_IWUSR)
    print(f"manifest: {manifest_path}")
    return 0 if status == "pass" else 1


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser()
    parser.add_argument("--run-id")
    parser.add_argument("--require-clean", action="store_true")
    return parser.parse_args()


if __name__ == "__main__":
    try:
        raise SystemExit(run(parse_args()))
    except (OSError, ValueError, RuntimeError, subprocess.CalledProcessError) as error:
        print(f"desktop UI acceptance: {base.sanitize(str(error))}", file=os.sys.stderr)
        raise SystemExit(2)
