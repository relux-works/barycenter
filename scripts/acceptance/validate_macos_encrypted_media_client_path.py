#!/usr/bin/env python3
"""Validate the production-dark macOS encrypted-media client integration."""

from __future__ import annotations

import json
import pathlib


ROOT = pathlib.Path(__file__).resolve().parents[2]
PROTOCOL_PATH = ROOT / "protocol/macos-encrypted-media-client-path-v1.json"
EVIDENCE_PATH = ROOT / "acceptance/phase3/macos-encrypted-media-client-path-v1.json"


class MacEncryptedMediaClientPathError(ValueError):
    pass


def require(condition: bool, message: str) -> None:
    if not condition:
        raise MacEncryptedMediaClientPathError(message)


def load() -> tuple[dict, dict]:
    return (
        json.loads(PROTOCOL_PATH.read_text(encoding="utf-8")),
        json.loads(EVIDENCE_PATH.read_text(encoding="utf-8")),
    )


def validate(protocol: dict, evidence: dict) -> None:
    require(protocol.get("schema_version") == 1, "unsupported protocol schema")
    require(
        protocol.get("contract") == "macos-encrypted-media-client-path.v1",
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
        "retained_single-owner-or-cross-process-serialization-lease",
        "verified_current_device",
        "current_membership_and_nonzero_epoch",
        "explicit_unsupported_recipient_exclusion",
    }:
        require(item in requirements, f"protected status gate missing: {item}")
    require(
        protocol.get("composed_accepted_components") == [
            "MacE2EEKeyStateRepository",
            "MacProtectedMediaSendService",
            "MacProtectedMediaPlaybackService",
            "MacE2EELiveSessionFactory",
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
    require(evidence.get("task") == "TASK-260712-2nppt6", "wrong evidence task")
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

    model = (ROOT / "node-app/Sources/NodeAppUI/PulsarEncryptedMediaModel.swift").read_text()
    view = (ROOT / "node-app/Sources/NodeAppUI/PulsarEncryptedMediaView.swift").read_text()
    composition = (
        ROOT / "node-app/Sources/NodeApp/MacEncryptedMediaClientPathComposition.swift"
    ).read_text()
    main = (ROOT / "node-app/Sources/NodeApp/main.swift").read_text()
    tests = (
        ROOT / "node-app/Tests/NodeAppUITests/PulsarEncryptedMediaModelTests.swift"
    ).read_text()

    for token in {
        "protectedFoundationReady", "unsupportedExclusionCommand(confirmed: Bool)",
        "createHistoryGrantCommand(",
        "decryptedEvidenceExportCommand", "sameRepositoryWitness",
        "public var selectedPath", "return .confirmUnsupportedExclusion",
    }:
        require(token in model, f"model invariant missing: {token}")
    for token in {
        ".confirmationDialog(", "Report metadata only", "Include decrypted evidence",
        "no plaintext fallback", ".keyboardShortcut(\"r\", modifiers: .command)",
        ".accessibilityElement(children: .contain)",
    }:
        require(token in view, f"view boundary missing: {token}")
    require("onTapGesture" not in view, "non-accessible tap gesture added")
    for token in {
        "private let ownershipLease", "ownershipLease.coversOtherProcesses",
        "let repository = MacE2EEKeyStateRepository()",
        "MacProtectedMediaSendService(", "MacProtectedMediaPlaybackService(",
        "MacE2EELiveSessionFactory(", "keyState: repository",
        "crossProcessGenerationSerializationApproved: true",
    }:
        require(token in composition, f"composition ownership invariant missing: {token}")
    require("MacEncryptedMediaClientPathComposition" not in main, "runtime composition wired")
    require("PulsarEncryptedMediaView" not in main, "encrypted media view runtime-wired")
    for token in {
        "unsupportedTargetsNeverDowngrade", "deviceLifecycleCommandsFailClosed",
        "historyAndReportConsentAreSeparate", "descriptionsAreRedacted",
    }:
        require(token in tests, f"negative test missing: {token}")


def main() -> int:
    validate(*load())
    print("macOS encrypted-media client path: PASS (production dark)")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
