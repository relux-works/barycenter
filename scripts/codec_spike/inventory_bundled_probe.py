#!/usr/bin/env python3
"""Inventory and fail-close a built bundled codec prototype."""

from __future__ import annotations

import argparse
import hashlib
import json
import pathlib
import platform
import re
import shlex
import subprocess


ROOT = pathlib.Path(__file__).resolve().parents[2]
CONTRACT_PATH = ROOT / "acceptance" / "codec-spike" / "bundled-probe-v1.json"


def digest(path: pathlib.Path) -> str:
    value = hashlib.sha256()
    with path.open("rb") as stream:
        for chunk in iter(lambda: stream.read(1024 * 1024), b""):
            value.update(chunk)
    return value.hexdigest()


def run(*argv: str) -> str:
    return subprocess.run(argv, check=True, text=True, capture_output=True).stdout


def load_contract() -> dict:
    return json.loads(CONTRACT_PATH.read_text(encoding="utf-8"))


def validate_contract(contract: dict) -> None:
    if contract.get("contract") != "p2-bundled-ffmpeg-probe.v1":
        raise ValueError("wrong bundled probe contract")
    configure = contract.get("configure", {})
    required = set(configure.get("required", []))
    forbidden = set(configure.get("forbidden", []))
    if required & forbidden:
        raise ValueError("configure flag is both required and forbidden")
    for flag in ("--disable-programs", "--disable-network", "--disable-static",
                 "--enable-shared", "--disable-avfilter", "--disable-swscale"):
        if flag not in required:
            raise ValueError(f"missing build floor {flag}")
    if contract.get("package", {}).get("runtimeExecutableDownload") is not False:
        raise ValueError("runtime executable download enabled")
    if contract.get("package", {}).get("decoderProcessOwnsNetwork") is not False:
        raise ValueError("decoder owns network")
    if contract.get("package", {}).get("hostileInputContainment") != "probe-process-boundary-only":
        raise ValueError("hostile input containment claim changed")
    matrix = {item.get("id"): item for item in contract.get("platformMatrix", [])}
    if set(matrix) != {"macos-arm64", "windows-amd64", "windows-arm64"}:
        raise ValueError("required package architecture matrix changed")
    if not all(item.get("required") for item in matrix.values()):
        raise ValueError("required architecture became optional")
    fixtures = contract.get("smokeFixtures", [])
    if len(fixtures) != 6 or {item.get("codec") for item in fixtures} != {"mp3", "aac", "opus"}:
        raise ValueError("smoke fixture inventory changed")
    for item in fixtures:
        path = ROOT / item["path"]
        if not path.is_file() or digest(path) != item["sha256"]:
            raise ValueError(f"fixture digest mismatch: {item['id']}")
    decision = contract.get("decision", {})
    if decision.get("shipping") != "rejected-until-all-required-platform-and-release-evidence-exists":
        raise ValueError("shipping decision no longer fails closed")
    if decision.get("missingEvidenceIsRejection") is not True:
        raise ValueError("missing package evidence no longer rejects")


def inventory(stage: pathlib.Path, config: pathlib.Path, platform_id: str) -> dict:
    contract = load_contract()
    validate_contract(contract)
    configuration = config.read_text(encoding="utf-8")
    configuration_line = next(
        (line.partition("=")[2] for line in configuration.splitlines()
         if line.startswith("FFMPEG_CONFIGURATION=")),
        "",
    )
    configured_flags = set(shlex.split(configuration_line))
    for flag in contract["configure"]["required"]:
        if flag not in configured_flags:
            raise ValueError(f"configure receipt missing {flag}")
    for flag in contract["configure"]["forbidden"]:
        if flag in configured_flags:
            raise ValueError(f"configure receipt contains forbidden {flag}")

    files = []
    forbidden_names = contract["package"]["forbiddenNames"]
    allowed_library_fragments = contract["package"]["allowedLibraries"]
    for path in sorted(item for item in stage.rglob("*") if item.is_file()):
        relative = path.relative_to(stage).as_posix()
        lower = path.name.lower()
        if any(re.fullmatch(name + r"(?:\.exe)?", lower) for name in forbidden_names):
            raise ValueError(f"forbidden FFmpeg program staged: {relative}")
        record = {"path": relative, "bytes": path.stat().st_size, "sha256": digest(path)}
        if path.suffix == ".dylib":
            if not any(fragment in lower for fragment in allowed_library_fragments):
                raise ValueError(f"unexpected library staged: {relative}")
            record["imports"] = [
                line.strip().split(" ", 1)[0]
                for line in run("otool", "-L", str(path)).splitlines()[1:]
                if line.strip()
            ]
            signature = subprocess.run(
                ["codesign", "--verify", "--strict", "--verbose=2", str(path)],
                text=True, capture_output=True,
            )
            if signature.returncode != 0:
                raise ValueError(f"unsigned nested library: {relative}")
            record["engineeringSignature"] = "ad-hoc"
        files.append(record)

    dylibs = [item for item in files if item["path"].endswith(".dylib")]
    expected_fragments = {"libavformat.", "libavcodec.", "libavutil.", "libswresample.",
                          "libpulsar_codec_bridge.dylib"}
    if {next(fragment for fragment in expected_fragments if fragment in pathlib.Path(item["path"]).name)
        for item in dylibs} != expected_fragments:
        raise ValueError("staged library allowlist incomplete")
    return {
        "schemaVersion": 1,
        "contract": contract["contract"],
        "candidateId": contract["candidateId"],
        "platform": platform_id,
        "host": {"system": platform.system(), "machine": platform.machine()},
        "sourceSha256": contract["source"]["sha256"],
        "configurationSha256": hashlib.sha256(configuration.encode()).hexdigest(),
        "packageBytes": sum(item["bytes"] for item in files),
        "files": files,
        "runtimeExecutableDownload": False,
        "decoderProcessOwnsNetwork": False,
        "releaseSignature": "not-proven",
        "notarization": "not-proven",
        "shippingDecision": "rejected-until-all-required-platform-and-release-evidence-exists",
        "claimClass": "repository-engineering-prototype",
    }


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--stage", type=pathlib.Path, required=True)
    parser.add_argument("--config", type=pathlib.Path, required=True)
    parser.add_argument("--platform", required=True)
    parser.add_argument("--output", type=pathlib.Path, required=True)
    args = parser.parse_args()
    result = inventory(args.stage.resolve(), args.config.resolve(), args.platform)
    args.output.parent.mkdir(parents=True, exist_ok=True)
    args.output.write_text(json.dumps(result, indent=2) + "\n", encoding="utf-8")
    print(args.output)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
