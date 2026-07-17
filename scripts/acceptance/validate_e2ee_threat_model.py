#!/usr/bin/env python3
"""Fail-closed validation for the P3 E2EE threat model and claim boundary."""

from __future__ import annotations

import json
import pathlib
import subprocess


ROOT = pathlib.Path(__file__).resolve().parents[2]
CONTRACT_PATH = ROOT / "acceptance/phase3/e2ee-threat-model-v1.json"


class E2EEThreatModelError(ValueError):
    pass


def require(condition: bool, message: str) -> None:
    if not condition:
        raise E2EEThreatModelError(message)


def load(path: pathlib.Path = CONTRACT_PATH) -> dict:
    return json.loads(path.read_text(encoding="utf-8"))


def git(*args: str) -> str:
    return subprocess.run(
        ["git", *args], cwd=ROOT, check=True, capture_output=True, text=True
    ).stdout.strip()


def ids(records: list[dict]) -> set[str]:
    return {str(record.get("id", "")) for record in records}


def validate(contract: dict) -> None:
    require(contract.get("schemaVersion") == 1, "unsupported schema")
    require(contract.get("contract") == "p3-e2ee-threat-model.v1", "wrong contract")
    require(contract.get("task") == "TASK-260712-2e2ymn", "wrong task")
    require(contract.get("publishedAt") == "2026-07-17", "publication date drifted")

    baseline = contract.get("baselineCommit", "")
    require(len(baseline) == 40, "baseline commit missing")
    require(git("rev-parse", baseline) == baseline, "baseline commit unavailable")
    git("merge-base", "--is-ancestor", baseline, "HEAD")

    require(
        contract.get("decision")
        == {
            "result": "threat-model-frozen-spikes-and-review-required",
            "threatModelFrozen": True,
            "spikesAuthorized": True,
            "implementationAuthorized": False,
            "e2eeFeatureEnabled": False,
            "productClaimAllowed": False,
            "independentReview": "not-run",
        },
        "unsafe E2EE decision or invented review result",
    )

    roles = contract.get("trustRoles", [])
    require(
        ids(roles)
        == {
            "client-endpoint",
            "coordinator-delivery-service",
            "coordinator-authentication-service",
            "object-storage-backup",
            "moderation",
            "telegram-spotify",
        },
        "trust role inventory drifted",
    )
    role_by_id = {record["id"]: record for record in roles}
    require(
        role_by_id["coordinator-delivery-service"].get("trust")
        == "untrusted-for-content",
        "coordinator delivery role trusted with content",
    )
    require(
        role_by_id["coordinator-authentication-service"].get("trust")
        == "identity-authority-requiring-independent-detection",
        "identity equivocation boundary missing",
    )
    require(
        "must not create, unwrap, escrow, log or recover content secrets"
        in role_by_id["coordinator-delivery-service"].get("boundary", ""),
        "coordinator secret boundary weakened",
    )
    require(
        "insufficient for a verified-device claim"
        in role_by_id["coordinator-authentication-service"].get("boundary", ""),
        "coordinator identity verification boundary weakened",
    )

    assets = contract.get("protectedAssets", [])
    require(len(assets) == 8 and len(set(assets)) == 8, "protected assets incomplete")
    for required in (
        "media-plaintext",
        "content-and-session-keys",
        "group-epoch-state",
        "local-decrypt-cache-and-temporary-plaintext",
    ):
        require(required in assets, f"protected asset missing: {required}")

    attackers = contract.get("attackerClasses", [])
    require(
        ids(attackers)
        == {
            "network-storage-backup-capture",
            "honest-but-curious-coordinator",
            "malicious-delivery-coordinator",
            "malicious-identity-coordinator",
            "malicious-group-member",
            "compromised-or-cloned-device",
            "lost-device-or-state",
            "local-os-compromise",
            "physical-recipient-recording",
            "availability-and-traffic-analysis",
        },
        "attacker inventory drifted",
    )
    attacker_by_id = {record["id"]: record for record in attackers}
    for attacker in (
        "network-storage-backup-capture",
        "honest-but-curious-coordinator",
        "malicious-delivery-coordinator",
        "malicious-identity-coordinator",
        "malicious-group-member",
        "compromised-or-cloned-device",
        "lost-device-or-state",
    ):
        require(attacker_by_id[attacker].get("inScope") is True, f"in-scope attacker lost: {attacker}")
    for exclusion in (
        "local-os-compromise",
        "physical-recipient-recording",
        "availability-and-traffic-analysis",
    ):
        require(attacker_by_id[exclusion].get("inScope") is False, f"non-goal drifted: {exclusion}")
    for attacker, fragment in {
        "malicious-delivery-coordinator": "Cannot decrypt or inject",
        "malicious-identity-coordinator": "must be detectable",
        "malicious-group-member": "retain, record, copy or disclose",
        "compromised-or-cloned-device": "Compromise exposes",
        "lost-device-or-state": "may be impossible",
        "local-os-compromise": "No confidentiality claim",
    }.items():
        require(
            fragment in attacker_by_id[attacker].get("guarantee", ""),
            f"attacker guarantee weakened: {attacker}",
        )

    paths = contract.get("mediaPaths", [])
    require(
        ids(paths)
        == {
            "clip",
            "track",
            "saved-cue",
            "live-ptt",
            "telegram-upload-or-delivery",
            "spotify-playback-or-control",
            "legacy-or-mixed-version-target",
            "voluntary-report-evidence",
        },
        "media path inventory drifted",
    )
    path_by_id = {record["id"]: record for record in paths}
    expected_path_states = {
        "clip": ("required-protected", "plaintext-coordinator-processing"),
        "track": ("required-protected", "plaintext-coordinator-storage"),
        "saved-cue": ("required-protected", "plaintext-canonical-media-reference"),
        "live-ptt": ("required-protected", "engineering-live-transport-without-e2ee"),
        "telegram-upload-or-delivery": ("excluded-plaintext-boundary", "telegram-and-bot-see-plaintext"),
        "spotify-playback-or-control": ("excluded-provider-boundary", "provider-controlled"),
        "legacy-or-mixed-version-target": ("fail-closed-or-explicitly-excluded", "no-e2ee-capability"),
        "voluntary-report-evidence": ("explicit-boundary-exit", "not-implemented"),
    }
    for path_id, (target, current) in expected_path_states.items():
        require(
            (path_by_id[path_id].get("targetState"), path_by_id[path_id].get("currentState"))
            == (target, current),
            f"media path state drifted: {path_id}",
        )
    for protected in ("clip", "track", "saved-cue", "live-ptt"):
        require(
            path_by_id[protected].get("targetState") == "required-protected",
            f"protected path downgraded: {protected}",
        )
        require(
            path_by_id[protected].get("currentState") != "protected",
            f"unreviewed current E2EE claim: {protected}",
        )
    require(
        path_by_id["legacy-or-mixed-version-target"].get("targetState")
        == "fail-closed-or-explicitly-excluded",
        "silent mixed-version downgrade allowed",
    )
    for excluded in ("telegram-upload-or-delivery", "spotify-playback-or-control"):
        require("Never" in path_by_id[excluded].get("claim", ""), f"excluded claim weakened: {excluded}")

    metadata = contract.get("metadataDisclosure", {})
    expected_metadata = {
        "coordinatorVisible": {
            "account-actor-and-device-identifiers",
            "orbit-air-and-membership-identifiers",
            "recipient-and-target-snapshots",
            "group-epoch-and-public-commit-identifiers",
            "protocol-container-and-capability-versions",
            "media-class-delivery-mode-and-policy-state",
            "ciphertext-size-chunk-count-and-declared-duration",
            "upload-send-play-receipt-and-revoke-timestamps",
            "network-address-connection-and-request-metadata",
            "retention-delete-report-and-audit-state",
        },
        "encryptedOrLocalOnly": {
            "audio-plaintext-and-decoded-samples",
            "content-session-epoch-and-history-grant-secrets",
            "user-file-name-title-caption-waveform-and-loudness-analysis",
            "encrypted-manifest-fields-not-required-for-routing",
            "local-decrypt-cache-and-temporary-plaintext",
        },
        "moderationVisibleWithoutEvidence": {
            "reporter-reported-subject-policy-category-and-target-metadata",
            "documented-routing-retention-and-audit-metadata",
            "cryptographic-validation-status-without-content-secrets",
        },
        "moderationVisibleAfterExplicitEvidence": {
            "purpose-limited-recipient-exported-plaintext-copy",
            "evidence-copy-hash-format-size-and-retention-state",
        },
    }
    require(set(metadata) == set(expected_metadata), "metadata categories drifted")
    for name, expected in expected_metadata.items():
        values = metadata.get(name, [])
        require(set(values) == expected and len(values) == len(expected), f"metadata inventory invalid: {name}")
    require(
        "ciphertext-size-chunk-count-and-declared-duration"
        in metadata["coordinatorVisible"],
        "size and duration leakage hidden",
    )
    require(
        "audio-plaintext-and-decoded-samples" in metadata["encryptedOrLocalOnly"],
        "audio plaintext not protected",
    )

    requirements = contract.get("requirements", [])
    expected_requirement_ids = {f"E2EE-{number:03d}" for number in range(1, 23)}
    require(ids(requirements) == expected_requirement_ids, "requirement inventory drifted")
    require(len({record.get("text") for record in requirements}) == 22, "duplicate requirement")
    requirement_text = "\n".join(record["text"] for record in requirements)
    for fragment in (
        "coordinator-issued credentials alone",
        "Never silently send protected",
        "irreversible protected-history loss",
        "No primitive, algorithm suite, library",
    ):
        require(fragment.lower() in requirement_text.lower(), f"required boundary missing: {fragment}")

    abuse = contract.get("abuseCases", [])
    require(
        ids(abuse)
        == {
            "JOIN-FAKE-DEVICE",
            "REVOKE-STALE-EPOCH",
            "HISTORY-GRANT-CONFUSED-DEPUTY",
            "RECOVERY-SERVER-ESCROW",
            "REPORT-HIDDEN-DECRYPT",
            "SILENT-PLAINTEXT-DOWNGRADE",
            "COORDINATOR-EQUIVOCATION",
            "NONCE-REUSE-AFTER-CRASH",
            "AUTHORIZED-MEMBER-EXFILTRATION",
            "LOCAL-PLAINTEXT-REMANENCE",
        },
        "abuse case inventory drifted",
    )
    for record in abuse:
        refs = record.get("requirements", [])
        require(refs and set(refs) <= expected_requirement_ids, f"bad abuse refs: {record.get('id')}")

    scenarios = contract.get("acceptanceScenarios", {})
    require(set(scenarios) == {"C4", "C5", "C6"}, "C4-C6 mapping incomplete")
    for name, record in scenarios.items():
        refs = record.get("requirements", [])
        require(refs and set(refs) <= expected_requirement_ids, f"bad scenario refs: {name}")
        require(record.get("claim"), f"scenario claim missing: {name}")
    require("removed" in scenarios["C4"]["claim"], "membership removal proof missing")
    require("cannot produce playable" in scenarios["C5"]["claim"], "coordinator privacy proof missing")
    require("deliberately export" in scenarios["C6"]["claim"], "report consent proof missing")

    claims = contract.get("claimRules", {})
    require(
        set(claims) == {"allowedOnlyAfterAllGates", "requiredLimitations", "forbidden"},
        "claim rule categories drifted",
    )
    expected_claims = {
        "allowedOnlyAfterAllGates": {
            "End-to-end encrypted between the verified Pulsar devices shown for this item.",
            "The coordinator routes this protected item but cannot read its audio content.",
            "This report copy leaves end-to-end encryption and will be available to authorized moderators.",
        },
        "requiredLimitations": {
            "Telegram uploads and deliveries are not Pulsar end-to-end encrypted.",
            "Removing a member or deleting an item cannot erase copies, keys or recordings already obtained by another device.",
            "Protected history may be irrecoverable without a surviving authorized device or user-held recovery capability.",
            "Size, duration, timing, participants, targets and delivery metadata remain visible as documented.",
        },
        "forbidden": {
            "all Pulsar audio is end-to-end encrypted",
            "the coordinator can never add or substitute a device",
            "deletion erases every copy",
            "moderators can inspect protected content without a report copy",
            "end-to-end encryption makes users anonymous",
            "TLS or a private Air means end-to-end encrypted",
            "unsupported targets transparently fall back to plaintext",
        },
    }
    for name, expected in expected_claims.items():
        values = claims.get(name, [])
        require(set(values) == expected and len(values) == len(expected), f"claim rules invalid: {name}")
    require(
        any("Telegram" in value for value in claims["requiredLimitations"]),
        "Telegram limitation missing",
    )
    require(
        any("private Air" in value for value in claims["forbidden"]),
        "room privacy confusion missing",
    )

    review = contract.get("externalReviewEntry", [])
    require(len(review) == 9 and len(set(review)) == 9, "review entry packet incomplete")
    for fragment in ("library-algorithm", "equivocation", "cross-platform", "critical-and-high"):
        require(any(fragment in item for item in review), f"review criterion missing: {fragment}")

    risks = contract.get("residualRisks", [])
    require(ids(risks) == {f"RR-{number:02d}" for number in range(1, 9)}, "residual risks incomplete")
    require(
        len({record.get("risk") for record in risks}) == 8
        and all(record.get("disposition") for record in risks),
        "residual risk disposition missing",
    )

    sources = contract.get("sources", [])
    require(
        ids(sources)
        == {"RFC9750", "RFC9420", "RFC9180", "RFC5116", "RFC3552", "NIST-SP-800-154-IPD"},
        "source inventory drifted",
    )
    for source in sources:
        url = source.get("url", "")
        require(
            url.startswith("https://www.rfc-editor.org/")
            or url.startswith("https://csrc.nist.gov/"),
            f"non-primary source: {source.get('id')}",
        )
        require(source.get("status") and source.get("use"), f"source context missing: {source.get('id')}")
    nist = next(source for source in sources if source["id"] == "NIST-SP-800-154-IPD")
    require("initial public draft" in nist["status"].lower(), "NIST draft represented as final")

    document = (ROOT / "docs/analysis/p3-e2ee-threat-model-v1.md").read_text(encoding="utf-8")
    normalized_document = " ".join(document.split()).lower()
    for fragment in (
        "does not authorize E2EE implementation",
        "coordinator-issued credential alone is not a verified-device claim",
        "Telegram upload and delivery are never called Pulsar E2EE",
        "There is no plaintext fallback",
        "C4 membership crypto",
        "C5 coordinator privacy",
        "C6 report",
        "NIST SP 800-154 initial public draft",
    ):
        require(fragment.lower() in normalized_document, f"threat-model disclosure missing: {fragment}")

    diagram = (ROOT / "docs/diagrams/p3-e2ee-threat-model-v1.puml").read_text(encoding="utf-8")
    for fragment in (
        "Client-owned epoch commit",
        "Coordinator: untrusted for content",
        "Equivocation must be detectable",
        "This is the only intentional content exit",
    ):
        require(fragment in diagram, f"diagram boundary missing: {fragment}")

    spec = (ROOT / "docs/spec-self-contained-audio.md").read_text(encoding="utf-8")
    require("p3-e2ee-threat-model-v1.md" in spec, "authoritative spec link missing")

    listing = (ROOT / "docs/store/phase1/listing-en-US.json").read_text(encoding="utf-8")
    privacy = (ROOT / "docs/legal/en/privacy.md").read_text(encoding="utf-8")
    require("not end-to-end encrypted" in listing, "current Store limitation removed early")
    require("not end-to-end encrypted" in privacy.lower(), "current privacy limitation removed early")


def main() -> int:
    contract = load()
    validate(contract)
    print(
        json.dumps(
            {
                "contract": contract["contract"],
                "decision": contract["decision"]["result"],
                "independentReview": contract["decision"]["independentReview"],
                "productClaimAllowed": contract["decision"]["productClaimAllowed"],
                "status": "pass",
            },
            sort_keys=True,
        )
    )
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
