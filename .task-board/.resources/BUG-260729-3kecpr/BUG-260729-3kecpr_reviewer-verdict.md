# BUG-260729-3kecpr reviewer verdict

Verdict: **accepted**

Review run: `RUN-260730-3ec3ce`  
Date: 2026-07-30  
Goal binding: none

## Acceptance evidence

- The production ordinary-clip composition constructs `MacAVAudioCaptureBackend`, whose system session is input-only: it selects and reads the microphone input, installs/removes the input tap, and prepares/starts/stops/resets the engine. It has no output-node, VPIO, voice-processing, quality-consent, or output-route dependency.
- The former VPIO implementation remains isolated as `MacAVAudioFullDuplexCaptureBackend` for actual simultaneous input/output workflows and retains fail-closed processing and bounded recovery policy.
- Input selection persists `captureInputDeviceUID.v2`; legacy numeric `captureInputDevice.v1` values are cleared, valid prerelease UID values migrate only while present, and stale stable selections safely fall back to the current default.
- The ordinary UI is Record -> recording -> Stop -> durable draft. Quality/AEC/VPIO controls and output-route recovery language are absent from this journey; EN/RU recovery text distinguishes permission, missing/default input, selected-device loss, input-only startup, storage, and cue failures.
- Startup/runtime failure paths stop and reset capture, remove owned partials, and cannot publish a partial draft. Output construction is deferred to cue playback and therefore cannot gate microphone startup.
- The reviewed deterministic tests cover 48 kHz built-in input with unrelated 44.1 kHz output and the production `!dev` (560227702) receipt, stable UID resolution after numeric device churn, stale selection, no-output dependency, genuine input-only failure cleanup, no-partial-draft behavior, EN/RU copy, and the one-button workflow.

## Independent validation

- `xcrun swift test --package-path node-app --filter '(MacAVAudioInputCaptureBackendTests|MacCaptureInputSelectionStoreTests|MacCaptureWorkflowControllerTests|MacMicrophoneCaptureEngineTests|MacCaptureStartupRecoveryTests|MacCaptureConsentCoordinatorTests|PulsarShellModelTests|PulsarCaptureQualityUISourceTests)'`: 61 tests in 8 suites passed.
- `xcrun swift test --package-path node-app`: 390 tests in 62 suites passed.
- Strict `swift-format` lint passed for all task-owned capture sources, dedicated UI source, and focused tests. Shared large files contain unrelated pre-existing/foreign-worktree whole-file formatter findings; their scoped `git diff --check` passed with no task-diff whitespace errors.
- `xcrun swift build --package-path node-app -c release`: passed.
- `/Applications/Pulsar.app`: version 0.3.0 (958.4), `codesign --verify --deep --strict` passed; Developer ID Application `Relux Works, LLC (262RZ595FP)`; NodeApp SHA-256 `dc022c30cd3f7840324232d73df6bf044471e5f2b5063f39c768aec4f8f5f137`.
- Producer physical-Mac evidence was inspected: built-in microphone at 48 kHz with 44.1 kHz output recorded a 21-second mono 48 kHz Int16 WAV, survived relaunch as a durable draft, showed no quality prompt, and produced no `!dev`, DDAgg, or VPIO console receipt. The 958.4 recording and draft screenshots corroborate the one-button journey.

No correctness, architecture, safety, privacy, localization, or regression defect requiring rework was found.
