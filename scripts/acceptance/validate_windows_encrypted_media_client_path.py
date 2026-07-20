#!/usr/bin/env python3
"""Validate the production-dark Windows encrypted-media client integration."""

from __future__ import annotations

import json
import pathlib


ROOT = pathlib.Path(__file__).resolve().parents[2]
PROTOCOL_PATH = ROOT / "protocol/windows-encrypted-media-client-path-v1.json"
EVIDENCE_PATH = ROOT / "acceptance/phase3/windows-encrypted-media-client-path-v1.json"


class WindowsEncryptedMediaClientPathError(ValueError):
    pass


def require(condition: bool, message: str) -> None:
    if not condition:
        raise WindowsEncryptedMediaClientPathError(message)


def load() -> tuple[dict, dict]:
    return (
        json.loads(PROTOCOL_PATH.read_text(encoding="utf-8")),
        json.loads(EVIDENCE_PATH.read_text(encoding="utf-8")),
    )


def validate(protocol: dict, evidence: dict) -> None:
    require(protocol.get("schema_version") == 1, "unsupported protocol schema")
    require(
        protocol.get("contract") == "windows-encrypted-media-client-path.v1",
        "wrong protocol contract",
    )
    require(protocol.get("status") == "production-dark", "production-dark status drifted")
    require(protocol.get("runtime_wired") is False, "runtime must remain dark")
    require(protocol.get("capability_advertised") is False, "capability must remain dark")
    require(protocol.get("selected_production_suite") is None, "suite must remain unselected")
    require(
        protocol.get("paths") == [
            "plaintext", "protected_clip", "protected_track", "protected_live",
        ],
        "path inventory drifted",
    )
    requirements = set(protocol.get("protected_status_requires", []))
    for item in {
        "reviewed_suite_selected",
        "capability_advertised",
        "same_repository_witness",
        "win32-share-none-cross-process-generation-lock",
        "verified_current_device",
        "current_membership_and_nonzero_epoch",
        "explicit_unsupported_recipient_exclusion",
    }:
        require(item in requirements, f"protected status gate missing: {item}")
    require(
        protocol.get("composed_accepted_components") == [
            "WindowsE2EEKeyStateRepository",
            "WindowsProtectedMediaSendService",
            "WindowsProtectedMediaPlaybackService",
            "WindowsE2EELiveSessionFactory",
        ],
        "accepted component composition drifted",
    )
    require(
        set(protocol.get("commands", [])) == {
            "refresh", "select_path", "verify_device", "revoke_device",
            "device_transfer", "user_held_recovery", "confirm_unsupported_exclusion",
            "create_history_grant", "revoke_history_grant", "report_metadata",
            "export_decrypted_evidence",
        },
        "command inventory drifted",
    )
    recovery = protocol.get("recovery", {})
    require(recovery.get("current_epoch_only") is True, "transfer broadened beyond current epoch")
    require(recovery.get("history_included_by_default") is False, "history became implicit")
    require(recovery.get("coordinator_key_recovery") is False, "coordinator recovery invented")
    require(recovery.get("irrecoverable_history_warning_required") is True,
            "irrecoverable warning removed")
    report_modes = {item.get("name"): item for item in protocol.get("report_modes", [])}
    require(set(report_modes) == {"metadata_only", "decrypted_evidence_copy"},
            "report boundary drifted")
    require(report_modes["metadata_only"].get("discloses_decrypted_media") is False,
            "metadata report disclosure drifted")
    require(report_modes["decrypted_evidence_copy"].get("discloses_decrypted_media") is True,
            "evidence disclosure hidden")
    require(report_modes["decrypted_evidence_copy"].get("separate_confirmation_required") is True,
            "evidence consent removed")
    fail_closed = set(protocol.get("fail_closed", []))
    for item in {
        "silent_plaintext_fallback", "false_encrypted_status",
        "unconfirmed_evidence_export", "unattested_generation_owner",
        "multiple_repository_instances",
    }:
        require(item in fail_closed, f"fail-closed rule missing: {item}")

    require(evidence.get("schemaVersion") == 1, "unsupported evidence schema")
    require(evidence.get("task") == "TASK-260712-2q4jbu", "wrong evidence task")
    require(evidence.get("contract") == protocol.get("contract"), "contract mismatch")
    require(evidence.get("scope") == "repository-automated-production-dark",
            "evidence scope drifted")
    require(evidence.get("production") == {
        "runtimeWired": False,
        "capabilityAdvertised": False,
        "providerSelected": False,
        "suiteSelected": False,
        "containerSelected": False,
    }, "production-dark evidence drifted")
    require(evidence.get("manualEpic") == "EPIC-260714-th54l3", "manual epic missing")
    require(len(evidence.get("manualDeferred", [])) >= 7, "manual claims were hidden")

    model = (ROOT / "pulsar-win/windows_encrypted_media_client.go").read_text()
    tests = (ROOT / "pulsar-win/windows_encrypted_media_client_test.go").read_text()
    main = (ROOT / "pulsar-win/main.go").read_text()
    for token in {
        "windowsEncryptedMediaProtectedFoundationReady",
        "UnsupportedExclusionCommand(confirmed bool)",
        "CreateHistoryGrantCommand(confirmed bool",
        "DecryptedEvidenceExportCommand(confirmed bool)",
        "SameRepositoryWitness",
        "WindowsEncryptedMediaPresentation",
        "AccessibleName",
        "Blocked; no plaintext fallback",
    }:
        require(token in model, f"model invariant missing: {token}")
    for token in {
        "NewWindowsEncryptedMediaClientPathComposition",
        "NewWindowsProtectedMediaSendService",
        "NewWindowsProtectedMediaPlaybackService",
        "NewWindowsE2EELiveSessionFactory",
        "KeyState: options.KeyState",
        "SameRepositoryWitness: componentsReady",
    }:
        require(token in model, f"composition invariant missing: {token}")
    require("WindowsEncryptedMediaClientPathComposition" not in main, "runtime composition wired")
    require("NewWindowsEncryptedMediaClientPathComposition" not in main, "runtime constructor wired")
    for token in {
        "UnsupportedTargetsNeverDowngrade",
        "DeviceLifecycleCommandsFailClosed",
        "HistoryAndReportConsentAreSeparate",
        "DescriptionsAndPresentationAreRedactedAndAccessible",
        "CompositionUsesOneRepositoryAndStaysRuntimeDark",
    }:
        require(token in tests, f"negative test missing: {token}")


def main() -> int:
    validate(*load())
    print("Windows encrypted-media client path: PASS (production dark)")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
