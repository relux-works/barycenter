#!/usr/bin/env python3
"""Validate repository-only macOS/Windows E2EE fixture parity."""

from __future__ import annotations

import json
import pathlib


ROOT = pathlib.Path(__file__).resolve().parents[2]


class CrossPlatformParityError(ValueError):
    pass


def require(condition: bool, message: str) -> None:
    if not condition:
        raise CrossPlatformParityError(message)


def load(relative: str) -> dict:
    return json.loads((ROOT / relative).read_text(encoding="utf-8"))


def require_equal(left: dict, right: dict, fields: tuple[str, ...], family: str) -> None:
    for field in fields:
        require(left.get(field) == right.get(field), f"{family} parity mismatch: {field}")


def failure_map(packet: dict) -> dict[str, str]:
    return {item.get("name", ""): item.get("expected", "") for item in packet.get("failClosed", [])}


def validate() -> dict:
    mac_live = load("protocol/macos-e2ee-live-ptt-v1-vectors.json")
    win_live = load("protocol/windows-e2ee-live-ptt-v1-vectors.json")
    require_equal(mac_live, win_live, ("schemaVersion", "status", "wireAuthority", "opaqueFrame", "aadBindings", "bounds"), "live")
    live_common = {
        "tampered_ciphertext", "replayed_sequence", "reused_nonce", "stale_epoch",
        "changed_commit_digest", "foreign_target", "removed_sender", "gap_outside_window",
        "legacy_bp_downgrade", "unapproved_provider",
    }
    require(live_common <= set(mac_live.get("failClosed", [])), "macOS live common failures incomplete")
    require(live_common <= set(win_live.get("failClosed", [])), "Windows live common failures incomplete")
    require(set(mac_live.get("failClosed", [])) - live_common == {"missing_cross_process_owner_approval"},
            "macOS live platform-specific failure drifted")
    require(set(win_live.get("failClosed", [])) - live_common == {"malformed_provider_output"},
            "Windows live platform-specific failure drifted")

    mac_send = load("protocol/macos-protected-media-send-v1-vectors.json")
    win_send = load("protocol/windows-protected-media-send-v1-vectors.json")
    require_equal(mac_send, win_send, (
        "schemaVersion", "status", "fixtureSuite", "fixtureContainer", "sourceSHA256",
        "ciphertextSHA256", "manifestSHA256", "chunks", "resume",
    ), "send")
    send_common = {
        "unsupported-recipient": "unsupported_target",
        "target-membership-changed": "target_changed",
        "duplicate-nonce": "invalid_artifact",
        "invalid-signature": "invalid_artifact",
        "ciphertext-chunk-tamper": "persistence_failed",
        "resume-source-tamper": "invalid_request",
        "unapproved-runtime-provider": "production_disabled",
    }
    for name, expected in send_common.items():
        require(failure_map(mac_send).get(name) == expected, f"macOS send failure drifted: {name}")
        require(failure_map(win_send).get(name) == expected, f"Windows send failure drifted: {name}")
    require(win_send.get("crossPlatformFixture") == "macos-protected-media-send-v1-vectors",
            "Windows send cross-platform authority drifted")

    mac_play = load("protocol/macos-protected-media-playback-v1-vectors.json")
    win_play = load("protocol/windows-protected-media-playback-v1-vectors.json")
    require_equal(mac_play, win_play, (
        "schemaVersion", "status", "fixtureSuite", "fixtureContainer", "ciphertextSHA256",
        "chunks", "bounds", "platformProducers",
    ), "playback")
    playback_common = {
        "invalid-record-authentication": "invalid_authentication",
        "mixed-version-downgrade": "downgrade_forbidden",
        "wrong-target-snapshot": "target_changed",
        "membership-revision-change": "target_changed",
        "expired-route": "expired",
        "missing-history-grant": "missing_grant",
        "revoked-cache-restart": "revoked",
    }
    for name, expected in playback_common.items():
        require(failure_map(mac_play).get(name) == expected, f"macOS playback failure drifted: {name}")
        require(failure_map(win_play).get(name) == expected, f"Windows playback failure drifted: {name}")
    require(failure_map(mac_play).get("ciphertext-tamper") == "chunk_hash_mismatch",
            "macOS ciphertext-tamper classification drifted")
    require(failure_map(win_play).get("ciphertext-tamper") == "corrupt_ciphertext",
            "Windows ciphertext-tamper classification drifted")

    mac_client = load("protocol/macos-encrypted-media-client-path-v1.json")
    win_client = load("protocol/windows-encrypted-media-client-path-v1.json")
    require_equal(mac_client, win_client, (
        "schema_version", "status", "runtime_wired", "capability_advertised",
        "selected_production_suite", "paths", "commands", "recovery", "history_grants",
        "report_modes", "ui_forbidden_state", "fail_closed",
    ), "client")
    mac_requirements = set(mac_client.get("protected_status_requires", []))
    win_requirements = set(win_client.get("protected_status_requires", []))
    mac_owner = "retained_single-owner-or-cross-process-serialization-lease"
    win_owner = "win32-share-none-cross-process-generation-lock"
    require(mac_requirements - {mac_owner} == win_requirements - {win_owner},
            "client protected-status requirements drifted beyond ownership witness")
    require(mac_owner in mac_requirements and win_owner in win_requirements,
            "platform ownership witnesses missing")
    require(len(mac_client.get("composed_accepted_components", [])) == 4 and
            len(win_client.get("composed_accepted_components", [])) == 4,
            "client component inventory incomplete")

    return {
        "contract": "e2ee-cross-platform-repository-parity.v1",
        "families": 4,
        "manualInteroperability": "not-run",
        "scope": "repository-fixtures-only",
        "status": "pass",
    }


def main() -> int:
    print(json.dumps(validate(), sort_keys=True))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
