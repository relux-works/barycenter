# BUG-260729-3kecpr developer evidence

## Outcome

Ordinary recorded clips and the local self-test now compose a microphone-only
`AVAudioEngine` backend whose platform seam exposes input operations only. It
does not access `outputNode`, enable voice processing, resolve an output route,
or implement the capture-quality configurator. The former VPIO/default-duplex
backend remains available as `MacAVAudioFullDuplexCaptureBackend` for workflows
that explicitly require simultaneous input/output processing.

Accountless capture also defers construction and startup of its cue/output graph
until the input-only backend has started and requests the start cue. An output
startup failure is therefore reported as a cue failure and removes the partial;
it cannot disable Record before the user presses it.

Microphone persistence now stores the stable Core Audio device UID in
`captureInputDeviceUID.v2`. Numeric `captureInputDevice.v1` values are cleared
and fall back to the current system default; a nonnumeric prerelease v1 UID is
migrated only when the device is currently present. If an explicitly selected
microphone disappears between enumeration and Core Audio attachment, the
diagnostic is logged and the UI receives the dedicated selected-device recovery
instead of generic startup copy.

The ordinary UI is Record → recording → Stop → draft. Its window/menu surfaces
no quality, AEC, VPIO, limited-recording, headphone, or output-route controls.
Input-only failures have localized EN/RU recovery copy. Runtime failure and
startup cleanup remain terminal and remove partial media.

## Deterministic coverage

- Built-in 48 kHz input with unrelated 44.1 kHz output and retained production
  OSStatus `560227702` (`!dev`) receipt.
- No output-node, VPIO, output-route, or quality-configurator surface on the
  ordinary backend; full-duplex source retains those contracts.
- Stable UID resolution across numeric `AudioDeviceID` churn (`92` → `429`).
- Numeric and string legacy ID clearing, valid prerelease UID migration, stale
  external selection fallback, and stable UID persistence.
- Genuine input-only `!dev` typing, nonretryability, and
  stop/remove-tap/reset cleanup ordering.
- Output startup deferred until the start-cue request; a deterministic failing
  output factory proves input begins first, then yields `cuePlaybackFailed` with
  cancellation and no draft.
- Selected external microphone disappearance during input attachment after
  successful discovery, with typed recovery and cleanup.
- Runtime failure cleanup and no partial draft for default and explicitly
  selected microphones.
- One-button recording state, visible Stop ownership, and input-only EN/RU copy.
- Existing live/full-duplex fail-closed, safety ceiling, privacy, Air, cue
  ordering, capture limits, and no-partial-draft suites remain green.

## Validation receipts

| Gate | Exit | Receipt |
| --- | ---: | --- |
| Focused Swift Testing gate | 0 | Unlocked-console rerun passed 66 tests / 8 suites on the revised source state |
| Full `xcrun swift test --package-path node-app` | 0 | Unlocked-console rerun passed all 390 tests / 62 suites |
| Earlier locked-console full Swift attempt | 1 | All 390 tests ran; exactly 3 unrelated `PhaseOneDraftOutboxTests` failed `.persistence` while `IOConsoleLocked=true`. A direct write matrix reproduced EPERM only for `.completeFileProtection`; this expected-red attempt is not counted as passing |
| Strict task-owned `xcrun swift-format lint --strict --configuration .temp/BUG-260729-3kecpr/swift-format.json` | 0 | Unlocked-console rerun produced no diagnostics across the new backend split, selection store, startup diagnostic, composition, recording bar, and focused Core/UI tests |
| Scoped `git diff --check` | 0 | Unlocked-console rerun found no whitespace errors in task paths |
| `xcrun swift build --package-path node-app -c release` | 0 | Unlocked-console rerun linked the optimized `NodeApp` |
| First app packaging attempt | 1 | Expected guard failure: default local `go-librespot` lacked `PULSAR_ZEROCONF_HOST`; guard was not weakened |
| Revised production packaging with the installed known-good helper | 0 | Pulsar 0.3.0 (958.4), hardened runtime, Developer ID Application: Relux Works, LLC (262RZ595FP) |
| Candidate `codesign --verify --deep --strict --verbose=2` | 0 | Valid on disk and satisfies designated requirement |
| Candidate `spctl --assess --type execute --verbose=4` | 3 | Rejected as `Unnotarized Developer ID`; local ASC notarization credentials are absent |
| Installed `/Applications/Pulsar.app` deep/strict code-sign check | 0 | Reverified after smoke: Developer-ID signed 0.3.0 (958.4), Team ID `262RZ595FP` |
| Candidate/installed NodeApp SHA-256 comparison | 0 | Both `dc022c30cd3f7840324232d73df6bf044471e5f2b5063f39c768aec4f8f5f137` |

