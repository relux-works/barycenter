#!/usr/bin/env python3
"""Fail-closed validation for the protected-media container spike no-go."""

from __future__ import annotations

import hashlib
import json
import pathlib
import subprocess


ROOT = pathlib.Path(__file__).resolve().parents[2]
CONTRACT_PATH = ROOT / "acceptance/phase3/protected-media-container-spike-v1.json"
PROBE = ROOT / "scripts/e2ee_container/probe"


class ProtectedMediaContainerSpikeError(ValueError):
    pass


def require(condition: bool, message: str) -> None:
    if not condition:
        raise ProtectedMediaContainerSpikeError(message)


def load(path: pathlib.Path = CONTRACT_PATH) -> dict:
    return json.loads(path.read_text(encoding="utf-8"))


def sha256(path: pathlib.Path) -> str:
    return hashlib.sha256(path.read_bytes()).hexdigest()


def git(*args: str) -> str:
    return subprocess.run(
        ["git", *args], cwd=ROOT, check=True, capture_output=True, text=True
    ).stdout.strip()


def records_by_id(records: list[dict]) -> dict[str, dict]:
    return {str(record.get("id", "")): record for record in records}


def validate(contract: dict) -> None:
    require(contract.get("schemaVersion") == 1, "unsupported schema")
    require(
        contract.get("contract") == "p3-protected-media-container-spike.v1",
        "wrong contract",
    )
    require(contract.get("task") == "TASK-260712-16xmy2", "wrong task")
    require(contract.get("publishedAt") == "2026-07-17", "publication date drifted")

    baseline = contract.get("baselineCommit", "")
    require(len(baseline) == 40, "baseline commit missing")
    require(git("rev-parse", baseline) == baseline, "baseline commit unavailable")
    git("merge-base", "--is-ancestor", baseline, "HEAD")

    require(
        contract.get("decision")
        == {
            "result": "repository-probe-pass-production-no-go",
            "repositoryProbeAccepted": True,
            "productionContainerSelected": False,
            "productionCodecSelected": False,
            "localPreparationToolchainSelected": False,
            "implementationAuthorized": False,
            "e2eeFeatureEnabled": False,
            "productClaimAllowed": False,
            "independentReview": "not-run",
        },
        "unsafe production decision or invented review",
    )

    upstream = contract.get("upstream", {})
    require(
        set(upstream) == {"threatModel", "codecPlayerHandoff", "streamPerformanceReview"},
        "upstream inventory drifted",
    )
    for record in upstream.values():
        path = ROOT / record.get("path", "")
        require(path.is_file(), f"upstream input missing: {path}")
        require(sha256(path) == record.get("sha256"), f"upstream input drifted: {path}")
    codec = json.loads((ROOT / upstream["codecPlayerHandoff"]["path"]).read_text(encoding="utf-8"))
    require(
        codec.get("decision", {}).get("production") == "no-go"
        and codec["decision"].get("selectedCodec") is None
        and codec["decision"].get("selectedContainer") is None,
        "upstream codec/player no-go no longer matches",
    )
    require(
        upstream["codecPlayerHandoff"].get("production") == "no-go"
        and upstream["codecPlayerHandoff"].get("selectedCodec") is None
        and upstream["codecPlayerHandoff"].get("selectedContainer") is None,
        "contract hides upstream codec/player no-go",
    )
    require(
        upstream["streamPerformanceReview"].get("openBlockingFindings")
        == ["P2-PERF-002", "P2-PERF-003", "P2-PERF-004"],
        "open P2 performance findings hidden",
    )

    probe = contract.get("probe", {})
    require(
        probe.get("contractName") == "pmc-probe-v1"
        and probe.get("registration") == "repository-experiment-only-never-e2ee_media_v1",
        "probe registered or represented as production",
    )
    require(
        probe.get("goVersion") == "1.25.12"
        and probe.get("goToolchain") == "go1.25.12"
        and probe.get("dependencies") == "go-standard-library-only",
        "probe toolchain or dependency boundary drifted",
    )
    require(
        probe.get("runtimeDownload") is False
        and probe.get("thirdPartyCodec") is False
        and probe.get("productionCodec") is None
        and probe.get("productionContainer") is None
        and probe.get("primitiveApprovedForProduction") is False,
        "unreviewed production or supply claim",
    )
    require(
        probe.get("deterministicVectorSHA256")
        == "1ed44e2c5e5739c97840d2d82ccb6582e16647686159a85578b0516eb74398b8",
        "deterministic vector drifted",
    )
    go_mod = (PROBE / "go.mod").read_text(encoding="utf-8")
    require(
        go_mod
        == "module relux.works/duet/e2ee-container-probe\n\ngo 1.25.12\n",
        "probe module is not exactly pinned or gained dependencies",
    )

    format_contract = contract.get("format", {})
    require(
        format_contract.get("headerBytes") == 144
        and format_contract.get("manifestTagBytes") == 16
        and format_contract.get("maximumChunkPlaintextBytes") == 1048576,
        "format bounds drifted",
    )
    require(
        format_contract.get("mediaKinds") == ["clip", "track", "saved-cue"],
        "media kind coverage drifted",
    )
    require(
        set(format_contract.get("keyDomains", []))
        == {
            "barycenter/pmc-probe/v1/manifest",
            "barycenter/pmc-probe/v1/private-manifest",
            "barycenter/pmc-probe/v1/chunks",
        },
        "key separation drifted",
    )
    for field in ("nonceRule", "rangeRule", "resumeRule", "replayBoundary", "wholeObjectRule"):
        require(format_contract.get(field), f"format rule missing: {field}")
    require(
        "must never reuse" in format_contract["nonceRule"],
        "nonce uniqueness responsibility hidden",
    )
    require(
        "replay of an entire still-valid container requires protocol-owned" in format_contract["replayBoundary"],
        "full-container replay boundary hidden",
    )
    require(
        format_contract.get("maximumFourChunkOverheadBytes") == 512,
        "overhead bound drifted",
    )

    evidence = contract.get("automatedEvidence", {})
    required_tests = {
        "deterministic-vector-and-round-trip",
        "independent-range-and-resume-boundaries",
        "header-manifest-private-and-chunk-tamper",
        "truncation",
        "chunk-reorder",
        "other-container-substitution",
        "stale-epoch-record-replay-into-current-container",
        "wrong-key",
        "unsafe-chunk-bound",
        "four-mebibyte-bounded-overhead",
        "two-hour-duration-metadata-authentication",
    }
    require(
        evidence.get("scope") == "repository-only-synthetic"
        and evidence.get("manualEvidence") == "not-run"
        and set(evidence.get("tests", [])) == required_tests,
        "automated evidence scope or vector matrix drifted",
    )
    require(
        evidence.get("syntheticTwoHourInputBytes") == 2097152
        and evidence.get("syntheticTwoHourChunkCount") == 2
        and evidence.get("syntheticTwoHourStructuralOverheadBytes") == 252
        and evidence.get("timingOrHeapMeasurementsAreAcceptanceEvidence") is False,
        "synthetic structural result misrepresented",
    )

    manual = contract.get("manualAndIndependentEvidence", {})
    require(manual.get("epic") == "EPIC-260714-th54l3", "manual epic missing")
    require(
        set(manual.values()) <= {
            "EPIC-260714-th54l3",
            "not-run",
            "not-run-no-codec-selected",
        },
        "manual or independent evidence invented",
    )
    require(manual.get("status") == "not-run", "manual status invented")

    lifecycle = contract.get("plaintextLifecycle", {})
    require(
        "compiler-or-runtime-guaranteed-zeroization" in lifecycle.get("notClaimed", [])
        and "production-codec-buffer-lifecycle" in lifecycle.get("notClaimed", []),
        "plaintext lifecycle limitations hidden",
    )
    supply = contract.get("licenseAndSupply", {})
    require(
        supply.get("runtimeExecutableDownload") is False
        and supply.get("productionSBOMComplete") is False
        and supply.get("productionCodecLicenseApproved") is False
        and supply.get("productionCryptoSupplyReviewPassed") is False,
        "production supply approval invented",
    )

    findings = records_by_id(contract.get("blockingFindings", []))
    require(set(findings) == {f"PMC-{number:03d}" for number in range(1, 9)}, "blocking findings incomplete")
    require(
        all(record.get("status") == "open" for record in findings.values()),
        "blocking finding closed without evidence",
    )
    require(
        findings["PMC-001"].get("severity") == "critical"
        and all(findings[f"PMC-{number:03d}"].get("severity") == "high" for number in range(2, 9)),
        "blocking severity drifted",
    )

    sources = records_by_id(contract.get("sources", []))
    require(set(sources) == {"GO-HKDF", "GO-AES", "GO-CIPHER", "GO-LICENSE"}, "source inventory incomplete")
    require(
        all(record.get("url", "").startswith(("https://go.dev/", "https://pkg.go.dev/")) for record in sources.values()),
        "non-primary toolchain source",
    )

    exit_contract = contract.get("exit", {})
    require(
        exit_contract.get("taskAcceptedAsNoGo") is True
        and exit_contract.get("nextTask") == "TASK-260712-3er89x"
        and exit_contract.get("e2eeMediaBlocked") is True
        and len(exit_contract.get("unblockRequires", [])) == 7,
        "unsafe exit decision",
    )

    source = (PROBE / "container.go").read_text(encoding="utf-8")
    tests = (PROBE / "container_test.go").read_text(encoding="utf-8")
    cli = (PROBE / "cmd/pmcprobe/main.go").read_text(encoding="utf-8")
    for fragment in (
        "It is not a production format",
        "MaximumChunkBytes  = 1 << 20",
        '"barycenter/pmc-probe/v1/manifest"',
        '"barycenter/pmc-probe/v1/private-manifest"',
        '"barycenter/pmc-probe/v1/chunks"',
        "func ResumeBoundary",
    ):
        require(fragment in source, f"probe source contract missing: {fragment}")
    for fragment in (
        "1ed44e2c5e5739c97840d2d82ccb6582e16647686159a85578b0516eb74398b8",
        't.Run("replay-stale-epoch"',
        "TestProbeContainerDurationMetadataAndBoundedOverhead",
    ):
        require(fragment in tests, f"probe vector missing: {fragment}")
    require(
        'Result:         "repository-experiment-pass-production-no-go"' in cli
        and 'ManualEvidence: "not-run"' in cli
        and "clear(master)" in cli,
        "probe CLI overclaims or lost lifecycle boundary",
    )


def main() -> None:
    validate(load())
    print("protected-media container spike: repository probe pass; production no-go")


if __name__ == "__main__":
    main()
