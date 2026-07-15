# P2 codec license and distribution audit

Status: accepted engineering audit, 2026-07-15

Owner and approver: Ivan Oparin

Scope: exact candidates frozen by `p2-codec-spike-rubric.v1`

Legal boundary: this is a technical shipping gate, not legal advice

## Decision

| Candidate | Decision | Exact shipping shape | Blocking obligations |
|---|---|---|---|
| `native-canonical-aac-v1` | shippable with obligations | Inbox Media Foundation on Windows, inbox AudioToolbox on macOS, exact minimal FFmpeg 8.1.2 only in the server conversion image | runtime capability probe, supported OS servicing, exact FFmpeg compliance bundle, AAC counsel approval |
| `pure-go-composite-v1` | rejected | Would statically combine four Go modules into proprietary client packages | exact `go-aac` is GPL-2.0-only, says it is a FAAD2 port, and its origin repository was unavailable; no proprietary static-combination assumption is permitted |
| `bundled-ffmpeg-8.1.2-v1` | shippable with obligations | Package-local shared libraries plus a narrow in-process bridge; no CLI | minimal reproducible LGPL build, exact corresponding source and notices, signed nested binaries, notarization, fresh CVE/SBOM scan, AAC counsel approval |

The machine-readable disposition, exact commits, Go sums, source/license SHA-256
values, transitive classification and release gates are frozen in
`acceptance/codec-spike/license-audit-v1.json`. The validator rejects a missing
license tripwire, an unknown dependency, candidate drift, a runtime download,
sandbox weakening or removal of a release gate.

## Native framework boundary

Microsoft documents inbox desktop support for MP3 and AAC, but also says apps
must query runtime availability where device support can vary. The client must
therefore probe the documented Media Foundation path, retain its declared MSIX
trust level and canonicalize an unsupported source on the server. It may not
download a decoder, copy an OS codec DLL, or widen capabilities as fallback.

Apple documents Audio File Stream Services for incremental MP3 and AAC parsing.
The macOS client uses only documented AudioToolbox/AVFoundation interfaces,
retains App Sandbox and treats a format rejection as a normal unsupported path.
No OS codec binary is copied into the app.

Primary references:

