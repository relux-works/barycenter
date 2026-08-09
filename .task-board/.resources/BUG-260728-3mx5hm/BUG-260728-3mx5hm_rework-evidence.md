# BUG-260728-3mx5hm — reviewer rework evidence

Timestamp: 2026-07-28T05:11:29+04:00

## Reviewer findings resolved

1. Failed prepared/start attempts now enter a fully stopped state before retry
   or consented fallback. The production cleanup sequence is unconditional
   `AVAudioEngine.stop()`, tap removal, engine reset, then mailbox reset.
2. `MacCaptureStartupSequencer` is used by the production backend and permits
   fallback only after the failed attempt has been released. Existing bounded
   policy remains four attempts in 125 ms with 25/35/45 ms delays, and only an
   explicitly consented initial VPIO failure is eligible.
3. `MacCaptureAppComposition` now uses `MacCaptureConsentCoordinator` for the
   one-generation journey instead of composing flag operations inline.
   Executable tests drive prompt, cancel, retry, terminal failure, generation
   close, and deferred reset behavior.

## Deterministic regression coverage

- prepared-but-not-running failure releases resources before the next prepare;
- terminal VPIO release occurs before disabling processing and fallback start;
- built-in speaker rejection offers the quality prompt;
- accepted headphone capture does not prompt;
- cancel selects headphones, grants no consent, and triggers no retry;
- allow produces exactly one retry with degraded consent;
- generation close revokes the grant immediately and retains a busy reset until
  the production workflow can apply it;
- an eligible initial VPIO failure offers fallback;
- a consented terminal fallback failure cannot prompt again;
- existing typed diagnostics, bounded retry, localization, and no-partial-draft
  tests remain green.

## Validation receipts

Every gate was run directly as a standalone process; no `tee` or status-hiding
pipeline was used.

| Gate | Result | Exit |
| --- | --- | ---: |
| Initial executable rework check | 19 tests / 3 suites passed | 0 |
| Focused Swift gate | 52 tests / 6 suites passed | 0 |
| Full NodeApp Swift suite | 378 tests / 60 suites passed | 0 |
| Strict task-owned `swift-format lint` | no diagnostics | 0 |
| Scoped `git diff --check` | no diagnostics | 0 |
| Optimized `swift build -c release` | NodeApp linked | 0 |
| Raw internal-string `rg` scan | no matches (expected no-match status) | 1 |

The release build reports only the already-documented Sendable warnings in
`PlayerCore.swift` and payload types in `Protocol.swift`; no warning points to
the rework files.

## Attached logs

- `BUG-260728-3mx5hm_rework-focused-tests.log`
- `BUG-260728-3mx5hm_rework-full-swift-tests.log`
- `BUG-260728-3mx5hm_rework-release-build.log`
