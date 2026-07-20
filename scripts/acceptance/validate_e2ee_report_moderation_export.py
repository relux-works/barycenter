#!/usr/bin/env python3
"""Fail-closed validation for the production-dark E2EE report evidence boundary."""

from __future__ import annotations

import hashlib
import json
import pathlib
import re


ROOT = pathlib.Path(__file__).resolve().parents[2]
CONTRACT_PATH = ROOT / "acceptance/phase3/e2ee-report-evidence-moderation-export-v1.json"


class E2EEReportModerationError(ValueError):
    pass


def require(condition: bool, message: str) -> None:
    if not condition:
        raise E2EEReportModerationError(message)


def sha256(path: pathlib.Path) -> str:
    return hashlib.sha256(path.read_bytes()).hexdigest()


def load(path: pathlib.Path = CONTRACT_PATH) -> dict:
    return json.loads(path.read_text(encoding="utf-8"))


def validate(contract: dict) -> None:
    require(contract.get("schemaVersion") == 1, "unsupported schema")
    require(contract.get("contract") == "p3-e2ee-report-evidence-moderation-export.v1",
            "wrong contract")
    require(contract.get("task") == "TASK-260712-2i0w6x", "wrong task")
    require(contract.get("publishedAt") == "2026-07-20", "publication date drifted")
    require(contract.get("decision") == {
        "result": "production-dark-explicit-recipient-evidence-export-foundation",
        "productionEnabled": False,
        "runtimeHTTPWired": False,
        "storageAdapterWired": False,
        "capabilityAdvertised": False,
        "coordinatorDecryptsContent": False,
        "coordinatorStoresPlaintextEvidence": False,
        "metadataOnlyReporting": True,
        "explicitConsentRequired": True,
        "deltaReviewRequired": True,
    }, "production-dark decision drifted")

    artifacts = {item.get("id"): item for item in contract.get("artifacts", [])}
    require(set(artifacts) == {
        "schema", "report-schema", "foundation-repository", "report-repository",
        "opaque-router", "moderation-service", "store-tests", "service-tests", "adr",
    }, "artifact inventory incomplete")
    for name, item in artifacts.items():
        path = ROOT / item.get("path", "")
        require(path.is_file(), f"artifact missing: {name}")
        require(sha256(path) == item.get("sha256"), f"artifact drifted: {name}")

    report_schema = (ROOT / artifacts["report-schema"]["path"]).read_text(encoding="utf-8")
    tables = set(contract.get("additiveTables", []))
    require(tables == {
        "e2ee_moderation_reports", "e2ee_report_evidence_consents",
        "e2ee_report_evidence_state", "e2ee_moderation_decisions",
        "e2ee_report_audit_events",
    }, "report moderation table inventory drifted")
    for table in tables:
        require(f"CREATE TABLE IF NOT EXISTS {table}" in report_schema,
                f"table missing: {table}")
    require(" BLOB" not in report_schema, "evidence bytes admitted to report schema")
    columns = set(re.findall(
        r"^\s{2}([a-z][a-z0-9_]*)\s+(?:TEXT|INTEGER|BLOB)\b",
        report_schema, re.MULTILINE,
    ))
    require(not (columns & {
        "plaintext", "decrypted_evidence", "content_key", "group_secret",
        "private_key", "decoded_audio", "evidence_bytes",
    }), "secret/plaintext report column admitted")
    for token in {
        "explicit_report_evidence_export", "metadata_only", "provided",
        "e2ee_report_audit_no_update", "e2ee_report_audit_no_delete",
        "e2ee_report_consent_immutable", "e2ee_report_evidence_identity_immutable",
    }:
        require(token in report_schema, f"schema boundary missing: {token}")

    repository = (ROOT / artifacts["report-repository"]["path"]).read_text(encoding="utf-8")
    foundation = (ROOT / artifacts["foundation-repository"]["path"]).read_text(encoding="utf-8")
    router = (ROOT / artifacts["opaque-router"]["path"]).read_text(encoding="utf-8")
    service = (ROOT / artifacts["moderation-service"]["path"]).read_text(encoding="utf-8")
    for token in {
        "CreateE2EEModerationReport", "AttachE2EEReportEvidence",
        "authorizeE2EEProtectedRecipientTx", "AuthorizeE2EEReportEvidence",
        "DeleteE2EEReportEvidence", "ExpireE2EEReportEvidence",
        "BeginE2EEModerationDecision", "CompleteE2EEModerationDecision",
        "DeleteE2EEProtectedObjectForModeration",
    }:
        require(token in repository, f"repository boundary missing: {token}")
    require("return s.AttachE2EEReportEvidence(params)" not in foundation,
            "strict evidence delegate result accidentally discarded")
    require("created, err := s.AttachE2EEReportEvidence(params)" in foundation,
            "legacy evidence entry point bypasses strict transition")
    require("deleteE2EEProtectedObjectTx" in router and
            "moderation_decision" in repository,
            "canonical opaque deletion not reused")
    require("ApplyE2EEDecision" in service and
            "DeleteE2EEProtectedObjectForModeration" in service,
            "dormant moderation service seam incomplete")
    for token in {"slog.", "log.Printf", "fmt.Printf", "logger."}:
        require(token not in repository, f"report evidence can enter logs: {token}")

    runtime_sources = [
        ROOT / "coordinator/cmd/duet-coordinator/main.go",
        ROOT / "coordinator/cmd/duet-coordinator/moderation_http.go",
    ]
    for path in runtime_sources:
        source = path.read_text(encoding="utf-8")
        require("CreateE2EEModerationReport" not in source and
                "AuthorizeE2EEReportEvidence" not in source and
                "ApplyE2EEDecision" not in source,
                f"production runtime wiring admitted: {path}")

    require(contract.get("bounds") == {
        "statementBytes": 2000,
        "evidenceBytes": 67108864,
        "retentionDays": 30,
        "operatorListLimit": 100,
        "auditListLimit": 500,
        "expiryBatchLimit": 1000,
    }, "report resource bounds drifted")
    invariants = set(contract.get("invariants", []))
    require(len(invariants) == 12, "report invariant inventory drifted")
    for item in {
        "metadata-only-report-creates-no-evidence-or-consent-row",
        "new-evidence-export-requires-current-exact-recipient-access",
        "explicit-consent-and-authenticated-evidence-digests-are-immutable",
        "operator-create-read-delete-expire-and-decision-events-are-audited",
        "canonical-opaque-delete-removes-server-ciphertext-chunks",
        "no-runtime-route-storage-adapter-or-capability-advertisement",
    }:
        require(item in invariants, f"report invariant missing: {item}")
    require(all(value == "covered" for value in contract.get("fixtures", {}).values()),
            "fixture represented as covered without evidence")

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
    print("E2EE report moderation export: PASS (production disabled)")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
