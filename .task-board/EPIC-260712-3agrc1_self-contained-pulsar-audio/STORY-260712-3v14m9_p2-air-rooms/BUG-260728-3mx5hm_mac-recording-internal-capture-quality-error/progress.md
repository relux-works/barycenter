## Status
done

## Review
required

## Task Class
code

## Estimate
estimated(fibonacci(3))

## Blocked By
- (none)

## Blocks
- (none)

## Checklist
- [x] Reproduce built-in-speaker Record failure and preserve a redacted timestamped receipt
- [x] Replace the raw capture_captureQualityUnsupported surface with localized actionable quality guidance
- [x] Allow explicit one-generation degraded consent to retry the recording without weakening fail-closed defaults
- [x] Cover built-in speaker, accepted headphone route, cancel, retry, and consent reset
- [x] Run focused Swift tests, full relevant suite, format/lint/build, and attach outcome evidence
- [x] Reviewer accepts safety, localization, accessibility, architecture, and regression coverage
- [x] Harden macOS VPIO/default-duplex aggregate startup against CoreAudio error 35 race and channel-layout churn; retry only within a bounded safe startup window or fall back through explicit consent
- [x] Add deterministic regression coverage for late-valid aggregate startup, repeated validation mismatch, typed diagnostics, and no partial draft on terminal failure
- [x] Code written per task description and AC
- [x] Relevant tests written for new or changed behavior and passing
- [x] Lint clean
- [x] Relevant build/validation commands run after changes and build not broken
- [x] New outcome artifact attached on the board with a task-scoped name when the work produces notes, logs, screenshots, or other deliverables
- [x] Important findings, decisions, anomalies, or regressions recorded in logbook when relevant
- [x] Implementation matches AC
- [x] Solution fits project architecture
- [x] Tests green
- [x] If review does not accept the work — verdict evidence added and status routed by the explicit verdict branches

