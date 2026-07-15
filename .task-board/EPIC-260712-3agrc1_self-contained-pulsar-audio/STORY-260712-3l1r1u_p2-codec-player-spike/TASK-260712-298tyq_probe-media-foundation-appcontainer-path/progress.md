## Status
done

## Assigned To
codex-inline-developer

## Created
2026-07-12T16:11:49Z

## Last Update
2026-07-15T20:28:51Z

## Blocked By
- TASK-260712-14u0yk
- TASK-260712-dqdoqj
- TASK-260712-6kba80
- TASK-260712-13rbnw

## Blocks
- TASK-260712-ibuaxj

## Checklist
- [x] Reuse the phase-one signed packaging posture instead of inventing a new Windows distribution baseline
- [x] Integrate a Media Foundation or WinRT-backed decode path with range-backed prepare and bounded buffering
- [x] Prove MP3 AAC and Opus decode plus scheduled start, pause, seek, and resume under AppContainer rules
- [x] Measure two-hour RSS and record COM, thread, manifest, or capability assumptions
- [x] Publish a pass or reject note with exact package and run steps plus concrete failure evidence if rejected
- [x] Test actual Opus containers and OS builds rather than assuming Media Foundation support
- [x] Prove no developer-mode, runFullTrust or render-thread dependency

## Notes
2026-07-15 strict-sequence start: implementing inline outside task-board spawn workflow. Scope is best-effort code plus automated unit/integration/hosted Windows evidence. Physical Win10/Win11 hardware validation remains in the dedicated manual-test epic and will not be claimed here. Reusing the accepted Phase 1 signed MSIX/AppContainer baseline and frozen codec fixtures.
Accepted after engineering PR #110 merged as 6cb817d (engineering head 4083e5b). Dedicated hosted Windows run 29447847569 passed: signed x64 and ARM64 AppContainer MSIX packages; AO_NONE AUMID activation with debug disabled; direct packaged launch self-reported an AppContainer token; MP3 CBR/VBR and AAC M4A/ADTS decoded with scheduled start, pause-without-read, seek/new generation, resume and drain; exact Ogg/Opus CBR/VBR rejection was 0xC00D36C4 (MF_E_UNSUPPORTED_BYTESTREAM_TYPE). Sixty-second hosted soak completed 2,214 iterations with 21,970,944-byte start RSS, 24,764,416-byte end RSS and 24,805,376-byte peak; max underlying read was 262,144 bytes. Final standard CI run 29448173596 passed 4/4, and local repository acceptance passed 12/12. The probe supports a requested soak up to 7,200 seconds, but a physical two-hour run and Win10/Win11 hardware matrix remain explicitly unclaimed in manual-test epic EPIC-260714-th54l3. Shipping candidate is rejected: Media Foundation does not accept the frozen Ogg/Opus fixtures, so selecting this path requires canonical AAC/M4A conversion plus later manual hardware evidence. No developer mode, runFullTrust, network-owned decoder, WASAPI callback, render-thread work or debug activation is used.

## Precondition Resources
(none)

## Outcome Resources
- [media-foundation-probe-v1.json](file://TASK-260712-298tyq/media-foundation-probe-v1.json) — Fail-closed AppContainer package, fixture, lifecycle and acceptance contract
- [p2-media-foundation-appcontainer-probe.md](file://TASK-260712-298tyq/p2-media-foundation-appcontainer-probe.md) — Engineering result, reproduction steps, metrics and shipping decision
- [repository-acceptance-manifest.json](file://TASK-260712-298tyq/repository-acceptance-manifest.json) — Local repository acceptance manifest; all 12 commands passed
- [receipt-windows-media-foundation.json](file://TASK-260712-298tyq/receipt-windows-media-foundation.json) — Hosted x64 and ARM64 signed AppContainer activation, fixture and soak receipt
