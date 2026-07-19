#!/usr/bin/env python3
"""Fail-closed validation for the dormant E2EE schema/epoch foundation."""

from __future__ import annotations

import hashlib
import json
import pathlib
import re


ROOT = pathlib.Path(__file__).resolve().parents[2]
CONTRACT_PATH = ROOT / "acceptance/phase3/e2ee-schema-epoch-foundation-v1.json"


class E2EESchemaFoundationError(ValueError):
    pass


def require(condition: bool, message: str) -> None:
    if not condition:
        raise E2EESchemaFoundationError(message)


def sha256(path: pathlib.Path) -> str:
    return hashlib.sha256(path.read_bytes()).hexdigest()


def load(path: pathlib.Path = CONTRACT_PATH) -> dict:
    return json.loads(path.read_text(encoding="utf-8"))


def validate(contract: dict) -> None:
    require(contract.get("schemaVersion") == 1, "unsupported schema")
    require(contract.get("contract") == "p3-e2ee-schema-epoch-foundation.v1", "wrong contract")
    require(contract.get("task") == "TASK-260712-3w1cst", "wrong task")
    require(contract.get("publishedAt") == "2026-07-19", "publication date drifted")
    require(
        contract.get("decision")
        == {
            "result": "additive-dormant-foundation",
            "productionEnabled": False,
            "runtimeWired": False,
            "productionLibrarySelected": False,
            "productionSuiteSelected": False,
            "productionContainerSelected": False,
            "plaintextFallbackAllowed": False,
            "coordinatorContentKeyAccess": False,
            "deltaReviewRequired": True,
        },
        "production-dark decision drifted",
    )

    artifacts = {item.get("id"): item for item in contract.get("artifacts", [])}
    expected_artifacts = {
        "schema", "repository", "tests", "startup", "protocol", "vectors",
        "coordinator-model", "coordinator-model-tests", "windows-model",
        "windows-model-tests", "macos-model", "macos-model-tests", "adr",
    }
    require(set(artifacts) == expected_artifacts, "artifact inventory incomplete")
    for name, item in artifacts.items():
        path = ROOT / item.get("path", "")
        require(path.is_file(), f"artifact missing: {name}")
        require(sha256(path) == item.get("sha256"), f"artifact drifted: {name}")

    schema = (ROOT / artifacts["schema"]["path"]).read_text(encoding="utf-8")
    tables = contract.get("tables", [])
    require(len(tables) == 20 and len(set(tables)) == 20, "table inventory drifted")
    for table in tables:
        require(f"CREATE TABLE IF NOT EXISTS {table}" in schema, f"table missing: {table}")
    require(
        re.search(r"enabled INTEGER NOT NULL DEFAULT 0 CHECK\(enabled = 0\)", schema) is not None
        and "selected_suite TEXT NOT NULL DEFAULT '' CHECK(selected_suite = '')" in schema
        and "selected_container TEXT NOT NULL DEFAULT '' CHECK(selected_container = '')" in schema,
        "schema no longer physically locks production off",
    )
    require("e2ee_audit_events_no_update" in schema and "e2ee_audit_events_no_delete" in schema,
            "immutable audit triggers missing")
    require("e2ee_protected_object_payload_immutable" in schema, "immutable payload trigger missing")

    forbidden = set(contract.get("forbiddenStorageFields", []))
    required_forbidden = {
        "private_key", "key_package_private_key", "epoch_secret", "sender_key",
        "content_key", "recovery_secret", "history_grant_secret", "plaintext",
        "decrypted_evidence",
    }
    require(forbidden == required_forbidden, "forbidden storage inventory drifted")
    # Parse only SQL column declarations, not comments describing the boundary.
    declared_columns = set(
        re.findall(r"^\s{2}([a-z][a-z0-9_]*)\s+(?:TEXT|INTEGER|BLOB)\b", schema, re.MULTILINE)
    )
    require(not (declared_columns & forbidden), "secret/plaintext storage column admitted")

    transitions = set(contract.get("conditionalTransitions", []))
    require(
        transitions
        == {
            "exact-previous-epoch-and-commit-single-winner",
            "fork-state-freezes-protected-writes",
            "staged-ready-revoked-exact-revision",
            "event-and-nonce-replay-persistence",
            "monotonic-generation-and-sequence",
            "history-grant-revoke-single-winner",
            "immutable-payload-and-audit-triggers",
        },
        "conditional transition inventory drifted",
    )
    require(all(value == "covered" for value in contract.get("fixtures", {}).values()),
            "fixture represented as covered without evidence")
    findings = contract.get("reviewFindings", {})
    require(
        findings.get("IDR-001") == "implemented-pending-delta-review"
        and findings.get("IDR-002") == "implemented-pending-delta-review"
        and findings.get("IDR-003") == "implemented-pending-delta-review",
        "design-review delta hidden",
    )
    require(
        set(contract.get("openProductionGates", []))
        == {"EPC-001", "EPC-002", "EPC-004", "EPC-005", "TASK-260712-1ulshp"},
        "production gate hidden or closed",
    )
    manual = contract.get("manualEvidence", {})
    require(manual.get("epic") == "EPIC-260714-th54l3", "manual epic missing")
    for key, value in manual.items():
        if key != "epic":
            require(value in {"not-run", "not-run-no-selected-stack"}, f"manual evidence invented: {key}")

    production_sources = [
        ROOT / "coordinator/internal/protocol/protocol.go",
        ROOT / "pulsar-win/main.go",
        ROOT / "pulsar-win/wsclient.go",
        ROOT / "node-app/Sources/NodeCore/Protocol.swift",
        ROOT / "node-app/Sources/NodeCore/CoordinatorClient.swift",
        ROOT / "node-app/Sources/NodeApp/main.swift",
    ]
    for path in production_sources:
        require("e2ee_media_v1" not in path.read_text(encoding="utf-8"),
                f"capability leaked into production: {path}")
    require(not list((ROOT / "protocol/golden").glob("*e2ee*")),
            "draft E2EE added to production golden wire catalog")


def main() -> int:
    validate(load())
    print("e2ee schema/epoch foundation: PASS (production disabled)")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
