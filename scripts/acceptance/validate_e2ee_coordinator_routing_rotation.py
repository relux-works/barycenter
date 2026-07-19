#!/usr/bin/env python3
"""Fail-closed validation for dormant coordinator E2EE routing/rotation."""

from __future__ import annotations

import hashlib
import json
import pathlib
import re


ROOT = pathlib.Path(__file__).resolve().parents[2]
CONTRACT_PATH = ROOT / "acceptance/phase3/e2ee-coordinator-routing-rotation-v1.json"


class E2EERoutingRotationError(ValueError):
    pass


def require(condition: bool, message: str) -> None:
    if not condition:
        raise E2EERoutingRotationError(message)


def sha256(path: pathlib.Path) -> str:
    return hashlib.sha256(path.read_bytes()).hexdigest()


def load(path: pathlib.Path = CONTRACT_PATH) -> dict:
    return json.loads(path.read_text(encoding="utf-8"))


def validate(contract: dict) -> None:
    require(contract.get("schemaVersion") == 1, "unsupported schema")
    require(contract.get("contract") == "p3-e2ee-coordinator-routing-rotation.v1", "wrong contract")
    require(contract.get("task") == "TASK-260712-20j5tm", "wrong task")
    require(contract.get("publishedAt") == "2026-07-19", "publication date drifted")
    require(contract.get("decision") == {
        "result": "dormant-keyless-routing-foundation",
        "productionEnabled": False,
        "runtimeHTTPWired": False,
        "capabilityAdvertised": False,
        "productionLibrarySelected": False,
        "productionSuiteSelected": False,
        "productionContainerSelected": False,
        "plaintextFallbackAllowed": False,
        "coordinatorCreatesCommits": False,
        "coordinatorContentKeyAccess": False,
        "deltaReviewRequired": True,
    }, "production-dark decision drifted")

    artifacts = {item.get("id"): item for item in contract.get("artifacts", [])}
    require(set(artifacts) == {
        "schema", "foundation-repository", "routing-repository", "routing-tests",
        "coordinator-contract", "coordinator-contract-tests",
        "protocol-authority", "protocol-vectors",
        "threat-model", "key-lifecycle-packet", "schema-foundation-packet", "adr",
    }, "artifact inventory incomplete")
    for name, item in artifacts.items():
        path = ROOT / item.get("path", "")
        require(path.is_file(), f"artifact missing: {name}")
        require(sha256(path) == item.get("sha256"), f"artifact drifted: {name}")

    schema = (ROOT / artifacts["schema"]["path"]).read_text(encoding="utf-8")
    tables = set(contract.get("additiveTables", []))
    require(tables == {
        "e2ee_protocol_actor_bindings", "e2ee_group_members",
        "e2ee_rotation_requirements", "e2ee_group_event_deliveries",
    }, "routing table inventory drifted")
    for table in tables:
        require(f"CREATE TABLE IF NOT EXISTS {table}" in schema, f"table missing: {table}")
    require("e2ee_protocol_actor_binding_consistent" in schema,
            "protocol actor binding consistency trigger missing")
    require("e2ee_protocol_actor_binding_immutable" in schema,
            "protocol actor binding immutability trigger missing")
    require("e2ee_group_event_delivery_binding_immutable" in schema,
            "delivery binding immutability trigger missing")
    require("enabled INTEGER NOT NULL DEFAULT 0 CHECK(enabled = 0)" in schema,
            "E2EE production lock removed")

    declared_columns = set(re.findall(
        r"^\s{2}([a-z][a-z0-9_]*)\s+(?:TEXT|INTEGER|BLOB)\b", schema, re.MULTILINE
    ))
    forbidden = {
        "private_key", "key_package_private_key", "epoch_secret", "sender_key",
        "content_key", "recovery_secret", "history_grant_secret", "plaintext",
        "decrypted_evidence",
    }
    require(not (declared_columns & forbidden), "secret/plaintext storage column admitted")

    invariants = set(contract.get("routingInvariants", []))
    require(len(invariants) == 12, "routing invariant inventory drifted")
    for required in {
        "commit-single-winner-exact-next-epoch-and-predecessor",
        "removed-device-no-new-delivery-or-protected-write",
        "join-leave-device-revoke-actor-disable-require-rotation",
        "unsupported-target-no-plaintext-downgrade",
        "durable-restart-delivery-and-exact-acknowledgement",
        "coordinator-never-creates-unwraps-escrows-or-logs-secrets",
    }:
        require(required in invariants, f"routing invariant missing: {required}")
    require(all(value == "covered" for value in contract.get("fixtures", {}).values()),
            "fixture represented as covered without evidence")
    require(set(contract.get("openProductionGates", [])) == {
        "EPC-001", "EPC-002", "EPC-004", "EPC-005", "TASK-260712-1ulshp",
    }, "production gate hidden or closed")

    manual = contract.get("manualEvidence", {})
    require(manual.get("epic") == "EPIC-260714-th54l3", "manual epic missing")
    for key, value in manual.items():
        if key != "epic":
            require(value in {"not-run", "not-run-no-selected-stack"},
                    f"manual evidence invented: {key}")

    production_sources = [
        ROOT / "coordinator/internal/protocol/protocol.go",
        ROOT / "coordinator/cmd/duet-coordinator/main.go",
        ROOT / "pulsar-win/main.go",
        ROOT / "node-app/Sources/NodeCore/Protocol.swift",
        ROOT / "node-app/Sources/NodeApp/main.swift",
    ]
    for path in production_sources:
        require("e2ee_media_v1" not in path.read_text(encoding="utf-8"),
                f"capability leaked into production: {path}")
    require(not list((ROOT / "protocol/golden").glob("*e2ee*")),
            "draft E2EE added to production golden wire catalog")


def main() -> int:
    validate(load())
    print("e2ee coordinator routing/rotation: PASS (production disabled)")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
