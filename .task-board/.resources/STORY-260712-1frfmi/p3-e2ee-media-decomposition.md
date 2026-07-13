# P3 End-to-end Encrypted Media Decomposition

Story: `STORY-260712-1frfmi`

## Spec slices reviewed

- `docs/spec-self-contained-audio.md` sections 6.1, 8, 11.1-11.4, 12-15.5,
  18, 21.2-21.5, 22-23
- `docs/goal-self-contained-audio.md`
- `docs/spec.md` and `docs/protocol.md` for current shipped plaintext media,
  protocol, storage, and rollout constraints

## Current implementation snapshot

- `coordinator/internal/media/media.go`
  - Protected media does not exist yet; the coordinator still runs `ffmpeg`
    itself and emits plaintext WAV output.
- `coordinator/cmd/duet-coordinator/loop.go`
  - Telegram voice intake still lands in `media.Process(...)`, updates a
    plaintext `MediaRecord`, and sends a coordinator-owned `file_url` to
    nodes.
- `coordinator/internal/store/store.go`
  - Media persistence is still `path_wav` plus loudnorm metadata, and
    `GetMediaForOrbit` authorizes fetch by orbit or active approach rather than
    by encrypted recipient grants and epochs.
- `coordinator/cmd/duet-coordinator/main.go`
  - `/media/{id}` serves plaintext bytes to authenticated nodes; the
    coordinator is therefore still a decrypt-capable media host, not a
    ciphertext router.
- `node-app/Sources/NodeCore/VoiceCache.swift`
  - macOS downloads and caches plaintext WAVs keyed by `media_id`.
- `pulsar-win/voice.go`
  - Windows mirrors the same plaintext WAV fetch-and-cache model.
- `node-app/Sources/NodeCore/Keychain.swift`
  - macOS has secure storage only for pairing credentials today, not a
    dedicated group-key or history-grant store.
- `pulsar-win/config.go`
  - Windows still stores pairing credentials in plaintext JSON and carries a
    `TODO` for DPAPI hardening; there is no protected key store for E2EE media.
- `docs/protocol.md` and the Go or Swift or Windows codecs
  - There is no `e2ee_media_v1` capability, no epoch or grant vocabulary, and
    no report-evidence or history-grant contract.

## Task set

1. `TASK-260712-2e2ymn` Threat-model phase-three E2EE media boundaries and honest claims
   - Blocking design task for trust boundaries, attacker classes, metadata
     disclosure, product claims, and external-review entry criteria.
2. `TASK-260712-2ys1ww` Freeze E2EE media protocol, key lifecycle, and compatibility contract
   - Blocking contract task for `e2ee_media_v1`, manifests, recipient wraps,
     epochs, grants, recovery or transfer packages, and mixed-version rules.
3. `TASK-260712-3w1cst` Add encrypted media schema, epoch repositories, and feature-flag foundation
   - Additive storage and repository work for encrypted manifests, epochs,
     grants, transfers, report metadata, and audit state.
4. `TASK-260712-20j5tm` Implement coordinator ciphertext routing, target-key distribution, and Air rotation runtime
   - Turns the coordinator into a ciphertext router that rotates epochs on
     join, leave, and revoke and reuses target ACL, delete, retention, inbox,
     and history services without plaintext regressions.
5. `TASK-260712-1rziyo` Implement recovery, multi-device transfer, and history-grant key services
   - Builds explicit bootstrap, same-user transfer, recovery, and old-history
     grant flows with one-time or time-bound approval and audit.
6. `TASK-260712-2i0w6x` Implement voluntary report-evidence copy and moderation-safe export flow
   - Extends reports so a recipient may explicitly attach a decrypted copy
     without introducing hidden server decryption.
7. `TASK-260712-2nppt6` Implement macOS encrypted media encode, key storage, and decrypt playback
   - macOS local normalize or encode or encrypt packaging, secure grant and key
     storage, decrypt playback, and explicit grant or report consent surfaces.
