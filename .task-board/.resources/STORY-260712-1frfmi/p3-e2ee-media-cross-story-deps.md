# P3 End-to-end Encrypted Media Cross-story Dependencies

Story: `STORY-260712-1frfmi`

## Required upstream foundations

- `STORY-260712-2ve1c8` P1 identity and self-service onboarding
  - Supplies the actor model, role checks, recovery-secret rules, and secure
    Windows or macOS credential posture that E2EE recovery, device transfer,
    and history-grant services must extend instead of replacing.
- `STORY-260712-ld674h` P1 generic media ingest and storage
  - Supplies upload sessions, media-item lifecycle, delete or retention hooks,
    and storage publication seams that protected media must reuse after
    replacing coordinator-side normalization with local preparation.
- `STORY-260712-25lysg` P1 transmission protocol and scheduler
  - Supplies capability negotiation, immutable target snapshots, prepare or
    play scheduling, partial-delivery semantics, and transport-contract hygiene
    that the encrypted manifest and recipient-wrap contract extends.
- `STORY-260712-3v14m9` P2 Air rooms and approach migration
  - Supplies stable Air identity, join or leave or revoke lifecycle, and
    runtime membership truth that drive epoch rotation.
- `STORY-260712-ob1tx2` P2 explicit targets, inbox and transport parity
  - Supplies target-snapshot ACLs, inbox or history or replay semantics, and
    the rule that only explicit targets may discover media, which becomes the
    authorization basis for wrapped keys and manifest access.
- `STORY-260712-2ori1t` P2 streamed user audio tracks
  - Supplies the phase-two `audio_track` storage and playback substrate that
    E2EE must protect in addition to short clips.
- `STORY-260712-3l1r1u` P2 codec and streaming player spike
  - Supplies the approved decode or streaming path for protected long-track
    playback, and should inform local encode or container choices instead of
    creating an unreviewed second media format.

## Downstream consumers

- `STORY-260712-2ft5wd` P3 security acceptance and rollout
  - Consumes the threat model, C4-C6 evidence, external review pack, residual
    risk log, and exact feature-flag or claim gating produced here.
- `STORY-260712-34kbkn` P1 Telegram adapter, history and presence
  - Consumes the shared protected-media report and history contract for parity
    surfaces, but must not redefine the encrypted-media rules itself.

## Parallel siblings, not blockers

- `STORY-260712-sskhip` P3 near-live push-to-talk
  - This E2EE story must not silently absorb live chunk encryption or hold
    transport semantics. Any future live-PTT crypto reuse should build on the
    phase-three media contract explicitly.
- `STORY-260712-3pt00e` P3 capture quality and diagnostics
  - E2EE claims do not substitute for AEC, noise suppression, AGC, or route
    diagnostics, and that story does not own key lifecycle or ciphertext
    routing.
- `STORY-260712-326wd5` P3 soundboard and safe automation
  - Automation may consume protected-media send APIs later, but this story must
    not absorb schedules, token scope, or webhook controls.

## Boundary decisions kept explicit

- The coordinator may store ciphertext, manifests, recipient-wrap metadata, and
  audit state, but it may not gain a silent decrypt path.
- Recovery restores current authorized access only; historical protected media
  remains behind explicit history grants.
- Report handling stays compatible with the existing moderation model, but any
  decrypted evidence copy must be created deliberately by a recipient device and
  audited as a separate action.
- Product claims remain feature-gated: phase-three may ship live PTT without
  claiming E2EE if this story's proof or review pack is incomplete.