## Notes
2026-07-28 production reproduction: /Applications/Pulsar.app PID 3423, default input MacBook Pro Microphone at 48kHz, default output MacBook Pro Speakers at 44.1kHz. Toolbar Record briefly changed UI then reported capture_captureQualityUnsupported. Source trace: MacCaptureAppComposition always requests processing with degradedConsent=false; MacCaptureQualityDecision classifies speaker as degraded/reference_unavailable or unavailable processing; MacAVAudioCaptureBackend throws captureQualityUnsupported when degraded consent is absent. No transmission or draft was created. Existing Settings/Try Locally toggle Allow this limited recording is a safe immediate workaround but is undiscoverable from the primary failure.
2026-07-28 follow-up after user enabled one-generation degraded consent: primary toolbar Record now advances past capture-quality gating but fails as raw capture_backendUnavailable. Production defaults contain no captureInputDevice.v1 override, so stale selected-input configuration is ruled out; system default is MacBook Pro Microphone at 48 kHz and remains enumerated. TCC denial is unlikely because the engine maps denied/restricted to distinct typed errors before backend start. Failure is narrowed to MacAVAudioCaptureBackend after quality-session creation: invalid native format, AVAudioEngine.start failure, or AudioUnit current-device failure; selectedDeviceID is nil, making the last branch inapplicable. Existing logs do not retain the underlying CoreAudio error, so the raw cause is currently destroyed by catch-to-backendUnavailable mapping. This is part of the same end-user recording blocker and needs actionable localization plus underlying OSStatus/AVAudioEngine error diagnostics without sensitive audio persistence.
CoreAudio receipt for the 03:46:57-03:46:59 production failure: NodeApp enables VoiceProcessor and asks CoreAudio for CADefaultDeviceAggregate using built-in output (2 ch, 44.1 kHz) plus built-in input (1 ch). com.apple.coreaudio:ddagg repeatedly reports error fetching default pair and mInput.streamChannelCounts/totalChannelCount mismatch; HALC_ProxyIOContext::_StartIO then fails with error 35. CoreAudio reports Built valid aggregate only about 11 ms after the application start call has already failed. This confirms a duplex voice-processing aggregate startup race/channel-layout integration failure. The current UI cannot disable processingRequested: MacCaptureAppComposition passes true for every quality mode, so degraded consent does not provide an unprocessed fallback.
Second production reproduction at 03:52:45-03:52:46 with explicit captureInputDevice.v1=92 (MacBook Pro Microphone) and the one-generation degraded-consent toggle visibly enabled. CoreAudio created CADefaultDeviceAggregate-13584-0 from device 92 input and device 85 output, failed validation through retry 9/10 with input stream-channel mismatch, then HALC StartIO failed with error 35 at 03:52:46.518. A valid aggregate 314 was built at 03:52:46.541, approximately 23 ms after failure. No CaptureMedia file/draft was finalized. Explicit input selection and manual retry are therefore ruled out as user workarounds.
spawn agent resolution: Agent selection: codex via explicit_override
spawn launch composition: degraded_contract_unavailable; contract=agents-infra.child-launch-composition; provider=codex; schema=1; diagnostic=composition_contract_unavailable; bare child launch retained
spawn queued: [implementer] developer (codex) (run=RUN-260727-2b40a2, max_parallel=20)
spawn run started: [implementer] developer (codex) (run=RUN-260727-2b40a2)
2026-07-28 developer handoff: implemented typed Core-to-SwiftUI recording failures, native EN/RU actionable quality prompt, one-generation consent retry/reset, and bounded macOS VPIO aggregate startup recovery (4 attempts/125 ms; 25/35/45 ms) limited to CoreAudio-domain status 35 or duplex layout churn. Only an initial VPIO failure can enter consented fallback; unrelated/input-selection/already-fallback errors remain fail-closed. Terminal failure leaves no partial draft. Gates: focused Swift 44 tests/5 suites exit 0; full Swift 370 tests/59 suites exit 0; strict task-owned swift-format exit 0; git diff check exit 0; release NodeApp build exit 0 (pre-existing PlayerCore/Protocol sendability warnings only). Attached BUG-260728-3mx5hm_implementation-evidence.md plus focused/full/build logs. LOGBOOK entry 0435 records decisions and evidence. Reviewer acceptance checklist remains intentionally unchecked for independent review.
Workflow note: checklist item 6 (Reviewer accepts safety, localization, accessibility, architecture, and regression coverage) remains intentionally unchecked because no independent reviewer has acted yet. The handoff facade requires every checklist item even for the developer-to-review transition, which conflicts with that ownership boundary. Route status is therefore being set explicitly to to-review; the required handoff facade is still invoked as the last board command and must not be made green by falsely asserting reviewer acceptance.
agent completed: [implementer] developer (codex) (exit=0)
spawn run completed: codex (run=RUN-260727-2b40a2, pid=28604, exit=0)
spawn agent resolution: Agent selection: codex via explicit_override
spawn launch composition: degraded_contract_unavailable; contract=agents-infra.child-launch-composition; provider=codex; schema=1; diagnostic=composition_contract_unavailable; bare child launch retained
spawn queued: [reviewer] reviewer (codex) (run=RUN-260728-7d8125, max_parallel=20)
spawn run started: [reviewer] reviewer (codex) (run=RUN-260728-7d8125)
2026-07-28 reviewer verdict: CHANGES REQUESTED. MacAVAudioCaptureBackend retry cleanup skips AVAudioEngine.stop() whenever a prepared/start attempt fails with isRunning=false, although the SDK contract says stop releases prepare resources and reset only resets nodes; stale VPIO/default-duplex resources can survive into retry/fallback. Also, cancel/retry/deferred-reset coverage is source-string plus isolated helper testing rather than executable production-composition behavior. Independent validation remains green: focused 44/5, full 370/59, strict task-owned swift-format, git diff check, and release build all exit 0. See BUG-260728-3mx5hm_review-verdict-changes-requested.md.
agent completed: [reviewer] reviewer (codex) (exit=0)
spawn run completed: codex (run=RUN-260728-7d8125, pid=62164, exit=0)
spawn agent resolution: Agent selection: codex via explicit_override
spawn launch composition: degraded_contract_unavailable; contract=agents-infra.child-launch-composition; provider=codex; schema=1; diagnostic=composition_contract_unavailable; bare child launch retained
spawn queued: [implementer] developer (codex) (run=RUN-260728-3190d5, max_parallel=20)
spawn run started: [implementer] developer (codex) (run=RUN-260728-3190d5)
2026-07-28 developer rework handoff: resolved both reviewer blockers. Failed prepared/start attempts now unconditionally stop AVAudioEngine before tap removal/reset; the production startup sequencer proves release occurs before retry and before VPIO disable/fallback. MacCaptureAppComposition now uses an executable-tested consent coordinator covering built-in speaker, accepted headphones, cancel/no retry, exactly one consented retry, deferred generation reset, and no terminal fallback re-prompt. Gates: initial 19 tests/3 suites exit 0; focused 52/6 exit 0; full 378/60 exit 0; strict swift-format exit 0; scoped git diff check exit 0; release build exit 0 with only pre-existing PlayerCore/payload Sendable warnings. Attached BUG-260728-3mx5hm_rework-evidence.md and focused/full/build logs; LOGBOOK entry 0511 added. Reviewer acceptance checklist item remains intentionally unchecked pending independent review.
agent completed: [implementer] developer (codex) (exit=0)
spawn run completed: codex (run=RUN-260728-3190d5, pid=70599, exit=0)
spawn agent resolution: Agent selection: codex via explicit_override
spawn launch composition: degraded_contract_unavailable; contract=agents-infra.child-launch-composition; provider=codex; schema=1; diagnostic=composition_contract_unavailable; bare child launch retained
spawn queued: [reviewer] reviewer (codex) (run=RUN-260728-041ab3, max_parallel=20)
spawn run started: [reviewer] reviewer (codex) (run=RUN-260728-041ab3)
2026-07-28 independent reviewer verdict: ACCEPTED after rework. The AVAudioEngine prepared-attempt resource release and executable one-generation consent coverage resolve both prior blockers. Fresh validation: focused 52/6, full 378/60, strict task-scoped swift-format, scoped git diff check, raw internal-string scan, and optimized release build all pass. Safety, localization, accessibility, architecture, privacy/redaction, and regression coverage accepted. See BUG-260728-3mx5hm_review-verdict-accepted.md.
agent completed: [reviewer] reviewer (codex) (exit=0)
spawn run completed: codex (run=RUN-260728-041ab3, pid=83368, exit=0)
2026-07-28 installed accepted capture fix on Mac as production-signed Pulsar 0.3.0 (958.2), NodeApp sha256 4efb3069...e98a2. Strict deep signature, Team ID 262RZ595FP, hardened runtime, mic entitlement, installed/candidate hash, launch PID 90081, TLS/runtime health passed. Previous app recoverable at /Applications/Pulsar.app.backup-20260728-capturefix. See BUG-260728-3mx5hm_macos-install-receipt.md. Real capture and Windows audibility still require user verification.
2026-07-29 real installed smoke exposed a distinct architecture bug after this item was accepted: ordinary clip capture still enters VPIO/default-duplex and fails with disappearing aggregate OSStatus !dev despite a healthy built-in mic. Tracked separately as BUG-260729-3kecpr; this prior item remains historical evidence for the narrow consent/retry fix.

