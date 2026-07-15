#!/usr/bin/env python3
"""Fail-closed validation for the exact codec license/distribution audit."""

from __future__ import annotations

import json
import pathlib
import re


ROOT = pathlib.Path(__file__).resolve().parents[2]
AUDIT_PATH = ROOT / "acceptance" / "codec-spike" / "license-audit-v1.json"
SHA256 = re.compile(r"[0-9a-f]{64}")
COMMIT = re.compile(r"[0-9a-f]{40}")


def require(condition: bool, message: str) -> None:
    if not condition:
        raise ValueError(message)


def load(path: pathlib.Path = AUDIT_PATH) -> dict:
    with path.open(encoding="utf-8") as stream:
        return json.load(stream)


def validate(audit: dict) -> None:
    require(audit.get("schemaVersion") == 1, "unsupported audit schema")
    require(audit.get("contract") == "p2-codec-license-distribution-audit.v1", "wrong audit id")
    require(audit.get("auditedAt") == "2026-07-15", "audit retrieval date drifted")
    policy = audit.get("decisionPolicy", {})
    require(policy.get("runtimeExecutableDownloads") == "forbidden", "runtime download allowed")
    require(policy.get("sandboxWeakening") == "forbidden", "sandbox weakening allowed")
    require(policy.get("unknownRuntimeDependencies") == "forbidden", "unknown dependencies allowed")
    require(policy.get("unpatchedKnownVulnerabilities") == "forbidden", "unpatched CVEs allowed")
    require(policy.get("patentConclusion") == "legal-review-required", "patent review bypassed")

    sources = audit.get("authoritativeSources", [])
    source_ids = [item.get("id") for item in sources]
    require(len(source_ids) == len(set(source_ids)) >= 12, "source inventory incomplete or duplicated")
    require(all(item.get("url", "").startswith("https://") for item in sources), "non-HTTPS source")
    require(all(item.get("retrievedAt") == audit["auditedAt"] for item in sources), "source date missing")
    for required in ("ffmpeg-legal", "ffmpeg-security", "microsoft-codecs", "apple-stream-audio",
                     "apple-code-signing", "apple-notarization", "apple-review", "aac-pool",
                     "mp3-program", "opus-license", "go-vuln-db"):
        require(required in source_ids, f"missing authoritative source {required}")

    components = audit.get("components", [])
    by_id = {item.get("id"): item for item in components}
    require(len(by_id) == len(components) == 7, "exact component inventory changed")
    require(all(isinstance(item.get("runtimeDependencies"), list) for item in components),
            "component has unknown runtime dependencies")
    require(all(item.get("status") in policy["allowedClassifications"] for item in components),
            "component classification invalid")

    expected_modules = {
        "go-mp3-v0.3.4": ("Apache-2.0", "26f0b17459ab4b2b3d9d2b484a27a4eab3cca17b",
                           "b40930bbcf80744c86c46a12bc9da056641d722716c378f5659b9e555ef833e1"),
        "go-aac-5f2857eb82ad": ("GPL-2.0-only", "5f2857eb82ad85603d217fb231bb066e179ff124",
                                "4d0dcf8cdc91412bff804eeaf2e62d5ab699b79589f4ede05b7efc16e47288a9"),
        "mp4ff-v0.54.0": ("MIT", "8c9f99a4143239827775e16a99cb89137882cec1",
                          "bea4e5a13d04f33d2743929c91f5552bbac369212c9f8731bc8d70e919a96ef1"),
        "pion-opus-v0.1.0": ("MIT", "eb60bfd0e51a58e122fcfd0c0df027b014db7daa",
                             "9ea8a12deea1b232d639881e35b0540f8678315ebcc27053a9d8359bd78ad040"),
    }
    for component_id, (license_id, commit, license_hash) in expected_modules.items():
        item = by_id.get(component_id, {})
        require(item.get("license") == license_id, f"{component_id} license drifted")
        require(item.get("sourceCommit") == commit and COMMIT.fullmatch(commit),
                f"{component_id} source commit drifted")
        require(item.get("licenseSha256") == license_hash and SHA256.fullmatch(license_hash),
                f"{component_id} license digest drifted")
        require(SHA256.fullmatch(item.get("sourceZipSha256", "")) is not None,
                f"{component_id} source zip digest missing")
        require(item.get("moduleSum", "").startswith("h1:") and
                item.get("goModSum", "").startswith("h1:"), f"{component_id} Go sums missing")
        require("vulnerability" in " ".join(item.keys()).lower() or item.get("vulnerabilityScan"),
                f"{component_id} vulnerability disposition missing")

    aac = by_id["go-aac-5f2857eb82ad"]
    require(aac.get("status") == "rejected", "GPL AAC component must remain rejected")
    require("FAAD2" in aac.get("provenance", ""), "AAC derivative provenance missing")
    require(len(aac.get("rejectionReasons", [])) >= 2, "AAC rejection is not actionable")

    ffmpeg = by_id.get("ffmpeg-8.1.2-minimal-shared", {})
    require(ffmpeg.get("sourceSha256") ==
            "464beb5e7bf0c311e68b45ae2f04e9cc2af88851abb4082231742a74d97b524c",
            "FFmpeg source drifted")
    require(ffmpeg.get("status") == "shippable-with-obligations", "FFmpeg disposition drifted")
    required_flags = {"--disable-everything", "--disable-autodetect", "--disable-programs",
                      "--disable-doc", "--disable-network", "--disable-static", "--enable-shared"}
    require(set(ffmpeg.get("requiredConfigure", [])) == required_flags, "FFmpeg build floor changed")
    require(set(ffmpeg.get("forbiddenConfigure", [])) ==
            {"--enable-gpl", "--enable-version3", "--enable-nonfree"}, "FFmpeg license tripwire changed")
    require(ffmpeg.get("allowedMediaSurface", {}).get("decoders") == ["aac", "mp3", "opus"],
            "FFmpeg decoder surface changed")
    require("no ffmpeg/ffprobe CLI" in ffmpeg.get("packageShape", ""), "FFmpeg CLI entered package")
    require(ffmpeg.get("patentDisposition", "").endswith("rights"), "codec patent caveat missing")

    candidates = audit.get("candidates", [])
    candidate_by_id = {item.get("id"): item for item in candidates}
    require(list(candidate_by_id) == ["native-canonical-aac-v1", "pure-go-composite-v1",
                                     "bundled-ffmpeg-8.1.2-v1"], "candidate order/inventory changed")
    require(candidate_by_id["pure-go-composite-v1"].get("classification") == "rejected",
            "pure-Go composite must remain rejected")
    require(candidate_by_id["native-canonical-aac-v1"].get("classification") ==
            "shippable-with-obligations", "native candidate disposition drifted")
    require(candidate_by_id["bundled-ffmpeg-8.1.2-v1"].get("classification") ==
            "shippable-with-obligations", "bundled candidate disposition drifted")
    for candidate in candidates:
        require(candidate.get("classification") in policy["allowedClassifications"],
                "candidate classification invalid")
        require(all(component in by_id for component in candidate.get("components", [])),
                "candidate references unknown component")
        require(candidate.get("binaryShape"), "candidate binary shape missing")
        require(candidate.get("unresolvedLegal"), "candidate legal disposition missing")

    gates = set(audit.get("releaseGates", []))
    for gate in ("complete-runtime-sbom", "zero-known-unpatched-vulnerabilities",
                 "notices-and-corresponding-source-published", "signed-msix-all-architectures",
                 "signed-notarized-macos-arm64", "sandbox-and-no-download-receipts",
                 "aac-counsel-approval"):
        require(gate in gates, f"release gate missing: {gate}")


def main() -> int:
    validate(load())
    print(f"codec license audit valid: {AUDIT_PATH.relative_to(ROOT)}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