- Microsoft, [Supported codecs](https://learn.microsoft.com/en-us/windows/apps/develop/media-authoring-processing/supported-codecs) and [MSIX containerization overview](https://learn.microsoft.com/en-us/windows/msix/msix-containerization-overview), retrieved 2026-07-15.
- Apple, [Audio File Stream Services](https://developer.apple.com/documentation/audiotoolbox/audio-file-stream-services) and [Protecting user data with App Sandbox](https://developer.apple.com/documentation/security/protecting-user-data-with-app-sandbox), retrieved 2026-07-15.

Using an inbox implementation avoids redistributing that implementation; it is
not a conclusion that every codec patent obligation is already covered for our
product, territories or server conversion use.

## Exact Go inventory

The audit downloaded every exact version through the Go module proxy, retained
the module and `go.mod` sums, hashed the source zip and license text, and traced
the selected runtime packages. The selected package paths use only their own
module plus the Go standard library. Dependencies declared only for examples or
tests are recorded separately and must not silently enter a shipping SBOM.

| Component | License | Runtime transitives in selected package | Maintenance decision |
|---|---|---|---|
| `go-mp3@v0.3.4` | Apache-2.0 | none | individually usable with notice and security ownership, but upstream is archived |
| `go-aac@5f2857e…` | GPL-2.0-only | none | rejected; FAAD2-derived, origin unavailable, no acceptable proprietary-static or maintenance assumption |
| `mp4ff@v0.54.0` | MIT | none | individually usable with notice and release scanning |
| `pion/opus@v0.1.0` | MIT | none | individually usable with notice, release scanning and counsel review of disclosed Opus IPR |

The Go Vulnerability Database module index had no entry for these four exact
module paths on 2026-07-15. That is a time-bounded query result, not evidence
that the code is safe or will remain free of vulnerabilities. Every release
must rescan exact source reachability and the produced binary.

Primary references:

- Immutable source/version records: [Go module proxy](https://proxy.golang.org/), exact paths and hashes in the JSON audit.
- Vulnerability data: [Go Vulnerability Database index](https://vuln.go.dev/index/modules.json), retrieved 2026-07-15.
- Opus project, [software and patent license disclosures](https://opus-codec.org/license/), retrieved 2026-07-15. The page itself records both royalty-free grants and potentially royalty-bearing third-party disclosures, so counsel review remains mandatory.

## FFmpeg build and LGPL gate

Only FFmpeg 8.1.2 source SHA-256
`464beb5e7bf0c311e68b45ae2f04e9cc2af88851abb4082231742a74d97b524c`
may be used. The accepted shape is shared libraries only. The floor is:

```text
--disable-everything --disable-autodetect --disable-programs --disable-doc
--disable-network --disable-static --enable-shared
```

The following flags are hard failures:

```text
--enable-gpl --enable-version3 --enable-nonfree
```

The build may expose only the locked MP3/AAC/Opus decoder, MP3/AAC/MOV/Ogg
demuxer, file protocol and resampling surface required by the spike. Configure
output, compiler/linker inputs, final import tables and SBOM are release
artifacts; `--disable-autodetect` is not permission to skip checking them.

FFmpeg's own compliance page recommends dynamic linking and distribution of
exact corresponding source, modifications and build instructions. The release
therefore publishes the exact source tarball, patches, configure command, full
LGPL-2.1 text and attribution alongside the product. Counsel must confirm the
final relinking and reverse-engineering language before Store submission.

Primary references:

- FFmpeg, [License and Legal Considerations](https://ffmpeg.org/legal.html), retrieved 2026-07-15.
- FFmpeg, [Security](https://ffmpeg.org/security.html), retrieved 2026-07-15. The 8.1.2 section lists fixes for CVE-2026-8461 and CVE-2026-30999; future advisories still block a stale binary.

## Patent boundary

Software copyright licenses and codec patent rights are tracked separately.

- Via Licensing Alliance still administers an [AAC patent program](https://www.via-la.com/licensing-programs/aac/) as of the retrieval date. Whether native playback, client distribution, hosted encoding, user-generated content and each intended market fall within a license is an explicit counsel decision.
- Fraunhofer states that the former Technicolor/Fraunhofer [MP3 licensing program terminated in 2017](https://www.iis.fraunhofer.de/en/ff/amm/consumer-electronics/mp3.html). This is recorded as a primary-source fact, not a universal non-infringement opinion.
- The Opus project publishes royalty-free grants and also identifies third-party IPR disclosures. That mixed primary-source record is why the audit says `legal-review-required`, never simply “patent free.”

Until counsel approves AAC scope, a candidate may pass engineering experiments
but cannot pass the commercial release gate.

## Store, signing, update and incident rules

Windows DLLs are immutable MSIX package members for both `amd64` and `arm64`.
They are built and scanned before package signing and cannot be replaced or
downloaded on first run. The package receipt records trust level, architecture,
hash and import table.

macOS dylibs live in `Contents/Frameworks`, are signed from the inside out, and
the outer `arm64` app is signed and notarized. Apple's current review rules
require a self-contained app and prohibit downloaded executable functionality.

Primary references:

- Apple, [Creating distribution-signed code for macOS](https://developer.apple.com/documentation/xcode/creating-distribution-signed-code-for-the-mac), [Notarizing macOS software](https://developer.apple.com/documentation/security/notarizing-macos-software-before-distribution), and [App Review Guidelines](https://developer.apple.com/app-store/review/guidelines/), retrieved 2026-07-15.

A release is blocked when any of these is absent: exact build receipt, complete
runtime SBOM, zero known unpatched findings, matching notices/corresponding
source, signed Windows architecture matrix, signed/notarized macOS artifact,
sandbox/no-download receipt, or AAC counsel approval. A newly disclosed issue
triggers rebuild/retest or candidate withdrawal; a binary is never silently
grandfathered.

## Handoff to the remaining spike

`TASK-260712-1canzv` may prototype only the audited minimal shared FFmpeg shape.
`TASK-260712-298tyq` and `TASK-260712-350u8d` may prototype the native paths.
`TASK-260712-3vkcki` must record `pure-go-composite-v1` as rejected and may use
individual permissive modules only for bounded research that cannot become a
shipping artifact without reopening this audit. The final ADR must preserve
these dispositions and may tighten them; it may not relax them based only on
performance evidence.
