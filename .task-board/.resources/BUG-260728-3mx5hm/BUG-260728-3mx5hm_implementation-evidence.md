# BUG-260728-3mx5hm — developer handoff evidence

Timestamp: 2026-07-28T04:35:17+04:00

## Outcome

- Replaced stringly recording failures with typed Core → workflow → SwiftUI failure models, so `capture_captureQualityUnsupported`, `capture_backendUnavailable`, OS error descriptions, and debug enum values cannot cross the presentation boundary.
- Added a native SwiftUI quality alert with localized English and Russian guidance:
  - **Use headphones** cancels the attempt, remains fail-closed, and selects the headphone quality mode.
  - **Allow this limited recording** grants degraded consent, then retries the same primary recording journey.
- Added explicit one-generation consent state. The grant resets when the backend closes that quality generation; if reset arrives while the workflow is still busy, it remains pending until the workflow returns idle. The “used for this attempt” marker prevents prompt loops after a consented failure.
- Added typed, redacted macOS capture-startup diagnostics and bounded VPIO recovery:
  - retry candidates are restricted to CoreAudio-domain status 35 and invalid/changing duplex layouts;
  - at most four attempts are made within a 125 ms window, with 25/35/45 ms delays;
  - only the initial voice-processing stage can fall back to unprocessed capture, and only with explicit consent;
  - input-selection failures, an already-consented fallback failure, unrelated engine errors, and an unrelated NSError whose numeric code happens to be 35 remain fail-closed;
  - terminal startup cleanup removes the tap, resets the engine/mailbox/quality state, and leaves no partial draft.
- Kept capture limits, privacy boundaries, AEC/NS/AGC truth reporting, Air routing, and secret redaction unchanged.

## Main implementation surfaces

- `node-app/Sources/NodeCore/MacCaptureStartupRecovery.swift`
- `node-app/Sources/NodeCore/MacAVAudioCaptureBackend.swift`
- `node-app/Sources/NodeCore/MacMicrophoneCaptureEngine.swift`
- `node-app/Sources/NodeCore/MacCaptureWorkflowController.swift`
- `node-app/Sources/NodeCore/MacCaptureQualityProcessor.swift`
- `node-app/Sources/NodeApp/MacCaptureAppComposition.swift`
- `node-app/Sources/NodeAppUI/PulsarShellModel.swift`
- `node-app/Sources/NodeAppUI/PulsarMainWindow.swift`
- `node-app/Sources/NodeApp/main.swift`

## Regression coverage

- Late-valid default-duplex aggregate after nested CoreAudio status 35.
- Repeated layout churn through the terminal bounded attempt.
- Typed/redacted engine diagnostics and unrelated numeric code 35.
- Explicit-consent-only fallback and no second fallback from non-VPIO stages.
- Built-in-speaker degraded route and accepted headphone route.
- Consent prompt retry, cancel-to-headphones, one-generation reset, and deferred reset while busy.
- English/Russian actionable copy and exhaustive safe labels for every typed recording failure.
- No normal draft event or retained/partial media after terminal startup failure.

## Green validation receipts

Every command below was run directly as a standalone process.

| Gate | Command | Exit | Result |
|---|---|---:|---|
| Focused Swift tests | `DEVELOPER_DIR=/Applications/Xcode_26_5.app/Contents/Developer xcrun swift test --filter 'MacCaptureStartupRecoveryTests\|MacCaptureQualityProcessorTests\|MacMicrophoneCaptureEngineTests\|MacCaptureWorkflowControllerTests\|PulsarShellModelTests'` | 0 | 44 tests in 5 suites passed |
| Full relevant suite | `DEVELOPER_DIR=/Applications/Xcode_26_5.app/Contents/Developer xcrun swift test` | 0 | 370 tests in 59 suites passed |
| Strict task-owned format lint | `xcrun swift-format lint --strict --configuration .temp/BUG-260728-3mx5hm/swift-format.json` over all changed Core/composition sources and Core capture tests | 0 | No diagnostics |
| Optimized package build | `DEVELOPER_DIR=/Applications/Xcode_26_5.app/Contents/Developer xcrun swift build -c release` | 0 | `NodeApp` linked successfully |
| Whitespace validation | `git diff --check -- <all task-touched Swift paths>` | 0 | No diagnostics |

The optimized build retains 12 existing Swift 6 sendability warnings in `PlayerCore.swift` and its payload types in `Protocol.swift`; none are in task-touched capture code.

## Non-green iteration receipts

- The first toolchain probe through the repository’s stale `/Applications/Xcode.app` path exited 1 because that path does not exist. All required gates were rerun with the installed `/Applications/Xcode_26_5.app/Contents/Developer`.
- The first compile check exited 1 on explicit-`self` closure capture and switch inference errors introduced during implementation. Those errors were corrected; the next compile check exited 0.
- Early focused runs exited 1 first on an invalid mutating call inside `#expect`, then on an over-broad source-string assertion plus an asynchronous event-order assertion. The tests and wait condition were corrected; all subsequent focused runs exited 0.
- Running `swift-format` with its unconfigured two-space default exited 1 because this repository uses four-space Swift indentation. A task-local four-space/120-column configuration was recorded and used for the green strict lint gate.
- Initial strict lint of the two new files exited 1, they were formatted mechanically, and the rerun exited 0.
- Whole-file diagnostic scans of already-dirty `PulsarShellModel.swift`, `PulsarMainWindow.swift`, `main.swift`, and `PulsarShellModelTests.swift` exited 1 on pre-existing unrelated semicolon, line-length, and wrapping debt. Those user-owned changes were not reformatted wholesale. The task additions themselves produced no matching diagnostics, compile in the full package, pass focused UI tests, and are covered by the exit-0 `git diff --check`.

## Evidence files

- `focused-tests-evidence.log`
- `full-swift-tests-evidence.log`
- `swift-format-evidence.log`
- `release-build-evidence.log`
- `git-diff-check-evidence.log`

The redacted production reproduction and CoreAudio timestamps remain in the task notes; no audio, device secret, invite, credential, or payload content was added to this artifact.
