#!/usr/bin/env python3
"""Validate the pure-Go research probe without relaxing its rejected status."""

from __future__ import annotations

import argparse
import hashlib
import json
import os
import subprocess
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
CONTRACT_PATH = ROOT / "acceptance/codec-spike/pure-go-probe-v1.json"
MODULE_DIR = ROOT / "scripts/codec_spike/purego_probe"
FIXTURE_IDS = [
    "mp3_cbr_12s", "mp3_vbr_12s", "aac_m4a_12s", "aac_adts_12s",
    "opus_ogg_cbr_12s", "opus_ogg_vbr_12s",
]


def load(path: Path) -> dict:
    return json.loads(path.read_text(encoding="utf-8"))


def digest(path: Path) -> str:
    value = hashlib.sha256()
    with path.open("rb") as handle:
        for chunk in iter(lambda: handle.read(1024 * 1024), b""):
            value.update(chunk)
    return value.hexdigest()


def validate_contract(contract: dict) -> None:
    assert contract["contract"] == "p2-pure-go-streaming-decoder-probe.v1"
    assert contract["candidateId"] == "pure-go-composite-v1"
    assert contract["classification"] == "rejected"
    assert contract["manualEpic"] == "EPIC-260714-th54l3"
    assert [item["id"] for item in contract["fixtures"]] == FIXTURE_IDS
    modules = {item["path"]: item for item in contract["modules"]}
    assert modules["github.com/hajimehoshi/go-mp3"]["version"] == "v0.3.4"
    assert modules["github.com/pion/opus"]["version"] == "v0.1.0"
    assert modules["github.com/llehouerou/go-aac"]["use"] == "forbidden-not-in-go-mod"
    assert contract["bounds"] == {
        "maximumUnderlyingReadBytes": 65_536,
        "pcmRingBytes": 1_048_576,
        "decoderOwnsNetwork": False,
        "renderThreadIO": False,
        "cgoAllowed": False,
        "fullFileAllocationAllowed": False,
    }
    assert len(contract["shippingRejectionReasons"]) == 4


def validate_module() -> list[str]:
    go_mod = (MODULE_DIR / "go.mod").read_text(encoding="utf-8")
    go_sum = (MODULE_DIR / "go.sum").read_text(encoding="utf-8")
    assert "github.com/hajimehoshi/go-mp3 v0.3.4" in go_mod
    assert "github.com/pion/opus v0.1.0" in go_mod
    assert "go-aac" not in go_mod + go_sum
    env = {**os.environ, "CGO_ENABLED": "0"}
    completed = subprocess.run(
        ["go", "list", "-deps", "-f", "{{if or .CgoFiles .CFiles .CXXFiles}}{{.ImportPath}}{{end}}", "./..."],
        cwd=MODULE_DIR, env=env, check=True, text=True, capture_output=True,
    )
    assert not completed.stdout.strip(), completed.stdout
    graph = subprocess.run(
        ["go", "list", "-m", "all"], cwd=MODULE_DIR, env=env,
        check=True, text=True, capture_output=True,
    ).stdout.splitlines()
    assert not any("go-aac" in item for item in graph)
    runtime_lines = subprocess.run(
        ["go", "list", "-deps", "-f", "{{with .Module}}{{.Path}} {{.Version}}{{end}}", "./cmd/purego-probe"],
        cwd=MODULE_DIR, env=env, check=True, text=True, capture_output=True,
    ).stdout.splitlines()
    modules = sorted({item.strip() for item in runtime_lines if item.strip()})
    assert modules == sorted([
        "live.barycenter/purego-codec-probe",
        "github.com/hajimehoshi/go-mp3 v0.3.4",
        "github.com/pion/opus v0.1.0",
    ])
    return modules


