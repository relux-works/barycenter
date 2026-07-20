#!/usr/bin/env python3
"""Fail-closed validation for production-dark E2EE recovery and history grants."""

from __future__ import annotations

import hashlib
import json
import pathlib
import re


ROOT = pathlib.Path(__file__).resolve().parents[2]
CONTRACT_PATH = ROOT / "acceptance/phase3/e2ee-recovery-device-transfer-v1.json"


class E2EERecoveryError(ValueError):
    pass


def require(condition: bool, message: str) -> None:
    if not condition:
        raise E2EERecoveryError(message)


def sha256(path: pathlib.Path) -> str:
    return hashlib.sha256(path.read_bytes()).hexdigest()


def load(path: pathlib.Path = CONTRACT_PATH) -> dict:
    return json.loads(path.read_text(encoding="utf-8"))


def validate(contract: dict) -> None:
    require(contract.get("schemaVersion") == 1, "unsupported schema")
    require(contract.get("contract") == "p3-e2ee-recovery-device-transfer.v1",
            "wrong contract")
    require(contract.get("task") == "TASK-260712-1rziyo", "wrong task")
    require(contract.get("publishedAt") == "2026-07-20", "publication date drifted")
    require(contract.get("decision") == {
        "result": "production-dark-explicit-current-epoch-transfer-and-history-grants",
        "productionEnabled": False,
        "runtimeHTTPWired": False,
        "capabilityAdvertised": False,
        "productionCryptoSelected": False,
        "coordinatorDecryptsOrUnwraps": False,
        "historicalKeysByDefault": False,
        "manualDeviceEvidenceAccepted": False,
        "deltaReviewRequired": True,
    }, "production-dark decision drifted")

    artifacts = {item.get("id"): item for item in contract.get("artifacts", [])}
    require(set(artifacts) == {
        "foundation-repository", "recovery-schema", "recovery-repository",
        "routing-repository", "coordinator-tests", "macos-repository",
        "macos-tests", "windows-repository", "windows-tests", "vectors", "adr",
    }, "artifact inventory incomplete")
    for name, item in artifacts.items():
        path = ROOT / item.get("path", "")
        require(path.is_file(), f"artifact missing: {name}")
        require(sha256(path) == item.get("sha256"), f"artifact drifted: {name}")

    schema = (ROOT / artifacts["recovery-schema"]["path"]).read_text(encoding="utf-8")
    repository = (ROOT / artifacts["recovery-repository"]["path"]).read_text(encoding="utf-8")
    routing = (ROOT / artifacts["routing-repository"]["path"]).read_text(encoding="utf-8")
    macos = (ROOT / artifacts["macos-repository"]["path"]).read_text(encoding="utf-8")
    windows = (ROOT / artifacts["windows-repository"]["path"]).read_text(encoding="utf-8")

    for table in {"e2ee_transfer_package_bindings", "e2ee_history_grant_bindings"}:
        require(f"CREATE TABLE IF NOT EXISTS {table}" in schema,
                f"recovery binding table missing: {table}")
    columns = set(re.findall(
        r"^\s{2}([a-z][a-z0-9_]*)\s+(?:TEXT|INTEGER|BLOB)\b", schema, re.MULTILINE))
    require(not (columns & {
        "plaintext", "content_key", "group_secret", "private_key",
        "recovery_secret", "unwrapped_key", "decrypted_media",
    }), "secret/plaintext recovery column admitted")
    for token in {
        "e2ee_transfer_package_binding_immutable",
        "e2ee_history_grant_binding_immutable",
        "issuer_device_revision", "recipient_device_revision",
        "issuer_verification_digest", "recipient_verification_digest",
    }:
        require(token in schema, f"recovery schema boundary missing: {token}")

    for token in {
        "CreateAuthorizedE2EETransferPackage", "ConsumeAuthorizedE2EETransferPackage",
        "RevokeAuthorizedE2EETransferPackage", "CreateAuthorizedE2EEHistoryGrant",
        "AuthorizeE2EEHistoryGrant", "RevokeAuthorizedE2EEHistoryGrant",
        "ExpireE2EERecoveryArtifacts", "revokeE2EERecoveryForDeviceTx",
    }:
        require(token in repository, f"recovery repository boundary missing: {token}")
    require("issuer.Member.OrbitID != recipient.Member.OrbitID" in repository,
            "same-user device-transfer restriction missing")
    require("read_count < max_reads" in repository,
            "atomic history read budget missing")
    require("revokeE2EERecoveryForDeviceTx" in routing and
            "reconcileE2EERotationTx" in routing,
            "lost-device revoke and rotation are not atomic")

    require("resetDeviceIdentityForReenrollment" in macos and
            "cleanupExpiredGrants" in macos,
            "macOS reset or bounded cleanup missing")
    require("ResetDeviceIdentityForReenrollment" in windows and
            "CleanupExpiredGrants" in windows,
            "Windows reset or bounded cleanup missing")
    for source in (repository, macos, windows):
        for token in {"log.Printf", "slog.", "fmt.Printf", "Logger("}:
            require(token not in source, f"recovery secret can enter logs: {token}")

    vectors = json.loads((ROOT / artifacts["vectors"]["path"]).read_text(encoding="utf-8"))
    require(vectors.get("contract") == "e2ee-recovery.v1", "wrong recovery vectors")
    require(vectors.get("status") == "production-disabled", "recovery vectors enabled")
    require(vectors.get("transfer_max_ttl_ms") == 900000, "transfer TTL drifted")
    require(vectors.get("history_max_ttl_ms") == 2592000000, "history TTL drifted")
    require(vectors.get("local_cleanup_max_grants") == 100, "cleanup bound drifted")
    require(len(vectors.get("fail_closed", [])) == 10, "fail-closed inventory drifted")
    require(contract.get("bounds") == {
        "transferTTLMS": 900000,
        "historyTTLMS": 2592000000,
        "historyMaxReads": 32,
        "coordinatorExpiryBatch": 1000,
        "localCleanupGrantIDs": 100,
    }, "recovery resource bounds drifted")

    require(len(contract.get("invariants", [])) == 12,
            "recovery invariant inventory drifted")
    require(all(value == "covered" for value in contract.get("fixtures", {}).values()),
            "fixture represented as covered without evidence")

    for path in [
        ROOT / "coordinator/cmd/duet-coordinator/main.go",
        ROOT / "coordinator/cmd/duet-coordinator/moderation_http.go",
        ROOT / "node-app/Sources/NodeCore/CoordinatorClient.swift",
        ROOT / "pulsar-win/main.go",
    ]:
        source = path.read_text(encoding="utf-8")
        require("ConsumeAuthorizedE2EETransferPackage" not in source and
                "AuthorizeE2EEHistoryGrant" not in source,
                f"production recovery wiring admitted: {path}")

    manual = contract.get("manualEvidence", {})
    require(manual.get("epic") == "EPIC-260714-th54l3", "manual epic missing")
    for key, value in manual.items():
        if key != "epic":
            require(value in {"not-run", "not-run-no-selected-stack"},
                    f"manual evidence invented: {key}")
    require(set(contract.get("openProductionGates", [])) == {
        "EPC-001", "EPC-002", "EPC-004", "EPC-005", "TASK-260712-1ulshp",
    }, "production gate hidden or closed")


def main() -> int:
    validate(load())
    print("E2EE recovery/device transfer: PASS (production disabled)")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