## Precondition Resources
(none)

## Outcome Resources
- [BUG-260728-3mx5hm_spawn-log_-implementer--developer--codex-_RUN-260727-2b40a2.log](file://BUG-260728-3mx5hm/BUG-260728-3mx5hm_spawn-log_-implementer--developer--codex-_RUN-260727-2b40a2.log) — System spawn log captured by task-board
- [BUG-260728-3mx5hm_implementation-evidence.md](file://BUG-260728-3mx5hm/BUG-260728-3mx5hm_implementation-evidence.md) — Developer implementation, safety decisions, coverage, and exit-code evidence
- [BUG-260728-3mx5hm_focused-tests.log](file://BUG-260728-3mx5hm/BUG-260728-3mx5hm_focused-tests.log) — Focused macOS capture and SwiftUI regression test output: 44 tests in 5 suites, exit 0
- [BUG-260728-3mx5hm_full-swift-tests.log](file://BUG-260728-3mx5hm/BUG-260728-3mx5hm_full-swift-tests.log) — Full node-app Swift test output: 370 tests in 59 suites, exit 0
- [BUG-260728-3mx5hm_release-build.log](file://BUG-260728-3mx5hm/BUG-260728-3mx5hm_release-build.log) — Optimized NodeApp Swift package build output, exit 0
- [BUG-260728-3mx5hm_spawn-log_-reviewer--reviewer--codex-_RUN-260728-7d8125.log](file://BUG-260728-3mx5hm/BUG-260728-3mx5hm_spawn-log_-reviewer--reviewer--codex-_RUN-260728-7d8125.log) — System spawn log captured by task-board
- [BUG-260728-3mx5hm_review-verdict-changes-requested.md](file://BUG-260728-3mx5hm/BUG-260728-3mx5hm_review-verdict-changes-requested.md) — Independent reviewer verdict and rework evidence
- [BUG-260728-3mx5hm_spawn-log_-implementer--developer--codex-_RUN-260728-3190d5.log](file://BUG-260728-3mx5hm/BUG-260728-3mx5hm_spawn-log_-implementer--developer--codex-_RUN-260728-3190d5.log) — System spawn log captured by task-board
- [BUG-260728-3mx5hm_rework-evidence.md](file://BUG-260728-3mx5hm/BUG-260728-3mx5hm_rework-evidence.md) — Reviewer rework implementation, behavioral coverage, and exit-code receipts
- [BUG-260728-3mx5hm_rework-focused-tests.log](file://BUG-260728-3mx5hm/BUG-260728-3mx5hm_rework-focused-tests.log) — Focused capture, consent, startup, workflow, and shell tests: 52 tests in 6 suites, exit 0
- [BUG-260728-3mx5hm_rework-full-swift-tests.log](file://BUG-260728-3mx5hm/BUG-260728-3mx5hm_rework-full-swift-tests.log) — Full NodeApp Swift suite: 378 tests in 60 suites, exit 0
- [BUG-260728-3mx5hm_rework-release-build.log](file://BUG-260728-3mx5hm/BUG-260728-3mx5hm_rework-release-build.log) — Optimized NodeApp release build: exit 0; pre-existing unrelated Sendable warnings only
- [BUG-260728-3mx5hm_spawn-log_-reviewer--reviewer--codex-_RUN-260728-041ab3.log](file://BUG-260728-3mx5hm/BUG-260728-3mx5hm_spawn-log_-reviewer--reviewer--codex-_RUN-260728-041ab3.log) — System spawn log captured by task-board
- [BUG-260728-3mx5hm_review-verdict-accepted.md](file://BUG-260728-3mx5hm/BUG-260728-3mx5hm_review-verdict-accepted.md) — Independent accepted reviewer verdict after rework
- [BUG-260728-3mx5hm_macos-install-receipt.md](file://BUG-260728-3mx5hm/BUG-260728-3mx5hm_macos-install-receipt.md) — Installed production-signed macOS capture-fix receipt

## Created
2026-07-27T23:44:08Z

## Last Update
2026-07-29T13:34:35Z

## Assigned To
[reviewer] reviewer (codex)