8. `TASK-260712-2q4jbu` Implement Windows encrypted media encode, key storage, and decrypt playback
   - Windows local normalize or encode or encrypt packaging, DPAPI or
     Credential Locker key storage, decrypt playback, and explicit grant or
     report consent surfaces.
9. `TASK-260712-1bcpda` Prove C4-C6, publish the external review pack, and gate `e2ee_media` rollout
   - Final evidence pack for membership crypto, coordinator privacy, voluntary
     evidence copy, mixed-version behavior, and feature-flag or claim gating.

## Execution shape

- Blocking design first:
  - `TASK-260712-2e2ymn`
  - `TASK-260712-2e2ymn` -> `TASK-260712-2ys1ww`
- Shared foundation next:
  - `TASK-260712-2ys1ww` -> `TASK-260712-3w1cst`
  - `TASK-260712-2ys1ww` + `TASK-260712-3w1cst`
    -> `TASK-260712-20j5tm`
  - `TASK-260712-2ys1ww` + `TASK-260712-3w1cst`
    -> `TASK-260712-1rziyo`
- Reporting after the router is real:
  - `TASK-260712-2e2ymn` + `TASK-260712-2ys1ww` +
    `TASK-260712-3w1cst` + `TASK-260712-20j5tm`
    -> `TASK-260712-2i0w6x`
- Client paths after shared control-plane work:
  - `TASK-260712-2ys1ww` + `TASK-260712-3w1cst` +
    `TASK-260712-20j5tm` + `TASK-260712-1rziyo` +
    `TASK-260712-2i0w6x`
    -> `TASK-260712-2nppt6`
  - `TASK-260712-2ys1ww` + `TASK-260712-3w1cst` +
    `TASK-260712-20j5tm` + `TASK-260712-1rziyo` +
    `TASK-260712-2i0w6x`
    -> `TASK-260712-2q4jbu`
- Final proof last:
  - `TASK-260712-20j5tm` + `TASK-260712-1rziyo` +
    `TASK-260712-2i0w6x` + `TASK-260712-2nppt6` +
    `TASK-260712-2q4jbu`
    -> `TASK-260712-1bcpda`

## Gaps closed explicitly

- The authoritative spec names the P3-E2EE outcome, but does not freeze the
  exact threat model, metadata disclosure, or claim boundary.
  `TASK-260712-2e2ymn` closes that gap and blocks every downstream contract.
- The spec requires group-key lifecycle, history grants, recovery, and
  voluntary evidence copy, but does not choose the wire or storage contract.
  `TASK-260712-2ys1ww` closes that gap before persistence or client work.
- The current codebase assumes coordinator-side `ffmpeg` plus plaintext WAV
  fetch. The decomposition makes local normalize or encode or encrypt a
  first-class client responsibility instead of leaving it as an implicit
  implementation detail.

## Completeness check

- Covered:
  - threat model and honest E2EE product claims
  - protocol, epoch, manifest, and compatibility contract
  - encrypted media persistence, grants, transfers, and audit metadata
  - coordinator ciphertext routing and Air-driven rotation
  - recovery, multi-device transfer, and explicit history grants
  - voluntary decrypted report evidence with moderation-safe export
  - macOS and Windows protected-media client paths
  - C4-C6 evidence, external review packet, and feature-flag gating
- Intentionally not re-owned here:
  - live chunk transport and hold-to-talk semantics from `STORY-260712-sskhip`
  - AEC, noise suppression, AGC, and capture diagnostics from
    `STORY-260712-3pt00e`
  - soundboard, schedules, webhook or local automation policy from
    `STORY-260712-326wd5`
  - phase-wide beta soak, disclosure refresh, and final external-review
    closure from `STORY-260712-2ft5wd`
- Diagrams attached:
  - `docs/diagrams/p3-e2ee-media-components.puml`
  - `docs/diagrams/p3-e2ee-media-sequence.puml`
