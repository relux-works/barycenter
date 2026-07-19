#!/usr/bin/env python3
"""Fail-closed validation for the E2EE protocol/key-lifecycle audit packet."""

from __future__ import annotations

import hashlib
import json
import pathlib
import subprocess


ROOT = pathlib.Path(__file__).resolve().parents[2]
CONTRACT_PATH = ROOT / "acceptance/phase3/e2ee-protocol-key-lifecycle-v1.json"


class E2EEProtocolContractError(ValueError):
    pass


def require(condition: bool, message: str) -> None:
    if not condition:
        raise E2EEProtocolContractError(message)


def sha256(path: pathlib.Path) -> str:
    return hashlib.sha256(path.read_bytes()).hexdigest()


def git(*args: str) -> str:
    return subprocess.run(
        ["git", *args], cwd=ROOT, check=True, capture_output=True, text=True
    ).stdout.strip()


def by_id(records: list[dict]) -> dict[str, dict]:
    return {str(record.get("id", "")): record for record in records}


def load(path: pathlib.Path = CONTRACT_PATH) -> dict:
    return json.loads(path.read_text(encoding="utf-8"))


def validate(contract: dict) -> None:
    require(contract.get("schemaVersion") == 1, "unsupported schema")
    require(contract.get("contract") == "p3-e2ee-protocol-key-lifecycle.v1", "wrong contract")
    require(contract.get("task") == "TASK-260712-2ys1ww", "wrong task")
    require(contract.get("publishedAt") == "2026-07-17", "publication date drifted")
    baseline = contract.get("baselineCommit", "")
    require(len(baseline) == 40 and git("rev-parse", baseline) == baseline, "baseline unavailable")
    git("merge-base", "--is-ancestor", baseline, "HEAD")

    require(
        contract.get("decision")
        == {
            "result": "audit-contract-frozen-production-disabled",
            "protocolSemantics": "RFC 9420",
            "productionLibrarySelected": False,
            "productionCipherSuiteSelected": False,
            "productionContainerSelected": False,
            "canonicalMLSSerializationSelected": False,
            "implementationAuthorized": False,
            "capabilityAdvertised": False,
            "plaintextFallbackAllowed": False,
            "coordinatorContentKeyAccess": False,
            "independentReview": "not-run",
            "auditPacketReady": True,
        },
        "no-go or audit status drifted",
    )

    upstream = by_id(contract.get("upstreamContracts", []))
    require(set(upstream) == {"threat-model", "container-spike", "group-crypto-spike"}, "upstreams incomplete")
    for record in upstream.values():
        path = ROOT / record.get("path", "")
        require(path.is_file() and sha256(path) == record.get("sha256"), f"upstream drifted: {path}")
    require(upstream["container-spike"].get("production") == "no-go", "container no-go hidden")
    require(upstream["group-crypto-spike"].get("production") == "no-go", "crypto no-go hidden")

    resources = by_id(contract.get("authoritativeResources", []))
    require(set(resources) == {"protocol", "vectors", "adr"}, "authority set incomplete")
    for name in ("protocol", "vectors"):
        path = ROOT / resources[name].get("path", "")
        require(path.is_file() and sha256(path) == resources[name].get("sha256"), f"resource drifted: {name}")
    require((ROOT / resources["adr"].get("path", "")).is_file(), "ADR missing")

    protocol = json.loads((ROOT / resources["protocol"]["path"]).read_text(encoding="utf-8"))
    require(protocol.get("status") == "audit-only-production-disabled", "protocol enabled")
    capability = protocol.get("capability", {})
    require(
        capability == {
            "name": "e2ee_media_v1",
            "advertisementAllowed": False,
            "selectedSuite": None,
            "productionSuites": [],
        },
        "capability or suite selected",
    )
    require(protocol.get("mediaKinds") == ["clip", "live_ptt", "saved_cue", "track"], "media kinds drifted")
    require(len(protocol.get("stateRules", [])) == 11, "lifecycle rules incomplete")
    require(set(protocol.get("flows", {})) == {"membership", "storedMedia", "livePTT", "history", "recovery", "report"}, "flow inventory incomplete")
    expected_failures = {
        "downgrade", "expired_grant", "foreign_target", "forked_epoch",
        "invalid_signature", "nonce_reuse", "replay", "stale_epoch",
        "tampered_manifest", "unknown_suite",
    }
    require(set(protocol.get("failureCodes", [])) == expected_failures, "failure taxonomy drifted")
    forbidden = set(protocol.get("coordinatorForbiddenFields", []))
    require({"plaintext", "content_key", "epoch_secret", "private_key"} <= forbidden, "secret boundary incomplete")
    envelope_fields = protocol.get("coordinatorEnvelopeFields", {})
    require(
        set(envelope_fields) == {"proposal", "welcome", "key_package", "history_grant"}
        and all(envelope_fields.values()),
        "public envelope authority incomplete",
    )
    require(
        protocol.get("failurePrecedence")
        == [
            "downgrade", "unknown_suite", "malformed", "invalid_signature",
            "tampered_manifest", "foreign_target", "stale_epoch", "forked_epoch",
            "replay", "nonce_reuse", "expired_grant",
        ],
        "multi-fault failure precedence drifted",
    )

    vectors = json.loads((ROOT / resources["vectors"]["path"]).read_text(encoding="utf-8"))
    require(vectors.get("status") == "audit-only-production-disabled", "vectors represented as production")
    require(vectors.get("fixtureSuite") == "AUDIT_FIXTURE_SUITE_NOT_FOR_PRODUCTION", "fixture suite renamed")
    require(
        set(vectors.get("validContent", {})) == set(protocol.get("coordinatorVisibleFields", [])),
        "fixture and coordinator-visible field authority differ",
    )
    require(
        set(vectors.get("validCommit", {})) == set(protocol.get("commitFields", [])),
        "fixture and commit field authority differ",
    )
    malformed = vectors.get("malformedVectors", [])
    require(len(malformed) == 10, "malformed vector count drifted")
    require({record.get("expected") for record in malformed} == expected_failures, "malformed coverage incomplete")
    require(
        vectors.get("multiFaultVectors")
        == [{
            "name": "invalid-signature-precedes-tampered-manifest",
            "mutations": {
                "signature": "fixture-invalid",
                "manifest_digest": "e" * 64,
            },
            "expected": "invalid_signature",
        }],
        "dual-fault precedence vector drifted",
    )
    replay_vectors = vectors.get("replayStateVectors", [])
    require(
        [record.get("name") for record in replay_vectors]
        == [
            "sequence-regression", "generation-reset-must-start-at-one",
            "next-generation-starts-at-one",
        ]
        and [record.get("expected") for record in replay_vectors] == ["replay", "replay", ""],
        "sender sequence/generation vectors incomplete",
    )
    require("injected deterministic verifier" in vectors.get("fixtureVerifier", ""), "test verifier boundary hidden")

    implementations = by_id(contract.get("implementations", []))
    require(
        set(implementations)
        == {"coordinator-keyless-mirror", "coordinator-tests", "windows-audit-model", "windows-tests", "macos-audit-model", "macos-tests"},
        "cross-platform implementation inventory incomplete",
    )
    for record in implementations.values():
        path = ROOT / record.get("path", "")
        require(path.is_file() and sha256(path) == record.get("sha256"), f"implementation drifted: {path}")
    for name in ("coordinator-keyless-mirror", "windows-audit-model", "macos-audit-model"):
        require(implementations[name].get("runtimeWired") is False, f"{name} represented as runtime")
        require(implementations[name].get("cryptographyImplemented") is False, f"{name} represented as crypto")

    vocabulary = contract.get("vocabulary", {})
    require(vocabulary.get("capabilityAdvertisementAllowed") is False, "capability advertisement enabled")
    require(vocabulary.get("productionSuites") == [], "production suite admitted")
    require(set(vocabulary.get("failureCodes", [])) == expected_failures, "packet failure taxonomy drifted")
    require(len(contract.get("lifecycleCoverage", [])) == 11, "lifecycle coverage incomplete")
    bindings = set(contract.get("authenticatedDataBindings", []))
    for field in ("actor_id", "air_id", "target_snapshot_digest", "object_id", "generation", "sequence", "suite", "expires_at_ms"):
        require(field in bindings, f"authenticated binding missing: {field}")

    boundary = contract.get("coordinatorBoundary", {})
    require(
        boundary.get("routesCiphertextOnly") is True
        and boundary.get("generatesContentKeys") is False
        and boundary.get("unwrapsContentKeys") is False
        and boundary.get("escrowsEpochOrRecoverySecrets") is False
        and boundary.get("strictUnknownFieldRejection") is True,
        "coordinator boundary weakened",
    )

    production_sources = [
        ROOT / "coordinator/internal/protocol/protocol.go",
        ROOT / "pulsar-win/main.go",
        ROOT / "pulsar-win/wsclient.go",
        ROOT / "node-app/Sources/NodeCore/Protocol.swift",
        ROOT / "node-app/Sources/NodeCore/CoordinatorClient.swift",
        ROOT / "node-app/Sources/NodeApp/main.swift",
    ]
    for path in production_sources:
        require("e2ee_media_v1" not in path.read_text(encoding="utf-8"), f"capability leaked into production: {path}")
    require(not list((ROOT / "protocol/golden").glob("*e2ee*")), "draft E2EE added to production golden wire catalog")

    evidence = contract.get("automatedEvidence", {})
    require(evidence.get("sharedMalformedVectorCount") == 10, "automated vector count drifted")
    require(evidence.get("customCryptographicPrimitiveImplemented") is False, "custom crypto claim drifted")
    require(evidence.get("fixtureVerifier") == "injected deterministic verifier only", "verifier seam hidden")
    manual = contract.get("manualEvidence", {})
    require(manual.get("epic") == "EPIC-260714-th54l3", "manual epic missing")
    for key, value in manual.items():
        if key != "epic":
            require(value in {"not-run", "not-run-no-selected-stack"}, f"manual evidence invented: {key}")

    findings = by_id(contract.get("blockingFindings", []))
    require(set(findings) == {f"EPC-{index:03d}" for index in range(1, 6)}, "blocking findings incomplete")
    require(findings["EPC-001"].get("severity") == "critical", "library blocker downgraded")
    require(findings["EPC-002"].get("severity") == "critical", "container blocker downgraded")
    require(all(record.get("status") == "open" for record in findings.values()), "blocker closed without evidence")

    require(
        contract.get("handoff")
        == {
            "nextTask": "TASK-260712-aniuyy",
            "ownerEpic": "EPIC-260716-3qsztl",
            "reviewerMustReproduceExactHashes": True,
            "reviewMayAuthorizeImplementation": False,
            "implementationRemainsDeferredUntilAcceptedIndependentReview": True,
        },
        "independent audit handoff drifted",
    )


def main() -> int:
    validate(load())
    print("e2ee protocol/key-lifecycle audit packet: PASS (production disabled)")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
