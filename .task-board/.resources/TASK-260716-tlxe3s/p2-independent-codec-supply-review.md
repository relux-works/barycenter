# Phase 2 codec, license and supply-chain review

Date: 2026-07-16  
Task: `TASK-260712-2g3fkt`  
Reviewed base: `496c07272e4a5406b44be8709fa84c9b5932cdda`  
Engineering reviewer: `codex-inline-reviewer`  
Independent approver: Ivan Oparin

## Decision

**BLOCK PHASE 2.** There is no accepted Windows-plus-macOS decoder
combination. Production playback remains disabled, all six high findings are
open, and the next strict-sequence task may not start from this review.

This is intentionally not represented as an independent acceptance. The same
root execution session implemented earlier codec-spike work, so the required
implementation-independent approval is unsatisfied. The machine-readable
contract is
`acceptance/codec-spike/independent-supply-review-v1.json`; its validator
rejects any attempt to turn this artifact into an approval, select a winner,
close a high finding silently, or bypass the Phase 2 block.

## Evidence reviewed

The review pins the exact SHA-256 of the rubric, license audit, comparative
matrix and no-go handoff. The matrix preserves every raw failure and has no
aggregate score that could average away a mandatory gate.

| Combination | Exact blocking evidence | Disposition |
|---|---|---|
| Bundled FFmpeg 8.1.2 on both platforms | Candidate smoke decoding and hostile-input containment pass, but end-to-end range/long-duration evidence, Windows ARM64 release packaging, production signatures, macOS notarization, runtime SBOM, release-time vulnerability closure, corresponding-source publication and counsel decisions are absent. | Rejected for production. |
| Windows Media Foundation plus macOS native frameworks | Windows rejects both required Ogg/Opus fixtures with `0xC00D36C4`; the current macOS probe consumes the complete source before first PCM and one cold lifecycle row fails. Packages are test/ad-hoc signed, not production release evidence. | Rejected for production. |
| Pure Go on both platforms | The exact AAC dependency is GPL-2.0-only with unavailable origin and is intentionally excluded. MP3 seek construction scans the source and Ogg has no acceptable random-seek lifecycle. | Rejected. |

The current repository correctly permits candidate-neutral contracts and test
doubles to continue. It does not permit a production decoder, production
playback, promotion, or a claim that Phase 2 is accepted.

## Source review

- [FFmpeg legal guidance](https://ffmpeg.org/legal.html) keeps the bundled
  option conditional on the exact non-GPL configuration, shared linking,
  notices, corresponding source, build changes and relinking terms.
- [FFmpeg security advisories](https://ffmpeg.org/security.html) identify
  fixes included in 8.1.2. This does not replace a release-time scan of the
  exact produced binaries and complete runtime SBOM.
- [Microsoft's current codec table](https://learn.microsoft.com/en-us/windows/apps/develop/media-authoring-processing/supported-codecs)
  documents AAC-LC and MP3 support but does not establish the mandatory
  Ogg/Opus path. The recorded runtime failure therefore remains authoritative
  for this candidate.
- Apple requires the final nested code to be
  [distribution signed](https://developer.apple.com/documentation/xcode/creating-distribution-signed-code-for-the-mac)
  and the release artifact to satisfy
  [notarization requirements](https://developer.apple.com/documentation/security/notarizing-macos-software-before-distribution).
  The current ad-hoc probe proves neither.
- Apple's [App Review Guidelines](https://developer.apple.com/app-store/review/guidelines/)
  retain the self-contained/no-downloaded-code boundary; no runtime decoder
  download is an allowed fallback.
- [Via LA's AAC program](https://www.via-la.com/licensing-programs/aac/)
  confirms that AAC patent licensing applicability exists. Qualified counsel,
  not this engineering review, must decide the exact product, hosted
  conversion and market obligations.
- The [Opus license and patent grant](https://opus-codec.org/license/) is
  recorded, while its third-party-IP caveat remains part of legal review.
- The current [Go vulnerability tooling/database](https://vuln.go.dev/) found
  zero called vulnerabilities in the pure-Go probe. It also reported findings
  in imported/required code not reached by the probe. This narrow result does
  not approve the rejected candidate or scan an FFmpeg release binary.

No statement in this report is legal advice.

## Representative reruns

On 2026-07-16 the review reran all eight codec contract validators and all 19
codec contract tests. The tests include deterministic corrupt/truncated input,
range authorization/fault/redaction, bounded private cache, missing/failed
sample rejection, stale artifact rejection and false-selection rejection.

The pure-Go probe was rebuilt and executed locally, with additional
`darwin/arm64`, `windows/amd64` and `windows/arm64` cross-builds. It reproduced
`rejected-license-seek-and-manual-evidence-gates`. The sandboxed macOS
engineering probe was rebuilt, ad-hoc signed and executed; it reproduced
`rejected-full-file-and-lifecycle-gates`. Its receipt explicitly says
`productionSignature: not-proven`, `notarization: not-proven` and
`realHardwareClaim: false`.

```text
python3 scripts/codec_spike/validate_contract.py
python3 scripts/codec_spike/stream_contract.py --validate
python3 scripts/codec_spike/validate_license_audit.py
python3 scripts/codec_spike/validate_mf_probe.py
python3 scripts/codec_spike/validate_macos_native_probe.py
python3 scripts/codec_spike/validate_pure_go_probe.py
python3 scripts/codec_spike/validate_comparative_matrix.py
python3 scripts/codec_spike/validate_player_handoff.py
PYTHONDONTWRITEBYTECODE=1 python3 -m unittest -v scripts/codec_spike/test_codec_spike.py
bash scripts/codec_spike/run_pure_go_probe.sh .temp/review-2g3fkt/purego
bash scripts/codec_spike/build_macos_native_probe.sh .temp/review-2g3fkt/macos
(cd scripts/codec_spike/purego_probe && go run golang.org/x/vuln/cmd/govulncheck@latest ./...)
```

## Blocking findings and reopen rule

The machine-readable contract records six open high findings: no passing
combination, missing reviewer independence, incomplete release supply-chain
proof, unresolved AAC/LGPL legal decisions, native mandatory-gate failures and
the rejected pure-Go dependency/lifecycle set.

The review may be replaced only after one exact combination passes every hard
gate, all critical/high findings are fixed and re-reviewed, exact SBOM/source/
build/signing/notarization evidence exists, legal questions are resolved, and
an implementation-independent reviewer signs the exact candidate commit.
Real packaged-hardware runs remain exclusively in manual epic
`EPIC-260714-th54l3`; this report makes no real-hardware claim.