def validate_evidence(evidence: dict) -> None:
    assert evidence["contract"] == "p2-pure-go-streaming-decoder-probe.v1"
    assert evidence["candidateId"] == "pure-go-composite-v1"
    assert evidence["claimClass"] == "bounded-nondistributable-research"
    assert evidence["cgoEnabled"] is False
    assert evidence["decoderOwnsNetwork"] is False
    assert evidence["renderThreadIO"] is False
    assert evidence["passed"] is False
    assert evidence["shippingDecision"] == "rejected-license-seek-and-manual-evidence-gates"
    assert [item["id"] for item in evidence["fixtures"]] == FIXTURE_IDS
    for item in evidence["fixtures"]:
        assert item["sourceBytes"] > 0
        assert item["reads"]["maximumRead"] <= 65_536
        assert item["ringMaximumUsedBytes"] <= item["ringCapacityBytes"]
        if item["codec"] == "mp3":
            assert item["outcome"] == "decode-rejected-seek-full-scan"
            assert item["incrementalFirstPCM"] is True
            assert item["seekSupported"] is True
            assert item["seekRequiresFullScan"] is True
            assert item["pausedWithoutRead"] and item["resumed"] and item["drained"] and item["cancelled"]
        elif item["codec"] == "opus":
            assert item["outcome"] == "decode-rejected-no-random-seek"
            assert item["incrementalFirstPCM"] is True
            assert item["seekSupported"] is False
            assert item["pausedWithoutRead"] and item["resumed"] and item["drained"] and item["cancelled"]
        else:
            assert item["outcome"] == "reject-forbidden-module"
            assert "GPL-2.0-only" in item["rejectReason"]


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--evidence", type=Path)
    parser.add_argument("--binary", type=Path)
    parser.add_argument("--receipt", type=Path)
    parser.add_argument("--cross-directory", type=Path)
    args = parser.parse_args()
    contract = load(CONTRACT_PATH)
    validate_contract(contract)
    modules = validate_module()
    if args.evidence is None:
        print("pure-Go probe contract and module graph valid")
        return 0
    evidence = load(args.evidence)
    validate_evidence(evidence)
    if args.binary is None or args.receipt is None:
        raise SystemExit("--binary and --receipt are required with --evidence")
    build_info = subprocess.run(
        ["go", "version", "-m", str(args.binary)], check=True, text=True, capture_output=True,
    ).stdout
    assert "CGO_ENABLED=0" in build_info
    assert "github.com/hajimehoshi/go-mp3\tv0.3.4" in build_info
    assert "github.com/pion/opus\tv0.1.0" in build_info
    assert "go-aac" not in build_info
    cross_builds = []
    if args.cross_directory is not None:
        expected = ["purego-probe-darwin-arm64", "purego-probe-windows-amd64.exe", "purego-probe-windows-arm64.exe"]
        assert sorted(path.name for path in args.cross_directory.iterdir() if path.is_file()) == expected
        for name in expected:
            path = args.cross_directory / name
            info = subprocess.run(
                ["go", "version", "-m", str(path)], check=True, text=True, capture_output=True,
            ).stdout
            assert "CGO_ENABLED=0" in info and "go-aac" not in info
            cross_builds.append({"name": name, "bytes": path.stat().st_size, "sha256": digest(path)})
    receipt = {
        "schemaVersion": 1,
        "contract": contract["contract"],
        "classification": "rejected",
        "goos": evidence["goos"],
        "goarch": evidence["goarch"],
        "binaryBytes": args.binary.stat().st_size,
        "binarySha256": digest(args.binary),
        "evidenceSha256": digest(args.evidence),
        "modules": modules,
        "cgoEnabled": False,
        "productionArtifact": False,
        "manualEvidence": "EPIC-260714-th54l3",
        "crossBuilds": cross_builds,
    }
    args.receipt.write_text(json.dumps(receipt, indent=2, sort_keys=True) + "\n", encoding="utf-8")
    print(json.dumps({"shippingDecision": evidence["shippingDecision"], "goos": evidence["goos"], "goarch": evidence["goarch"]}))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
