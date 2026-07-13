# Independent Phase 1 protocol and compatibility review

## Description
Have a non-implementing reviewer verify the canonical wire contract, scheduler and every downgrade or legacy seam.

## Scope
Diff Go, Windows and Swift codecs and golden files; inspect version and capability negotiation, target and receipt state machines, trusted ordering, clock conversion and barrier math, one playback-domain FIFO, duplicates and reconnect, cancellation and sender delete, overlay versus interrupt confirmation, after_current bridge, legacy play_voice and Telegram default races. Re-run cross-codec, mixed-version and failure-injection suites and trace all protocol changes to docs/spec-self-contained-audio.md.

## Acceptance Criteria
An independent report proves every message and enum is identical across mirrors, mixed versions follow explicit policy, clocks and idempotency cannot late-autoplay or duplicate, and legacy behavior remains green. Critical or high findings are closed and re-reviewed before root acceptance.
