# P1.0 Windows Store platform spike decomposition

## Current implementation findings

- `pulsar-win/msix/AppxManifest.xml.in` already uses
  `TrustLevel="appContainer"` and `RuntimeBehavior="packagedClassicApp"`, but
  it only declares networking capabilities. There is no microphone capability
  declaration yet.
- `pulsar-win/audio_windows.go` implements render-only WASAPI output. The repo
  does not contain a microphone capture path, selected-input capture flow,
  file-picker bridge, or permission-revoke handling.
- `pulsar-win/ui_windows.go` provides onboarding and tray plumbing, but there
  is no `RegisterHotKey` path and no recording lifecycle integrated into that
  message loop.
- The current MSIX workflow in `.github/workflows/release.yml` packs an MSIX
  package, but the signed probe route for reproducible local Windows 10 and 11
  evidence still needs to be made explicit.

## Child tasks

1. `TASK-260712-6kba80` selects the legal capture and picker bridge and records
   the manifest and sandbox contract.
2. `TASK-260712-dib11l` implements the smallest packaged probe that exercises
   permission, default and selected capture, hotkey, picker and hidden-window
   behavior.
3. `TASK-260712-2y74io` hardens lifecycle handling for quit, suspend, session
   lock and permission revoke.
4. `TASK-260712-13rbnw` packages and documents the smallest signed MSIX probe.
5. `TASK-260712-1vtwkl` runs the real Windows 10 and Windows 11 matrix and
   publishes the final pass or fail evidence with next actions.

Within-story dependencies:

- `TASK-260712-dib11l` is blocked by `TASK-260712-6kba80`.
- `TASK-260712-2y74io` is blocked by `TASK-260712-dib11l`.
- `TASK-260712-13rbnw` is blocked by `TASK-260712-dib11l`.
- `TASK-260712-1vtwkl` is blocked by both `TASK-260712-2y74io` and
  `TASK-260712-13rbnw`.

## Cross-story handoff

- `STORY-260712-2e36uz` should consume the selected capture and picker bridge,
  manifest declarations, hotkey approach and lifecycle constraints before
  Windows capture or file-input implementation is written there.
- `STORY-260712-1i0doc` should consume the signed-probe packaging notes and the
  Windows 10 and Windows 11 evidence matrix for microphone capability,
  certification notes and A1 or A8 proof.
- `STORY-260712-sskhip` and `STORY-260712-3pt00e` are not phase-one blockers,
  but they should reuse the selected capture bridge and any recorded lock or
  revoke limitations when phase-three hold-to-talk and capture-quality work
  starts.

## External prerequisites called out explicitly

- Real hardware is still required: at least one Windows 10 machine and one
  Windows 11 machine.
- The signed-artifact route must stay within the current AppContainer and
  packagedClassicApp posture. Any need for `runFullTrust`, broad filesystem
  capability or another sandbox relaxation must be escalated as a separate
  decision, not folded into the spike.