Early formatter probes without the task's established 4-space/120-column
configuration returned exit 1, as did whole-file probes of legacy large shell
files with unrelated pre-existing formatting debt. Those files were not
mass-reformatted because the shared worktree contains other owners' changes.
The task-owned/new files pass the configured strict gate, the ordinary UI
source-contract tests execute against the legacy files, and the scoped diff
scan passes.

## Signed install and physical Mac smoke

The independently reviewed output-deferral revision is installed and was
exercised on the physical Mac as Pulsar 0.3.0 (958.4).

- Backed up the prior 0.3.0 (958.2) installation under this task's temporary
  directory. The first input-only smoke used 958.3; the revised 958.4 candidate
  was then installed and reverified before the handoff smoke.
- `system_profiler SPAudioDataType` reported:
  - MacBook Pro Microphone: default input, 1 channel, 48,000 Hz, built-in.
  - MacBook Pro Speakers: default output, 2 channels, 44,100 Hz, built-in.
- Neither `captureInputDevice.v1` nor `captureInputDeviceUID.v2` remained
  persisted, so the current healthy default microphone was selected.
- The visible toolbar Record action was enabled. Pressing it changed the
  control to Stop recording and exposed the plain recording bar. There was no
  alert, sheet, quality prompt, or route objection.
- Pressing Stop returned the control to Start recording and produced one
  `Ready to send` draft.
- `afinfo` verified the 958.4 draft as mono 48,000 Hz Int16 WAVE, 21 seconds,
  2,016,000 audio bytes. The partials directory was empty.
- A transient authenticated-app-data refresh error initially masked the draft
  card. A clean relaunch restored the coordinator view and displayed the same
  durable `Pulsar recording` card, proving the local draft survived restart.
- Targeted unified-log review found no `!dev`, `560227702`, DDAgg failure, or
  voice-processing/VPIO event during the 958.4 recording. The first log query
  used an invalid spaced UTC-offset spelling and exited 64; the corrected
  timestamp query exited 0 with no matching events.
- After preserving screenshots and format metadata, the developer-created
  smoke draft was removed through Pulsar's own `Delete local draft` action.
  No pre-existing user draft was present or touched, and CaptureMedia contained
  no draft or partial afterward.

The local app is Developer-ID signed but not notarized; the repository's
notarization path requires CI-held ASC key material unavailable in this
environment. This does not affect the in-place signed smoke installation, but
Gatekeeper acceptance of a quarantined distribution image remains a release
workflow gate.

## Resumed gate resolution

The physical console was unlocked on 2026-07-30. Without any product flag,
test-only temp override, file-protection weakening, or capture workaround, the
previously blocked full suite passed and the revised signed 958.4 Record → Stop
→ durable draft smoke succeeded.

## Independent review

The first independent review requested two changes: defer accountless
cue/output graph startup so output cannot disable Record, and retain selected
device recovery when the device disappears during Core Audio attachment. Both
were implemented with deterministic regressions. The independent re-review
reported no remaining code or acceptance findings and approved the revised
scope.
