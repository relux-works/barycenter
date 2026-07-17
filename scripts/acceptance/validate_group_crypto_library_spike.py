#!/usr/bin/env python3
"""Fail-closed validation for the group-crypto library spike no-go."""

from __future__ import annotations

import hashlib
import json
import pathlib
import subprocess


ROOT = pathlib.Path(__file__).resolve().parents[2]
CONTRACT_PATH = ROOT / "acceptance/phase3/group-crypto-library-spike-v1.json"


class GroupCryptoLibrarySpikeError(ValueError):
    pass


def require(condition: bool, message: str) -> None:
    if not condition:
        raise GroupCryptoLibrarySpikeError(message)


def load(path: pathlib.Path = CONTRACT_PATH) -> dict:
    return json.loads(path.read_text(encoding="utf-8"))


def sha256(path: pathlib.Path) -> str:
    return hashlib.sha256(path.read_bytes()).hexdigest()


def git(*args: str) -> str:
    return subprocess.run(
        ["git", *args], cwd=ROOT, check=True, capture_output=True, text=True
    ).stdout.strip()


def by_id(records: list[dict]) -> dict[str, dict]:
    return {str(record.get("id", "")): record for record in records}


def validate(contract: dict) -> None:
    require(contract.get("schemaVersion") == 1, "unsupported schema")
    require(contract.get("contract") == "p3-group-crypto-library-spike.v1", "wrong contract")
    require(contract.get("task") == "TASK-260712-3er89x", "wrong task")
    require(contract.get("publishedAt") == "2026-07-17", "publication date drifted")

    baseline = contract.get("baselineCommit", "")
    require(len(baseline) == 40, "baseline commit missing")
    require(git("rev-parse", baseline) == baseline, "baseline commit unavailable")
    git("merge-base", "--is-ancestor", baseline, "HEAD")

    require(
        contract.get("decision")
        == {
            "result": "rfc9420-protocol-fit-production-library-no-go",
            "standardizedProtocolCandidate": "RFC 9420 Messaging Layer Security",
            "productionLibrarySelected": False,
            "productionCipherSuiteSelected": False,
            "platformBindingsSelected": False,
            "canonicalSerializationSelected": False,
            "implementationAuthorized": False,
            "e2eeFeatureEnabled": False,
            "productClaimAllowed": False,
            "independentReview": "not-run",
        },
        "unsafe library selection or invented review",
    )

    upstream = by_id(contract.get("upstreamContracts", []))
    require(set(upstream) == {"threat-model", "container-spike"}, "upstream inventory drifted")
    for record in upstream.values():
        path = ROOT / record.get("path", "")
        require(path.is_file(), f"upstream contract missing: {path}")
        require(sha256(path) == record.get("sha256"), f"upstream contract drifted: {path}")
    require(upstream["container-spike"].get("production") == "no-go", "container no-go hidden")

    rubric = by_id(contract.get("rubric", []))
    require(set(rubric) == {f"GC-R{number:02d}" for number in range(1, 8)}, "rubric incomplete")
    referenced_requirements = {
        requirement
        for record in rubric.values()
        for requirement in record.get("requirements", [])
    }
    require(
        {
            "E2EE-002", "E2EE-003", "E2EE-004", "E2EE-005", "E2EE-006",
            "E2EE-007", "E2EE-008", "E2EE-009", "E2EE-010", "E2EE-011",
            "E2EE-012", "E2EE-013", "E2EE-014", "E2EE-017", "E2EE-018",
            "E2EE-020", "E2EE-021",
        }
        <= referenced_requirements,
        "threat-model requirements omitted from rubric",
    )
    rubric_text = "\n".join(record.get("rule", "") for record in rubric.values()).lower()
    for fragment in (
        "actor-visible device identity",
        "current verified client",
        "reject forks",
        "without downgrade",
        "never escrows",
        "delete consumed",
        "delivery as malicious",
    ):
        require(fragment in rubric_text, f"rubric boundary missing: {fragment}")

    protocols = by_id(contract.get("protocolAssessment", []))
    require(set(protocols) == {"ietf-mls-rfc9420", "pairwise-sender-key-or-custom-tree"}, "protocol comparison incomplete")
    mls = protocols["ietf-mls-rfc9420"]
    require(
        mls.get("status") == "only-standardized-fit-candidate"
        and mls.get("productionLibraryAvailable") is False,
        "RFC 9420 fit overrepresented as a production selection",
    )
    require(
        "device-identity-verification-and-authentication-service-equivocation"
        in mls.get("applicationResponsibilities", []),
        "identity equivocation incorrectly delegated to MLS",
    )
    require(
        protocols["pairwise-sender-key-or-custom-tree"].get("status") == "rejected"
        and "no-custom-cryptography" in protocols["pairwise-sender-key-or-custom-tree"].get("reason", ""),
        "custom or sender-key protocol admitted",
    )

    candidates = by_id(contract.get("libraryCandidates", []))
    require(set(candidates) == {"openmls", "mls-rs", "mlspp"}, "library candidate inventory drifted")
    require(
        all(record.get("result", "").startswith("no-go-") for record in candidates.values()),
        "unreviewed library represented as selected",
    )

    openmls = candidates["openmls"]
    require(
        openmls.get("snapshot")
        == {
            "version": "0.8.1",
            "tag": "openmls-v0.8.1",
            "commit": "47dbedecad0c1fd8eb5368d582250ebfcc1e1ce6",
            "commitDate": "2026-02-13T15:33:38Z",
            "commitVerified": True,
            "license": "MIT",
        },
        "OpenMLS snapshot drifted",
    )
    openmls_security = openmls.get("security", {})
    require(
        openmls_security.get("externalAudit") == "SRLabs-2026-complete"
        and openmls_security.get("auditFindings") == 8
        and openmls_security.get("highestFinding") == "high"
        and openmls_security.get("highFindingRemediatedInCandidate") is True
        and openmls_security.get("remainingAuditFinding") == "one-low-open"
        and openmls_security.get("fullDependencyAuditPerformedByThisTask") is False,
        "OpenMLS audit status invented or hidden",
    )
    advisories = by_id(openmls_security.get("repositoryAdvisories", []))
    require(
        set(advisories) == {"GHSA-8x3w-qj7j-gqhf", "GHSA-qr9h-x63w-vqfm"}
        and all(record.get("candidateAffected") is False for record in advisories.values()),
        "OpenMLS advisory snapshot drifted",
    )
    openmls_platform = openmls.get("platform", {})
    require(
        "aarch64-apple-darwin" in openmls_platform.get("buildOnlyRelevantTargets", [])
        and openmls_platform.get("officialSwiftBinding") is False
        and openmls_platform.get("officialGoBinding") is False
        and openmls_platform.get("signedMSIXEvidence") == "not-run"
        and openmls_platform.get("signedNotarizedMacOSEvidence") == "not-run",
        "OpenMLS platform gap hidden",
    )

    mls_rs = candidates["mls-rs"]
    require(
        mls_rs.get("snapshot", {}).get("version") == "0.55.2"
        and mls_rs["snapshot"].get("commit") == "42131c9959efb1d3928428259bc89853027f730d"
        and mls_rs["snapshot"].get("minimumRust") == "1.82.0",
        "mls-rs snapshot drifted",
    )
    require(
        mls_rs.get("security", {}).get("maintainerStatement")
        == "not-yet-fully-audited-by-a-third-party"
        and mls_rs["security"].get("zeroPublishedAdvisoriesMeansNoKnownVulnerabilities") is False
        and mls_rs["security"].get("fullDependencyAuditPerformedByThisTask") is False,
        "mls-rs audit gap hidden",
    )
    binding = mls_rs.get("bindingSnapshot", {})
    require(
        binding.get("crate") == "mls-rs-uniffi"
        and binding.get("version") == "0.13.0"
        and binding.get("cryptoProvider") == "mls-rs-crypto-openssl-0.21.0"
        and binding.get("officialForeignTests") == ["Kotlin", "Python"]
        and binding.get("officialSwiftTests") is False
        and binding.get("officialGoBinding") is False
        and binding.get("uniffiUsesCryptoKitProvider") is False,
        "mls-rs binding evidence overstated",
    )
    require(
        mls_rs.get("platform", {}).get("independentImplementationInterop") is False
        and mls_rs["platform"].get("upstreamInterop")
        == "mls-rs-full-vs-mls-rs-feature-variants-only",
        "mls-rs self interop represented as independent",
    )

    mlspp = candidates["mlspp"]
    require(
        mlspp.get("snapshot", {}).get("latestTag") == "v0.1.0"
        and mlspp["snapshot"].get("latestTagCommitDate") == "2021-05-26T03:22:45Z"
        and mlspp["snapshot"].get("mainAssessmentCommit")
        == "92aaa4134fa45ec39957a7c81a342401fba7feb2"
        and mlspp.get("supply", {}).get("currentVersionedReleaseForMain") is False
        and mlspp["supply"].get("externalAuditEvidenceFound") is False
        and mlspp["supply"].get("officialSwiftBinding") is False
        and mlspp["supply"].get("officialGoBinding") is False,
        "MLS++ release, audit or binding evidence overstated",
    )

    probe = contract.get("localProbe", {})
    require(
        probe.get("scope") == "repository-and-upstream-metadata-only"
        and probe.get("rustc") == "not-installed"
        and probe.get("cargo") == "not-installed"
        and probe.get("customCryptographicPrimitiveImplemented") is False
        and probe.get("thirdPartyCryptoDownloaded") is False
        and probe.get("runtimeExecutableDownloadAdded") is False,
        "local crypto implementation or toolchain claim drifted",
    )

    evidence = contract.get("evidenceMatrix", {})
    require(evidence.get("manualEpic") == "EPIC-260714-th54l3", "manual epic missing")
    for name, value in evidence.items():
        if name == "manualEpic":
            continue
        require(
            value in {"not-run", "not-run-no-selected-stack"},
            f"manual or interoperability evidence invented: {name}",
        )

    rules = contract.get("rulesFrozenForProtocolContract", [])
    require(len(rules) == 11 and len(set(rules)) == 11, "protocol handoff rules incomplete")
    for required in (
        "use-rfc9420-semantics-only-no-sender-key-or-custom-group-crypto",
        "authentication-service-device-binding-requires-independent-verification",
        "application-tracks-epoch-generation-fork-and-replay-state",
        "no-coordinator-content-key-generation-unwrap-escrow-or-recovery",
        "no-capability-or-cipher-suite-downgrade",
    ):
        require(required in rules, f"protocol handoff rule missing: {required}")

    findings = by_id(contract.get("blockingFindings", []))
    require(set(findings) == {f"GCL-{number:03d}" for number in range(1, 11)}, "blocking findings incomplete")
    require(
        findings["GCL-001"].get("severity") == "critical"
        and all(findings[f"GCL-{number:03d}"].get("severity") == "high" for number in range(2, 11))
        and all(record.get("status") == "open" for record in findings.values()),
        "blocking finding severity or status drifted",
    )

    sources = by_id(contract.get("sources", []))
    require(
        set(sources)
        == {
            "RFC9420", "RFC9750", "OPENMLS-RELEASE", "OPENMLS-README",
            "OPENMLS-BUILD", "OPENMLS-TESTS", "OPENMLS-LIBCRUX", "OPENMLS-AUDIT",
            "OPENMLS-HIGH", "OPENMLS-MEDIUM", "MLSRS-README", "MLSRS-CARGO",
            "MLSRS-UNIFFI", "MLSRS-CRYPTOKIT", "MLSRS-OPENSSL", "MLSRS-NATIVE-CI",
            "MLSRS-INTEROP-CI",
            "MLSPP-README", "MLSPP-LICENSE",
        },
        "primary source inventory incomplete",
    )
    allowed_prefixes = (
        "https://www.rfc-editor.org/",
        "https://github.com/openmls/",
        "https://blog.openmls.tech/",
        "https://github.com/awslabs/",
        "https://github.com/cisco/",
    )
    require(
        all(record.get("url", "").startswith(allowed_prefixes) for record in sources.values()),
        "non-primary library source",
    )

    exit_contract = contract.get("exit", {})
    require(
        exit_contract.get("taskAcceptedAsNoGo") is True
        and exit_contract.get("nextTask") == "TASK-260712-2ys1ww"
        and exit_contract.get("e2eeMediaBlocked") is True
        and exit_contract.get("downstreamMode")
        == "rfc9420-semantics-and-candidate-neutral-contract-only"
        and len(exit_contract.get("unblockRequires", [])) == 8,
        "unsafe downstream exit",
    )

    adr = (ROOT / "docs/analysis/p3-group-crypto-library-spike-no-go-v1.md").read_text(encoding="utf-8")
    adr_text = " ".join(adr.split())
    for fragment in (
        "No production library",
        "one still-open Low",
        "not yet received a full third-party security audit",
        "No RFC known-answer suite was run",
        "E2EE remains blocked, disabled and unclaimed",
    ):
        require(fragment in adr_text, f"ADR boundary missing: {fragment}")


def main() -> None:
    validate(load())
    print("group-crypto library spike: RFC 9420 fit; production library no-go")


if __name__ == "__main__":
    main()
