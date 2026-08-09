# BUG-260728-3mx5hm review verdict — changes requested

Timestamp: 2026-07-28T04:53:37+04:00

## Verdict

Changes requested; route to development. The typed Core-to-SwiftUI failure boundary, EN/RU actionable alert, explicit consent gate, redacted diagnostics, bounded retry policy, and partial-draft cleanup are directionally sound. Independent focused tests, full tests, lint, whitespace validation, and release build are green. One startup lifecycle defect and one required behavioral-coverage gap prevent acceptance.

## Blocking finding 1 — failed prepared attempts do not release AVAudioEngine resources

MacAVAudioCaptureBackend.swift lines 246-253 calls engine.stop() only when engine.isRunning, then removes the tap and calls engine.reset(). Both a post-prepare layout mismatch and AVAudioEngine.start() failure occur after engine.prepare() while isRunning is false. The installed Xcode 26.5 SDK contract in AVAudioEngine.h lines 319-386 states that prepare preallocates resources, reset only resets nodes, and stop releases resources allocated by prepare. Apple documentation states the same: https://developer.apple.com/documentation/avfaudio/avaudioengine

Therefore the second and later attempts can retain the stale VPIO/default-duplex resources from the failed aggregate that the retry is intended to recover from. The same stale prepared state is carried into setVoiceProcessingEnabled(false) before the consented fallback. This is especially material for the production receipt where the aggregate became valid only after start had already failed.

Required rework: after every failed prepared/start attempt, enter a documented fully stopped resource state before retry or consented fallback. Calling stop unconditionally before reset is the smallest likely correction; recreating the engine/I/O node per attempt is an alternative if required by platform behavior. Preserve tap/mailbox cleanup and the 125 ms bound. Add a deterministic backend lifecycle seam/test proving release/stop occurs after a prepared-but-not-running failure and before the next prepare/start and before disabling VPIO for fallback.

## Blocking finding 2 — cancel, retry, and reset are not behaviorally covered at the production composition boundary

MacCaptureWorkflowControllerTests.applicationConsentBoundary only searches source text for method names. MacCaptureOneGenerationConsentTests exercises isolated flags, but no executable test drives quality rejection -> prompt -> cancel or allow -> same-journey retry -> quality generation close -> consent reset. Thus the task checklist claim that cancel, retry, and deferred consent reset are covered is stronger than the evidence.

Required rework: extract an injectable consent coordinator/reducer or make MacCaptureAppComposition testable, then assert built-in-speaker rejection prompts, accepted headphone capture bypasses the prompt, cancel performs no retry and remains fail-closed, allow performs exactly one retry, quality-generation close clears consent including the busy/deferred-reset path, and a consented terminal fallback failure cannot re-prompt.

## Independent validation

- Focused Swift tests: 44 tests in 5 suites passed, exit 0.
- Full node-app Swift suite: 370 tests in 59 suites passed, exit 0.
- Strict task-owned swift-format lint: exit 0.
- git diff --check over task paths: exit 0.
- Optimized swift build -c release: exit 0.
- Swift 6.3.2 / Xcode 26.5 toolchain.

## Non-blocking review results

- No raw capture_captureQualityUnsupported or capture_backendUnavailable string remains on the normal recording presentation path.
- Native alert buttons and text provide localized EN/RU, non-color, VoiceOver-readable actions. Apple documents that alert actions run before automatic dismissal, so the dismissal binding does not override the selected action.
- Fallback remains explicit-consent-only and unrelated errors remain fail-closed in the policy model.
- Diagnostics contain stage, bounded timing, status/domain/code, and channel/rate metadata only; no device ID/name, audio, credential, invite, or payload data.

Unrelated dirty-worktree changes were not reviewed as part of this verdict.