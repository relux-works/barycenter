# P3 Capture quality and diagnostics decomposition

## Spec slices reviewed

- `docs/spec-self-contained-audio.md` sections 7.3, 10.3, 11.3, 16, 17, 18,
  21.1-21.5 and 22-23
- `docs/goal-self-contained-audio.md`
- `docs/spec.md` and `docs/protocol.md` for current shipped audio and state
  constraints

## Current implementation snapshot

- Windows currently has playback, state heartbeats and speaker-name reporting,
  but no live PTT transport, no microphone DSP path, no capture diagnostics
  model, no AEC or AGC path, and no honest degraded or unsupported surface.
- macOS currently has playback, output-device monitoring and degraded speaker
  reporting, but no live PTT transport, no microphone DSP path, no route-aware
  AEC or AGC pipeline, and no input-health or capability surface.
- The shared state model today only exposes playback, volume, degraded and
  underruns. There is no protocol or UI vocabulary yet for route mode, capture
  effect state, input health, or C3 evidence claims.

## Story tasks created

- `TASK-260712-1gmsvh` Freeze the phase-three capture-quality contract and
  failure policy
- `TASK-260712-265o0f` Prove the Windows AppContainer voice-processing path
- `TASK-260712-2gaswa` Prove the macOS voice-processing and route-detection path
- `TASK-260712-wcdz08` Implement Windows live capture effects and bounded AGC
- `TASK-260712-2egweh` Implement macOS live capture effects and bounded AGC
- `TASK-260712-1pw1l1` Wire capture diagnostics and honest capability surfaces
- `TASK-260712-39czd2` Build the capture-quality regression harness and fixtures
- `TASK-260712-2e80pr` Run the C3 matrix and publish the honest capability pack

## Within-story dependency chain

- Shared contract first: `TASK-260712-1gmsvh` freezes the product, protocol and
  evidence vocabulary before platform code diverges.
- Platform research next: `TASK-260712-265o0f` and `TASK-260712-2gaswa`
  validate the Windows and macOS voice-processing paths in parallel.
- Platform implementation after proof: `TASK-260712-wcdz08` and
  `TASK-260712-2egweh` extend the live capture path with AEC, noise suppression
  and bounded AGC under the approved route modes.
- Shared diagnostics after both platforms: `TASK-260712-1pw1l1` wires the
  honest state surfaces only after both platform implementations exist.
- Verification closes the story: `TASK-260712-39czd2` creates the rerunnable
  C3 harness, then `TASK-260712-2e80pr` publishes the final evidence and
  capability matrix.

## Cross-story dependencies to track

- `STORY-260712-30ju1k` P1 Windows packaged-app spike
  - Blocks `TASK-260712-265o0f` because Windows phase-three effect claims must
    be proven inside the signed packagedClassicApp and AppContainer posture.
- `STORY-260712-2e36uz` P1 main UI and capture foundation
  - Blocks both platform implementation tasks because route pickers, explicit
    capture indication, bounded capture lifecycle and capture-service seams do
    not exist yet.
- `STORY-260712-fes2jj` P1 overlay and local ceiling mixer
  - Blocks both platform implementation tasks because phase-three AGC must keep
    local ceiling ordering and live duck behavior consistent with the mixer.
- `STORY-260712-sskhip` P3 live PTT transport
  - Blocks both platform implementation tasks and the regression harness
    because the quality work must run on the real chunked live-stream path and
    exercise packet-loss or cancellation behavior.
- `STORY-260712-2ft5wd` P3 acceptance and rollout
  - Is blocked by `TASK-260712-2e80pr`, which must publish the objective and
    listening-test capability pack before phase-three acceptance can close.

## Completeness check

- Covered:
  - shared product and protocol contract for capture-quality states
  - explicit Windows and macOS voice-processing research tasks
  - platform implementation tasks for AEC, noise suppression, AGC and route
    modes
  - shared diagnostics and honest capability surfacing
  - rerunnable regression harness plus final evidence or rollout handoff
- Gaps explicitly closed with blocking research:
  - Windows AppContainer-safe voice-processing support is still unproven
  - macOS route-detection and voice-processing quirks across built-in, wired,
    Bluetooth and external devices are still unproven
  - mixed-version and unsupported-target wording had no shared contract
- Deferred to sibling stories, not forgotten:
  - global hold key-down or key-up behavior and live chunk transport
  - end-to-end encryption media design and review
  - automation and soundboard controls
  - final phase-three acceptance gate, beta soak and security-review closure
