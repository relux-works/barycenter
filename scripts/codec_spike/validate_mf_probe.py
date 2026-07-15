#!/usr/bin/env python3
"""Validate the fail-closed Media Foundation AppContainer probe contract."""

from __future__ import annotations

import json
import pathlib


ROOT = pathlib.Path(__file__).resolve().parents[2]
CONTRACT_PATH = ROOT / "acceptance" / "codec-spike" / "media-foundation-probe-v1.json"


def load() -> dict:
    return json.loads(CONTRACT_PATH.read_text(encoding="utf-8"))


def validate(contract: dict) -> None:
    if contract.get("contract") != "p2-media-foundation-appcontainer-probe.v1":
        raise ValueError("wrong Media Foundation probe contract")
    baseline = contract.get("distributionBaseline", {})
    if baseline.get("trustLevel") != "appContainer":
        raise ValueError("probe no longer requires AppContainer")
    if baseline.get("runFullTrust") is not False:
        raise ValueError("runFullTrust is forbidden")
    if baseline.get("developerMode") is not False:
        raise ValueError("developer mode is forbidden")
    if baseline.get("capabilities") != []:
        raise ValueError("offline decoder probe must not declare capabilities")
    adapter = contract.get("adapter", {})
    if adapter.get("decoderOwnsNetwork") is not False:
        raise ValueError("decoder must not own network")
    if adapter.get("renderCallbackUsed") is not False:
        raise ValueError("render callback work is forbidden")
    if adapter.get("maximumPreparedReadBytes") != 1048576:
        raise ValueError("prepared read ceiling changed")
    matrix = {item.get("id"): item for item in contract.get("platformMatrix", [])}
    if set(matrix) != {"windows-amd64", "windows-arm64"}:
        raise ValueError("Windows architecture matrix changed")
    if not all(item.get("required") is True for item in matrix.values()):
        raise ValueError("required architecture became optional")
    fixtures = contract.get("smokeFixtures", [])
    if len(fixtures) != 6:
        raise ValueError("exact six-fixture corpus required")
    expected = {item["id"]: item["expected"] for item in fixtures}
    if any(expected[item] != "decode" for item in (
            "mp3_cbr_12s", "mp3_vbr_12s", "aac_m4a_12s", "aac_adts_12s")):
        raise ValueError("MP3/AAC decode expectation changed")
    if any(expected[item] != "reject-with-hresult" for item in (
            "opus_ogg_cbr_12s", "opus_ogg_vbr_12s")):
        raise ValueError("Ogg/Opus must fail closed without runtime proof")
    for item in fixtures:
        if not (ROOT / item["path"]).is_file():
            raise ValueError(f"fixture missing: {item['id']}")
    manual = contract.get("manualLimits", {})
    if manual.get("soakSeconds") != 7200 or manual.get("manualEpic") != "EPIC-260714-th54l3":
        raise ValueError("two-hour manual evidence boundary changed")
    decision = contract.get("decision", {})
    if decision.get("shipping") != "rejected-until-opus-and-manual-platform-evidence-exists":
        raise ValueError("shipping decision no longer fails closed")
    if decision.get("missingEvidenceIsRejection") is not True:
        raise ValueError("missing evidence no longer rejects")


def main() -> int:
    validate(load())
    print("Media Foundation AppContainer probe contract is valid")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
